package mq

import (
	"time"

	"podsentinel/internal/k8s"
)

// MetricMessageSchemaVersion is bumped on any breaking change to
// MetricMessage so the detector can reject payloads it doesn't know.
const MetricMessageSchemaVersion = 1

// MetricMessage is the JSON body of one metrics.raw message: a single
// pod's sample from a single poll cycle.
type MetricMessage struct {
	// SchemaVersion is MetricMessageSchemaVersion at publish time.
	SchemaVersion int `json:"schema_version"`
	// Timestamp is when the sample was taken (see k8s.PodSample).
	Timestamp time.Time `json:"timestamp"`
	// Namespace is the pod's namespace.
	Namespace string `json:"namespace"`
	// Pod is the pod's name.
	Pod string `json:"pod"`
	// PodUID is the pod's Kubernetes UID.
	PodUID string `json:"pod_uid"`
	// OwnerKind is the pod's resolved owner kind, empty for a bare pod.
	OwnerKind string `json:"owner_kind"`
	// OwnerName is the pod's resolved owner name, empty for a bare pod.
	OwnerName string `json:"owner_name"`
	// CPU is the pod's CPU usage in cores.
	CPU float64 `json:"cpu"`
	// Memory is the pod's memory usage in bytes.
	Memory float64 `json:"memory"`
	// Status is the pod's phase.
	Status string `json:"status"`
	// RestartCount is the pod's summed container restart count.
	RestartCount int32 `json:"restart_count"`
}

// Purpose: converts a poll sample into its metrics.raw message body.
// Params:
//   - sample: one pod's joined sample from a poll cycle.
//
// Returns: the MetricMessage for sample.
func NewMetricMessage(sample k8s.PodSample) MetricMessage {
	return MetricMessage{
		SchemaVersion: MetricMessageSchemaVersion,
		Timestamp:     sample.Timestamp,
		Namespace:     sample.Namespace,
		Pod:           sample.Name,
		PodUID:        sample.UID,
		OwnerKind:     sample.OwnerKind,
		OwnerName:     sample.OwnerName,
		CPU:           sample.CPU,
		Memory:        sample.Memory,
		Status:        sample.Status,
		RestartCount:  sample.RestartCount,
	}
}

// Purpose: builds a sample's metrics.raw routing key.
// Params:
//   - namespace: the pod's namespace.
//   - pod: the pod's name.
//
// Returns: "<namespace>.<pod>".
func RoutingKey(namespace, pod string) string {
	return namespace + "." + pod
}
