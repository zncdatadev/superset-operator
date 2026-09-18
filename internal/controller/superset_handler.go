package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	opgoconfig "github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/security"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	"github.com/zncdatadev/operator-go/pkg/vector"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
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
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=authentication.kubedoop.dev,resources=authenticationclasses,verbs=get;list;watch

const (
	portHTTPName    = "http"
	portMetricsName = "metrics"
	portStatsDName  = "statsd"
	httpPort        = int32(8088)
	metricsPort     = int32(9102)
	statsDPort      = int32(9125)
	healthPath      = "/health"

	metricsContainerName       = "metrics"
	ldapCredentialsVolumeName  = "ldap-bind-credentials"
	ldapCredentialsStorageSize = "1Mi"

	// Gen 2 sized the shared emptyDir from a 10Mi logical log budget, which the old
	// CalculateLogVolumeSizeLimit helper expanded to 30Mi. Keep that effective pod
	// resource limit while retaining its 1Mi/one-backup Python rotation contract.
	logVolumeSize     = "30Mi"
	logFileSize       = "1MB"
	logFileMaxHistory = 1
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
// the framework), the role-level PDB and the ServiceAccount. Role shape is declared by
// DeclareRoles once per reconcile, while per-group providers are registered on the
// build context before the framework builds.
type SupersetRoleGroupHandler struct {
	*reconciler.BaseRoleGroupHandler[*supersetv1alpha1.SupersetCluster]
}

// Ensure framework interface implementations.
var _ reconciler.RoleGroupHandler[*supersetv1alpha1.SupersetCluster] = &SupersetRoleGroupHandler{}
var _ reconciler.RoleProvider[*supersetv1alpha1.SupersetCluster] = &SupersetRoleGroupHandler{}
var _ reconciler.RoleGroupResolver[*supersetv1alpha1.SupersetCluster] = reconciler.RoleGroupResolverFunc[*supersetv1alpha1.SupersetCluster](ResolveSupersetConfig)

// NewSupersetRoleGroupHandler creates the handler and configures the reconcile-invariant
// framework defaults.
func NewSupersetRoleGroupHandler(scheme *runtime.Scheme) *SupersetRoleGroupHandler {
	base := reconciler.NewBaseRoleGroupHandler[*supersetv1alpha1.SupersetCluster](scheme)
	configGenerator := opgoconfig.NewMultiFormatConfigGenerator()
	configGenerator.RegisterFormat(supersetConfigFilename, supersetPythonConfigMarshaler{})
	base.ConfigGenerator = configGenerator

	return &SupersetRoleGroupHandler{BaseRoleGroupHandler: base}
}

// DeclareRoles implements reconciler.RoleProvider. It is evaluated once per cluster reconcile,
// so CR-dependent declarations cannot leak through the process-wide handler instance.
func (h *SupersetRoleGroupHandler) DeclareRoles(
	_ context.Context,
	_ client.Client,
	cr *supersetv1alpha1.SupersetCluster,
) (reconciler.RoleCatalog, error) {
	listenerClass := listener.ListenerClassExternalUnstable
	var configDefaults *commonsv1alpha1.RoleGroupConfigSpec
	env := []corev1.EnvVar{{
		Name:  "SUPERSET_PORT",
		Value: strconv.FormatInt(int64(httpPort), 10),
	}}

	if cr.Spec.ClusterConfig != nil {
		if cr.Spec.ClusterConfig.ListenerClass != "" {
			listenerClass = listener.ListenerClass(cr.Spec.ClusterConfig.ListenerClass)
		}
		if cr.Spec.ClusterConfig.CredentialsSecret != "" {
			env = append(env, credentialsEnv(cr.Spec.ClusterConfig.CredentialsSecret)...)
		}
		if auth := cr.Spec.ClusterConfig.Authentication; auth != nil && auth.Oidc != nil {
			env = append(env, oidcClientCredentialsEnv(auth.Oidc.ClientCredentialsSecret)...)
		}
		// Gen 2 treated the presence of an aggregator discovery ConfigMap as enabling
		// Vector. Model that as a product default beneath the role/group logging config,
		// so an existing CR keeps working and an explicit enableVectorAgent: false wins.
		if cr.Spec.ClusterConfig.VectorAggregatorConfigMapName != "" {
			configDefaults = &commonsv1alpha1.RoleGroupConfigSpec{
				Logging: &commonsv1alpha1.LoggingSpec{EnableVectorAgent: ptr.To(true)},
			}
		}
	}

	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: healthPath,
				Port: intstr.FromInt32(httpPort),
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}
	return reconciler.RoleCatalog{
		supersetv1alpha1.RoleNameNode: {
			MainContainerName: supersetv1alpha1.RoleNameNode,
			ContainerPorts: []corev1.ContainerPort{
				{Name: portHTTPName, ContainerPort: httpPort, Protocol: corev1.ProtocolTCP},
			},
			ServicePorts: []corev1.ServicePort{
				{Name: portHTTPName, Port: httpPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(portHTTPName)},
				{Name: portMetricsName, Port: metricsPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString(portMetricsName)},
			},
			Command:        []string{"sh", "-x", "-c", mainContainerCommands()},
			ReadinessProbe: probe.DeepCopy(),
			LivenessProbe:  probe.DeepCopy(),
			ListenerClass:  listenerClass,
			LogProducers: []productlogging.ContainerLogging{
				{
					Container:   supersetv1alpha1.RoleNameNode,
					LogFileName: supersetLogFileName,
					// An empty Framework declares a product-rendered logging file. The
					// producer remains the real pod container (so Vector mounts it), while
					// the historical event tag and log directory stay "superset".
					LogDirName: supersetLogContainer,
				},
			},
			LogVolumeSize:  logVolumeSize,
			ConfigDefaults: configDefaults,
			Env:            env,
		},
	}, nil
}

// BuildResources declares the product-specific inputs on the per-call build context,
// delegates the framework-owned skeleton, then appends the Prometheus scrape annotations.
// Superset's Python module is already present in MergedConfig through ResolveSupersetConfig
// and is rendered by the base handler's file-specific ConfigGenerator.
func (h *SupersetRoleGroupHandler) BuildResources(
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.RoleGroupResources, error) {
	applyLegacyCLIOverrides(buildCtx)
	applyLegacyPodDefaults(buildCtx)
	registerLegacyLogVolume(buildCtx)

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
		registration := security.CredentialsVolume(
			ldapCredentialsVolumeName,
			ldap.BindCredentials.SecretClass,
		).WithStorageSize(ldapCredentialsStorageSize)
		if scope := security.ScopeString(ldap.BindCredentials.Scope); scope != "" {
			registration = registration.WithScope(scope)
		}
		provisioner.Register(registration)
		buildCtx.VolumeProviders = append(buildCtx.VolumeProviders, &fixedMountVolumeProvider{
			VolumeProvider: provisioner,
			mountPath:      path.Join(constant.KubedoopSecretDir, ldap.BindCredentials.SecretClass),
		})
	}

	metricsContainer, err := metricsNativeSidecar(buildCtx)
	if err != nil {
		return nil, reconciler.NewValidationError(
			"podOverrides", buildCtx.RoleName, buildCtx.RoleGroupName, err)
	}
	if buildCtx.SidecarManager == nil {
		return nil, reconciler.NewValidationError(
			"sidecar", buildCtx.RoleName, buildCtx.RoleGroupName,
			fmt.Errorf("the GenericReconciler did not provide a SidecarManager"))
	}
	buildCtx.SidecarManager.Register(
		sidecar.NewStaticContainerProvider(metricsContainer),
		&sidecar.SidecarConfig{Enabled: true},
	)

	resources, err := h.BaseRoleGroupHandler.BuildResources(ctx, k8sClient, cr, buildCtx)
	if err != nil {
		return nil, err
	}
	if err := renderLegacyFileLogging(resources.ConfigMap, buildCtx); err != nil {
		return nil, fmt.Errorf("render legacy file logging: %w", err)
	}

	// Superset historically used OrderedReady. The framework defaults to Parallel for
	// quorum products, but Superset has no quorum-startup requirement and changing this
	// immutable field would be an unnecessary workload semantic drift during migration.
	resources.StatefulSet.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
	addPrometheusAnnotations(resources.Service)

	return resources, nil
}

// applyLegacyCLIOverrides preserves the CRD's shipped Gen 2 behavior. Although the field is
// named cliOverrides, operator-go v0.12 replaced the container Command with it. v0.13 normally
// writes the merged value to Args, which would make it a set of unused positional parameters for
// Superset's `sh -c` wrapper. Moving the merged value into the declaration before the base build
// retains command replacement while still allowing podOverrides to win during strategic merge.
func applyLegacyCLIOverrides(buildCtx *reconciler.RoleGroupBuildContext) {
	if buildCtx == nil || buildCtx.MergedConfig == nil || len(buildCtx.MergedConfig.CliArgs) == 0 {
		return
	}
	buildCtx.Declaration.Command = append([]string(nil), buildCtx.MergedConfig.CliArgs...)
	buildCtx.MergedConfig.CliArgs = nil
}

// applyLegacyPodDefaults keeps kubelet Service environment injection enabled when the CR has no
// opinion. Gen 2 relied on the Kubernetes default (true); operator-go v0.13 deliberately defaults
// it to false. Writing this into the product's pod-override layer before the base build preserves
// the old behavior while still allowing an explicit podOverride value of false to win.
func applyLegacyPodDefaults(buildCtx *reconciler.RoleGroupBuildContext) {
	if buildCtx == nil || buildCtx.MergedConfig == nil {
		return
	}
	if buildCtx.MergedConfig.PodOverrides == nil {
		buildCtx.MergedConfig.PodOverrides = &corev1.PodTemplateSpec{}
	}
	if buildCtx.MergedConfig.PodOverrides.Spec.EnableServiceLinks == nil {
		buildCtx.MergedConfig.PodOverrides.Spec.EnableServiceLinks = ptr.To(true)
	}
}

// registerLegacyLogVolume restores Gen 2's always-present application log volume when the
// framework's Vector pipeline is inactive. operator-go v0.13 owns the same volume and producer
// mount while Vector is active, so the fallback must be mutually exclusive to avoid duplicate
// Kubernetes volume names.
func registerLegacyLogVolume(buildCtx *reconciler.RoleGroupBuildContext) {
	if buildCtx == nil || len(buildCtx.Declaration.LogProducers) == 0 ||
		buildCtx.LogFileTarget(buildCtx.Declaration.LogProducers[0]) != "" {
		return
	}
	size := resource.MustParse(logVolumeSize)
	buildCtx.VolumeProviders = append(buildCtx.VolumeProviders, &staticVolumeProvider{
		volumes: []corev1.Volume{{
			Name: vector.VectorLogVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: &size,
			}},
		}},
		mounts: []corev1.VolumeMount{{
			Name:      vector.VectorLogVolumeName,
			MountPath: constant.KubedoopLogDir,
		}},
	})
}

// renderLegacyFileLogging keeps Superset's rolling JSON file appender active with or without
// Vector. The producer is declared product-rendered because the framework intentionally couples
// its generated file appender to Vector; this compatibility layer adds the ConfigMap entry after
// the base build and keeps the previous rotation bounds in both modes.
func renderLegacyFileLogging(
	configMap *corev1.ConfigMap,
	buildCtx *reconciler.RoleGroupBuildContext,
) error {
	if configMap == nil || buildCtx == nil || len(buildCtx.Declaration.LogProducers) == 0 {
		return nil
	}
	producer := buildCtx.Declaration.LogProducers[0]
	generator, err := productlogging.GeneratorFor(productlogging.LoggingFrameworkPython)
	if err != nil {
		return err
	}
	fileName := producer.FileName
	if fileName == "" {
		fileName = generator.DefaultFileName()
	}
	logFileName := producer.LogFileName
	if logFileName == "" {
		logFileName = productlogging.ContainerLogFileName(productlogging.LoggingFrameworkPython, producer.Container)
	}
	content, err := generator.Render(
		productlogging.LogConfigFromSpec(buildCtx.ContainerLogging(producer.Container)),
		productlogging.RenderOptions{
			Pattern:        producer.Pattern,
			FileOutputPath: path.Join(productlogging.LogDirFor(producer), logFileName),
			MaxFileSize:    logFileSize,
			MaxHistory:     logFileMaxHistory,
		},
	)
	if err != nil {
		return err
	}
	if _, exists := configMap.Data[fileName]; exists {
		return reconciler.NewValidationError(
			"logging", buildCtx.RoleName, buildCtx.RoleGroupName,
			fmt.Errorf("product logging config file %q collides with an existing ConfigMap key", fileName),
		)
	}
	configMap.Data[fileName] = content
	return nil
}

// metricsNativeSidecar declares the statsd-exporter as a Kubernetes native sidecar. A user
// podOverride that addresses the historical "metrics" container is folded over the product
// defaults, then removed from the regular/init container lists so one name is emitted exactly once.
func metricsNativeSidecar(buildCtx *reconciler.RoleGroupBuildContext) (corev1.Container, error) {
	container := corev1.Container{
		Name:            metricsContainerName,
		Image:           buildCtx.ResolvedImage.Reference,
		ImagePullPolicy: buildCtx.ResolvedImage.PullPolicy,
		Command:         []string{"sh", "-x", "-c"},
		Args:            []string{metricsContainerCommands()},
		Ports: []corev1.ContainerPort{
			{Name: portMetricsName, ContainerPort: metricsPort, Protocol: corev1.ProtocolTCP},
			{Name: portStatsDName, ContainerPort: statsDPort, Protocol: corev1.ProtocolUDP},
		},
		RestartPolicy:   sidecar.SidecarRestartPolicy(),
		SecurityContext: sidecar.DefaultSecurityContext(),
	}

	if buildCtx.MergedConfig == nil || buildCtx.MergedConfig.PodOverrides == nil {
		return container, nil
	}

	podSpec := &buildCtx.MergedConfig.PodOverrides.Spec
	regular, regularCount := extractNamedContainer(&podSpec.Containers, metricsContainerName)
	init, initCount := extractNamedContainer(&podSpec.InitContainers, metricsContainerName)
	if regularCount+initCount > 1 {
		return corev1.Container{}, fmt.Errorf(
			"container %q is declared %d times across podOverrides containers and initContainers",
			metricsContainerName, regularCount+initCount)
	}
	if regularCount == 1 {
		return mergeContainer(container, regular)
	}
	if initCount == 1 {
		return mergeContainer(container, init)
	}
	return container, nil
}

func extractNamedContainer(containers *[]corev1.Container, name string) (corev1.Container, int) {
	kept := (*containers)[:0]
	var found corev1.Container
	count := 0
	for _, container := range *containers {
		if container.Name == name {
			found = container
			count++
			continue
		}
		kept = append(kept, container)
	}
	*containers = kept
	return found, count
}

func mergeContainer(base, override corev1.Container) (corev1.Container, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("encode metrics sidecar defaults: %w", err)
	}
	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("encode metrics sidecar override: %w", err)
	}
	mergedJSON, err := strategicpatch.StrategicMergePatch(baseJSON, overrideJSON, corev1.Container{})
	if err != nil {
		return corev1.Container{}, fmt.Errorf("merge metrics sidecar override: %w", err)
	}
	var merged corev1.Container
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return corev1.Container{}, fmt.Errorf("decode merged metrics sidecar: %w", err)
	}
	merged.Name = metricsContainerName
	merged.RestartPolicy = sidecar.SidecarRestartPolicy()
	return merged, nil
}

// addPrometheusAnnotations advertises the metrics endpoint to Prometheus scrapers
// unconditionally, matching the Gen 2 operator's always-on Service annotations. The
// Service type itself is framework-owned via RoleDeclaration.ListenerClass.
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

// ResolveSupersetConfig contributes the operator-owned Python module at the lowest config merge
// layer. Role and role-group configOverrides then replace individual keys (including the special
// header/footer blocks) before the handler's emit-only marshaler writes superset_config.py.
func ResolveSupersetConfig(
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) (*reconciler.Contribution, error) {
	authProvider, err := fetchAuthProvider(ctx, k8sClient, cr)
	if err != nil {
		return nil, err
	}

	var auth *supersetv1alpha1.AuthenticationSpec
	if cr.Spec.ClusterConfig != nil {
		auth = cr.Spec.ClusterConfig.Authentication
	}
	return &reconciler.Contribution{ConfigOverrides: map[string]map[string]string{
		supersetConfigFilename: {
			supersetConfigGeneratedContentKey: renderSupersetConfig(authProvider, auth),
		},
	}}, nil
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

func oidcClientCredentialsEnv(credentialsSecret string) []corev1.EnvVar {
	if credentialsSecret == "" {
		return nil
	}
	return []corev1.EnvVar{
		{
			Name: "CLIENT_ID",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
				Key:                  "CLIENT_ID",
			}},
		},
		{
			Name: "CLIENT_SECRET",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
				Key:                  "CLIENT_SECRET",
			}},
		},
	}
}

// fixedMountVolumeProvider keeps the secret-operator volume name DNS-safe while preserving the
// historical mount path, which is embedded in the generated Superset Python configuration.
type fixedMountVolumeProvider struct {
	reconciler.VolumeProvider
	mountPath string
}

type staticVolumeProvider struct {
	volumes []corev1.Volume
	mounts  []corev1.VolumeMount
}

func (p *staticVolumeProvider) Volumes() []corev1.Volume {
	return append([]corev1.Volume(nil), p.volumes...)
}

func (p *staticVolumeProvider) VolumeMounts() []corev1.VolumeMount {
	return append([]corev1.VolumeMount(nil), p.mounts...)
}

func (p *fixedMountVolumeProvider) VolumeMounts() []corev1.VolumeMount {
	mounts := p.VolumeProvider.VolumeMounts()
	for i := range mounts {
		if mounts[i].Name == ldapCredentialsVolumeName {
			mounts[i].MountPath = p.mountPath
		}
	}
	return mounts
}
