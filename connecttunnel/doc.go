// Package connecttunnel provides TCP tunneling over HTTP CONNECT protocol.
//
// This package supports HTTP/1.1 CONNECT (RFC 7231), HTTP/2 CONNECT (RFC 9113),
// and HTTP/2 cleartext (h2c) connections. It provides both server handlers and
// client dialers for creating tunneled connections.
//
// # Server Usage
//
// Create a handler that accepts CONNECT requests and establishes tunnels:
//
//	cfg := &connecttunnel.ServerConfig{
//	    OnTunnel: func(ctx context.Context, req *http.Request) error {
//	        // Optional: authenticate, log, or reject connections
//	        return nil
//	    },
//	}
//	handler := connecttunnel.NewHandler(cfg)
//	http.ListenAndServe(":8080", handler)
//
// The unified handler automatically detects HTTP/1.1 and HTTP/2 protocols.
// For protocol-specific handlers, use NewH1Handler or NewH2Handler.
//
// # Client Usage
//
// Create a dialer that connects through a proxy:
//
//	cfg := &connecttunnel.ClientConfig{
//	    ProxyURL: "http://proxy.example.com:8080",
//	}
//	dialer := connecttunnel.NewH1Dialer(cfg)
//	conn, err := dialer.DialContext(ctx, "tcp", "example.com:443")
//
// For HTTP/2 proxies, use NewH2Dialer. For h2c proxies, use NewH2CDialer.
//
// # HTTP/2 Cleartext (h2c) Support
//
// Server setup with h2c:
//
//	import "golang.org/x/net/http2"
//	import "golang.org/x/net/http2/h2c"
//
//	handler := connecttunnel.NewHandler(cfg)
//	h2s := &http2.Server{}
//	h1s := &http.Server{
//	    Addr:    ":8080",
//	    Handler: h2c.NewHandler(handler, h2s),
//	}
//	h1s.ListenAndServe()
//
// Client setup with h2c:
//
//	cfg := &connecttunnel.ClientConfig{
//	    ProxyURL: "http://proxy.example.com:8080",
//	}
//	dialer := connecttunnel.NewH2CDialer(cfg)
//
// # Composability
//
// Dialers can be chained to stack multiple proxies:
//
//	dialer1 := connecttunnel.NewH1Dialer(&connecttunnel.ClientConfig{
//	    ProxyURL: "http://proxy1:8080",
//	})
//	dialer2 := connecttunnel.NewH2Dialer(&connecttunnel.ClientConfig{
//	    ProxyURL: "https://proxy2:8443",
//	    DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
//	        return dialer1.DialContext(ctx, network, addr)
//	    },
//	})
//
// Handlers can be mounted at different paths:
//
//	mux := http.NewServeMux()
//	mux.Handle("/tunnel-h1", connecttunnel.NewH1Handler(cfg))
//	mux.Handle("/tunnel-h2", connecttunnel.NewH2Handler(cfg))
package connecttunnel
