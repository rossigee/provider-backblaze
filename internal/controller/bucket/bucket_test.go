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
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rossigee/provider-backblaze/apis/bucket/v1"
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
	cr, ok := mg.(*v1.Bucket)
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
	cr, ok := mg.(*v1.Bucket)
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
	cr, ok := mg.(*v1.Bucket)
	if !ok {
		return nil, errors.New(errNotBucket)
	}

	bucketName := cr.GetBucketName()

	// Handle deletion policy
	if cr.Spec.ForProvider.BucketDeletionPolicy == v1.DeleteAll {
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
	_, ok := mg.(*v1.Bucket)
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
		bucket         *v1.Bucket
		mockBehavior   func(*MockBackblazeClient)
		expectedExists bool
		expectedError  bool
	}{
		{
			name: "bucket exists and up to date",
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
		bucket        *v1.Bucket
		mockBehavior  func(*MockBackblazeClient)
		expectedError bool
	}{
		{
			name: "successful creation",
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
		bucket        *v1.Bucket
		mockBehavior  func(*MockBackblazeClient)
		expectedError bool
	}{
		{
			name: "successful deletion without objects",
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
						BucketName:           "test-bucket",
						Region:               "us-west-001",
						BucketDeletionPolicy: v1.DeleteAll,
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
			bucket: &v1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bucket",
				},
				Spec: v1.BucketSpec{
					ForProvider: v1.BucketParameters{
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
	bucket := &v1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-bucket",
		},
		Spec: v1.BucketSpec{
			ForProvider: v1.BucketParameters{
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
