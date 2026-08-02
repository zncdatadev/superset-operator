package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"
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
// the framework), the role-level PDB and the ServiceAccount. The override below only
// adds the product-specific bits the merge pipeline cannot model declaratively.
type SupersetRoleGroupHandler struct {
	*reconciler.BaseRoleGroupHandler[*supersetv1alpha1.SupersetCluster]
}

// Ensure interface implementation.
var _ reconciler.RoleGroupHandler[*supersetv1alpha1.SupersetCluster] = &SupersetRoleGroupHandler{}

// NewSupersetRoleGroupHandler creates the handler and configures the framework defaults.
func NewSupersetRoleGroupHandler(scheme *runtime.Scheme) *SupersetRoleGroupHandler {
	base := reconciler.NewBaseRoleGroupHandler[*supersetv1alpha1.SupersetCluster]("", scheme)

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

// BuildResources delegates the framework-owned skeleton, then appends the product-specific
// pieces: the resolved product image, the Superset entrypoint and credentials wiring, the
// statsd-exporter metrics sidecar, LDAP bind-credential volumes, the NodePort service type
// with Prometheus scrape annotations, and the rendered superset_config.py / log_config.py /
// vector.yaml ConfigMap entries.
func (h *SupersetRoleGroupHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	image := resolveImage(cr)
	h.Image = image
	h.ImagePullPolicy = resolvePullPolicy(cr)

	// Without ProductName the framework leaves app.kubernetes.io/name unset; stamp it so
	// metadata labels stay continuous with the Gen 2 operator's rendered resources.
	buildCtx.ClusterLabels[constant.LabelKubernetesName] = supersetv1alpha1.DefaultProductName

	authProvider, err := fetchAuthProvider(ctx, k8sClient, cr)
	if err != nil {
		return nil, err
	}
	var ldap *authv1alpha1.LDAPProvider
	if authProvider != nil {
		ldap = authProvider.LDAP
	}

	// LDAP bind credentials arrive through a secret-operator CSI volume. Registered before
	// the base build so the volume and its main-container mount flow through the framework's
	// VolumeProvider path.
	if ldap != nil && ldap.BindCredentials != nil {
		provisioner := security.NewSecretProvisioner()
		registration := security.CredentialsVolume(ldap.BindCredentials.SecretClass, ldap.BindCredentials.SecretClass)
		if scope := secretScope(ldap.BindCredentials.Scope); scope != "" {
			registration = registration.WithScope(scope)
		}
		provisioner.Register(registration)
		buildCtx.VolumeProviders = append(buildCtx.VolumeProviders, provisioner)
	}

	// The framework gates the Vector log pipeline on enableVectorAgent AND a resolvable
	// aggregator address; ride that gate so the sidecar, the vector.yaml ConfigMap entry and
	// the log_config.py file appender always agree.
	vectorActive := buildCtx.VectorLogPipelineActive != nil && *buildCtx.VectorLogPipelineActive
	if vectorActive {
		buildCtx.SidecarManager.Register(vector.NewVectorSidecarProvider(
			image,
			vector.WithConfigMapName(buildCtx.ResourceName),
			vector.WithProducers([]string{supersetv1alpha1.RoleNameNode}),
			vector.WithLogVolumeSize(resource.MustParse(maxLogFileSize)),
		), nil)
	}

	resources, err := h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	if err != nil {
		return nil, err
	}

	if err := h.customizeStatefulSet(resources.StatefulSet, cr, authProvider, image); err != nil {
		return nil, err
	}
	customizeService(resources.Service)
	if err := renderConfigMapData(resources.ConfigMap, buildCtx, authProvider, cr.Spec.ClusterConfig.Authentication, vectorActive); err != nil {
		return nil, err
	}

	return resources, nil
}

// customizeStatefulSet mutates only the product-specific parts of the framework-built
// StatefulSet: the main container entrypoint, probes and environment, plus the
// statsd-exporter metrics sidecar.
func (h *SupersetRoleGroupHandler) customizeStatefulSet(
	sts *appsv1.StatefulSet,
	cr *supersetv1alpha1.SupersetCluster,
	authProvider *authv1alpha1.AuthenticationProvider,
	image string,
) error {
	podSpec := &sts.Spec.Template.Spec

	main := findContainer(podSpec, supersetv1alpha1.RoleNameNode)
	if main == nil {
		return fmt.Errorf("main container %q not found in built StatefulSet %s/%s",
			supersetv1alpha1.RoleNameNode, sts.Namespace, sts.Name)
	}

	main.Command = []string{"sh", "-x", "-c"}
	main.Args = []string{mainContainerCommands()}

	// Product environment. Appended after the framework-injected merged config so user
	// envOverrides (injected by the framework) keep their position; the names here never
	// collide with user overrides in practice.
	env := []corev1.EnvVar{
		{Name: "SUPERSET_PORT", Value: strconv.FormatInt(int64(httpPort), 10)},
	}
	if cr.Spec.ClusterConfig != nil && cr.Spec.ClusterConfig.CredentialsSecret != "" {
		env = append(env, credentialsEnv(cr.Spec.ClusterConfig.CredentialsSecret)...)
	}
	main.Env = append(main.Env, env...)

	if authProvider != nil && authProvider.OIDC != nil &&
		cr.Spec.ClusterConfig != nil && cr.Spec.ClusterConfig.Authentication != nil &&
		cr.Spec.ClusterConfig.Authentication.Oidc != nil {
		main.EnvFrom = append(main.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Spec.ClusterConfig.Authentication.Oidc.ClientCredentialsSecret,
				},
			},
		})
	}

	// Superset serves its health endpoint over HTTP; replace the framework's TCP readiness
	// probe with the product check for both liveness and readiness.
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
	main.LivenessProbe = probe
	main.ReadinessProbe = probe

	// Superset emits StatsD counters over UDP; the exporter sidecar serves them as
	// Prometheus metrics on the Service's named "metrics" port.
	podSpec.Containers = append(podSpec.Containers, corev1.Container{
		Name:            metricsContainerName,
		Image:           image,
		ImagePullPolicy: h.ImagePullPolicy,
		Command:         []string{"sh", "-x", "-c"},
		Args:            []string{metricsContainerCommands()},
	})

	return nil
}

// customizeService forces the NodePort type the product has always exposed (the Gen 2
// operator hardcoded the external-unstable listener class) and advertises the metrics
// endpoint to Prometheus scrapers unconditionally, matching the previous annotations.
func customizeService(svc *corev1.Service) {
	if svc == nil {
		return
	}
	svc.Spec.Type = corev1.ServiceTypeNodePort
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

// resolveImage reproduces the Gen 2 image resolution: {repo}/superset:{productVersion}
// always carries the -kubedoop{kubedoopVersion} suffix, defaulting to the operator's own
// build version — quay.io/zncdatadev publishes no bare product tags.
func resolveImage(cr *supersetv1alpha1.SupersetCluster) string {
	img := cr.Spec.Image
	if img == nil {
		img = &supersetv1alpha1.ImageSpec{}
	}
	if img.Custom != "" {
		return img.Custom
	}
	repo := img.Repo
	if repo == "" {
		repo = supersetv1alpha1.DefaultRepository
	}
	productVersion := img.ProductVersion
	if productVersion == "" {
		productVersion = supersetv1alpha1.DefaultProductVersion
	}
	kubedoopVersion := img.KubedoopVersion
	if kubedoopVersion == "" {
		kubedoopVersion = version.BuildVersion
	}
	return fmt.Sprintf("%s/%s:%s-kubedoop%s", repo, supersetv1alpha1.DefaultProductName, productVersion, kubedoopVersion)
}

func resolvePullPolicy(cr *supersetv1alpha1.SupersetCluster) corev1.PullPolicy {
	if cr.Spec.Image != nil && cr.Spec.Image.PullPolicy != nil {
		return *cr.Spec.Image.PullPolicy
	}
	return corev1.PullIfNotPresent
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
