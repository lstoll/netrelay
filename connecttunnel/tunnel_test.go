package connecttunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// TestH1ServerClient tests HTTP/1.1 CONNECT tunnel.
func TestH1ServerClient(t *testing.T) {
	// Create echo server
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	}))
	defer echoServer.Close()

	// Create proxy server
	proxyHandler := NewH1Handler(&ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			t.Logf("Tunnel to: %s", req.RequestURI)
			return nil
		},
	})
	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	// Create client dialer
	dialer := NewH1Dialer(&ClientConfig{
		ProxyURL: proxyServer.URL,
	})

	// Extract echo server host:port
	echoAddr := strings.TrimPrefix(echoServer.URL, "http://")

	// Connect through proxy
	ctx := context.Background()
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatalf("Failed to dial through proxy: %v", err)
	}
	defer conn.Close()

	// Send HTTP request through tunnel
	message := "Hello, World!"
	req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
		echoAddr, len(message), message)

	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, message) {
		t.Errorf("Response does not contain echo: %s", response)
	}

	t.Logf("Test passed: %s", response)
}

// TestH2ServerClient tests HTTP/2 CONNECT tunnel over TLS.
func TestH2ServerClient(t *testing.T) {
	// Create echo server
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	}))
	defer echoServer.Close()

	// Create proxy server with HTTP/2
	proxyHandler := NewH2Handler(&ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			t.Logf("Tunnel to: %s", req.Host)
			return nil
		},
	})
	proxyServer := httptest.NewUnstartedServer(proxyHandler)
	proxyServer.EnableHTTP2 = true
	proxyServer.StartTLS()
	defer proxyServer.Close()

	// Create client dialer
	dialer := NewH2Dialer(&ClientConfig{
		ProxyURL: proxyServer.URL,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})

	// Extract echo server host:port
	echoAddr := strings.TrimPrefix(echoServer.URL, "http://")

	// Connect through proxy
	ctx := context.Background()
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatalf("Failed to dial through proxy: %v", err)
	}
	defer conn.Close()

	// Send HTTP request through tunnel
	message := "Hello, HTTP/2!"
	req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
		echoAddr, len(message), message)

	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, message) {
		t.Errorf("Response does not contain echo: %s", response)
	}

	t.Logf("Test passed: %s", response)
}

// TestH2CServerClient tests HTTP/2 cleartext (h2c) CONNECT tunnel.
func TestH2CServerClient(t *testing.T) {
	// Create echo server
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	}))
	defer echoServer.Close()

	// Create h2c proxy server
	proxyHandler := NewHandler(&ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			t.Logf("Tunnel to: %s", req.Host)
			return nil
		},
	})
	h2s := &http2.Server{}
	h1s := &http.Server{
		Handler: h2c.NewHandler(proxyHandler, h2s),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	proxyAddr := listener.Addr().String()
	go h1s.Serve(listener)
	defer h1s.Close()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create h2c client dialer
	dialer := NewH2CDialer(&ClientConfig{
		ProxyURL: "http://" + proxyAddr,
	})

	// Extract echo server host:port
	echoAddr := strings.TrimPrefix(echoServer.URL, "http://")

	// Connect through proxy
	ctx := context.Background()
	conn, err := dialer.DialContext(ctx, "tcp", echoAddr)
	if err != nil {
		t.Fatalf("Failed to dial through proxy: %v", err)
	}
	defer conn.Close()

	// Send HTTP request through tunnel
	message := "Hello, h2c!"
	req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
		echoAddr, len(message), message)

	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, message) {
		t.Errorf("Response does not contain echo: %s", response)
	}

	t.Logf("Test passed: %s", response)
}

// TestUnifiedHandler tests the unified handler with both HTTP/1.1 and HTTP/2.
func TestUnifiedHandler(t *testing.T) {
	// Create echo server
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	}))
	defer echoServer.Close()

	// Create unified proxy server
	proxyHandler := NewHandler(&ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			t.Logf("Unified tunnel to: %s (proto: %s)", req.Host, req.Proto)
			return nil
		},
	})

	// Test with HTTP/1.1
	t.Run("HTTP1", func(t *testing.T) {
		proxyServer := httptest.NewServer(proxyHandler)
		defer proxyServer.Close()

		dialer := NewH1Dialer(&ClientConfig{
			ProxyURL: proxyServer.URL,
		})

		echoAddr := strings.TrimPrefix(echoServer.URL, "http://")
		conn, err := dialer.DialContext(context.Background(), "tcp", echoAddr)
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
		}
		defer conn.Close()

		message := "Test HTTP/1.1"
		req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
			echoAddr, len(message), message)
		conn.Write([]byte(req))

		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		if !strings.Contains(string(buf[:n]), message) {
			t.Errorf("HTTP/1.1 test failed")
		}
	})

	// Test with HTTP/2
	t.Run("HTTP2", func(t *testing.T) {
		proxyServer := httptest.NewUnstartedServer(proxyHandler)
		proxyServer.EnableHTTP2 = true
		proxyServer.StartTLS()
		defer proxyServer.Close()

		dialer := NewH2Dialer(&ClientConfig{
			ProxyURL: proxyServer.URL,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		})

		echoAddr := strings.TrimPrefix(echoServer.URL, "http://")
		conn, err := dialer.DialContext(context.Background(), "tcp", echoAddr)
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
		}
		defer conn.Close()

		message := "Test HTTP/2"
		req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
			echoAddr, len(message), message)
		conn.Write([]byte(req))

		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		if !strings.Contains(string(buf[:n]), message) {
			t.Errorf("HTTP/2 test failed")
		}
	})
}

// TestTunnelRejection tests that OnTunnel callback can reject connections.
func TestTunnelRejection(t *testing.T) {
	// Create proxy that rejects all tunnels
	proxyHandler := NewH1Handler(&ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			return fmt.Errorf("access denied")
		},
	})
	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	// Create client dialer
	dialer := NewH1Dialer(&ClientConfig{
		ProxyURL: proxyServer.URL,
	})

	// Try to connect
	ctx := context.Background()
	_, err := dialer.DialContext(ctx, "tcp", "example.com:80")
	if err == nil {
		t.Fatal("Expected connection to be rejected")
	}

	// Check for ProxyError
	var proxyErr *ProxyError
	if !strings.Contains(err.Error(), "proxy returned") {
		t.Errorf("Expected proxy error, got: %v", err)
	}

	t.Logf("Connection correctly rejected: %v", proxyErr)
}
