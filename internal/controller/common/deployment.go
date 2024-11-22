package common

import (
	"context"
	"fmt"
	"path"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

var (
	LogVolumeName    = "log"
	ConfigVolumeName = "config"
)

var _ builder.DeploymentBuilder = &DeploymentBuilder{}

type DeploymentBuilder struct {
	builder.Deployment
	Ports         []corev1.ContainerPort
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
	ClusterName   string
	RoleName      string
}

func NewDeploymentBuilder(
	client *client.Client,
	roleGroupInfo reconciler.RoleGroupInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	replicas *int32,
	ports []corev1.ContainerPort,
	image *util.Image,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
) *DeploymentBuilder {
	return &DeploymentBuilder{
		Deployment: *builder.NewDeployment(
			client,
			roleGroupInfo.GetFullName(),
			replicas,
			image,
			overrides,
			roleGroupConfig,
			func(o *builder.Options) {
				o.ClusterName = roleGroupInfo.GetClusterName()
				o.RoleName = roleGroupInfo.GetRoleName()
				o.Labels = roleGroupInfo.GetLabels()
				o.Annotations = roleGroupInfo.GetAnnotations()
			},
		),
		ClusterConfig: clusterConfig,
		Ports:         ports,
		ClusterName:   roleGroupInfo.GetClusterName(),
		RoleName:      roleGroupInfo.GetRoleName(),
	}
}

func (b *DeploymentBuilder) GetMainCommands() string {

	cmds := `
mkdir --parents /kubedoop/app/pythonpath

cp /kubedoop/mount/config/* /kubedoop/app/pythonpath


prepare_signal_handlers()
{
	unset term_child_pid
	unset term_kill_needed
	trap 'handle_term_signal' TERM
}


handle_term_signal()
{
	if [ "${term_child_pid}" ]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	else
		term_kill_needed="yes"
	fi
}


wait_for_termination()
{
	set +e
	term_child_pid=$1
	if [[ -v term_kill_needed ]]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	fi
	wait ${term_child_pid} 2>/dev/null
	trap - TERM
	wait ${term_child_pid} 2>/dev/null
	set -e
}


superset db upgrade

set +x # Disable debug mode
superset fab create-admin \
	--username "${ADMIN_USERNAME}" \
	--firstname "${ADMIN_FIRSTNAME}" \
	--lastname "${ADMIN_LASTNAME}" \
	--email "${ADMIN_EMAIL}" \
	--password "${ADMIN_PASSWORD}"

set -x # Enable debug mode

superset init

rm -rf /kubedoop/log/_vector/shutdown

prepare_signal_handlers


gunicorn \
	--bind 0.0.0.0:${SUPERSET_PORT} \
	--threads 20 \
	--timeout 300 \
	--limit-request-line 0 \
	--limit-request-field_size 0 \
	'superset.app:create_app()' &


wait_for_termination $!


mkdir -p /kubedoop/log/_vector/ && touch /kubedoop/log/_vector/shutdown
`

	return util.IndentTab4Spaces(cmds)
}

func (b *DeploymentBuilder) GetInitContainerCommands() string {
	cmds := `
prepare_signal_handlers()
{
	unset term_child_pid
	unset term_kill_needed
	trap 'handle_term_signal' TERM
}


handle_term_signal()
{
	if [ "${term_child_pid}" ]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	else
		term_kill_needed="yes"
	fi
}


wait_for_termination()
{
	set +e
	term_child_pid=$1
	if [[ -v term_kill_needed ]]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	fi
	wait ${term_child_pid} 2>/dev/null
	trap - TERM
	wait ${term_child_pid} 2>/dev/null
	set -e
}


prepare_signal_handlers
/kubedoop/bin/statsd-exporter &
wait_for_termination $!
`
	return util.IndentTab4Spaces(cmds)
}

func (b *DeploymentBuilder) getAppPort() int32 {
	var portNum int32
	for _, port := range b.Ports {
		if port.Name == "http" {
			portNum = port.ContainerPort
			break
		}
	}
	return portNum
}

func (b *DeploymentBuilder) GetMetricsContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewContainer(
		"metrics",
		b.GetImage(),
	)
	containerBuilder.SetCommand([]string{"sh", "-x", "-c"})
	containerBuilder.SetArgs([]string{b.GetInitContainerCommands()})
	return containerBuilder
}

func (b *DeploymentBuilder) GetMainContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewContainer(
		b.RoleName,
		b.GetImage(),
	)
	containerBuilder.SetCommand([]string{"sh", "-x", "-c"})
	containerBuilder.SetArgs([]string{b.GetMainCommands()})
	containerBuilder.SetProbeWithHealth()
	containerBuilder.AddPorts(b.Ports)

	containerBuilder.AddEnvVars([]corev1.EnvVar{
		{Name: "SUPERSET_PORT", Value: fmt.Sprintf("%d", b.getAppPort())},
	})

	containerBuilder.AddVolumeMount(&corev1.VolumeMount{Name: ConfigVolumeName, MountPath: "/kubedoop/mount/config", ReadOnly: true})

	if b.ClusterConfig.CredentialsSecret != "" {
		InjectCredentials(b.ClusterConfig.CredentialsSecret, containerBuilder)
	}

	if b.ClusterConfig.Authentication != nil && b.ClusterConfig.Authentication.Oidc != nil {
		containerBuilder.AddEnvFromSecret(b.ClusterConfig.Authentication.Oidc.ClientCredentialsSecret)
	}

	return containerBuilder
}

func (b *DeploymentBuilder) GetDefaultAffinityBuilder() *AffinityBuilder {
	antiAffinityLabels := map[string]string{
		constants.LabelKubernetesInstance:  b.ClusterName,
		constants.LabelKubernetesName:      "hbase",
		constants.LabelKubernetesComponent: b.RoleName,
	}

	affinity := NewAffinityBuilder(
		*NewPodAffinity(antiAffinityLabels, false, true).Weight(70),
	)

	return affinity
}

func (b *DeploymentBuilder) addAuthLdapCredentials(ctx context.Context) error {
	if b.ClusterConfig.Authentication == nil || b.ClusterConfig.Authentication.AuthenticationClass == "" {
		return nil
	}
	authClass := &authv1alpha1.AuthenticationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.ClusterConfig.Authentication.AuthenticationClass,
			Namespace: b.Client.GetOwnerNamespace(),
		},
	}
	if err := b.Client.GetWithObject(ctx, authClass); err != nil {
		return err
	}

	if authClass.Spec.AuthenticationProvider.LDAP == nil {
		return nil
	}

	ldapProvider := authClass.Spec.AuthenticationProvider.LDAP

	credentials := ldapProvider.BindCredentials

	scopes := []string{}

	if credentials.Scope != nil {
		if credentials.Scope.Node {
			scopes = append(scopes, "node")
		}
		if credentials.Scope.Pod {
			scopes = append(scopes, "pod")
		}
		if len(credentials.Scope.Services) > 0 {
			scopes = append(scopes, credentials.Scope.Services...)
		}

	}

	b.addSecretVolume("ldap-bind-credentials", credentials.SecretClass, scopes)

	return nil

}

func (b *DeploymentBuilder) addSecretVolume(name string, secretClass string, scopes []string) {
	secretVolume := &corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							constants.AnnotationSecretsClass: secretClass,
							constants.AnnotationSecretsScope: strings.Join(scopes, constants.CommonDelimiter),
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
						StorageClassName: constants.SecretStorageClassPtr(),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Mi"),
							},
						},
					},
				},
			},
		},
	}

	b.AddVolume(secretVolume)

	secretVolumeMount := &corev1.VolumeMount{
		Name:      name,
		MountPath: path.Join(constants.KubedoopSecretDir, secretClass),
		ReadOnly:  true,
	}

	b.GetMainContainer().AddVolumeMount(secretVolumeMount)
}

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {

	b.AddContainer(b.GetMainContainer().Build())
	b.AddVolume(
		&corev1.Volume{
			Name: ConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode:          &[]int32{420}[0],
					LocalObjectReference: corev1.LocalObjectReference{Name: b.Name},
				},
			},
		},
	)
	b.AddContainer(b.GetMetricsContainer().Build())
	b.SetAffinity(b.GetDefaultAffinityBuilder().Build())

	if err := b.addAuthLdapCredentials(ctx); err != nil {
		return nil, err
	}

	obj, err := b.GetObject()
	if err != nil {
		return nil, err
	}

	if b.ClusterConfig != nil && b.ClusterConfig.VectorAggregatorConfigMapName != "" {
		builder.NewVectorDecorator(
			obj,
			b.GetImage(),
			LogVolumeName,
			ConfigVolumeName,
			b.ClusterConfig.VectorAggregatorConfigMapName,
		)
	}

	return obj, nil
}
