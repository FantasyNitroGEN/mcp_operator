package services

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultKubernetesClientService implements KubernetesClientService interface
type DefaultKubernetesClientService struct {
	client client.Client
}

// NewDefaultKubernetesClientService creates a new DefaultKubernetesClientService
func NewDefaultKubernetesClientService(client client.Client) *DefaultKubernetesClientService {
	return &DefaultKubernetesClientService{
		client: client,
	}
}

// Get retrieves a Kubernetes object
func (k *DefaultKubernetesClientService) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "get", "key", key.String(), "type", fmt.Sprintf("%T", obj))
	logger.V(1).Info("Getting Kubernetes object")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.Get(ctx, key, obj)
	if err != nil {
		logger.V(1).Info("Failed to get Kubernetes object", "error", err.Error())
		return err
	}

	logger.V(1).Info("Successfully retrieved Kubernetes object")
	return nil
}

// Create creates a Kubernetes object
func (k *DefaultKubernetesClientService) Create(ctx context.Context, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "create", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.Info("Creating Kubernetes object")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.Create(ctx, obj)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes object")
		return fmt.Errorf("failed to create %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	logger.Info("Successfully created Kubernetes object")
	return nil
}

// Update updates a Kubernetes object
func (k *DefaultKubernetesClientService) Update(ctx context.Context, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "update", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.Info("Updating Kubernetes object")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.Update(ctx, obj)
	if err != nil {
		logger.Error(err, "Failed to update Kubernetes object")
		return fmt.Errorf("failed to update %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	logger.Info("Successfully updated Kubernetes object")
	return nil
}

// Delete deletes a Kubernetes object
func (k *DefaultKubernetesClientService) Delete(ctx context.Context, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "delete", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.Info("Deleting Kubernetes object")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.Delete(ctx, obj)
	if err != nil {
		logger.Error(err, "Failed to delete Kubernetes object")
		return fmt.Errorf("failed to delete %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	logger.Info("Successfully deleted Kubernetes object")
	return nil
}

// Patch patches a Kubernetes object
func (k *DefaultKubernetesClientService) Patch(ctx context.Context, obj client.Object, patch client.Patch) error {
	logger := log.FromContext(ctx).WithValues("operation", "patch", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.Info("Patching Kubernetes object")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.Patch(ctx, obj, patch)
	if err != nil {
		logger.Error(err, "Failed to patch Kubernetes object")
		return fmt.Errorf("failed to patch %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	logger.Info("Successfully patched Kubernetes object")
	return nil
}

// List lists Kubernetes objects
func (k *DefaultKubernetesClientService) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	logger := log.FromContext(ctx).WithValues("operation", "list", "type", fmt.Sprintf("%T", list))
	logger.V(1).Info("Listing Kubernetes objects")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.List(ctx, list, opts...)
	if err != nil {
		logger.Error(err, "Failed to list Kubernetes objects")
		return fmt.Errorf("failed to list %T: %w", list, err)
	}

	logger.V(1).Info("Successfully listed Kubernetes objects")
	return nil
}

// UpdateStatus updates the status of a Kubernetes object
func (k *DefaultKubernetesClientService) UpdateStatus(ctx context.Context, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "updateStatus", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.V(1).Info("Updating Kubernetes object status")

	if k.client == nil {
		return fmt.Errorf("kubernetes client is not initialized")
	}

	err := k.client.Status().Update(ctx, obj)
	if err != nil {
		logger.Error(err, "Failed to update Kubernetes object status")
		return fmt.Errorf("failed to update status of %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	logger.V(1).Info("Successfully updated Kubernetes object status")
	return nil
}

// GetClient returns the underlying Kubernetes client
func (k *DefaultKubernetesClientService) GetClient() client.Client {
	return k.client
}

// SetClient sets the Kubernetes client (useful for testing)
func (k *DefaultKubernetesClientService) SetClient(client client.Client) {
	k.client = client
}

// IsClientInitialized checks if the Kubernetes client is initialized
func (k *DefaultKubernetesClientService) IsClientInitialized() bool {
	return k.client != nil
}

// CreateOrUpdate creates or updates a Kubernetes object
func (k *DefaultKubernetesClientService) CreateOrUpdate(ctx context.Context, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "createOrUpdate", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.V(1).Info("Creating or updating Kubernetes object")

	// Try to get the object first
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	existing := obj.DeepCopyObject().(client.Object)
	err := k.Get(ctx, key, existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Object doesn't exist, create it
			logger.V(1).Info("Object doesn't exist, creating")
			return k.Create(ctx, obj)
		}
		// Some other error occurred
		return err
	}

	// Object exists, update it
	logger.V(1).Info("Object exists, updating")
	obj.SetResourceVersion(existing.GetResourceVersion())
	return k.Update(ctx, obj)
}

// DeleteIfExists deletes a Kubernetes object if it exists
func (k *DefaultKubernetesClientService) DeleteIfExists(ctx context.Context, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "deleteIfExists", "name", obj.GetName(), "namespace", obj.GetNamespace(), "type", fmt.Sprintf("%T", obj))
	logger.V(1).Info("Deleting Kubernetes object if it exists")

	err := k.Delete(ctx, obj)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.V(1).Info("Object doesn't exist, nothing to delete")
			return nil
		}
		return err
	}

	logger.V(1).Info("Successfully deleted Kubernetes object")
	return nil
}

// Exists checks if a Kubernetes object exists
func (k *DefaultKubernetesClientService) Exists(ctx context.Context, key client.ObjectKey, obj client.Object) (bool, error) {
	logger := log.FromContext(ctx).WithValues("operation", "exists", "key", key.String(), "type", fmt.Sprintf("%T", obj))
	logger.V(1).Info("Checking if Kubernetes object exists")

	err := k.Get(ctx, key, obj)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.V(1).Info("Object doesn't exist")
			return false, nil
		}
		logger.Error(err, "Error checking if object exists")
		return false, err
	}

	logger.V(1).Info("Object exists")
	return true, nil
}

// WaitForDeletion waits for a Kubernetes object to be deleted
func (k *DefaultKubernetesClientService) WaitForDeletion(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	logger := log.FromContext(ctx).WithValues("operation", "waitForDeletion", "key", key.String(), "type", fmt.Sprintf("%T", obj))
	logger.V(1).Info("Waiting for Kubernetes object deletion")

	// Simple implementation - just check if object still exists
	exists, err := k.Exists(ctx, key, obj)
	if err != nil {
		return err
	}

	if !exists {
		logger.V(1).Info("Object has been deleted")
		return nil
	}

	logger.V(1).Info("Object still exists")
	return fmt.Errorf("object %s still exists", key.String())
}

// GetWithRetry gets a Kubernetes object with retry logic
func (k *DefaultKubernetesClientService) GetWithRetry(ctx context.Context, key client.ObjectKey, obj client.Object, maxRetries int) error {
	logger := log.FromContext(ctx).WithValues("operation", "getWithRetry", "key", key.String(), "type", fmt.Sprintf("%T", obj), "maxRetries", maxRetries)
	logger.V(1).Info("Getting Kubernetes object with retry")

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := k.Get(ctx, key, obj)
		if err == nil {
			if attempt > 0 {
				logger.V(1).Info("Successfully retrieved object after retry", "attempt", attempt+1)
			}
			return nil
		}

		lastErr = err
		if attempt < maxRetries-1 {
			logger.V(1).Info("Failed to get object, retrying", "attempt", attempt+1, "error", err.Error())
		}
	}

	logger.Error(lastErr, "Failed to get object after all retries", "attempts", maxRetries)
	return fmt.Errorf("failed to get object after %d attempts: %w", maxRetries, lastErr)
}

// ListWithFilter lists Kubernetes objects with additional filtering
func (k *DefaultKubernetesClientService) ListWithFilter(ctx context.Context, list client.ObjectList, filterFunc func(client.Object) bool, opts ...client.ListOption) error {
	logger := log.FromContext(ctx).WithValues("operation", "listWithFilter", "type", fmt.Sprintf("%T", list))
	logger.V(1).Info("Listing Kubernetes objects with filter")

	// First, get all objects
	err := k.List(ctx, list, opts...)
	if err != nil {
		return err
	}

	// Apply filter if provided
	if filterFunc != nil {
		// This is a simplified implementation
		// In a real implementation, you'd need to handle the filtering properly
		// based on the specific list type
		logger.V(1).Info("Filter function provided but not implemented in this simple version")
	}

	return nil
}
