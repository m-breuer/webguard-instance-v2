package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/m-breuer/webguard-instance-v2/internal/config"
	"github.com/m-breuer/webguard-instance-v2/internal/core"
	"github.com/m-breuer/webguard-instance-v2/internal/monitor"
)

func TestNormalizeHeaders(t *testing.T) {
	t.Parallel()

	headers := normalizeHeaders(`{"X-Test":"value","X-Int":1}`)
	if headers["X-Test"] != "value" {
		t.Fatalf("expected X-Test header")
	}
	if headers["X-Int"] != "1" {
		t.Fatalf("expected X-Int header to be stringified")
	}

	headers = normalizeHeaders("not-json")
	if len(headers) != 0 {
		t.Fatalf("expected empty headers for invalid json, got %#v", headers)
	}
}

func TestNormalizeBody(t *testing.T) {
	t.Parallel()

	body := normalizeBody(`{"key":"value"}`)
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("expected valid JSON body, got error: %v", err)
	}
	if parsed["key"] != "value" {
		t.Fatalf("unexpected parsed value: %#v", parsed)
	}

	body = normalizeBody("invalid-json")
	if string(body) != "[]" {
		t.Fatalf("expected fallback body [] for invalid JSON string, got %s", string(body))
	}
}

func TestPerformHTTPRequestGETWithHeadersAndBasicAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", request.Method)
		}
		if request.Header.Get("X-Test") != "value" {
			t.Fatalf("expected X-Test header")
		}

		username, password, ok := request.BasicAuth()
		if !ok {
			t.Fatalf("expected basic auth")
		}
		if username != "user" || password != "pass" {
			t.Fatalf("unexpected basic auth credentials")
		}

		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	statusCode, body, err := r.performHTTPRequest(context.Background(), monitor.Monitoring{
		Target:       server.URL,
		Timeout:      2,
		HTTPMethod:   monitor.HTTPMethodGet,
		HTTPHeaders:  `{"X-Test":"value"}`,
		AuthUsername: "user",
		AuthPassword: "pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}
	if body != "ok" {
		t.Fatalf("expected body ok, got %q", body)
	}
}

func TestPerformHTTPRequestPOSTBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", request.Method)
		}

		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed reading body: %v", err)
		}

		var parsed map[string]string
		if err := json.Unmarshal(payload, &parsed); err != nil {
			t.Fatalf("invalid JSON body: %v", err)
		}
		if parsed["key"] != "value" {
			t.Fatalf("unexpected body payload: %#v", parsed)
		}

		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	statusCode, _, err := r.performHTTPRequest(context.Background(), monitor.Monitoring{
		Target:     server.URL,
		Timeout:    2,
		HTTPMethod: monitor.HTTPMethodPost,
		HTTPBody:   `{"key":"value"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", statusCode)
	}
}

func TestPerformHTTPRequestRetriesOnTransportError(t *testing.T) {
	t.Parallel()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	start := time.Now()
	_, _, err := r.performHTTPRequest(context.Background(), monitor.Monitoring{
		Target:     "http://127.0.0.1:1",
		Timeout:    1,
		HTTPMethod: monitor.HTTPMethodGet,
	})
	if err == nil {
		t.Fatalf("expected transport error")
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Fatalf("expected retry delay to be applied")
	}
}

func TestHandlePingMonitoring(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	status, responseTime := handlePingMonitoring(monitor.Monitoring{
		Target: "127.0.0.1",
		Port:   port,
	})
	if status != monitor.StatusUp {
		t.Fatalf("expected up, got %s", status)
	}
	if responseTime == nil {
		t.Fatalf("expected response time")
	}
}

func TestHandlePortMonitoringDown(t *testing.T) {
	t.Parallel()

	status, responseTime := handlePortMonitoring(monitor.Monitoring{
		Target: "127.0.0.1",
		Port:   1,
	})
	if status != monitor.StatusDown {
		t.Fatalf("expected down, got %s", status)
	}
	if responseTime != nil {
		t.Fatalf("expected nil response time for failed port monitoring")
	}
}

func TestCrawlResponseMonitoringUnknownType(t *testing.T) {
	t.Parallel()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	status, responseTime := r.crawlResponseMonitoring(context.Background(), monitor.Monitoring{
		Type: monitor.Type("custom"),
	})
	if status != monitor.StatusUnknown {
		t.Fatalf("expected unknown status, got %s", status)
	}
	if responseTime != nil {
		t.Fatalf("expected nil response time for unknown type")
	}
}

func TestCrawlMonitoringSSLValid(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	payload := r.crawlMonitoringSSL(monitor.Monitoring{
		ID:     "12",
		Target: server.URL,
	})

	if payload.MonitoringID != "12" {
		t.Fatalf("unexpected monitoring id: %s", payload.MonitoringID)
	}
	if !payload.IsValid {
		t.Fatalf("expected certificate to be valid for httptest TLS server")
	}
	if payload.ExpiresAt == nil || payload.IssuedAt == nil {
		t.Fatalf("expected issued/expires timestamps")
	}
}

func TestRunSSLPostsResults(t *testing.T) {
	t.Parallel()

	client := &fakeCoreClient{
		sslMonitorings: []monitor.Monitoring{
			{
				ID:     "3",
				Target: "https://127.0.0.1:" + strconv.Itoa(1),
			},
		},
	}

	cfg := config.Config{
		WebGuardLocation:    "de-1",
		QueueDefaultWorkers: 1,
	}
	r := New(client, cfg, log.New(io.Discard, "", 0))
	if err := r.runSSL(context.Background()); err != nil {
		t.Fatalf("runSSL failed: %v", err)
	}

	client.mu.Lock()
	postedSSL := append([]monitor.SSLResultPayload(nil), client.postedSSL...)
	client.mu.Unlock()

	if len(postedSSL) != 1 {
		t.Fatalf("expected one ssl result post, got %d", len(postedSSL))
	}
	if postedSSL[0].MonitoringID != "3" {
		t.Fatalf("unexpected monitoring id: %s", postedSSL[0].MonitoringID)
	}
}

func TestLogFetchErrorIncludesStatusBody(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	r := New(nil, config.Config{}, log.New(&logs, "", 0))

	r.logFetchError(&core.HTTPStatusError{
		StatusCode: http.StatusForbidden,
		Body:       "forbidden",
	})

	if !bytes.Contains(logs.Bytes(), []byte("Failed to fetch monitorings from the Core API.")) {
		t.Fatalf("expected generic fetch error log, got %q", logs.String())
	}
	if !bytes.Contains(logs.Bytes(), []byte("forbidden")) {
		t.Fatalf("expected response body to be logged, got %q", logs.String())
	}
}
