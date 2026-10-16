package v1beta1

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGetters(t *testing.T) {
	b := &Bucket{Spec: BucketSpec{ForProvider: BucketParameters{BucketName: "bkt"}}}
	if b.GetBucketName() != "bkt" {
		t.Errorf("GetBucketName = %q", b.GetBucketName())
	}
	u := &User{Spec: UserSpec{ForProvider: UserParameters{KeyName: "k"}}}
	if u.GetKeyName() != "k" {
		t.Errorf("GetKeyName = %q", u.GetKeyName())
	}
	name := "pol"
	p := &Policy{Spec: PolicySpec{ForProvider: PolicyParameters{PolicyName: &name}}}
	if p.GetPolicyName() != "pol" {
		t.Errorf("GetPolicyName = %q", p.GetPolicyName())
	}
	n := &BucketNotification{Spec: NotificationSpec{ForProvider: NotificationParameters{Name: "rule1"}}}
	if n.GetNotificationName() != "rule1" {
		t.Errorf("GetNotificationName = %q", n.GetNotificationName())
	}
}

func TestAddKnownTypes(t *testing.T) {
	if Group != "backblaze.m.crossplane.io" {
		t.Errorf("Group = %q", Group)
	}
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, kind := range []string{"Bucket", "BucketNotification", "Policy", "User"} {
		gvk := schema.GroupVersionKind{Group: Group, Version: "v1beta1", Kind: kind}
		if _, err := s.New(gvk); err != nil {
			t.Errorf("scheme missing %v: %v", gvk, err)
		}
	}
}
