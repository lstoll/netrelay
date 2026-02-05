package connect

import (
	"errors"
	"fmt"
)

// Common errors returned by the package.
var (
	// ErrInvalidMethod is returned when a request uses a method other than CONNECT.
	ErrInvalidMethod = errors.New("connecttunnel: invalid method, expected CONNECT")

	// ErrInvalidTarget is returned when the target address is missing or malformed.
	ErrInvalidTarget = errors.New("connecttunnel: invalid target address")

	// ErrTunnelRejected is returned when the OnTunnel callback rejects a connection.
	ErrTunnelRejected = errors.New("connecttunnel: tunnel rejected by callback")

	// ErrUpstreamDial is returned when dialing the upstream target fails.
	ErrUpstreamDial = errors.New("connecttunnel: failed to dial upstream")

	// ErrHijackFailed is returned when HTTP/1.1 connection hijacking fails.
	ErrHijackFailed = errors.New("connecttunnel: failed to hijack connection")

	// ErrProxyConnect is returned when the proxy connection fails.
	ErrProxyConnect = errors.New("connecttunnel: proxy connection failed")
)

// ProxyError represents an error response from a proxy server.
type ProxyError struct {
	// StatusCode is the HTTP status code returned by the proxy.
	StatusCode int

	// Status is the HTTP status line (e.g., "403 Forbidden").
	Status string

	// Message is additional error information, if available.
	Message string
}

// Error implements the error interface.
func (e *ProxyError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("connecttunnel: proxy returned %s: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("connecttunnel: proxy returned %s", e.Status)
}

// Is implements error matching for ProxyError.
func (e *ProxyError) Is(target error) bool {
	_, ok := target.(*ProxyError)
	return ok
}
