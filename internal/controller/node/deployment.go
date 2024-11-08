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
	ports []corev1.ContainerPort,
	image *util.Image,
	stopped bool,
	spec *supersetv1alpha1.NodeRoleGroupSpec,
) (*reconciler.Deployment, error) {

	options := builder.WorkloadOptions{
		Option: builder.Option{
			ClusterName:   roleGroupInfo.ClusterName,
			RoleName:      roleGroupInfo.RoleName,
			RoleGroupName: roleGroupInfo.RoleGroupName,
			Labels:        roleGroupInfo.GetLabels(),
			Annotations:   roleGroupInfo.GetAnnotations(),
		},
		PodOverrides: spec.PodOverride,
		EnvOverrides: spec.EnvOverrides,
		CliOverrides: spec.CliOverrides,
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
		options.Resource = spec.Config.Resources
	}

	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		roleGroupInfo.GetFullName(),
		clusterConfig,
		spec.Replicas,
		ports,
		image,
		options,
	)

	return reconciler.NewDeployment(
		client,
		roleGroupInfo.GetFullName(),
		deploymentBuilder,
		stopped,
	), nil
}
