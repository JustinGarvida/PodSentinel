package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PodMetricRow is one poll cycle's persisted sample for a pod.
type PodMetricRow struct {
	// Time is when this sample was taken.
	Time time.Time
	// Namespace is the pod's namespace.
	Namespace string
	// Pod is the pod's name.
	Pod string
	// PodUID is the pod object's Kubernetes UID — stable for this
	// pod's whole lifetime and never reused, unlike Pod (a crashed pod
	// under a Deployment is deleted and replaced by a new Pod object
	// with a new name and UID). Empty for samples predating this
	// column (existing rows aren't backfilled).
	PodUID string
	// OwnerKind is the pod's controlling owner's kind at Time (e.g.
	// "Deployment", "StatefulSet", "DaemonSet"), or empty for a bare
	// pod with no controller, or a sample predating this column.
	OwnerKind string
	// OwnerName is the pod's controlling owner's name at Time, in the
	// same terms as OwnerKind.
	OwnerName string
	// CPU is the pod's CPU usage in cores at Time.
	CPU float64
	// Memory is the pod's memory usage in bytes at Time.
	Memory float64
	// Status is the pod's phase at Time (e.g. "Running", "Pending").
	Status string
	// RestartCount is the pod's total container restart count at Time.
	RestartCount int32
}

// PodSummary is the latest known row for one pod — the dashboard's pod
// list view.
type PodSummary struct {
	// Namespace is the pod's namespace.
	Namespace string
	// Pod is the pod's name.
	Pod string
	// PodUID is the pod object's Kubernetes UID at the most recent
	// sample. See PodMetricRow.PodUID.
	PodUID string
	// OwnerKind is the pod's controlling owner's kind at the most
	// recent sample. See PodMetricRow.OwnerKind.
	OwnerKind string
	// OwnerName is the pod's controlling owner's name at the most
	// recent sample. See PodMetricRow.OwnerName.
	OwnerName string
	// Status is the pod's most recently observed phase.
	Status string
	// RestartCount is the pod's most recently observed restart count.
	RestartCount int32
	// CPU is the pod's most recently observed CPU usage in cores.
	CPU float64
	// Memory is the pod's most recently observed memory usage in bytes.
	Memory float64
	// Time is when the most recent sample was taken.
	Time time.Time
}

// Anomaly is a single detected deviation for a pod's metric.
type Anomaly struct {
	// Time is when the anomaly was detected.
	Time time.Time
	// Namespace is the affected pod's namespace.
	Namespace string
	// Pod is the affected pod's name.
	Pod string
	// Metric is the name of the deviating metric (e.g. "cpu").
	Metric string
	// Value is the observed value that triggered the anomaly.
	Value float64
	// Baseline is the pod's expected value the anomaly deviated from.
	Baseline float64
	// Severity is the anomaly's severity classification.
	Severity string
}

// Store persists and queries pod metrics and anomalies in Postgres.
type Store struct {
	// pool is the underlying Postgres connection pool.
	pool *pgxpool.Pool
}

// Purpose: connects to Postgres and applies pending schema
// migrations.
// Params:
//   - ctx: used for the connection pool's setup.
//   - dsn: the Postgres connection string.
//
// Returns: a ready-to-use *Store, or an error if migration or
// connection failed.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if err := Migrate(dsn); err != nil {
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Purpose: shuts down the Postgres connection pool.
// Params: none.
// Returns: nothing.
func (s *Store) Close() {
	s.pool.Close()
}

// Purpose: writes a single row to pod_metrics.
// Params:
//   - ctx: used for the insert.
//   - row: the sample to persist.
//
// Returns: nil on success, or an error if the insert failed.
func (s *Store) InsertPodMetric(ctx context.Context, row PodMetricRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pod_metrics (time, namespace, pod, pod_uid, owner_kind, owner_name, cpu, memory, status, restart_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, row.Time, row.Namespace, row.Pod, row.PodUID, row.OwnerKind, row.OwnerName, row.CPU, row.Memory, row.Status, row.RestartCount)
	if err != nil {
		return fmt.Errorf("inserting pod metric for %s/%s: %w", row.Namespace, row.Pod, err)
	}
	return nil
}

// Purpose: returns the latest known row for every pod with a
// metric in the last hour; older-deleted pods age out of the list.
// Params:
//   - ctx: used for the query.
//
// Returns: one PodSummary per pod with a recent sample, or an error
// if the query failed.
func (s *Store) ListPods(ctx context.Context) ([]PodSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (namespace, pod)
			namespace, pod, pod_uid, owner_kind, owner_name, status, restart_count, cpu, memory, time
		FROM pod_metrics
		WHERE time >= now() - interval '1 hour'
		ORDER BY namespace, pod, time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}
	defer rows.Close()

	var summaries []PodSummary
	for rows.Next() {
		var summary PodSummary
		if err := rows.Scan(&summary.Namespace, &summary.Pod, &summary.PodUID, &summary.OwnerKind, &summary.OwnerName, &summary.Status, &summary.RestartCount, &summary.CPU, &summary.Memory, &summary.Time); err != nil {
			return nil, fmt.Errorf("scanning pod summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// Purpose: returns a pod's metric samples from the last hour, oldest
// first.
// Params:
//   - ctx: used for the query.
//   - namespace: the pod's namespace.
//   - pod: the pod's name.
//
// Returns: the pod's samples within the last hour, oldest first, or
// an error if the query failed.
func (s *Store) GetPodMetrics(ctx context.Context, namespace, pod string) ([]PodMetricRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, namespace, pod, pod_uid, owner_kind, owner_name, cpu, memory, status, restart_count
		FROM pod_metrics
		WHERE namespace = $1 AND pod = $2 AND time >= now() - interval '1 hour'
		ORDER BY time ASC
	`, namespace, pod)
	if err != nil {
		return nil, fmt.Errorf("getting metrics for %s/%s: %w", namespace, pod, err)
	}
	defer rows.Close()

	var samples []PodMetricRow
	for rows.Next() {
		var r PodMetricRow
		if err := rows.Scan(&r.Time, &r.Namespace, &r.Pod, &r.PodUID, &r.OwnerKind, &r.OwnerName, &r.CPU, &r.Memory, &r.Status, &r.RestartCount); err != nil {
			return nil, fmt.Errorf("scanning pod metric row: %w", err)
		}
		samples = append(samples, r)
	}
	return samples, rows.Err()
}

// Purpose: returns a pod's anomaly history, most recent first; an
// empty (not nil) slice until the Python detector writes to it.
// Params:
//   - ctx: used for the query.
//   - namespace: the pod's namespace.
//   - pod: the pod's name.
//
// Returns: the pod's anomalies, most recent first (possibly empty),
// or an error if the query failed.
func (s *Store) ListAnomalies(ctx context.Context, namespace, pod string) ([]Anomaly, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, namespace, pod, metric, value, baseline, severity
		FROM anomalies
		WHERE namespace = $1 AND pod = $2
		ORDER BY time DESC
	`, namespace, pod)
	if err != nil {
		return nil, fmt.Errorf("listing anomalies for %s/%s: %w", namespace, pod, err)
	}
	defer rows.Close()

	anomalies := []Anomaly{}
	for rows.Next() {
		var a Anomaly
		if err := rows.Scan(&a.Time, &a.Namespace, &a.Pod, &a.Metric, &a.Value, &a.Baseline, &a.Severity); err != nil {
			return nil, fmt.Errorf("scanning anomaly row: %w", err)
		}
		anomalies = append(anomalies, a)
	}
	return anomalies, rows.Err()
}
