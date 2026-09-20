package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_SetsCORSAllowOriginOnGET(t *testing.T) {
	router, _, _ := newTestRouterWithOrigin(t, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}
}

func TestRouter_HandlesOPTIONSPreflight(t *testing.T) {
	router, _, _ := newTestRouterWithOrigin(t, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/pods", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}
}
