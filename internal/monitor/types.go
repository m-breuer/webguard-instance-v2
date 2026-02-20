package monitor

import "time"

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
