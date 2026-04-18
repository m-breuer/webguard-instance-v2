package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestPerformHTTPRequestFollowsRedirectAcrossHosts(t *testing.T) {
	t.Parallel()

	targetServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("redirect-ok"))
	}))
	defer targetServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, targetServer.URL, http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	statusCode, body, err := r.performHTTPRequest(context.Background(), monitor.Monitoring{
		Target:     redirectServer.URL,
		Timeout:    2,
		HTTPMethod: monitor.HTTPMethodGet,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected final status 200 after redirect, got %d", statusCode)
	}
	if body != "redirect-ok" {
		t.Fatalf("expected redirected response body, got %q", body)
	}
}

func TestHandleHTTPMonitoringTreatsRedirectStatusAsUp(t *testing.T) {
	t.Parallel()

	redirectOnlyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusMovedPermanently)
	}))
	defer redirectOnlyServer.Close()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	status, responseTime, httpStatusCode := r.handleHTTPMonitoring(context.Background(), monitor.Monitoring{
		Target:     redirectOnlyServer.URL,
		Timeout:    2,
		HTTPMethod: monitor.HTTPMethodGet,
	})

	if status != monitor.StatusUp {
		t.Fatalf("expected up for redirect response, got %s", status)
	}
	if responseTime == nil {
		t.Fatalf("expected response time for redirect response")
	}
	if httpStatusCode == nil {
		t.Fatalf("expected http status code")
	}
	if *httpStatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected http status code 301, got %d", *httpStatusCode)
	}
}

func TestHandleKeywordMonitoringReturnsHTTPStatusCodeWhenKeywordMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte("different-content"))
	}))
	defer server.Close()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	status, responseTime, httpStatusCode := r.handleKeywordMonitoring(context.Background(), monitor.Monitoring{
		Target:     server.URL,
		Timeout:    2,
		HTTPMethod: monitor.HTTPMethodGet,
		Keyword:    "needle",
	})

	if status != monitor.StatusDown {
		t.Fatalf("expected down when keyword is missing, got %s", status)
	}
	if responseTime != nil {
		t.Fatalf("expected nil response time when keyword is missing, got %v", *responseTime)
	}
	if httpStatusCode == nil {
		t.Fatalf("expected http status code")
	}
	if *httpStatusCode != http.StatusTeapot {
		t.Fatalf("expected http status code %d, got %d", http.StatusTeapot, *httpStatusCode)
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

func TestHandlePingMonitoringSupportsHostnameAndIPTargets(t *testing.T) {
	originalExecutor := pingExecutor
	t.Cleanup(func() {
		pingExecutor = originalExecutor
	})

	testCases := []struct {
		name   string
		target string
	}{
		{name: "hostname", target: "example.com"},
		{name: "ipv4", target: "8.8.8.8"},
		{name: "ipv6", target: "2001:4860:4860::8888"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			var receivedHost string
			var receivedTimeout int
			pingExecutor = func(_ context.Context, host string, timeoutSeconds int) ([]byte, error) {
				receivedHost = host
				receivedTimeout = timeoutSeconds
				return []byte("64 bytes from " + host + ": icmp_seq=1 ttl=57 time=12.34 ms"), nil
			}

			status, responseTime := handlePingMonitoring(monitor.Monitoring{
				Target:  testCase.target,
				Timeout: 2,
			})

			if status != monitor.StatusUp {
				t.Fatalf("expected up, got %s", status)
			}
			if responseTime == nil {
				t.Fatalf("expected response time")
			}
			if *responseTime != 12.34 {
				t.Fatalf("expected response time 12.34, got %v", *responseTime)
			}
			if receivedHost != testCase.target {
				t.Fatalf("expected ping target %q, got %q", testCase.target, receivedHost)
			}
			if receivedTimeout != 2 {
				t.Fatalf("expected timeout 2, got %d", receivedTimeout)
			}
		})
	}
}

func TestHandlePingMonitoringDown(t *testing.T) {
	originalExecutor := pingExecutor
	t.Cleanup(func() {
		pingExecutor = originalExecutor
	})

	pingExecutor = func(_ context.Context, _ string, _ int) ([]byte, error) {
		return []byte("100% packet loss"), errors.New("exit status 1")
	}

	status, responseTime := handlePingMonitoring(monitor.Monitoring{
		Target: "8.8.8.8",
	})
	if status != monitor.StatusDown {
		t.Fatalf("expected down, got %s", status)
	}
	if responseTime == nil {
		t.Fatalf("expected fallback response time")
	}
}

func TestBuildPingCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		host     string
		timeout  int
		expected []string
	}{
		{
			name:     "hostname",
			host:     "example.com",
			timeout:  5,
			expected: []string{"-c", "1", "-W", "5", "example.com"},
		},
		{
			name:     "ipv4",
			host:     "8.8.8.8",
			timeout:  3,
			expected: []string{"-c", "1", "-W", "3", "-4", "8.8.8.8"},
		},
		{
			name:     "ipv6",
			host:     "2001:4860:4860::8888",
			timeout:  4,
			expected: []string{"-c", "1", "-W", "4", "-6", "2001:4860:4860::8888"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			command, args := buildPingCommand(testCase.host, testCase.timeout)
			if command != "ping" {
				t.Fatalf("expected ping command, got %q", command)
			}
			if !reflect.DeepEqual(args, testCase.expected) {
				t.Fatalf("unexpected ping args: got %#v want %#v", args, testCase.expected)
			}
		})
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
	status, responseTime, httpStatusCode := r.crawlResponseMonitoring(context.Background(), monitor.Monitoring{
		Type: monitor.Type("custom"),
	})
	if status != monitor.StatusUnknown {
		t.Fatalf("expected unknown status, got %s", status)
	}
	if responseTime != nil {
		t.Fatalf("expected nil response time for unknown type")
	}
	if httpStatusCode != nil {
		t.Fatalf("expected nil http status code for unknown type")
	}
}

func TestCrawlResponseMonitoringPortReturnsNilHTTPStatusCode(t *testing.T) {
	t.Parallel()

	server, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open listener: %v", err)
	}
	defer server.Close()

	_, portRaw, err := net.SplitHostPort(server.Addr().String())
	if err != nil {
		t.Fatalf("failed to split listener address: %v", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatalf("failed to parse listener port: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := server.Accept()
		if acceptErr == nil && conn != nil {
			_ = conn.Close()
		}
	}()

	r := New(nil, config.Config{}, log.New(io.Discard, "", 0))
	status, _, httpStatusCode := r.crawlResponseMonitoring(context.Background(), monitor.Monitoring{
		Type:   monitor.TypePort,
		Target: "127.0.0.1",
		Port:   port,
	})

	if status != monitor.StatusUp {
		t.Fatalf("expected up status for open port, got %s", status)
	}
	if httpStatusCode != nil {
		t.Fatalf("expected nil http status code for port monitoring")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("port monitor did not connect to test listener")
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
				Type:   monitor.TypeHTTP,
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
