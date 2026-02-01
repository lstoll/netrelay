# funnelproxy

A comprehensive Go library for TCP tunneling over HTTP CONNECT protocol, supporting HTTP/1.1, HTTP/2 with TLS, and HTTP/2 cleartext (h2c).

## Features

- **HTTP/1.1 CONNECT**: Standard HTTP/1.1 proxy support (RFC 7231)
- **HTTP/2 CONNECT**: HTTP/2 tunneling over TLS (RFC 9113)
- **HTTP/2 Cleartext (h2c)**: HTTP/2 without TLS
- **Unified Handler**: Automatically detects and handles both HTTP/1.1 and HTTP/2
- **Composable**: Chain multiple proxies or customize connection establishment
- **Authentication**: Easy integration with proxy authentication
- **Full Bidirectional I/O**: Complete support for streaming data in both directions

## Installation

```bash
go get lds.li/funnelproxy
```

### Ready-to-Run Server

For a production-ready proxy server with Tailscale Funnel support:

```bash
go install lds.li/funnelproxy/cmd/ts-server@latest
ts-server -hostname my-proxy
```

This creates a publicly accessible CONNECT proxy at `https://my-proxy.ts.net:443` with automatic TLS via Tailscale Funnel. See [cmd/ts-server](cmd/ts-server/README.md) for full documentation.

## Quick Start

### Server (Proxy)

```go
package main

import (
    "context"
    "log"
    "net/http"

    "lds.li/funnelproxy/connecttunnel"
)

func main() {
    handler := connecttunnel.NewHandler(&connecttunnel.ServerConfig{
        OnTunnel: func(ctx context.Context, req *http.Request) error {
            log.Printf("Tunnel: %s -> %s", req.RemoteAddr, req.Host)
            return nil // Return error to reject
        },
    })

    log.Fatal(http.ListenAndServe(":8080", handler))
}
```

### Client

```go
package main

import (
    "context"
    "fmt"
    "io"
    "log"

    "lds.li/funnelproxy/connecttunnel"
)

func main() {
    dialer := connecttunnel.NewH1Dialer(&connecttunnel.ClientConfig{
        ProxyURL: "http://localhost:8080",
    })

    conn, err := dialer.DialContext(context.Background(), "tcp", "example.com:80")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Use conn as a normal net.Conn
    conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
    io.Copy(os.Stdout, conn)
}
```

## HTTP/2 Cleartext (h2c)

### Server

```go
import (
    "golang.org/x/net/http2"
    "golang.org/x/net/http2/h2c"
)

handler := connecttunnel.NewHandler(cfg)
h2s := &http2.Server{}
server := &http.Server{
    Addr:    ":8080",
    Handler: h2c.NewHandler(handler, h2s),
}
server.ListenAndServe()
```

### Client

```go
dialer := connecttunnel.NewH2CDialer(&connecttunnel.ClientConfig{
    ProxyURL: "http://localhost:8080",
})
```

## Authentication

### Server

```go
handler := connecttunnel.NewHandler(&connecttunnel.ServerConfig{
    OnTunnel: func(ctx context.Context, req *http.Request) error {
        auth := req.Header.Get("Proxy-Authorization")
        if !isValid(auth) {
            return connecttunnel.ErrTunnelRejected
        }
        return nil
    },
})
```

### Client

```go
dialer := connecttunnel.NewH1Dialer(&connecttunnel.ClientConfig{
    ProxyURL: "http://localhost:8080",
    Header: http.Header{
        "Proxy-Authorization": []string{"Basic <base64>"},
    },
})
```

## Chaining Proxies

```go
proxy1 := connecttunnel.NewH1Dialer(&connecttunnel.ClientConfig{
    ProxyURL: "http://proxy1:8080",
})

proxy2 := connecttunnel.NewH2Dialer(&connecttunnel.ClientConfig{
    ProxyURL: "https://proxy2:8443",
    DialContext: proxy1.DialContext,
})

// Connects through proxy1, then proxy2, then to target
conn, err := proxy2.DialContext(ctx, "tcp", "target:443")
```

## Examples

See the [examples](examples/) directory for complete working examples:

- [basic-proxy](examples/basic-proxy/main.go) - Simple HTTP/1.1 and HTTP/2 proxy
- [basic-client](examples/basic-client/main.go) - Basic client usage
- [h2c-proxy](examples/h2c-proxy/main.go) - HTTP/2 cleartext server
- [h2c-client](examples/h2c-client/main.go) - HTTP/2 cleartext client
- [auth-proxy](examples/auth-proxy/main.go) - Proxy with authentication
- [auth-client](examples/auth-client/main.go) - Client with authentication
- [chained-proxies](examples/chained-proxies/main.go) - Chaining multiple proxies

## Package Structure

```
connecttunnel/
├── doc.go              # Package documentation
├── tunnel.go           # Core types and interfaces
├── errors.go           # Error types
├── server.go           # Unified handler (auto-detects protocol)
├── server_h1.go        # HTTP/1.1 server implementation
├── server_h2.go        # HTTP/2 server implementation
├── client.go           # Common client functionality
├── client_h1.go        # HTTP/1.1 client dialer
├── client_h2.go        # HTTP/2 client dialer
└── conn.go             # net.Conn wrapper for tunnels
```

## API Overview

### Server Types

- `NewHandler(cfg)` - Unified handler for both HTTP/1.1 and HTTP/2 (recommended)
- `NewH1Handler(cfg)` - HTTP/1.1 specific handler
- `NewH2Handler(cfg)` - HTTP/2 specific handler

### Client Types

- `NewH1Dialer(cfg)` - HTTP/1.1 client
- `NewH2Dialer(cfg)` - HTTP/2 over TLS client
- `NewH2CDialer(cfg)` - HTTP/2 cleartext client

### Configuration

```go
type ServerConfig struct {
    OnTunnel TunnelFunc  // Callback for tunnel establishment
    Dial     DialFunc    // Custom upstream dialer
    ErrorLog Logger      // Error logging
}

type ClientConfig struct {
    ProxyURL    string        // Proxy server URL
    TLSConfig   *tls.Config   // TLS configuration
    Header      http.Header   // Additional headers
    DialContext DialFunc      // Custom proxy connection dialer
}
```

## Standards

- **HTTP/1.1 CONNECT**: [RFC 7231, Section 4.3.6](https://tools.ietf.org/html/rfc7231#section-4.3.6)
- **HTTP/2 CONNECT**: [RFC 9113](https://tools.ietf.org/html/rfc9113)
- **HTTP/2 Cleartext**: [golang.org/x/net/http2/h2c](https://pkg.go.dev/golang.org/x/net/http2/h2c)

## Testing

```bash
go test lds.li/funnelproxy/connecttunnel
```

## License

[Add your license here]

## Contributing

[Add contributing guidelines here]
