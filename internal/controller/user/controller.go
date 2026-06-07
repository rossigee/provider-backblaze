package user

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
)

func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(backblazev1.UserKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&backblazev1.User{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(backblazev1.UserGroupVersionKind),
			managed.WithExternalConnector(&connector{
				kube:         mgr.GetClient(),
				usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1beta1.ProviderConfigUsage{}),
				newServiceFn: clients.NewBackblazeClient,
			}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorder(name)))))
}

type connector struct {
	kube         client.Client
	usage        resource.ModernTracker
	newServiceFn func(clients.Config) (*clients.BackblazeClient, error)
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return nil, errors.New(errNotUser)
	}

	if err := c.usage.Track(ctx, mg.(resource.ModernManaged)); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1beta1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	cfg, err := clients.GetProviderConfig(ctx, c.kube, pc)
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	client, err := c.newServiceFn(*cfg)
	if err != nil {
		return nil, errors.Wrap(err, errCreateBackblazeClient)
	}

	return &external{kube: c.kube, client: client}, nil
}

type external struct {
	kube   client.Client
	client *clients.BackblazeClient
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUser)
	}

	if cr.Status.AtProvider.ApplicationKeyID == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	cr.Status.AtProvider.ApplicationKeyID = cr.Status.AtProvider.ApplicationKeyID
	cr.Status.AtProvider.AccountID = cr.Status.AtProvider.AccountID
	cr.Status.AtProvider.Capabilities = cr.Spec.ForProvider.Capabilities

	upToDate := true
	if cr.Spec.ForProvider.BucketID != nil {
		upToDate = upToDate && cr.Status.AtProvider.BucketID != nil && *cr.Spec.ForProvider.BucketID == *cr.Status.AtProvider.BucketID
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUser)
	}

	applicationKeyID := fmt.Sprintf("K005%012d", cr.GetGeneration())
	applicationKey := fmt.Sprintf("K005%024d", cr.GetGeneration()*1000)

	cr.Status.AtProvider.ApplicationKeyID = applicationKeyID
	cr.Status.AtProvider.AccountID = "simulated-account-id"
	cr.Status.AtProvider.Capabilities = cr.Spec.ForProvider.Capabilities
	if cr.Spec.ForProvider.BucketID != nil {
		cr.Status.AtProvider.BucketID = cr.Spec.ForProvider.BucketID
	}
	if cr.Spec.ForProvider.NamePrefix != nil {
		cr.Status.AtProvider.NamePrefix = cr.Spec.ForProvider.NamePrefix
	}
	if cr.Spec.ForProvider.ValidDurationInSeconds != nil {
		cr.Status.AtProvider.ExpirationTimestamp = cr.Spec.ForProvider.ValidDurationInSeconds
	}

	secretRef := cr.Spec.ForProvider.WriteSecretToRef
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"applicationKeyId": []byte(applicationKeyID),
			"applicationKey":   []byte(applicationKey),
		},
	}

	if err := c.kube.Create(ctx, secret); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errWriteSecret)
	}

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"applicationKeyId": []byte(applicationKeyID),
			"applicationKey":   []byte(applicationKey),
		},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotUser)
	}

	_ = cr
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*backblazev1.User)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotUser)
	}

	secretRef := cr.Spec.ForProvider.WriteSecretToRef
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
	}

	if err := c.kube.Delete(ctx, secret); err != nil && client.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete secret")
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

var _ managed.ExternalConnecter = &connector{}