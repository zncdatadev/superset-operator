package builder

import (
	"context"

	resourceClient "github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type JobBuilder interface {
	Builder
	GetObject() *batchv1.Job
	AddContainers([]corev1.Container) JobBuilder
	AddContainer(corev1.Container) JobBuilder
	ResetContainers([]corev1.Container) JobBuilder
	GetContainers() []corev1.Container
	AddInitContainers([]corev1.Container) JobBuilder
	AddInitContainer(corev1.Container) JobBuilder
	ResetInitContainers([]corev1.Container) JobBuilder
	GetInitContainers() []corev1.Container
	AddVolumes([]corev1.Volume) JobBuilder
	AddVolume(corev1.Volume) JobBuilder
	ResetVolumes([]corev1.Volume) JobBuilder
	GetVolumes() []corev1.Volume
}

type GenericJobBuilder struct {
	BaseResourceBuilder
	Image util.Image

	obj *batchv1.Job
}

func NewGenericJobBuilder(
	client resourceClient.ResourceClient,
	name string,
	image util.Image,
) *GenericJobBuilder {
	return &GenericJobBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client: client,
			Name:   name,
		},
		Image: image,
	}
}

func (b *GenericJobBuilder) GetObject() *batchv1.Job {
	if b.obj == nil {
		b.obj = &batchv1.Job{
			ObjectMeta: b.GetObjectMeta(),
			Spec: batchv1.JobSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: b.Client.GetLabels(),
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      b.Client.GetLabels(),
						Annotations: b.Client.GetAnnotations(),
					},
				},
			},
		}
	}
	return b.obj
}

func (b *GenericJobBuilder) AddContainers(containers []corev1.Container) JobBuilder {
	c := b.GetObject().Spec.Template.Spec.Containers
	c = append(c, containers...)
	b.GetObject().Spec.Template.Spec.Containers = c
	return b
}

func (b *GenericJobBuilder) AddContainer(container corev1.Container) JobBuilder {
	return b.AddContainers([]corev1.Container{container})
}

func (b *GenericJobBuilder) ResetContainers(containers []corev1.Container) JobBuilder {
	b.GetObject().Spec.Template.Spec.Containers = containers
	return b
}

func (b *GenericJobBuilder) GetContainers() []corev1.Container {
	return b.GetObject().Spec.Template.Spec.Containers
}

func (b *GenericJobBuilder) AddInitContainers(containers []corev1.Container) JobBuilder {
	c := b.GetObject().Spec.Template.Spec.InitContainers
	c = append(c, containers...)
	b.GetObject().Spec.Template.Spec.InitContainers = c
	return b
}

func (b *GenericJobBuilder) AddInitContainer(container corev1.Container) JobBuilder {
	return b.AddInitContainers([]corev1.Container{container})
}

func (b *GenericJobBuilder) ResetInitContainers(containers []corev1.Container) JobBuilder {
	b.GetObject().Spec.Template.Spec.InitContainers = containers
	return b
}

func (b *GenericJobBuilder) GetInitContainers() []corev1.Container {
	return b.GetObject().Spec.Template.Spec.InitContainers
}

func (b *GenericJobBuilder) AddVolumes(volumes []corev1.Volume) JobBuilder {
	v := b.GetObject().Spec.Template.Spec.Volumes
	v = append(v, volumes...)
	b.GetObject().Spec.Template.Spec.Volumes = v
	return b
}

func (b *GenericJobBuilder) AddVolume(volume corev1.Volume) JobBuilder {
	return b.AddVolumes([]corev1.Volume{volume})
}

func (b *GenericJobBuilder) ResetVolumes(volumes []corev1.Volume) JobBuilder {
	b.GetObject().Spec.Template.Spec.Volumes = volumes
	return b
}

func (b *GenericJobBuilder) GetVolumes() []corev1.Volume {
	return b.GetObject().Spec.Template.Spec.Volumes
}

func (b *GenericJobBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
}
