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
	r := &PolicyReconciler{
		Client: mgr.GetClient(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("policy-controller").
		For(&backblazev1.Policy{}).
		Watches(&apisv1beta1.ProviderConfig{}, handler.Funcs{}).
		Complete(r)
}

// PolicyReconciler reconciles a Policy object
type PolicyReconciler struct {
	Client client.Client
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PolicyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("policy", req.NamespacedName)

	// Fetch the Policy instance
	policy := &backblazev1.Policy{}
	err := r.Client.Get(ctx, req.NamespacedName, policy)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Object not found, return without error
			logger.Info("Policy resource not found, likely deleted")
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get Policy")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciling policy", "policyName", policy.GetPolicyName())

	// Check for deletion - in this simple implementation, we let Kubernetes handle deletion
	if !policy.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, policy)
	}

	// Get provider config and create client
	service, err := r.getBackblazeClient(ctx, policy)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client")
		r.setCondition(policy, xpv1.TypeReady, "False", "ClientError", err.Error())
		// Use shorter requeue time for ProviderConfig not found errors (likely cache sync issue)
		requeueAfter := time.Minute
		if strings.Contains(err.Error(), "not found") {
			requeueAfter = 10 * time.Second
		}
		return reconcile.Result{RequeueAfter: requeueAfter}, r.Client.Status().Update(ctx, policy)
	}

	// Check if policy already exists
	if policy.Status.AtProvider.PolicyName == "" {
		// Create policy
		if err := r.createPolicy(ctx, policy, service); err != nil {
			logger.Error(err, "Failed to create policy")
			r.setCondition(policy, xpv1.TypeReady, "False", "CreateError", err.Error())
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
		}
	}

	// Policy exists and is ready
	r.setCondition(policy, xpv1.TypeReady, "True", "Available", "Policy is available")
	r.setCondition(policy, xpv1.TypeSynced, "True", "ReconcileSuccess", "Successfully reconciled")

	logger.Info("Successfully reconciled policy")
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, r.Client.Status().Update(ctx, policy)
}

func (r *PolicyReconciler) handleDeletion(ctx context.Context, policy *backblazev1.Policy) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// For this implementation, we'll simulate policy deletion
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 policy deletion

	logger.Info("Policy deletion handled")
	return reconcile.Result{}, nil
}

func (r *PolicyReconciler) createPolicy(ctx context.Context, policy *backblazev1.Policy, service *clients.BackblazeClient) error {
	// Validate policy parameters
	params := policy.Spec.ForProvider
	if (params.AllowBucket != nil && params.RawPolicy != nil) ||
		(params.AllowBucket == nil && params.RawPolicy == nil) {
		return errors.New(errInvalidPolicyParams)
	}

	var policyDocument string
	var err error

	if params.AllowBucket != nil {
		// Generate simple policy for the bucket
		policyDocument, err = r.generateSimplePolicy(*params.AllowBucket)
		if err != nil {
			return errors.Wrap(err, errGenerateSimplePolicy)
		}
	} else {
		// Use raw policy document
		policyDocument = *params.RawPolicy
		// Validate it's valid JSON
		var temp interface{}
		if err := json.Unmarshal([]byte(policyDocument), &temp); err != nil {
			return errors.Wrap(err, errInvalidRawPolicy)
		}
	}

	// Get policy name
	policyName := policy.GetPolicyName()

	// For this implementation, we'll simulate policy creation
	// In a real implementation, you would use the Backblaze B2 API
	// TODO: Implement actual B2 policy creation

	// Update the resource status
	policy.Status.AtProvider.PolicyName = policyName
	policy.Status.AtProvider.PolicyDocument = policyDocument
	policy.Status.AtProvider.PolicyID = fmt.Sprintf("policy-%d", policy.GetGeneration())
	now := metav1.NewTime(time.Now())
	policy.Status.AtProvider.CreationTime = &now

	return nil
}

func (r *PolicyReconciler) getBackblazeClient(ctx context.Context, policy *backblazev1.Policy) (*clients.BackblazeClient, error) {
	// Determine ProviderConfig name - use "default" if not specified
	providerConfigName := "default"
	if policy.GetProviderConfigReference() != nil {
		providerConfigName = policy.GetProviderConfigReference().Name
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

// generateSimplePolicy creates a basic policy that allows all operations for a specific bucket
func (r *PolicyReconciler) generateSimplePolicy(bucketName string) (string, error) {
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

func (r *PolicyReconciler) setCondition(policy *backblazev1.Policy, conditionType xpv1.ConditionType, status, reason, message string) {
	policy.SetConditions(xpv1.Condition{
		Type:               conditionType,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            message,
	})
}