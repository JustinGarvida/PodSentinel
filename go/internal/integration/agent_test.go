//go:build integration

// Package integration exercises the Go agent's real ingestion path
// against a live KIND cluster and the docker-compose TimescaleDB
// instance. Run with:
//
//	go test -tags=integration ./internal/integration/...
//
// Requires: the kind-podsentinel cluster (see docs/infra/kind.md, with
// metrics-server installed) as the current kubectl context, and
// infra/docker-compose.yml's timescaledb service running.
package integration

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"podsentinel/internal/ingest"
	"podsentinel/internal/k8s"
	"podsentinel/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAgent_PollsRealClusterAndPersistsToRealPostgres(t *testing.T) {
	ctx := context.Background()

	clients, err := k8s.BuildClients()
	if err != nil {
		t.Fatalf("k8s.BuildClients() error = %v (is the kind-podsentinel cluster reachable?)", err)
	}

	namespace := "podsentinel-integration-test"
	if _, err := clients.Core.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating test namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = clients.Core.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	})

	podName := "ingestion-path-probe"
	if _, err := clients.Core.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "probe",
				Image: "registry.k8s.io/pause:3.9",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
				},
			}},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating test pod: %v", err)
	}

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable"
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("store.Open() error = %v (is `docker compose -f infra/docker-compose.yml up -d` running?)", err)
	}
	t.Cleanup(st.Close)

	poller := k8s.NewPoller(clients, []string{namespace}, testLogger())

	deadline := time.Now().Add(90 * time.Second)
	var seen bool
	for time.Now().Before(deadline) {
		ingest.Run(ctx, poller, st, nil, testLogger())

		pods, err := st.ListPods(ctx)
		if err != nil {
			t.Fatalf("ListPods() error = %v", err)
		}
		for _, p := range pods {
			if p.Namespace == namespace && p.Pod == podName {
				seen = true
			}
		}
		if seen {
			break
		}
		time.Sleep(5 * time.Second)
	}

	if !seen {
		t.Fatalf("pod %s/%s never appeared via the real ingestion path within 90s (metrics-server may need longer to scrape a brand-new pod)", namespace, podName)
	}
}
