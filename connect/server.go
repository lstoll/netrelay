package connect

import (
	"net/http"
)

// NewHandler creates a unified handler that automatically detects and handles
// both HTTP/1.1 and HTTP/2 CONNECT requests.
//
// This is the recommended handler for most use cases. It inspects the request
// protocol and delegates to the appropriate protocol-specific handler.
func NewHandler(cfg *ServerConfig) http.Handler {
	if cfg == nil {
		cfg = &ServerConfig{}
	}
	return &unifiedHandler{
		cfg: cfg,
		h1:  NewH1Handler(cfg).(*h1Handler),
		h2:  NewH2Handler(cfg).(*h2Handler),
	}
}

type unifiedHandler struct {
	cfg *ServerConfig
	h1  *h1Handler
	h2  *h2Handler
}

func (h *unifiedHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Check method first
	if req.Method != http.MethodConnect {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Detect protocol and delegate
	if req.ProtoMajor == 2 {
		h.h2.ServeHTTP(w, req)
	} else {
		h.h1.ServeHTTP(w, req)
	}
}
