// Package main implements a port forwarder through CONNECT proxies.
//
// This command forwards multiple local ports to remote hosts through a CONNECT proxy.
// Each incoming connection to a local port is tunneled through the proxy to the
// specified remote destination.
//
// Example:
//   port-forward -proxy https://proxy.example.com:443 \
//     -forward localhost:8080=example.com:80 \
//     -forward localhost:8443=example.com:443
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/oauth2"
	"lds.li/funnelproxy/connecttunnel"
	"lds.li/oauth2ext/clitoken"
	"lds.li/oauth2ext/provider"
)

var (
	proxyURL  = flag.String("proxy", "", "CONNECT proxy URL (required, e.g., https://proxy.example.com:443)")
	proxyType = flag.String("type", "", "Proxy type: h1, h2, or h2c (default: auto-detect from URL)")
	proxyAuth = flag.String("auth", "", "Proxy authentication header value (e.g., 'Bearer token')")
	insecure  = flag.Bool("insecure", false, "Skip TLS verification")
	verbose   = flag.Bool("verbose", false, "Enable verbose logging")

	// OIDC authentication flags
	oidcIssuer   = flag.String("oidc-issuer", "", "OIDC issuer URL for automatic token acquisition")
	oidcClientID = flag.String("oidc-client-id", "", "OIDC client ID (required if -oidc-issuer is set)")
	oidcScopes   = flag.String("oidc-scopes", "openid", "OIDC scopes (comma-separated, default: openid)")
)

// forwardFlags is a custom flag type that accepts multiple -forward flags.
type forwardFlags []string

func (f *forwardFlags) String() string {
	return strings.Join(*f, ", ")
}

func (f *forwardFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

var forwards forwardFlags

// forwardConfig represents a single port forward configuration.
type forwardConfig struct {
	name   string
	listen string
	remote string
}

func init() {
	flag.Var(&forwards, "forward", "Port forward in format [name=]listen:port=remote:port (can be repeated)")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Forward local ports to remote hosts through a CONNECT proxy.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Forward localhost:8080 to example.com:80\n")
		fmt.Fprintf(os.Stderr, "  %s -proxy https://proxy.example.com:443 -forward localhost:8080=example.com:80\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Multiple forwards with names\n")
		fmt.Fprintf(os.Stderr, "  %s -proxy https://proxy.example.com:443 \\\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    -forward web=localhost:8080=example.com:80 \\\n")
		fmt.Fprintf(os.Stderr, "    -forward api=localhost:8081=api.example.com:443\n\n")
		fmt.Fprintf(os.Stderr, "  # Forward to any interface\n")
		fmt.Fprintf(os.Stderr, "  %s -proxy https://proxy.example.com:443 -forward :8080=example.com:80\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # With authentication\n")
		fmt.Fprintf(os.Stderr, "  %s -proxy https://proxy.example.com:443 \\\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    -auth 'Bearer my-token' \\\n")
		fmt.Fprintf(os.Stderr, "    -forward localhost:8080=example.com:80\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Validate arguments
	if *proxyURL == "" {
		fmt.Fprintf(os.Stderr, "Error: -proxy is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if len(forwards) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one -forward is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate OIDC configuration
	if *oidcIssuer != "" && *oidcClientID == "" {
		fmt.Fprintf(os.Stderr, "Error: -oidc-client-id is required when -oidc-issuer is set\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if *proxyAuth != "" && *oidcIssuer != "" {
		fmt.Fprintf(os.Stderr, "Error: cannot use both -auth and -oidc-issuer (choose one)\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Acquire OIDC token if configured
	if *oidcIssuer != "" {
		token, err := acquireOIDCToken(context.Background())
		if err != nil {
			log.Fatalf("Failed to acquire OIDC token: %v", err)
		}
		*proxyAuth = "Bearer " + token
		if *verbose {
			log.Println("✓ OIDC token acquired")
		}
	}

	// Parse forward configurations
	configs := make([]forwardConfig, 0, len(forwards))
	for i, fwd := range forwards {
		cfg, err := parseForward(fwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing forward #%d (%s): %v\n", i+1, fwd, err)
			os.Exit(1)
		}
		configs = append(configs, cfg)
	}

	// Create dialer
	dialer, err := createDialer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dialer: %v\n", err)
		os.Exit(1)
	}

	// Start all port forwards
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listeners := make([]net.Listener, 0, len(configs))

	for _, cfg := range configs {
		listener, err := net.Listen("tcp", cfg.listen)
		if err != nil {
			log.Fatalf("Failed to listen on %s: %v", cfg.listen, err)
		}
		listeners = append(listeners, listener)

		log.Printf("✓ [%s] Forwarding %s -> %s (via %s)", cfg.name, listener.Addr(), cfg.remote, *proxyURL)

		wg.Add(1)
		go func(l net.Listener, remote, name string) {
			defer wg.Done()
			acceptLoop(ctx, l, remote, name, dialer)
		}(listener, cfg.remote, cfg.name)
	}

	log.Printf("✓ All forwards active - press Ctrl+C to stop")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\nShutting down gracefully...")
	cancel()

	// Close all listeners
	for _, listener := range listeners {
		listener.Close()
	}

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All connections closed")
	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for connections to close")
	}
}

// parseForward parses a forward flag into a forwardConfig.
// Accepts formats:
//   - listen:port=remote:port
//   - name=listen:port=remote:port
func parseForward(s string) (forwardConfig, error) {
	var cfg forwardConfig

	// Split by '=' to find format
	parts := strings.Split(s, "=")

	switch len(parts) {
	case 2:
		// Format: listen:port=remote:port
		cfg.listen = parts[0]
		cfg.remote = parts[1]
		cfg.name = parts[0] // Use listen address as name
	case 3:
		// Format: name=listen:port=remote:port
		cfg.name = parts[0]
		cfg.listen = parts[1]
		cfg.remote = parts[2]
	default:
		return cfg, fmt.Errorf("invalid format, expected [name=]listen:port=remote:port")
	}

	// Validate listen and remote addresses
	if cfg.listen == "" {
		return cfg, fmt.Errorf("listen address cannot be empty")
	}
	if cfg.remote == "" {
		return cfg, fmt.Errorf("remote address cannot be empty")
	}

	// Ensure listen has a port
	if !strings.Contains(cfg.listen, ":") {
		return cfg, fmt.Errorf("listen address must include port (e.g., localhost:8080 or :8080)")
	}

	// Ensure remote has a port
	if !strings.Contains(cfg.remote, ":") {
		return cfg, fmt.Errorf("remote address must include port (e.g., example.com:80)")
	}

	return cfg, nil
}

// createDialer creates a CONNECT dialer from the flags.
func createDialer() (connecttunnel.Dialer, error) {
	// Build client config
	clientCfg := &connecttunnel.ClientConfig{
		ProxyURL: *proxyURL,
	}

	// Add authentication header if provided
	if *proxyAuth != "" {
		clientCfg.Header = http.Header{
			"Proxy-Authorization": []string{*proxyAuth},
		}
	}

	// Configure TLS if needed
	if *insecure {
		log.Println("Warning: TLS verification disabled")
		clientCfg.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	// Determine dialer type
	dialerType := *proxyType
	if dialerType == "" {
		// Auto-detect from URL
		if strings.HasPrefix(*proxyURL, "https") {
			dialerType = "h2"
		} else {
			dialerType = "h1"
		}
	}

	// Create appropriate dialer
	switch dialerType {
	case "h1":
		return connecttunnel.NewH1Dialer(clientCfg), nil
	case "h2":
		return connecttunnel.NewH2Dialer(clientCfg), nil
	case "h2c":
		return connecttunnel.NewH2CDialer(clientCfg), nil
	default:
		return nil, fmt.Errorf("invalid proxy type: %s (must be h1, h2, or h2c)", dialerType)
	}
}

// acceptLoop accepts connections and forwards them through the proxy.
func acceptLoop(ctx context.Context, listener net.Listener, remote, name string, dialer connecttunnel.Dialer) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Accept with timeout to check context
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		localConn, err := listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue // Timeout, check context
			}
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("[%s] Accept error: %v", name, err)
				continue
			}
		}

		// Handle connection in goroutine
		go handleConnection(ctx, localConn, remote, name, dialer)
	}
}

// handleConnection forwards a single connection through the proxy.
func handleConnection(ctx context.Context, localConn net.Conn, remote, name string, dialer connecttunnel.Dialer) {
	defer localConn.Close()

	if *verbose {
		log.Printf("[%s] New connection from %s", name, localConn.RemoteAddr())
	}

	// Dial through proxy with timeout
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	remoteConn, err := dialer.DialContext(dialCtx, "tcp", remote)
	if err != nil {
		log.Printf("[%s] Failed to dial %s: %v", name, remote, err)
		return
	}
	defer remoteConn.Close()

	if *verbose {
		log.Printf("[%s] Connected to %s", name, remote)
	}

	// Bidirectional copy
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(remoteConn, localConn)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(localConn, remoteConn)
		errCh <- err
	}()

	// Wait for either direction to finish
	err = <-errCh
	if err != nil && *verbose {
		log.Printf("[%s] Copy error: %v", name, err)
	}

	if *verbose {
		log.Printf("[%s] Connection closed", name)
	}
}

// acquireOIDCToken gets an OIDC ID token using the OAuth2 flow.
func acquireOIDCToken(ctx context.Context) (string, error) {
	// Discover the OIDC provider
	p, err := provider.DiscoverOIDCProvider(ctx, *oidcIssuer)
	if err != nil {
		return "", fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	// Parse scopes
	scopes := strings.Split(*oidcScopes, ",")
	for i := range scopes {
		scopes[i] = strings.TrimSpace(scopes[i])
	}

	// Configure OAuth2 client
	oauth2Config := oauth2.Config{
		ClientID: *oidcClientID,
		Endpoint: p.Endpoint(),
		Scopes:   scopes,
	}

	// Create CLI token source with automatic browser flow
	cliConfig := &clitoken.Config{
		OAuth2Config: oauth2Config,
	}

	tokenSource, err := cliConfig.TokenSource(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create token source: %w", err)
	}

	// Get the token (this will launch browser if needed)
	token, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	// Extract ID token
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", fmt.Errorf("no id_token in response")
	}

	return idToken, nil
}
