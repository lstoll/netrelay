# ts-server

A Tailscale Funnel-enabled CONNECT proxy server that uses Tailscale as a library to expose the proxy to the public internet.

## Features

- **Tailscale Funnel Integration**: Automatically exposes the proxy to the internet via Tailscale Funnel
- **HTTP/1.1 and HTTP/2 Support**: Handles both CONNECT protocols
- **h2c (HTTP/2 Cleartext)**: Supports HTTP/2 without TLS (Tailscale handles TLS termination)
- **Multiple Authentication Methods**: Bearer token or OIDC/OAuth2 ID token authentication
- **Automatic TLS**: Tailscale Funnel provides automatic HTTPS with valid certificates

## Installation

```bash
go install lds.li/funnelproxy/cmd/ts-server@latest
```

Or build from source:

```bash
go build -o ts-server ./cmd/ts-server
```

## Quick Start

### 1. Basic Usage (No Authentication)

```bash
ts-server
```

This will:
- Start a Tailscale node
- Enable Funnel on port 443
- Expose a CONNECT proxy at `https://<hostname>.ts.net:443`

### 2. With Custom Hostname

```bash
ts-server -hostname my-proxy
```

Access at: `https://my-proxy.ts.net:443`

### 3. With Bearer Token Authentication

```bash
ts-server -auth -auth-token my-secret-token
```

Clients must include the header:
```
Proxy-Authorization: Bearer my-secret-token
```

### 4. With OIDC Authentication

```bash
ts-server -oidc-issuer https://accounts.google.com \
  -oidc-audience my-client-id
```

Clients must include a valid OIDC ID token:
```
Proxy-Authorization: Bearer <id-token>
```

Supported OIDC providers:
- Google (https://accounts.google.com)
- Azure AD (https://login.microsoftonline.com/{tenant}/v2.0)
- Okta (https://{domain}.okta.com)
- Auth0 (https://{domain}.auth0.com)
- Any OIDC-compliant provider

The server will:
- Validate the token signature against the provider's JWKS
- Verify the token hasn't expired
- Check the audience matches your client ID
- Extract user identity (email/subject) for logging

### 5. With Tailscale Auth Key (for unattended setup)

```bash
ts-server -authkey tskey-auth-xxxxx -hostname my-proxy
```

Get an auth key from: https://login.tailscale.com/admin/settings/keys

## Usage

```
Usage of ts-server:
  -auth
        Enable simple bearer token authentication
  -auth-token string
        Authentication token (required if -auth is set)
  -oidc-issuer string
        OIDC issuer URL (e.g., https://accounts.google.com)
  -oidc-audience string
        OIDC audience/client ID (required if -oidc-issuer is set)
  -authkey string
        Tailscale auth key (optional, uses existing auth if not provided)
  -hostname string
        Tailscale hostname (default: generates one)
  -port string
        Port to listen on (default: 443 for Funnel) (default "443")
  -statedir string
        Directory to store Tailscale state (default: .tsnet-state)
  -verbose
        Enable verbose logging
```

## Client Configuration

### Using with curl

```bash
# Without authentication
curl -x https://my-proxy.ts.net:443 https://example.com

# With bearer token authentication
curl -x https://my-proxy.ts.net:443 \
     -H "Proxy-Authorization: Bearer my-secret-token" \
     https://example.com

# With OIDC authentication (ID token)
curl -x https://my-proxy.ts.net:443 \
     -H "Proxy-Authorization: Bearer <id-token>" \
     https://example.com
```

### Using with the connecttunnel library

```go
import "lds.li/funnelproxy/connecttunnel"

// Without authentication
dialer := connecttunnel.NewH2Dialer(&connecttunnel.ClientConfig{
    ProxyURL: "https://my-proxy.ts.net:443",
})

// With authentication
dialer := connecttunnel.NewH2Dialer(&connecttunnel.ClientConfig{
    ProxyURL: "https://my-proxy.ts.net:443",
    Header: http.Header{
        "Proxy-Authorization": []string{"Bearer my-secret-token"},
    },
})

conn, err := dialer.DialContext(ctx, "tcp", "example.com:443")
```

### Using with standard Go http.Client

```go
proxyURL, _ := url.Parse("https://my-proxy.ts.net:443")
transport := &http.Transport{
    Proxy: http.ProxyURL(proxyURL),
}

// With authentication
transport.ProxyConnectHeader = http.Header{
    "Proxy-Authorization": []string{"Bearer my-secret-token"},
}

client := &http.Client{Transport: transport}
```

## How It Works

1. **Tailscale Connection**: Uses `tsnet` to create a Tailscale node as a library
2. **Funnel Listener**: Creates a listener using `ListenFunnel()` which exposes the service publicly
3. **TLS Termination**: Tailscale Funnel handles TLS, so the proxy receives cleartext HTTP/2
4. **h2c Handler**: The connecttunnel handler is wrapped with h2c support for HTTP/2 cleartext
5. **Protocol Detection**: Automatically handles both HTTP/1.1 and HTTP/2 CONNECT requests

## Architecture

```
Internet → Tailscale Funnel (TLS) → ts-server (h2c) → Upstream Target
          └─ HTTPS termination      └─ CONNECT proxy
```

The proxy receives:
- HTTP/1.1 CONNECT requests (standard)
- HTTP/2 CONNECT requests over cleartext (h2c)

## Security Considerations

### Tailscale Funnel

- Funnel makes your service accessible to the **entire internet**
- Anyone can connect to the proxy unless authentication is enabled
- Use `-auth` and a strong `-auth-token` for production deployments

### Authentication

#### Bearer Token Authentication

For simple use cases, generate a strong random token:

```bash
# Generate a random token
AUTH_TOKEN=$(openssl rand -hex 32)
ts-server -auth -auth-token "$AUTH_TOKEN"
```

#### OIDC Authentication (Recommended for Production)

OIDC provides stronger security with:
- Cryptographic token verification
- Automatic expiration
- User identity tracking
- No shared secrets

Example with Google:

```bash
ts-server -oidc-issuer https://accounts.google.com \
  -oidc-audience your-client-id.apps.googleusercontent.com
```

**Obtaining ID Tokens:**

Clients need to obtain an ID token from the OIDC provider. Example using OAuth2:

```bash
# Get ID token using OAuth2 device flow or other flow
# The token will look like: eyJhbGciOiJSUzI1NiIs...

# Use with curl
curl -x https://my-proxy.ts.net:443 \
  -H "Proxy-Authorization: Bearer $ID_TOKEN" \
  https://example.com
```

**Setting up OIDC Provider:**

1. Register your application with the OIDC provider
2. Get the client ID (this is your audience)
3. Configure the issuer URL
4. Clients authenticate and receive ID tokens
5. Clients include the ID token in `Proxy-Authorization` header

The server logs will show authenticated users:
```
Tunnel: 1.2.3.4 (user@example.com) -> example.com:443 (proto: HTTP/2.0)
```

### Network Access

The proxy can connect to:
- Any public internet address
- Any address accessible from the machine running ts-server
- Other Tailscale nodes in your tailnet

**Important**: Consider firewall rules to limit upstream connectivity if needed.

## Examples

### Private Tailnet Proxy (No Funnel)

For a proxy accessible only within your Tailnet, use `Listen()` instead:

```go
// Modify main.go:
listener, err := srv.Listen("tcp", ":443")
```

### Custom Filtering

Edit `OnTunnel` callback in `main.go` to add custom logic:

```go
OnTunnel: func(ctx context.Context, req *http.Request) error {
    // Block specific domains
    if strings.Contains(req.Host, "blocked.com") {
        return connecttunnel.ErrTunnelRejected
    }

    // Rate limiting per client
    // Authorization checks
    // Logging to external service

    return nil
},
```

## Troubleshooting

### "Failed to listen with Funnel"

1. Ensure Tailscale Funnel is enabled for your tailnet:
   - Go to https://login.tailscale.com/admin/settings
   - Enable "Funnel" under Features

2. Port 443 is recommended (default):
   - Funnel works best on standard HTTPS ports
   - Some clients require port 443 or 8443

### State Directory

Tailscale state is stored in `.tsnet-state` by default. To change:

```bash
ts-server -statedir /var/lib/ts-proxy-state
```

### Logs

Enable verbose logging:

```bash
ts-server -verbose
```

## Production Deployment

### Systemd Service

Create `/etc/systemd/system/ts-proxy.service`:

```ini
[Unit]
Description=Tailscale Funnel CONNECT Proxy
After=network.target

[Service]
Type=simple
User=ts-proxy
ExecStart=/usr/local/bin/ts-server \
    -hostname my-proxy \
    -auth \
    -auth-token ${AUTH_TOKEN} \
    -statedir /var/lib/ts-proxy
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Docker

```dockerfile
FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN go build -o ts-server ./cmd/ts-server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/ts-server /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/ts-server"]
```

Run:
```bash
docker run -v /var/lib/ts-proxy:/state \
    -e TS_AUTHKEY=tskey-auth-xxxxx \
    ts-proxy -statedir /state -auth -auth-token my-token
```

## License

[Add your license here]
