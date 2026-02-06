package connect

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestH2Multiplexing verifies that multiple concurrent tunnels
// can be established over a single HTTP/2 connection.
func TestH2Multiplexing(t *testing.T) {
	// Create multiple echo servers
	const numEchos = 5
	echoAddrs := make([]string, numEchos)
	for i := 0; i < numEchos; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to create echo server %d: %v", i, err)
		}
		defer func() { _ = listener.Close() }()
		echoAddrs[i] = listener.Addr().String()

		// Echo server
		go func(l net.Listener, idx int) {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer func() { _ = c.Close() }()
					_, _ = io.Copy(c, c)
				}(conn)
			}
		}(listener, i)
	}

	// Create HTTP/2 proxy server
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

	// Create ONE dialer (should reuse connection)
	dialer := NewH2Dialer(&ClientConfig{
		ProxyURL: proxyServer.URL,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})

	// Establish multiple concurrent tunnels
	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, numEchos)

	for i := 0; i < numEchos; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Connect through proxy
			conn, err := dialer.DialContext(ctx, "tcp", echoAddrs[idx])
			if err != nil {
				errors <- err
				return
			}
			defer func() { _ = conn.Close() }()

			// Send unique message
			message := []byte("test-" + string(rune('0'+idx)))
			if _, err := conn.Write(message); err != nil {
				errors <- err
				return
			}

			// Read echo
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, len(message))
			if _, err := io.ReadFull(conn, buf); err != nil {
				errors <- err
				return
			}

			if string(buf) != string(message) {
				t.Errorf("Tunnel %d: expected %q, got %q", idx, message, buf)
			}
		}(i)
	}

	// Wait for all tunnels to complete
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Tunnel error: %v", err)
	}

	t.Logf("Successfully multiplexed %d concurrent tunnels", numEchos)
}
