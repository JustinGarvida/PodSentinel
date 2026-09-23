package k8s

import (
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// noOwner is a resolveOwner stub for tests that ignore owners.
func noOwner(pod corev1.Pod) (kind, name string) {
	return "", ""
}

func TestJoinPodsAndMetrics_JoinsByNamespaceAndName(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	scrapeTime := time.Date(2026, 8, 28, 11, 59, 30, 0, time.UTC)

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 2},
					{RestartCount: 1},
				},
			},
		},
	}

	metrics := []metricsv1beta1.PodMetrics{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Timestamp:  metav1.NewTime(scrapeTime),
			Containers: []metricsv1beta1.ContainerMetrics{
				{Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				}},
				{Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				}},
			},
		},
	}

	samples := joinPodsAndMetrics(pods, metrics, now, testLogger(), noOwner)

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1", len(samples))
	}
	got := samples[0]
	if got.Namespace != "default" || got.Name != "web-1" {
		t.Errorf("identity = %s/%s, want default/web-1", got.Namespace, got.Name)
	}
	if got.Status != "Running" {
		t.Errorf("Status = %q, want %q", got.Status, "Running")
	}
	if got.RestartCount != 3 {
		t.Errorf("RestartCount = %d, want 3", got.RestartCount)
	}
	if math.Abs(got.CPU-0.15) > 1e-10 {
		t.Errorf("CPU = %v, want 0.15", got.CPU)
	}
	wantMemory := float64(96 * 1024 * 1024)
	if got.Memory != wantMemory {
		t.Errorf("Memory = %v, want %v", got.Memory, wantMemory)
	}
	if !got.Timestamp.Equal(scrapeTime) {
		t.Errorf("Timestamp = %v, want the metrics object's own scrape time %v (not poll wall-clock %v)", got.Timestamp, scrapeTime, now)
	}
}

func TestJoinPodsAndMetrics_ZeroesUsageForPodWithNoMetrics(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	scrapeTime := time.Date(2026, 8, 28, 11, 59, 30, 0, time.UTC)

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "no-metrics-yet"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 4},
				},
			},
		},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	metrics := []metricsv1beta1.PodMetrics{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"}, Timestamp: metav1.NewTime(scrapeTime)},
	}

	samples := joinPodsAndMetrics(pods, metrics, now, testLogger(), noOwner)

	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2 (the pod missing metrics should still be included, not dropped)", len(samples))
	}

	var noMetrics *PodSample
	for i := range samples {
		if samples[i].Name == "no-metrics-yet" {
			noMetrics = &samples[i]
		}
	}
	if noMetrics == nil {
		t.Fatalf("samples %+v did not include the pod with no metrics", samples)
	}
	if noMetrics.CPU != 0 {
		t.Errorf("CPU = %v, want 0", noMetrics.CPU)
	}
	if noMetrics.Memory != 0 {
		t.Errorf("Memory = %v, want 0", noMetrics.Memory)
	}
	if noMetrics.Status != "Pending" {
		t.Errorf("Status = %q, want %q (from the core API, independent of metrics)", noMetrics.Status, "Pending")
	}
	if noMetrics.RestartCount != 4 {
		t.Errorf("RestartCount = %d, want 4 (from the core API, independent of metrics)", noMetrics.RestartCount)
	}
	if !noMetrics.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want the passed-in now %v (no real metrics scrape time to use)", noMetrics.Timestamp, now)
	}
}

func TestJoinPodsAndMetrics_SetsUIDAndUsesResolverForOwner(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1", UID: "pod-uid-123"}},
	}

	resolveOwner := func(pod corev1.Pod) (kind, name string) {
		if pod.Namespace == "default" && pod.Name == "web-1" {
			return "Deployment", "web"
		}
		t.Fatalf("resolveOwner called with unexpected pod %s/%s", pod.Namespace, pod.Name)
		return "", ""
	}

	samples := joinPodsAndMetrics(pods, nil, now, testLogger(), resolveOwner)

	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1", len(samples))
	}
	got := samples[0]
	if got.UID != "pod-uid-123" {
		t.Errorf("UID = %q, want %q", got.UID, "pod-uid-123")
	}
	if got.OwnerKind != "Deployment" || got.OwnerName != "web" {
		t.Errorf("owner = %s/%s, want Deployment/web (from the injected resolver)", got.OwnerKind, got.OwnerName)
	}
}
