# ssh-proxy

An SSH ProxyCommand that forwards SSH connections through a CONNECT proxy. Works with stdin/stdout for seamless SSH integration.

## Features

- **SSH ProxyCommand**: Works as `ProxyCommand` in SSH config
- **Stdin/Stdout**: Reads from stdin, writes to stdout (SSH protocol compatible)
- **Protocol Support**: HTTP/1.1, HTTP/2, and h2c CONNECT proxies
- **Authentication**: Bearer token or other proxy authentication
- **Transparent**: SSH client is unaware of the proxy

## Installation

```bash
go install lds.li/funnelproxy/cmd/ssh-proxy@latest
```

Or build from source:

```bash
go build -o ssh-proxy ./cmd/ssh-proxy
```

## Usage

### Command Line

```bash
ssh-proxy -proxy https://proxy.example.com:443 target.host.com 22
```

This connects stdin/stdout to `target.host.com:22` through the proxy.

### With SSH

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' user@target.host.com
```

SSH automatically substitutes:
- `%h` → target hostname
- `%p` → target port (usually 22)

### In SSH Config

Add to `~/.ssh/config`:

```ssh-config
Host *
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
```

Or for specific hosts:

```ssh-config
Host *.internal.example.com
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
  User myuser

Host bastion
  HostName bastion.internal.example.com
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
```

## Command-Line Options

```
Usage: ssh-proxy [options] <target-host> <target-port>

Options:
  -proxy string
        CONNECT proxy URL (required, e.g., https://proxy.example.com:443)
  -type string
        Proxy type: h1, h2, or h2c (default: auto-detect from URL)
  -auth string
        Proxy authentication (e.g., 'Bearer token' or 'Basic base64')
  -oidc-issuer string
        OIDC issuer URL for automatic token acquisition
  -oidc-client-id string
        OIDC client ID (required if -oidc-issuer is set)
  -oidc-scopes string
        OIDC scopes (comma-separated, default: openid)
  -insecure
        Skip TLS verification
  -timeout duration
        Connection timeout (default 30s)
  -buffer int
        I/O buffer size in bytes (default 32768)
  -verbose
        Enable verbose logging (written to stderr)
```

## Examples

### Basic Usage

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' user@server.com
```

### With Bearer Token Authentication

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 -auth "Bearer my-token" %h %p' user@server.com
```

### With OIDC Authentication

Automatically acquire OIDC tokens:

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 -oidc-issuer https://accounts.google.com -oidc-client-id your-client-id.apps.googleusercontent.com %h %p' user@server.com
```

On first use:
1. Your browser will launch for OAuth2 authentication
2. After authentication, the ID token is cached locally
3. Subsequent SSH connections reuse the cached token
4. Token is automatically refreshed when needed

In `~/.ssh/config`:

```ssh-config
Host *.internal.example.com
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 -oidc-issuer https://accounts.google.com -oidc-client-id your-client-id.apps.googleusercontent.com %h %p
  User myuser
```

### With Custom Timeout

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 -timeout 60s %h %p' user@server.com
```

### Verbose Mode (Debugging)

```bash
ssh -o ProxyCommand='ssh-proxy -verbose -proxy https://proxy.example.com:443 %h %p' user@server.com
```

Logs go to stderr (won't interfere with SSH):
```
2024/02/01 15:30:00 Connecting to server.com:22 via https://proxy.example.com:443
2024/02/01 15:30:01 Connected to server.com:22
```

### HTTP/2 Cleartext (h2c)

```bash
ssh -o ProxyCommand='ssh-proxy -proxy http://proxy.example.com:443 -type h2c %h %p' user@server.com
```

## SSH Config Examples

### Global Proxy for All Hosts

```ssh-config
Host *
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
```

### Proxy for Specific Domain

```ssh-config
Host *.internal.example.com
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
  User myuser
  IdentityFile ~/.ssh/internal_key
```

### Multiple Proxies for Different Networks

```ssh-config
Host *.corp.example.com
  ProxyCommand ssh-proxy -proxy https://corp-proxy.example.com:443 %h %p

Host *.dev.example.com
  ProxyCommand ssh-proxy -proxy https://dev-proxy.example.com:443 %h %p

Host *.prod.example.com
  ProxyCommand ssh-proxy -proxy https://prod-proxy.example.com:443 -auth "Bearer ${PROD_TOKEN}" %h %p
```

### Jump Host Alternative

Instead of SSH jump hosts, use a CONNECT proxy:

```ssh-config
# Old way: SSH jump host
Host internal-server
  HostName internal.example.com
  ProxyJump jump.example.com

# New way: CONNECT proxy
Host internal-server
  HostName internal.example.com
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
```

## Use Cases

### Access Internal Servers

SSH to servers behind a firewall:

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' user@internal-server.local
```

### Bastion Host Alternative

Replace SSH bastion hosts with CONNECT proxies:

```ssh-config
Host *.internal
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
```

### SCP Through Proxy

SCP also supports ProxyCommand:

```bash
scp -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' \
  file.txt user@internal-server.local:/path/
```

### SFTP Through Proxy

SFTP works too:

```bash
sftp -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' \
  user@internal-server.local
```

### Git Over SSH

Works with git SSH URLs:

```ssh-config
Host github-internal
  HostName git.internal.example.com
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
  IdentityFile ~/.ssh/git_key
```

```bash
git clone git@github-internal:myorg/myrepo.git
```

### Rsync Through Proxy

Rsync over SSH:

```bash
rsync -avz -e "ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p'" \
  /local/path/ user@internal-server.local:/remote/path/
```

## Authentication

### Bearer Token

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 -auth "Bearer my-secret-token" %h %p' user@server.com
```

### Basic Auth

```bash
# Encode credentials
echo -n "user:pass" | base64
# Output: dXNlcjpwYXNz

ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 -auth "Basic dXNlcjpwYXNz" %h %p' user@server.com
```

### Environment Variables

Store tokens in environment:

```bash
export PROXY_TOKEN="my-secret-token"

ssh -o ProxyCommand="ssh-proxy -proxy https://proxy.example.com:443 -auth 'Bearer $PROXY_TOKEN' %h %p" user@server.com
```

Or in SSH config with a wrapper script:

```bash
#!/bin/bash
# ~/.ssh/proxy-with-auth.sh
ssh-proxy -proxy https://proxy.example.com:443 -auth "Bearer $PROXY_TOKEN" "$@"
```

```ssh-config
Host *
  ProxyCommand ~/.ssh/proxy-with-auth.sh %h %p
```

## Integration with SSH Tools

### Ansible

```yaml
# ansible.cfg or inventory
[all:vars]
ansible_ssh_common_args='-o ProxyCommand="ssh-proxy -proxy https://proxy.example.com:443 %h %p"'
```

### Terraform

```hcl
provisioner "remote-exec" {
  connection {
    type = "ssh"
    host = "internal-server.local"
    user = "admin"

    bastion_host = "proxy.example.com"
    bastion_port = 443
    # Or use ProxyCommand alternative
  }
}
```

### SSH Multiplexing

Works with SSH ControlMaster:

```ssh-config
Host *.internal
  ProxyCommand ssh-proxy -proxy https://proxy.example.com:443 %h %p
  ControlMaster auto
  ControlPath ~/.ssh/control-%r@%h:%p
  ControlPersist 10m
```

## How It Works

1. SSH invokes `ssh-proxy` as ProxyCommand
2. `ssh-proxy` connects to the CONNECT proxy
3. Proxy establishes tunnel to target SSH server
4. `ssh-proxy` bridges stdin/stdout ↔ tunnel
5. SSH protocol flows through the tunnel

```
SSH Client → stdin/stdout → ssh-proxy → CONNECT Proxy → Target SSH Server
```

## Debugging

### Verbose SSH

See what SSH is doing:

```bash
ssh -vvv -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' user@server.com
```

### Verbose Proxy

See what ssh-proxy is doing:

```bash
ssh -o ProxyCommand='ssh-proxy -verbose -proxy https://proxy.example.com:443 %h %p' user@server.com
```

### Test Connection Manually

Test the proxy connection:

```bash
ssh-proxy -verbose -proxy https://proxy.example.com:443 example.com 22 < /dev/null
```

Should connect and then close (since stdin closes immediately).

### Common Issues

**"Connection refused"**
- Target server not reachable through proxy
- Firewall blocking connection
- Target port incorrect

**"Connection timeout"**
- Proxy not responding
- Increase timeout: `-timeout 60s`

**"Proxy authentication required"**
- Add `-auth` flag with credentials

**"Protocol mismatch"**
- Wrong proxy type selected
- Try different `-type` (h1, h2, h2c)

## Performance Tuning

### Buffer Size

Larger buffers for better throughput:

```bash
ssh -o ProxyCommand='ssh-proxy -buffer 65536 -proxy https://proxy.example.com:443 %h %p' user@server.com
```

### Connection Timeout

Adjust for slow networks:

```bash
ssh -o ProxyCommand='ssh-proxy -timeout 60s -proxy https://proxy.example.com:443 %h %p' user@server.com
```

### SSH Compression

Enable SSH compression for slow connections:

```bash
ssh -C -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' user@server.com
```

## Security Considerations

### TLS Verification

By default, TLS certificates are verified. Only use `-insecure` for testing:

```bash
# Testing only!
ssh -o ProxyCommand='ssh-proxy -insecure -proxy https://self-signed.example.com:443 %h %p' user@server.com
```

### Proxy Authentication

Always use authentication for production proxies:

```bash
ssh -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 -auth "Bearer $TOKEN" %h %p' user@server.com
```

### SSH Keys

ProxyCommand doesn't affect SSH authentication. Use SSH keys as normal:

```bash
ssh -i ~/.ssh/mykey -o ProxyCommand='ssh-proxy -proxy https://proxy.example.com:443 %h %p' user@server.com
```

## Comparison with Other Methods

### vs SSH ProxyJump

**SSH ProxyJump:**
- Requires SSH access to jump host
- Jump host must be SSH server
- Double authentication (jump + target)

**ssh-proxy:**
- Only needs CONNECT proxy (not SSH)
- Single authentication flow
- Proxy can be shared service

### vs VPN

**VPN:**
- Full network access
- All traffic routed
- Requires VPN client

**ssh-proxy:**
- Per-connection tunneling
- Only SSH traffic
- No special client needed

### vs Direct SSH

**Direct SSH:**
- Simple
- Target must be publicly accessible

**ssh-proxy:**
- Access internal servers
- No public exposure needed
- Centralized access control

## Troubleshooting

### SSH Hangs

Check if proxy is responsive:
```bash
curl -v --proxy https://proxy.example.com:443 https://example.com
```

### Authentication Fails

Test proxy auth:
```bash
curl -v --proxy https://proxy.example.com:443 \
  -H "Proxy-Authorization: Bearer token" \
  https://example.com
```

### Wrong Proxy Type

Try different types:
```bash
# Try HTTP/1.1
ssh -o ProxyCommand='ssh-proxy -type h1 -proxy https://proxy.example.com:443 %h %p' user@server.com

# Try HTTP/2
ssh -o ProxyCommand='ssh-proxy -type h2 -proxy https://proxy.example.com:443 %h %p' user@server.com
```

## License

[Add your license here]
