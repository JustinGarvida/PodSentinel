package store

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := requireTestDB(t)

	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func cleanNamespace(t *testing.T, s *Store, namespace string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `DELETE FROM pod_metrics WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("cleaning pod_metrics for %q: %v", namespace, err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM anomalies WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("cleaning anomalies for %q: %v", namespace, err)
	}
}

func TestStore_InsertPodMetricAndListPods(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-list-pods"
	cleanNamespace(t, s, namespace)
	t.Cleanup(func() { cleanNamespace(t, s, namespace) })

	ctx := context.Background()
	older := time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond)
	newer := time.Now().UTC().Truncate(time.Millisecond)

	if err := s.InsertPodMetric(ctx, PodMetricRow{
		Time: older, Namespace: namespace, Pod: "web-1",
		CPU: 0.1, Memory: 1e8, Status: "Running", RestartCount: 0,
	}); err != nil {
		t.Fatalf("InsertPodMetric() error = %v", err)
	}
	if err := s.InsertPodMetric(ctx, PodMetricRow{
		Time: newer, Namespace: namespace, Pod: "web-1",
		PodUID: "pod-uid-123", OwnerKind: "Deployment", OwnerName: "web",
		CPU: 0.2, Memory: 2e8, Status: "Running", RestartCount: 1,
	}); err != nil {
		t.Fatalf("InsertPodMetric() error = %v", err)
	}

	pods, err := s.ListPods(ctx)
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}

	var found *PodSummary
	for i := range pods {
		if pods[i].Namespace == namespace && pods[i].Pod == "web-1" {
			found = &pods[i]
		}
	}
	if found == nil {
		t.Fatalf("ListPods() did not include %s/web-1", namespace)
	}
	if found.RestartCount != 1 || found.CPU != 0.2 {
		t.Errorf("ListPods() returned stale row: %+v, want the latest sample (restart_count=1, cpu=0.2)", found)
	}
	if found.PodUID != "pod-uid-123" || found.OwnerKind != "Deployment" || found.OwnerName != "web" {
		t.Errorf("ListPods() owner fields = %+v, want PodUID=pod-uid-123 OwnerKind=Deployment OwnerName=web", found)
	}
}

func TestStore_ListPodsExcludesStaleRows(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-list-pods-stale"
	cleanNamespace(t, s, namespace)
	t.Cleanup(func() { cleanNamespace(t, s, namespace) })

	ctx := context.Background()
	stale := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Millisecond)

	if err := s.InsertPodMetric(ctx, PodMetricRow{
		Time: stale, Namespace: namespace, Pod: "ghost-1",
		CPU: 0.1, Memory: 1e8, Status: "Running", RestartCount: 0,
	}); err != nil {
		t.Fatalf("InsertPodMetric() error = %v", err)
	}

	pods, err := s.ListPods(ctx)
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}

	for _, p := range pods {
		if p.Namespace == namespace && p.Pod == "ghost-1" {
			t.Errorf("ListPods() included a row older than 1 hour: %+v, want it excluded", p)
		}
	}
}

func TestStore_GetPodMetricsReturnsTimeSeriesOldestFirst(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-get-metrics"
	cleanNamespace(t, s, namespace)
	t.Cleanup(func() { cleanNamespace(t, s, namespace) })

	ctx := context.Background()
	t1 := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Millisecond)
	t2 := time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond)

	for _, row := range []PodMetricRow{
		{Time: t2, Namespace: namespace, Pod: "api-1", PodUID: "pod-uid-456", OwnerKind: "StatefulSet", OwnerName: "api", CPU: 0.2, Memory: 2e8, Status: "Running"},
		{Time: t1, Namespace: namespace, Pod: "api-1", PodUID: "pod-uid-456", OwnerKind: "StatefulSet", OwnerName: "api", CPU: 0.1, Memory: 1e8, Status: "Running"},
	} {
		if err := s.InsertPodMetric(ctx, row); err != nil {
			t.Fatalf("InsertPodMetric() error = %v", err)
		}
	}

	samples, err := s.GetPodMetrics(ctx, namespace, "api-1")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("len(samples) = %d, want 2", len(samples))
	}
	if !samples[0].Time.Equal(t1) || !samples[1].Time.Equal(t2) {
		t.Errorf("samples not ordered oldest-first: %+v", samples)
	}
	if samples[0].PodUID != "pod-uid-456" || samples[0].OwnerKind != "StatefulSet" || samples[0].OwnerName != "api" {
		t.Errorf("samples[0] owner fields = %+v, want PodUID=pod-uid-456 OwnerKind=StatefulSet OwnerName=api", samples[0])
	}
}

func TestStore_GetPodMetricsExcludesRowsOlderThanOneHour(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-get-metrics-window"
	cleanNamespace(t, s, namespace)
	t.Cleanup(func() { cleanNamespace(t, s, namespace) })

	ctx := context.Background()
	inWindow := time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond)
	stale := time.Now().Add(-90 * time.Minute).UTC().Truncate(time.Millisecond)

	for _, row := range []PodMetricRow{
		{Time: stale, Namespace: namespace, Pod: "api-1", CPU: 0.9, Memory: 9e8, Status: "Running"},
		{Time: inWindow, Namespace: namespace, Pod: "api-1", CPU: 0.1, Memory: 1e8, Status: "Running"},
	} {
		if err := s.InsertPodMetric(ctx, row); err != nil {
			t.Fatalf("InsertPodMetric() error = %v", err)
		}
	}

	samples, err := s.GetPodMetrics(ctx, namespace, "api-1")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d, want 1 (the row older than 1 hour should be excluded): %+v", len(samples), samples)
	}
	if !samples[0].Time.Equal(inWindow) {
		t.Errorf("samples[0].Time = %v, want the in-window row %v", samples[0].Time, inWindow)
	}
}

func TestStore_ListAnomaliesReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)
	namespace := "store-test-anomalies-empty"
	cleanNamespace(t, s, namespace)

	anomalies, err := s.ListAnomalies(context.Background(), namespace, "unknown-pod")
	if err != nil {
		t.Fatalf("ListAnomalies() error = %v", err)
	}
	if anomalies == nil {
		t.Error("ListAnomalies() = nil, want empty non-nil slice")
	}
	if len(anomalies) != 0 {
		t.Errorf("len(anomalies) = %d, want 0", len(anomalies))
	}
}
