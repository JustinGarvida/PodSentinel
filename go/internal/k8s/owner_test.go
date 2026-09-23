package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func boolPtr(b bool) *bool { return &b }

func TestOwnerCache_Resolve_NoOwnerReferences(t *testing.T) {
	c := newOwnerCache(k8sfake.NewSimpleClientset(), testLogger())

	kind, name := c.resolve(context.Background(), "default", nil)

	if kind != "" || name != "" {
		t.Errorf("resolve() = (%q, %q), want (\"\", \"\") for a bare pod", kind, name)
	}
}

func TestOwnerCache_Resolve_DirectOwnerNotReplicaSet(t *testing.T) {
	c := newOwnerCache(k8sfake.NewSimpleClientset(), testLogger())

	refs := []metav1.OwnerReference{
		{Kind: "StatefulSet", Name: "web", Controller: boolPtr(true)},
	}

	kind, name := c.resolve(context.Background(), "default", refs)

	if kind != "StatefulSet" || name != "web" {
		t.Errorf("resolve() = (%q, %q), want (StatefulSet, web)", kind, name)
	}
}

func TestOwnerCache_Resolve_FollowsReplicaSetToDeployment(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "web-7d9f8c6b5d",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "web", Controller: boolPtr(true)},
				},
			},
		},
	)
	c := newOwnerCache(core, testLogger())

	refs := []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "web-7d9f8c6b5d", Controller: boolPtr(true)},
	}

	kind, name := c.resolve(context.Background(), "default", refs)

	if kind != "Deployment" || name != "web" {
		t.Errorf("resolve() = (%q, %q), want (Deployment, web)", kind, name)
	}
}

func TestOwnerCache_Resolve_ReplicaSetWithNoFurtherOwner(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "bare-rs"},
		},
	)
	c := newOwnerCache(core, testLogger())

	refs := []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "bare-rs", Controller: boolPtr(true)},
	}

	kind, name := c.resolve(context.Background(), "default", refs)

	if kind != "ReplicaSet" || name != "bare-rs" {
		t.Errorf("resolve() = (%q, %q), want (ReplicaSet, bare-rs) when the ReplicaSet has no controller owner", kind, name)
	}
}

func TestOwnerCache_Resolve_ReplicaSetLookupFailureFallsBackToReplicaSet(t *testing.T) {
	// No ReplicaSet seeded, so the Get call 404s.
	c := newOwnerCache(k8sfake.NewSimpleClientset(), testLogger())

	refs := []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "missing-rs", Controller: boolPtr(true)},
	}

	kind, name := c.resolve(context.Background(), "default", refs)

	if kind != "ReplicaSet" || name != "missing-rs" {
		t.Errorf("resolve() = (%q, %q), want (ReplicaSet, missing-rs) when the lookup fails", kind, name)
	}
}

func TestOwnerCache_Resolve_MemoizesReplicaSetLookupWithinACycle(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "web-7d9f8c6b5d",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "web", Controller: boolPtr(true)},
				},
			},
		},
	)
	var gets int
	core.PrependReactor("get", "replicasets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		gets++
		return false, nil, nil
	})

	c := newOwnerCache(core, testLogger())
	refs := []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "web-7d9f8c6b5d", Controller: boolPtr(true)},
	}

	// Two pods sharing the same ReplicaSet, resolved within one cycle.
	c.resolve(context.Background(), "default", refs)
	c.resolve(context.Background(), "default", refs)

	if gets != 1 {
		t.Errorf("ReplicaSet Get calls = %d, want 1 (second resolve should hit the memo)", gets)
	}
}
