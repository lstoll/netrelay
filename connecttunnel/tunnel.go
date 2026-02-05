package connecttunnel

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
)

// Dialer establishes network connections through a tunnel.
// All client implementations satisfy this interface.
type Dialer interface {
	// DialContext connects to the address on the named network using the provided context.
	// The network must be "tcp", "tcp4", or "tcp6".
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// TunnelFunc is called when a tunnel is established on the server side.
// It receives the incoming request and can inspect headers, perform authentication,
// or reject the connection by returning an error.
//
// If TunnelFunc returns an error, the tunnel is rejected and a 403 Forbidden
// response is sent to the client.
type TunnelFunc func(ctx context.Context, req *http.Request) error

// DialFunc is a function that establishes a network connection.
// It has the same signature as net.Dialer.DialContext.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Logger is a minimal logging interface compatible with *log.Logger.
type Logger interface {
	Printf(format string, v ...interface{})
}

// ServerConfig configures server-side tunnel handlers.
type ServerConfig struct {
	// OnTunnel is called when a tunnel is established.
	// If nil, all tunnels are accepted.
	// If it returns an error, the tunnel is rejected with 403 Forbidden.
	OnTunnel TunnelFunc

	// Dial is used to establish connections to upstream targets.
	// If nil, net.Dialer{}.DialContext is used.
	Dial DialFunc

	// ErrorLog specifies an optional logger for errors.
	// If nil, logging goes to os.Stderr via the log package's standard logger.
	ErrorLog Logger
}

// ClientConfig configures client-side tunnel dialers.
type ClientConfig struct {
	// ProxyURL is the URL of the proxy server (e.g., "http://proxy.example.com:8080").
	// Required. Scheme must be "http" or "https".
	ProxyURL string

	// TLSConfig specifies the TLS configuration for HTTPS proxies.
	// Optional. Only used when ProxyURL scheme is "https".
	TLSConfig *tls.Config

	// HeadersForRequest is called if present for a given request to get
	// additional headers to send with the CONNECT request. If nil, no
	// additional headers are sent. Used for authentication etc.
	HeadersForRequest func(req *http.Request) (http.Header, error)

	// DialContext specifies an optional dialer for establishing the proxy connection.
	// If nil, net.Dialer{}.DialContext is used.
	// This can be used to chain proxies or customize the transport layer.
	DialContext DialFunc
}

// getDialFunc returns a DialFunc from the config, or a default dialer.
func (c *ServerConfig) getDialFunc() DialFunc {
	if c.Dial != nil {
		return c.Dial
	}
	d := &net.Dialer{}
	return d.DialContext
}

// getLogger returns the configured logger or a default logger.
func (c *ServerConfig) getLogger() Logger {
	if c.ErrorLog != nil {
		return c.ErrorLog
	}
	return log.Default()
}

// checkTunnel calls the OnTunnel callback if configured.
// Returns nil if the tunnel should be accepted.
func (c *ServerConfig) checkTunnel(ctx context.Context, req *http.Request) error {
	if c.OnTunnel != nil {
		return c.OnTunnel(ctx, req)
	}
	return nil
}
