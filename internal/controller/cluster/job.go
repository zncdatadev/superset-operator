package cluster

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// var _ builder.JobBuilder = &JobBuilder{}

type JobBuilder struct {
	builder.BaseWorkloadBuilder

	ClusterConfig    *supersetv1alpha1.ClusterConfigSpec
	EnvSecretName    string
	ConfigSecretName string
}

func NewJobBuilder(
	client *client.Client,
	name string,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	image *util.Image,
	options builder.WorkloadOptions,
) *JobBuilder {
	return &JobBuilder{
		BaseWorkloadBuilder: *builder.NewBaseWorkloadBuilder(
			client,
			name,
			image,
			options,
		),
		ClusterConfig: clusterConfig,
	}
}

func (b *JobBuilder) mainContainer() *corev1.Container {
	volumeMount := &corev1.VolumeMount{
		Name:      "superset-config",
		MountPath: "/app/pythonpath",
		ReadOnly:  true,
	}
	containerBuilder := builder.NewContainer(
		"superset-init",
		b.GetImageWithTag(),
	)
	containerBuilder.AddVolumeMount(volumeMount)
	// SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; . /app/pythonpath/superset_init.sh"})
	containerBuilder.SetCommand([]string{"tail", "-f"})
	containerBuilder.AddEnvFromSecret(b.EnvSecretName)

	existAdminSecretName := b.ClusterConfig.Administrator.ExistSecret
	if existAdminSecretName != "" {
		logger.Info("Using existing admin secret", "secret", existAdminSecretName, "namespace", b.Client.GetOwnerNamespace(), "name", b.GetName())
		containerBuilder.AddEnvFromSecret(existAdminSecretName)
	}
	return containerBuilder.Build()
}

func (b *JobBuilder) initContainer() *corev1.Container {
	containerBuilder := builder.NewContainer(
		"wait-for-postgres-redis",
		"apache/superset:dockerize",
	)
	containerBuilder.SetCommand([]string{"/bin/sh", "-c", "dockerize -wait \"tcp://$DB_HOST:$DB_PORT\" -wait \"tcp://$REDIS_HOST:$REDIS_PORT\" -timeout 120s"})
	containerBuilder.AddEnvFromSecret(b.EnvSecretName)
	return containerBuilder.Build()
}

func (b *JobBuilder) GetVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "superset-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: b.ConfigSecretName,
				},
			},
		},
	}
}

func (b *JobBuilder) GetObject() (*batchv1.Job, error) {

	obj := &batchv1.Job{
		ObjectMeta: b.GetObjectMeta(),
		Spec: batchv1.JobSpec{
			Selector: b.GetLabelSelector(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      b.GetLabels(),
					Annotations: b.GetAnnotations(),
				},
				Spec: corev1.PodSpec{
					InitContainers:                b.GetInitContainers(),
					Containers:                    b.GetContainers(),
					Volumes:                       b.GetVolumes(),
					Affinity:                      b.GetAffinity(),
					TerminationGracePeriodSeconds: b.GetTerminationGracePeriodSeconds(),
					ImagePullSecrets:              b.GetImagePullSecrets(),
					SecurityContext:               b.GetSecurityContext(),
				},
			},
		},
	}
	return obj, nil
}

func (b *JobBuilder) Build(_ context.Context) (k8sclient.Object, error) {
	b.AddContainer(b.mainContainer())
	b.AddInitContainer(b.initContainer())
	b.AddVolumes(b.GetVolumes())
	obj, err := b.GetObject()
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func NewJobReconciler(
	client *client.Client,
	clusterInfo reconciler.ClusterInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	image *util.Image,
) *reconciler.SimpleResourceReconciler[builder.ResourceBuilder] {

	options := builder.WorkloadOptions{
		Options: builder.Options{
			ClusterName: clusterInfo.GetFullName(),
			Labels:      clusterInfo.GetLabels(),
			Annotations: clusterInfo.GetAnnotations(),
		},
	}

	jobBuilder := NewJobBuilder(
		client,
		clusterInfo.GetFullName(),
		clusterConfig,
		envSecretName,
		configSecretName,
		image,
		options,
	)
	return reconciler.NewSimpleResourceReconciler[builder.ResourceBuilder](
		client,
		clusterInfo.GetFullName(),
		jobBuilder,
	)
}
