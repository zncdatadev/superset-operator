package node

import (
	"context"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

var _ reconciler.RoleReconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseRoleReconciler[*supersetv1alpha1.NodeSpec]
	ClusterConfig    *supersetv1alpha1.ClusterConfigSpec
	EnvSecretName    string
	ConfigSecretName string
	Image            *util.Image
}

func NewReconciler(
	client *resourceClient.Client,
	roleInfo reconciler.RoleInfo,
	clusterOperation *commonsv1alpha1.ClusterOperationSpec,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	image *util.Image,
	spec *supersetv1alpha1.NodeSpec,
) *Reconciler {
	return &Reconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler(
			client,
			roleInfo,
			clusterOperation,
			spec,
		),
		ClusterConfig:    clusterConfig,
		EnvSecretName:    envSecretName,
		ConfigSecretName: configSecretName,
		Image:            image,
	}
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {
	for name, rg := range r.Spec.RoleGroups {
		mergedRoleGroup := rg.DeepCopy()
		r.MergeRoleGroupSpec(mergedRoleGroup)

		info := reconciler.RoleGroupInfo{
			RoleInfo:      r.RoleInfo,
			RoleGroupName: name,
		}

		reconcilers, err := r.RegisterResourceWithRoleGroup(ctx, info, mergedRoleGroup)

		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
		}
	}
	return nil
}

func (r *Reconciler) RegisterResourceWithRoleGroup(ctx context.Context, info reconciler.RoleGroupInfo, spec *supersetv1alpha1.NodeRoleGroupSpec) ([]reconciler.Reconciler, error) {

	stopped := false

	if r.ClusterOperation != nil && r.ClusterOperation.Stopped {
		stopped = true
	}

	deploymentReconciler, err := NewDeploymentReconciler(
		r.Client,
		info,
		r.ClusterConfig,
		r.EnvSecretName,
		r.ConfigSecretName,
		Ports,
		r.Image,
		stopped,
		spec,
	)
	if err != nil {
		return nil, err
	}

	return []reconciler.Reconciler{deploymentReconciler}, nil
}
