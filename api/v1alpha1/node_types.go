package v1alpha1

import (
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

// RoleNameNode is the single role of a SupersetCluster, serving both the web UI
// and the API. It names the generic Roles map key, the framework resource names
// (<cluster>-node-<group>) and the main pod container.
const RoleNameNode = "node"

type NodeSpec struct {
	RoleGroups                     map[string]NodeRoleGroupSpec    `json:"roleGroups,omitempty"`
	Config                         *NodeConfigSpec                 `json:"config,omitempty"`
	RoleConfig                     *commonsv1alpha1.RoleConfigSpec `json:"roleConfig,omitempty"`
	*commonsv1alpha1.OverridesSpec `json:",inline"`
}

type NodeConfigSpec struct {
	*commonsv1alpha1.RoleGroupConfigSpec `json:",inline"`
}

type NodeRoleGroupSpec struct {
	Replicas                       *int32          `json:"replicas,omitempty"`
	Config                         *NodeConfigSpec `json:"config,omitempty"`
	*commonsv1alpha1.OverridesSpec `json:",inline"`
}
