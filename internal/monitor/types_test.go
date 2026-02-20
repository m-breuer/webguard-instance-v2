package monitor

import (
	"encoding/json"
	"testing"
)

func TestMonitoringUnmarshalWithNumericID(t *testing.T) {
	t.Parallel()

	var monitoring Monitoring
	err := json.Unmarshal([]byte(`{
		"id": 42,
		"type": "http",
		"target": "https://example.com",
		"timeout": 10,
		"maintenance_active": false
	}`), &monitoring)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if monitoring.ID != "42" {
		t.Fatalf("expected id 42, got %s", monitoring.ID)
	}
}

func TestMonitoringUnmarshalWithStringID(t *testing.T) {
	t.Parallel()

	var monitoring Monitoring
	err := json.Unmarshal([]byte(`{
		"id": "42",
		"type": "http",
		"target": "https://example.com",
		"timeout": "10",
		"port": "443",
		"maintenance_active": "true"
	}`), &monitoring)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if monitoring.ID != "42" {
		t.Fatalf("expected id 42, got %s", monitoring.ID)
	}
	if monitoring.Timeout != 10 {
		t.Fatalf("expected timeout 10, got %d", monitoring.Timeout)
	}
	if monitoring.Port != 443 {
		t.Fatalf("expected port 443, got %d", monitoring.Port)
	}
	if !monitoring.MaintenanceActive {
		t.Fatalf("expected maintenance_active=true")
	}
}

func TestMonitoringUnmarshalWithInvalidID(t *testing.T) {
	t.Parallel()

	var monitoring Monitoring
	err := json.Unmarshal([]byte(`{
		"id": {"nested": true},
		"type": "http",
		"target": "https://example.com"
	}`), &monitoring)
	if err == nil {
		t.Fatalf("expected error for invalid id")
	}
}
