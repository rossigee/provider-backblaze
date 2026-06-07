package policy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
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
	name := managed.ControllerName(backblazev1.PolicyKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&backblazev1.Policy{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(backblazev1.PolicyGroupVersionKind),
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
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return nil, errors.New(errNotPolicy)
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

	return &external{client: client}, nil
}

type external struct {
	client *clients.BackblazeClient
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotPolicy)
	}

	policyName := cr.GetPolicyName()

	if cr.Status.AtProvider.PolicyName == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	cr.Status.AtProvider.PolicyName = policyName

	policyMatches := cr.Status.AtProvider.PolicyDocument != ""

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: policyMatches,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotPolicy)
	}

	params := cr.Spec.ForProvider
	if (params.AllowBucket != nil && params.RawPolicy != nil) ||
		(params.AllowBucket == nil && params.RawPolicy == nil) {
		return managed.ExternalCreation{}, errors.New(errInvalidPolicyParams)
	}

	var policyDocument string

	if params.AllowBucket != nil {
		doc, err := generateSimplePolicy(*params.AllowBucket)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errGenerateSimplePolicy)
		}
		policyDocument = doc
	} else {
		policyDocument = *params.RawPolicy
		var temp interface{}
		if err := json.Unmarshal([]byte(policyDocument), &temp); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errInvalidRawPolicy)
		}
	}

	policyName := cr.GetPolicyName()

	cr.Status.AtProvider.PolicyName = policyName
	cr.Status.AtProvider.PolicyDocument = policyDocument
	cr.Status.AtProvider.PolicyID = fmt.Sprintf("policy-%d", cr.GetGeneration())
	now := metav1.Now()
	cr.Status.AtProvider.CreationTime = &now

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotPolicy)
	}

	_ = cr
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*backblazev1.Policy)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotPolicy)
	}

	_ = cr

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func generateSimplePolicy(bucketName string) (string, error) {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:*"},
				"Resource": []string{
					fmt.Sprintf("arn:aws:s3:::%s", bucketName),
					fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
				},
			},
		},
	}

	policyBytes, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return "", err
	}

	return string(policyBytes), nil
}

var _ managed.ExternalConnecter = &connector{}