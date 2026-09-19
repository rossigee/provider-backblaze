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

package bucket

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotBucket       = "managed resource is not a Bucket custom resource"
	errTrackPCUsage    = "cannot track ProviderConfig usage"
	errGetPC           = "cannot get ProviderConfig"
	errGetCreds        = "cannot get credentials"
	errNewClient       = "cannot create new Service"
	errCreateBucket    = "cannot create bucket"
	errDeleteBucket    = "cannot delete bucket"
	errObserveBucket   = "cannot observe bucket"
	errApplyBucketType = "cannot apply bucket type"
	errApplyLifecycle  = "cannot apply bucket lifecycle configuration"
	errApplyCors       = "cannot apply bucket CORS configuration"
	errEmptyBucket     = "cannot empty bucket before deletion"
)

// SetupBucket adds a controller that reconciles Bucket managed resources.
func SetupBucket(mgr ctrl.Manager, o controller.Options) error {
	r := &BucketReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorder("bucket-controller"),
	}

	// Explicitly wait for ProviderConfig informer to sync before starting reconciliation
	// This prevents the cache-sync race where Watches() with empty handler.Funcs{} doesn't
	// block until HasSynced, causing immediate Get(ProviderConfig) calls to fail with NotFound
	cache := mgr.GetCache()
	if _, err := cache.GetInformer(context.Background(), &apisv1beta1.ProviderConfig{}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("bucket-controller").
		For(&backblazev1beta1.Bucket{}).
		Watches(&apisv1beta1.ProviderConfig{}, handler.Funcs{}).
		Complete(r)
}

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	Client   client.Client
	Recorder events.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BucketReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("bucket", req.NamespacedName)

	// Fetch the Bucket instance
	bucket := &backblazev1beta1.Bucket{}
	err := r.Client.Get(ctx, req.NamespacedName, bucket)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Object not found, return without error
			logger.Info("Bucket resource not found, likely deleted")
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get Bucket")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciling bucket", "bucketName", bucket.Spec.ForProvider.BucketName)

	// Handle deletion - respect managementPolicies
	if !bucket.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, bucket)
	}

	// Get provider config and create client
	service, err := r.getBackblazeClient(ctx, bucket)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client")
		r.setCondition(bucket, xpv1.TypeReady, "False", "ClientError", err.Error())
		requeueAfter := time.Minute
		if strings.Contains(err.Error(), "not found") {
			requeueAfter = 10 * time.Second
			logger.Info("ProviderConfig not found, retrying in 10 seconds (likely cache sync issue)")
		}
		r.emitWarning(bucket, "ClientError", err)
		return reconcile.Result{RequeueAfter: requeueAfter}, r.Client.Status().Update(ctx, bucket)
	}

	bucketName := bucket.GetBucketName()
	bucketType := bucket.Spec.ForProvider.BucketType
	if bucketType == "" {
		bucketType = "allPrivate"
	}

	// Observe current bucket via B2 native API (gives us BucketID/AccountID/BucketType).
	obs, err := service.B2ListBuckets(ctx)
	if err != nil {
		logger.Error(err, "Failed to list buckets in B2")
		r.setCondition(bucket, xpv1.TypeReady, "False", "ObserveError", err.Error())
		r.emitWarning(bucket, "ObserveError", err)
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
	}

	var current *b2BucketSummary
	for i := range obs.Buckets {
		if obs.Buckets[i].BucketName == bucketName {
			var fileLock bool
			if obs.Buckets[i].FileLockEnabled != nil {
				fileLock = *obs.Buckets[i].FileLockEnabled
			}
			current = &b2BucketSummary{
				BucketID:                    obs.Buckets[i].BucketID,
				AccountID:                   obs.Buckets[i].AccountID,
				BucketType:                  obs.Buckets[i].BucketType,
				BucketInfo:                  obs.Buckets[i].BucketInfo,
				LifecycleRules:              obs.Buckets[i].LifecycleRules,
				CORSRules:                   obs.Buckets[i].CORSRules,
				DefaultServerSideEncryption: obs.Buckets[i].DefaultServerSideEncryption,
				FileLockEnabled:             fileLock,
				DefaultRetention:            obs.Buckets[i].DefaultRetention,
			}
			break
		}
	}

	if current == nil {
		// Bucket does not exist - create it.
		if !shouldCreate(bucket.GetManagementPolicies()) {
			logger.Info("Skipping bucket creation due to managementPolicies", "managementPolicies", bucket.GetManagementPolicies())
			r.setCondition(bucket, xpv1.TypeReady, "False", "ObserveOnly", "External resource does not exist and creation is disabled by managementPolicies")
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
		}
		logger.Info("Creating bucket", "bucketName", bucketName, "bucketType", bucketType)
		info := clients.B2BucketInfo(bucket.Spec.ForProvider.BucketInfo)
		created, err := service.B2CreateBucket(ctx, clients.B2CreateBucketRequest{
			AccountID:  current.AccountID,
			BucketName: bucketName,
			BucketType: bucketType,
			BucketInfo: &info,
		})
		if err != nil {
			logger.Error(err, "Failed to create bucket")
			r.setCondition(bucket, xpv1.TypeReady, "False", "CreateError", err.Error())
			r.emitWarning(bucket, "CreateError", err)
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
		}
		meta.SetExternalName(bucket, bucketName)
		current = &b2BucketSummary{
			BucketID:   created.BucketID,
			AccountID:  created.AccountID,
			BucketType: created.BucketType,
			BucketInfo: created.BucketInfo,
		}
	}

	driftMessages := []string{}

	// BucketType drift. b2_create_bucket always creates "allPrivate"; flipping
	// to "allPublic" requires b2_update_bucket.
	if !strings.EqualFold(current.BucketType, bucketType) {
		if !shouldCreate(bucket.GetManagementPolicies()) {
			driftMessages = append(driftMessages, fmt.Sprintf("bucketType drift detected (%s -> %s) but managementPolicies disallows update", current.BucketType, bucketType))
		} else if current.BucketID == "" {
			driftMessages = append(driftMessages, "bucket ID unknown, cannot apply bucketType drift until next reconcile")
		} else {
			logger.Info("Updating bucket type", "bucketName", bucketName, "from", current.BucketType, "to", bucketType)
			if _, err := service.B2UpdateBucket(ctx, clients.B2UpdateBucketRequest{
				AccountID:  current.AccountID,
				BucketID:   current.BucketID,
				BucketType: bucketType,
			}); err != nil {
				logger.Error(err, "Failed to update bucket type")
				r.setCondition(bucket, xpv1.TypeReady, "False", errApplyBucketType, err.Error())
				r.emitWarning(bucket, "ApplyTypeError", err)
				return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
			}
			current.BucketType = bucketType
		}
	}

	// Lifecycle rules drift (driven through S3 API for the lifecycle block).
	if err := r.reconcileLifecycle(ctx, service, bucketName, bucket.Spec.ForProvider.LifecycleRules, current); err != nil {
		logger.Error(err, "Unable to reconcile lifecycle rules")
		r.setCondition(bucket, xpv1.TypeReady, "False", errApplyLifecycle, err.Error())
		r.emitWarning(bucket, "ApplyLifecycleError", err)
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
	}

	// CORS rules drift.
	if err := r.reconcileCors(ctx, service, bucketName, bucket.Spec.ForProvider.CorsRules, current); err != nil {
		logger.Error(err, "Unable to reconcile CORS rules")
		r.setCondition(bucket, xpv1.TypeReady, "False", errApplyCors, err.Error())
		r.emitWarning(bucket, "ApplyCorsError", err)
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
	}

	// bucketInfo / SSE / file lock / retention. All handled via the same native
	// patch endpoint (B2UpdateBucket); if any of these drift, we send the full
	// set so the server view converges on the spec view.
	if err := r.reconcileNativeSettings(ctx, service, bucket, current, &driftMessages); err != nil {
		logger.Error(err, "Unable to reconcile native bucket settings")
		r.setCondition(bucket, xpv1.TypeReady, "False", errApplyBucketType, err.Error())
		r.emitWarning(bucket, "ApplyNativeError", err)
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
	}

	bucket.Status.AtProvider.BucketName = bucketName
	bucket.Status.AtProvider.BucketID = current.BucketID
	bucket.Status.AtProvider.AccountID = current.AccountID
	bucket.Status.AtProvider.Region = bucket.Spec.ForProvider.Region
	r.setCondition(bucket, xpv1.TypeReady, "True", "Available", "Bucket is ready")
	syncedMsg := "Successfully reconciled"
	if len(driftMessages) > 0 {
		syncedMsg = strings.Join(driftMessages, "; ")
	}
	r.setCondition(bucket, xpv1.TypeSynced, "True", "ReconcileSuccess", syncedMsg)

	if err := r.Client.Status().Update(ctx, bucket); err != nil {
		logger.Error(err, "Failed to update bucket status")
		return reconcile.Result{}, err
	}

	logger.Info("Successfully reconciled bucket", "bucketName", bucketName)
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}

// b2BucketSummary is the subset of b2_list_buckets we care about for drift.
type b2BucketSummary struct {
	BucketID                    string
	AccountID                   string
	BucketType                  string
	BucketInfo                  clients.B2BucketInfo
	LifecycleRules              []clients.B2NativeLifecycleRule
	CORSRules                   []clients.B2NativeCORSRule
	DefaultServerSideEncryption *clients.B2NativeSSESettings
	FileLockEnabled             bool
	DefaultRetention            *clients.B2FileLockRetention
}

// reconcileNativeSettings brings bucketInfo, default SSE, file lock, and
// retention in line with the spec via a single b2_update_bucket call when any
// of them drift.
func (r *BucketReconciler) reconcileNativeSettings(ctx context.Context, service *clients.BackblazeClient, bucket *backblazev1beta1.Bucket, current *b2BucketSummary, driftMessages *[]string) error {
	if current == nil || current.BucketID == "" || current.AccountID == "" {
		return nil
	}

	desiredInfo := clients.B2BucketInfo(bucket.Spec.ForProvider.BucketInfo)

	var desiredSSE *clients.B2NativeSSESettings
	if bucket.Spec.ForProvider.DefaultServerSideEncryption != nil {
		algo := bucket.Spec.ForProvider.DefaultServerSideEncryption.Algorithm
		if algo == "" {
			algo = "AES256"
		}
		desiredSSE = &clients.B2NativeSSESettings{
			Mode:      bucket.Spec.ForProvider.DefaultServerSideEncryption.Mode,
			Algorithm: algo,
		}
	}

	desiredFileLock := bucket.Spec.ForProvider.FileLockEnabled
	var desiredRetention *clients.B2FileLockRetention
	if bucket.Spec.ForProvider.DefaultRetention != nil {
		desiredRetention = &clients.B2FileLockRetention{
			Mode:   bucket.Spec.ForProvider.DefaultRetention.Mode,
			Period: bucket.Spec.ForProvider.DefaultRetention.Period,
		}
	}

	drift := false
	if !stringMapsEqual(current.BucketInfo, desiredInfo) {
		drift = true
		*driftMessages = append(*driftMessages, "bucketInfo drift detected")
	}
	if !sseEqual(current.DefaultServerSideEncryption, desiredSSE) {
		drift = true
		*driftMessages = append(*driftMessages, "defaultServerSideEncryption drift detected")
	}
	if current.FileLockEnabled != desiredFileLock {
		drift = true
		*driftMessages = append(*driftMessages, fmt.Sprintf("fileLockEnabled drift (%v -> %v)", current.FileLockEnabled, desiredFileLock))
	}
	if !retentionEqual(current.DefaultRetention, desiredRetention) {
		drift = true
		*driftMessages = append(*driftMessages, "defaultRetention drift detected")
	}
	if !drift {
		return nil
	}

	req := clients.B2UpdateBucketRequest{
		AccountID:                   current.AccountID,
		BucketID:                    current.BucketID,
		BucketInfo:                  &desiredInfo,
		DefaultServerSideEncryption: desiredSSE,
	}
	if desiredFileLock {
		req.FileLockEnabled = &desiredFileLock
	}
	req.DefaultRetention = desiredRetention

	if !shouldCreate(bucket.GetManagementPolicies()) {
		// ManagementPolicies disallow patching drift; surface in the ready/synced
		// condition messages but don't bubble errors.
		return nil
	}

	resp, err := service.B2UpdateBucket(ctx, req)
	if err != nil {
		return err
	}
	current.BucketInfo = resp.BucketInfo
	current.DefaultServerSideEncryption = resp.DefaultServerSideEncryption
	if resp.FileLockEnabled != nil {
		current.FileLockEnabled = *resp.FileLockEnabled
	}
	current.DefaultRetention = resp.DefaultRetention
	return nil
}

func stringMapsEqual(a, b clients.B2BucketInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func sseEqual(a, b *clients.B2NativeSSESettings) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Mode == b.Mode && a.Algorithm == b.Algorithm
}

func retentionEqual(a, b *clients.B2FileLockRetention) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Mode == b.Mode && a.Period == b.Period
}

// reconcileLifecycle brings the B2 native lifecycle rules on the bucket in
// line with the spec. The S3-compatible lifecycle API is not used here
// because it can only express deletion, not the B2-native "hiding" of older
// file versions (DaysFromUploadingToHiding).
func (r *BucketReconciler) reconcileLifecycle(ctx context.Context, service *clients.BackblazeClient, _ string, desired []backblazev1beta1.LifecycleRule, current *b2BucketSummary) error {
	if current == nil || current.BucketID == "" || current.AccountID == "" {
		// Can't address the bucket without an ID; skip silently and let the
		// next reconcile (after observation populates status) take care of it.
		return nil
	}
	desiredNative := convertNativeLifecycleRules(desired)
	if nativeLifecycleRulesEqual(desiredNative, current.LifecycleRules) {
		return nil
	}

	_, err := service.B2UpdateBucket(ctx, clients.B2UpdateBucketRequest{
		AccountID:      current.AccountID,
		BucketID:       current.BucketID,
		LifecycleRules: desiredNative,
	})
	if err != nil {
		return err
	}
	current.LifecycleRules = desiredNative
	return nil
}

// reconcileCors brings the S3 CORS configuration on the bucket in line with the spec.
func (r *BucketReconciler) reconcileCors(ctx context.Context, service *clients.BackblazeClient, bucketName string, desired []backblazev1beta1.CORSRule, _ *b2BucketSummary) error {
	desiredSDKRules := convertCorsRules(desired)
	currentSDKRules, err := service.GetBucketCors(ctx, bucketName)
	if err != nil && !errors.Is(err, clients.ErrBucketCorsNotFound) {
		return err
	}
	haveCurrent := err == nil

	if !haveCurrent || !corsRulesEqual(desiredSDKRules, currentSDKRules) {
		if len(desiredSDKRules) == 0 {
			if haveCurrent {
				return service.DeleteBucketCors(ctx, bucketName)
			}
			return nil
		}
		return service.PutBucketCors(ctx, bucketName, desiredSDKRules)
	}
	return nil
}

func (r *BucketReconciler) emitWarning(bucket *backblazev1beta1.Bucket, reason string, err error) {
	if r.Recorder == nil || err == nil {
		return
	}
	r.Recorder.Eventf(bucket, nil, corev1.EventTypeWarning, reason, "Reconcile", err.Error())
}

func (r *BucketReconciler) getBackblazeClient(ctx context.Context, bucket *backblazev1beta1.Bucket) (*clients.BackblazeClient, error) {
	// Crossplane v2 sets a kubebuilder default of {"kind":"ClusterProviderConfig","name":"default"}.
	// We always honour the referenced name; if it's empty we treat it as "default".
	providerConfigName := "default"
	if ref := bucket.GetProviderConfigReference(); ref != nil && ref.Name != "" {
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
			return nil, errors.Wrap(err, errGetPC)
		}
		// Other errors (permission, etc.) - return immediately
		return nil, errors.Wrap(err, errGetPC)
	}

	cfg, err := clients.GetProviderConfig(ctx, r.Client, pc)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	return clients.NewBackblazeClient(*cfg)
}

func (r *BucketReconciler) handleDeletion(ctx context.Context, bucket *backblazev1beta1.Bucket) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Respect managementPolicies - if Observe only, don't delete external resource
	if !shouldDelete(bucket.GetManagementPolicies()) {
		logger.Info("Skipping bucket deletion due to managementPolicies", "managementPolicies", bucket.GetManagementPolicies())
		return reconcile.Result{}, nil
	}

	bucketName := bucket.GetBucketName()
	service, err := r.getBackblazeClient(ctx, bucket)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client for deletion")
		// Continue without blocking deletion if client creation fails (ProviderConfig may be gone)
		return reconcile.Result{}, nil
	}

	exists, err := service.BucketExists(ctx, bucketName)
	if err != nil {
		logger.Error(err, "Failed to check bucket existence for deletion")
		// Continue with deletion even if check fails
		return reconcile.Result{}, nil
	}
	if !exists {
		logger.Info("Bucket already absent in B2", "bucketName", bucketName)
		return reconcile.Result{}, nil
	}

	// Honour BucketDeletionPolicy.
	switch bucket.Spec.ForProvider.BucketDeletionPolicy {
	case backblazev1beta1.DeleteAll:
		if err := service.DeleteAllObjectsInBucket(ctx, bucketName); err != nil {
			r.setCondition(bucket, xpv1.TypeReady, "False", errEmptyBucket, err.Error())
			r.emitWarning(bucket, "EmptyBucketError", err)
			// Don't block Kubernetes GC - log and continue to the bucket delete attempt.
			logger.Error(err, "Failed to empty bucket before deletion; will still attempt bucket delete")
		} else {
			logger.Info("Emptied bucket prior to deletion", "bucketName", bucketName)
		}
	case backblazev1beta1.DeleteIfEmpty, "":
		// No-op. DeleteBucket will fail with BucketNotEmpty if the bucket has objects;
		// that surfaces as a kube error and is intentional.
	}

	if err := service.DeleteBucket(ctx, bucketName); err != nil {
		if isBucketNotFound(err) {
			logger.Info("Bucket already absent in B2", "bucketName", bucketName)
			return reconcile.Result{}, nil
		}
		if bucket.Spec.ForProvider.BucketDeletionPolicy == backblazev1beta1.DeleteIfEmpty && strings.Contains(err.Error(), "BucketNotEmpty") {
			logger.Info("Bucket still has objects and BucketDeletionPolicy is DeleteIfEmpty - leaving for manual cleanup",
				"bucketName", bucketName)
			r.setCondition(bucket, xpv1.TypeReady, "False", "BucketNotEmpty",
				"Bucket has objects and BucketDeletionPolicy is DeleteIfEmpty; delete objects manually or change the policy to DeleteAll")
			r.emitWarning(bucket, "BucketNotEmpty", err)
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to delete bucket from B2")
		r.emitWarning(bucket, "DeleteError", err)
		return reconcile.Result{}, nil
	}

	logger.Info("Successfully deleted bucket from B2", "bucketName", bucketName)
	r.emitWarning(bucket, "BucketDeleted", errors.New("bucket successfully deleted from Backblaze B2"))
	return reconcile.Result{}, nil
}

func isBucketNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return s == "NotFound" || s == "NoSuchBucket" || strings.Contains(s, "404")
}

// convertNativeLifecycleRules turns our CRD LifecycleRule into the B2 native
// shape understood by b2_update_bucket.
func convertNativeLifecycleRules(in []backblazev1beta1.LifecycleRule) []clients.B2NativeLifecycleRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]clients.B2NativeLifecycleRule, 0, len(in))
	for _, r := range in {
		out = append(out, clients.B2NativeLifecycleRule{
			FileNamePrefix:            r.FileNamePrefix,
			DaysFromUploadingToHiding: r.DaysFromUploadingToHiding,
			DaysFromHidingToDeleting:  r.DaysFromHidingToDeleting,
		})
	}
	return out
}

// nativeLifecycleRulesEqual compares two slices of B2-native lifecycle rules
// for semantic equality, sorting by FileNamePrefix.
func nativeLifecycleRulesEqual(desired, current []clients.B2NativeLifecycleRule) bool {
	if len(desired) != len(current) {
		return false
	}
	d := append([]clients.B2NativeLifecycleRule(nil), desired...)
	c := append([]clients.B2NativeLifecycleRule(nil), current...)
	sort.Slice(d, func(i, j int) bool { return d[i].FileNamePrefix < d[j].FileNamePrefix })
	sort.Slice(c, func(i, j int) bool { return c[i].FileNamePrefix < c[j].FileNamePrefix })
	for i := range d {
		if d[i].FileNamePrefix != c[i].FileNamePrefix {
			return false
		}
		if !equalIntPtr(d[i].DaysFromUploadingToHiding, c[i].DaysFromUploadingToHiding) {
			return false
		}
		if !equalIntPtr(d[i].DaysFromHidingToDeleting, c[i].DaysFromHidingToDeleting) {
			return false
		}
	}
	return true
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// convertCorsRules turns our CRD CORSRule into the AWS S3 SDK shape.
func convertCorsRules(in []backblazev1beta1.CORSRule) []types.CORSRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.CORSRule, 0, len(in))
	for _, r := range in {
		rule := types.CORSRule{
			ID:             aws.String(r.CorsRuleName),
			AllowedMethods: r.AllowedMethods,
			AllowedOrigins: r.AllowedOrigins,
			AllowedHeaders: r.AllowedHeaders,
			ExposeHeaders:  r.ExposeHeaders,
		}
		if r.MaxAgeSeconds != nil {
			rule.MaxAgeSeconds = aws.Int32(int32(*r.MaxAgeSeconds))
		}
		out = append(out, rule)
	}
	return out
}

// corsRulesEqual compares two slices of CORS rules for semantic equality,
// sorting by rule ID so the comparison is order-independent.
func corsRulesEqual(desired, current []types.CORSRule) bool {
	if len(desired) != len(current) {
		return false
	}
	d := append([]types.CORSRule(nil), desired...)
	c := append([]types.CORSRule(nil), current...)
	idOf := func(r types.CORSRule) string {
		if r.ID != nil {
			return *r.ID
		}
		return ""
	}
	sort.Slice(d, func(i, j int) bool { return idOf(d[i]) < idOf(d[j]) })
	sort.Slice(c, func(i, j int) bool { return idOf(c[i]) < idOf(c[j]) })
	for i := range d {
		dr, cr := d[i], c[i]
		if idOf(dr) != idOf(cr) {
			return false
		}
		if !equalStringSlices(dr.AllowedOrigins, cr.AllowedOrigins) {
			return false
		}
		if !equalStringSlices(dr.AllowedMethods, cr.AllowedMethods) {
			return false
		}
		if !equalStringSlices(dr.AllowedHeaders, cr.AllowedHeaders) {
			return false
		}
		if !equalStringSlices(dr.ExposeHeaders, cr.ExposeHeaders) {
			return false
		}
		if !equalInt32Ptr(dr.MaxAgeSeconds, cr.MaxAgeSeconds) {
			return false
		}
	}
	return true
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func equalInt32Ptr(a, b *int32) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (r *BucketReconciler) setCondition(bucket *backblazev1beta1.Bucket, conditionType xpv1.ConditionType, status, reason, message string) {
	bucket.SetConditions(xpv1.Condition{
		Type:               conditionType,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            message,
	})
}

// shouldCreate returns true if managementPolicies allow creation.
// Empty (nil) policies are treated as default "*": allow all.
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
