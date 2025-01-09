/*
Copyright 2024 zncdatadev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/cluster"
)

// SupersetClusterReconciler reconciles a SupersetCluster object
type SupersetClusterReconciler struct {
	k8sClient.Client
	Scheme *runtime.Scheme
}

var (
	logger = ctrl.Log.WithName("common").WithName("reconciler")
)

// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=superset.kubedoop.dev,resources=supersetclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentication.kubedoop.dev,resources=authenticationclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

func (r *SupersetClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger.V(0).Info("Reconciling SupersetCluster")

	instance := &supersetv1alpha1.SupersetCluster{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8sClient.IgnoreNotFound(err) == nil {
			logger.V(1).Info("SupersetCluster resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	resourceClient := &client.Client{
		Client:         r.Client,
		OwnerReference: instance,
	}

	clusterInfo := reconciler.ClusterInfo{
		GVK: &metav1.GroupVersionKind{
			Group:   supersetv1alpha1.GroupVersion.Group,
			Version: supersetv1alpha1.GroupVersion.Version,
			Kind:    "SupersetCluster",
		},
		ClusterName: instance.Name,
	}

	clusterRreconciler := cluster.NewReconciler(resourceClient, clusterInfo, &instance.Spec)

	if err := clusterRreconciler.RegisterResources(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if result, err := clusterRreconciler.Reconcile(ctx); err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	if result, err := clusterRreconciler.Ready(ctx); err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	logger.V(0).Info("Reconcile completed")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SupersetClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&supersetv1alpha1.SupersetCluster{}).
		Complete(r)
}
