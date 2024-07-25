package node

import (
	"errors"
	"time"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	corev1 "k8s.io/api/core/v1"
)

func NewDeploymentReconciler(
	client *client.Client,
	roleGroupInfo reconciler.RoleGroupInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	ports []corev1.ContainerPort,
	image *util.Image,
	spec *supersetv1alpha1.NodeRoleGroupSpec,
) (*reconciler.Deployment, error) {

	options := &builder.WorkloadOptions{
		Labels:           roleGroupInfo.GetLabels(),
		PodOverrides:     spec.PodOverride,
		EnvOverrides:     spec.EnvOverrides,
		CommandOverrides: spec.CommandOverrides,
		Resource:         spec.Config.Resources,
	}

	if spec.Config != nil {

		var gracefulShutdownTimeout time.Duration
		var err error

		if spec.Config.GracefulShutdownTimeout != "" {
			gracefulShutdownTimeout, err = time.ParseDuration(spec.Config.GracefulShutdownTimeout)

			if err != nil {
				return nil, errors.New("failed to parse graceful shutdown")
			}
		}

		options.TerminationGracePeriod = &gracefulShutdownTimeout

		options.Affinity = spec.Config.Affinity
	}

	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		roleGroupInfo.GetFullName(),
		&builder.RoleGroupInfo{
			ClusterName:   roleGroupInfo.ClusterName,
			RoleName:      roleGroupInfo.RoleName,
			RoleGroupName: roleGroupInfo.RoleGroupName,
		},
		clusterConfig,
		envSecretName,
		configSecretName,
		spec.Replicas,
		ports,
		image,
		options,
	)

	return reconciler.NewDeployment(
		client,
		roleGroupInfo.GetFullName(),
		deploymentBuilder,
	), nil
}

// func addAffinityToStatefulSetBuilder(objectBuilder *common.DeploymentBuilder, specAffinity *corev1.Affinity,
// 	instanceName string, roleName string) {
// 	antiAffinityLabels := metav1.LabelSelector{
// 		MatchLabels: map[string]string{
// 			reconciler.LabelInstance:  instanceName,
// 			reconciler.LabelServer:    "superset",
// 			reconciler.LabelComponent: roleName,
// 		},
// 	}
// 	defaultAffinityBuilder := builder.AffinityBuilder{PodAffinity: []*builder.PodAffinity{
// 		builder.NewPodAffinity(builder.StrengthPrefer, true, antiAffinityLabels).Weight(70),
// 	}}

// 	objectBuilder.Affinity(specAffinity, defaultAffinityBuilder.Build())
// }
