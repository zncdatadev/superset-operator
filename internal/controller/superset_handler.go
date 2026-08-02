package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/security"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/util/version"
)

// Compile-time proof that SupersetCluster wires framework-owned aggregator discovery:
// the GenericReconciler reads the ConfigMap name, resolves the ADDRESS key, and hands
// the address to the handler as buildCtx.VectorAggregatorAddress.
var _ reconciler.VectorAggregatorProvider = (*supersetv1alpha1.SupersetCluster)(nil)

// RBAC for the resources the SDK GenericReconciler owns on behalf of a SupersetCluster.
// The GenericReconciler watches the CR plus every kind it Owns() (StatefulSet, Service,
// ConfigMap, ServiceAccount, PodDisruptionBudget), lists Pods for health status, deletes
// orphaned PVCs when a role group shrinks, and records events — the manager ClusterRole
// has to cover all of them or the informers fail to start. AuthenticationClasses are read
// to render LDAP/OIDC configuration. Regenerate config/rbac/role.yaml via `make manifests`.
//
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=authentication.kubedoop.dev,resources=authenticationclasses,verbs=get;list;watch

const (
	portHTTPName    = "http"
	portMetricsName = "metrics"
	httpPort        = int32(8088)
	metricsPort     = int32(9102)

	metricsContainerName = "metrics"

	// maxLogFileSize bounds the shared log emptyDir the Vector sidecar provider creates.
	maxLogFileSize = "10Mi"
)

// credentialsKeyMapping maps container environment variables to keys of the credentials
// secret named in clusterConfig.credentialsSecret.
var credentialsKeyMapping = [][]string{
	{"ADMIN_USERNAME", "adminUser.username"},
	{"ADMIN_FIRSTNAME", "adminUser.firstname"},
	{"ADMIN_LASTNAME", "adminUser.lastname"},
	{"ADMIN_EMAIL", "adminUser.email"},
	{"ADMIN_PASSWORD", "adminUser.password"},
	{"SECRET_KEY", "appSecretKey"},
	{"SQLALCHEMY_DATABASE_URI", "connections.sqlalchemyDatabaseUri"},
}

// SupersetRoleGroupHandler builds Superset role group resources. It embeds the SDK's
// BaseRoleGroupHandler so the framework owns the bulk of resource orchestration —
// ConfigMap, Services, the StatefulSet skeleton (sidecars and podOverrides applied by
// the framework), the role-level PDB and the ServiceAccount. Product-specific behavior
// is declared BEFORE the framework builds, through the per-call build context:
// MainContainerCustomizer for the entrypoint/env/probes, ListenerClass for the Service
// type, VolumeProviders for LDAP credentials and the SidecarManager for the Vector
// agent — so user podOverrides keep precedence over product defaults.
type SupersetRoleGroupHandler struct {
	*reconciler.BaseRoleGroupHandler[*supersetv1alpha1.SupersetCluster]
}

// Ensure interface implementation.
var _ reconciler.RoleGroupHandler[*supersetv1alpha1.SupersetCluster] = &SupersetRoleGroupHandler{}

// NewSupersetRoleGroupHandler creates the handler and configures the reconcile-invariant
// framework defaults.
func NewSupersetRoleGroupHandler(scheme *runtime.Scheme) *SupersetRoleGroupHandler {
	base := reconciler.NewBaseRoleGroupHandler[*supersetv1alpha1.SupersetCluster]("", scheme)

	// ProductName names the product (app.kubernetes.io/name, the version label and the
	// repository path segment); ImageDefaults fills whatever spec.image leaves empty,
	// evaluated every reconcile. KubedoopVersion defaults to the operator's own build
	// version — quay.io/zncdatadev publishes no bare product tags, and a webhook default
	// would freeze the suffix at the admitting operator's version.
	base.ProductName = supersetv1alpha1.DefaultProductName
	base.ImageDefaults = commonsv1alpha1.ImageSpec{
		Repo:            supersetv1alpha1.DefaultRepository,
		ProductVersion:  supersetv1alpha1.DefaultProductVersion,
		KubedoopVersion: version.BuildVersion,
	}

	// The main container keeps the Gen 2 name "node" — rendered resource selectors and
	// downstream tooling reference it.
	base.MainContainerName = supersetv1alpha1.RoleNameNode

	base.SetRoleContainerPorts(supersetv1alpha1.RoleNameNode, []corev1.ContainerPort{
		{Name: portHTTPName, ContainerPort: httpPort, Protocol: corev1.ProtocolTCP},
		{Name: portMetricsName, ContainerPort: metricsPort, Protocol: corev1.ProtocolTCP},
	})
	base.SetRoleServicePorts(supersetv1alpha1.RoleNameNode, []corev1.ServicePort{
		{Name: portHTTPName, Port: httpPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(portHTTPName)},
		{Name: portMetricsName, Port: metricsPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(portMetricsName)},
	})

	return &SupersetRoleGroupHandler{BaseRoleGroupHandler: base}
}

// BuildResources declares the product-specific inputs on the per-call build context,
// delegates the framework-owned skeleton, then appends only what no pre-build hook
// covers: the statsd-exporter container, the Prometheus scrape annotations and the
// rendered ConfigMap files.
func (h *SupersetRoleGroupHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	authProvider, err := fetchAuthProvider(ctx, k8sClient, cr)
	if err != nil {
		return nil, err
	}
	var ldap *authv1alpha1.LDAPProvider
	if authProvider != nil {
		ldap = authProvider.LDAP
	}

	// LDAP bind credentials arrive through a secret-operator CSI volume, injected by the
	// framework's VolumeProvider path alongside the config volume. Mounted under
	// /kubedoop/secret (the kubedoop convention superset_config.py reads from), not the
	// provisioner's /kubedoop/mount default.
	if ldap != nil && ldap.BindCredentials != nil {
		provisioner := security.NewSecretProvisioner().WithMountBasePath(constant.KubedoopSecretDir)
		registration := security.CredentialsVolume(ldap.BindCredentials.SecretClass, ldap.BindCredentials.SecretClass)
		if scope := secretScope(ldap.BindCredentials.Scope); scope != "" {
			registration = registration.WithScope(scope)
		}
		provisioner.Register(registration)
		buildCtx.VolumeProviders = append(buildCtx.VolumeProviders, provisioner)
	}

	// Listener class: the CR's clusterConfig value, else the product's historical default
	// (the Gen 2 operator hardcoded external-unstable, which ServiceTypeFor maps to NodePort).
	listenerClass := listener.ListenerClassExternalUnstable
	if cr.Spec.ClusterConfig != nil && cr.Spec.ClusterConfig.ListenerClass != "" {
		listenerClass = listener.ListenerClass(cr.Spec.ClusterConfig.ListenerClass)
	}
	buildCtx.ListenerClass = listenerClass

	// The framework gates the Vector log pipeline on enableVectorAgent AND a resolvable
	// aggregator address; ride that gate so the sidecar, the vector.yaml ConfigMap entry and
	// the log_config.py file appender always agree. The provider's image is left empty: the
	// framework resolves the product image once and propagates it to every sidecar config.
	vectorActive := buildCtx.VectorLogPipelineActive != nil && *buildCtx.VectorLogPipelineActive
	if vectorActive {
		buildCtx.SidecarManager.Register(vector.NewVectorSidecarProvider(
			"",
			vector.WithConfigMapName(buildCtx.ResourceName),
			vector.WithProducers([]string{supersetv1alpha1.RoleNameNode}),
			vector.WithLogVolumeSize(resource.MustParse(maxLogFileSize)),
		), nil)
	}

	// Main container customization runs BEFORE podOverrides are strategic-merged, so user
	// overrides keep precedence over these product defaults.
	buildCtx.MainContainerCustomizer = func(c *corev1.Container) error {
		c.Command = []string{"sh", "-x", "-c"}
		c.Args = []string{mainContainerCommands()}

		env := []corev1.EnvVar{
			{Name: "SUPERSET_PORT", Value: strconv.FormatInt(int64(httpPort), 10)},
		}
		if cr.Spec.ClusterConfig != nil && cr.Spec.ClusterConfig.CredentialsSecret != "" {
			env = append(env, credentialsEnv(cr.Spec.ClusterConfig.CredentialsSecret)...)
		}
		c.Env = append(c.Env, env...)

		if authProvider != nil && authProvider.OIDC != nil &&
			cr.Spec.ClusterConfig != nil && cr.Spec.ClusterConfig.Authentication != nil &&
			cr.Spec.ClusterConfig.Authentication.Oidc != nil {
			c.EnvFrom = append(c.EnvFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.ClusterConfig.Authentication.Oidc.ClientCredentialsSecret,
					},
				},
			})
		}

		// Superset serves its health endpoint over HTTP; replace the framework's TCP
		// readiness probe with the product check for both liveness and readiness.
		probe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt32(httpPort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}
		c.LivenessProbe = probe
		c.ReadinessProbe = probe
		return nil
	}

	resources, err := h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	if err != nil {
		return nil, err
	}

	addMetricsContainer(resources.StatefulSet)
	addPrometheusAnnotations(resources.Service)
	if err := renderConfigMapData(resources.ConfigMap, buildCtx, authProvider, cr.Spec.ClusterConfig.Authentication, vectorActive); err != nil {
		return nil, err
	}

	return resources, nil
}

// addMetricsContainer appends the statsd-exporter sidecar: Superset emits StatsD
// counters over UDP and the exporter serves them as Prometheus metrics on the
// Service's named "metrics" port. It reuses the resolved product image (read back from
// the framework-built main container) and runs as a regular container, matching the
// Gen 2 pod shape. There is no pre-build hook for extra regular containers — a
// podOverrides patch naming "metrics" would collide; the Gen 2 operator had the same
// theoretical edge and no user hit it.
func addMetricsContainer(sts *appsv1.StatefulSet) {
	if sts == nil {
		return
	}
	main := findContainer(&sts.Spec.Template.Spec, supersetv1alpha1.RoleNameNode)
	if main == nil {
		return
	}
	sts.Spec.Template.Spec.Containers = append(sts.Spec.Template.Spec.Containers, corev1.Container{
		Name:            metricsContainerName,
		Image:           main.Image,
		ImagePullPolicy: main.ImagePullPolicy,
		Command:         []string{"sh", "-x", "-c"},
		Args:            []string{metricsContainerCommands()},
	})
}

// addPrometheusAnnotations advertises the metrics endpoint to Prometheus scrapers
// unconditionally, matching the Gen 2 operator's always-on Service annotations. The
// Service type itself is framework-owned via buildCtx.ListenerClass.
func addPrometheusAnnotations(svc *corev1.Service) {
	if svc == nil {
		return
	}
	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}
	svc.Annotations["prometheus.io/scrape"] = "true"
	svc.Annotations["prometheus.io/port"] = strconv.FormatInt(int64(metricsPort), 10)
	svc.Annotations["prometheus.io/path"] = "/metrics"
	svc.Annotations["prometheus.io/scheme"] = "http"
}

// renderConfigMapData fills the framework-built ConfigMap with the product files:
// superset_config.py (with the resolved authentication section), log_config.py (framework
// python dictConfig writing /kubedoop/log/superset/superset.py.json) and, when the Vector
// pipeline is active, vector.yaml rendered from the shared framework template.
func renderConfigMapData(
	cm *corev1.ConfigMap,
	buildCtx *reconciler.RoleGroupBuildContext,
	authProvider *authv1alpha1.AuthenticationProvider,
	auth *supersetv1alpha1.AuthenticationSpec,
	vectorActive bool,
) error {
	if cm == nil {
		return nil
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	// superset_config.py is a free-form Python module, not key=value, so it is rendered
	// here as a whole file rather than flowing through the merge pipeline. configOverrides
	// for it were never supported (the e2e case for it is commented out upstream), and
	// writing unconditionally keeps that behavior.
	cm.Data[supersetConfigFilename] = renderSupersetConfig(authProvider, auth, vectorActive)

	_, logContent, err := reconciler.RenderContainerLogging(buildCtx, productlogging.ContainerLogging{
		Container:   supersetLogContainer,
		Framework:   productlogging.LoggingFrameworkPython,
		LogFileName: supersetLogFileName,
	})
	if err != nil {
		return fmt.Errorf("render log config: %w", err)
	}
	cm.Data[logConfigFilename] = logContent

	if vectorActive {
		vectorYaml, err := vector.RenderVectorConfig(vector.VectorConfigData{
			LogDir:            constant.KubedoopLogDir,
			AggregatorAddress: buildCtx.VectorAggregatorAddress,
			Namespace:         buildCtx.ClusterNamespace,
			ClusterName:       buildCtx.ClusterName,
			RoleName:          buildCtx.RoleName,
			RoleGroupName:     buildCtx.RoleGroupName,
		})
		if err != nil {
			return fmt.Errorf("render vector config: %w", err)
		}
		cm.Data[vector.VectorConfigFileName] = vectorYaml
	}

	return nil
}

// fetchAuthProvider resolves the AuthenticationClass referenced by the cluster config,
// returning nil when authentication is not configured.
func fetchAuthProvider(
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
) (*authv1alpha1.AuthenticationProvider, error) {
	if cr.Spec.ClusterConfig == nil || cr.Spec.ClusterConfig.Authentication == nil ||
		cr.Spec.ClusterConfig.Authentication.AuthenticationClass == "" {
		return nil, nil
	}
	authClass := &authv1alpha1.AuthenticationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.ClusterConfig.Authentication.AuthenticationClass,
			Namespace: cr.Namespace,
		},
	}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(authClass), authClass); err != nil {
		return nil, fmt.Errorf("fetch AuthenticationClass %q: %w", authClass.Name, err)
	}
	return authClass.Spec.AuthenticationProvider, nil
}

// secretScope translates the AuthenticationClass bind-credentials scope into the CSI
// annotation value. An unset scope yields "" so no annotation is rendered, matching the
// Gen 2 behavior (secret-operator applies its own default).
func secretScope(scope *commonsv1alpha1.CredentialsScope) string {
	if scope == nil {
		return ""
	}
	parts := make([]string, 0, 2+len(scope.Services))
	if scope.Node {
		parts = append(parts, "node")
	}
	if scope.Pod {
		parts = append(parts, "pod")
	}
	parts = append(parts, scope.Services...)
	return strings.Join(parts, ",")
}

// credentialsEnv maps the credentials secret keys onto the container environment the
// Superset entrypoint script consumes.
func credentialsEnv(credentialsSecret string) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0, len(credentialsKeyMapping))
	for _, pair := range credentialsKeyMapping {
		env = append(env, corev1.EnvVar{
			Name: pair[0],
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: pair[1],
					LocalObjectReference: corev1.LocalObjectReference{
						Name: credentialsSecret,
					},
				},
			},
		})
	}
	return env
}

func findContainer(podSpec *corev1.PodSpec, name string) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			return &podSpec.Containers[i]
		}
	}
	return nil
}
