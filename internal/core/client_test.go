package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m-breuer/webguard-instance-v2/internal/monitor"
)

func TestGetMonitoringsIncludesHeaderAndQuery(t *testing.T) {
	t.Parallel()

	var gotAPIKey string
	var gotLocation string
	var gotTypes string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAPIKey = request.Header.Get("X-API-KEY")
		gotLocation = request.URL.Query().Get("location")
		gotTypes = request.URL.Query().Get("types")

		if request.URL.Path != "/api/v1/internal/monitorings" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"id":1,"type":"http","target":"https://example.com","timeout":10}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-key")
	monitorings, err := client.GetMonitorings(context.Background(), "de-1", []monitor.Type{
		monitor.TypeHTTP,
		monitor.TypeKeyword,
		monitor.TypePort,
	})
	if err != nil {
		t.Fatalf("GetMonitorings failed: %v", err)
	}

	if gotAPIKey != "secret-key" {
		t.Fatalf("expected api key secret-key, got %q", gotAPIKey)
	}
	if gotLocation != "de-1" {
		t.Fatalf("expected location=de-1, got %q", gotLocation)
	}
	if gotTypes != "http,keyword,port" {
		t.Fatalf("expected types=http,keyword,port, got %q", gotTypes)
	}
	if len(monitorings) != 1 {
		t.Fatalf("expected 1 monitoring, got %d", len(monitorings))
	}
}

func TestGetMonitoringsSupportsStringIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"id":"123","type":"http","target":"https://example.com","timeout":"10"}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-key")
	monitorings, err := client.GetMonitorings(context.Background(), "de-1", nil)
	if err != nil {
		t.Fatalf("GetMonitorings failed: %v", err)
	}
	if len(monitorings) != 1 {
		t.Fatalf("expected 1 monitoring, got %d", len(monitorings))
	}
	if monitorings[0].ID != 123 {
		t.Fatalf("expected id 123, got %d", monitorings[0].ID)
	}
	if monitorings[0].Timeout != 10 {
		t.Fatalf("expected timeout 10, got %d", monitorings[0].Timeout)
	}
}

func TestPostMonitoringResponsePayloadShape(t *testing.T) {
	t.Parallel()

	var body map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/internal/monitoring-responses" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}

		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-key")
	err := client.PostMonitoringResponse(context.Background(), monitor.MonitoringResponsePayload{
		MonitoringID: 42,
		Status:       monitor.StatusUnknown,
		ResponseTime: nil,
	})
	if err != nil {
		t.Fatalf("PostMonitoringResponse failed: %v", err)
	}

	if body["monitoring_id"] != float64(42) {
		t.Fatalf("expected monitoring_id=42, got %#v", body["monitoring_id"])
	}
	if body["status"] != "unknown" {
		t.Fatalf("expected status=unknown, got %#v", body["status"])
	}
	if value, ok := body["response_time"]; !ok || value != nil {
		t.Fatalf("expected response_time=null, got %#v", body["response_time"])
	}
}

func TestPostSSLResultPayloadShape(t *testing.T) {
	t.Parallel()

	var body map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/internal/ssl-results" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}

		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-key")
	now := time.Now().UTC()
	err := client.PostSSLResult(context.Background(), monitor.SSLResultPayload{
		MonitoringID: 10,
		IsValid:      true,
		ExpiresAt:    &now,
		Issuer:       ptr("issuer"),
		IssuedAt:     &now,
	})
	if err != nil {
		t.Fatalf("PostSSLResult failed: %v", err)
	}

	if body["monitoring_id"] != float64(10) {
		t.Fatalf("expected monitoring_id=10, got %#v", body["monitoring_id"])
	}
	if body["is_valid"] != true {
		t.Fatalf("expected is_valid=true, got %#v", body["is_valid"])
	}
	if body["issuer"] != "issuer" {
		t.Fatalf("expected issuer=issuer, got %#v", body["issuer"])
	}
}

func TestGetMonitoringsReturnsStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-key")
	_, err := client.GetMonitorings(context.Background(), "de-1", nil)
	if err == nil {
		t.Fatalf("expected error")
	}

	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected HTTPStatusError, got %T", err)
	}
	if statusErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", statusErr.StatusCode)
	}
	if statusErr.Body != "unauthorized" {
		t.Fatalf("expected body unauthorized, got %q", statusErr.Body)
	}
}

func TestGetMonitoringsWithoutBaseURLFails(t *testing.T) {
	t.Parallel()

	client := NewClient("", "secret")
	_, err := client.GetMonitorings(context.Background(), "de-1", nil)
	if err == nil {
		t.Fatalf("expected error for empty base URL")
	}
}

func ptr(value string) *string {
	return &value
}
