package controller

import (
	"context"
	"slices"
	"strings"
	"testing"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	opgoconfig "github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/listener"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/security"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

const (
	testClusterName      = "analytics"
	testClusterNamespace = "superset-test"
	testRoleGroupName    = "default"
	testProductImage     = "example.invalid/superset:test"
)

func TestSupersetRoleGroupHandler_DeclareRoles(t *testing.T) {
	cr := newControllerTestCluster()
	cr.Spec.ClusterConfig.ListenerClass = string(listener.ListenerClassExternalStable)
	cr.Spec.ClusterConfig.Authentication = &supersetv1alpha1.AuthenticationSpec{
		AuthenticationClass: "oidc-auth",
		Oidc: &supersetv1alpha1.OidcSpec{
			ClientCredentialsSecret: "oidc-client",
		},
	}

	handler := NewSupersetRoleGroupHandler(runtime.NewScheme())
	catalog, err := handler.DeclareRoles(context.Background(), nil, cr)
	if err != nil {
		t.Fatalf("DeclareRoles() error = %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("DeclareRoles() returned %d roles, want 1", len(catalog))
	}
	declaration, ok := catalog[supersetv1alpha1.RoleNameNode]
	if !ok {
		t.Fatalf("DeclareRoles() omitted role %q", supersetv1alpha1.RoleNameNode)
	}

	if declaration.MainContainerName != supersetv1alpha1.RoleNameNode {
		t.Errorf("main container = %q, want %q", declaration.MainContainerName, supersetv1alpha1.RoleNameNode)
	}
	if len(declaration.ContainerPorts) != 1 {
		t.Fatalf("main container ports = %#v, want only the HTTP port", declaration.ContainerPorts)
	}
	assertContainerPort(t, declaration.ContainerPorts, portHTTPName, httpPort, corev1.ProtocolTCP)

	if len(declaration.ServicePorts) != 2 {
		t.Fatalf("service ports = %#v, want HTTP and metrics ports", declaration.ServicePorts)
	}
	assertServicePort(t, declaration.ServicePorts, portHTTPName, httpPort, corev1.ProtocolTCP)
	assertServicePort(t, declaration.ServicePorts, portMetricsName, metricsPort, corev1.ProtocolTCP)

	assertSupersetProbe(t, "readiness", declaration.ReadinessProbe)
	assertSupersetProbe(t, "liveness", declaration.LivenessProbe)
	assertSupersetStartupProbe(t, declaration.StartupProbe)
	if declaration.ListenerClass != listener.ListenerClassExternalStable {
		t.Errorf("listener class = %q, want %q", declaration.ListenerClass, listener.ListenerClassExternalStable)
	}

	wantSecretEnv := map[string]string{
		"ADMIN_USERNAME":          "adminUser.username",
		"ADMIN_FIRSTNAME":         "adminUser.firstname",
		"ADMIN_LASTNAME":          "adminUser.lastname",
		"ADMIN_EMAIL":             "adminUser.email",
		"ADMIN_PASSWORD":          "adminUser.password",
		"SECRET_KEY":              "appSecretKey",
		"SQLALCHEMY_DATABASE_URI": "connections.sqlalchemyDatabaseUri",
	}
	if got, want := len(declaration.Env), 1+len(wantSecretEnv)+2; got != want {
		t.Fatalf("declared env count = %d, want %d: %#v", got, want, declaration.Env)
	}
	portEnv := requireEnvVar(t, declaration.Env, "SUPERSET_PORT")
	if portEnv.Value != "8088" || portEnv.ValueFrom != nil {
		t.Errorf("SUPERSET_PORT = %#v, want literal 8088", portEnv)
	}
	for name, key := range wantSecretEnv {
		assertSecretEnvVar(t, declaration.Env, name, "superset-credentials", key)
	}
	assertSecretEnvVar(t, declaration.Env, "CLIENT_ID", "oidc-client", "CLIENT_ID")
	assertSecretEnvVar(t, declaration.Env, "CLIENT_SECRET", "oidc-client", "CLIENT_SECRET")

	if len(declaration.LogProducers) != 1 {
		t.Fatalf("log producers = %#v, want one Python producer", declaration.LogProducers)
	}
	producer := declaration.LogProducers[0]
	if producer.Container != supersetv1alpha1.RoleNameNode ||
		producer.Framework != productlogging.LoggingFrameworkPython ||
		producer.LogFileName != supersetLogFileName ||
		producer.LogDirName != supersetLogContainer {
		t.Errorf("log producer = %#v, want node Python producer for %s/%s",
			producer, supersetLogContainer, supersetLogFileName)
	}
	if declaration.LogVolumeSize != maxLogFileSize {
		t.Errorf("log volume size = %q, want %q", declaration.LogVolumeSize, maxLogFileSize)
	}

	t.Run("defaults listener class", func(t *testing.T) {
		defaultCR := newControllerTestCluster()
		defaultCR.Spec.ClusterConfig.ListenerClass = ""
		defaultCatalog, err := handler.DeclareRoles(context.Background(), nil, defaultCR)
		if err != nil {
			t.Fatalf("DeclareRoles() error = %v", err)
		}
		if got := defaultCatalog[supersetv1alpha1.RoleNameNode].ListenerClass; got != listener.ListenerClassExternalUnstable {
			t.Errorf("default listener class = %q, want %q", got, listener.ListenerClassExternalUnstable)
		}
	})
}

func TestSupersetConfigResolverAndGenerator(t *testing.T) {
	ctx := context.Background()
	scheme := controllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	cr := newControllerTestCluster()
	cr.Spec.Node.OverridesSpec = &commonsv1alpha1.OverridesSpec{
		ConfigOverrides: map[string]map[string]string{
			supersetConfigFilename: {
				configOverrideFileHeaderKey: "COMMON_HEADER_VAR = \"role-value\"\n" +
					"ROLE_HEADER_VAR = \"role-value\"\n",
				configOverrideFileFooterKey: "ROLE_FOOTER_VAR = \"role-value\"\n",
				"ROW_LIMIT":                 "25000",
			},
		},
	}
	group := cr.Spec.Node.RoleGroups[testRoleGroupName]
	group.OverridesSpec = &commonsv1alpha1.OverridesSpec{
		ConfigOverrides: map[string]map[string]string{
			supersetConfigFilename: {
				configOverrideFileHeaderKey: "COMMON_HEADER_VAR = \"group-value\"\n",
			},
		},
	}
	cr.Spec.Node.RoleGroups[testRoleGroupName] = group

	handler := NewSupersetRoleGroupHandler(scheme)
	catalog, err := handler.DeclareRoles(ctx, k8sClient, cr)
	if err != nil {
		t.Fatalf("DeclareRoles() error = %v", err)
	}
	clusterSpec := cr.GetSpec()
	roleSpec := clusterSpec.Roles[supersetv1alpha1.RoleNameNode]
	groupSpec := roleSpec.RoleGroups[testRoleGroupName]
	buildCtx := &reconciler.RoleGroupBuildContext{
		ClusterName:      cr.Name,
		ClusterNamespace: cr.Namespace,
		ClusterSpec:      clusterSpec,
		RoleName:         supersetv1alpha1.RoleNameNode,
		RoleSpec:         &roleSpec,
		RoleGroupName:    testRoleGroupName,
		RoleGroupSpec:    groupSpec,
		Declaration:      catalog[supersetv1alpha1.RoleNameNode],
	}
	buildCtx.MergedConfig = resolveAndMergeSupersetConfig(t, ctx, k8sClient, cr, buildCtx)

	files, err := handler.ConfigGenerator.GenerateFiles(buildCtx.MergedConfig.ConfigFiles)
	if err != nil {
		t.Fatalf("GenerateFiles() error = %v", err)
	}
	rendered, ok := files[supersetConfigFilename]
	if !ok {
		t.Fatalf("GenerateFiles() omitted %q: %#v", supersetConfigFilename, files)
	}

	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	if got, want := lines[0], `COMMON_HEADER_VAR = "group-value"`; got != want {
		t.Errorf("first line = %q, want group-level header %q", got, want)
	}
	if got, want := lines[len(lines)-1], `ROLE_FOOTER_VAR = "role-value"`; got != want {
		t.Errorf("last line = %q, want inherited role-level footer %q", got, want)
	}
	if strings.Contains(rendered, `COMMON_HEADER_VAR = "role-value"`) ||
		strings.Contains(rendered, `ROLE_HEADER_VAR = "role-value"`) {
		t.Errorf("group-level header did not replace the role-level header block:\n%s", rendered)
	}
	for _, line := range []string{
		"import logging.config",
		"ROW_LIMIT = 10000",
		"ROW_LIMIT = 25000",
	} {
		if got := strings.Count(rendered, line); got != 1 {
			t.Errorf("rendered config contains %q %d times, want exactly once:\n%s", line, got, rendered)
		}
	}
	if defaultIndex, overrideIndex := strings.Index(rendered, "ROW_LIMIT = 10000"), strings.Index(rendered, "ROW_LIMIT = 25000"); defaultIndex < 0 || overrideIndex <= defaultIndex {
		t.Errorf("ordinary config override must follow operator defaults (indexes %d, %d):\n%s",
			defaultIndex, overrideIndex, rendered)
	}
	if strings.Contains(rendered, configOverrideFileHeaderKey+" =") ||
		strings.Contains(rendered, configOverrideFileFooterKey+" =") ||
		strings.Contains(rendered, supersetConfigGeneratedContentKey) {
		t.Errorf("marshaler leaked a pseudo-key into Python output:\n%s", rendered)
	}
}

func TestMetricsNativeSidecar(t *testing.T) {
	t.Run("declares native sidecar ports", func(t *testing.T) {
		container, err := metricsNativeSidecar(newMetricsBuildContext(nil))
		if err != nil {
			t.Fatalf("metricsNativeSidecar() error = %v", err)
		}
		assertMetricsContainer(t, container, testProductImage)
	})

	t.Run("merges and removes legacy regular container override", func(t *testing.T) {
		podOverride := &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: supersetv1alpha1.RoleNameNode, Image: testProductImage},
					{
						Name:  metricsContainerName,
						Image: "example.invalid/custom-statsd-exporter:v2",
						Env: []corev1.EnvVar{{
							Name:  "CUSTOM_MAPPING",
							Value: "enabled",
						}},
					},
				},
				InitContainers: []corev1.Container{{Name: "setup", Image: "example.invalid/setup:test"}},
			},
		}
		buildCtx := newMetricsBuildContext(podOverride)

		container, err := metricsNativeSidecar(buildCtx)
		if err != nil {
			t.Fatalf("metricsNativeSidecar() error = %v", err)
		}
		assertMetricsContainer(t, container, "example.invalid/custom-statsd-exporter:v2")
		if got := requireEnvVar(t, container.Env, "CUSTOM_MAPPING").Value; got != "enabled" {
			t.Errorf("merged CUSTOM_MAPPING = %q, want enabled", got)
		}
		if len(buildCtx.MergedConfig.PodOverrides.Spec.Containers) != 1 ||
			buildCtx.MergedConfig.PodOverrides.Spec.Containers[0].Name != supersetv1alpha1.RoleNameNode {
			t.Errorf("regular containers after extraction = %#v, want only node",
				buildCtx.MergedConfig.PodOverrides.Spec.Containers)
		}
		if len(buildCtx.MergedConfig.PodOverrides.Spec.InitContainers) != 1 ||
			buildCtx.MergedConfig.PodOverrides.Spec.InitContainers[0].Name != "setup" {
			t.Errorf("init containers after extraction = %#v, want only setup",
				buildCtx.MergedConfig.PodOverrides.Spec.InitContainers)
		}

		manager := sidecar.NewSidecarManager()
		manager.Register(sidecar.NewStaticContainerProvider(container), &sidecar.SidecarConfig{Enabled: true})
		podSpec := buildCtx.MergedConfig.PodOverrides.Spec.DeepCopy()
		if err := manager.InjectAll(podSpec); err != nil {
			t.Fatalf("InjectAll() error = %v", err)
		}
		if got := countNamedContainers(podSpec, metricsContainerName); got != 1 {
			t.Fatalf("metrics container count after native-sidecar injection = %d, want 1: %#v", got, podSpec)
		}
		if findContainer(podSpec.Containers, metricsContainerName) != nil {
			t.Errorf("metrics container remained a regular container: %#v", podSpec.Containers)
		}
		injected := findContainer(podSpec.InitContainers, metricsContainerName)
		if injected == nil {
			t.Fatalf("metrics native sidecar missing from initContainers: %#v", podSpec.InitContainers)
		}
		assertMetricsContainer(t, *injected, "example.invalid/custom-statsd-exporter:v2")
	})

	t.Run("rejects duplicate legacy declarations", func(t *testing.T) {
		podOverride := &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers:     []corev1.Container{{Name: metricsContainerName}},
				InitContainers: []corev1.Container{{Name: metricsContainerName}},
			},
		}
		_, err := metricsNativeSidecar(newMetricsBuildContext(podOverride))
		if err == nil || !strings.Contains(err.Error(), "declared 2 times") {
			t.Fatalf("metricsNativeSidecar() error = %v, want duplicate declaration error", err)
		}
	})
}

func TestSupersetRoleGroupHandler_BuildResourcesLDAPCredentials(t *testing.T) {
	scheme := controllerTestScheme(t)
	secretClass := "corp.ldap"
	authClass := &authv1alpha1.AuthenticationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ldap-auth",
			Namespace: testClusterNamespace,
		},
		Spec: authv1alpha1.AuthenticationClassSpec{
			AuthenticationProvider: &authv1alpha1.AuthenticationProvider{
				LDAP: &authv1alpha1.LDAPProvider{
					Hostname: "ldap.example.test",
					BindCredentials: &commonsv1alpha1.Credentials{
						SecretClass: secretClass,
						Scope: &commonsv1alpha1.CredentialsScope{
							Node:     true,
							Services: []string{"superset"},
						},
					},
				},
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(authClass).Build()
	cr := newControllerTestCluster()
	cr.Spec.ClusterConfig.Authentication = &supersetv1alpha1.AuthenticationSpec{
		AuthenticationClass: authClass.Name,
	}
	handler := NewSupersetRoleGroupHandler(scheme)
	catalog, err := handler.DeclareRoles(context.Background(), k8sClient, cr)
	if err != nil {
		t.Fatalf("DeclareRoles() error = %v", err)
	}
	declaration, ok := catalog[supersetv1alpha1.RoleNameNode]
	if !ok {
		t.Fatalf("DeclareRoles() omitted role %q", supersetv1alpha1.RoleNameNode)
	}

	clusterSpec := cr.GetSpec()
	roleSpec, ok := clusterSpec.Roles[supersetv1alpha1.RoleNameNode]
	if !ok {
		t.Fatalf("generic spec omitted role %q", supersetv1alpha1.RoleNameNode)
	}
	groupSpec, ok := roleSpec.RoleGroups[testRoleGroupName]
	if !ok {
		t.Fatalf("generic role omitted group %q", testRoleGroupName)
	}
	buildCtx := &reconciler.RoleGroupBuildContext{
		ClusterName:        cr.Name,
		ClusterNamespace:   cr.Namespace,
		ClusterLabels:      cr.Labels,
		ClusterSpec:        clusterSpec,
		RoleName:           supersetv1alpha1.RoleNameNode,
		RoleSpec:           &roleSpec,
		RoleGroupName:      testRoleGroupName,
		RoleGroupSpec:      groupSpec,
		MergedConfig:       opgoconfig.NewMergedConfig(),
		ResourceName:       reconciler.RoleGroupResourceName(cr.Name, supersetv1alpha1.RoleNameNode, testRoleGroupName),
		ServiceAccountName: "supersetcluster-" + cr.Name,
		SidecarManager:     sidecar.NewSidecarManager(),
		Declaration:        declaration,
		ResolvedImage: reconciler.ResolvedImage{
			Reference:      testProductImage,
			PullPolicy:     corev1.PullIfNotPresent,
			ProductVersion: supersetv1alpha1.DefaultProductVersion,
		},
		ProductName: supersetv1alpha1.DefaultProductName,
	}
	buildCtx.MergedConfig = resolveAndMergeSupersetConfig(t, context.Background(), k8sClient, cr, buildCtx)
	legacyCommand := []string{"custom-superset-entrypoint", "--serve"}
	buildCtx.MergedConfig.CliArgs = legacyCommand

	resources, err := handler.BuildResources(context.Background(), k8sClient, cr, buildCtx)
	if err != nil {
		t.Fatalf("BuildResources() error = %v", err)
	}
	if resources.StatefulSet == nil || resources.ConfigMap == nil {
		t.Fatal("BuildResources() omitted StatefulSet or ConfigMap")
	}

	volumes := resources.StatefulSet.Spec.Template.Spec.Volumes
	volume := requireVolume(t, volumes, ldapCredentialsVolumeName)
	if findVolume(volumes, secretClass) != nil {
		t.Errorf("SecretClass %q was used as a Kubernetes volume name: %#v", secretClass, volumes)
	}
	if volume.Ephemeral == nil || volume.Ephemeral.VolumeClaimTemplate == nil {
		t.Fatalf("LDAP volume is not an ephemeral SecretClass volume: %#v", volume)
	}
	storage := volume.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
	if storage.Cmp(resource.MustParse(ldapCredentialsStorageSize)) != 0 {
		t.Errorf("LDAP credentials storage request = %s, want %s", storage.String(), ldapCredentialsStorageSize)
	}
	annotations := volume.Ephemeral.VolumeClaimTemplate.Annotations
	if got := annotations[security.SecretClassAnnotation]; got != secretClass {
		t.Errorf("LDAP SecretClass annotation = %q, want %q", got, secretClass)
	}
	if got, want := annotations[security.SecretClassScopeAnnotation], "node,service=superset"; got != want {
		t.Errorf("LDAP scope annotation = %q, want %q", got, want)
	}

	mainContainer := requireContainer(t, resources.StatefulSet.Spec.Template.Spec.Containers, supersetv1alpha1.RoleNameNode)
	if !slices.Equal(mainContainer.Command, legacyCommand) {
		t.Errorf("legacy cliOverrides command = %#v, want %#v", mainContainer.Command, legacyCommand)
	}
	if len(mainContainer.Args) != 0 {
		t.Errorf("legacy cliOverrides leaked into Args: %#v", mainContainer.Args)
	}
	mount := requireVolumeMount(t, mainContainer.VolumeMounts, ldapCredentialsVolumeName)
	wantMountPath := "/kubedoop/secret/" + secretClass
	if mount.MountPath != wantMountPath || !mount.ReadOnly {
		t.Errorf("LDAP volume mount = %#v, want read-only %q", mount, wantMountPath)
	}
	renderedConfig := resources.ConfigMap.Data[supersetConfigFilename]
	for _, credentialFile := range []string{wantMountPath + "/user", wantMountPath + "/password"} {
		if !strings.Contains(renderedConfig, credentialFile) {
			t.Errorf("rendered %s does not reference %q:\n%s", supersetConfigFilename, credentialFile, renderedConfig)
		}
	}

	podSpec := &resources.StatefulSet.Spec.Template.Spec
	if got := countNamedContainers(podSpec, metricsContainerName); got != 1 {
		t.Fatalf("built StatefulSet metrics container count = %d, want 1", got)
	}
	if findContainer(podSpec.Containers, metricsContainerName) != nil {
		t.Errorf("metrics was emitted as a regular container: %#v", podSpec.Containers)
	}
	metrics := requireContainer(t, podSpec.InitContainers, metricsContainerName)
	assertMetricsContainer(t, *metrics, testProductImage)
}

func resolveAndMergeSupersetConfig(
	t *testing.T,
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
	buildCtx *reconciler.RoleGroupBuildContext,
) *opgoconfig.MergedConfig {
	t.Helper()
	contribution, err := ResolveSupersetConfig(ctx, k8sClient, cr, buildCtx)
	if err != nil {
		t.Fatalf("ResolveSupersetConfig() error = %v", err)
	}
	derived := &commonsv1alpha1.OverridesSpec{}
	if contribution != nil {
		derived.ConfigOverrides = contribution.ConfigOverrides
		derived.EnvOverrides = contribution.EnvVars
	}
	merged := opgoconfig.NewConfigMerger().Merge(
		derived,
		buildCtx.RoleSpec.GetOverrides(),
		buildCtx.RoleGroupSpec.GetOverrides(),
	)
	merged.Logging = buildCtx.EffectiveConfig().Logging
	return merged
}

func newControllerTestCluster() *supersetv1alpha1.SupersetCluster {
	replicas := int32(1)
	return &supersetv1alpha1.SupersetCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: supersetv1alpha1.GroupVersion.String(),
			Kind:       "SupersetCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testClusterNamespace,
		},
		Spec: supersetv1alpha1.SupersetClusterSpec{
			Image: &supersetv1alpha1.ImageSpec{Custom: testProductImage},
			ClusterConfig: &supersetv1alpha1.ClusterConfigSpec{
				CredentialsSecret: "superset-credentials",
			},
			Node: &supersetv1alpha1.NodeSpec{
				RoleGroups: map[string]supersetv1alpha1.NodeRoleGroupSpec{
					testRoleGroupName: {Replicas: &replicas},
				},
			},
		},
	}
}

func controllerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps API to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}
	if err := authv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add authentication API to scheme: %v", err)
	}
	if err := supersetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Superset API to scheme: %v", err)
	}
	return scheme
}

func newMetricsBuildContext(podOverride *corev1.PodTemplateSpec) *reconciler.RoleGroupBuildContext {
	merged := opgoconfig.NewMergedConfig()
	merged.PodOverrides = podOverride
	return &reconciler.RoleGroupBuildContext{
		RoleName:      supersetv1alpha1.RoleNameNode,
		RoleGroupName: testRoleGroupName,
		MergedConfig:  merged,
		ResolvedImage: reconciler.ResolvedImage{
			Reference:  testProductImage,
			PullPolicy: corev1.PullIfNotPresent,
		},
	}
}

func assertMetricsContainer(t *testing.T, container corev1.Container, wantImage string) {
	t.Helper()
	if container.Name != metricsContainerName {
		t.Errorf("metrics container name = %q, want %q", container.Name, metricsContainerName)
	}
	if container.Image != wantImage {
		t.Errorf("metrics container image = %q, want %q", container.Image, wantImage)
	}
	if container.RestartPolicy == nil || *container.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Errorf("metrics restart policy = %v, want Always", container.RestartPolicy)
	}
	if len(container.Ports) != 2 {
		t.Fatalf("metrics ports = %#v, want Prometheus and StatsD ports", container.Ports)
	}
	assertContainerPort(t, container.Ports, portMetricsName, metricsPort, corev1.ProtocolTCP)
	assertContainerPort(t, container.Ports, portStatsDName, statsDPort, corev1.ProtocolUDP)
}

func assertContainerPort(
	t *testing.T,
	ports []corev1.ContainerPort,
	name string,
	number int32,
	protocol corev1.Protocol,
) {
	t.Helper()
	for _, port := range ports {
		if port.Name == name {
			if port.ContainerPort != number || port.Protocol != protocol {
				t.Errorf("container port %q = %#v, want %s/%d", name, port, protocol, number)
			}
			return
		}
	}
	t.Errorf("container ports %#v do not include %q", ports, name)
}

func assertServicePort(
	t *testing.T,
	ports []corev1.ServicePort,
	name string,
	number int32,
	protocol corev1.Protocol,
) {
	t.Helper()
	for _, port := range ports {
		if port.Name == name {
			if port.Port != number || port.Protocol != protocol ||
				port.TargetPort.Type != intstr.String || port.TargetPort.StrVal != name {
				t.Errorf("service port %q = %#v, want %s/%d targeting named port %q",
					name, port, protocol, number, name)
			}
			return
		}
	}
	t.Errorf("service ports %#v do not include %q", ports, name)
}

func assertSupersetProbe(t *testing.T, label string, probe *corev1.Probe) {
	t.Helper()
	if probe == nil || probe.HTTPGet == nil {
		t.Fatalf("%s probe is not an HTTP probe: %#v", label, probe)
	}
	if probe.HTTPGet.Path != healthPath ||
		probe.HTTPGet.Port.Type != intstr.Int ||
		probe.HTTPGet.Port.IntVal != httpPort {
		t.Errorf("%s probe target = %#v, want /health on %d", label, probe.HTTPGet, httpPort)
	}
	if probe.InitialDelaySeconds != 30 || probe.PeriodSeconds != 10 ||
		probe.TimeoutSeconds != 5 || probe.FailureThreshold != 3 || probe.SuccessThreshold != 1 {
		t.Errorf("%s probe timing = %#v, want 30s initial, 10s period, 5s timeout, 3/1 thresholds", label, probe)
	}
}

func assertSupersetStartupProbe(t *testing.T, probe *corev1.Probe) {
	t.Helper()
	if probe == nil || probe.HTTPGet == nil {
		t.Fatalf("startup probe is not an HTTP probe: %#v", probe)
	}
	if probe.HTTPGet.Path != healthPath ||
		probe.HTTPGet.Port.Type != intstr.String ||
		probe.HTTPGet.Port.StrVal != portHTTPName {
		t.Errorf("startup probe target = %#v, want /health on named port %q", probe.HTTPGet, portHTTPName)
	}
	if probe.InitialDelaySeconds != 4 || probe.PeriodSeconds != 6 ||
		probe.TimeoutSeconds != 3 || probe.FailureThreshold != 30 || probe.SuccessThreshold != 1 {
		t.Errorf("startup probe timing = %#v, want 4s initial, 6s period, 3s timeout, 30/1 thresholds", probe)
	}
}

func assertSecretEnvVar(t *testing.T, env []corev1.EnvVar, name, secretName, key string) {
	t.Helper()
	value := requireEnvVar(t, env, name)
	if value.ValueFrom == nil || value.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("env %q is not sourced from a Secret key: %#v", name, value)
	}
	ref := value.ValueFrom.SecretKeyRef
	if ref.Name != secretName || ref.Key != key {
		t.Errorf("env %q SecretKeyRef = %#v, want %s/%s", name, ref, secretName, key)
	}
}

func requireEnvVar(t *testing.T, env []corev1.EnvVar, name string) *corev1.EnvVar {
	t.Helper()
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	t.Fatalf("env %#v does not include %q", env, name)
	return nil
}

func countNamedContainers(podSpec *corev1.PodSpec, name string) int {
	count := 0
	for _, container := range podSpec.Containers {
		if container.Name == name {
			count++
		}
	}
	for _, container := range podSpec.InitContainers {
		if container.Name == name {
			count++
		}
	}
	return count
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

func requireContainer(t *testing.T, containers []corev1.Container, name string) *corev1.Container {
	t.Helper()
	container := findContainer(containers, name)
	if container == nil {
		t.Fatalf("containers %#v do not include %q", containers, name)
	}
	return container
}

func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

func requireVolume(t *testing.T, volumes []corev1.Volume, name string) *corev1.Volume {
	t.Helper()
	volume := findVolume(volumes, name)
	if volume == nil {
		t.Fatalf("volumes %#v do not include %q", volumes, name)
	}
	return volume
}

func requireVolumeMount(t *testing.T, mounts []corev1.VolumeMount, name string) *corev1.VolumeMount {
	t.Helper()
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i]
		}
	}
	t.Fatalf("volume mounts %#v do not include %q", mounts, name)
	return nil
}
