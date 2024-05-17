package worker

import (
	"context"

	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
)

type ServiceReconciler struct {
	reconciler.BaseResourceReconciler[*supersetv1alpha1.WorkerRoleGroupSpec]
	Ports []corev1.ContainerPort
}

func (r *ServiceReconciler) Build(_ context.Context) (*corev1.Service, error) {
	var ports []corev1.ServicePort

	for _, port := range r.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:     port.Name,
			Port:     port.ContainerPort,
			Protocol: port.Protocol,
		})
	}

	obj := &corev1.Service{
		ObjectMeta: r.GetObjectMeta(),
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": r.GetName(),
			},
			Ports: ports,
		},
	}
	return obj, nil
}

func NewServiceReconciler(
	client reconciler.ResourceClient,
	roleGroupName string,
	ports []corev1.ContainerPort,
	spec *supersetv1alpha1.WorkerRoleGroupSpec,
) *ServiceReconciler {
	return &ServiceReconciler{
		BaseResourceReconciler: *reconciler.NewBaseResourceReconciler(
			client,
			roleGroupName,
			spec,
		),
		Ports: ports,
	}
}
