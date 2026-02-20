package target

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func TCPAddress(rawTarget string, port int) (string, error) {
	host, _, err := extractHostPort(rawTarget)
	if err != nil {
		return "", err
	}
	if port <= 0 {
		return "", fmt.Errorf("invalid port %d", port)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func SSLAddressAndServerName(rawTarget string) (string, string, error) {
	host, parsedPort, err := extractHostPort(rawTarget)
	if err != nil {
		return "", "", err
	}
	if parsedPort == "" {
		parsedPort = "443"
	}
	return net.JoinHostPort(host, parsedPort), host, nil
}

func extractHostPort(rawTarget string) (string, string, error) {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return "", "", fmt.Errorf("target is empty")
	}

	hostPort := target
	if strings.Contains(target, "://") {
		parsedURL, err := url.Parse(target)
		if err != nil {
			return "", "", err
		}
		hostPort = parsedURL.Host
	} else {
		parsedURL, err := url.Parse("//" + target)
		if err == nil && parsedURL.Host != "" {
			hostPort = parsedURL.Host
		}
	}

	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return "", "", fmt.Errorf("target host is empty")
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port, nil
	}

	return hostPort, "", nil
}
