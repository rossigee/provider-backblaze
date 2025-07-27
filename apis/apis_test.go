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

package apis

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	
	// Test that AddToScheme can be called without error
	err := AddToScheme(scheme)
	if err != nil {
		t.Errorf("AddToScheme failed: %v", err)
	}

	// Test that calling AddToScheme multiple times doesn't cause issues
	err = AddToScheme(scheme)
	if err != nil {
		t.Errorf("AddToScheme failed on second call: %v", err)
	}
}

func TestSchemeRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	
	err := AddToScheme(scheme)
	if err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	// Test that scheme has some known types registered
	// We can't easily test specific types without importing them,
	// but we can verify the scheme is not empty
	allKnownTypes := scheme.AllKnownTypes()
	if len(allKnownTypes) == 0 {
		t.Error("Expected some types to be registered in scheme, but found none")
	}

	t.Logf("Registered %d types in scheme", len(allKnownTypes))
}