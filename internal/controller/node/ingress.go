package node

import (
	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
)

type IngressReconciler struct {
	reconciler.BaseResourceReconciler[*supersetv1alpha1.NodeRoleGroupSpec]
	Ports []corev1.ContainerPort
}

func (r *IngressReconciler) Build() (*networkv1.Ingress, error) {
	panic("unimplemented")
}

func NewIngressReconciler(
	client reconciler.ResourceClient,
	roleGroupName string,
	spec *supersetv1alpha1.NodeRoleGroupSpec,
) *IngressReconciler {
	return &IngressReconciler{
		BaseResourceReconciler: *reconciler.NewBaseResourceReconciler(
			client,
			roleGroupName,
			spec,
		),
	}
}
