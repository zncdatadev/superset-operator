package common

import supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"

// ClusterConfig represents the configuration of the HBase cluster.
// This is not ClusterConfigSpec wrapper.
type ClusterConfig struct {
	// env secret name for the cluster, which contains the environment variables
	// deployments can use this secret to get the environment variables
	// secret data from ClusterConfigSpec
	EnvSecretName string `json:"envSecretName"`
	// config secret name for the cluster, which contains the configuration files
	// deployments can use this secret to get the configuration files
	// secret data from ClusterConfigSpec
	ConfigSecretName string `json:"configSecretName"`
	Spec             *supersetv1alpha1.ClusterConfigSpec
}
