package reconciler

import (
	"context"

	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ ResourceReconciler[builder.DeploymentBuilder] = &DeploymentReconciler{}

// TODO should remove AnySpec? Now builder requires AnySpec, and reconciler requires builder,

type DeploymentReconciler struct {
	GenericResourceReconciler[AnySpec, builder.DeploymentBuilder]
	Ports         []corev1.ContainerPort
	RoleGroupInfo *RoleGroupInfo
}

// getReplicas returns the number of replicas for the role group.
// handle cluster operation stopped state.
func (r *DeploymentReconciler) getReplicas() *int32 {
	if r.RoleGroupInfo.ClusterOperation != nil && r.RoleGroupInfo.ClusterOperation.Stopped {
		logger.Info("Cluster operation stopped, set replicas to 0")
		zero := int32(0)
		return &zero
	}
	return r.RoleGroupInfo.Replicas
}

func (r *DeploymentReconciler) Reconcile(ctx context.Context) Result {
	resource, err := r.GetBuilder().
		SetReplicas(r.getReplicas()).
		Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *DeploymentReconciler) Ready(ctx context.Context) Result {
	obj := appv1.Deployment{}
	if err := r.GetClient().Get(ctx, &obj); err != nil {
		return NewResult(true, 0, err)
	}
	if obj.Status.ReadyReplicas == *obj.Spec.Replicas {
		logger.V(1).Info("Deployment is ready", "namespace", obj.Namespace, "name", obj.Name)
		return NewResult(false, 0, nil)
	}
	logger.V(1).Info("Deployment is not ready", "namespace", obj.Namespace, "name", obj.Name)
	return NewResult(false, 5, nil)
}

func NewDeploymentReconciler(
	client client.ResourceClient,
	roleGroupInfo *RoleGroupInfo,
	ports []corev1.ContainerPort,
	deployBuilder builder.DeploymentBuilder,
) *DeploymentReconciler {
	return &DeploymentReconciler{
		GenericResourceReconciler: *NewGenericResourceReconciler[AnySpec, builder.DeploymentBuilder](
			client,
			roleGroupInfo.GetFullName(),
			nil,
			deployBuilder,
		),
		RoleGroupInfo: roleGroupInfo,
		Ports:         ports,
	}
}
