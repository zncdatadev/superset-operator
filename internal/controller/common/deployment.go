package common

import (
	"context"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
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
	name string,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	replicas *int32,
	ports []corev1.ContainerPort,
	image *util.Image,
	options builder.WorkloadOptions,
) *DeploymentBuilder {
	return &DeploymentBuilder{
		Deployment: *builder.NewDeployment(
			client,
			name,
			replicas,
			image,
			options,
		),
		ClusterConfig: clusterConfig,
		Ports:         ports,
		ClusterName:   options.ClusterName,
		RoleName:      options.RoleName,
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
		b.GetImageWithTag(),
	)
	containerBuilder.SetCommand([]string{"sh", "-x", "-c"})
	containerBuilder.SetArgs([]string{b.GetInitContainerCommands()})
	return containerBuilder
}

func (b *DeploymentBuilder) GetMainContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewContainer(
		b.RoleName,
		b.GetImageWithTag(),
	)
	containerBuilder.SetCommand([]string{"sh", "-x", "-c"})
	containerBuilder.SetArgs([]string{b.GetMainCommands()})
	containerBuilder.SetProbeWithHealth()
	containerBuilder.AddPorts(b.Ports)

	containerBuilder.AddEnvVars([]corev1.EnvVar{
		{Name: "SUPERSET_PORT", Value: fmt.Sprintf("%d", b.getAppPort())},
	})

	containerBuilder.AddVolumeMount(&corev1.VolumeMount{Name: "superset-config", MountPath: "/kubedoop/mount/config", ReadOnly: true})

	if b.ClusterConfig.CredentialsSecret != "" {
		InjectCredentials(b.ClusterConfig.CredentialsSecret, containerBuilder)
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

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	b.AddContainer(b.GetMainContainer().Build())
	b.AddVolume(
		&corev1.Volume{
			Name: "superset-config",
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
	return b.GetObject()
}
