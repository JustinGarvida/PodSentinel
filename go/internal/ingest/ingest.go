// Package ingest wires the Kubernetes poller to the Postgres store and
// the RabbitMQ publisher, running one poll-persist-publish cycle at a
// time.
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

// Publisher is the subset of *mq.Publisher that ingest depends on.
type Publisher interface {
	// Publish sends a single pod sample to metrics.raw.
	Publish(ctx context.Context, sample k8s.PodSample) error
}

// Purpose: polls once, then persists and publishes every sample. The
// insert and publish are independent — either failing is logged and
// skipped without affecting the other, so Postgres recording survives
// a RabbitMQ outage.
// Params:
//   - ctx: propagated to the poll call and every insert/publish.
//   - poller: source of pod samples for this cycle.
//   - st: destination store for persisted samples.
//   - pub: destination for per-pod metrics.raw messages; nil disables
//     publishing.
//   - logger: structured logger for per-pod insert/publish failures.
//
// Returns: nothing; per-pod failures are logged, not returned.
func Run(ctx context.Context, poller Poller, st Store, pub Publisher, logger *slog.Logger) {
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
		}

		if pub != nil {
			if err := pub.Publish(ctx, sample); err != nil {
				logger.Warn("publishing pod metric failed, skipping", "namespace", sample.Namespace, "pod", sample.Name, "error", err)
			}
		}
	}
}
