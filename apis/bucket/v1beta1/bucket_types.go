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
	"reflect"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// DeleteIfEmpty only deletes the bucket if the bucket is empty.
	DeleteIfEmpty BucketDeletionPolicy = "DeleteIfEmpty"
	// DeleteAll recursively deletes all objects in the bucket and then removes it.
	DeleteAll BucketDeletionPolicy = "DeleteAll"
)

// BucketDeletionPolicy determines how buckets should be deleted when a Bucket is deleted.
type BucketDeletionPolicy string

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="External Name",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Bucket Name",type="string",JSONPath=".status.atProvider.bucketName"
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.forProvider.region"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,backblaze}

type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec"`
	Status BucketStatus `json:"status,omitempty"`
}

type BucketSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       BucketParameters `json:"forProvider,omitempty"`
}

type BucketStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          BucketProviderStatus `json:"atProvider,omitempty"`
}

type BucketParameters struct {
	// BucketName is the name of the bucket to create.
	// Defaults to `metadata.name` if unset.
	// Cannot be changed after bucket is created.
	// Name must be acceptable by the S3 protocol, which follows RFC 1123.
	// Be aware that Backblaze B2 requires unique bucket names across the entire platform.
	BucketName string `json:"bucketName,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="us-west-001"

	// Region is the name of the region where the bucket shall be created.
	// The region must be available in the Backblaze B2 service.
	// Cannot be changed after bucket is created.
	// Common regions: us-west-001, us-west-002, eu-central-003
	Region string `json:"region,omitempty"`

	// BucketDeletionPolicy determines how buckets should be deleted when Bucket is deleted.
	//  `DeleteIfEmpty` only deletes the bucket if the bucket is empty.
	//  `DeleteAll` recursively deletes all objects in the bucket and then removes it.
	// To skip deletion of the bucket (orphan it) set `spec.deletionPolicy=Orphan`.
	BucketDeletionPolicy BucketDeletionPolicy `json:"bucketDeletionPolicy,omitempty"`

	// BucketType defines the access level for the bucket.
	// +kubebuilder:validation:Enum=allPrivate;allPublic
	// +kubebuilder:default="allPrivate"
	// allPrivate: Files in this bucket are private and require authorization to access
	// allPublic: Files in this bucket can be downloaded by anybody
	BucketType string `json:"bucketType,omitempty"`

	// LifecycleRules defines lifecycle rules for automatic file management.
	// This controls when files are automatically hidden or deleted.
	LifecycleRules []LifecycleRule `json:"lifecycleRules,omitempty"`

	// CorsRules defines Cross-Origin Resource Sharing rules for the bucket.
	CorsRules []CorsRule `json:"corsRules,omitempty"`
}

// LifecycleRule defines automatic file lifecycle management.
type LifecycleRule struct {
	// DaysFromHidingToDeleting specifies how many days after hiding a file version it should be deleted.
	DaysFromHidingToDeleting *int `json:"daysFromHidingToDeleting,omitempty"`

	// DaysFromUploadingToHiding specifies how many days after uploading a file version it should be hidden.
	DaysFromUploadingToHiding *int `json:"daysFromUploadingToHiding,omitempty"`

	// FileNamePrefix limits the rule to files whose names start with this prefix.
	FileNamePrefix string `json:"fileNamePrefix,omitempty"`
}

// CorsRule defines Cross-Origin Resource Sharing rules.
type CorsRule struct {
	// CorsRuleName is a name for this rule (for your reference).
	CorsRuleName string `json:"corsRuleName"`

	// AllowedOrigins lists the origins that are allowed to make requests.
	AllowedOrigins []string `json:"allowedOrigins"`

	// AllowedHeaders lists the headers that can be used in requests.
	AllowedHeaders []string `json:"allowedHeaders"`

	// AllowedOperations lists the operations that are allowed.
	// Valid values: b2_download_file_by_id, b2_download_file_by_name, b2_upload_file, b2_upload_part
	AllowedOperations []string `json:"allowedOperations"`

	// ExposeHeaders lists headers that browsers are allowed to access.
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`

	// MaxAgeSeconds specifies how long browsers can cache the CORS response.
	MaxAgeSeconds *int `json:"maxAgeSeconds,omitempty"`
}

type BucketProviderStatus struct {
	// BucketName is the name of the actual bucket.
	BucketName string `json:"bucketName,omitempty"`

	// BucketID is the unique identifier assigned by Backblaze B2.
	BucketID string `json:"bucketID,omitempty"`

	// BucketType reflects the current access level of the bucket.
	BucketType string `json:"bucketType,omitempty"`
}

// +kubebuilder:object:root=true

type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

// Dummy type metadata.
var (
	BucketKind             = reflect.TypeOf(Bucket{}).Name()
	BucketGroupKind        = schema.GroupKind{Group: Group, Kind: BucketKind}.String()
	BucketKindAPIVersion   = BucketKind + "." + SchemeGroupVersion.String()
	BucketGroupVersionKind = SchemeGroupVersion.WithKind(BucketKind)
)

// GetBucketName returns the spec.forProvider.bucketName if given, otherwise defaults to metadata.name.
func (in *Bucket) GetBucketName() string {
	if in.Spec.ForProvider.BucketName == "" {
		return in.Name
	}
	return in.Spec.ForProvider.BucketName
}

func init() {
	SchemeBuilder.Register(&Bucket{}, &BucketList{})
}