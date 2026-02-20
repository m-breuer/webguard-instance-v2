package runner

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/m-breuer/webguard-instance-v2/internal/config"
	"github.com/m-breuer/webguard-instance-v2/internal/core"
	"github.com/m-breuer/webguard-instance-v2/internal/monitor"
	"github.com/m-breuer/webguard-instance-v2/internal/target"
)

const fixedHTTPRetryTimes = 1
const fixedHTTPRetryDelay = 250 * time.Millisecond
const fixedHTTPMaxRedirects = 5

type CoreClient interface {
	GetMonitorings(ctx context.Context, location string, types []monitor.Type) ([]monitor.Monitoring, error)
	PostMonitoringResponse(ctx context.Context, payload monitor.MonitoringResponsePayload) error
	PostSSLResult(ctx context.Context, payload monitor.SSLResultPayload) error
}

type Runner struct {
	client CoreClient
	cfg    config.Config
	logger *log.Logger
}

func New(client CoreClient, cfg config.Config, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Runner{
		client: client,
		cfg:    cfg,
		logger: logger,
	}
}

func (r *Runner) runResponse(ctx context.Context) error {
	r.logger.Println("Dispatching response monitoring jobs...")

	monitorings, err := r.client.GetMonitorings(ctx, r.cfg.WebGuardLocation, nil)
	if err != nil {
		r.logFetchError(err)
		return err
	}

	if len(monitorings) == 0 {
		r.logger.Println("No active response monitoring found.")
		return nil
	}

	dispatched := 0
	skippedMaintenance := 0

	jobs := make(chan monitor.Monitoring)
	var workers sync.WaitGroup

	workerCount := max(1, r.cfg.QueueDefaultWorkers)
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for monitoring := range jobs {
				status, responseTime := r.crawlResponseMonitoring(ctx, monitoring)
				if err := r.client.PostMonitoringResponse(ctx, monitor.MonitoringResponsePayload{
					MonitoringID: monitoring.ID,
					Status:       status,
					ResponseTime: responseTime,
				}); err != nil {
					r.logger.Printf("Failed to post response result (monitoring_id=%s): %v", monitoring.ID, err)
				}
			}
		}()
	}

	for _, monitoring := range monitorings {
		if monitoring.MaintenanceActive {
			skippedMaintenance++
			if err := r.client.PostMonitoringResponse(ctx, monitor.MonitoringResponsePayload{
				MonitoringID: monitoring.ID,
				Status:       monitor.StatusUnknown,
				ResponseTime: nil,
			}); err != nil {
				r.logger.Printf("Failed to post maintenance response result (monitoring_id=%s): %v", monitoring.ID, err)
			}
			continue
		}

		dispatched++
		jobs <- monitoring
	}
	close(jobs)
	workers.Wait()

	r.logger.Printf(
		"Response monitoring dispatch done. total=%d dispatched=%d skipped_maintenance=%d",
		len(monitorings),
		dispatched,
		skippedMaintenance,
	)

	return nil
}

func (r *Runner) runSSL(ctx context.Context) error {
	r.logger.Println("Dispatching SSL monitoring jobs...")

	types := []monitor.Type{monitor.TypeHTTP, monitor.TypeKeyword, monitor.TypePort}
	monitorings, err := r.client.GetMonitorings(ctx, r.cfg.WebGuardLocation, types)
	if err != nil {
		r.logFetchError(err)
		return err
	}

	if len(monitorings) == 0 {
		r.logger.Println("No active SSL monitoring found.")
		return nil
	}

	dispatched := 0
	skippedMaintenance := 0

	jobs := make(chan monitor.Monitoring)
	var workers sync.WaitGroup

	workerCount := max(1, r.cfg.QueueDefaultWorkers)
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for monitoring := range jobs {
				payload := r.crawlMonitoringSSL(monitoring)
				if err := r.client.PostSSLResult(ctx, payload); err != nil {
					r.logger.Printf("Failed to post SSL result (monitoring_id=%s): %v", monitoring.ID, err)
				}
			}
		}()
	}

	for _, monitoring := range monitorings {
		if monitoring.MaintenanceActive {
			skippedMaintenance++
			continue
		}
		dispatched++
		jobs <- monitoring
	}
	close(jobs)
	workers.Wait()

	r.logger.Printf(
		"SSL monitoring dispatch done. total=%d dispatched=%d skipped_maintenance=%d",
		len(monitorings),
		dispatched,
		skippedMaintenance,
	)

	return nil
}

func (r *Runner) RunMonitoring(ctx context.Context) error {
	r.logger.Println("Dispatching all monitoring jobs...")

	var responseErr error
	var sslErr error
	var phases sync.WaitGroup
	phases.Add(2)

	go func() {
		defer phases.Done()
		responseErr = r.runResponse(ctx)
	}()

	go func() {
		defer phases.Done()
		sslErr = r.runSSL(ctx)
	}()

	phases.Wait()

	if responseErr != nil {
		r.logger.Printf("response monitoring phase failed: %v", responseErr)
	}
	if sslErr != nil {
		r.logger.Printf("SSL monitoring phase failed: %v", sslErr)
	}

	r.logger.Println("All monitoring jobs have been dispatched successfully.")
	return nil
}

func (r *Runner) logFetchError(err error) {
	r.logger.Println("Failed to fetch monitorings from the Core API.")

	var statusError *core.HTTPStatusError
	if errors.As(err, &statusError) && strings.TrimSpace(statusError.Body) != "" {
		r.logger.Println(statusError.Body)
	}
}

func (r *Runner) crawlResponseMonitoring(ctx context.Context, monitoring monitor.Monitoring) (monitor.Status, *float64) {
	switch monitoring.Type {
	case monitor.TypeHTTP:
		return r.handleHTTPMonitoring(ctx, monitoring)
	case monitor.TypePing:
		return handlePingMonitoring(monitoring)
	case monitor.TypeKeyword:
		return r.handleKeywordMonitoring(ctx, monitoring)
	case monitor.TypePort:
		return handlePortMonitoring(monitoring)
	default:
		return monitor.StatusUnknown, nil
	}
}

func (r *Runner) handleHTTPMonitoring(ctx context.Context, monitoring monitor.Monitoring) (monitor.Status, *float64) {
	start := time.Now()
	statusCode, _, err := r.performHTTPRequest(ctx, monitoring)
	if err != nil {
		return monitor.StatusDown, nil
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusBadRequest {
		responseTime := roundMilliseconds(time.Since(start))
		return monitor.StatusUp, &responseTime
	}
	return monitor.StatusDown, nil
}

func (r *Runner) handleKeywordMonitoring(ctx context.Context, monitoring monitor.Monitoring) (monitor.Status, *float64) {
	start := time.Now()
	_, body, err := r.performHTTPRequest(ctx, monitoring)
	if err != nil {
		return monitor.StatusDown, nil
	}
	if strings.Contains(body, monitoring.Keyword) {
		responseTime := roundMilliseconds(time.Since(start))
		return monitor.StatusUp, &responseTime
	}
	return monitor.StatusDown, nil
}

func handlePingMonitoring(monitoring monitor.Monitoring) (monitor.Status, *float64) {
	port := monitoring.Port
	if port <= 0 {
		port = 80
	}
	address, err := target.TCPAddress(monitoring.Target, port)
	if err != nil {
		return monitor.StatusDown, nil
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	responseTime := roundMilliseconds(time.Since(start))
	if err != nil {
		return monitor.StatusDown, &responseTime
	}
	_ = conn.Close()

	return monitor.StatusUp, &responseTime
}

func handlePortMonitoring(monitoring monitor.Monitoring) (monitor.Status, *float64) {
	if monitoring.Port <= 0 {
		return monitor.StatusDown, nil
	}

	address, err := target.TCPAddress(monitoring.Target, monitoring.Port)
	if err != nil {
		return monitor.StatusDown, nil
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return monitor.StatusDown, nil
	}
	_ = conn.Close()

	responseTime := roundMilliseconds(time.Since(start))
	return monitor.StatusUp, &responseTime
}

func (r *Runner) performHTTPRequest(ctx context.Context, monitoring monitor.Monitoring) (int, string, error) {
	targetURL := strings.TrimSpace(monitoring.Target)
	if targetURL == "" {
		return 0, "", fmt.Errorf("monitoring target is empty")
	}

	method := strings.ToLower(strings.TrimSpace(string(monitoring.HTTPMethod)))
	if method == "" || !slices.Contains([]string{"get", "post", "put", "patch", "delete"}, method) {
		method = string(monitor.HTTPMethodGet)
	}

	headers := normalizeHeaders(monitoring.HTTPHeaders)
	body := normalizeBody(monitoring.HTTPBody)
	if method == "get" || method == "delete" {
		body = nil
	}
	if len(body) > 0 && headers["Content-Type"] == "" && headers["content-type"] == "" {
		headers["Content-Type"] = "application/json"
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // Keep PHP compatibility (withoutVerifying)
			},
		},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= fixedHTTPMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", fixedHTTPMaxRedirects)
			}
			return nil
		},
	}
	if monitoring.Timeout > 0 {
		httpClient.Timeout = time.Duration(monitoring.Timeout) * time.Second
	}

	retryTimes := fixedHTTPRetryTimes
	attempts := retryTimes + 1
	delay := fixedHTTPRetryDelay

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		var requestBody io.Reader
		if len(body) > 0 {
			requestBody = bytes.NewReader(body)
		}

		request, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), targetURL, requestBody)
		if err != nil {
			return 0, "", err
		}

		for key, value := range headers {
			request.Header.Set(key, value)
		}
		if monitoring.AuthUsername != "" && monitoring.AuthPassword != "" {
			request.SetBasicAuth(monitoring.AuthUsername, monitoring.AuthPassword)
		}

		response, err := httpClient.Do(request)
		if err != nil {
			lastErr = err
			if attempt < attempts-1 {
				time.Sleep(delay)
				continue
			}
			return 0, "", lastErr
		}

		payload, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			return 0, "", err
		}

		return response.StatusCode, string(payload), nil
	}

	return 0, "", lastErr
}

func (r *Runner) crawlMonitoringSSL(monitoring monitor.Monitoring) monitor.SSLResultPayload {
	payload := monitor.SSLResultPayload{
		MonitoringID: monitoring.ID,
		IsValid:      false,
	}

	address, serverName, err := target.SSLAddressAndServerName(monitoring.Target)
	if err != nil {
		return payload
	}

	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", address, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, //nolint:gosec // Needed to inspect certificate even when invalid.
	})
	if err != nil {
		return payload
	}
	defer connection.Close()

	peerCertificates := connection.ConnectionState().PeerCertificates
	if len(peerCertificates) == 0 {
		return payload
	}

	certificate := peerCertificates[0]
	now := time.Now()
	if now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
		return payload
	}
	if err := certificate.VerifyHostname(serverName); err != nil {
		return payload
	}

	payload.IsValid = true
	expiresAt := certificate.NotAfter.UTC()
	issuedAt := certificate.NotBefore.UTC()
	payload.ExpiresAt = &expiresAt
	payload.IssuedAt = &issuedAt

	issuer := certificate.Issuer.CommonName
	if issuer == "" {
		issuer = certificate.Issuer.String()
	}
	if issuer != "" {
		payload.Issuer = &issuer
	}

	return payload
}

func normalizeHeaders(rawHeaders any) map[string]string {
	result := make(map[string]string)

	switch value := rawHeaders.(type) {
	case nil:
		return result
	case string:
		if strings.TrimSpace(value) == "" {
			return result
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return result
		}
		for key, raw := range parsed {
			result[key] = fmt.Sprintf("%v", raw)
		}
	case map[string]any:
		for key, raw := range value {
			result[key] = fmt.Sprintf("%v", raw)
		}
	case map[string]string:
		for key, raw := range value {
			result[key] = raw
		}
	}

	return result
}

func normalizeBody(rawBody any) []byte {
	if rawBody == nil {
		return []byte("[]")
	}

	switch value := rawBody.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return []byte("[]")
		}
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return []byte("[]")
		}
		payload, err := json.Marshal(parsed)
		if err != nil {
			return []byte("[]")
		}
		return payload
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return []byte("[]")
		}
		return payload
	}
}

func roundMilliseconds(duration time.Duration) float64 {
	value := float64(duration.Microseconds()) / 1000
	return math.Round(value*100) / 100
}
