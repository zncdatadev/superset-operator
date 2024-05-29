package node

import (
	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewDeploymentReconciler(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options *builder.RoleGroupOptions,
	spec *supersetv1alpha1.NodeConfigSpec,
) *reconciler.DeploymentReconciler {
	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		clusterConfig,
		options,
	)

	var specAffinity *corev1.Affinity
	if spec != nil {
		specAffinity = spec.Affinity
	}
	addAffinityToStatefulSetBuilder(deploymentBuilder, specAffinity, options.RoleOptions.ClusterOptions.Name,
		options.RoleOptions.Name)

	return reconciler.NewDeploymentReconciler(
		client,
		options,
		deploymentBuilder,
	)
}

func addAffinityToStatefulSetBuilder(objectBuilder *common.DeploymentBuilder, specAffinity *corev1.Affinity,
	instanceName string, roleName string) {
	antiAffinityLabels := metav1.LabelSelector{
		MatchLabels: map[string]string{
			reconciler.LabelInstance:  instanceName,
			reconciler.LabelServer:    "superset",
			reconciler.LabelComponent: roleName,
		},
	}
	defaultAffinityBuilder := builder.AffinityBuilder{PodAffinity: []*builder.PodAffinity{
		builder.NewPodAffinity(builder.StrengthPrefer, true, antiAffinityLabels).Weight(70),
	}}

	objectBuilder.Affinity(specAffinity, defaultAffinityBuilder.Build())
}
