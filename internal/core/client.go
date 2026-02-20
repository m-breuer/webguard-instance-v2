package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/m-breuer/webguard-instance-v2/internal/monitor"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("core API returned status %d", e.StatusCode)
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) SetHTTPClient(httpClient *http.Client) {
	if httpClient == nil {
		return
	}
	c.httpClient = httpClient
}

func (c *Client) GetMonitorings(ctx context.Context, location string, types []monitor.Type) ([]monitor.Monitoring, error) {
	query := make(url.Values)
	if location != "" {
		query.Set("location", location)
	}
	if len(types) > 0 {
		values := make([]string, 0, len(types))
		for _, value := range types {
			values = append(values, string(value))
		}
		query.Set("types", strings.Join(values, ","))
	}

	request, err := c.newRequest(ctx, http.MethodGet, "/api/v1/internal/monitorings", query, nil)
	if err != nil {
		return nil, err
	}

	var monitorings []monitor.Monitoring
	if err := c.doJSON(request, &monitorings); err != nil {
		return nil, err
	}
	return monitorings, nil
}

func (c *Client) PostMonitoringResponse(ctx context.Context, payload monitor.MonitoringResponsePayload) error {
	request, err := c.newRequest(ctx, http.MethodPost, "/api/v1/internal/monitoring-responses", nil, payload)
	if err != nil {
		return err
	}

	return c.doJSON(request, nil)
}

func (c *Client) PostSSLResult(ctx context.Context, payload monitor.SSLResultPayload) error {
	request, err := c.newRequest(ctx, http.MethodPost, "/api/v1/internal/ssl-results", nil, payload)
	if err != nil {
		return err
	}

	return c.doJSON(request, nil)
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("WEBGUARD_CORE_API_URL is empty")
	}

	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, err
	}
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("X-API-KEY", c.apiKey)
	}

	return request, nil
}

func (c *Client) doJSON(request *http.Request, out any) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if response.StatusCode >= http.StatusBadRequest {
		return &HTTPStatusError{
			StatusCode: response.StatusCode,
			Body:       string(raw),
		}
	}

	if out == nil || len(raw) == 0 {
		return nil
	}

	return json.Unmarshal(raw, out)
}
