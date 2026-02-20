package runner

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/m-breuer/webguard-instance-v2/internal/config"
	"github.com/m-breuer/webguard-instance-v2/internal/monitor"
)

type getMonitoringsCall struct {
	location string
	types    []monitor.Type
}

type fakeCoreClient struct {
	mu sync.Mutex

	responseMonitorings []monitor.Monitoring
	sslMonitorings      []monitor.Monitoring

	calls []getMonitoringsCall

	postedResponses []monitor.MonitoringResponsePayload
	postedSSL       []monitor.SSLResultPayload
}

func (f *fakeCoreClient) GetMonitorings(_ context.Context, location string, types []monitor.Type) ([]monitor.Monitoring, error) {
	f.mu.Lock()
	f.calls = append(f.calls, getMonitoringsCall{
		location: location,
		types:    append([]monitor.Type(nil), types...),
	})
	f.mu.Unlock()

	if len(types) == 0 {
		return append([]monitor.Monitoring(nil), f.responseMonitorings...), nil
	}

	return append([]monitor.Monitoring(nil), f.sslMonitorings...), nil
}

func (f *fakeCoreClient) PostMonitoringResponse(_ context.Context, payload monitor.MonitoringResponsePayload) error {
	f.mu.Lock()
	f.postedResponses = append(f.postedResponses, payload)
	f.mu.Unlock()
	return nil
}

func (f *fakeCoreClient) PostSSLResult(_ context.Context, payload monitor.SSLResultPayload) error {
	f.mu.Lock()
	f.postedSSL = append(f.postedSSL, payload)
	f.mu.Unlock()
	return nil
}

func (f *fakeCoreClient) snapshotCalls() []getMonitoringsCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]getMonitoringsCall(nil), f.calls...)
}

func (f *fakeCoreClient) snapshotPostedResponses() []monitor.MonitoringResponsePayload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]monitor.MonitoringResponsePayload(nil), f.postedResponses...)
}

func TestRunMonitoringMaintenancePostsUnknown(t *testing.T) {
	t.Parallel()

	client := &fakeCoreClient{
		responseMonitorings: []monitor.Monitoring{
			{
				ID:                "7",
				MaintenanceActive: true,
			},
		},
		sslMonitorings: []monitor.Monitoring{},
	}

	cfg := config.Config{
		WebGuardLocation:    "de-1",
		QueueDefaultWorkers: 1,
	}
	runner := New(client, cfg, log.New(io.Discard, "", 0))

	if err := runner.RunMonitoring(context.Background()); err != nil {
		t.Fatalf("RunMonitoring failed: %v", err)
	}

	calls := client.snapshotCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 monitoring fetch calls, got %d", len(calls))
	}

	var foundResponseFetch bool
	var foundSSLFetch bool
	for _, call := range calls {
		if call.location != "de-1" {
			t.Fatalf("expected location de-1, got %q", call.location)
		}

		if len(call.types) == 0 {
			foundResponseFetch = true
			continue
		}

		if len(call.types) == 3 &&
			call.types[0] == monitor.TypeHTTP &&
			call.types[1] == monitor.TypeKeyword &&
			call.types[2] == monitor.TypePort {
			foundSSLFetch = true
			continue
		}

		t.Fatalf("unexpected type filter: %#v", call.types)
	}

	if !foundResponseFetch {
		t.Fatalf("response fetch call missing")
	}
	if !foundSSLFetch {
		t.Fatalf("ssl fetch call missing")
	}

	postedResponses := client.snapshotPostedResponses()
	if len(postedResponses) != 1 {
		t.Fatalf("expected 1 posted response, got %d", len(postedResponses))
	}
	payload := postedResponses[0]
	if payload.MonitoringID != "7" {
		t.Fatalf("expected monitoring_id 7, got %s", payload.MonitoringID)
	}
	if payload.Status != monitor.StatusUnknown {
		t.Fatalf("expected unknown status, got %s", payload.Status)
	}
	if payload.ResponseTime != nil {
		t.Fatalf("expected nil response_time, got %v", *payload.ResponseTime)
	}
}

func TestRunMonitoringRequestsNonPingTypesForSSL(t *testing.T) {
	t.Parallel()

	client := &fakeCoreClient{
		responseMonitorings: []monitor.Monitoring{},
		sslMonitorings:      []monitor.Monitoring{},
	}
	cfg := config.Config{
		WebGuardLocation:    "us-1",
		QueueDefaultWorkers: 1,
	}
	runner := New(client, cfg, log.New(io.Discard, "", 0))

	if err := runner.RunMonitoring(context.Background()); err != nil {
		t.Fatalf("RunMonitoring failed: %v", err)
	}

	calls := client.snapshotCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 monitoring fetch calls, got %d", len(calls))
	}

	var foundSSLFetch bool
	for _, call := range calls {
		if call.location != "us-1" {
			t.Fatalf("expected location us-1, got %q", call.location)
		}
		if len(call.types) == 3 &&
			call.types[0] == monitor.TypeHTTP &&
			call.types[1] == monitor.TypeKeyword &&
			call.types[2] == monitor.TypePort {
			foundSSLFetch = true
		}
	}

	if !foundSSLFetch {
		t.Fatalf("ssl types fetch missing")
	}
}

type parallelPhasesClient struct {
	started chan string
	release chan struct{}
}

func (p *parallelPhasesClient) GetMonitorings(_ context.Context, _ string, types []monitor.Type) ([]monitor.Monitoring, error) {
	phase := "response"
	if len(types) > 0 {
		phase = "ssl"
	}
	p.started <- phase
	<-p.release
	return []monitor.Monitoring{}, nil
}

func (p *parallelPhasesClient) PostMonitoringResponse(_ context.Context, _ monitor.MonitoringResponsePayload) error {
	return nil
}

func (p *parallelPhasesClient) PostSSLResult(_ context.Context, _ monitor.SSLResultPayload) error {
	return nil
}

func TestRunMonitoringRunsPhasesInParallel(t *testing.T) {
	t.Parallel()

	client := &parallelPhasesClient{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}

	cfg := config.Config{
		WebGuardLocation:    "de-1",
		QueueDefaultWorkers: 1,
	}
	runner := New(client, cfg, log.New(io.Discard, "", 0))

	done := make(chan struct{})
	go func() {
		_ = runner.RunMonitoring(context.Background())
		close(done)
	}()

	timeout := time.After(500 * time.Millisecond)
	startedPhases := map[string]bool{}
	for len(startedPhases) < 2 {
		select {
		case phase := <-client.started:
			startedPhases[phase] = true
		case <-timeout:
			t.Fatalf("expected both phases to start in parallel, got: %#v", startedPhases)
		}
	}

	close(client.release)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("RunMonitoring did not finish after releasing blocked phases")
	}
}
