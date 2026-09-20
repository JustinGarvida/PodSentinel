package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"podsentinel/internal/models"
	"podsentinel/internal/store"
)

func testDSN() string {
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable"
}

func newTestRouter(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	dsn := testDSN()

	st, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping: Postgres not reachable at %s (is `docker compose -f infra/docker-compose.yml up -d` running?): %v", dsn, err)
	}
	t.Cleanup(st.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(logger, st), st, dsn
}

func cleanupNamespace(t *testing.T, dsn, namespace string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return
	}
	defer pool.Close()
	_, _ = pool.Exec(context.Background(), `DELETE FROM pod_metrics WHERE namespace = $1`, namespace)
}

func TestListPods_ReturnsStoreData(t *testing.T) {
	router, st, dsn := newTestRouter(t)
	namespace := "api-test-list-pods"
	t.Cleanup(func() { cleanupNamespace(t, dsn, namespace) })

	ctx := context.Background()
	if err := st.InsertPodMetric(ctx, store.PodMetricRow{
		Time: time.Now().UTC().Truncate(time.Millisecond), Namespace: namespace, Pod: "web-1",
		CPU: 0.1, Memory: 1e8, Status: "Running", RestartCount: 2,
	}); err != nil {
		t.Fatalf("seeding pod metric: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var pods []models.PodSummary
	if err := json.NewDecoder(rec.Body).Decode(&pods); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	var found bool
	for _, p := range pods {
		if p.Namespace == namespace && p.Name == "web-1" && p.RestartCount == 2 {
			found = true
			if p.CPU != 0.1 {
				t.Errorf("CPU = %v, want 0.1", p.CPU)
			}
			if p.Memory != 1e8 {
				t.Errorf("Memory = %v, want 1e8", p.Memory)
			}
		}
	}
	if !found {
		t.Errorf("response %+v did not include the seeded pod", pods)
	}
}

func TestGetPod_ReturnsLatestStatusAndAllMetricsOldestFirst(t *testing.T) {
	router, st, dsn := newTestRouter(t)
	namespace := "api-test-get-pod"
	t.Cleanup(func() { cleanupNamespace(t, dsn, namespace) })

	ctx := context.Background()
	older := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Millisecond)
	newer := time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond)

	if err := st.InsertPodMetric(ctx, store.PodMetricRow{
		Time: older, Namespace: namespace, Pod: "web-1",
		CPU: 0.1, Memory: 1e8, Status: "Pending", RestartCount: 0,
	}); err != nil {
		t.Fatalf("seeding older pod metric: %v", err)
	}
	if err := st.InsertPodMetric(ctx, store.PodMetricRow{
		Time: newer, Namespace: namespace, Pod: "web-1",
		CPU: 0.2, Memory: 2e8, Status: "Running", RestartCount: 3,
	}); err != nil {
		t.Fatalf("seeding newer pod metric: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods/"+namespace+"/web-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var detail models.PodDetail
	if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if detail.Namespace != namespace || detail.Name != "web-1" {
		t.Errorf("identity = %s/%s, want %s/web-1", detail.Namespace, detail.Name, namespace)
	}
	if detail.Status != "Running" {
		t.Errorf("Status = %q, want %q (the newest row's status)", detail.Status, "Running")
	}
	if detail.RestartCount != 3 {
		t.Errorf("RestartCount = %d, want 3 (the newest row's restart count)", detail.RestartCount)
	}
	if len(detail.Metrics) != 2 {
		t.Fatalf("len(Metrics) = %d, want 2", len(detail.Metrics))
	}
	if !detail.Metrics[0].Timestamp.Equal(older) || !detail.Metrics[1].Timestamp.Equal(newer) {
		t.Errorf("Metrics not ordered oldest-first: %+v", detail.Metrics)
	}
}

func TestListPodAnomalies_ReturnsEmptyArrayNotNull(t *testing.T) {
	router, _, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pods/default/no-anomalies-yet/anomalies", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want %q (empty array, not null)", body, "[]\n")
	}
}
