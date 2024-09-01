package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	DefaultRepository      = "quay.io/zncdatadev"
	DefaultProductVersion  = "4.0.2"
	DefaultPlatformVersion = "0.0.0-dev"
	DefaultProductName     = "superset"
)

type ImageSpec struct {
	// +kubebuilder:validation:Optional
	Custom string `json:"custom,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=quay.io/zncdatadev
	Repo string `json:"repo,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="0.0.0-dev"
	PlatformVersion string `json:"platformVersion,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="4.0.2"
	ProductVersion string `json:"productVersion,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=IfNotPresent
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// +kubebuilder:validation:Optional
	PullSecretName string `json:"pullSecretName,omitempty"`
}
