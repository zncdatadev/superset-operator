package builder

import (
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"github.com/zncdata-labs/superset-operator/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentBuilder interface {
	Builder
	GetObject() *appv1.Deployment
	SetReplicas(replicas *int32) DeploymentBuilder

	AddContainer(*corev1.Container) DeploymentBuilder
	AddContainers([]corev1.Container) DeploymentBuilder
	ResetContainers(containers []corev1.Container) DeploymentBuilder

	AddInitContainer(*corev1.Container) DeploymentBuilder
	AddInitContainers([]corev1.Container) DeploymentBuilder
	ResetInitContainers(containers []corev1.Container) DeploymentBuilder

	AddVolume(*corev1.Volume) DeploymentBuilder
	AddVolumes([]corev1.Volume) DeploymentBuilder
	ResetVolumes(volumes []corev1.Volume) DeploymentBuilder

	AddTerminationGracePeriodSeconds(seconds *int64) DeploymentBuilder
	AddAffinity(*corev1.Affinity) DeploymentBuilder
}

var _ DeploymentBuilder = &GenericDeploymentBuilder{}

type GenericDeploymentBuilder struct {
	BaseResourceBuilder
	EnvOverrides     map[string]string
	CommandOverrides []string
	Image            util.Image

	obj *appv1.Deployment
}

func NewGenericDeploymentBuilder(
	client resourceClient.ResourceClient,
	name string,
	envOverrides map[string]string,
	commandOverrides []string,
	image util.Image,
) *GenericDeploymentBuilder {
	return &GenericDeploymentBuilder{
		BaseResourceBuilder: BaseResourceBuilder{
			Client: client,
			Name:   name,
		},
		EnvOverrides:     envOverrides,
		CommandOverrides: commandOverrides,
		Image:            image,
	}
}

func (b *GenericDeploymentBuilder) GetObject() *appv1.Deployment {
	if b.obj == nil {
		b.obj = &appv1.Deployment{
			ObjectMeta: b.GetObjectMeta(),
			Spec: appv1.DeploymentSpec{
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

func (b *GenericDeploymentBuilder) SetReplicas(replicas *int32) DeploymentBuilder {
	b.GetObject().Spec.Replicas = replicas
	return b
}

func (b *GenericDeploymentBuilder) AddContainer(container *corev1.Container) DeploymentBuilder {
	c := b.GetObject().Spec.Template.Spec.Containers
	c = append(c, *container)
	b.GetObject().Spec.Template.Spec.Containers = c
	return b
}

func (b *GenericDeploymentBuilder) AddContainers(containers []corev1.Container) DeploymentBuilder {
	c := b.GetObject().Spec.Template.Spec.Containers
	c = append(c, containers...)
	b.GetObject().Spec.Template.Spec.Containers = c
	return b
}

func (b *GenericDeploymentBuilder) ResetContainers(containers []corev1.Container) DeploymentBuilder {
	b.GetObject().Spec.Template.Spec.Containers = containers
	return b
}
func (b *GenericDeploymentBuilder) AddInitContainers(containers []corev1.Container) DeploymentBuilder {
	c := b.GetObject().Spec.Template.Spec.InitContainers
	c = append(c, containers...)
	b.GetObject().Spec.Template.Spec.InitContainers = c
	return b
}

func (b *GenericDeploymentBuilder) ResetInitContainers(containers []corev1.Container) DeploymentBuilder {
	b.GetObject().Spec.Template.Spec.InitContainers = containers
	return b
}

func (b *GenericDeploymentBuilder) AddInitContainer(container *corev1.Container) DeploymentBuilder {
	c := b.GetObject().Spec.Template.Spec.InitContainers
	c = append(c, *container)
	b.GetObject().Spec.Template.Spec.InitContainers = c
	return b
}

func (b *GenericDeploymentBuilder) AddVolume(volume *corev1.Volume) DeploymentBuilder {
	v := b.GetObject().Spec.Template.Spec.Volumes
	v = append(v, *volume)
	b.GetObject().Spec.Template.Spec.Volumes = v
	return b
}
func (b *GenericDeploymentBuilder) AddVolumes(volumes []corev1.Volume) DeploymentBuilder {
	v := b.GetObject().Spec.Template.Spec.Volumes
	v = append(v, volumes...)
	b.GetObject().Spec.Template.Spec.Volumes = v
	return b
}

func (b *GenericDeploymentBuilder) ResetVolumes(volumes []corev1.Volume) DeploymentBuilder {
	b.GetObject().Spec.Template.Spec.Volumes = volumes
	return b
}

func (b *GenericDeploymentBuilder) AddTerminationGracePeriodSeconds(seconds *int64) DeploymentBuilder {
	b.GetObject().Spec.Template.Spec.TerminationGracePeriodSeconds = seconds
	return b
}

func (b *GenericDeploymentBuilder) AddAffinity(affinity *corev1.Affinity) DeploymentBuilder {
	b.GetObject().Spec.Template.Spec.Affinity = affinity
	return b
}
