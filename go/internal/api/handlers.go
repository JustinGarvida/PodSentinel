package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"podsentinel/internal/models"
	"podsentinel/internal/store"
)

// handlers holds the dependencies shared by the API's HTTP handlers.
type handlers struct {
	// logger is the structured logger used to report handler-level errors.
	logger *slog.Logger
	// store is the Postgres-backed source of pod/metric/anomaly data.
	store *store.Store
}

// Purpose: writes body to w as JSON with the given HTTP status code.
// Params:
//   - w: the response writer to write the status and body to.
//   - status: the HTTP status code to send.
//   - body: the value to JSON-encode as the response body.
//
// Returns: nothing; encoding errors are ignored (the response is
// already committed once the status is written).
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Purpose: a liveness check for the agent.
// Params:
//   - w: the response writer.
//   - r: the incoming request (unused).
//
// Returns: nothing; always writes 200 OK.
func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Purpose: returns the latest known summary for every pod that has
// reported a metric within the last hour.
// Params:
//   - w: the response writer.
//   - r: the incoming request (unused beyond its context).
//
// Returns: nothing; writes a JSON array of models.PodSummary, or a
// 500 with a JSON error body if the store query fails.
func (h *handlers) listPods(w http.ResponseWriter, r *http.Request) {
	pods, err := h.store.ListPods(r.Context())
	if err != nil {
		h.logger.Error("listing pods failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list pods"})
		return
	}

	summaries := make([]models.PodSummary, 0, len(pods))
	for _, p := range pods {
		summaries = append(summaries, models.PodSummary{
			Namespace:    p.Namespace,
			Name:         p.Pod,
			UID:          p.PodUID,
			OwnerKind:    p.OwnerKind,
			OwnerName:    p.OwnerName,
			Status:       p.Status,
			RestartCount: int(p.RestartCount),
			CPU:          p.CPU,
			Memory:       p.Memory,
			LastSeen:     p.Time,
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

// Purpose: returns the pod's identity/status (from its most recent
// metric row) plus its last hour of CPU/memory samples, oldest first.
// Params:
//   - w: the response writer.
//   - r: the incoming request; "namespace" and "pod" are read from
//     its URL path parameters.
//
// Returns: nothing; writes a JSON models.PodDetail, or a 500 with a
// JSON error body if the store query fails.
func (h *handlers) getPod(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	rows, err := h.store.GetPodMetrics(r.Context(), namespace, pod)
	if err != nil {
		h.logger.Error("getting pod metrics failed", "namespace", namespace, "pod", pod, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get pod metrics"})
		return
	}

	detail := models.PodDetail{
		Namespace: namespace,
		Name:      pod,
		Metrics:   make([]models.MetricSample, 0, len(rows)),
	}
	for i, row := range rows {
		if i == len(rows)-1 {
			detail.Status = row.Status
			detail.RestartCount = int(row.RestartCount)
		}
		detail.Metrics = append(detail.Metrics, models.MetricSample{
			Timestamp: row.Time,
			CPU:       row.CPU,
			Memory:    row.Memory,
		})
	}
	writeJSON(w, http.StatusOK, detail)
}

// Purpose: returns the pod's anomaly history, most recent first;
// empty (never null) until the Python anomaly detector exists.
// Params:
//   - w: the response writer.
//   - r: the incoming request; "namespace" and "pod" are read from
//     its URL path parameters.
//
// Returns: nothing; writes a JSON array of models.Anomaly, or a 500
// with a JSON error body if the store query fails.
func (h *handlers) listPodAnomalies(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	rows, err := h.store.ListAnomalies(r.Context(), namespace, pod)
	if err != nil {
		h.logger.Error("listing anomalies failed", "namespace", namespace, "pod", pod, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list anomalies"})
		return
	}

	anomalies := make([]models.Anomaly, 0, len(rows))
	for _, a := range rows {
		anomalies = append(anomalies, models.Anomaly{
			Timestamp: a.Time,
			Namespace: a.Namespace,
			Pod:       a.Pod,
			Metric:    a.Metric,
			Value:     a.Value,
			Baseline:  a.Baseline,
			Severity:  a.Severity,
		})
	}
	writeJSON(w, http.StatusOK, anomalies)
}
