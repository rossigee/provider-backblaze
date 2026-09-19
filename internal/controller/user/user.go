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
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotUser               = "managed resource is not a User custom resource"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errGetProviderConfig     = "cannot get referenced ProviderConfig"
	errCreateBackblazeClient = "cannot create Backblaze client"
	errCreateApplicationKey  = "cannot create application key"
	errDeleteApplicationKey  = "cannot delete application key"
	errGetApplicationKey     = "cannot get application key"
	errWriteSecret           = "cannot write application key secret"
	errReadSecret            = "cannot read application key secret"
)

// SetupUser adds a controller that reconciles User managed resources.
func SetupUser(mgr ctrl.Manager, o controller.Options) error {
	r := &UserReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorder("user-controller"),
	}

	// Explicitly wait for ProviderConfig informer to sync before starting reconciliation
	// This prevents the cache-sync race where Watches() with empty handler.Funcs{} doesn't
	// block until HasSynced, causing immediate Get(ProviderConfig) calls to fail with NotFound
	cache := mgr.GetCache()
	if _, err := cache.GetInformer(context.Background(), &apisv1beta1.ProviderConfig{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("user-controller").
		For(&backblazev1beta1.User{}).
		Watches(&apisv1beta1.ProviderConfig{}, handler.Funcs{}).
		Complete(r)
}

// UserReconciler reconciles a User object
type UserReconciler struct {
	Client   client.Client
	Recorder events.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *UserReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("user", req.NamespacedName)

	// Fetch the User instance
	user := &backblazev1beta1.User{}
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
		if r.Recorder != nil {
			r.Recorder.Eventf(user, nil, corev1.EventTypeWarning, "ClientError", "GetBackblazeClient", err.Error())
		}
		return reconcile.Result{RequeueAfter: requeueAfter}, r.Client.Status().Update(ctx, user)
	}

	// Determine whether the application key exists in B2.
	//
	// - Empty status.ApplicationKeyID => never created, take the create path.
	// - Non-empty + key not found in B2 => external drift, recreate.
	// - Non-empty + key found => existing key, drift-check spec fields and
	//   ensure the connection secret is still in place.
	var observed *clients.B2CreateKeyResponse
	keyExists := false
	if user.Status.AtProvider.ApplicationKeyID != "" {
		k, getErr := service.GetApplicationKey(ctx, user.Status.AtProvider.ApplicationKeyID)
		switch {
		case getErr == nil:
			observed = k
			keyExists = true
		case errors.Is(getErr, clients.ErrApplicationKeyNotFound):
			logger.Info("Backblaze key missing out-of-band; will recreate",
				"keyID", user.Status.AtProvider.ApplicationKeyID)
			r.clearKeyStatus(user)
		default:
			logger.Error(getErr, "Failed to observe application key")
			r.setCondition(user, xpv1.TypeReady, "False", "ObserveError", getErr.Error())
			if r.Recorder != nil {
				r.Recorder.Eventf(user, nil, corev1.EventTypeWarning, "ObserveError", "GetApplicationKey", getErr.Error())
			}
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, user)
		}
	}

	if !keyExists {
		if !shouldCreate(user.GetManagementPolicies()) {
			logger.Info("Skipping application key creation due to managementPolicies", "managementPolicies", user.GetManagementPolicies())
			r.setCondition(user, xpv1.TypeReady, "False", "ObserveOnly", "External resource does not exist and creation is disabled by managementPolicies")
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, user)
		}
		if err := r.createApplicationKey(ctx, user, service); err != nil {
			logger.Error(err, "Failed to create application key")
			r.setCondition(user, xpv1.TypeReady, "False", "CreateError", err.Error())
			if r.Recorder != nil {
				r.Recorder.Eventf(user, nil, corev1.EventTypeWarning, "CreateError", "CreateApplicationKey", err.Error())
			}
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, user)
		}
	} else {
		// Live key exists; reconcile spec drift via b2_update_key.
		if err := r.applyKeyUpdate(ctx, user, service, observed); err != nil {
			logger.Error(err, "Failed to apply key update")
			r.setCondition(user, xpv1.TypeReady, "False", "UpdateError", err.Error())
			if r.Recorder != nil {
				r.Recorder.Eventf(user, nil, corev1.EventTypeWarning, "UpdateError", "UpdateApplicationKey", err.Error())
			}
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, user)
		}
		if secretRef := user.GetWriteConnectionSecretToReference(); secretRef != nil && secretRef.Name != "" {
			// Key still exists in B2; make sure the local connection secret is
			// still present. Without this, a user who deletes the secret by
			// hand would never get it back without recreating the MR.
			if err := r.ensureConnectionSecret(ctx, user); err != nil {
				logger.Error(err, "Failed to reconcile connection secret")
				r.setCondition(user, xpv1.TypeReady, "False", "SecretError", err.Error())
				if r.Recorder != nil {
					r.Recorder.Eventf(user, nil, corev1.EventTypeWarning, "SecretError", "EnsureConnectionSecret", err.Error())
				}
				return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, user)
			}
		}
	}

	r.setCondition(user, xpv1.TypeReady, "True", "Available", "Application key is available")
	r.setCondition(user, xpv1.TypeSynced, "True", "ReconcileSuccess", "Successfully reconciled")

	logger.Info("Successfully reconciled user")
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, r.Client.Status().Update(ctx, user)
}

// clearKeyStatus resets any status fields that reflect B2 state so that the
// next reconcile pass treats the resource as if it has never been created.
func (r *UserReconciler) clearKeyStatus(user *backblazev1beta1.User) {
	user.Status.AtProvider.ApplicationKeyID = ""
	user.Status.AtProvider.AccountID = ""
	user.Status.AtProvider.Capabilities = nil
	user.Status.AtProvider.BucketID = nil
	user.Status.AtProvider.NamePrefix = nil
	user.Status.AtProvider.ExpirationTimestamp = nil
}

func (r *UserReconciler) handleDeletion(ctx context.Context, user *backblazev1beta1.User) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Respect managementPolicies - if Observe only, don't delete external resource
	if !shouldDelete(user.GetManagementPolicies()) {
		logger.Info("Skipping application key deletion due to managementPolicies", "managementPolicies", user.GetManagementPolicies())
		// Still delete the secret reference? No - skip external deletion but allow K8s deletion to proceed.
		// Do not delete external key, just return.
		logger.Info("User deletion handled (Observe-only, external resource preserved)")
		return reconcile.Result{}, nil
	}

	// Delete the application key from B2 if it exists
	if user.Status.AtProvider.ApplicationKeyID != "" {
		service, err := r.getBackblazeClient(ctx, user)
		if err != nil {
			logger.Error(err, "Failed to create Backblaze client for deletion")
		} else {
			if err := service.DeleteApplicationKey(ctx, user.Status.AtProvider.ApplicationKeyID); err != nil {
				if errors.Is(err, clients.ErrApplicationKeyNotFound) {
					logger.Info("Application key already absent in B2",
						"keyID", user.Status.AtProvider.ApplicationKeyID)
				} else if strings.Contains(err.Error(), "404") {
					logger.Info("Application key already absent in B2 (404)",
						"keyID", user.Status.AtProvider.ApplicationKeyID)
				} else {
					logger.Error(err, "Failed to delete application key from B2")
					if r.Recorder != nil {
						r.Recorder.Eventf(user, nil, corev1.EventTypeWarning, "DeleteError", "DeleteApplicationKey", err.Error())
					}
				}
			} else {
				logger.Info("Successfully deleted application key from B2", "keyID", user.Status.AtProvider.ApplicationKeyID)
			}
		}
	}

	// Delete the associated secret. Errors here are logged and not surfaced as
	// reconcile failures - the secret may already be gone, and we don't want to
	// block Kubernetes GC of the User CR.
	if err := r.deleteSecret(ctx, user); err != nil {
		logger.Error(err, "Failed to delete application key secret")
	}

	logger.Info("User deletion handled")
	return reconcile.Result{}, nil
}

func (r *UserReconciler) createApplicationKey(ctx context.Context, user *backblazev1beta1.User, service *clients.BackblazeClient) error {
	// Call real B2 API to create application key
	var validDuration *int
	if user.Spec.ForProvider.ValidDurationInSeconds != nil {
		v := int(*user.Spec.ForProvider.ValidDurationInSeconds)
		validDuration = &v
	}

	var bucketID, namePrefix string
	if user.Spec.ForProvider.BucketID != nil {
		bucketID = *user.Spec.ForProvider.BucketID
	}
	if user.Spec.ForProvider.NamePrefix != nil {
		namePrefix = *user.Spec.ForProvider.NamePrefix
	}

	keyResp, err := service.CreateApplicationKey(
		ctx,
		user.Spec.ForProvider.KeyName,
		user.Spec.ForProvider.Capabilities,
		bucketID,
		namePrefix,
		validDuration,
	)
	if err != nil {
		return errors.Wrap(err, errCreateApplicationKey)
	}

	// Update the resource status with real values from B2
	user.Status.AtProvider.ApplicationKeyID = keyResp.ApplicationKeyID
	user.Status.AtProvider.AccountID = keyResp.AccountID
	user.Status.AtProvider.Capabilities = user.Spec.ForProvider.Capabilities
	user.Status.AtProvider.BucketID = user.Spec.ForProvider.BucketID
	user.Status.AtProvider.NamePrefix = user.Spec.ForProvider.NamePrefix
	if user.Spec.ForProvider.ValidDurationInSeconds != nil {
		user.Status.AtProvider.ExpirationTimestamp = user.Spec.ForProvider.ValidDurationInSeconds
	}

	// Write (create or update) the connection secret with the real application
	// key credentials.
	return r.writeSecret(ctx, user, keyResp.ApplicationKeyID, keyResp.ApplicationKey)
}

// applyKeyUpdate reconciles live B2 application keys against the spec via
// b2_update_key. The application's secret value isn't affected by B2's
// update, so the connection secret in the cluster remains valid throughout.
func (r *UserReconciler) applyKeyUpdate(ctx context.Context, user *backblazev1beta1.User, service *clients.BackblazeClient, observed *clients.B2CreateKeyResponse) error {
	desiredBucket := ""
	if user.Spec.ForProvider.BucketID != nil {
		desiredBucket = *user.Spec.ForProvider.BucketID
	}
	desiredPrefix := ""
	if user.Spec.ForProvider.NamePrefix != nil {
		desiredPrefix = *user.Spec.ForProvider.NamePrefix
	}
	desiredValidSec := -1 // sentinel: "leave alone"
	if user.Spec.ForProvider.ValidDurationInSeconds != nil {
		desiredValidSec = int(*user.Spec.ForProvider.ValidDurationInSeconds)
	}

	if observed != nil {
		sameCapabilities := stringSlicesEqualSet(observed.Capabilities, user.Spec.ForProvider.Capabilities)
		observedBucket := ""
		if observed.BucketID != "" {
			observedBucket = observed.BucketID
		}
		observedPrefix := ""
		if observed.NamePrefix != "" {
			observedPrefix = observed.NamePrefix
		}
		var observedValidSec int64
		if observed.ExpirationTimestamp != nil {
			// ExpirationTimestamp is ms-since-epoch; not directly comparable to
			// validDurationInSeconds. Treat unchanged-by-spec as matched.
			observedValidSec = -1
			_ = observedValidSec
		}
		// We can't precisely diff expiration without a reliable timestamp
		// comparison, so always send valid duration if specified - B2 returns
		// error on no-op writes only when nothing changed.
		if sameCapabilities && observedBucket == desiredBucket &&
			observedPrefix == desiredPrefix &&
			desiredValidSec == -1 &&
			observed.KeyName == user.Spec.ForProvider.KeyName {
			return nil
		}
	}

	upd := clients.B2UpdateKeyRequest{
		ApplicationKeyID: user.Status.AtProvider.ApplicationKeyID,
		KeyName:          user.Spec.ForProvider.KeyName,
		Capabilities:     user.Spec.ForProvider.Capabilities,
		BucketID:         desiredBucket,
		NamePrefix:       desiredPrefix,
	}
	if desiredValidSec >= 0 {
		upd.ValidDurationInSeconds = &desiredValidSec
	}

	resp, err := service.UpdateApplicationKey(ctx, upd)
	if err != nil {
		return err
	}
	// Status reflects the post-update server view.
	user.Status.AtProvider.Capabilities = resp.Capabilities
	if resp.BucketID != "" {
		b := resp.BucketID
		user.Status.AtProvider.BucketID = &b
	} else {
		user.Status.AtProvider.BucketID = nil
	}
	if resp.NamePrefix != "" {
		p := resp.NamePrefix
		user.Status.AtProvider.NamePrefix = &p
	} else {
		user.Status.AtProvider.NamePrefix = nil
	}
	if resp.ExpirationTimestamp != nil {
		exp := *resp.ExpirationTimestamp
		user.Status.AtProvider.ExpirationTimestamp = &exp
	}
	return nil
}

func stringSlicesEqualSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

func (r *UserReconciler) getBackblazeClient(ctx context.Context, user *backblazev1beta1.User) (*clients.BackblazeClient, error) {
	// Crossplane v2 sets a kubebuilder default of {"kind":"ClusterProviderConfig","name":"default"}.
	// We always honour the referenced name; if it's empty we treat it as "default".
	providerConfigName := "default"
	if ref := user.GetProviderConfigReference(); ref != nil && ref.Name != "" {
		providerConfigName = ref.Name
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

// connectionSecretKeyID and connectionSecretKey are the data keys written into
// the connection secret.
const (
	connectionSecretKeyID = "applicationKeyId"
	connectionSecretKey   = "applicationKey"
)

// writeSecret idempotently creates or updates the connection secret. It sets
// a controller owner reference back to the User so `kubectl delete user` will
// garbage-collect the secret.
func (r *UserReconciler) writeSecret(ctx context.Context, user *backblazev1beta1.User, applicationKeyID, applicationKey string) error {
	secretRef := user.GetWriteConnectionSecretToReference()
	if secretRef == nil || secretRef.Name == "" {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: user.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Ensure the secret is owned by the User so deletion of the User CR
		// cascade-deletes the secret.
		if err := controllerutil.SetControllerReference(user, secret, r.Client.Scheme()); err != nil {
			// SetControllerReference only fails on cluster/namespace scope
			// mismatch, which isn't possible here (User and Secret are both
			// namespace-scoped and in the same namespace). Treat as fatal.
			return errors.Wrap(err, errWriteSecret)
		}
		secret.Type = corev1.SecretTypeOpaque
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[connectionSecretKeyID] = []byte(applicationKeyID)
		secret.Data[connectionSecretKey] = []byte(applicationKey)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, errWriteSecret)
	}
	return nil
}

// ensureConnectionSecret makes sure the connection secret exists when the key
// is observed as already-present in B2 but the local secret may be missing.
func (r *UserReconciler) ensureConnectionSecret(ctx context.Context, user *backblazev1beta1.User) error {
	secretRef := user.GetWriteConnectionSecretToReference()
	if secretRef == nil || secretRef.Name == "" {
		return nil
	}

	existing := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: secretRef.Name, Namespace: user.Namespace}, existing)
	if err == nil {
		// Secret already exists - contents are immutable from our side because
		// B2 never returns the secret value on subsequent reads.
		return nil
	}
	if client.IgnoreNotFound(err) == nil {
		// Secret genuinely missing - we have no way to recover the secret value
		// from B2 (only key ID is returned on read). Surface a clear error so the
		// operator knows to delete and recreate the User to rotate credentials.
		return errors.New("connection secret is missing but B2 application key already exists; delete the User and recreate it to regenerate credentials")
	}
	return errors.Wrap(err, errReadSecret)
}

// deleteSecret removes the secret containing the application key credentials.
// Missing secrets are not an error.
func (r *UserReconciler) deleteSecret(ctx context.Context, user *backblazev1beta1.User) error {
	secretRef := user.GetWriteConnectionSecretToReference()
	if secretRef == nil || secretRef.Name == "" {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: user.Namespace,
		},
	}

	return client.IgnoreNotFound(r.Client.Delete(ctx, secret))
}

func (r *UserReconciler) setCondition(user *backblazev1beta1.User, conditionType xpv1.ConditionType, status, reason, message string) {
	user.SetConditions(xpv1.Condition{
		Type:               conditionType,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            message,
	})
}

// shouldCreate returns true if managementPolicies allow creation.
func shouldCreate(mp xpv1.ManagementPolicies) bool {
	if len(mp) == 0 {
		return true
	}
	for _, p := range mp {
		if p == xpv1.ManagementActionCreate || p == xpv1.ManagementActionAll {
			return true
		}
	}
	return false
}

// shouldDelete returns true if managementPolicies allow deletion.
func shouldDelete(mp xpv1.ManagementPolicies) bool {
	if len(mp) == 0 {
		return true
	}
	for _, p := range mp {
		if p == xpv1.ManagementActionDelete || p == xpv1.ManagementActionAll {
			return true
		}
	}
	return false
}
