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

package bucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"

	providerapis "github.com/rossigee/provider-backblaze/apis"
	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	clients "github.com/rossigee/provider-backblaze/internal/clients"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// BackblazeClientInterface defines the interface for Backblaze client operations
type BackblazeClientInterface interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	CreateBucket(ctx context.Context, bucketName, bucketType, region string) error
	DeleteBucket(ctx context.Context, bucketName string) error
	GetBucketLocation(ctx context.Context, bucketName string) (string, error)
	DeleteAllObjectsInBucket(ctx context.Context, bucketName string) error
}

// MockBackblazeClient implements a mock for testing
type MockBackblazeClient struct {
	bucketExists             func(ctx context.Context, bucketName string) (bool, error)
	createBucket             func(ctx context.Context, bucketName, bucketType, region string) error
	deleteBucket             func(ctx context.Context, bucketName string) error
	getBucketLocation        func(ctx context.Context, bucketName string) (string, error)
	deleteAllObjectsInBucket func(ctx context.Context, bucketName string) error
}

func (m *MockBackblazeClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	if m.bucketExists != nil {
		return m.bucketExists(ctx, bucketName)
	}
	return false, nil
}

func (m *MockBackblazeClient) CreateBucket(ctx context.Context, bucketName, bucketType, region string) error {
	if m.createBucket != nil {
		return m.createBucket(ctx, bucketName, bucketType, region)
	}
	return nil
}

func (m *MockBackblazeClient) DeleteBucket(ctx context.Context, bucketName string) error {
	if m.deleteBucket != nil {
		return m.deleteBucket(ctx, bucketName)
	}
	return nil
}

func (m *MockBackblazeClient) GetBucketLocation(ctx context.Context, bucketName string) (string, error) {
	if m.getBucketLocation != nil {
		return m.getBucketLocation(ctx, bucketName)
	}
	return "us-west-001", nil
}

func (m *MockBackblazeClient) DeleteAllObjectsInBucket(ctx context.Context, bucketName string) error {
	if m.deleteAllObjectsInBucket != nil {
		return m.deleteAllObjectsInBucket(ctx, bucketName)
	}
	return nil
}

// testExternal is a version of external that uses the interface for testing
type testExternal struct {
	service BackblazeClientInterface
}

func (c *testExternal) Observe(ctx context.Context, mg interface{}) (interface{}, error) {
	cr, ok := mg.(*backblazev1beta1.Bucket)
	if !ok {
		return nil, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()

	exists, err := c.service.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, errors.Wrap(err, errObserveBucket)
	}

	if !exists {
		return struct{ ResourceExists, ResourceUpToDate bool }{
			ResourceExists:   false,
			ResourceUpToDate: false,
		}, nil
	}

	// Update status with current state
	cr.Status.AtProvider.BucketName = bucketName

	// Get bucket location/region
	location, err := c.service.GetBucketLocation(ctx, bucketName)
	if err != nil {
		// Don't fail observation if we can't get location
		location = cr.Spec.ForProvider.Region
	}

	// Check if the bucket configuration matches desired state
	upToDate := location == "" || location == cr.Spec.ForProvider.Region

	return struct{ ResourceExists, ResourceUpToDate bool }{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *testExternal) Create(ctx context.Context, mg interface{}) (interface{}, error) {
	cr, ok := mg.(*backblazev1beta1.Bucket)
	if !ok {
		return nil, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()
	bucketType := cr.Spec.ForProvider.BucketType
	if bucketType == "" {
		bucketType = "allPrivate"
	}
	region := cr.Spec.ForProvider.Region

	err := c.service.CreateBucket(ctx, bucketName, bucketType, region)
	if err != nil {
		return nil, errors.Wrap(err, errCreateBucket)
	}

	return struct{}{}, nil
}

func (c *testExternal) Delete(ctx context.Context, mg interface{}) (interface{}, error) {
	cr, ok := mg.(*backblazev1beta1.Bucket)
	if !ok {
		return nil, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()

	// Handle deletion policy
	if cr.Spec.ForProvider.BucketDeletionPolicy == backblazev1beta1.DeleteAll {
		// Delete all objects first
		if err := c.service.DeleteAllObjectsInBucket(ctx, bucketName); err != nil {
			return nil, errors.Wrap(err, "cannot delete objects in bucket")
		}
	}

	// Delete the bucket
	err := c.service.DeleteBucket(ctx, bucketName)
	if err != nil {
		return nil, errors.Wrap(err, errDeleteBucket)
	}

	return struct{}{}, nil
}

func (c *testExternal) Update(ctx context.Context, mg interface{}) (interface{}, error) {
	_, ok := mg.(*backblazev1beta1.Bucket)
	if !ok {
		return nil, errors.New(errNotBucket)
	}

	// Most bucket properties cannot be updated after creation in Backblaze B2
	// This method exists to satisfy the interface but may not perform actual updates
	// for properties that cannot be changed.

	return struct{}{}, nil
}

func (c *testExternal) Disconnect(ctx context.Context) error {
	// No special disconnect logic needed for Backblaze B2 client
	return nil
}

func TestExternalObserve(t *testing.T) {
	tests := []struct {
		name           string
		bucket         *backblazev1beta1.Bucket
		mockBehavior   func(*MockBackblazeClient)
		expectedExists bool
		expectedError  bool
	}{
		{
			name: "bucket exists and up to date",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.bucketExists = func(ctx context.Context, bucketName string) (bool, error) {
					return true, nil
				}
				m.getBucketLocation = func(ctx context.Context, bucketName string) (string, error) {
					return "us-west-001", nil
				}
			},
			expectedExists: true,
			expectedError:  false,
		},
		{
			name: "bucket does not exist",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.bucketExists = func(ctx context.Context, bucketName string) (bool, error) {
					return false, nil
				}
			},
			expectedExists: false,
			expectedError:  false,
		},
		{
			name: "error checking bucket existence",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.bucketExists = func(ctx context.Context, bucketName string) (bool, error) {
					return false, errors.New("API error")
				}
			},
			expectedExists: false,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockBackblazeClient{}
			tt.mockBehavior(mockClient)

			external := &testExternal{
				service: mockClient,
			}

			observationRaw, err := external.Observe(context.Background(), tt.bucket)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			observation, ok := observationRaw.(struct{ ResourceExists, ResourceUpToDate bool })
			if !ok {
				t.Errorf("Unexpected observation type: %T", observationRaw)
				return
			}
			if observation.ResourceExists != tt.expectedExists {
				t.Errorf("Expected ResourceExists=%v, got %v",
					tt.expectedExists, observation.ResourceExists)
			}
		})
	}
}

func TestExternalCreate(t *testing.T) {
	tests := []struct {
		name          string
		bucket        *backblazev1beta1.Bucket
		mockBehavior  func(*MockBackblazeClient)
		expectedError bool
	}{
		{
			name: "successful creation",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						BucketType: "allPrivate",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.createBucket = func(ctx context.Context, bucketName, bucketType, region string) error {
					if bucketName != "test-bucket" {
						return errors.New("wrong bucket name")
					}
					if bucketType != "allPrivate" {
						return errors.New("wrong bucket type")
					}
					if region != "us-west-001" {
						return errors.New("wrong region")
					}
					return nil
				}
			},
			expectedError: false,
		},
		{
			name: "creation fails",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.createBucket = func(ctx context.Context, bucketName, bucketType, region string) error {
					return errors.New("creation failed")
				}
			},
			expectedError: true,
		},
		{
			name: "default bucket type",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
						// No bucket type specified
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.createBucket = func(ctx context.Context, bucketName, bucketType, region string) error {
					if bucketType != "allPrivate" {
						return errors.Errorf("expected default bucket type 'allPrivate', got %v", bucketType)
					}
					return nil
				}
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockBackblazeClient{}
			tt.mockBehavior(mockClient)

			external := &testExternal{
				service: mockClient,
			}

			_, err := external.Create(context.Background(), tt.bucket)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestExternalDelete(t *testing.T) {
	tests := []struct {
		name          string
		bucket        *backblazev1beta1.Bucket
		mockBehavior  func(*MockBackblazeClient)
		expectedError bool
	}{
		{
			name: "successful deletion without objects",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.deleteBucket = func(ctx context.Context, bucketName string) error {
					if bucketName != "test-bucket" {
						return errors.New("wrong bucket name")
					}
					return nil
				}
			},
			expectedError: false,
		},
		{
			name: "successful deletion with objects (DeleteAll policy)",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName:           "test-bucket",
						Region:               "us-west-001",
						BucketDeletionPolicy: backblazev1beta1.DeleteAll,
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.deleteAllObjectsInBucket = func(ctx context.Context, bucketName string) error {
					if bucketName != "test-bucket" {
						return errors.New("wrong bucket name")
					}
					return nil
				}
				m.deleteBucket = func(ctx context.Context, bucketName string) error {
					return nil
				}
			},
			expectedError: false,
		},
		{
			name: "deletion fails",
			bucket: &backblazev1beta1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: backblazev1beta1.BucketSpec{
					ForProvider: backblazev1beta1.BucketParameters{
						BucketName: "test-bucket",
						Region:     "us-west-001",
					},
				},
			},
			mockBehavior: func(m *MockBackblazeClient) {
				m.deleteBucket = func(ctx context.Context, bucketName string) error {
					return errors.New("deletion failed")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockBackblazeClient{}
			tt.mockBehavior(mockClient)

			external := &testExternal{
				service: mockClient,
			}

			_, err := external.Delete(context.Background(), tt.bucket)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestExternalUpdate(t *testing.T) {
	bucket := &backblazev1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-bucket",
		},
		Spec: backblazev1beta1.BucketSpec{
			ForProvider: backblazev1beta1.BucketParameters{
				BucketName: "test-bucket",
				Region:     "us-west-001",
			},
		},
	}

	mockClient := &MockBackblazeClient{}
	external := &testExternal{
		service: mockClient,
	}

	// Update should succeed but do nothing (Backblaze B2 doesn't support many update operations)
	_, err := external.Update(context.Background(), bucket)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestExternalDisconnect(t *testing.T) {
	mockClient := &MockBackblazeClient{}
	external := &testExternal{
		service: mockClient,
	}

	// Disconnect should succeed
	err := external.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestObserveWithWrongType(t *testing.T) {
	mockClient := &MockBackblazeClient{}
	external := &testExternal{
		service: mockClient,
	}

	// Pass wrong type (simple string instead of bucket)
	wrongType := "not-a-bucket"

	_, err := external.Observe(context.Background(), wrongType)
	if err == nil {
		t.Error("Expected error when passing wrong type")
	}
	if err.Error() != errNotBucket {
		t.Errorf("Expected error %q, got %q", errNotBucket, err.Error())
	}
}

// Pure-function tests for the drift comparators and converters added in this
// session. These run without any external dependency (no S3 client, no fake k8s
// client) so they're cheap to add and easy to extend.

func TestNativeLifecycleRulesEqual(t *testing.T) {
	one, two := 1, 2
	cases := []struct {
		name string
		a, b []clients.B2NativeLifecycleRule
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []clients.B2NativeLifecycleRule{}, []clients.B2NativeLifecycleRule{}, true},
		{
			"identical order",
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromHidingToDeleting: &one}},
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromHidingToDeleting: &one}},
			true,
		},
		{
			"different order, same content",
			[]clients.B2NativeLifecycleRule{
				{FileNamePrefix: "b/"},
				{FileNamePrefix: "a/", DaysFromHidingToDeleting: &one},
			},
			[]clients.B2NativeLifecycleRule{
				{FileNamePrefix: "a/", DaysFromHidingToDeleting: &one},
				{FileNamePrefix: "b/"},
			},
			true,
		},
		{
			"different prefix count",
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/"}},
			[]clients.B2NativeLifecycleRule{
				{FileNamePrefix: "a/"},
				{FileNamePrefix: "b/"},
			},
			false,
		},
		{
			"different hiding days",
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromUploadingToHiding: &one}},
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromUploadingToHiding: &two}},
			false,
		},
		{
			"different deleting days",
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromHidingToDeleting: &one}},
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromHidingToDeleting: &two}},
			false,
		},
		{
			"nil vs present",
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/"}},
			[]clients.B2NativeLifecycleRule{{FileNamePrefix: "a/", DaysFromHidingToDeleting: &one}},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeLifecycleRulesEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestConvertNativeLifecycleRules(t *testing.T) {
	one := 1
	in := []backblazev1beta1.LifecycleRule{
		{FileNamePrefix: "logs/", DaysFromUploadingToHiding: &one},
	}
	out := convertNativeLifecycleRules(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out))
	}
	if out[0].FileNamePrefix != "logs/" {
		t.Errorf("prefix = %q", out[0].FileNamePrefix)
	}
	if out[0].DaysFromUploadingToHiding == nil || *out[0].DaysFromUploadingToHiding != 1 {
		t.Errorf("hiding days = %v", out[0].DaysFromUploadingToHiding)
	}
}

func TestConvertNativeLifecycleRules_NilAndEmpty(t *testing.T) {
	if out := convertNativeLifecycleRules(nil); out != nil {
		t.Errorf("expected nil for nil input, got %v", out)
	}
	if out := convertNativeLifecycleRules([]backblazev1beta1.LifecycleRule{}); out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
}

func TestCorsRulesEqual(t *testing.T) {
	ma := int32(3600)
	mb := int32(7200)
	cases := []struct {
		name string
		a, b []types.CORSRule
		want bool
	}{
		{
			"identical",
			[]types.CORSRule{{ID: aws.String("r1"), AllowedOrigins: []string{"https://a"}, AllowedMethods: []string{"GET"}}},
			[]types.CORSRule{{ID: aws.String("r1"), AllowedOrigins: []string{"https://a"}, AllowedMethods: []string{"GET"}}},
			true,
		},
		{
			"origin order irrelevant",
			[]types.CORSRule{{ID: aws.String("r1"), AllowedOrigins: []string{"https://a", "https://b"}, AllowedMethods: []string{"GET"}}},
			[]types.CORSRule{{ID: aws.String("r1"), AllowedOrigins: []string{"https://b", "https://a"}, AllowedMethods: []string{"GET"}}},
			true,
		},
		{
			"different id",
			[]types.CORSRule{{ID: aws.String("r1")}},
			[]types.CORSRule{{ID: aws.String("r2")}},
			false,
		},
		{
			"different MaxAgeSeconds",
			[]types.CORSRule{{ID: aws.String("r1"), MaxAgeSeconds: &ma}},
			[]types.CORSRule{{ID: aws.String("r1"), MaxAgeSeconds: &mb}},
			false,
		},
		{
			"different ExposeHeaders",
			[]types.CORSRule{{ID: aws.String("r1"), ExposeHeaders: []string{"a"}}},
			[]types.CORSRule{{ID: aws.String("r1"), ExposeHeaders: []string{"b"}}},
			false,
		},
		{
			"count mismatch",
			[]types.CORSRule{{ID: aws.String("r1")}},
			[]types.CORSRule{{ID: aws.String("r1")}, {ID: aws.String("r2")}},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := corsRulesEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestConvertCorsRules(t *testing.T) {
	ma := 3600
	in := []backblazev1beta1.CORSRule{
		{
			CorsRuleName:   "AllowWeb",
			AllowedOrigins: []string{"https://example.com"},
			AllowedMethods: []string{"GET", "POST"},
			AllowedHeaders: []string{"Authorization"},
			ExposeHeaders:  []string{"X-Request-Id"},
			MaxAgeSeconds:  &ma,
		},
	}
	out := convertCorsRules(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 CORS rule, got %d", len(out))
	}
	r := out[0]
	if r.ID == nil || *r.ID != "AllowWeb" {
		t.Errorf("ID = %v", r.ID)
	}
	if len(r.AllowedOrigins) != 1 || r.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("AllowedOrigins = %v", r.AllowedOrigins)
	}
	if r.MaxAgeSeconds == nil || *r.MaxAgeSeconds != 3600 {
		t.Errorf("MaxAgeSeconds = %v", r.MaxAgeSeconds)
	}
	if r.AllowedHeaders[0] != "Authorization" || r.ExposeHeaders[0] != "X-Request-Id" {
		t.Errorf("headers not round-tripped: %v / %v", r.AllowedHeaders, r.ExposeHeaders)
	}
}

func TestStringMapsEqual(t *testing.T) {
	must := func(m clients.B2BucketInfo) clients.B2BucketInfo { return m }
	cases := []struct {
		name string
		a, b clients.B2BucketInfo
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", must(clients.B2BucketInfo{}), must(clients.B2BucketInfo{}), true},
		{"equal one entry", must(map[string]string{"k": "v"}), must(map[string]string{"k": "v"}), true},
		{"different value", must(map[string]string{"k": "v"}), must(map[string]string{"k": "w"}), false},
		{"missing key", must(map[string]string{"k": "v"}), must(map[string]string{"k": "v", "k2": "v2"}), false},
		{"extra key", must(map[string]string{"k": "v", "k2": "v2"}), must(map[string]string{"k": "v"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringMapsEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSseEqual(t *testing.T) {
	modeB2 := "SSE-B2"
	modeC := "SSE-C"
	cases := []struct {
		name string
		a, b *clients.B2NativeSSESettings
		want bool
	}{
		{"both nil", nil, nil, true},
		{"only a nil", &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES256"}, nil, false},
		{"only b nil", nil, &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES256"}, false},
		{"same mode+algo different ptrs", &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES256"}, &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES256"}, true},
		{"different mode", &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES256"}, &clients.B2NativeSSESettings{Mode: modeC, Algorithm: "AES256"}, false},
		{"different algo", &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES256"}, &clients.B2NativeSSESettings{Mode: modeB2, Algorithm: "AES128"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sseEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestRetentionEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b *clients.B2FileLockRetention
		want bool
	}{
		{"both nil", nil, nil, true},
		{"only a nil", &clients.B2FileLockRetention{Mode: "governance", Period: 3600}, nil, false},
		{"only b nil", nil, &clients.B2FileLockRetention{Mode: "governance", Period: 3600}, false},
		{"same value different ptrs", &clients.B2FileLockRetention{Mode: "governance", Period: 3600},
			&clients.B2FileLockRetention{Mode: "governance", Period: 3600}, true},
		{"different period", &clients.B2FileLockRetention{Mode: "governance", Period: 3600},
			&clients.B2FileLockRetention{Mode: "governance", Period: 86400}, false},
		{"different mode", &clients.B2FileLockRetention{Mode: "governance", Period: 3600},
			&clients.B2FileLockRetention{Mode: "compliance", Period: 3600}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retentionEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEqualIntPtr(t *testing.T) {
	one, two := 1, 2
	var nilInt *int
	if !equalIntPtr(&one, &one) {
		t.Errorf("&1 vs &1 should be equal")
	}
	if equalIntPtr(&one, &two) {
		t.Errorf("&1 vs &2 should not be equal")
	}
	if equalIntPtr(nil, &one) {
		t.Errorf("nil vs &1 should not be equal")
	}
	if equalIntPtr(&one, nil) {
		t.Errorf("&1 vs nil should not be equal")
	}
	if !equalIntPtr(nilInt, nilInt) {
		t.Errorf("nil vs nil should be equal")
	}
	if !equalIntPtr(&one, &one) {
		t.Errorf("regression: same pointer not equal")
	}
}

func TestEqualInt32Ptr(t *testing.T) {
	a, b := int32(1), int32(2)
	ap, bp := &a, &b
	if !equalInt32Ptr(&a, &a) {
		t.Errorf("&a vs &a should be equal")
	}
	if equalInt32Ptr(ap, bp) {
		t.Errorf("&1 vs &2 should not be equal")
	}
	if equalInt32Ptr(nil, ap) {
		t.Errorf("nil vs &a should not be equal")
	}
	if equalInt32Ptr(ap, nil) {
		t.Errorf("&a vs nil should not be equal")
	}
}

func TestEqualStringSlices(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"different content", []string{"a"}, []string{"b"}, false},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := equalStringSlices(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestIsBucketNotFound(t *testing.T) {
	if isBucketNotFound(nil) {
		t.Errorf("nil should not be not-found")
	}
	for _, s := range []string{"NotFound", "NoSuchBucket", "404 foo", "boom 404 boom"} {
		if !isBucketNotFound(errors.New(s)) {
			t.Errorf("%q should be not-found", s)
		}
	}
	if isBucketNotFound(errors.New("AccessDenied")) {
		t.Errorf("AccessDenied should not be not-found")
	}
}

func TestBucketShouldCreateDelete(t *testing.T) {
	if !shouldCreate(nil) || !shouldDelete(nil) {
		t.Errorf("empty policies should allow all")
	}
	if shouldCreate(xpv1.ManagementPolicies{xpv1.ManagementActionDelete}) {
		t.Errorf("Delete-only should not allow create")
	}
	if shouldDelete(xpv1.ManagementPolicies{xpv1.ManagementActionCreate}) {
		t.Errorf("Create-only should not allow delete")
	}
}

func bucketTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1.AddToScheme: %v", err)
	}
	if err := providerapis.AddToScheme(s); err != nil {
		t.Fatalf("apis.AddToScheme: %v", err)
	}
	return s
}

func bucketB2Server(t *testing.T, buckets []any) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID:          "acc-1",
				AuthorizationToken: "tok",
				APIInfo: clients.B2APIInfo{
					StorageAPI: clients.B2StorageAPI{
						APIURL:      srv.URL,
						DownloadURL: srv.URL + "/file",
					},
				},
			})
		case "/b2api/v3/b2_list_buckets":
			_ = json.NewEncoder(w).Encode(map[string]any{"buckets": buckets})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	prev := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	t.Cleanup(func() { clients.B2AuthorizeAccountURL = prev })
	return srv
}

func bucketProviderConfig(t *testing.T, s *runtime.Scheme) (*apisv1beta1.ProviderConfig, *corev1.Secret) {
	t.Helper()
	pc := &apisv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "crossplane-system"},
		Spec: apisv1beta1.ProviderConfigSpec{
			BackblazeRegion: "us-west-001",
			Credentials: apisv1beta1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{Name: "creds", Namespace: "crossplane-system"},
					},
				},
			},
		},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"applicationKeyId": []byte("id"), "applicationKey": []byte("key")},
	}
	return pc, sec
}

func TestBucketReconcile_ObserveOnlySkipsCreate(t *testing.T) {
	bucketB2Server(t, []any{})
	s := bucketTestScheme(t)
	pc, sec := bucketProviderConfig(t, s)
	b := &backblazev1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{Name: "bkt", Namespace: "default"},
		Spec: backblazev1beta1.BucketSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
			ForProvider: backblazev1beta1.BucketParameters{BucketName: "my-bucket"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, sec, b).WithStatusSubresource(b).Build()
	r := &BucketReconciler{Client: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8stypes.NamespacedName{Name: "bkt", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := &backblazev1beta1.Bucket{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "bkt", Namespace: "default"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetCondition(xpv1.TypeReady).Status != corev1.ConditionFalse {
		t.Errorf("Ready should be False in observe-only mode")
	}
}

func TestBucketReconcile_ClientError(t *testing.T) {
	s := bucketTestScheme(t)
	b := &backblazev1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{Name: "bkt", Namespace: "default"},
		Spec:       backblazev1beta1.BucketSpec{ForProvider: backblazev1beta1.BucketParameters{BucketName: "my-bucket"}},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(b).WithStatusSubresource(b).Build()
	r := &BucketReconciler{Client: cl}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8stypes.NamespacedName{Name: "bkt", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("expected requeue on client error")
	}
	got := &backblazev1beta1.Bucket{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "bkt", Namespace: "default"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetCondition(xpv1.TypeReady).Status != corev1.ConditionFalse {
		t.Errorf("Ready should be False on client error")
	}
}

func TestBucketHandleDeletion_ObserveOnly(t *testing.T) {
	s := bucketTestScheme(t)
	now := metav1.Now()
	b := &backblazev1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{Name: "bkt", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{"test"}},
		Spec: backblazev1beta1.BucketSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
			ForProvider: backblazev1beta1.BucketParameters{BucketName: "my-bucket"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(b).Build()
	r := &BucketReconciler{Client: cl}
	if _, err := r.handleDeletion(context.Background(), b); err != nil {
		t.Fatalf("handleDeletion observe-only should be no-op, got %v", err)
	}
}

func bucketNativeUpdateServer(t *testing.T, calls *int32) (*httptest.Server, *clients.BackblazeClient) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID:          "acc-1",
				AuthorizationToken: "tok",
				APIInfo: clients.B2APIInfo{
					StorageAPI: clients.B2StorageAPI{
						APIURL:      srv.URL,
						DownloadURL: srv.URL + "/file",
					},
				},
			})
		case "/b2api/v3/b2_update_bucket":
			atomic.AddInt32(calls, 1)
			var req clients.B2UpdateBucketRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode update req: %v", err)
			}
			resp := clients.B2UpdateBucketResponse{
				AccountID: "acc-1", BucketID: req.BucketID, BucketName: "my-bucket",
				LifecycleRules:              req.LifecycleRules,
				DefaultServerSideEncryption: req.DefaultServerSideEncryption,
				DefaultRetention:            req.DefaultRetention,
			}
			if req.BucketInfo != nil {
				resp.BucketInfo = *req.BucketInfo
			}
			if req.FileLockEnabled != nil {
				resp.FileLockEnabled = req.FileLockEnabled
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	prev := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	t.Cleanup(func() { clients.B2AuthorizeAccountURL = prev })
	c, err := clients.NewBackblazeClient(clients.Config{ApplicationKeyID: "id", ApplicationKey: "key", Region: "us-west-001"})
	if err != nil {
		t.Fatalf("NewBackblazeClient: %v", err)
	}
	c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	c.APIURL = srv.URL
	return srv, c
}

func TestReconcileLifecycle_NoDrift(t *testing.T) {
	var calls int32
	_, svc := bucketNativeUpdateServer(t, &calls)
	r := &BucketReconciler{}
	one := 1
	current := &b2BucketSummary{
		BucketID: "b1", AccountID: "acc-1",
		LifecycleRules: []clients.B2NativeLifecycleRule{{FileNamePrefix: "logs/", DaysFromHidingToDeleting: &one}},
	}
	desired := []backblazev1beta1.LifecycleRule{{FileNamePrefix: "logs/", DaysFromHidingToDeleting: &one}}
	if err := r.reconcileLifecycle(context.Background(), svc, "my-bucket", desired, current); err != nil {
		t.Fatalf("reconcileLifecycle: %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("no drift should mean no update call, got %d", calls)
	}
}

func TestReconcileLifecycle_Drift(t *testing.T) {
	var calls int32
	_, svc := bucketNativeUpdateServer(t, &calls)
	r := &BucketReconciler{}
	one, thirty := 1, 30
	current := &b2BucketSummary{
		BucketID: "b1", AccountID: "acc-1",
		LifecycleRules: []clients.B2NativeLifecycleRule{{FileNamePrefix: "logs/", DaysFromHidingToDeleting: &one}},
	}
	desired := []backblazev1beta1.LifecycleRule{{FileNamePrefix: "logs/", DaysFromHidingToDeleting: &thirty}}
	if err := r.reconcileLifecycle(context.Background(), svc, "my-bucket", desired, current); err != nil {
		t.Fatalf("reconcileLifecycle: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 update call, got %d", calls)
	}
	if len(current.LifecycleRules) != 1 || current.LifecycleRules[0].DaysFromHidingToDeleting == nil ||
		*current.LifecycleRules[0].DaysFromHidingToDeleting != 30 {
		t.Errorf("current not updated: %+v", current.LifecycleRules)
	}
}

func TestReconcileLifecycle_NilCurrent(t *testing.T) {
	r := &BucketReconciler{}
	if err := r.reconcileLifecycle(context.Background(), nil, "b", nil, nil); err != nil {
		t.Errorf("nil current should be nil, got %v", err)
	}
}

func TestReconcileNativeSettings_Drift(t *testing.T) {
	var calls int32
	_, svc := bucketNativeUpdateServer(t, &calls)
	r := &BucketReconciler{}
	b := &backblazev1beta1.Bucket{
		Spec: backblazev1beta1.BucketSpec{
			ForProvider: backblazev1beta1.BucketParameters{
				BucketInfo: map[string]string{"env": "prod"},
			},
		},
	}
	current := &b2BucketSummary{BucketID: "b1", AccountID: "acc-1", BucketInfo: clients.B2BucketInfo{}}
	var msgs []string
	if err := r.reconcileNativeSettings(context.Background(), svc, b, current, &msgs); err != nil {
		t.Fatalf("reconcileNativeSettings: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 update call, got %d", calls)
	}
	if current.BucketInfo["env"] != "prod" {
		t.Errorf("current info not updated: %v", current.BucketInfo)
	}
	if len(msgs) == 0 {
		t.Errorf("expected drift messages")
	}
}

func TestReconcileNativeSettings_NoDrift(t *testing.T) {
	var calls int32
	_, svc := bucketNativeUpdateServer(t, &calls)
	r := &BucketReconciler{}
	b := &backblazev1beta1.Bucket{}
	current := &b2BucketSummary{BucketID: "b1", AccountID: "acc-1"}
	var msgs []string
	if err := r.reconcileNativeSettings(context.Background(), svc, b, current, &msgs); err != nil {
		t.Fatalf("reconcileNativeSettings: %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("no drift should mean no call, got %d", calls)
	}
}
