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

package v1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
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

// A PolicySpec defines the desired state of a Policy.
type PolicySpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       PolicyParameters `json:"forProvider"`
}

// A PolicyStatus represents the observed state of a Policy.
type PolicyStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          PolicyObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,backblaze}
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="POLICY NAME",type="string",JSONPath=".status.atProvider.policyName"

// A Policy represents a Backblaze B2 S3-compatible policy.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:",inline"`

	Spec   PolicySpec   `json:"spec"`
	Status PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyList contains a list of Policy
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []Policy `json:"items"`
}

// Policy type metadata.
var (
	PolicyKind             = reflect.TypeOf(Policy{}).Name()
	PolicyGroupKind        = schema.GroupKind{Group: Group, Kind: PolicyKind}
	PolicyKindAPIVersion   = PolicyKind + "." + SchemeGroupVersion.String()
	PolicyGroupVersionKind = SchemeGroupVersion.WithKind(PolicyKind)
)

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
}

// GetPolicyName returns the policy name from the Policy resource.
func (mg *Policy) GetPolicyName() string {
	if mg.Spec.ForProvider.PolicyName != nil {
		return *mg.Spec.ForProvider.PolicyName
	}
	return mg.GetName()
}