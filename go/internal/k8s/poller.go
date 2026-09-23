package k8s

import (
	"context"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Poller lists pods and their metrics from the Kubernetes API on each
// Poll call, joining them into PodSamples.
type Poller struct {
	// Clients holds the core and metrics clientsets to poll.
	Clients *Clients
	// Namespaces restricts polling to these namespaces. Empty means
	// watch all namespaces.
	Namespaces []string
	// Logger is the structured logger used for per-namespace list
	// failures and the join's no-metrics-match debug line.
	Logger *slog.Logger
	// Now returns the current time; overridable in tests, defaults to
	// time.Now via NewPoller.
	Now func() time.Time
}

// Purpose: constructs a Poller ready to run against a real cluster.
// Params:
//   - clients: the Kubernetes clientsets to poll.
//   - namespaces: the namespace allow-list (empty means all).
//   - logger: structured logger for the poller and its join step.
//
// Returns: a *Poller with Now set to time.Now.
func NewPoller(clients *Clients, namespaces []string, logger *slog.Logger) *Poller {
	return &Poller{
		Clients:    clients,
		Namespaces: namespaces,
		Logger:     logger,
		Now:        time.Now,
	}
}

// Purpose: lists pods and pod metrics per namespace and returns
// the joined samples; a failed namespace is logged and skipped.
// Params:
//   - ctx: propagated to every Kubernetes API list call.
//
// Returns: the joined PodSamples from every namespace that listed
// successfully; nil if every namespace failed.
func (p *Poller) Poll(ctx context.Context) []PodSample {
	namespaces := p.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	now := p.Now()
	var samples []PodSample

	// Shared for the whole cycle so pods in one ReplicaSet cost a single Get.
	owners := newOwnerCache(p.Clients.Core, p.Logger)
	resolveOwner := func(pod corev1.Pod) (kind, name string) {
		return owners.resolve(ctx, pod.Namespace, pod.OwnerReferences)
	}

	for _, ns := range namespaces {
		pods, err := p.Clients.Core.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			p.Logger.Error("listing pods failed, skipping namespace", "namespace", ns, "error", err)
			continue
		}

		metrics, err := p.Clients.Metrics.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			p.Logger.Error("listing pod metrics failed, skipping namespace", "namespace", ns, "error", err)
			continue
		}

		samples = append(samples, joinPodsAndMetrics(pods.Items, metrics.Items, now, p.Logger, resolveOwner)...)
	}

	return samples
}
