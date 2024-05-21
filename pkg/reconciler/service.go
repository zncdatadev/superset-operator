package reconciler

import (
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
)

var _ ResourceReconciler[builder.ServiceBuilder] = &GenericServiceReconciler{}

type GenericServiceReconciler struct {
	GenericResourceReconciler[AnySpec, builder.ServiceBuilder]
	Ports []corev1.ContainerPort
}

func NewServiceReconciler(
	client client.ResourceClient,
	roleGroupName string,
	ports []corev1.ContainerPort,
) *GenericServiceReconciler {

	svcBuilder := builder.NewServiceBuilder(
		client,
		roleGroupName,
		ports,
	)
	return &GenericServiceReconciler{
		GenericResourceReconciler: *NewGenericResourceReconciler[AnySpec, builder.ServiceBuilder](
			client,
			roleGroupName,
			nil,
			svcBuilder,
		),
		Ports: ports,
	}
}
