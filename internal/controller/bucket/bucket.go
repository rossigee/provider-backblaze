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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	bucketv1beta1 "github.com/rossigee/provider-backblaze/apis/bucket/v1beta1"
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
	name := managed.ControllerName(bucketv1beta1.BucketGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(bucketv1beta1.BucketGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1beta1.ProviderConfigUsage{}),
			newServiceFn: clients.NewBackblazeClient,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&bucketv1beta1.Bucket{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}


// connector and external implementations for namespaced resources

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(config clients.Config) (*clients.BackblazeClient, error)
}

// Connect produces an ExternalClient for v1beta1 Bucket resources.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*bucketv1beta1.Bucket)
	if !ok {
		return nil, errors.New(errNotBucket)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1beta1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	cfg, err := clients.GetProviderConfig(ctx, c.kube, pc)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	service, err := c.newServiceFn(*cfg)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{service: service}, nil
}

// An external observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	service *clients.BackblazeClient
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*bucketv1beta1.Bucket)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()

	exists, err := c.service.BucketExists(ctx, bucketName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveBucket)
	}

	if !exists {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Update status with current state
	cr.Status.AtProvider.BucketName = bucketName

	// Get bucket location/region
	location, err := c.service.GetBucketLocation(ctx, bucketName)
	if err != nil {
		// Don't fail observation if we can't get location
		location = cr.Spec.ForProvider.Region
	}

	// Check if the bucket configuration matches desired state
	upToDate := location == "" || location == cr.Spec.ForProvider.Region

	// Set external name if not already set
	if meta.GetExternalName(cr) == "" {
		meta.SetExternalName(cr, bucketName)
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*bucketv1beta1.Bucket)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()
	bucketType := cr.Spec.ForProvider.BucketType
	if bucketType == "" {
		bucketType = "allPrivate"
	}
	region := cr.Spec.ForProvider.Region

	err := c.service.CreateBucket(ctx, bucketName, bucketType, region)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBucket)
	}

	// Set external name
	meta.SetExternalName(cr, bucketName)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	_, ok := mg.(*bucketv1beta1.Bucket)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotBucket)
	}

	// Most bucket properties cannot be updated after creation in Backblaze B2
	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*bucketv1beta1.Bucket)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()

	// Handle deletion policy
	if cr.Spec.ForProvider.BucketDeletionPolicy == bucketv1beta1.DeleteAll {
		// Delete all objects first
		if err := c.service.DeleteAllObjectsInBucket(ctx, bucketName); err != nil {
			return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete objects in bucket")
		}
	}

	// Delete the bucket
	err := c.service.DeleteBucket(ctx, bucketName)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteBucket)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	// No special disconnect logic needed for Backblaze B2 client
	return nil
}
