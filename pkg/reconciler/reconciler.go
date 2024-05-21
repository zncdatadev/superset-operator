package reconciler

import (
	"context"

	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AnySpec any

type Reconciler interface {
	GetClient() *resourceClient.ResourceClient
	GetCtrlClient() ctrlclient.Client
	GetCtrlScheme() *runtime.Scheme
	Reconcile(ctx context.Context) Result
	Ready(ctx context.Context) Result
	GetNameWithSuffix(suffix string) string
}

var _ Reconciler = &BaseReconciler[AnySpec]{}

type BaseReconciler[T AnySpec] struct {
	// Do not use ptr, to avoid other packages to modify the client
	Client resourceClient.ResourceClient
	Name   string

	Spec T
}

func (b *BaseReconciler[T]) GetClient() *resourceClient.ResourceClient {
	return &b.Client
}

func (b *BaseReconciler[T]) GetCtrlClient() ctrlclient.Client {
	return b.Client.Client
}

func (b *BaseReconciler[T]) GetName() string {
	return b.Name
}

func (b *BaseReconciler[T]) GetNameWithSuffix(suffix string) string {
	return b.Name + "-" + suffix
}

func (b *BaseReconciler[T]) GetCtrlScheme() *runtime.Scheme {
	return b.Client.Client.Scheme()
}

func (b *BaseReconciler[T]) Ready(ctx context.Context) Result {
	panic("unimplemented")
}

func (b *BaseReconciler[T]) Reconcile(ctx context.Context) Result {
	panic("unimplemented")
}

func (b *BaseReconciler[T]) GetSpec() T {
	return b.Spec
}
