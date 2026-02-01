# port-forward

A port forwarding client that tunnels local ports to remote hosts through a CONNECT proxy.

## Features

- **Multiple Port Forwards**: Forward multiple local ports to different remote destinations
- **Named Forwards**: Optionally name each forward for better logging
- **Protocol Support**: HTTP/1.1, HTTP/2, and h2c CONNECT proxies
- **Authentication**: Bearer token or other proxy authentication
- **Graceful Shutdown**: Clean connection closure on Ctrl+C

## Installation

```bash
go install lds.li/funnelproxy/cmd/port-forward@latest
```

Or build from source:

```bash
go build -o port-forward ./cmd/port-forward
```

## Usage

### Basic Example

Forward localhost:8080 to example.com:80 through a proxy:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward localhost:8080=example.com:80
```

### Multiple Forwards

Forward multiple local ports:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward localhost:8080=example.com:80 \
  -forward localhost:8443=example.com:443 \
  -forward localhost:5432=db.example.com:5432
```

### Named Forwards

Use custom names for better logging:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward web=localhost:8080=example.com:80 \
  -forward api=localhost:8081=api.example.com:443
```

### With Authentication

Add proxy authentication:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -auth 'Bearer my-secret-token' \
  -forward localhost:8080=example.com:80
```

### Listen on All Interfaces

Forward from all interfaces (not just localhost):

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward :8080=example.com:80
```

### Verbose Logging

Enable detailed connection logging:

```bash
port-forward -verbose \
  -proxy https://proxy.example.com:443 \
  -forward localhost:8080=example.com:80
```

## Command-Line Options

```
Usage: port-forward [options]

Options:
  -proxy string
        CONNECT proxy URL (required, e.g., https://proxy.example.com:443)
  -forward value
        Port forward in format [name=]listen:port=remote:port (can be repeated)
  -type string
        Proxy type: h1, h2, or h2c (default: auto-detect from URL)
  -auth string
        Proxy authentication header value (e.g., 'Bearer token')
  -insecure
        Skip TLS verification
  -verbose
        Enable verbose logging
```

## Forward Format

The `-forward` flag accepts two formats:

### Simple Format

```
-forward listen:port=remote:port
```

Example: `-forward localhost:8080=example.com:80`

### Named Format

```
-forward name=listen:port=remote:port
```

Example: `-forward web=localhost:8080=example.com:80`

### Format Details

- **listen:port**: Local address and port to listen on
  - Use `localhost:8080` to listen only on localhost
  - Use `:8080` to listen on all interfaces
  - Port is required

- **remote:port**: Remote destination to forward to
  - Must include hostname and port
  - Example: `example.com:80`, `192.168.1.1:22`

- **name** (optional): A friendly name for logging
  - If not provided, uses the listen address as the name

## Use Cases

### Database Access

Forward a local port to a remote database:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward postgres=localhost:5432=db.internal.example.com:5432

# Now connect to localhost:5432
psql -h localhost -p 5432 -U user database
```

### Web Development

Access internal web services:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward localhost:8080=internal-api.example.com:80 \
  -forward localhost:8443=internal-web.example.com:443

# Access at http://localhost:8080 and https://localhost:8443
```

### SSH Jump Host

Forward SSH through a proxy:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward localhost:2222=internal-server.example.com:22

ssh -p 2222 user@localhost
```

### Multiple Services

Forward multiple services simultaneously:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward db=localhost:5432=db.internal:5432 \
  -forward redis=localhost:6379=redis.internal:6379 \
  -forward api=localhost:8080=api.internal:80
```

## Connection Management

### Graceful Shutdown

Press `Ctrl+C` to shut down gracefully:

```
^C
Shutting down gracefully...
All connections closed
```

The command will:
1. Stop accepting new connections
2. Close all listeners
3. Wait up to 5 seconds for active connections to finish

### Active Connections

Existing connections remain active until:
- Either side closes the connection
- An error occurs
- The shutdown timeout (5s) is reached

## Logging

### Normal Mode

Shows startup and connection info:

```
✓ [localhost:8080] Forwarding 127.0.0.1:8080 -> example.com:80 (via https://proxy.example.com:443)
✓ All forwards active - press Ctrl+C to stop
```

### Verbose Mode

Shows detailed connection activity:

```bash
port-forward -verbose -proxy ... -forward ...
```

Output:
```
✓ [web] Forwarding 127.0.0.1:8080 -> example.com:80 (via https://proxy.example.com:443)
✓ All forwards active - press Ctrl+C to stop
[web] New connection from 127.0.0.1:52341
[web] Connected to example.com:80
[web] Connection closed
```

## Proxy Types

The `-type` flag allows you to specify the proxy protocol:

- **h1**: HTTP/1.1 CONNECT (standard)
- **h2**: HTTP/2 CONNECT over TLS
- **h2c**: HTTP/2 CONNECT cleartext (no TLS)

If not specified, auto-detects from URL scheme:
- `https://` → uses `h2`
- `http://` → uses `h1`

Force HTTP/2 cleartext:
```bash
port-forward -proxy http://proxy.example.com:443 -type h2c \
  -forward localhost:8080=example.com:80
```

## Error Handling

### Connection Failures

If the proxy connection fails:

```
[web] Failed to dial example.com:80: connecttunnel: proxy returned 403 Forbidden
```

Common causes:
- Authentication required (use `-auth`)
- Target blocked by proxy policy
- Network connectivity issues

### Listen Failures

If a local port is already in use:

```
Failed to listen on localhost:8080: address already in use
```

Solution: Use a different local port or stop the conflicting service.

## Examples

### Connect to Internal Kubernetes API

```bash
port-forward -proxy https://proxy.example.com:443 \
  -auth 'Bearer my-token' \
  -forward k8s=localhost:6443=k8s.internal.example.com:6443

kubectl --server=https://localhost:6443 get pods
```

### MySQL Tunnel

```bash
port-forward -proxy https://proxy.example.com:443 \
  -forward mysql=localhost:3306=mysql.internal.example.com:3306

mysql -h 127.0.0.1 -P 3306 -u user -p
```

### Multiple Environment Access

```bash
# Development
port-forward -proxy https://proxy.example.com:443 \
  -forward dev-db=localhost:5432=dev-db.internal:5432 \
  -forward dev-api=localhost:8080=dev-api.internal:80

# Production (different terminal)
port-forward -proxy https://proxy.example.com:443 \
  -forward prod-db=localhost:5433=prod-db.internal:5432 \
  -forward prod-api=localhost:8081=prod-api.internal:80
```

## Security Considerations

### Local Binding

When forwarding on all interfaces (`:8080`), the port is accessible from other machines:

```bash
# Accessible from other machines
port-forward -proxy ... -forward :8080=example.com:80

# Only accessible from localhost (recommended)
port-forward -proxy ... -forward localhost:8080=example.com:80
```

### Authentication

Always use authentication for production proxies:

```bash
port-forward -proxy https://proxy.example.com:443 \
  -auth 'Bearer my-secret-token' \
  -forward localhost:8080=example.com:80
```

### TLS Verification

Only use `-insecure` for testing:

```bash
# Testing only!
port-forward -insecure -proxy https://self-signed.example.com:443 \
  -forward localhost:8080=example.com:80
```

## Troubleshooting

### Port Already in Use

```
Failed to listen on localhost:8080: address already in use
```

Find what's using the port:
```bash
lsof -i :8080
# or
netstat -an | grep 8080
```

### Proxy Connection Timeout

Increase timeout (not currently configurable, hardcoded to 30s per connection).

### No Route to Host

Ensure the proxy can reach the remote target. Some proxies may block certain destinations.

## Integration with Other Tools

### Docker

```bash
# Start port-forward in background
port-forward -proxy https://proxy.example.com:443 \
  -forward localhost:5432=db.example.com:5432 &

# Run container connecting to forwarded port
docker run -e DATABASE_URL=postgresql://localhost:5432/db myapp
```

### SystemD Service

Create `/etc/systemd/system/port-forward.service`:

```ini
[Unit]
Description=Port Forward via CONNECT Proxy
After=network.target

[Service]
Type=simple
User=port-forward
ExecStart=/usr/local/bin/port-forward \
  -proxy https://proxy.example.com:443 \
  -auth 'Bearer ${PROXY_TOKEN}' \
  -forward localhost:5432=db.example.com:5432
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## License

[Add your license here]
