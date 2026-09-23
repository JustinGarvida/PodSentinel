package k8s

import (
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// PodSample is one pod's joined identity, status, and resource usage
// for a single poll cycle.
type PodSample struct {
	// Namespace is the pod's namespace.
	Namespace string
	// Name is the pod's name.
	Name string
	// UID is the pod object's Kubernetes UID — stable for this pod's
	// whole lifetime and never reused, unlike Name (a crashed pod under
	// a Deployment is deleted and replaced by a new Pod object with a
	// new name and UID). Distinguishing "same pod restarted" from
	// "pod replaced" requires this, not just Name.
	UID string
	// OwnerKind is the pod's controlling owner's kind (e.g.
	// "Deployment", "StatefulSet", "DaemonSet"), resolved by following
	// a ReplicaSet owner up to its own Deployment where possible. Empty
	// for a bare pod with no controller.
	OwnerKind string
	// OwnerName is the pod's controlling owner's name, in the same
	// terms as OwnerKind. Empty for a bare pod with no controller.
	OwnerName string
	// Status is the pod's phase (e.g. "Running", "Pending").
	Status string
	// RestartCount is the sum of restart counts across the pod's
	// containers.
	RestartCount int32
	// CPU is the pod's total CPU usage in cores, summed across
	// containers. Zero if no matching metrics entry was found.
	CPU float64
	// Memory is the pod's total memory usage in bytes, summed across
	// containers. Zero if no matching metrics entry was found.
	Memory float64
	// Timestamp is when this sample was taken: metrics-server's own
	// scrape time when metrics were found, otherwise the poll's
	// wall-clock time.
	Timestamp time.Time
}

// Purpose: joins pod identity/status with resource usage; a pod
// with no metrics entry gets a zeroed sample (logged at Debug).
// Params:
//   - pods: the core API's pod list for the polled namespace(s).
//   - metrics: the metrics API's pod metrics list for the same scope.
//   - now: the poll's wall-clock time, used as a fallback Timestamp.
//   - logger: structured logger for the no-metrics-match debug line.
//   - resolveOwner: resolves a pod's controlling owner (kind, name);
//     injected so this stays a pure function in tests (see poller.go
//     for the real, API-backed resolver).
//
// Returns: one PodSample per pod in pods, in the same order.
func joinPodsAndMetrics(pods []corev1.Pod, metrics []metricsv1beta1.PodMetrics, now time.Time, logger *slog.Logger, resolveOwner func(pod corev1.Pod) (kind, name string)) []PodSample {
	metricsByKey := make(map[string]metricsv1beta1.PodMetrics, len(metrics))
	for _, m := range metrics {
		metricsByKey[m.Namespace+"/"+m.Name] = m
	}

	samples := make([]PodSample, 0, len(pods))
	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		m, ok := metricsByKey[key]

		var cpu, memory float64
		timestamp := now
		if ok {
			for _, c := range m.Containers {
				cpu += c.Usage.Cpu().AsApproximateFloat64()
				memory += c.Usage.Memory().AsApproximateFloat64()
			}
			// Prefer metrics-server's own scrape timestamp over the poll
			// wall-clock: metrics-server serves cached values on its own
			// ~60s cadence, so at the default 15s poll interval several
			// consecutive polls can really be the same underlying
			// reading. Using the scrape time (rather than "now" for
			// every poll) avoids deflating variance for the z-score/EWMA
			// anomaly detector. Fall back to now if unset.
			if !m.Timestamp.Time.IsZero() {
				timestamp = m.Timestamp.Time
			}
		} else {
			logger.Debug("no metrics for pod, recording with zeroed usage", "namespace", pod.Namespace, "pod", pod.Name)
		}

		var restarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}

		ownerKind, ownerName := resolveOwner(pod)

		samples = append(samples, PodSample{
			Namespace:    pod.Namespace,
			Name:         pod.Name,
			UID:          string(pod.UID),
			OwnerKind:    ownerKind,
			OwnerName:    ownerName,
			Status:       string(pod.Status.Phase),
			RestartCount: restarts,
			CPU:          cpu,
			Memory:       memory,
			Timestamp:    timestamp,
		})
	}

	return samples
}
