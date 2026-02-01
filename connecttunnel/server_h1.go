package connecttunnel

import (
	"context"
	"io"
	"net"
	"net/http"
)

// NewH1Handler creates an HTTP/1.1 CONNECT handler.
// It handles CONNECT requests by hijacking the connection and establishing
// a bidirectional tunnel to the requested target.
func NewH1Handler(cfg *ServerConfig) http.Handler {
	if cfg == nil {
		cfg = &ServerConfig{}
	}
	return &h1Handler{cfg: cfg}
}

type h1Handler struct {
	cfg *ServerConfig
}

func (h *h1Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Verify method is CONNECT
	if req.Method != http.MethodConnect {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract target from RequestURI (e.g., "example.com:443")
	target := req.RequestURI
	if target == "" || target == "/" {
		http.Error(w, "Bad request: missing target", http.StatusBadRequest)
		return
	}

	// Call OnTunnel callback if configured
	if err := h.cfg.checkTunnel(req.Context(), req); err != nil {
		h.cfg.getLogger().Printf("tunnel rejected: %v", err)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Dial upstream target
	dial := h.cfg.getDialFunc()
	upstream, err := dial(req.Context(), "tcp", target)
	if err != nil {
		h.cfg.getLogger().Printf("failed to dial %s: %v", target, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	client, bufrw, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		h.cfg.getLogger().Printf("hijack failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Send success response
	_, err = bufrw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if err != nil {
		client.Close()
		upstream.Close()
		h.cfg.getLogger().Printf("failed to write response: %v", err)
		return
	}
	if err = bufrw.Flush(); err != nil {
		client.Close()
		upstream.Close()
		h.cfg.getLogger().Printf("failed to flush response: %v", err)
		return
	}

	// Start bidirectional copy in a goroutine
	// Note: We use context.Background() instead of req.Context() because hijacked
	// connections are independent of the HTTP request lifecycle
	go h.tunnel(context.Background(), client, upstream)
}

// tunnel performs bidirectional copying between client and upstream connections.
func (h *h1Handler) tunnel(ctx context.Context, client, upstream net.Conn) {
	defer client.Close()
	defer upstream.Close()

	errCh := make(chan error, 2)

	// Copy from client to upstream
	go func() {
		_, err := io.Copy(upstream, client)
		// Close write side of upstream when client sends EOF
		if conn, ok := upstream.(*net.TCPConn); ok {
			conn.CloseWrite()
		}
		errCh <- err
	}()

	// Copy from upstream to client
	go func() {
		_, err := io.Copy(client, upstream)
		// Close write side of client when upstream sends EOF
		if conn, ok := client.(*net.TCPConn); ok {
			conn.CloseWrite()
		}
		errCh <- err
	}()

	// Wait for both copies to complete or context cancellation
	select {
	case <-ctx.Done():
		// Context cancelled, close connections
		return
	case err := <-errCh:
		// One direction finished (possibly with error)
		if err != nil && err != io.EOF {
			h.cfg.getLogger().Printf("tunnel error: %v", err)
		}
		// Wait for the other direction to finish
		err2 := <-errCh
		if err2 != nil && err2 != io.EOF {
			h.cfg.getLogger().Printf("tunnel error: %v", err2)
		}
		return
	}
}
