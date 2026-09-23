package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// newFakeMetricsClient works around a quirk in
// k8s.io/metrics/pkg/client/clientset/versioned/fake: NewSimpleClientset's
// object tracker guesses the resource name for *v1beta1.PodMetrics as
// "podmetricses" (via meta.UnsafeGuessKindToResource on the Kind), but the
// generated fake typed client lists resource "pods" (matching the real
// metrics.k8s.io API, which deliberately reuses "pods" as its resource
// name). That mismatch makes objects passed directly to
// metricsfake.NewSimpleClientset(...) invisible to List calls, so we seed
// the tracker explicitly with the correct GroupVersionResource instead.
func newFakeMetricsClient(t *testing.T, pods ...*metricsv1beta1.PodMetrics) *metricsfake.Clientset {
	t.Helper()
	cs := metricsfake.NewSimpleClientset()
	gvr := metricsv1beta1.SchemeGroupVersion.WithResource("pods")
	for _, p := range pods {
		if err := cs.Tracker().Create(gvr, p, p.Namespace); err != nil {
			t.Fatalf("seeding fake metrics client: %v", err)
		}
	}
	return cs
}

func TestPoller_Poll_JoinsAcrossWatchedNamespaces(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"}},
	)
	metrics := newFakeMetricsClient(t,
		&metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Containers: []metricsv1beta1.ContainerMetrics{
				{Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				}},
			},
		},
		&metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"},
		},
	)

	poller := &Poller{
		Clients:    &Clients{Core: core, Metrics: metrics},
		Namespaces: []string{"default"},
		Logger:     testLogger(),
		Now:        func() time.Time { return time.Unix(0, 0) },
	}

	samples := poller.Poll(context.Background())

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1 (kube-system is not watched)", len(samples))
	}
	if samples[0].Namespace != "default" || samples[0].Name != "web-1" {
		t.Errorf("got %s/%s, want default/web-1", samples[0].Namespace, samples[0].Name)
	}
}

func TestPoller_Poll_EmptyNamespacesMeansAll(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"}},
	)
	metrics := newFakeMetricsClient(t,
		&metricsv1beta1.PodMetrics{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}},
		&metricsv1beta1.PodMetrics{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns-1"}},
	)

	poller := &Poller{
		Clients: &Clients{Core: core, Metrics: metrics},
		Logger:  testLogger(),
		Now:     time.Now,
	}

	samples := poller.Poll(context.Background())

	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2 (no namespace filter means watch all)", len(samples))
	}
}

func TestPoller_Poll_ResolvesPodUIDAndOwnerThroughAReplicaSet(t *testing.T) {
	core := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "web-7d9f8c6b5d-x8k2p",
				UID:       "pod-uid-123",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "web-7d9f8c6b5d", Controller: boolPtr(true)},
				},
			},
		},
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
	metrics := newFakeMetricsClient(t)

	poller := &Poller{
		Clients:    &Clients{Core: core, Metrics: metrics},
		Namespaces: []string{"default"},
		Logger:     testLogger(),
		Now:        time.Now,
	}

	samples := poller.Poll(context.Background())

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1", len(samples))
	}
	got := samples[0]
	if got.UID != "pod-uid-123" {
		t.Errorf("UID = %q, want %q", got.UID, "pod-uid-123")
	}
	if got.OwnerKind != "Deployment" || got.OwnerName != "web" {
		t.Errorf("owner = %s/%s, want Deployment/web (resolved through the ReplicaSet)", got.OwnerKind, got.OwnerName)
	}
}
