package connect

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
)

// NewH2Handler creates an HTTP/2 CONNECT handler.
// It handles CONNECT requests using HTTP/2 streams for bidirectional tunneling.
// This handler works with both HTTP/2 over TLS and HTTP/2 cleartext (h2c).
func NewH2Handler(cfg *ServerConfig) http.Handler {
	if cfg == nil {
		cfg = &ServerConfig{}
	}
	return &h2Handler{cfg: cfg}
}

type h2Handler struct {
	cfg *ServerConfig
}

func (h *h2Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Verify protocol is HTTP/2
	if req.ProtoMajor != 2 {
		http.Error(w, "HTTP/2 required", http.StatusHTTPVersionNotSupported)
		return
	}

	// Verify method is CONNECT
	if req.Method != http.MethodConnect {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract target from Host header (HTTP/2 CONNECT uses :authority pseudo-header)
	target := req.Host
	if target == "" {
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

	// Enable full duplex mode for HTTP/2 streams
	rc := http.NewResponseController(w)
	if err := rc.EnableFullDuplex(); err != nil {
		_ = upstream.Close()
		h.cfg.getLogger().Printf("failed to enable full duplex: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Send 200 OK response
	w.WriteHeader(http.StatusOK)

	// Flush headers to establish the tunnel
	if err := rc.Flush(); err != nil {
		_ = upstream.Close()
		h.cfg.getLogger().Printf("failed to flush response: %v", err)
		return
	}

	// Start bidirectional copy between request body and upstream
	// For HTTP/2, we must stay in the handler to keep the response stream open
	h.tunnel(req.Context(), req.Body, w, upstream)
}

// tunnel performs bidirectional copying between the HTTP/2 stream and upstream connection.
func (h *h2Handler) tunnel(ctx context.Context, reqBody io.ReadCloser, w http.ResponseWriter, upstream net.Conn) {
	defer func() { _ = reqBody.Close() }()
	defer func() { _ = upstream.Close() }()

	// Get flusher for immediate data transmission
	// HTTP/2 requires explicit flushing to send data frames immediately
	flusher, _ := w.(http.Flusher)

	errCh := make(chan error, 2)

	// Copy from request body (client) to upstream
	go func() {
		_, err := io.Copy(upstream, reqBody)
		// Close write side of upstream when client sends EOF
		if conn, ok := upstream.(*net.TCPConn); ok {
			_ = conn.CloseWrite()
		}
		errCh <- err
	}()

	// Copy from upstream to response body (client), with explicit flushing
	go func() {
		buf := make([]byte, 32*1024)
		for {
			nr, er := upstream.Read(buf)
			if nr > 0 {
				nw, ew := w.Write(buf[0:nr])
				// Flush after each write to ensure data is sent immediately
				if flusher != nil {
					flusher.Flush()
				}
				if nw < 0 || nr < nw {
					nw = 0
					if ew == nil {
						ew = fmt.Errorf("invalid write result")
					}
				}
				if ew != nil {
					errCh <- ew
					return
				}
				if nr != nw {
					errCh <- io.ErrShortWrite
					return
				}
			}
			if er != nil {
				errCh <- er
				return
			}
		}
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
