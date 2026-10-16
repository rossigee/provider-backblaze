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

// NotificationEvent represents a B2 bucket event that the notification rule
// subscribes to.
type NotificationEvent string

const (
	NotificationEventBucketCreated NotificationEvent = "b2:ObjectCreated"
	NotificationEventBucketDeleted NotificationEvent = "b2:ObjectDeleted"
	NotificationEventFileHidden    NotificationEvent = "b2:ObjectHidden"
	NotificationEventFileNotHide   NotificationEvent = "b2:ObjectNotHidden"
)

// NotificationParameters are the configurable fields of a
// BucketNotification (B2 event notification rule).
type NotificationParameters struct {
	// BucketID is the B2-assigned bucket ID. Set this OR BucketName; if both
	// are set the bucket is resolved by ID. B2's notification API addresses
	// buckets by ID, not name.
	// +optional
	BucketID *string `json:"bucketId,omitempty"`
	// BucketName is the bucket name; resolved via b2_list_buckets at reconcile
	// time if BucketID is not provided.
	// +optional
	BucketName *string `json:"bucketName,omitempty"`
	// Name is the friendly name of the notification rule (must be unique per
	// bucket).
	Name string `json:"name"`
	// Events is the set of B2 events to subscribe to.
	// +kubebuilder:validation:MinItems=1
	Events []NotificationEvent `json:"events"`
	// WebhookURL is the HTTPS endpoint delivered to. Must be reachable from
	// B2's egress.
	WebhookURL string `json:"webhookUrl"`
	// Description is an optional human-readable note (returned on Get but
	// primarily cosmetic).
	// +optional
	Description *string `json:"description,omitempty"`
	// Disabled suspends delivery without removing the rule.
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// NotificationObservation are the observable fields of a BucketNotification.
type NotificationObservation struct {
	NotificationID string              `json:"notificationId,omitempty"`
	BucketID       string              `json:"bucketId,omitempty"`
	Events         []NotificationEvent `json:"events,omitempty"`
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

// A BucketNotification represents a B2 bucket event notification rule.
type BucketNotification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NotificationSpec   `json:"spec"`
	Status            NotificationStatus `json:"status,omitempty"`
}

type NotificationSpec struct {
	xpv1.ManagedResourceSpec `json:",inline"`
	ForProvider              NotificationParameters `json:"forProvider"`
}

type NotificationStatus struct {
	xpv1.ManagedResourceStatus `json:",inline"`
	AtProvider                 NotificationObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// BucketNotificationList contains a list of BucketNotification.
type BucketNotificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BucketNotification `json:"items"`
}

// GetNotificationName returns the rule name from the spec.
func (mg *BucketNotification) GetNotificationName() string {
	return mg.Spec.ForProvider.Name
}
