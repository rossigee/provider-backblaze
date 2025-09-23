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
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotBucket     = "managed resource is not a Bucket custom resource"
	errTrackPCUsage  = "cannot track ProviderConfig usage"
	errGetPC         = "cannot get ProviderConfig"
	errGetCreds      = "cannot get credentials"
	errNewClient     = "cannot create new Service"
	errCreateBucket  = "cannot create bucket"
	errDeleteBucket  = "cannot delete bucket"
	errObserveBucket = "cannot observe bucket"
)


// SetupBucket adds a controller that reconciles Bucket managed resources.
func SetupBucket(mgr ctrl.Manager, o controller.Options) error {
	r := &BucketReconciler{
		Client: mgr.GetClient(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("bucket-controller").
		For(&backblazev1.Bucket{}).
		Watches(&apisv1beta1.ProviderConfig{}, handler.Funcs{}).
		Complete(r)
}


// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	Client client.Client
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BucketReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("bucket", req.NamespacedName)

	// Fetch the Bucket instance
	bucket := &backblazev1.Bucket{}
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

	// Get provider config and create client
	service, err := r.getBackblazeClient(ctx, bucket)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client")
		r.setCondition(bucket, xpv1.TypeReady, "False", "ClientError", err.Error())
		// Use shorter requeue time for ProviderConfig not found errors (likely cache sync issue)
		requeueAfter := time.Minute
		if strings.Contains(err.Error(), "not found") {
			requeueAfter = 10 * time.Second
			logger.Info("ProviderConfig not found, retrying in 10 seconds (likely cache sync issue)")
		}
		return reconcile.Result{RequeueAfter: requeueAfter}, r.Client.Status().Update(ctx, bucket)
	}

	// Check if bucket exists
	bucketName := bucket.GetBucketName()
	exists, err := service.BucketExists(ctx, bucketName)
	if err != nil {
		logger.Error(err, "Failed to check bucket existence")
		r.setCondition(bucket, xpv1.TypeReady, "False", "CheckError", err.Error())
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
	}

	if !exists {
		// Create bucket
		logger.Info("Creating bucket", "bucketName", bucketName)
		bucketType := bucket.Spec.ForProvider.BucketType
		if bucketType == "" {
			bucketType = "allPrivate"
		}

		err = service.CreateBucket(ctx, bucketName, bucketType, bucket.Spec.ForProvider.Region)
		if err != nil {
			logger.Error(err, "Failed to create bucket")
			r.setCondition(bucket, xpv1.TypeReady, "False", "CreateError", err.Error())
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, bucket)
		}

		// Set external name
		meta.SetExternalName(bucket, bucketName)
	}

	// Update status
	bucket.Status.AtProvider.BucketName = bucketName
	r.setCondition(bucket, xpv1.TypeReady, "True", "Available", "Bucket is ready")

	// Update the resource
	if err := r.Client.Status().Update(ctx, bucket); err != nil {
		logger.Error(err, "Failed to update bucket status")
		return reconcile.Result{}, err
	}

	logger.Info("Successfully reconciled bucket")
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *BucketReconciler) getBackblazeClient(ctx context.Context, bucket *backblazev1.Bucket) (*clients.BackblazeClient, error) {
	// Determine ProviderConfig name - use "default" if not specified
	providerConfigName := "default"
	if bucket.GetProviderConfigReference() != nil {
		providerConfigName = bucket.GetProviderConfigReference().Name
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

func (r *BucketReconciler) setCondition(bucket *backblazev1.Bucket, conditionType xpv1.ConditionType, status, reason, message string) {
	bucket.SetConditions(xpv1.Condition{
		Type:               conditionType,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            message,
	})
}
