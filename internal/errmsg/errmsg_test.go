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

package errmsg

import "testing"

func TestStable(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "cors not found with ids",
			in:   "failed to get bucket cors configuration: operation error S3: GetBucketCors, https response error StatusCode: 404, RequestID: 507edb70c09b5994, HostID: aYRljuWLcNaE5iThKYZQzhTUmMY00zzYX, api error NoSuchCorsConfiguration: The CORS configuration does not exist",
			want: "failed to get bucket cors configuration: operation error S3: GetBucketCors, https response error StatusCode: 404, api error NoSuchCorsConfiguration: The CORS configuration does not exist",
		},
		{
			name: "forbidden not s3-enabled with ids",
			in:   "failed to get bucket cors configuration: operation error S3: GetBucketCors, https response error StatusCode: 403, RequestID: 31d8c24108fdd038, HostID: aYcNj4mKeNRo5/DjaYc0zyzX2MYM0gjZu, api error AccessDenied: Not allowed to access bucket because it is not s3-enabled: golder-family",
			want: "failed to get bucket cors configuration: operation error S3: GetBucketCors, https response error StatusCode: 403, api error AccessDenied: Not allowed to access bucket because it is not s3-enabled: golder-family",
		},
		{
			name: "invalid request with ids",
			in:   "failed to delete bucket cors configuration: operation error S3: DeleteBucketCors, https response error StatusCode: 400, RequestID: 3535277d72b6bcfa, HostID: aYU9jSWLUNRw53zgdYbAzQjUjMY80WDZa, api error InvalidRequest: The bucket contains B2 Native CORS rules. Please use B2 Native API instead.",
			want: "failed to delete bucket cors configuration: operation error S3: DeleteBucketCors, https response error StatusCode: 400, api error InvalidRequest: The bucket contains B2 Native CORS rules. Please use B2 Native API instead.",
		},
		{
			name: "leading id leaves no dangling separator",
			in:   "RequestID: 507edb70c09b5994, api error NoSuchBucket: missing",
			want: "api error NoSuchBucket: missing",
		},
		{
			name: "no ids left untouched",
			in:   "some plain error message",
			want: "some plain error message",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Stable(tc.in); got != tc.want {
				t.Errorf("Stable() = %q, want %q", got, tc.want)
			}
		})
	}
}
