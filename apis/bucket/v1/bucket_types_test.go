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
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBucketGetBucketName(t *testing.T) {
	tests := []struct {
		name         string
		bucket       *Bucket
		expectedName string
	}{
		{
			name: "spec bucket name set",
			bucket: &Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "resource-name",
					Annotations: map[string]string{
						meta.AnnotationKeyExternalName: "external-bucket-name",
					},
				},
				Spec: BucketSpec{
					ForProvider: BucketParameters{
						BucketName: "spec-bucket-name",
					},
				},
			},
			expectedName: "spec-bucket-name", // GetBucketName returns spec name, not external name
		},
		{
			name: "no external name, use spec",
			bucket: &Bucket{
				Spec: BucketSpec{
					ForProvider: BucketParameters{
						BucketName: "spec-bucket-name",
					},
				},
			},
			expectedName: "spec-bucket-name",
		},
		{
			name: "no names set",
			bucket: &Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "resource-name",
				},
			},
			expectedName: "resource-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.bucket.GetBucketName()
			if got != tt.expectedName {
				t.Errorf("GetBucketName() = %v, want %v", got, tt.expectedName)
			}
		})
	}
}

func TestBucketDeletionPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  BucketDeletionPolicy
		isValid bool
	}{
		{
			name:    "DeleteIfEmpty is valid",
			policy:  DeleteIfEmpty,
			isValid: true,
		},
		{
			name:    "DeleteAll is valid",
			policy:  DeleteAll,
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the policy constants are properly defined
			if tt.policy == "" && tt.isValid {
				t.Errorf("Policy should not be empty for valid policy")
			}
		})
	}
}

func TestLifecycleRule(t *testing.T) {
	days30 := 30
	days90 := 90
	rule := LifecycleRule{
		FileNamePrefix:            "logs/",
		DaysFromUploadingToHiding: &days30,
		DaysFromHidingToDeleting:  &days90,
	}

	if rule.FileNamePrefix != "logs/" {
		t.Errorf("FileNamePrefix = %v, want logs/", rule.FileNamePrefix)
	}
	if rule.DaysFromUploadingToHiding == nil || *rule.DaysFromUploadingToHiding != 30 {
		t.Errorf("DaysFromUploadingToHiding = %v, want 30", rule.DaysFromUploadingToHiding)
	}
	if rule.DaysFromHidingToDeleting == nil || *rule.DaysFromHidingToDeleting != 90 {
		t.Errorf("DaysFromHidingToDeleting = %v, want 90", rule.DaysFromHidingToDeleting)
	}
}

func TestCorsRule(t *testing.T) {
	maxAge := 3600
	rule := CorsRule{
		CorsRuleName:      "test-cors",
		AllowedOrigins:    []string{"https://example.com"},
		AllowedOperations: []string{"b2_download_file_by_name", "b2_upload_file"},
		AllowedHeaders:    []string{"Content-Type"},
		ExposeHeaders:     []string{"ETag"},
		MaxAgeSeconds:     &maxAge,
	}

	if rule.CorsRuleName != "test-cors" {
		t.Errorf("CorsRuleName = %v, want test-cors", rule.CorsRuleName)
	}
	if len(rule.AllowedOrigins) != 1 || rule.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("AllowedOrigins = %v, want [https://example.com]", rule.AllowedOrigins)
	}
	if rule.MaxAgeSeconds == nil || *rule.MaxAgeSeconds != 3600 {
		t.Errorf("MaxAgeSeconds = %v, want 3600", rule.MaxAgeSeconds)
	}
}

func TestBucketSpec(t *testing.T) {
	days30 := 30
	days90 := 90
	maxAge := 3600

	spec := BucketSpec{
		ForProvider: BucketParameters{
			BucketName:           "test-bucket",
			Region:               "us-west-001",
			BucketType:           "allPrivate",
			BucketDeletionPolicy: DeleteIfEmpty,
			LifecycleRules: []LifecycleRule{
				{
					FileNamePrefix:               "logs/",
					DaysFromUploadingToHiding:    &days30,
					DaysFromHidingToDeleting:     &days90,
				},
			},
			CorsRules: []CorsRule{
				{
					CorsRuleName:      "web-cors",
					AllowedOrigins:    []string{"https://example.com", "https://app.example.com"},
					AllowedOperations: []string{"b2_download_file_by_name"},
					AllowedHeaders:    []string{"Content-Type", "Authorization"},
					ExposeHeaders:     []string{"ETag", "Content-Length"},
					MaxAgeSeconds:     &maxAge,
				},
			},
		},
	}

	// Test basic fields
	if spec.ForProvider.BucketName != "test-bucket" {
		t.Errorf("BucketName = %v, want test-bucket", spec.ForProvider.BucketName)
	}
	if spec.ForProvider.Region != "us-west-001" {
		t.Errorf("Region = %v, want us-west-001", spec.ForProvider.Region)
	}
	if spec.ForProvider.BucketType != "allPrivate" {
		t.Errorf("BucketType = %v, want allPrivate", spec.ForProvider.BucketType)
	}
	if spec.ForProvider.BucketDeletionPolicy != DeleteIfEmpty {
		t.Errorf("BucketDeletionPolicy = %v, want %v", spec.ForProvider.BucketDeletionPolicy, DeleteIfEmpty)
	}

	// Test lifecycle rules
	if len(spec.ForProvider.LifecycleRules) != 1 {
		t.Errorf("Expected 1 lifecycle rule, got %d", len(spec.ForProvider.LifecycleRules))
	} else {
		rule := spec.ForProvider.LifecycleRules[0]
		if rule.FileNamePrefix != "logs/" {
			t.Errorf("FileNamePrefix = %v, want logs/", rule.FileNamePrefix)
		}
		if rule.DaysFromUploadingToHiding == nil || *rule.DaysFromUploadingToHiding != 30 {
			t.Errorf("DaysFromUploadingToHiding = %v, want 30", rule.DaysFromUploadingToHiding)
		}
	}

	// Test CORS rules
	if len(spec.ForProvider.CorsRules) != 1 {
		t.Errorf("Expected 1 CORS rule, got %d", len(spec.ForProvider.CorsRules))
	} else {
		rule := spec.ForProvider.CorsRules[0]
		if rule.CorsRuleName != "web-cors" {
			t.Errorf("CorsRuleName = %v, want web-cors", rule.CorsRuleName)
		}
		if len(rule.AllowedOrigins) != 2 {
			t.Errorf("Expected 2 allowed origins, got %d", len(rule.AllowedOrigins))
		}
	}
}

func TestBucketStatus(t *testing.T) {
	status := BucketStatus{
		AtProvider: BucketProviderStatus{
			BucketName: "actual-bucket-name",
			BucketID:   "b2-bucket-id-12345",
			BucketType: "allPrivate",
		},
	}

	if status.AtProvider.BucketName != "actual-bucket-name" {
		t.Errorf("BucketName = %v, want actual-bucket-name", status.AtProvider.BucketName)
	}
	if status.AtProvider.BucketID != "b2-bucket-id-12345" {
		t.Errorf("BucketID = %v, want b2-bucket-id-12345", status.AtProvider.BucketID)
	}
	if status.AtProvider.BucketType != "allPrivate" {
		t.Errorf("BucketType = %v, want allPrivate", status.AtProvider.BucketType)
	}
}

func TestBucketList(t *testing.T) {
	bucketList := BucketList{
		Items: []Bucket{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bucket-1",
				},
				Spec: BucketSpec{
					ForProvider: BucketParameters{
						BucketName: "bucket-1",
						Region:     "us-west-001",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bucket-2",
				},
				Spec: BucketSpec{
					ForProvider: BucketParameters{
						BucketName: "bucket-2",
						Region:     "eu-central-003",
					},
				},
			},
		},
	}

	if len(bucketList.Items) != 2 {
		t.Errorf("Expected 2 buckets, got %d", len(bucketList.Items))
	}

	for i, bucket := range bucketList.Items {
		expectedName := bucket.Spec.ForProvider.BucketName
		if bucket.GetBucketName() != expectedName {
			t.Errorf("Bucket %d: GetBucketName() = %v, want %v", i, bucket.GetBucketName(), expectedName)
		}
	}
}

func TestBucketGroupVersionKind(t *testing.T) {
	// Test that the group version kind constants are properly set
	expectedKind := "Bucket"
	if BucketKind != expectedKind {
		t.Errorf("BucketKind = %v, want %v", BucketKind, expectedKind)
	}

	if BucketGroupVersionKind.Kind != expectedKind {
		t.Errorf("BucketGroupVersionKind.Kind = %v, want %v", BucketGroupVersionKind.Kind, expectedKind)
	}

	if BucketGroupVersionKind.Group != Group {
		t.Errorf("BucketGroupVersionKind.Group = %v, want %v", BucketGroupVersionKind.Group, Group)
	}
}
