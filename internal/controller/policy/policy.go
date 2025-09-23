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

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
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
	errNotPolicy                = "managed resource is not a Policy custom resource"
	errTrackPCUsage             = "cannot track ProviderConfig usage"
	errGetProviderConfig        = "cannot get referenced ProviderConfig"
	errCreateBackblazeClient    = "cannot create Backblaze client"
	errCreatePolicy             = "cannot create policy"
	errDeletePolicy             = "cannot delete policy"
	errGetPolicy                = "cannot get policy"
	errInvalidPolicyParams      = "invalid policy parameters: specify either allowBucket or rawPolicy, not both"
	errGenerateSimplePolicy     = "cannot generate simple policy document"
	errInvalidRawPolicy         = "invalid raw policy: must be valid JSON"
)

// SetupPolicy adds a controller that reconciles Policy managed resources.
func SetupPolicy(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(backblazev1.PolicyGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(backblazev1.PolicyGroupVersionKind),
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
		For(&backblazev1.Policy{}).
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
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return nil, errors.New(errNotPolicy)
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
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotPolicy)
	}

	// If we don't have a policy name yet, the resource doesn't exist
	if cr.Status.AtProvider.PolicyName == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// For now, we assume the policy exists if we have a name
	// In a full implementation, you would call the Backblaze API to verify
	// TODO: Implement actual policy verification via B2 API

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotPolicy)
	}

	// Validate policy parameters
	params := cr.Spec.ForProvider
	if (params.AllowBucket != nil && params.RawPolicy != nil) ||
		(params.AllowBucket == nil && params.RawPolicy == nil) {
		return managed.ExternalCreation{}, errors.New(errInvalidPolicyParams)
	}

	var policyDocument string
	var err error

	if params.AllowBucket != nil {
		// Generate simple policy for the bucket
		policyDocument, err = c.generateSimplePolicy(*params.AllowBucket)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errGenerateSimplePolicy)
		}
	} else {
		// Use raw policy document
		policyDocument = *params.RawPolicy
		// Validate it's valid JSON
		var temp interface{}
		if err := json.Unmarshal([]byte(policyDocument), &temp); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errInvalidRawPolicy)
		}
	}

	// Get policy name
	policyName := cr.GetPolicyName()

	// For this implementation, we'll simulate policy creation
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 policy creation

	// Update the resource status
	cr.Status.AtProvider.PolicyName = policyName
	cr.Status.AtProvider.PolicyDocument = policyDocument
	cr.Status.AtProvider.PolicyID = fmt.Sprintf("policy-%d", cr.GetGeneration())
	now := metav1.NewTime(time.Now())
	cr.Status.AtProvider.CreationTime = &now

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	// Policies in Backblaze B2 would typically require deletion and recreation
	// for updates, but for now we'll support updates
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return errors.New(errNotPolicy)
	}

	// For this implementation, we'll simulate policy deletion
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 policy deletion

	_ = cr // Prevent unused variable warning

	return nil
}

// generateSimplePolicy creates a basic policy that allows all operations for a specific bucket
func (c *external) generateSimplePolicy(bucketName string) (string, error) {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:*"},
				"Resource": []string{
					fmt.Sprintf("arn:aws:s3:::%s", bucketName),
					fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
				},
			},
		},
	}

	policyBytes, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return "", err
	}

	return string(policyBytes), nil
}