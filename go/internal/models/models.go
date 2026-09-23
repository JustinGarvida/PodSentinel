// Package models defines the API's response shapes. They mirror the
// pod_metrics and anomalies tables described in docs/architecture.md,
// so real data can slot in later without changing handler signatures.
package models

import "time"

// PodSummary is a single row in the dashboard's pod list view.
type PodSummary struct {
	// Namespace is the pod's namespace.
	Namespace string `json:"namespace"`
	// Name is the pod's name.
	Name string `json:"name"`
	// UID is the pod's Kubernetes UID, empty for samples predating owner tracking.
	UID string `json:"uid"`
	// OwnerKind is the pod's controlling owner's kind (e.g. "Deployment"), empty for a bare pod.
	OwnerKind string `json:"ownerKind"`
	// OwnerName is the pod's controlling owner's name, empty for a bare pod.
	OwnerName string `json:"ownerName"`
	// Status is the pod's phase (e.g. "Running", "Pending").
	Status string `json:"status"`
	// RestartCount is the pod's total container restart count.
	RestartCount int `json:"restartCount"`
	// CPU is the pod's most recently observed CPU usage in cores.
	CPU float64 `json:"cpu"`
	// Memory is the pod's most recently observed memory usage in bytes.
	Memory float64 `json:"memory"`
	// LastSeen is when the pod's most recent sample was taken.
	LastSeen time.Time `json:"lastSeen"`
}

// MetricSample is a single CPU/memory reading for a pod at a point in time.
type MetricSample struct {
	// Timestamp is when this sample was taken.
	Timestamp time.Time `json:"timestamp"`
	// CPU is the pod's CPU usage in cores at Timestamp.
	CPU float64 `json:"cpu"`
	// Memory is the pod's memory usage in bytes at Timestamp.
	Memory float64 `json:"memory"`
}

// PodDetail is the dashboard's pod detail view: identity plus recent metrics.
type PodDetail struct {
	// Namespace is the pod's namespace.
	Namespace string `json:"namespace"`
	// Name is the pod's name.
	Name string `json:"name"`
	// Status is the pod's most recently observed phase.
	Status string `json:"status"`
	// RestartCount is the pod's most recently observed restart count.
	RestartCount int `json:"restartCount"`
	// Metrics is the pod's recent CPU/memory samples, oldest first.
	Metrics []MetricSample `json:"metrics"`
}

// Anomaly is a single detected deviation for a pod's metric.
type Anomaly struct {
	// Timestamp is when the anomaly was detected.
	Timestamp time.Time `json:"timestamp"`
	// Namespace is the affected pod's namespace.
	Namespace string `json:"namespace"`
	// Pod is the affected pod's name.
	Pod string `json:"pod"`
	// Metric is the name of the deviating metric (e.g. "cpu").
	Metric string `json:"metric"`
	// Value is the observed value that triggered the anomaly.
	Value float64 `json:"value"`
	// Baseline is the pod's expected value the anomaly deviated from.
	Baseline float64 `json:"baseline"`
	// Severity is the anomaly's severity classification.
	Severity string `json:"severity"`
}
