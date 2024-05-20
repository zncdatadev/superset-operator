package common

import (
	"context"

	"github.com/zncdata-labs/superset-operator/pkg/builder"
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.Builder = &DeploymentBuilder{}

type DeploymentBuilder struct {
	builder.GenericDeploymentBuilder
	Ports         []corev1.ContainerPort
	ClusterConfig *ClusterConfig
}

func NewDeploymentBuilder(
	client resourceClient.ResourceClient,
	clusterConfig *ClusterConfig,
	roleGroupInfo *reconciler.RoleGroupInfo,
	ports []corev1.ContainerPort,
	envOverrides map[string]string,
	commandOverrides []string,
) *DeploymentBuilder {
	return &DeploymentBuilder{
		GenericDeploymentBuilder: *builder.NewGenericDeploymentBuilder(
			client,
			roleGroupInfo.GetFullName(),
			envOverrides,
			commandOverrides,
			roleGroupInfo.Image,
		),
		Ports:         ports,
		ClusterConfig: clusterConfig,
	}
}

func (b *DeploymentBuilder) GetMainContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewGenericContainerBuilder(
		b.Name,
		b.Image.String(),
		b.Image.PullPolicy,
	).AddEnvFromSecret(b.ClusterConfig.EnvSecretName).
		SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; /usr/bin/run-server.sh"}).
		AddVolumeMount(corev1.VolumeMount{Name: "superset-config", MountPath: "/app/superset", ReadOnly: true}).
		AddPorts(b.Ports).
		SetProbeWithHealth()
	return containerBuilder
}

func (b *DeploymentBuilder) GetInitContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewGenericContainerBuilder(
		"wait-for-postgres-redis",
		b.Image.String(),
		b.Image.PullPolicy,
	).
		SetCommand([]string{"/bin/sh", "-c", "dockerize -wait \"tcp://$DB_HOST:$DB_PORT\" -timeout 120s"}).
		AddEnvFromSecret(b.ClusterConfig.EnvSecretName)
	return containerBuilder
}

func (b *DeploymentBuilder) GetVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: "superset-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: b.ClusterConfig.EnvSecretName,
			},
		},
	}
}

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	b.AddContainer(b.GetInitContainer().Build())
	b.AddInitContainer(b.GetInitContainer().Build())
	b.AddVolume(b.GetVolume())

	return b.GetObject(), nil
}
