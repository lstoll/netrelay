// Package main implements a local HTTP CONNECT proxy server.
//
// This command starts a local HTTP proxy server that accepts CONNECT requests
// and tunnels them through a remote CONNECT proxy. This allows any tool that
// supports HTTP CONNECT proxies (curl, browsers, SSH via nc) to tunnel through
// the remote proxy.
//
// Example:
//
//	local-proxy -proxy https://proxy.example.com:443 -listen localhost:8080
//
//	# Then use with any tool:
//	curl -x http://localhost:8080 https://example.com
//	ssh -o ProxyCommand='nc -X connect -x localhost:8080 %h %p' user@server
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
	"lds.li/oauth2ext/clitoken"
	"lds.li/oauth2ext/provider"
	"lds.li/oauth2ext/tokencache"
	connecttunnel "lds.li/tunnel/connect"
)

var (
	listen    = flag.String("listen", "localhost:8080", "Local proxy listen address")
	proxyURL  = flag.String("proxy", "", "CONNECT proxy URL (required, e.g., https://proxy.example.com:443)")
	proxyAuth = flag.String("auth", "", "Proxy authentication header value (e.g., 'Bearer token')")
	insecure  = flag.Bool("insecure", false, "Skip TLS verification")
	verbose   = flag.Bool("verbose", false, "Enable verbose logging")

	// OIDC authentication flags
	oidcIssuer       = flag.String("oidc-issuer", "", "OIDC issuer URL for automatic token acquisition")
	oidcClientID     = flag.String("oidc-client-id", "", "OIDC client ID (required if -oidc-issuer is set)")
	oidcClientSecret = flag.String("oidc-client-secret", "", "OIDC client secret (required if -oidc-issuer is set)")
	oidcScopes       = flag.String("oidc-scopes", "openid", "OIDC scopes (comma-separated, default: openid)")
)

// proxyHandler implements http.Handler for the CONNECT proxy.
type proxyHandler struct {
	dialer      connecttunnel.Dialer
	tokenSource oauth2.TokenSource
	verbose     bool
	dialerMu    sync.RWMutex
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Start a local HTTP CONNECT proxy that tunnels through a remote CONNECT proxy.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Start local proxy\n")
		fmt.Fprintf(os.Stderr, "  %s -proxy https://proxy.example.com:443\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Use with curl\n")
		fmt.Fprintf(os.Stderr, "  curl -x http://localhost:8080 https://example.com\n\n")
		fmt.Fprintf(os.Stderr, "  # Use with SSH\n")
		fmt.Fprintf(os.Stderr, "  ssh -o ProxyCommand='nc -X connect -x localhost:8080 %%h %%p' user@server\n\n")
		fmt.Fprintf(os.Stderr, "  # Use with environment variables\n")
		fmt.Fprintf(os.Stderr, "  export http_proxy=http://localhost:8080\n")
		fmt.Fprintf(os.Stderr, "  export https_proxy=http://localhost:8080\n\n")
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

	// Acquire OIDC token source if configured
	var tokenSource oauth2.TokenSource
	if *oidcIssuer != "" {
		ts, err := createTokenSource(context.Background())
		if err != nil {
			log.Fatalf("Failed to create token source: %v", err)
		}
		if *verbose {
			log.Println("✓ OIDC token source created")
		}
		tokenSource = ts
	}

	// Create dialer
	dialer, err := createDialer(tokenSource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dialer: %v\n", err)
		os.Exit(1)
	}

	// Create proxy handler
	handler := &proxyHandler{
		dialer:      dialer,
		tokenSource: tokenSource,
		verbose:     *verbose,
	}

	// Create HTTP server
	server := &http.Server{
		Addr:    *listen,
		Handler: handler,
		// Disable HTTP/2 for the local server (we only handle CONNECT)
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	log.Printf("✓ Local proxy listening on %s", *listen)
	log.Printf("✓ Tunneling via %s", *proxyURL)
	if tokenSource != nil {
		log.Printf("✓ OIDC authentication enabled")
	}

	// Handle graceful shutdown
	go handleShutdown(server)

	// Start server
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

// ServeHTTP implements http.Handler for the CONNECT proxy.
func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodConnect {
		http.Error(w, "Method not allowed. This proxy only supports CONNECT.", http.StatusMethodNotAllowed)
		if h.verbose {
			log.Printf("Rejected %s request to %s", req.Method, req.URL)
		}
		return
	}
	h.handleConnect(w, req)
}

// handleConnect handles a CONNECT request by tunneling through the remote proxy.
func (h *proxyHandler) handleConnect(w http.ResponseWriter, req *http.Request) {
	target := req.Host
	if target == "" {
		http.Error(w, "Bad Request: no target specified", http.StatusBadRequest)
		return
	}

	if h.verbose {
		log.Printf("CONNECT request to %s from %s", target, req.RemoteAddr)
	}

	// Get current dialer
	h.dialerMu.RLock()
	dialer := h.dialer
	h.dialerMu.RUnlock()

	// Dial through remote proxy
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	proxyConn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		log.Printf("Failed to dial %s: %v", target, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer proxyConn.Close()

	// Hijack client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Internal Server Error: hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("Hijack failed: %v", err)
		return
	}
	defer clientConn.Close()

	// Send success response
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Printf("Failed to send response: %v", err)
		return
	}

	if h.verbose {
		log.Printf("Connected to %s", target)
	}

	// Bidirectional copy
	copyBidirectional(clientConn, proxyConn)

	if h.verbose {
		log.Printf("Connection to %s closed", target)
	}
}

// copyBidirectional copies data bidirectionally between two connections.
func copyBidirectional(client, server net.Conn) {
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(server, client)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(client, server)
		done <- struct{}{}
	}()

	// Wait for first direction to finish
	<-done
}

// createDialer creates a CONNECT dialer from the flags.
func createDialer(tsTokenSource oauth2.TokenSource) (connecttunnel.Dialer, error) {
	// Build client config
	clientCfg := &connecttunnel.ClientConfig{
		ProxyURL: *proxyURL,
		HeadersForRequest: func(req *http.Request) (http.Header, error) {
			if tsTokenSource != nil {
				token, err := tsTokenSource.Token()
				if err != nil {
					return nil, fmt.Errorf("failed to get token: %w", err)
				}
				idToken, ok := token.Extra("id_token").(string)
				if !ok {
					return nil, fmt.Errorf("no id_token in response")
				}
				return http.Header{
					"Authorization": []string{"Bearer " + idToken},
				}, nil
			}
			if *proxyAuth != "" {
				return http.Header{
					"Proxy-Authorization": []string{*proxyAuth},
				}, nil
			}
			return nil, nil
		},
	}

	// Configure TLS if needed
	if *insecure {
		log.Println("Warning: TLS verification disabled")
		clientCfg.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return connecttunnel.NewH2Dialer(clientCfg), nil
}

// createTokenSource creates an OAuth2 token source for OIDC authentication.
func createTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	// Discover the OIDC provider
	p, err := provider.DiscoverOIDCProvider(ctx, *oidcIssuer)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	// Parse scopes
	scopes := strings.Split(*oidcScopes, ",")
	for i := range scopes {
		scopes[i] = strings.TrimSpace(scopes[i])
	}

	// Configure OAuth2 client
	oauth2Config := oauth2.Config{
		ClientID:     *oidcClientID,
		ClientSecret: *oidcClientSecret,
		Endpoint:     p.Endpoint(),
		Scopes:       scopes,
	}

	// Create CLI token source with automatic browser flow
	cliConfig := &clitoken.Config{
		OAuth2Config: oauth2Config,
	}

	clitsrc, err := cliConfig.TokenSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create token source: %w", err)
	}

	ccfg := tokencache.Config{
		Issuer: *oidcIssuer,
		CacheKey: tokencache.IDTokenCacheKey{
			ClientID: *oidcClientID,
			Scopes:   strings.Split(*oidcScopes, ","),
		}.Key(),
		WrappedSource: clitsrc,
		OAuth2Config:  &oauth2Config,
		Cache:         clitoken.BestCredentialCache(),
	}

	return ccfg.TokenSource(ctx)
}

// handleShutdown handles graceful shutdown on SIGINT/SIGTERM.
func handleShutdown(server *http.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\nShutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
