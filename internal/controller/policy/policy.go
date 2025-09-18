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

	policyv1beta1 "github.com/rossigee/provider-backblaze/apis/policy/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotPolicy      = "managed resource is not a Policy custom resource"
	errTrackPCUsage   = "cannot track ProviderConfig usage"
	errGetPC          = "cannot get ProviderConfig"
	errGetCreds       = "cannot get credentials"
	errNewClient      = "cannot create new Service"
	errCreatePolicy   = "cannot create bucket policy"
	errDeletePolicy   = "cannot delete bucket policy"
	errObservePolicy  = "cannot observe bucket policy"
	errInvalidPolicy  = "invalid policy configuration"
	errGeneratePolicy = "cannot generate policy document"
)

// SetupPolicy adds a controller that reconciles Policy managed resources.
func SetupPolicy(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(policyv1beta1.PolicyGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(policyv1beta1.PolicyGroupVersionKind),
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
		For(&policyv1beta1.Policy{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector and external implementations for namespaced resources

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(clients.Config) (*clients.BackblazeClient, error)
}

// Connect produces an ExternalClient for v1beta1 Policy resources.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*policyv1beta1.Policy)
	if !ok {
		return nil, errors.New(errNotPolicy)
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
// external resource to ensure it reflects the v1beta1 managed resource's desired state.
type external struct {
	service *clients.BackblazeClient
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*policyv1beta1.Policy)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotPolicy)
	}

	// Get the bucket name from the policy configuration
	bucketName, err := c.getBucketNameFromPolicyV1Beta1(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errInvalidPolicy)
	}

	// Check if bucket exists (policies are associated with buckets in S3)
	exists, err := c.service.BucketExists(ctx, bucketName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObservePolicy)
	}

	if !exists {
		// If the bucket doesn't exist, the policy can't exist either
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Get the current bucket policy
	currentPolicy, err := c.service.GetBucketPolicy(ctx, bucketName)
	if err != nil {
		if err.Error() == "NoSuchBucketPolicy" || err.Error() == "bucket policy not found" {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObservePolicy)
	}

	// Generate the desired policy document
	desiredPolicy, err := c.generatePolicyDocumentV1Beta1(cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGeneratePolicy)
	}

	// Update status with current state
	cr.Status.AtProvider.PolicyName = c.getPolicyNameV1Beta1(cr)
	cr.Status.AtProvider.Policy = currentPolicy

	// Check if the policy is up to date
	upToDate := c.isPolicyUpToDate(currentPolicy, desiredPolicy)

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
	cr, ok := mg.(*policyv1beta1.Policy)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotPolicy)
	}

	// Get the bucket name
	bucketName, err := c.getBucketNameFromPolicyV1Beta1(cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errInvalidPolicy)
	}

	// Generate the policy document
	policyDocument, err := c.generatePolicyDocumentV1Beta1(cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errGeneratePolicy)
	}

	// Apply the policy to the bucket
	err = c.service.PutBucketPolicy(ctx, bucketName, policyDocument)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreatePolicy)
	}

	// Set external name
	meta.SetExternalName(cr, bucketName)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*policyv1beta1.Policy)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotPolicy)
	}

	// Policies can be updated by applying the new policy document
	bucketName, err := c.getBucketNameFromPolicyV1Beta1(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errInvalidPolicy)
	}

	// Generate the new policy document
	policyDocument, err := c.generatePolicyDocumentV1Beta1(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errGeneratePolicy)
	}

	// Apply the updated policy
	err = c.service.PutBucketPolicy(ctx, bucketName, policyDocument)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errCreatePolicy)
	}

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*policyv1beta1.Policy)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotPolicy)
	}

	bucketName, err := c.getBucketNameFromPolicyV1Beta1(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errInvalidPolicy)
	}

	// Delete the bucket policy
	err = c.service.DeleteBucketPolicy(ctx, bucketName)
	if err != nil && err.Error() != "NoSuchBucketPolicy" && err.Error() != "bucket policy not found" {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeletePolicy)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	// No special disconnect logic needed for Backblaze B2 client
	return nil
}

// Helper functions for v1beta1 Policy resources

func (c *external) getBucketNameFromPolicyV1Beta1(cr *policyv1beta1.Policy) (string, error) {
	if cr.Spec.ForProvider.AllowBucket != "" {
		return cr.Spec.ForProvider.AllowBucket, nil
	}

	if cr.Spec.ForProvider.RawPolicy != "" {
		// Parse the raw policy to extract bucket name from Resource ARN
		var policy map[string]interface{}
		if err := json.Unmarshal([]byte(cr.Spec.ForProvider.RawPolicy), &policy); err != nil {
			return "", errors.Wrap(err, "invalid JSON in rawPolicy")
		}

		// Extract bucket name from the first resource ARN
		statements, ok := policy["Statement"].([]interface{})
		if !ok || len(statements) == 0 {
			return "", errors.New("no statements found in policy")
		}

		firstStatement, ok := statements[0].(map[string]interface{})
		if !ok {
			return "", errors.New("invalid statement format")
		}

		resources, ok := firstStatement["Resource"]
		if !ok {
			return "", errors.New("no Resource found in policy statement")
		}

		// Handle both string and array resource formats
		var bucketName string
		switch res := resources.(type) {
		case string:
			bucketName = c.extractBucketFromARN(res)
		case []interface{}:
			if len(res) > 0 {
				if resStr, ok := res[0].(string); ok {
					bucketName = c.extractBucketFromARN(resStr)
				}
			}
		}

		if bucketName == "" {
			return "", errors.New("could not extract bucket name from policy Resource ARN")
		}

		return bucketName, nil
	}

	return "", errors.New("either allowBucket or rawPolicy must be specified")
}

func (c *external) extractBucketFromARN(arn string) string {
	// S3 ARN format: arn:aws:s3:::bucket-name or arn:aws:s3:::bucket-name/*
	// For Backblaze B2, we support the same format
	if len(arn) > 13 && arn[:13] == "arn:aws:s3:::" {
		bucketPart := arn[13:]
		// Remove /* suffix if present
		for i, c := range bucketPart {
			if c == '/' {
				return bucketPart[:i]
			}
		}
		return bucketPart
	}
	return ""
}

func (c *external) generatePolicyDocumentV1Beta1(cr *policyv1beta1.Policy) (string, error) {
	if cr.Spec.ForProvider.RawPolicy != "" {
		// Validate that it's valid JSON
		var test map[string]interface{}
		if err := json.Unmarshal([]byte(cr.Spec.ForProvider.RawPolicy), &test); err != nil {
			return "", errors.Wrap(err, "rawPolicy is not valid JSON")
		}
		return cr.Spec.ForProvider.RawPolicy, nil
	}

	if cr.Spec.ForProvider.AllowBucket != "" {
		// Generate a simple allow-all policy for the bucket
		policy := map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect": "Allow",
					"Principal": map[string]interface{}{
						"AWS": "*",
					},
					"Action": "s3:*",
					"Resource": []string{
						fmt.Sprintf("arn:aws:s3:::%s", cr.Spec.ForProvider.AllowBucket),
						fmt.Sprintf("arn:aws:s3:::%s/*", cr.Spec.ForProvider.AllowBucket),
					},
				},
			},
		}

		policyBytes, err := json.Marshal(policy)
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal generated policy")
		}

		return string(policyBytes), nil
	}

	return "", errors.New("either allowBucket or rawPolicy must be specified")
}

func (c *external) getPolicyNameV1Beta1(cr *policyv1beta1.Policy) string {
	if cr.Spec.ForProvider.PolicyName != "" {
		return cr.Spec.ForProvider.PolicyName
	}
	return cr.Name
}

// Aliases for tests (standard naming without version suffix)
func (c *external) generatePolicyDocument(cr *policyv1beta1.Policy) (string, error) {
	return c.generatePolicyDocumentV1Beta1(cr)
}

func (c *external) getBucketNameFromPolicy(cr *policyv1beta1.Policy) (string, error) {
	return c.getBucketNameFromPolicyV1Beta1(cr)
}

func (c *external) getPolicyName(cr *policyv1beta1.Policy) string {
	return c.getPolicyNameV1Beta1(cr)
}

func (c *external) isPolicyUpToDate(current, desired string) bool {
	// Normalize JSON for comparison
	var currentMap, desiredMap map[string]interface{}

	if err := json.Unmarshal([]byte(current), &currentMap); err != nil {
		return false
	}

	if err := json.Unmarshal([]byte(desired), &desiredMap); err != nil {
		return false
	}

	// Convert back to JSON for comparison (normalizes formatting)
	currentBytes, _ := json.Marshal(currentMap)
	desiredBytes, _ := json.Marshal(desiredMap)

	return string(currentBytes) == string(desiredBytes)
}
