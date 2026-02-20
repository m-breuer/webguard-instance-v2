package monitor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Type string

const (
	TypeHTTP    Type = "http"
	TypePing    Type = "ping"
	TypeKeyword Type = "keyword"
	TypePort    Type = "port"
)

type Status string

const (
	StatusUp      Status = "up"
	StatusDown    Status = "down"
	StatusUnknown Status = "unknown"
)

type HTTPMethod string

const (
	HTTPMethodGet    HTTPMethod = "get"
	HTTPMethodPost   HTTPMethod = "post"
	HTTPMethodPut    HTTPMethod = "put"
	HTTPMethodPatch  HTTPMethod = "patch"
	HTTPMethodDelete HTTPMethod = "delete"
)

type Monitoring struct {
	ID   int64 `json:"id"`
	Type Type  `json:"type"`

	Target string `json:"target"`

	Timeout int `json:"timeout"`

	HTTPMethod  HTTPMethod `json:"http_method"`
	HTTPBody    any        `json:"http_body"`
	HTTPHeaders any        `json:"http_headers"`

	AuthUsername string `json:"auth_username"`
	AuthPassword string `json:"auth_password"`

	Keyword string `json:"keyword"`
	Port    int    `json:"port"`

	MaintenanceActive bool `json:"maintenance_active"`
}

func (m *Monitoring) UnmarshalJSON(data []byte) error {
	type rawMonitoring struct {
		ID any `json:"id"`

		Type Type `json:"type"`

		Target string `json:"target"`

		Timeout any `json:"timeout"`

		HTTPMethod  HTTPMethod `json:"http_method"`
		HTTPBody    any        `json:"http_body"`
		HTTPHeaders any        `json:"http_headers"`

		AuthUsername string `json:"auth_username"`
		AuthPassword string `json:"auth_password"`

		Keyword string `json:"keyword"`
		Port    any    `json:"port"`

		MaintenanceActive any `json:"maintenance_active"`
	}

	var raw rawMonitoring
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	id, err := parseInt64Flexible(raw.ID, "id")
	if err != nil {
		return err
	}

	timeout, err := parseIntFlexible(raw.Timeout, "timeout")
	if err != nil {
		return err
	}
	port, err := parseIntFlexible(raw.Port, "port")
	if err != nil {
		return err
	}
	maintenanceActive, err := parseBoolFlexible(raw.MaintenanceActive, "maintenance_active")
	if err != nil {
		return err
	}

	*m = Monitoring{
		ID:   id,
		Type: raw.Type,

		Target: raw.Target,

		Timeout: timeout,

		HTTPMethod:  raw.HTTPMethod,
		HTTPBody:    raw.HTTPBody,
		HTTPHeaders: raw.HTTPHeaders,

		AuthUsername: raw.AuthUsername,
		AuthPassword: raw.AuthPassword,

		Keyword: raw.Keyword,
		Port:    port,

		MaintenanceActive: maintenanceActive,
	}

	return nil
}

type MonitoringResponsePayload struct {
	MonitoringID int64    `json:"monitoring_id"`
	Status       Status   `json:"status"`
	ResponseTime *float64 `json:"response_time"`
}

type SSLResultPayload struct {
	MonitoringID int64      `json:"monitoring_id"`
	IsValid      bool       `json:"is_valid"`
	ExpiresAt    *time.Time `json:"expires_at"`
	Issuer       *string    `json:"issuer"`
	IssuedAt     *time.Time `json:"issued_at"`
}

func parseInt64Flexible(value any, field string) (int64, error) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", field, err)
		}
		return parsed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", field, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid %s type: %T", field, value)
	}
}

func parseIntFlexible(value any, field string) (int, error) {
	parsed, err := parseInt64Flexible(value, field)
	if err != nil {
		return 0, err
	}
	return int(parsed), nil
}

func parseBoolFlexible(value any, field string) (bool, error) {
	switch typed := value.(type) {
	case nil:
		return false, nil
	case bool:
		return typed, nil
	case float64:
		return typed != 0, nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return false, fmt.Errorf("invalid %s: %w", field, err)
		}
		return parsed != 0, nil
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		if trimmed == "" {
			return false, nil
		}
		switch trimmed {
		case "1", "true", "yes", "on":
			return true, nil
		case "0", "false", "no", "off":
			return false, nil
		default:
			return false, fmt.Errorf("invalid %s: %q", field, typed)
		}
	default:
		return false, fmt.Errorf("invalid %s type: %T", field, value)
	}
}
