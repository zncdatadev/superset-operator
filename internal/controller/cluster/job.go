package cluster

import (
	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/pkg/image"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobReconciler struct {
	reconciler.BaseResourceReconciler[*supersetv1alpha1.SupersetClusterSpec]
	Image image.Image
}

func (r *JobReconciler) Build() (*batchv1.Job, error) {
	obj := &batchv1.Job{
		ObjectMeta: r.GetObjectMeta(),
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.Client.GetLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  r.GetName(),
							Image: r.Image.Custom,
						},
					},
				},
			},
		},
	}
	return obj, nil
}
