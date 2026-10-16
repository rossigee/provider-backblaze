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
	"reflect"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"

	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	errNotPolicy             = "managed resource is not a Policy custom resource"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errGetProviderConfig     = "cannot get referenced ProviderConfig"
	errCreateBackblazeClient = "cannot create Backblaze client"
	errCreatePolicy          = "cannot create policy"
	errDeletePolicy          = "cannot delete policy"
	errGetPolicy             = "cannot get policy"
	errInvalidPolicyParams   = "invalid policy parameters: specify either allowBucket or rawPolicy, not both"
	errGenerateSimplePolicy  = "cannot generate simple policy document"
	errInvalidRawPolicy      = "invalid raw policy: must be valid JSON"
)

// SetupPolicy adds a controller that reconciles Policy managed resources.
func SetupPolicy(mgr ctrl.Manager, o controller.Options) error {
	r := &PolicyReconciler{
		Client: mgr.GetClient(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("policy-controller").
		For(&backblazev1beta1.Policy{}).
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
	policy := &backblazev1beta1.Policy{}
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

	// Check for deletion
	if !policy.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, policy)
	}

	// Get provider config and create client
	service, err := r.getBackblazeClient(ctx, policy)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client")
		r.setCondition(policy, xpv1.TypeReady, "False", "ClientError", err.Error())
		requeueAfter := time.Minute
		if strings.Contains(err.Error(), "not found") {
			requeueAfter = 10 * time.Second
		}
		return reconcile.Result{RequeueAfter: requeueAfter}, r.Client.Status().Update(ctx, policy)
	}

	// Validate the spec - exactly one of AllowBucket/RawPolicy must be set
	if err := validatePolicyParams(policy.Spec.ForProvider); err != nil {
		r.setCondition(policy, xpv1.TypeReady, "False", "InvalidSpec", err.Error())
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
	}

	// Resolve which bucket this policy attaches to. AllowBucket is the explicit
	// shorthand; otherwise the resource name / Spec.PolicyName is the bucket.
	bucketName := resolveBucketName(policy)
	if bucketName == "" {
		r.setCondition(policy, xpv1.TypeReady, "False", "InvalidSpec", "could not determine target bucket name")
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
	}

	// Verify the target bucket exists before we try to attach a policy to it.
	exists, err := service.BucketExists(ctx, bucketName)
	if err != nil {
		logger.Error(err, "Failed to check bucket existence")
		r.setCondition(policy, xpv1.TypeReady, "False", "CheckError", err.Error())
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
	}
	if !exists {
		logger.Info("Target bucket does not exist", "bucketName", bucketName)
		r.setCondition(policy, xpv1.TypeReady, "False", "BucketNotFound",
			fmt.Sprintf("target bucket %q does not exist in Backblaze B2", bucketName))
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
	}

	desiredDoc, err := renderPolicyDocument(policy.Spec.ForProvider)
	if err != nil {
		r.setCondition(policy, xpv1.TypeReady, "False", "InvalidSpec", err.Error())
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
	}

	// Observe current bucket policy.
	currentDoc, getErr := service.GetBucketPolicy(ctx, bucketName)
	hasPolicy := true
	if getErr != nil {
		if errors.Is(getErr, clients.ErrBucketPolicyNotFound) {
			hasPolicy = false
		} else {
			logger.Error(getErr, "Failed to read bucket policy")
			r.setCondition(policy, xpv1.TypeReady, "False", "ObserveError", getErr.Error())
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
		}
	}

	drift := !hasPolicy || !policyDocEqual(currentDoc, desiredDoc)
	if drift {
		if !shouldCreate(policy.GetManagementPolicies()) {
			logger.Info("Bucket policy drift detected but creation is disabled by managementPolicies",
				"bucketName", bucketName)
			r.setCondition(policy, xpv1.TypeReady, "False", "ObserveOnly",
				"Bucket policy drift detected and managementPolicies disallows creation")
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
		}
		if err := service.PutBucketPolicy(ctx, bucketName, desiredDoc); err != nil {
			logger.Error(err, "Failed to apply bucket policy", "bucketName", bucketName)
			r.setCondition(policy, xpv1.TypeReady, "False", "ApplyError", err.Error())
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, policy)
		}
		logger.Info("Applied bucket policy", "bucketName", bucketName, "created", !hasPolicy)
	}

	meta.SetExternalName(policy, bucketName)
	policy.Status.AtProvider.PolicyName = bucketName
	policy.Status.AtProvider.PolicyDocument = desiredDoc
	policy.Status.AtProvider.PolicyID = bucketName
	now := metav1.NewTime(time.Now())
	if policy.Status.AtProvider.CreationTime == nil {
		policy.Status.AtProvider.CreationTime = &now
	}

	r.setCondition(policy, xpv1.TypeReady, "True", "Available", "Bucket policy is applied")
	r.setCondition(policy, xpv1.TypeSynced, "True", "ReconcileSuccess", "Successfully reconciled")

	logger.Info("Successfully reconciled policy", "bucketName", bucketName, "drift", drift)
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, r.Client.Status().Update(ctx, policy)
}

func (r *PolicyReconciler) handleDeletion(ctx context.Context, policy *backblazev1beta1.Policy) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Respect managementPolicies - if Observe only, don't delete external resource
	if !shouldDelete(policy.GetManagementPolicies()) {
		logger.Info("Skipping policy deletion due to managementPolicies", "managementPolicies", policy.GetManagementPolicies())
		logger.Info("Policy deletion handled (Observe-only, external resource preserved)")
		return reconcile.Result{}, nil
	}

	bucketName := resolveBucketName(policy)
	if bucketName == "" {
		logger.Info("Policy has no resolvable bucket name; nothing to delete externally")
		return reconcile.Result{}, nil
	}

	service, err := r.getBackblazeClient(ctx, policy)
	if err != nil {
		// ProviderConfig may already be gone; allow Kubernetes GC to proceed.
		logger.Error(err, "Failed to create Backblaze client for deletion")
		return reconcile.Result{}, nil
	}

	if err := service.DeleteBucketPolicy(ctx, bucketName); err != nil {
		if errors.Is(err, clients.ErrBucketPolicyNotFound) {
			logger.Info("Bucket policy already absent in B2", "bucketName", bucketName)
			return reconcile.Result{}, nil
		}
		// Don't block Kubernetes deletion - log and continue.
		logger.Error(err, "Failed to delete bucket policy from B2", "bucketName", bucketName)
		return reconcile.Result{}, nil
	}

	logger.Info("Successfully deleted bucket policy", "bucketName", bucketName)
	return reconcile.Result{}, nil
}

func (r *PolicyReconciler) getBackblazeClient(ctx context.Context, policy *backblazev1beta1.Policy) (*clients.BackblazeClient, error) {
	// Crossplane v2 sets a kubebuilder default of {"kind":"ClusterProviderConfig","name":"default"}.
	// We always honour the referenced name; if it's empty we treat it as "default".
	providerConfigName := "default"
	if ref := policy.GetProviderConfigReference(); ref != nil && ref.Name != "" {
		providerConfigName = ref.Name
	}

	pc := &apisv1beta1.ProviderConfig{}
	// ProviderConfigs are namespaced resources - look in the same namespace as the provider
	key := client.ObjectKey{Name: providerConfigName, Namespace: "crossplane-system"}
	if err := r.Client.Get(ctx, key, pc); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, errors.Wrap(err, errGetProviderConfig)
		}
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	cfg, err := clients.GetProviderConfig(ctx, r.Client, pc)
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	return clients.NewBackblazeClient(*cfg)
}

// resolveBucketName picks the target bucket name for the policy. AllowBucket
// (the simple-mode shorthand) wins; otherwise we fall back to the explicit
// Spec.PolicyName or the resource's metadata name.
func resolveBucketName(p *backblazev1beta1.Policy) string {
	if p.Spec.ForProvider.AllowBucket != nil && *p.Spec.ForProvider.AllowBucket != "" {
		return *p.Spec.ForProvider.AllowBucket
	}
	return p.GetPolicyName()
}

// renderPolicyDocument validates the parameters and returns the desired
// bucket policy JSON document.
func renderPolicyDocument(params backblazev1beta1.PolicyParameters) (string, error) {
	switch {
	case params.AllowBucket != nil && params.RawPolicy != nil,
		params.AllowBucket == nil && params.RawPolicy == nil:
		return "", errors.New(errInvalidPolicyParams)
	case params.AllowBucket != nil:
		return generateSimplePolicy(*params.AllowBucket)
	default:
		doc := *params.RawPolicy
		var v interface{}
		if err := json.Unmarshal([]byte(doc), &v); err != nil {
			return "", errors.Wrap(err, errInvalidRawPolicy)
		}
		return doc, nil
	}
}

// validatePolicyParams returns an error if the parameters are not usable.
func validatePolicyParams(params backblazev1beta1.PolicyParameters) error {
	if (params.AllowBucket != nil && params.RawPolicy != nil) ||
		(params.AllowBucket == nil && params.RawPolicy == nil) {
		return errors.New(errInvalidPolicyParams)
	}
	return nil
}

// policyDocEqual compares two policy JSON documents semantically by parsing
// them into generic structures and comparing with reflect.DeepEqual. This makes
// the drift check robust against whitespace and key-order differences coming
// back from the B2 API.
func policyDocEqual(a, b string) bool {
	if a == b {
		return true
	}
	var av, bv interface{}
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// generateSimplePolicy creates a basic policy that allows all operations for a specific bucket.
func generateSimplePolicy(bucketName string) (string, error) {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{"s3:*"},
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

// generateSimplePolicy is also exposed as a method so tests can call it as
// `r.generateSimplePolicy(...)`.
func (r *PolicyReconciler) generateSimplePolicy(bucketName string) (string, error) {
	return generateSimplePolicy(bucketName)
}

func (r *PolicyReconciler) setCondition(policy *backblazev1beta1.Policy, conditionType xpv1.ConditionType, status, reason, message string) {
	policy.SetConditions(xpv1.Condition{
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
