package node

import corev1 "k8s.io/api/core/v1"

var (
	Ports = []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: 8088,
		},
		{
			Name:          "metrics",
			ContainerPort: 9102, // statsd-exporter metrics port
		},
	}
)
