package connect

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http2"
)

// h2Dialer implements Dialer for HTTP/2 CONNECT proxies.
type h2Dialer struct {
	proxyURL   *url.URL
	transport  *http2.Transport
	headerFunc func(req *http.Request) (http.Header, error)
}

// NewH2Dialer creates a Dialer that connects through an HTTP/2 proxy.
// The proxy URL must use "https" scheme (HTTP/2 over TLS).
// For HTTP/2 cleartext (h2c), use NewH2CDialer instead.
func NewH2Dialer(cfg *ClientConfig) Dialer {
	if cfg == nil {
		cfg = &ClientConfig{}
	}

	proxyURL, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		panic(fmt.Sprintf("connecttunnel: invalid proxy URL: %v", err))
	}

	if proxyURL.Scheme != "https" {
		panic("connecttunnel: NewH2Dialer requires https URL, use NewH2CDialer for http")
	}

	transport := &http2.Transport{
		TLSClientConfig: cfg.TLSConfig,
	}

	if cfg.DialContext != nil {
		transport.DialTLSContext = func(ctx context.Context, network, addr string, tlsCfg *tls.Config) (net.Conn, error) {
			conn, err := cfg.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return tls.Client(conn, tlsCfg), nil
		}
	}

	return &h2Dialer{
		proxyURL:   proxyURL,
		transport:  transport,
		headerFunc: cfg.HeadersForRequest,
	}
}

// NewH2CDialer creates a Dialer that connects through an HTTP/2 cleartext (h2c) proxy.
// The proxy URL must use "http" scheme.
func NewH2CDialer(cfg *ClientConfig) Dialer {
	if cfg == nil {
		cfg = &ClientConfig{}
	}

	proxyURL, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		panic(fmt.Sprintf("connecttunnel: invalid proxy URL: %v", err))
	}

	if proxyURL.Scheme != "http" {
		panic("connecttunnel: NewH2CDialer requires http URL, use NewH2Dialer for https")
	}

	dial := cfg.DialContext
	if dial == nil {
		d := &net.Dialer{}
		dial = d.DialContext
	}

	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			// Return cleartext connection for h2c
			return dial(ctx, network, addr)
		},
	}

	return &h2Dialer{
		proxyURL:   proxyURL,
		transport:  transport,
		headerFunc: cfg.HeadersForRequest,
	}
}

// DialContext establishes a connection through the HTTP/2 proxy.
func (d *h2Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Only support TCP networks
	if !strings.HasPrefix(network, "tcp") {
		return nil, fmt.Errorf("connecttunnel: unsupported network: %s", network)
	}

	// Create a pipe for bidirectional communication
	// pr/pw: client writes to pw, server reads from pr (client -> server)
	pr, pw := io.Pipe()

	// Create CONNECT request with the pipe reader as body
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    d.proxyURL,
		Host:   address,
		Header: make(http.Header),
		Body:   pr,
		// ContentLength must be -1 for CONNECT to signal streaming body
		ContentLength: -1,
	}

	// Copy custom headers
	if d.headerFunc != nil {
		addlHeaders, err := d.headerFunc(req)
		if err != nil {
			pr.Close()
			pw.Close()
			return nil, fmt.Errorf("%w: failed to get additional headers: %v", ErrProxyConnect, err)
		}
		maps.Copy(req.Header, addlHeaders)
	}

	// Set context
	req = req.WithContext(ctx)

	// Send request - this returns after response headers are received
	resp, err := d.transport.RoundTrip(req)
	if err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("%w: %v", ErrProxyConnect, err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		pr.Close()
		pw.Close()
		return nil, &ProxyError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}

	// Parse remote address
	// remoteAddr, err := net.ResolveTCPAddr("tcp", address)
	// if err != nil {
	// 	resp.Body.Close()
	// 	pr.Close()
	// 	pw.Close()
	// 	return nil, fmt.Errorf("connecttunnel: invalid address: %v", err)
	// }

	// Create a bidirectional stream:
	// - Write to pw (goes to server via request body)
	// - Read from resp.Body (comes from server via response body)
	conn := &h2Conn{
		reader: resp.Body,
		writer: pw,
		pr:     pr,
		pw:     pw,
	}

	// Wrap as net.Conn
	return newStreamConnRW(conn, &remoteAddr{addr: address}), nil
}

// h2Conn provides bidirectional I/O for HTTP/2 CONNECT.
type h2Conn struct {
	reader io.ReadCloser  // Response body (server -> client)
	writer io.WriteCloser // Pipe writer (client -> server)
	pr     *io.PipeReader // Keep reference for closing
	pw     *io.PipeWriter // Keep reference for closing
}

func (c *h2Conn) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

func (c *h2Conn) Write(p []byte) (n int, err error) {
	return c.writer.Write(p)
}

func (c *h2Conn) Close() error {
	// Close both directions
	err1 := c.reader.Close()
	err2 := c.writer.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
