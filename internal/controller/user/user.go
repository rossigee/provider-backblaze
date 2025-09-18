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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	userv1beta1 "github.com/rossigee/provider-backblaze/apis/user/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotUser       = "managed resource is not a User custom resource"
	errTrackPCUsage  = "cannot track ProviderConfig usage"
	errGetPC         = "cannot get ProviderConfig"
	errGetCreds      = "cannot get credentials"
	errNewClient     = "cannot create new Service"
	errCreateUser    = "cannot create application key"
	errDeleteUser    = "cannot delete application key"
	errObserveUser   = "cannot observe application key"
	errWriteSecret   = "cannot write secret"
)

// SetupUser adds a controller that reconciles User managed resources.
func SetupUser(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(userv1beta1.UserGroupKind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(userv1beta1.UserGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.TrackerFn(func(ctx context.Context, mg resource.Managed) error { return nil }),
			newServiceFn: clients.NewBackblazeClient,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&userv1beta1.User{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// Helper function for comparing string slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// Create a map to count occurrences in slice a
	counts := make(map[string]int)
	for _, v := range a {
		counts[v]++
	}

	// Check that slice b has the same elements with same counts
	for _, v := range b {
		count, exists := counts[v]
		if !exists || count == 0 {
			return false
		}
		counts[v]--
	}

	return true
}

// connector and external implementations for namespaced resources

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(clients.Config) (*clients.BackblazeClient, error)
}

// Connect produces an ExternalClient for v1beta1 User resources.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*userv1beta1.User)
	if !ok {
		return nil, errors.New(errNotUser)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if cr.GetProviderConfigReference() == nil {
		return nil, errors.New("no providerConfigRef provided")
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

	return &external{service: service, kube: c.kube}, nil
}

// An external observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the v1beta1 managed resource's desired state.
type external struct {
	service *clients.BackblazeClient
	kube    client.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*userv1beta1.User)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUser)
	}

	// Use external name if set, otherwise we haven't created the key yet
	applicationKeyID := meta.GetExternalName(cr)
	if applicationKeyID == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Get the application key from Backblaze B2
	key, err := c.service.GetApplicationKey(ctx, applicationKeyID)
	if err != nil {
		if err.Error() == "application key not found" {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveUser)
	}

	// Check if the key configuration matches desired state
	upToDate := key.KeyName == cr.Spec.ForProvider.KeyName &&
		equalStringSlices(key.Capabilities, cr.Spec.ForProvider.Capabilities) &&
		key.BucketID == cr.Spec.ForProvider.BucketID &&
		key.NamePrefix == cr.Spec.ForProvider.NamePrefix

	// Update status with current key information
	cr.Status.AtProvider.ApplicationKeyID = key.ApplicationKeyID
	cr.Status.AtProvider.KeyName = key.KeyName
	cr.Status.AtProvider.Capabilities = key.Capabilities
	cr.Status.AtProvider.BucketID = key.BucketID
	cr.Status.AtProvider.NamePrefix = key.NamePrefix

	if key.ExpirationTimestamp != nil {
		expirationTime := time.Unix(*key.ExpirationTimestamp/1000, 0)
		cr.Status.AtProvider.ExpirationTimestamp = &metav1.Time{Time: expirationTime}
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*userv1beta1.User)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUser)
	}

	// Create the application key
	key, err := c.service.CreateApplicationKey(
		ctx,
		cr.Spec.ForProvider.KeyName,
		cr.Spec.ForProvider.Capabilities,
		cr.Spec.ForProvider.BucketID,
		cr.Spec.ForProvider.NamePrefix,
		cr.Spec.ForProvider.ValidDurationInSeconds,
	)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateUser)
	}

	// Set external name to the application key ID
	meta.SetExternalName(cr, key.ApplicationKeyID)

	// Write the application key to the specified secret
	err = c.writeSecretForKeyV1Beta1(ctx, cr, key)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errWriteSecret)
	}

	return managed.ExternalCreation{
		// Connection details will be written to the secret
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	// Application keys cannot be updated in Backblaze B2
	// If the spec changes, we need to delete and recreate
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*userv1beta1.User)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotUser)
	}

	applicationKeyID := meta.GetExternalName(cr)
	if applicationKeyID == "" {
		// Nothing to delete
		return managed.ExternalDelete{}, nil
	}

	err := c.service.DeleteApplicationKey(ctx, applicationKeyID)
	if err != nil && err.Error() != "application key not found" {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteUser)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	// No special disconnect logic needed for Backblaze B2 client
	return nil
}

// writeSecretForKeyV1Beta1 writes the application key to the specified secret for v1beta1 resources
func (c *external) writeSecretForKeyV1Beta1(ctx context.Context, cr *userv1beta1.User, key *clients.B2CreateKeyResponse) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.ForProvider.WriteSecretToRef.Name,
			Namespace: cr.Spec.ForProvider.WriteSecretToRef.Namespace,
		},
		Data: map[string][]byte{
			"applicationKeyId": []byte(key.ApplicationKeyID),
			"applicationKey":   []byte(key.ApplicationKey),
			"keyName":          []byte(key.KeyName),
		},
	}

	err := c.kube.Create(ctx, secret)
	if err != nil {
		// If secret already exists, update it
		existingSecret := &corev1.Secret{}
		if getErr := c.kube.Get(ctx, types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}, existingSecret); getErr == nil {
			existingSecret.Data = secret.Data
			return c.kube.Update(ctx, existingSecret)
		}
		return err
	}

	return nil
}
