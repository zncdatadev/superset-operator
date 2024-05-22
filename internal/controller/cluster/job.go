package cluster

import (
	"context"

	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.JobBuilder = &JobBuilder{}

type JobBuilder struct {
	builder.GenericJobBuilder
	ClusterConfig *common.ClusterConfig
}

func NewJobBuilder(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options builder.Options,
) *JobBuilder {
	return &JobBuilder{
		GenericJobBuilder: *builder.NewGenericJobBuilder(
			client,
			options,
		),
		ClusterConfig: clusterConfig,
	}
}

func (b *JobBuilder) mainContainer() *corev1.Container {
	volumeMount := corev1.VolumeMount{
		Name:      "superset-config",
		MountPath: "/app/pythonpath",
		ReadOnly:  true,
	}
	containerBuilder := builder.NewGenericContainerBuilder(
		"superset-init",
		b.Options.GetImage().String(),
		b.Options.GetImage().PullPolicy,
	)
	containerBuilder.AddVolumeMount(volumeMount)
	// SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; . /app/pythonpath/superset_init.sh"})
	containerBuilder.SetCommand([]string{"tail", "-f"})
	containerBuilder.AddEnvFromSecret(b.ClusterConfig.ConfigSecretName)
	containerBuilder.AddEnvFromSecret(b.ClusterConfig.EnvSecretName)

	existAdminSecretName := b.ClusterConfig.Spec.Administrator.ExistSecret
	if existAdminSecretName != "" {
		logger.Info("Using existing admin secret", "secret", existAdminSecretName, "namespace", b.Client.GetOwnerNamespace(), "name", b.Options.GetName())
		containerBuilder.AddEnvFromSecret(existAdminSecretName)
	}
	return containerBuilder.Build()
}

func (b *JobBuilder) initContainer() *corev1.Container {
	containerBuilder := builder.NewGenericContainerBuilder(
		"wait-for-postgres-redis",
		"apache/superset:dockerize",
		b.Options.GetImage().PullPolicy,
	)
	containerBuilder.SetCommand([]string{"/bin/sh", "-c", "dockerize -wait \"tcp://$DB_HOST:$DB_PORT\" -wait \"tcp://$REDIS_HOST:$REDIS_PORT\" -timeout 120s"})
	containerBuilder.AddEnvFromSecret(b.ClusterConfig.EnvSecretName)
	return containerBuilder.Build()
}

func (b *JobBuilder) GetVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "superset-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: b.ClusterConfig.ConfigSecretName,
				},
			},
		},
	}
}

func (b *JobBuilder) Build(_ context.Context) (k8sclient.Object, error) {
	b.AddContainer(*b.mainContainer())
	b.AddInitContainer(*b.initContainer())
	b.AddVolumes(b.GetVolumes())
	b.SetRestPolicy(corev1.RestartPolicyNever)
	b.SetName(b.Options.GetName() + "-init")
	return b.GetObject(), nil
}

func NewJobReconciler(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options builder.Options,
) *reconciler.SimpleResourceReconciler[builder.JobBuilder] {
	jobBuilder := NewJobBuilder(
		client,
		clusterConfig,
		options,
	)
	return reconciler.NewSimpleResourceReconciler[builder.JobBuilder](
		client,
		options,
		jobBuilder,
	)
}
