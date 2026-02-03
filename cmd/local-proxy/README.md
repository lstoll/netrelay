# local-proxy

A local HTTP CONNECT proxy server that tunnels all traffic through a remote CONNECT proxy.

## Features

- **Universal Compatibility**: Works with any tool that supports HTTP CONNECT proxies
- **Protocol Support**: HTTP/1.1, HTTP/2, and h2c CONNECT to remote proxy
- **OIDC Authentication**: Automatic token acquisition and refresh
- **Token Refresh**: Long-running sessions with automatic OIDC token renewal
- **Simple Architecture**: Single local proxy for all your tools
- **Graceful Shutdown**: Clean connection closure on Ctrl+C

## Installation

```bash
go install lds.li/funnelproxy/cmd/local-proxy@latest
```

Or build from source:

```bash
go build -o local-proxy ./cmd/local-proxy
```

## Usage

### Quick Start

Start the local proxy:

```bash
local-proxy -proxy https://proxy.example.com:443
```

This starts a local HTTP CONNECT proxy on `localhost:8080` that tunnels through your remote proxy.

### Use with curl

```bash
# HTTPS request (uses CONNECT)
curl -x http://localhost:8080 https://example.com

# HTTP request (also uses CONNECT)
curl -x http://localhost:8080 http://example.com
```

### Use with SSH

```bash
# Command line
ssh -o ProxyCommand='nc -X connect -x localhost:8080 %h %p' user@server.com

# Or add to ~/.ssh/config
Host *.internal.example.com
  ProxyCommand nc -X connect -x localhost:8080 %h %p
```

### Use with Environment Variables

Many tools respect the standard proxy environment variables:

```bash
export http_proxy=http://localhost:8080
export https_proxy=http://localhost:8080

# Now these use the proxy automatically
curl https://example.com
wget https://example.com
git clone https://github.com/user/repo.git
```

### Use with Browser

Configure your browser to use `localhost:8080` as an HTTP proxy:

- **Chrome/Edge**: Settings → System → Open proxy settings
- **Firefox**: Settings → Network Settings → Manual proxy configuration
- **Safari**: System Preferences → Network → Advanced → Proxies

## Command-Line Options

```
Usage: local-proxy [options]

Options:
  -listen string
        Local proxy listen address (default "localhost:8080")
  -proxy string
        CONNECT proxy URL (required, e.g., https://proxy.example.com:443)
  -type string
        Proxy type: h1, h2, or h2c (default: auto-detect from URL)
  -auth string
        Proxy authentication header value (e.g., 'Bearer token')
  -oidc-issuer string
        OIDC issuer URL for automatic token acquisition
  -oidc-client-id string
        OIDC client ID (required if -oidc-issuer is set)
  -oidc-scopes string
        OIDC scopes (comma-separated, default: openid)
  -insecure
        Skip TLS verification
  -verbose
        Enable verbose logging
```

## Authentication

### Bearer Token

Use a static bearer token:

```bash
local-proxy -proxy https://proxy.example.com:443 \
  -auth 'Bearer my-secret-token'
```

### OIDC (Recommended)

Automatically acquire and refresh OIDC ID tokens:

```bash
local-proxy -proxy https://proxy.example.com:443 \
  -oidc-issuer https://accounts.google.com \
  -oidc-client-id your-client-id.apps.googleusercontent.com
```

This will:
1. Automatically launch your browser for OAuth2 authentication
2. Acquire an ID token from the OIDC provider
3. Use the ID token in `Proxy-Authorization: Bearer <id-token>`
4. Automatically refresh tokens before they expire
5. Cache tokens locally for reuse

Supported OIDC providers:
- Google: `https://accounts.google.com`
- Azure AD: `https://login.microsoftonline.com/{tenant}/v2.0`
- Okta: `https://{domain}.okta.com`
- Auth0: `https://{domain}.auth0.com`
- Any OIDC-compliant provider

## Advanced Configuration

### Custom Listen Address

Listen on a different port:

```bash
local-proxy -proxy https://proxy.example.com:443 -listen localhost:3128
```

Listen on all interfaces (not recommended for security):

```bash
local-proxy -proxy https://proxy.example.com:443 -listen :8080
```

### Force Proxy Protocol Type

The proxy type is auto-detected from the URL scheme, but you can override it:

```bash
# Force HTTP/2 cleartext (h2c)
local-proxy -proxy http://proxy.example.com:443 -type h2c

# Force HTTP/1.1
local-proxy -proxy https://proxy.example.com:443 -type h1

# Force HTTP/2 over TLS
local-proxy -proxy https://proxy.example.com:443 -type h2
```

Auto-detection rules:
- `https://` → uses `h2` (HTTP/2 over TLS)
- `http://` → uses `h1` (HTTP/1.1)

### Verbose Logging

See detailed connection logs:

```bash
local-proxy -verbose -proxy https://proxy.example.com:443
```

Output:
```
✓ Local proxy listening on localhost:8080
✓ Tunneling via https://proxy.example.com:443
✓ OIDC authentication enabled
CONNECT request to example.com:443 from 127.0.0.1:52341
Connected to example.com:443
Connection to example.com:443 closed
```

### Skip TLS Verification (Testing Only)

Only use for self-signed certificates in development:

```bash
local-proxy -insecure -proxy https://self-signed.example.com:443
```

## Use Cases

### Web Development

Access internal APIs and services:

```bash
# Start local proxy
local-proxy -proxy https://proxy.example.com:443 &

# Set environment variables
export http_proxy=http://localhost:8080
export https_proxy=http://localhost:8080

# Use your tools normally
curl https://internal-api.example.com/v1/users
npm install  # Downloads through proxy
docker pull myregistry.example.com/image  # If Docker configured
```

### SSH to Internal Servers

```bash
# Start local proxy
local-proxy -proxy https://proxy.example.com:443 &

# SSH via proxy using nc (netcat)
ssh -o ProxyCommand='nc -X connect -x localhost:8080 %h %p' user@internal.example.com

# Or configure in ~/.ssh/config
cat >> ~/.ssh/config <<EOF
Host *.internal.example.com
  ProxyCommand nc -X connect -x localhost:8080 %h %p
EOF

# Then SSH normally
ssh user@server.internal.example.com
```

Alternative: Use `socat` instead of `nc`:
```bash
ssh -o ProxyCommand='socat - PROXY:localhost:%h:%p,proxyport=8080' user@server
```

### Database Access

```bash
# Start local proxy
local-proxy -proxy https://proxy.example.com:443 &

# Use proxychains or similar tool for TCP forwarding
# Or use application-level proxy support
psql "postgresql://user:pass@db.internal.example.com:5432/mydb?sslmode=require" \
  --set=http_proxy=http://localhost:8080
```

Note: Most database clients don't support HTTP CONNECT proxies directly. Consider using:
- `proxychains` for transparent proxying
- Port forwarding tools that support HTTP CONNECT
- Application-specific proxy settings

### Browser Traffic

Configure your browser to use the local proxy and all traffic will tunnel through the remote proxy.

### Git Operations

```bash
# Set proxy environment variables
export http_proxy=http://localhost:8080
export https_proxy=http://localhost:8080

# Git operations now use the proxy
git clone https://github.com/user/repo.git
git push origin main
```

Or configure Git directly:
```bash
git config --global http.proxy http://localhost:8080
git config --global https.proxy http://localhost:8080
```

## Connection Management

### Graceful Shutdown

Press `Ctrl+C` to shut down gracefully:

```
^C
Shutting down gracefully...
Server stopped
```

The server will:
1. Stop accepting new connections
2. Wait up to 5 seconds for active connections to finish
3. Close all connections and exit

### Active Connections

Each connection remains active until:
- Either side closes the connection
- An error occurs
- The shutdown timeout is reached

## Security Considerations

### Local Binding

**Recommended**: Only listen on localhost:

```bash
local-proxy -proxy https://proxy.example.com:443 -listen localhost:8080
```

**Not Recommended**: Listening on all interfaces exposes the proxy to other machines:

```bash
local-proxy -proxy https://proxy.example.com:443 -listen :8080  # Exposed!
```

### Authentication

Always use authentication for production proxies:

```bash
# OIDC (recommended for automatic refresh)
local-proxy -proxy https://proxy.example.com:443 \
  -oidc-issuer https://accounts.google.com \
  -oidc-client-id your-client-id.apps.googleusercontent.com

# Or bearer token
local-proxy -proxy https://proxy.example.com:443 \
  -auth 'Bearer my-secret-token'
```

### Token Refresh

When using OIDC, tokens are automatically refreshed before expiry. This allows long-running proxy sessions without re-authentication.

### TLS Verification

Only use `-insecure` for testing with self-signed certificates:

```bash
# Testing only!
local-proxy -insecure -proxy https://self-signed.example.com:443
```

## How It Works

1. **Client connects** to local proxy (e.g., `curl -x http://localhost:8080 https://example.com`)
2. **CONNECT request** is received by local proxy
3. **Local proxy dials** through remote CONNECT proxy to destination
4. **Bidirectional tunnel** is established
5. **Data flows** transparently: Client ↔ Local Proxy ↔ Remote Proxy ↔ Destination

The local proxy only supports CONNECT requests (not plain HTTP proxy). This is the standard method used by all modern tools for both HTTP and HTTPS traffic.

## Logging

### Normal Mode

Shows startup and error information:

```
✓ Local proxy listening on localhost:8080
✓ Tunneling via https://proxy.example.com:443
✓ OIDC authentication enabled
```

### Verbose Mode

Shows detailed connection activity:

```
✓ Local proxy listening on localhost:8080
✓ Tunneling via https://proxy.example.com:443
✓ OIDC token acquired
CONNECT request to example.com:443 from 127.0.0.1:52341
Connected to example.com:443
Connection to example.com:443 closed
```

## Error Handling

### Non-CONNECT Requests

The proxy only handles CONNECT requests. Other methods are rejected:

```
Method not allowed. This proxy only supports CONNECT.
```

If you see this error, your client is trying to use plain HTTP proxy mode instead of CONNECT tunneling.

### Proxy Connection Failures

```
Failed to dial example.com:443: connecttunnel: proxy returned 403 Forbidden
```

Common causes:
- Authentication required (use `-auth` or `-oidc-*`)
- Token expired (automatic refresh should prevent this with OIDC)
- Target blocked by proxy policy
- Network connectivity issues

### Token Refresh Failures

```
Token refresh failed: failed to get token: <error>
```

If OIDC token refresh fails, the connection is rejected with 407 Proxy Authentication Required. Restart the proxy to re-authenticate.

### Listen Failures

```
Server error: listen tcp 127.0.0.1:8080: bind: address already in use
```

Solution: Use a different local port with `-listen localhost:PORT`

## Comparison with Port Forwarding

**Old approach** (port forwarding):
```bash
# Had to specify each destination upfront
port-forward -proxy https://proxy.example.com:443 \
  -forward localhost:8080=example.com:80 \
  -forward localhost:8443=example.com:443 \
  -forward localhost:2222=server.example.com:22
```

**New approach** (local proxy):
```bash
# Start once, use with any tool
local-proxy -proxy https://proxy.example.com:443

# Then use standard tools
curl -x http://localhost:8080 https://example.com
ssh -o ProxyCommand='nc -X connect -x localhost:8080 %h %p' user@server.example.com
```

Benefits:
- No need to specify destinations upfront
- Universal tool support
- Dynamic destination selection
- Single proxy server for all traffic

## Integration with Other Tools

### SystemD Service

Create `/etc/systemd/system/local-proxy.service`:

```ini
[Unit]
Description=Local CONNECT Proxy
After=network.target

[Service]
Type=simple
User=proxy
ExecStart=/usr/local/bin/local-proxy \
  -proxy https://proxy.example.com:443 \
  -oidc-issuer https://accounts.google.com \
  -oidc-client-id your-client-id.apps.googleusercontent.com
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Then:
```bash
sudo systemctl enable local-proxy
sudo systemctl start local-proxy
```

### Docker

Run in a container:

```dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go build -o local-proxy ./cmd/local-proxy

FROM debian:bookworm-slim
COPY --from=builder /app/local-proxy /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["local-proxy"]
CMD ["-listen", "0.0.0.0:8080", "-proxy", "https://proxy.example.com:443"]
```

### Proxychains

Use with proxychains for tools that don't support HTTP proxies:

Edit `/etc/proxychains.conf`:
```
[ProxyList]
http 127.0.0.1 8080
```

Then:
```bash
local-proxy -proxy https://proxy.example.com:443 &
proxychains telnet internal.example.com 23
```

## Troubleshooting

### "nc: invalid option -- 'X'"

Your version of `nc` (netcat) doesn't support the `-X` flag. Try:

```bash
# BSD netcat (macOS, some Linux)
ssh -o ProxyCommand='nc -X connect -x localhost:8080 %h %p' user@server

# GNU netcat alternative - use socat
ssh -o ProxyCommand='socat - PROXY:localhost:%h:%p,proxyport=8080' user@server

# Or install ncat (from nmap)
ssh -o ProxyCommand='ncat --proxy localhost:8080 --proxy-type http %h %p' user@server
```

### Port Already in Use

```
Server error: listen tcp 127.0.0.1:8080: bind: address already in use
```

Find what's using the port:
```bash
lsof -i :8080
# or
netstat -an | grep 8080
```

Then either stop that service or use a different port:
```bash
local-proxy -proxy https://proxy.example.com:443 -listen localhost:3128
```

### Browser Not Using Proxy

Make sure:
1. Browser proxy settings point to `localhost:8080` (or your custom port)
2. Proxy type is set to "HTTP" (not SOCKS)
3. "Use this proxy server for all protocols" is enabled
4. No proxy exceptions are blocking your target

### Tools Not Respecting Environment Variables

Some tools require explicit proxy configuration:

```bash
# curl
curl --proxy http://localhost:8080 https://example.com

# wget
wget -e use_proxy=yes -e http_proxy=localhost:8080 https://example.com

# npm
npm config set proxy http://localhost:8080
npm config set https-proxy http://localhost:8080

# yarn
yarn config set proxy http://localhost:8080
yarn config set https-proxy http://localhost:8080
```

## License

[Add your license here]
