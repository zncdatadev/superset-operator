package common

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

var _ builder.DeploymentBuilder = &DeploymentBuilder{}

type DeploymentBuilder struct {
	builder.Deployment
	Ports            []corev1.ContainerPort
	ClusterConfig    *supersetv1alpha1.ClusterConfigSpec
	RoleGroupInfo    *builder.RoleGroupInfo
	EnvSecretName    string
	ConfigSecretName string
}

func NewDeploymentBuilder(
	client *client.Client,
	name string,
	roleGroupInfo *builder.RoleGroupInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	replicas *int32,
	ports []corev1.ContainerPort,
	image *util.Image,
	options *builder.WorkloadOptions,
) *DeploymentBuilder {
	return &DeploymentBuilder{
		Deployment: *builder.NewDeployment(
			client,
			name,
			replicas,
			image,
			options,
		),
		ClusterConfig:    clusterConfig,
		RoleGroupInfo:    roleGroupInfo,
		EnvSecretName:    envSecretName,
		ConfigSecretName: configSecretName,
		Ports:            ports,
	}
}

func (b *DeploymentBuilder) GetMainContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewContainer(
		b.RoleGroupInfo.RoleName,
		b.GetImage(),
	)
	containerBuilder.AddEnvFromSecret(b.EnvSecretName)
	containerBuilder.SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; /usr/bin/run-server.sh"})
	containerBuilder.AddVolumeMount(&corev1.VolumeMount{Name: "superset-config", MountPath: "/app/pythonpath", ReadOnly: true})
	containerBuilder.AddPorts(b.Ports)
	containerBuilder.SetProbeWithHealth()
	return containerBuilder
}

func (b *DeploymentBuilder) GetInitContainer() builder.ContainerBuilder {
	containerBuilder := builder.NewContainer(
		"wait-for-postgres-redis",
		"apache/superset:dockerize",
	)
	containerBuilder.SetCommand([]string{"/bin/sh", "-c", "dockerize -wait \"tcp://$DB_HOST:$DB_PORT\" -wait \"tcp://$REDIS_HOST:$REDIS_PORT\" -timeout 120s"})
	containerBuilder.AddEnvFromSecret(b.EnvSecretName)
	return containerBuilder
}

func (b *DeploymentBuilder) GetVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: "superset-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: b.ConfigSecretName,
			},
		},
	}
}

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	b.AddContainer(b.GetMainContainer().Build())
	b.AddInitContainer(b.GetInitContainer().Build())
	b.AddVolume(b.GetVolume())

	return b.GetObject()
}
