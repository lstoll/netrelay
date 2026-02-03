# Manual Test Plan for local-proxy

## Prerequisites
- A running CONNECT proxy server (can use ts-server or examples/basic-proxy)
- curl, nc (netcat) installed

## Test 1: Basic Startup
```bash
./local-proxy -proxy http://localhost:8080
```

Expected:
- ✓ Local proxy listening on localhost:8080
- ✓ Tunneling via http://localhost:8080
- No errors

## Test 2: HTTPS via CONNECT
```bash
# Terminal 1: Start test proxy server
cd examples/basic-proxy && go run main.go

# Terminal 2: Start local-proxy
./local-proxy -proxy http://localhost:8080

# Terminal 3: Test with curl
curl -x http://localhost:8080 https://example.com -v
```

Expected:
- Connection established via CONNECT
- HTML response from example.com
- No errors

## Test 3: HTTP via CONNECT
```bash
curl -x http://localhost:8080 http://example.com -v
```

Expected:
- Connection established via CONNECT
- HTML response from example.com

## Test 4: Reject Non-CONNECT Methods
```bash
curl -x http://localhost:8080 http://example.com --proxy-header "Connection: keep-alive" -v
# Or try: curl http://localhost:8080 (direct request, not via proxy)
```

Expected:
- Should still work (curl uses CONNECT automatically for -x flag)

## Test 5: Verbose Mode
```bash
./local-proxy -proxy http://localhost:8080 -verbose
```

Then make a request from another terminal:
```bash
curl -x http://localhost:8080 https://example.com
```

Expected:
- Verbose logs showing:
  - CONNECT request received
  - Target destination
  - Connection established
  - Connection closed

## Test 6: Custom Listen Address
```bash
./local-proxy -proxy http://localhost:8080 -listen localhost:3128
curl -x http://localhost:3128 https://example.com
```

Expected:
- Local proxy listens on port 3128
- Request succeeds

## Test 7: Graceful Shutdown
```bash
./local-proxy -proxy http://localhost:8080
# Press Ctrl+C
```

Expected:
- "Shutting down gracefully..." message
- "Server stopped" message
- Clean exit

## Test 8: Invalid Proxy URL
```bash
./local-proxy -proxy http://invalid-proxy-that-doesnt-exist:9999
curl -x http://localhost:8080 https://example.com
```

Expected:
- Request fails with connection error
- Clear error message in local-proxy logs

## Test 9: SSH via nc (if nc supports -X)
```bash
# Terminal 1: Start local-proxy
./local-proxy -proxy http://localhost:8080

# Terminal 2: Test SSH (to a server you have access to)
ssh -o ProxyCommand='nc -X connect -x localhost:8080 %h %p' user@server
```

Expected:
- SSH connection works through proxy
- Can authenticate and get shell

## Test 10: Environment Variables
```bash
export http_proxy=http://localhost:8080
export https_proxy=http://localhost:8080

./local-proxy -proxy http://localhost:8080 &

curl https://example.com  # Should use proxy automatically
```

Expected:
- curl automatically uses the proxy
- Request succeeds

## Test 11: Multiple Concurrent Connections
```bash
./local-proxy -proxy http://localhost:8080 -verbose &

for i in {1..10}; do
  curl -x http://localhost:8080 https://example.com > /dev/null 2>&1 &
done
wait
```

Expected:
- All 10 requests succeed
- No connection errors
- Verbose logs show all connections

## Test 12: Help Output
```bash
./local-proxy -h
```

Expected:
- Usage information displayed
- All flags documented
- Examples shown

## Cleanup
```bash
# Kill any background processes
pkill -f local-proxy

# Unset environment variables
unset http_proxy https_proxy
```
