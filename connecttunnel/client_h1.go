package connecttunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// h1Dialer implements Dialer for HTTP/1.1 CONNECT proxies.
type h1Dialer struct {
	proxyAddr string
	proxyHost string
	useTLS    bool
	tlsConfig *tls.Config
	header    http.Header
	dial      DialFunc
}

// NewH1Dialer creates a Dialer that connects through an HTTP/1.1 proxy.
// The proxy URL must use "http" or "https" scheme.
func NewH1Dialer(cfg *ClientConfig) Dialer {
	if cfg == nil {
		cfg = &ClientConfig{}
	}

	proxyURL, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		panic(fmt.Sprintf("connecttunnel: invalid proxy URL: %v", err))
	}

	useTLS := proxyURL.Scheme == "https"
	proxyHost := proxyURL.Host
	if proxyURL.Port() == "" {
		if useTLS {
			proxyHost = net.JoinHostPort(proxyHost, "443")
		} else {
			proxyHost = net.JoinHostPort(proxyHost, "80")
		}
	}

	dial := cfg.DialContext
	if dial == nil {
		d := &net.Dialer{}
		dial = d.DialContext
	}

	return &h1Dialer{
		proxyAddr: proxyHost,
		proxyHost: proxyURL.Hostname(),
		useTLS:    useTLS,
		tlsConfig: cfg.TLSConfig,
		header:    cfg.Header,
		dial:      dial,
	}
}

// DialContext establishes a connection through the HTTP/1.1 proxy.
func (d *h1Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Only support TCP networks
	if !strings.HasPrefix(network, "tcp") {
		return nil, fmt.Errorf("connecttunnel: unsupported network: %s", network)
	}

	// Connect to proxy
	conn, err := d.dial(ctx, network, d.proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProxyConnect, err)
	}

	// Upgrade to TLS if needed
	if d.useTLS {
		tlsConfig := d.tlsConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{ServerName: d.proxyHost}
		} else if tlsConfig.ServerName == "" {
			tlsConfig = tlsConfig.Clone()
			tlsConfig.ServerName = d.proxyHost
		}
		conn = tls.Client(conn, tlsConfig)
	}

	// Send CONNECT request
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: address},
		Host:   address,
		Header: make(http.Header),
		Proto:  "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}

	// Copy custom headers
	for k, v := range d.header {
		req.Header[k] = v
	}

	// Write request
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: failed to write request: %v", ErrProxyConnect, err)
	}

	// Read response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: failed to read response: %v", ErrProxyConnect, err)
	}
	resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, &ProxyError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	// Return wrapped connection that includes buffered reader
	return &bufferedConn{
		Conn:   conn,
		reader: br,
	}, nil
}

// bufferedConn wraps a net.Conn with a bufio.Reader to handle any buffered data.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

// Read reads from the buffered reader, which wraps the underlying connection.
func (c *bufferedConn) Read(b []byte) (int, error) {
	// Always read from the buffered reader to maintain proper state
	return c.reader.Read(b)
}
