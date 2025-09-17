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

// Package controller contains the controllers for the Backblaze provider.
package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/pkg/controller"

	"github.com/rossigee/provider-backblaze/internal/controller/bucket"
	"github.com/rossigee/provider-backblaze/internal/controller/policy"
	"github.com/rossigee/provider-backblaze/internal/controller/user"
)

// Setup sets up all controllers for the Backblaze provider.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	// v1beta1 controllers (namespaced - Crossplane v2 only)
	if err := bucket.SetupBucket(mgr, o); err != nil {
		return err
	}
	if err := user.SetupUser(mgr, o); err != nil {
		return err
	}
	if err := policy.SetupPolicy(mgr, o); err != nil {
		return err
	}
	return nil
}
