/*
Copyright 2025 The Crossplane Authors.

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

package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotUser                = "managed resource is not a User custom resource"
	errTrackPCUsage           = "cannot track ProviderConfig usage"
	errGetProviderConfig      = "cannot get referenced ProviderConfig"
	errCreateBackblazeClient  = "cannot create Backblaze client"
	errCreateApplicationKey   = "cannot create application key"
	errDeleteApplicationKey   = "cannot delete application key"
	errGetApplicationKey      = "cannot get application key"
	errWriteSecret            = "cannot write application key secret"
)

// SetupUser adds a controller that reconciles User managed resources.
func SetupUser(mgr ctrl.Manager, o controller.Options) error {
	r := &UserReconciler{
		Client: mgr.GetClient(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("user-controller").
		For(&backblazev1.User{}).
		Watches(&apisv1beta1.ProviderConfig{}, handler.Funcs{}).
		Complete(r)
}

// UserReconciler reconciles a User object
type UserReconciler struct {
	Client client.Client
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *UserReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("user", req.NamespacedName)

	// Fetch the User instance
	user := &backblazev1.User{}
	err := r.Client.Get(ctx, req.NamespacedName, user)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Object not found, return without error
			logger.Info("User resource not found, likely deleted")
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get User")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciling user", "keyName", user.Spec.ForProvider.KeyName)

	// Check for deletion - in this simple implementation, we let Kubernetes handle deletion
	if !user.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, user)
	}

	// Get provider config and create client
	service, err := r.getBackblazeClient(ctx, user)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client")
		r.setCondition(user, xpv1.TypeReady, "False", "ClientError", err.Error())
		// Use shorter requeue time for ProviderConfig not found errors (likely cache sync issue)
		requeueAfter := time.Minute
		if strings.Contains(err.Error(), "not found") {
			requeueAfter = 10 * time.Second
		}
		return reconcile.Result{RequeueAfter: requeueAfter}, r.Client.Status().Update(ctx, user)
	}

	// Check if application key already exists
	if user.Status.AtProvider.ApplicationKeyID == "" {
		// Create application key
		if err := r.createApplicationKey(ctx, user, service); err != nil {
			logger.Error(err, "Failed to create application key")
			r.setCondition(user, xpv1.TypeReady, "False", "CreateError", err.Error())
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, user)
		}
	}

	// Application key exists and is ready
	r.setCondition(user, xpv1.TypeReady, "True", "Available", "Application key is available")
	r.setCondition(user, xpv1.TypeSynced, "True", "ReconcileSuccess", "Successfully reconciled")

	logger.Info("Successfully reconciled user")
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, r.Client.Status().Update(ctx, user)
}

func (r *UserReconciler) handleDeletion(ctx context.Context, user *backblazev1.User) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Delete the associated secret
	if err := r.deleteSecret(ctx, user); err != nil {
		logger.Error(err, "Failed to delete application key secret")
		// Continue with deletion even if secret deletion fails
	}

	// For this implementation, we'll simulate application key deletion
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 application key deletion

	logger.Info("User deletion handled")
	return reconcile.Result{}, nil
}

func (r *UserReconciler) createApplicationKey(ctx context.Context, user *backblazev1.User, service *clients.BackblazeClient) error {
	// For this implementation, we'll simulate application key creation
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 application key creation

	// Generate a simulated application key ID and key
	applicationKeyID := fmt.Sprintf("K005%012d", user.GetGeneration())
	applicationKey := fmt.Sprintf("K005%024d", user.GetGeneration()*1000)

	// Update the resource status
	user.Status.AtProvider.ApplicationKeyID = applicationKeyID
	user.Status.AtProvider.AccountID = "simulated-account-id"
	user.Status.AtProvider.Capabilities = user.Spec.ForProvider.Capabilities
	if user.Spec.ForProvider.BucketID != nil {
		user.Status.AtProvider.BucketID = user.Spec.ForProvider.BucketID
	}
	if user.Spec.ForProvider.NamePrefix != nil {
		user.Status.AtProvider.NamePrefix = user.Spec.ForProvider.NamePrefix
	}
	if user.Spec.ForProvider.ValidDurationInSeconds != nil {
		user.Status.AtProvider.ExpirationTimestamp = user.Spec.ForProvider.ValidDurationInSeconds
	}

	// Create the secret with the application key credentials
	return r.writeSecret(ctx, user, applicationKeyID, applicationKey)
}

func (r *UserReconciler) getBackblazeClient(ctx context.Context, user *backblazev1.User) (*clients.BackblazeClient, error) {
	// Determine ProviderConfig name - use "default" if not specified
	providerConfigName := "default"
	if user.GetProviderConfigReference() != nil {
		providerConfigName = user.GetProviderConfigReference().Name
	}

	pc := &apisv1beta1.ProviderConfig{}
	// ProviderConfigs are namespaced resources - look in the same namespace as the provider
	key := client.ObjectKey{Name: providerConfigName, Namespace: "crossplane-system"}
	if err := r.Client.Get(ctx, key, pc); err != nil {
		// Check if this is a "not found" error that could be due to cache sync timing
		if client.IgnoreNotFound(err) == nil {
			// ProviderConfig not found - this could be a cache sync issue
			// Return a retriable error to allow reconciliation to retry
			return nil, errors.Wrap(err, errGetProviderConfig)
		}
		// Other errors (permission, etc.) - return immediately
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	cfg, err := clients.GetProviderConfig(ctx, r.Client, pc)
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	return clients.NewBackblazeClient(*cfg)
}

// writeSecret creates or updates the secret containing the application key credentials
func (r *UserReconciler) writeSecret(ctx context.Context, user *backblazev1.User, applicationKeyID, applicationKey string) error {
	secretRef := user.Spec.ForProvider.WriteSecretToRef

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"applicationKeyId": []byte(applicationKeyID),
			"applicationKey":   []byte(applicationKey),
		},
	}

	return r.Client.Create(ctx, secret)
}

// deleteSecret removes the secret containing the application key credentials
func (r *UserReconciler) deleteSecret(ctx context.Context, user *backblazev1.User) error {
	secretRef := user.Spec.ForProvider.WriteSecretToRef

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
	}

	return client.IgnoreNotFound(r.Client.Delete(ctx, secret))
}

func (r *UserReconciler) setCondition(user *backblazev1.User, conditionType xpv1.ConditionType, status, reason, message string) {
	user.SetConditions(xpv1.Condition{
		Type:               conditionType,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            message,
	})
}