package worker

import corev1 "k8s.io/api/core/v1"

var (
	Ports = []corev1.ContainerPort{
		{
			Name:          "ws",
			ContainerPort: 8080,
		},
	}
)
