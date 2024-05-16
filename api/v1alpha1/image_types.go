package v1alpha1

type ImageSpec struct {
	Custom         string `json:"custom,omitempty"`
	Repo           string `json:"repo,omitempty"`
	KDSVersion     string `json:"kdsVersion,omitempty"`
	ProductVersion string `json:"productVersion,omitempty"`
}
