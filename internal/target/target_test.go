package target

import "testing"

func TestTCPAddress(t *testing.T) {
	t.Parallel()

	address, err := TCPAddress("https://example.com:8443/path", 443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "example.com:443" {
		t.Fatalf("expected example.com:443, got %q", address)
	}
}

func TestTCPAddressInvalidPort(t *testing.T) {
	t.Parallel()

	_, err := TCPAddress("example.com", 0)
	if err == nil {
		t.Fatalf("expected error for invalid port")
	}
}

func TestSSLAddressAndServerNameDefaultsTo443(t *testing.T) {
	t.Parallel()

	address, serverName, err := SSLAddressAndServerName("https://example.com/health")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "example.com:443" {
		t.Fatalf("expected example.com:443, got %q", address)
	}
	if serverName != "example.com" {
		t.Fatalf("expected server name example.com, got %q", serverName)
	}
}

func TestSSLAddressAndServerNameKeepsExplicitPort(t *testing.T) {
	t.Parallel()

	address, serverName, err := SSLAddressAndServerName("example.com:9443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "example.com:9443" {
		t.Fatalf("expected example.com:9443, got %q", address)
	}
	if serverName != "example.com" {
		t.Fatalf("expected server name example.com, got %q", serverName)
	}
}

func TestSSLAddressAndServerNameEmptyTarget(t *testing.T) {
	t.Parallel()

	_, _, err := SSLAddressAndServerName("   ")
	if err == nil {
		t.Fatalf("expected error for empty target")
	}
}
