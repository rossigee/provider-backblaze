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

package user

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

func TestExternalObserve(t *testing.T) {
	type args struct {
		mg resource.Managed
	}
	type want struct {
		o   managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"user_exists_and_up_to_date": {
			args: args{
				mg: &backblazev1.User{
					Status: backblazev1.UserStatus{
						AtProvider: backblazev1.UserObservation{
							ApplicationKeyID: "K005123456789012",
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"user_does_not_exist": {
			args: args{
				mg: &backblazev1.User{},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}
			got, err := e.Observe(context.Background(), tc.args.mg)

			if tc.want.err != nil && err == nil {
				t.Errorf("Observe(...): expected error %v, got nil", tc.want.err)
			}
			if tc.want.err == nil && err != nil {
				t.Errorf("Observe(...): expected no error, got %v", err)
			}
			if got.ResourceExists != tc.want.o.ResourceExists {
				t.Errorf("Observe(...): expected ResourceExists %v, got %v", tc.want.o.ResourceExists, got.ResourceExists)
			}
			if got.ResourceUpToDate != tc.want.o.ResourceUpToDate {
				t.Errorf("Observe(...): expected ResourceUpToDate %v, got %v", tc.want.o.ResourceUpToDate, got.ResourceUpToDate)
			}
		})
	}
}

func TestExternalCreate(t *testing.T) {
	type args struct {
		mg resource.Managed
	}
	type want struct {
		c   managed.ExternalCreation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"successful_creation": {
			args: args{
				mg: &backblazev1.User{
					Spec: backblazev1.UserSpec{
						ForProvider: backblazev1.UserParameters{
							KeyName:      "test-key",
							Capabilities: []string{"listBuckets", "readFiles"},
							WriteSecretToRef: xpv1.SecretReference{
								Name:      "test-secret",
								Namespace: "default",
							},
						},
					},
				},
			},
			want: want{
				c: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{
						"applicationKeyId": []byte("K005000000000000"),
						"applicationKey":   []byte("K005000000000000000000000"),
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{
				service: &clients.MockBackblazeClient{},
				kube:    &mockKubeClient{},
			}
			got, err := e.Create(context.Background(), tc.args.mg)

			if tc.want.err != nil && err == nil {
				t.Errorf("Create(...): expected error %v, got nil", tc.want.err)
			}
			if tc.want.err == nil && err != nil {
				t.Errorf("Create(...): expected no error, got %v", err)
			}
			if got.ConnectionDetails != nil {
				if string(got.ConnectionDetails["applicationKeyId"]) == "" {
					t.Error("Create(...): expected applicationKeyId in connection details")
				}
				if string(got.ConnectionDetails["applicationKey"]) == "" {
					t.Error("Create(...): expected applicationKey in connection details")
				}
			}
		})
	}
}

func TestExternalDelete(t *testing.T) {
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"successful_deletion": {
			args: args{
				mg: &backblazev1.User{
					Status: backblazev1.UserStatus{
						AtProvider: backblazev1.UserObservation{
							ApplicationKeyID: "K005123456789012",
						},
					},
					Spec: backblazev1.UserSpec{
						ForProvider: backblazev1.UserParameters{
							WriteSecretToRef: xpv1.SecretReference{
								Name:      "test-secret",
								Namespace: "default",
							},
						},
					},
				},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{
				service: &clients.MockBackblazeClient{},
				kube:    &mockKubeClient{},
			}
			err := e.Delete(context.Background(), tc.args.mg)

			if tc.want.err != nil && err == nil {
				t.Errorf("Delete(...): expected error %v, got nil", tc.want.err)
			}
			if tc.want.err == nil && err != nil {
				t.Errorf("Delete(...): expected no error, got %v", err)
			}
		})
	}
}

func TestExternalUpdate(t *testing.T) {
	e := &external{}
	_, err := e.Update(context.Background(), &backblazev1.User{})
	if err != nil {
		t.Errorf("Update(...): expected no error, got %v", err)
	}
}

func TestExternalDisconnect(t *testing.T) {
	// No explicit Disconnect method, so nothing to test
}

func TestObserveWithWrongType(t *testing.T) {
	e := &external{}
	_, err := e.Observe(context.Background(), &backblazev1.Bucket{})
	if err == nil {
		t.Error("Observe(...): expected error for wrong type, got nil")
	}
}

// mockKubeClient is a simple mock implementation of client.Client for testing
type mockKubeClient struct{}

func (m *mockKubeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}

func (m *mockKubeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (m *mockKubeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return nil
}

func (m *mockKubeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}

func (m *mockKubeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}

func (m *mockKubeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}

func (m *mockKubeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}

func (m *mockKubeClient) Status() client.StatusWriter {
	return &mockStatusWriter{}
}

func (m *mockKubeClient) Scheme() *runtime.Scheme {
	return nil
}

func (m *mockKubeClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (m *mockKubeClient) SubResource(subResource string) client.SubResourceClient {
	return nil
}

type mockStatusWriter struct{}

func (m *mockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (m *mockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (m *mockStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}