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

package notification

import (
	"context"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

const (
	errNotNotification    = "managed resource is not a BucketNotification custom resource"
	errGetProviderConfig  = "cannot get referenced ProviderConfig"
	errListBuckets        = "cannot list B2 buckets"
	errCreateNotification = "cannot create notification rule"
	errUpdateNotification = "cannot update notification rule"
	errDeleteNotification = "cannot delete notification rule"
)

// SetupNotification adds a controller that reconciles BucketNotification
// managed resources.
func SetupNotification(mgr ctrl.Manager, o controller.Options) error {
	r := &NotificationReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorder("notification-controller"),
	}
	cache := mgr.GetCache()
	if _, err := cache.GetInformer(context.Background(), &apisv1beta1.ProviderConfig{}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("notification-controller").
		For(&backblazev1beta1.BucketNotification{}).
		Watches(&apisv1beta1.ProviderConfig{}, handler.Funcs{}).
		Complete(r)
}

type NotificationReconciler struct {
	Client   client.Client
	Recorder events.EventRecorder
}

func (r *NotificationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("notification", req.NamespacedName)

	n := &backblazev1beta1.BucketNotification{}
	if err := r.Client.Get(ctx, req.NamespacedName, n); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get BucketNotification")
		return reconcile.Result{}, err
	}

	logger.Info("Reconciling bucket notification", "name", n.GetNotificationName())

	if !n.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, n)
	}

	svc, err := r.getBackblazeClient(ctx, n)
	if err != nil {
		logger.Error(err, "Failed to create Backblaze client")
		r.setReady(n, "False", "ClientError", err.Error())
		r.emit(n, "ClientError", err)
		return reconcile.Result{RequeueAfter: cacheRequeue(err)}, r.Client.Status().Update(ctx, n)
	}

	bucketID, accountID, err := resolveBucket(ctx, svc, n)
	if err != nil {
		logger.Error(err, "Failed to resolve target bucket")
		r.setReady(n, "False", "BucketResolve", err.Error())
		r.emit(n, "BucketResolveError", err)
		return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, n)
	}

	meta.SetExternalName(n, strings.Join([]string{bucketID, n.GetNotificationName()}, "/"))

	var observed *clients.B2EventNotification
	if n.Status.AtProvider.NotificationID != "" {
		listed, err := svc.B2ListEventNotifications(ctx, clients.B2ListEventNotificationsRequest{
			AccountID: accountID,
			BucketID:  bucketID,
		})
		if err != nil {
			logger.Error(err, "Failed to list event notifications")
			r.setReady(n, "False", "ObserveError", err.Error())
			r.emit(n, "ObserveError", err)
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, n)
		}
		for _, rule := range listed.Notifications {
			if rule.NotificationID == n.Status.AtProvider.NotificationID {
				observed = &rule
				break
			}
		}
	}

	desiredEvents := make([]string, 0, len(n.Spec.ForProvider.Events))
	for _, e := range n.Spec.ForProvider.Events {
		desiredEvents = append(desiredEvents, string(e))
	}

	if observed == nil {
		if !shouldCreate(n.GetManagementPolicies()) {
			logger.Info("Skipping notification creation due to managementPolicies")
			r.setReady(n, "False", "ObserveOnly", "External notification does not exist and creation is disabled")
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, n)
		}
		created, err := svc.B2CreateEventNotification(ctx, clients.B2CreateEventNotificationRequest{
			AccountID:   accountID,
			BucketID:    bucketID,
			Name:        n.GetNotificationName(),
			Events:      desiredEvents,
			WebhookURL:  n.Spec.ForProvider.WebhookURL,
			Description: stringPtrValue(n.Spec.ForProvider.Description),
			Disabled:    n.Spec.ForProvider.Disabled,
		})
		if err != nil {
			logger.Error(err, "Failed to create event notification")
			r.setReady(n, "False", errCreateNotification, err.Error())
			r.emit(n, "CreateError", err)
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, n)
		}
		n.Status.AtProvider.NotificationID = created.NotificationID
		n.Status.AtProvider.BucketID = bucketID
		n.Status.AtProvider.Events = n.Spec.ForProvider.Events
	} else if notificationDrift(observed, n) {
		// Drift: patch in place via b2_update_event_notification.
		if !shouldCreate(n.GetManagementPolicies()) {
			r.setReady(n, "False", "ObserveOnly", "Drift detected but managementPolicies disallows update")
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, n)
		}
		_, err := svc.B2UpdateEventNotification(ctx, clients.B2UpdateEventNotificationRequest{
			AccountID:      accountID,
			BucketID:       bucketID,
			NotificationID: observed.NotificationID,
			Name:           n.GetNotificationName(),
			Events:         desiredEvents,
			WebhookURL:     n.Spec.ForProvider.WebhookURL,
			Description:    stringPtrValue(n.Spec.ForProvider.Description),
			Disabled:       n.Spec.ForProvider.Disabled,
		})
		if err != nil {
			logger.Error(err, "Failed to update event notification")
			r.setReady(n, "False", errUpdateNotification, err.Error())
			r.emit(n, "UpdateError", err)
			return reconcile.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, n)
		}
		n.Status.AtProvider.Events = n.Spec.ForProvider.Events
	}

	r.setReady(n, "True", "Available", "Notification is active")
	r.setSynced(n, "True", "ReconcileSuccess", "Successfully reconciled")

	return reconcile.Result{RequeueAfter: 5 * time.Minute}, r.Client.Status().Update(ctx, n)
}

func (r *NotificationReconciler) handleDeletion(ctx context.Context, n *backblazev1beta1.BucketNotification) (reconcile.Result, error) {
	if !shouldDelete(n.GetManagementPolicies()) {
		return reconcile.Result{}, nil
	}
	if n.Status.AtProvider.NotificationID == "" {
		return reconcile.Result{}, nil
	}
	svc, err := r.getBackblazeClient(ctx, n)
	if err != nil {
		// ProviderConfig may already be gone; allow k8s GC to proceed.
		return reconcile.Result{}, nil
	}
	bucketID, accountID, err := resolveBucket(ctx, svc, n)
	if err != nil {
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}
	if err := svc.B2DeleteEventNotification(ctx, clients.B2DeleteEventNotificationRequest{
		AccountID:      accountID,
		BucketID:       bucketID,
		NotificationID: n.Status.AtProvider.NotificationID,
	}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to delete notification", "notificationID", n.Status.AtProvider.NotificationID)
		r.emit(n, "DeleteError", err)
	}
	return reconcile.Result{}, nil
}

func (r *NotificationReconciler) getBackblazeClient(ctx context.Context, n *backblazev1beta1.BucketNotification) (*clients.BackblazeClient, error) {
	providerConfigName := "default"
	if ref := n.GetProviderConfigReference(); ref != nil && ref.Name != "" {
		providerConfigName = ref.Name
	}
	pc := &apisv1beta1.ProviderConfig{}
	key := client.ObjectKey{Name: providerConfigName, Namespace: "crossplane-system"}
	if err := r.Client.Get(ctx, key, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}
	cfg, err := clients.GetProviderConfig(ctx, r.Client, pc)
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}
	return clients.NewBackblazeClient(*cfg)
}

func resolveBucket(ctx context.Context, svc *clients.BackblazeClient, n *backblazev1beta1.BucketNotification) (bucketID, accountID string, err error) {
	if n.Spec.ForProvider.BucketID != nil && *n.Spec.ForProvider.BucketID != "" {
		bucketID = *n.Spec.ForProvider.BucketID
		if n.Status.AtProvider.NotificationID == "" {
			// We still need the accountId for subsequent calls.
			list, lErr := svc.B2ListBuckets(ctx)
			if lErr != nil {
				return "", "", errors.Wrap(lErr, errListBuckets)
			}
			for _, b := range list.Buckets {
				if b.BucketID == bucketID {
					return bucketID, b.AccountID, nil
				}
			}
			return "", "", errors.Errorf("bucket %s not found", bucketID)
		}
		return bucketID, "", nil
	}
	if n.Spec.ForProvider.BucketName == nil || *n.Spec.ForProvider.BucketName == "" {
		return "", "", errors.New("either bucketId or bucketName must be specified")
	}
	list, err := svc.B2ListBuckets(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, errListBuckets)
	}
	for _, b := range list.Buckets {
		if b.BucketName == *n.Spec.ForProvider.BucketName {
			return b.BucketID, b.AccountID, nil
		}
	}
	return "", "", errors.Errorf("bucket %s not found", *n.Spec.ForProvider.BucketName)
}

func notificationDrift(observed *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) bool {
	if observed.Name != n.GetNotificationName() {
		return true
	}
	if observed.WebhookURL != n.Spec.ForProvider.WebhookURL {
		return true
	}
	if observed.Disabled != n.Spec.ForProvider.Disabled {
		return true
	}
	if stringPtrValue(n.Spec.ForProvider.Description) != observed.Description {
		return true
	}
	if len(observed.Events) != len(n.Spec.ForProvider.Events) {
		return true
	}
	want := make(map[string]struct{}, len(n.Spec.ForProvider.Events))
	for _, e := range n.Spec.ForProvider.Events {
		want[string(e)] = struct{}{}
	}
	for _, e := range observed.Events {
		if _, ok := want[e]; !ok {
			return true
		}
	}
	return false
}

func (r *NotificationReconciler) setReady(n *backblazev1beta1.BucketNotification, status, reason, msg string) {
	n.SetConditions(xpv1.Condition{
		Type:               xpv1.TypeReady,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            msg,
	})
}

func (r *NotificationReconciler) setSynced(n *backblazev1beta1.BucketNotification, status, reason, msg string) {
	n.SetConditions(xpv1.Condition{
		Type:               xpv1.TypeSynced,
		Status:             corev1.ConditionStatus(status),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             xpv1.ConditionReason(reason),
		Message:            msg,
	})
}

func (r *NotificationReconciler) emit(n *backblazev1beta1.BucketNotification, reason string, err error) {
	if r.Recorder == nil || err == nil {
		return
	}
	r.Recorder.Eventf(n, nil, corev1.EventTypeWarning, reason, "Reconcile", err.Error())
}

func cacheRequeue(err error) time.Duration {
	if err != nil && strings.Contains(err.Error(), "not found") {
		return 10 * time.Second
	}
	return time.Minute
}

func stringPtrValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func shouldCreate(mp xpv1.ManagementPolicies) bool {
	if len(mp) == 0 {
		return true
	}
	for _, p := range mp {
		if p == xpv1.ManagementActionCreate || p == xpv1.ManagementActionAll {
			return true
		}
	}
	return false
}

func shouldDelete(mp xpv1.ManagementPolicies) bool {
	if len(mp) == 0 {
		return true
	}
	for _, p := range mp {
		if p == xpv1.ManagementActionDelete || p == xpv1.ManagementActionAll {
			return true
		}
	}
	return false
}
