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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
	"github.com/rossigee/provider-backblaze/internal/clients"
	"github.com/rossigee/provider-backblaze/internal/features"
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
	name := managed.ControllerName(backblazev1.UserGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(backblazev1.UserGroupVersionKind),
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
		For(&backblazev1.User{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(ctx context.Context, region, applicationKeyID, applicationKey, endpointURL string) (clients.BackblazeClientInterface, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return nil, errors.New(errNotUser)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	// Get the ProviderConfig
	providerConfigName := cr.GetProviderConfigReference().Name
	if providerConfigName == "" {
		providerConfigName = "default"
	}

	pc := &apisv1beta1.ProviderConfig{}
	key := client.ObjectKey{Name: providerConfigName, Namespace: "crossplane-system"}
	if err := c.kube.Get(ctx, key, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	backblazeClient, err := clients.GetProviderConfigClient(ctx, c.kube, pc, c.newServiceFn)
	if err != nil {
		return nil, errors.Wrap(err, errCreateBackblazeClient)
	}

	return &external{service: backblazeClient, kube: c.kube}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	service clients.BackblazeClientInterface
	kube    client.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUser)
	}

	// If we don't have an application key ID yet, the resource doesn't exist
	if cr.Status.AtProvider.ApplicationKeyID == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// For now, we assume the key exists if we have an ID
	// In a full implementation, you would call the Backblaze API to verify
	// TODO: Implement actual key verification via B2 API

	// Update status with current information
	cr.Status.AtProvider.Capabilities = cr.Spec.ForProvider.Capabilities
	if cr.Spec.ForProvider.BucketID != nil {
		cr.Status.AtProvider.BucketID = cr.Spec.ForProvider.BucketID
	}
	if cr.Spec.ForProvider.NamePrefix != nil {
		cr.Status.AtProvider.NamePrefix = cr.Spec.ForProvider.NamePrefix
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUser)
	}

	// For this implementation, we'll simulate application key creation
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 application key creation

	// Generate a simulated application key ID and key
	applicationKeyID := fmt.Sprintf("K005%012d", cr.GetGeneration())
	applicationKey := fmt.Sprintf("K005%024d", cr.GetGeneration()*1000)

	// Update the resource status
	cr.Status.AtProvider.ApplicationKeyID = applicationKeyID
	cr.Status.AtProvider.AccountID = "simulated-account-id"
	cr.Status.AtProvider.Capabilities = cr.Spec.ForProvider.Capabilities
	if cr.Spec.ForProvider.BucketID != nil {
		cr.Status.AtProvider.BucketID = cr.Spec.ForProvider.BucketID
	}
	if cr.Spec.ForProvider.NamePrefix != nil {
		cr.Status.AtProvider.NamePrefix = cr.Spec.ForProvider.NamePrefix
	}
	if cr.Spec.ForProvider.ValidDurationInSeconds != nil {
		cr.Status.AtProvider.ExpirationTimestamp = cr.Spec.ForProvider.ValidDurationInSeconds
	}

	// Create the secret with the application key credentials
	if err := c.writeSecret(ctx, cr, applicationKeyID, applicationKey); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errWriteSecret)
	}

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"applicationKeyId": []byte(applicationKeyID),
			"applicationKey":   []byte(applicationKey),
		},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	// Application keys in Backblaze B2 are typically immutable
	// Updates would require creating a new key and deleting the old one
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return errors.New(errNotUser)
	}

	// For this implementation, we'll simulate application key deletion
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 application key deletion

	// Delete the associated secret
	if err := c.deleteSecret(ctx, cr); err != nil {
		// Log the error but don't fail the deletion
		// The main resource should still be considered deleted
		return nil
	}

	return nil
}

// writeSecret creates or updates the secret containing the application key credentials
func (c *external) writeSecret(ctx context.Context, cr *backblazev1.User, applicationKeyID, applicationKey string) error {
	secretRef := cr.Spec.ForProvider.WriteSecretToRef

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

	return c.kube.Create(ctx, secret)
}

// deleteSecret removes the secret containing the application key credentials
func (c *external) deleteSecret(ctx context.Context, cr *backblazev1.User) error {
	secretRef := cr.Spec.ForProvider.WriteSecretToRef

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
	}

	return client.IgnoreNotFound(c.kube.Delete(ctx, secret))
}