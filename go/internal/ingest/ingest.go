// Package ingest wires the Kubernetes poller to the Postgres store,
// running one poll-and-persist cycle at a time.
package ingest

import (
	"context"
	"log/slog"

	"podsentinel/internal/k8s"
	"podsentinel/internal/store"
)

// Poller is the subset of *k8s.Poller that ingest depends on.
type Poller interface {
	// Poll returns one poll cycle's joined pod samples.
	Poll(ctx context.Context) []k8s.PodSample
}

// Store is the subset of *store.Store that ingest depends on.
type Store interface {
	// InsertPodMetric persists a single pod sample.
	InsertPodMetric(ctx context.Context, row store.PodMetricRow) error
}

// Purpose: polls once and persists every sample; a failed insert is
// logged and skipped, not fatal — matches polling's fault isolation.
// Params:
//   - ctx: propagated to the poll call and every insert.
//   - poller: source of pod samples for this cycle.
//   - st: destination store for persisted samples.
//   - logger: structured logger for per-pod insert failures.
//
// Returns: nothing; per-pod failures are logged, not returned.
func Run(ctx context.Context, poller Poller, st Store, logger *slog.Logger) {
	samples := poller.Poll(ctx)

	for _, sample := range samples {
		row := store.PodMetricRow{
			Time:         sample.Timestamp,
			Namespace:    sample.Namespace,
			Pod:          sample.Name,
			PodUID:       sample.UID,
			OwnerKind:    sample.OwnerKind,
			OwnerName:    sample.OwnerName,
			CPU:          sample.CPU,
			Memory:       sample.Memory,
			Status:       sample.Status,
			RestartCount: sample.RestartCount,
		}

		if err := st.InsertPodMetric(ctx, row); err != nil {
			logger.Error("persisting pod metric failed, skipping", "namespace", sample.Namespace, "pod", sample.Name, "error", err)
			continue
		}
	}
}
