package cluster

import (
	"context"

	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	"github.com/zncdatadev/superset-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.JobBuilder = &JobBuilder{}

type JobBuilder struct {
	builder.GenericJobBuilder
	ClusterConfig *common.ClusterConfig
}

func NewJobBuilder(
	client client.ResourceClient,
	name string,
	image util.Image,
	clusterConfig *common.ClusterConfig,
) *JobBuilder {
	return &JobBuilder{
		GenericJobBuilder: *builder.NewGenericJobBuilder(
			client,
			name,
			image,
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
	container := builder.NewGenericContainerBuilder(
		"superset-init",
		b.Image.String(),
		b.Image.PullPolicy,
	).
		AddVolumeMount(volumeMount).
		SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; . /app/pythonpath/superset_init.sh"}).
		AddEnvFromSecret(b.ClusterConfig.ConfigSecretName).
		Build()
	return container
}

func (b *JobBuilder) initContainer() *corev1.Container {
	container := builder.NewGenericContainerBuilder(
		"superset-init",
		b.Image.String(),
		b.Image.PullPolicy,
	).
		SetCommand([]string{"/bin/sh", "-c", "dockerize -wait \"tcp://$DB_HOST:$DB_PORT\" -timeout 120s"}).
		AddEnvFromSecret(b.ClusterConfig.ConfigSecretName).
		Build()
	return container
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

func (b *JobBuilder) Build(ctx context.Context) (k8sclient.Object, error) {
	b.AddContainer(*b.mainContainer())
	b.AddInitContainer(*b.initContainer())
	return b.GetObject(), nil
}

func NewJobReconciler(
	client client.ResourceClient,
	clusterInfo *reconciler.ClusterInfo,
	clusterConfig *common.ClusterConfig,
) *reconciler.SimpleResourceReconciler[builder.JobBuilder] {
	name := "superset-init"
	jobBuilder := NewJobBuilder(
		client,
		name,
		clusterInfo.Image,
		clusterConfig,
	)
	return reconciler.NewSimpleResourceReconciler[builder.JobBuilder](
		client,
		name,
		jobBuilder,
	)
}
