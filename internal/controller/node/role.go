package node

import (
	"context"
	"strconv"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
)

var _ reconciler.RoleReconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseRoleReconciler[*supersetv1alpha1.NodeSpec]
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
	Image         *util.Image
}

func NewReconciler(
	client *resourceClient.Client,
	clusterStopped bool,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	roleInfo reconciler.RoleInfo,
	image *util.Image,
	spec *supersetv1alpha1.NodeSpec,
) *Reconciler {
	return &Reconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler(
			client,
			clusterStopped,
			roleInfo,
			spec,
		),
		ClusterConfig: clusterConfig,
		Image:         image,
	}
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {
	for name, rg := range r.Spec.RoleGroups {

		mergedConfig, err := util.MergeObject(r.Spec.Config, rg.Config)
		if err != nil {
			return err
		}
		overrides, err := util.MergeObject(r.Spec.OverridesSpec, rg.OverridesSpec)
		if err != nil {
			return err
		}

		info := reconciler.RoleGroupInfo{
			RoleInfo:      r.RoleInfo,
			RoleGroupName: name,
		}

		var roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec
		if mergedConfig != nil {
			roleGroupConfig = mergedConfig.RoleGroupConfigSpec
		}
		reconcilers, err := r.RegisterResourceWithRoleGroup(
			ctx,
			rg.Replicas,
			info,
			overrides,
			roleGroupConfig,
		)

		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
		}
	}
	return nil
}

func (r *Reconciler) RegisterResourceWithRoleGroup(
	ctx context.Context,
	replicas *int32,
	info reconciler.RoleGroupInfo,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
) ([]reconciler.Reconciler, error) {

	configmapReconciler := common.NewConfigReconciler(
		r.Client,
		r.ClusterConfig,
		info,
	)

	stsReconciler, err := NewStatefulSetReconciler(
		r.Client,
		info,
		r.ClusterConfig,
		Ports,
		r.Image,
		replicas,
		r.ClusterStopped(),
		overrides,
		roleGroupConfig,
	)
	if err != nil {
		return nil, err
	}

	// Merge Prometheus annotations with existing annotations
	annotations := info.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	prometheusAnnotations := GetPrometheusAnnotations(Ports)
	for k, v := range prometheusAnnotations {
		annotations[k] = v
	}

	serviceReconciler := reconciler.NewServiceReconciler(
		r.Client,
		info.GetFullName(),
		Ports,
		func(o *builder.ServiceBuilderOptions) {
			o.ListenerClass = constants.ExternalUnstable
			o.ClusterName = info.GetClusterName()
			o.RoleName = info.GetRoleName()
			o.RoleGroupName = info.GetGroupName()
			o.Labels = info.GetLabels()
			o.Annotations = annotations
		},
	)
	return []reconciler.Reconciler{configmapReconciler, stsReconciler, serviceReconciler}, nil
}

// Common annotations for Prometheus scraping
func GetPrometheusAnnotations(ports []corev1.ContainerPort) map[string]string {
	metricsPort := 0
	for _, port := range ports {
		if port.Name == "metrics" {
			metricsPort = int(port.ContainerPort)
			break
		}
	}

	if metricsPort > 0 {
		return map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   strconv.Itoa(metricsPort),
			"prometheus.io/path":   "/metrics",
			"prometheus.io/scheme": "http",
		}
	}

	return nil
}
