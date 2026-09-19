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

// ServerSideEncryption describes default encryption-at-rest applied to new
// files uploaded to the bucket. Backblaze B2 supports SSE-B2 only via
// `defaultServerSideEncryption: { mode: "SSE-B2", algorithm: "AES256" }`.
// SSE-C is exposed in the type for completeness but cannot be applied
// declaratively through b2_update_bucket.
type ServerSideEncryption struct {
	// Mode is "SSE-B2" (managed keys) or "SSE-C" (customer keys).
	// +kubebuilder:validation:Enum=SSE-B2;SSE-C
	Mode string `json:"mode"`
	// Algorithm is the encryption algorithm; "AES256" is the only supported
	// value for both modes.
	// +kubebuilder:validation:Enum=AES256
	// +optional
	Algorithm string `json:"algorithm,omitempty"`
}

// DefaultRetention describes the B2 file-lock default retention period.
// When set, every newly-uploaded file is governed by the configured retention
// automatically. To enable file lock on the bucket, set FileLockEnabled=true.
type DefaultRetention struct {
	// Mode is "governance" (overrideable) or "compliance" (not overrideable).
	// +kubebuilder:validation:Enum=governance;compliance
	Mode string `json:"mode"`
	// Period is the retention period in seconds.
	Period int `json:"period"`
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
	// BucketInfo maps free-form key/value metadata onto the bucket. B2 exposes
	// these via the bucketInfo field of b2_list_buckets. Use for cost-tracking
	// tags, environment labels, etc.
	// +optional
	BucketInfo map[string]string `json:"bucketInfo,omitempty"`
	// DefaultServerSideEncryption sets the default encryption-at-rest mode for
	// newly uploaded files. Cannot be unset once configured.
	// +optional
	DefaultServerSideEncryption *ServerSideEncryption `json:"defaultServerSideEncryption,omitempty"`
	// FileLockEnabled turns on B2 file lock for the bucket. Required before
	// DefaultRetention will be honoured.
	// +optional
	FileLockEnabled bool `json:"fileLockEnabled,omitempty"`
	// DefaultRetention sets the bucket-wide default retention period. Requires
	// FileLockEnabled=true.
	// +optional
	DefaultRetention *DefaultRetention `json:"defaultRetention,omitempty"`
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
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
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
	xpv1.ManagedResourceStatus `json:",inline"`
	AtProvider                 BucketObservation `json:"atProvider,omitempty"`
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
