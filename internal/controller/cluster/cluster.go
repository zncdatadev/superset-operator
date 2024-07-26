package cluster

import (
	"context"
	"fmt"

	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/node"
	"github.com/zncdatadev/superset-operator/internal/controller/worker"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	logger = ctrl.Log.WithName("controller").WithName("Cluster")
)

var _ reconciler.Reconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseCluster[*supersetv1alpha1.SupersetClusterSpec]
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
}

func NewReconciler(
	client *resourceClient.Client,
	clusterInfo reconciler.ClusterInfo,
	spec *supersetv1alpha1.SupersetClusterSpec,
) *Reconciler {

	return &Reconciler{
		BaseCluster: *reconciler.NewBaseCluster(
			client,
			clusterInfo,
			spec.ClusterOperation,
			spec,
		),
		ClusterConfig: spec.ClusterConfig,
	}

}

func (r *Reconciler) GetImage() *util.Image {
	image := &util.Image{
		Repository:     supersetv1alpha1.DefaultRepository,
		ProductName:    "superset",
		StackVersion:   "0.0.1",
		ProductVersion: supersetv1alpha1.DefaultProductVersion,
		PullPolicy:     &[]corev1.PullPolicy{corev1.PullIfNotPresent}[0],
	}

	if r.Spec.Image != nil {
		image.Custom = r.Spec.Image.Custom
		image.Repository = r.Spec.Image.Repository
		image.StackVersion = r.Spec.Image.StackVersion
		image.ProductVersion = r.Spec.Image.ProductVersion
		image.PullPolicy = &r.Spec.Image.PullPolicy
	}
	return image
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {

	envSecretName := fmt.Sprintf("%s-env", r.ClusterInfo.ClusterName)
	configSecretName := fmt.Sprintf("%s-config", r.ClusterInfo.ClusterName)

	envSecretReconciler := NewEnvSecretReconciler(
		r.Client,
		envSecretName,
		r.ClusterInfo,
		r.ClusterConfig,
	)
	r.AddResource(envSecretReconciler)

	configSecretReconciler := NewConfigSecretReconciler(
		r.Client,
		configSecretName,
		r.ClusterInfo,
		r.ClusterConfig,
	)
	r.AddResource(configSecretReconciler)

	job := NewJobReconciler(
		r.Client,
		r.ClusterInfo,
		r.ClusterConfig,
		envSecretName,
		configSecretName,
		r.GetImage(),
	)
	r.AddResource(job)

	node := node.NewReconciler(
		r.Client,
		reconciler.RoleInfo{
			ClusterInfo: r.ClusterInfo,
			RoleName:    "node",
		},
		r.GetClusterOperation(),
		r.ClusterConfig,
		envSecretName,
		configSecretName,
		r.GetImage(),
		r.Spec.Node,
	)

	if err := node.RegisterResources(ctx); err != nil {
		return err
	}

	r.AddResource(node)

	worker := worker.NewReconciler(
		r.Client,
		reconciler.RoleInfo{
			ClusterInfo: r.ClusterInfo,
			RoleName:    "worker",
		},
		r.GetClusterOperation(),
		r.ClusterConfig,
		envSecretName,
		configSecretName,
		r.GetImage(),
		r.Spec.Worker,
	)

	if err := worker.RegisterResources(ctx); err != nil {
		return err
	}

	r.AddResource(worker)

	return nil

}
