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
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rossigee/provider-backblaze/internal/controller/bucket"
	"github.com/rossigee/provider-backblaze/internal/controller/policy"
	"github.com/rossigee/provider-backblaze/internal/controller/providerconfig"
	"github.com/rossigee/provider-backblaze/internal/controller/user"
)

// Setup sets up all controllers for the Backblaze provider.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	if err := setupRBAC(mgr.GetClient(), o.Logger); err != nil {
		o.Logger.Info("RBAC setup warning (may be transient)", "error", err)
	}
	if err := providerconfig.Setup(mgr); err != nil {
		return err
	}
	// v1 controllers (cluster-scoped - Crossplane v2)
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

func setupRBAC(c client.Client, l logging.Logger) error {
	ctx := context.Background()

	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{"backblaze.crossplane.io"}, Resources: []string{"buckets", "buckets/status", "policies", "policies/status", "providerconfigs", "providerconfigs/status", "providerconfigusages", "providerconfigusages/status", "users", "users/status"}, Verbs: []string{"get", "list", "watch", "update", "patch", "create"}},
		{APIGroups: []string{"backblaze.crossplane.io"}, Resources: []string{"*/finalizers"}, Verbs: []string{"update"}},
		{APIGroups: []string{"", "coordination.k8s.io"}, Resources: []string{"secrets", "configmaps", "events", "leases"}, Verbs: []string{"*"}},
	}

	system := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "crossplane:provider:provider-backblaze:system", Labels: map[string]string{"rbac.crossplane.io/system": "provider-backblaze"}},
		Rules:      rules,
	}
	if err := c.Create(ctx, system); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if err := c.Update(ctx, system); err != nil {
		l.Info("system role update", "err", err)
	}

	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "crossplane:provider:provider-backblaze:system"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "crossplane:provider:provider-backblaze:system"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "provider-backblaze", Namespace: "crossplane-system"}},
	}
	if err := c.Create(ctx, binding); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if err := c.Update(ctx, binding); err != nil {
		l.Info("system binding update", "err", err)
	}

	edit := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crossplane:provider:provider-backblaze:aggregate-to-edit",
			Labels: map[string]string{"rbac.crossplane.io/aggregate-to-edit": "true", "rbac.crossplane.io/aggregate-to-admin": "true", "rbac.crossplane.io/aggregate-to-crossplane": "true", "rbac.crossplane.io/system": "provider-backblaze"},
		},
		Rules: withVerbs(rules, []string{"*"}),
	}
	if err := c.Create(ctx, edit); err != nil && !errors.IsAlreadyExists(err) {
		l.Info("aggregate-to-edit create warning (non-fatal)", "err", err)
	}
	_ = c.Update(ctx, edit)

	view := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "crossplane:provider:provider-backblaze:aggregate-to-view",
			Labels: map[string]string{"rbac.crossplane.io/aggregate-to-view": "true", "rbac.crossplane.io/system": "provider-backblaze"},
		},
		Rules: withVerbs(rules, []string{"get", "list", "watch"}),
	}
	if err := c.Create(ctx, view); err != nil && !errors.IsAlreadyExists(err) {
		l.Info("aggregate-to-view create warning (non-fatal)", "err", err)
	}
	_ = c.Update(ctx, view)

	l.Info("provider self-managed RBAC roles ensured")
	return nil
}

func withVerbs(r []rbacv1.PolicyRule, verbs []string) []rbacv1.PolicyRule {
	out := make([]rbacv1.PolicyRule, len(r))
	for i := range r {
		out[i] = r[i]
		out[i].Verbs = verbs
	}
	return out
}
