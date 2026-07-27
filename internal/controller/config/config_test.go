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

package config

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	v1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
)

func TestControllerName(t *testing.T) {
	name := providerconfig.ControllerName(v1beta1.ProviderConfigGroupKind.String())
	if name == "" {
		t.Error("ControllerName should not return empty string")
	}
	if name != "providerconfig/providerconfig.backblaze.crossplane.io" {
		t.Errorf("Expected controller name 'providerconfig.backblaze.crossplane.io', got %q", name)
	}
}

func TestProviderConfigKinds(t *testing.T) {
	of := resource.ProviderConfigKinds{
		Config:    v1beta1.ProviderConfigGroupVersionKind,
		Usage:     v1beta1.ProviderConfigUsageGroupVersionKind,
		UsageList: v1beta1.ProviderConfigUsageListGroupVersionKind,
	}

	if of.Config.Kind != "ProviderConfig" {
		t.Errorf("Expected Config kind to be ProviderConfig, got %s", of.Config.Kind)
	}
	if of.Usage.Kind != "ProviderConfigUsage" {
		t.Errorf("Expected Usage kind to be ProviderConfigUsage, got %s", of.Usage.Kind)
	}
	if of.UsageList.Kind != "ProviderConfigUsageList" {
		t.Errorf("Expected UsageList kind to be ProviderConfigUsageList, got %s", of.UsageList.Kind)
	}
}

func TestProviderConfigGroupVersionKind(t *testing.T) {
	if v1beta1.ProviderConfigGroupVersionKind.Group != "backblaze.crossplane.io" {
		t.Errorf("Expected group 'backblaze.crossplane.io', got %s", v1beta1.ProviderConfigGroupVersionKind.Group)
	}
	if v1beta1.ProviderConfigGroupVersionKind.Version != "v1beta1" {
		t.Errorf("Expected version 'v1beta1', got %s", v1beta1.ProviderConfigGroupVersionKind.Version)
	}
}

func TestProviderConfigUsageGroupVersionKind(t *testing.T) {
	if v1beta1.ProviderConfigUsageGroupVersionKind.Group != "backblaze.crossplane.io" {
		t.Errorf("Expected group 'backblaze.crossplane.io', got %s", v1beta1.ProviderConfigUsageGroupVersionKind.Group)
	}
	if v1beta1.ProviderConfigUsageGroupVersionKind.Kind != "ProviderConfigUsage" {
		t.Errorf("Expected kind 'ProviderConfigUsage', got %s", v1beta1.ProviderConfigUsageGroupVersionKind.Kind)
	}
}