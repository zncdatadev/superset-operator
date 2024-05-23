package v1alpha1

type ClusterConfigSpec struct {
	// +kubebuilder:validation:Required
	Database *DatabaseSpec `json:"database"`

	// +kubebuilder:validation:Required
	Redis *RedisSpec `json:"redis"`

	// +kubebuilder:validation:Required
	Administrator *AdministratorSpec `json:"administrator"`

	// +kubebuilder:validation:Optional
	// This is flask app secret key
	AppSecretKey *AppSecretKeySpec `json:"appSecretKey,omitempty"`

	// +kubebuilder:validation:Optional
	ListenerClass string `json:"listenerClass,omitempty"`
}

// AppSecretKeySpec defines the app secret key spec.
type AppSecretKeySpec struct {
	// +kubebuilder:validation=Optional
	// ExistSecret is the name of the secret that contains the secret key.
	// It must contain the key `SUPERSET_SECRET_KEY`.
	// Note: To avoid the key name confusions, the key name must be started with `SUPERSET_`.
	ExistSecret string `json:"existSecret,omitempty"`
	// +kubebuilder:validation=Optional
	// If value is not set, the secret will be generated.
	// When you migrate the Superset instance, you should keep the same secret key in the new instance.
	SecretKey string `json:"secretKey,omitempty"`
}

type AdministratorSpec struct {
	// +kubebuilder:validation=Optional
	// +kubebuilder:default="admin"
	Username string `json:"username,omitempty"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default="Superset"
	FirstName string `json:"firstName,omitempty"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default="Admin"
	LastName string `json:"lastName,omitempty"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default="admin@superset"
	Email string `json:"email,omitempty"`
	// +kubebuilder:validation=Optional
	// +kubebuilder:default="admin"
	Password string `json:"password,omitempty"`
	// +kubebuilder:validation=Optional
	// ExistSecret is the name of the secret that contains the administrator info.
	// It must contain the following keys:
	// - `ADMIN_USERNAME`
	// - `ADMIN_FIRST_NAME`
	// - `ADMIN_LAST_NAME`
	// - `ADMIN_EMAIL`
	// - `ADMIN_PASSWORD`
	ExistSecret string `json:"existSecret,omitempty"`
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
	Reference *string `json:"reference,omitempty"`

	// +kubebuilder:validation=Optional
	Inline *DatabaseInlineSpec `json:"inline,omitempty"`
}

// DatabaseInlineSpec defines the inline database spec.
type DatabaseInlineSpec struct {
	// +kubebuilder:validation:Enum=mysql;postgres
	// +kubebuilder:default="postgres"
	Driver string `json:"driver,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default="superset"
	DatabaseName string `json:"databaseName,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default="superset"
	Username string `json:"username,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default="superset"
	Password string `json:"password,omitempty"`

	// +kubebuilder:validation=Required
	Host string `json:"host,omitempty"`

	// +kubebuilder:validation=Optional
	// +kubebuilder:default=5432
	Port int32 `json:"port,omitempty"`
}
