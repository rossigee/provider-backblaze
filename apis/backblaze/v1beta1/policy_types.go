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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// PolicyParameters are the configurable fields of a Policy.
type PolicyParameters struct {
	// PolicyName is the name for this policy.
	// +optional
	PolicyName *string `json:"policyName,omitempty"`
	// Description provides a human-readable description of the policy.
	// +optional
	Description *string `json:"description,omitempty"`
	// AllowBucket creates a simple policy that allows all operations for the specified bucket.
	// This is mutually exclusive with RawPolicy.
	// +optional
	AllowBucket *string `json:"allowBucket,omitempty"`
	// RawPolicy contains the complete S3-compatible policy document as JSON.
	// This is mutually exclusive with AllowBucket.
	// +optional
	RawPolicy *string `json:"rawPolicy,omitempty"`
}

// PolicyObservation are the observable fields of a Policy.
type PolicyObservation struct {
	// PolicyName is the name of the policy.
	PolicyName string `json:"policyName,omitempty"`
	// PolicyDocument is the actual policy document stored.
	PolicyDocument string `json:"policyDocument,omitempty"`
	// PolicyID is the unique identifier for the policy (if applicable).
	PolicyID string `json:"policyId,omitempty"`
	// CreationTime is when the policy was created.
	CreationTime *metav1.Time `json:"creationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,backblaze}
// +genclient
// +genclient:namespaced
// +groupName=backblaze.m.crossplane.io

// A Policy represents a Backblaze B2 S3-compatible policy.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicySpec   `json:"spec"`
	Status            PolicyStatus `json:"status,omitempty"`
}

type PolicySpec struct {
	xpv1.ManagedResourceSpec `json:",inline"`
	ForProvider              PolicyParameters `json:"forProvider"`
}

// PolicyStatus represents the observed state of a Policy.
type PolicyStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             PolicyObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyList contains a list of Policy.
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Policy `json:"items"`
}
