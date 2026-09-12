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

// BucketDeletionPolicy represents the bucket deletion policy.
// +kubebuilder:validation:Enum=DeleteIfEmpty;DeleteAll
type BucketDeletionPolicy string

const (
	// DeleteIfEmpty deletes the bucket only if it's empty
	DeleteIfEmpty BucketDeletionPolicy = "DeleteIfEmpty"
	// DeleteAll deletes all objects in the bucket before deleting the bucket
	DeleteAll BucketDeletionPolicy = "DeleteAll"
)

// LifecycleRule defines automatic file lifecycle management.
type LifecycleRule struct {
	// FileNamePrefix limits the rule to files whose names start with this prefix.
	// +optional
	FileNamePrefix string `json:"fileNamePrefix,omitempty"`
	// DaysFromUploadingToHiding specifies how many days after uploading a file version it should be hidden.
	// +optional
	DaysFromUploadingToHiding *int `json:"daysFromUploadingToHiding,omitempty"`
	// DaysFromHidingToDeleting specifies how many days after hiding a file version it should be deleted.
	// +optional
	DaysFromHidingToDeleting *int `json:"daysFromHidingToDeleting,omitempty"`
}

// CORSRule defines CORS configuration for a bucket.
type CORSRule struct {
	// CorsRuleName is the name for this CORS rule.
	CorsRuleName string `json:"corsRuleName"`
	// AllowedOrigins specifies the allowed origins for CORS requests.
	AllowedOrigins []string `json:"allowedOrigins"`
	// AllowedMethods specifies the allowed HTTP methods.
	AllowedMethods []string `json:"allowedMethods"`
	// AllowedHeaders specifies the allowed headers.
	// +optional
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`
	// ExposeHeaders specifies headers that browsers are allowed to access.
	// +optional
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`
	// MaxAgeSeconds specifies how long browsers can cache preflight responses.
	// +optional
	MaxAgeSeconds *int `json:"maxAgeSeconds,omitempty"`
}

// BucketParameters are the configurable fields of a Bucket.
type BucketParameters struct {
	// BucketName is the name of the bucket. Must be globally unique.
	BucketName string `json:"bucketName"`
	// BucketType defines the access permissions for the bucket.
	// +kubebuilder:validation:Enum=allPublic;allPrivate
	// +kubebuilder:default=allPrivate
	BucketType string `json:"bucketType,omitempty"`
	// Region is the Backblaze B2 region where the bucket should be created.
	// +kubebuilder:default=us-west-001
	Region string `json:"region,omitempty"`
	// BucketDeletionPolicy defines how to handle bucket deletion.
	// +optional
	BucketDeletionPolicy BucketDeletionPolicy `json:"bucketDeletionPolicy,omitempty"`
	// LifecycleRules define automatic file lifecycle management.
	// +optional
	LifecycleRules []LifecycleRule `json:"lifecycleRules,omitempty"`
	// CorsRules define CORS configuration for the bucket.
	// +optional
	CorsRules []CORSRule `json:"corsRules,omitempty"`
}

// BucketObservation are the observable fields of a Bucket.
type BucketObservation struct {
	// BucketName is the name of the bucket.
	BucketName string `json:"bucketName,omitempty"`
	// BucketID is the unique identifier for the bucket.
	BucketID string `json:"bucketId,omitempty"`
	// AccountID is the account that owns the bucket.
	AccountID string `json:"accountId,omitempty"`
	// Region is the region where the bucket is located.
	Region string `json:"region,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,backblaze}
// +genclient
// +genclient:namespaced
// +groupName=backblaze.m.crossplane.io

// A Bucket is a managed resource representing a Backblaze B2 bucket.
type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BucketSpec   `json:"spec"`
	Status            BucketStatus `json:"status,omitempty"`
}

type BucketSpec struct {
	xpv1.ManagedResourceSpec `json:",inline"`
	ForProvider              BucketParameters `json:"forProvider"`
}

// BucketStatus represents the observed state of a Bucket.
type BucketStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BucketObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// BucketList contains a list of Bucket.
type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

// GetBucketName returns the bucket name from the Bucket resource.
func (mg *Bucket) GetBucketName() string {
	return mg.Spec.ForProvider.BucketName
}
