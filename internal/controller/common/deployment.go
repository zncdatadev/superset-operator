package common

import (
	"context"

	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.Builder = &DeploymentBuilder{}

type DeploymentBuilder struct {
	builder.GenericDeploymentBuilder
	ClusterConfig *ClusterConfig
}

func NewDeploymentBuilder(
	client *client.Client,
	clusterConfig *ClusterConfig,
	options *builder.RoleGroupOptions,
) *DeploymentBuilder {
	return &DeploymentBuilder{
		GenericDeploymentBuilder: *builder.NewGenericDeploymentBuilder(
			client,
			options,
		),
		ClusterConfig: clusterConfig,
	}
}

func (b *DeploymentBuilder) GetMainContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewGenericContainerBuilder(
		b.Options.Name,
		b.Options.GetImage().String(),
		b.Options.GetImage().PullPolicy,
	)
	containerBuilder.AddEnvFromSecret(b.ClusterConfig.EnvSecretName)
	containerBuilder.SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; /usr/bin/run-server.sh"})
	containerBuilder.AddVolumeMount(corev1.VolumeMount{Name: "superset-config", MountPath: "/app/pythonpath", ReadOnly: true})
	containerBuilder.AddPorts(b.Options.GetPorts())
	containerBuilder.SetProbeWithHealth()
	return containerBuilder
}

func (b *DeploymentBuilder) GetInitContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewGenericContainerBuilder(
		"wait-for-postgres-redis",
		"apache/superset:dockerize",
		b.Options.GetImage().PullPolicy,
	)
	containerBuilder.SetCommand([]string{"/bin/sh", "-c", "dockerize -wait \"tcp://$DB_HOST:$DB_PORT\" -wait \"tcp://$REDIS_HOST:$REDIS_PORT\" -timeout 120s"})
	containerBuilder.AddEnvFromSecret(b.ClusterConfig.EnvSecretName)
	return containerBuilder
}

func (b *DeploymentBuilder) GetVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: "superset-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: b.ClusterConfig.ConfigSecretName,
			},
		},
	}
}

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	b.AddContainer(b.GetMainContainer().Build())
	b.AddInitContainer(b.GetInitContainer().Build())
	b.AddVolume(b.GetVolume())

	return b.GetObject(), nil
}
