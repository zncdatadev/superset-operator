package v1alpha1

type ClusterConfigSpec struct {
	// +kubebuilder:validation:Required
	Database *DatabaseSpec `json:"database"`

	// +kubebuilder:validation:Required
	Redis *RedisSpec `json:"redis"`

	// +kubebuilder:validation:Optional
	ListenerClass string `json:"listenerClass,omitempty"`
}

// RedisSpec defines the redis spec.
type RedisSpec struct {
	// +kubebuilder:validation=Optional
	// ExistSecret is the name of the secret that contains the Redis password.
	// If this field is set, the Redis `password` will be read from the secret,
	// else the password will be read from the Password field.
	ExistSecret string `json:"existSecret,omitempty"`
	// +kubebuilder:validation=Required
	Host string `json:"host"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default=6379
	Port int32 `json:"port,omitempty"`
	// +kubebuilder:validation=Optional
	User string `json:"user,omitempty"`
	// +kubebuilder:validation=Optional
	Password string `json:"password,omitempty"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default="redis"
	Proto string `json:"proto,omitempty"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default=0
	DB int32 `json:"db,omitempty"`
}

type DatabaseSpec struct {
	// +kubebuilder:validation=Optional
	Reference string `json:"reference"`

	// +kubebuilder:validation=Optional
	Inline *DatabaseInlineSpec `json:"inline,omitempty"`
}

// DatabaseInlineSpec defines the inline database spec.
type DatabaseInlineSpec struct {
	// +kubebuilder:validation:Enum=mysql;postgres
	// +kubebuilder:default="postgres"
	Driver string `json:"driver,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default="hive"
	DatabaseName string `json:"databaseName,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default="hive"
	Username string `json:"username,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default="hive"
	Password string `json:"password,omitempty"`

	// +kubebuilder:validation=Required
	Host string `json:"host,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default=5432
	Port int32 `json:"port,omitempty"`
}
