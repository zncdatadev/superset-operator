package client

import (
	"context"
	"fmt"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	clientLogger = ctrl.Log.WithName("resourceClient")
)

type Client struct {
	Client ctrlclient.Client

	OwnerReference ctrlclient.Object
}

func (c *Client) GetCtrlClient() ctrlclient.Client {
	return c.Client
}

func (c *Client) GetCtrlScheme() *runtime.Scheme {
	return c.Client.Scheme()
}

func (c *Client) GetOwnerReference() ctrlclient.Object {
	return c.OwnerReference
}
func (c *Client) GetOwnerNamespace() string {
	return c.OwnerReference.GetNamespace()
}

func (c *Client) GetOwnerName() string {
	return c.OwnerReference.GetName()
}

// Get the object from the cluster
// If the object has no namespace, it will use the owner namespace
func (c *Client) Get(ctx context.Context, obj ctrlclient.Object) error {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = c.GetOwnerNamespace()
		clientLogger.V(5).Info(""+
			"ResourceClient.Get accept obj without namespace, try to use owner namespace",
			"namespace", namespace,
			"name", name,
		)
	}
	kind := obj.GetObjectKind()
	if err := c.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		opt := []any{"ns", namespace, "name", name, "kind", kind}
		if apierrors.IsNotFound(err) {
			clientLogger.V(0).Info("Fetch resource NotFound", opt...)
		} else {
			clientLogger.Error(err, "Fetch resource occur some unknown err", opt...)
		}
		return err
	}
	return nil
}

func (c *Client) CreateOrUpdate(ctx context.Context, obj ctrlclient.Object) (mutation bool, err error) {

	key := ctrlclient.ObjectKeyFromObject(obj)
	namespace := obj.GetNamespace()
	kinds, _, _ := c.Client.Scheme().ObjectKinds(obj)
	name := obj.GetName()

	clientLogger.V(5).Info("Creating or updating object", "Kind", kinds, "Namespace", namespace, "Name", name)

	current := obj.DeepCopyObject().(ctrlclient.Object)
	// Check if the object exists, if not create a new one
	err = c.Client.Get(ctx, key, current)
	var calculateOpt = []patch.CalculateOption{
		patch.IgnoreStatusFields(),
	}
	if errors.IsNotFound(err) {
		if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(obj); err != nil {
			return false, err
		}
		clientLogger.Info("Creating a new object", "Kind", kinds, "Namespace", namespace, "Name", name)

		if err := c.Client.Create(ctx, obj); err != nil {
			return false, err
		}
		return true, nil
	} else if err == nil {
		switch obj.(type) {
		case *corev1.Service:
			currentSvc := current.(*corev1.Service)
			svc := obj.(*corev1.Service)
			// Preserve the ClusterIP when updating the service
			svc.Spec.ClusterIP = currentSvc.Spec.ClusterIP
			// Preserve the annotation when updating the service, ensure any updated annotation is preserved
			//for key, value := range currentSvc.Annotations {
			//	if _, present := svc.Annotations[key]; !present {
			//		svc.Annotations[key] = value
			//	}
			//}

			if svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				for i := range svc.Spec.Ports {
					svc.Spec.Ports[i].NodePort = currentSvc.Spec.Ports[i].NodePort
				}
			}
		case *appsv1.StatefulSet:
			calculateOpt = append(calculateOpt, patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus())
		}
		result, err := patch.DefaultPatchMaker.Calculate(current, obj, calculateOpt...)
		if err != nil {
			clientLogger.Error(err, "failed to calculate patch to match objects, moving on to update")
			// if there is an error with matching, we still want to update
			resourceVersion := current.(metav1.ObjectMetaAccessor).GetObjectMeta().GetResourceVersion()
			obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetResourceVersion(resourceVersion)

			if err := c.Client.Update(ctx, obj); err != nil {
				return false, err
			}
			return true, nil
		}

		if !result.IsEmpty() {
			clientLogger.Info(
				fmt.Sprintf("Resource update for object %s:%s", kinds, obj.(metav1.ObjectMetaAccessor).GetObjectMeta().GetName()),
				"patch", string(result.Patch),
			)

			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(obj); err != nil {
				clientLogger.Error(err, "failed to annotate modified object", "object", obj)
			}

			resourceVersion := current.(metav1.ObjectMetaAccessor).GetObjectMeta().GetResourceVersion()
			obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetResourceVersion(resourceVersion)

			if err = c.Client.Update(ctx, obj); err != nil {
				return false, err
			}
			return true, nil
		}
		clientLogger.V(1).Info(fmt.Sprintf("Skipping update for object %s:%s", kinds, obj.(metav1.ObjectMetaAccessor).GetObjectMeta().GetName()))

	}
	return false, err
}
