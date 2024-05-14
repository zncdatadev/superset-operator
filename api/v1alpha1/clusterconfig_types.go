package v1alpha1

type ClusterConfigSpec struct {

	// +kubebuilder:validation:Optional
	ListenerClass string `json:"listenerClass,omitempty"`
}
