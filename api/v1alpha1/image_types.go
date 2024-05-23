package v1alpha1

type ImageSpec struct {
	// +kubebuilder:validation:Optional
	Custom string `json:"custom,omitempty"`
	// +kubebuilder:validation:Optional
	Repo string `json:"repo,omitempty"`
	// +kubebuilder:validation:Optional
	KDSVersion string `json:"kdsVersion,omitempty"`
	// +kubebuilder:validation:Optional
	ProductVersion string `json:"productVersion,omitempty"`
}
