package reconciler

import (
	"context"

	"github.com/zncdata-labs/superset-operator/pkg/image"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ ResourceReconciler[*appv1.StatefulSet] = &DeploymentReconciler[AnySpec]{}

type DeploymentReconciler[T AnySpec] struct {
	BaseResourceReconciler[T]
	Ports []corev1.ContainerPort
	Image image.Image
}

func (s *DeploymentReconciler[T]) GetSpec() T {
	return s.Spec
}

func (s *DeploymentReconciler[T]) Build(ctx context.Context) (*appv1.StatefulSet, error) {
	panic("unimplemented")
}

func (s *DeploymentReconciler[T]) Ready() Result {
	panic("unimplemented")
}

func (s *DeploymentReconciler[T]) Reconcile() Result {
	panic("unimplemented")
}

func (s *DeploymentReconciler[T]) AddFinalizer(obj *appv1.StatefulSet) {
	panic("unimplemented")
}

func NewDeploymentReconciler[T AnySpec](
	client ResourceClient,
	name string,
	image image.Image,
	ports []corev1.ContainerPort,
	spec T,
) *DeploymentReconciler[T] {
	return &DeploymentReconciler[T]{
		BaseResourceReconciler: BaseResourceReconciler[T]{
			BaseReconciler: BaseReconciler[T]{
				Client: client,
				Name:   name,
				Spec:   spec,
			},
		},
		Ports: ports,
		Image: image,
	}
}
