// Package main implements an SSH ProxyCommand that forwards connections through a CONNECT proxy.
//
// This command is designed to be used with SSH's ProxyCommand option:
//   ssh -o ProxyCommand="ssh-proxy -proxy https://proxy.example.com:443 %h %p" user@target
//
// It reads from stdin and writes to stdout, forwarding the SSH protocol through
// the CONNECT proxy to the target SSH server.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"lds.li/funnelproxy/connecttunnel"
	"lds.li/oauth2ext/clitoken"
	"lds.li/oauth2ext/provider"
)

var (
	proxyURL   = flag.String("proxy", "", "CONNECT proxy URL (required, e.g., https://proxy.example.com:443)")
	proxyType  = flag.String("type", "", "Proxy type: h1, h2, or h2c (default: auto-detect from URL)")
	proxyAuth  = flag.String("auth", "", "Proxy authentication (e.g., 'Bearer token' or 'Basic base64')")
	insecure   = flag.Bool("insecure", false, "Skip TLS verification")
	timeout    = flag.Duration("timeout", 30*time.Second, "Connection timeout")
	verbose    = flag.Bool("verbose", false, "Enable verbose logging (written to stderr)")
	bufferSize = flag.Int("buffer", 32*1024, "I/O buffer size in bytes")

	// OIDC authentication flags
	oidcIssuer   = flag.String("oidc-issuer", "", "OIDC issuer URL for automatic token acquisition")
	oidcClientID = flag.String("oidc-client-id", "", "OIDC client ID (required if -oidc-issuer is set)")
	oidcScopes   = flag.String("oidc-scopes", "openid", "OIDC scopes (comma-separated, default: openid)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <target-host> <target-port>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "SSH ProxyCommand for CONNECT proxies.\n\n")
		fmt.Fprintf(os.Stderr, "Example usage in SSH:\n")
		fmt.Fprintf(os.Stderr, "  ssh -o ProxyCommand='%s -proxy https://proxy.example.com:443 %%h %%p' user@target\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Or in ~/.ssh/config:\n")
		fmt.Fprintf(os.Stderr, "  Host *\n")
		fmt.Fprintf(os.Stderr, "    ProxyCommand %s -proxy https://proxy.example.com:443 %%h %%p\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Disable log output to stdout (SSH data goes there)
	if !*verbose {
		log.SetOutput(io.Discard)
	} else {
		// Verbose logs go to stderr
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Validate arguments
	if *proxyURL == "" {
		fmt.Fprintf(os.Stderr, "Error: -proxy is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if flag.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Error: target host and port are required\n\n")
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
			fmt.Fprintf(os.Stderr, "Error acquiring OIDC token: %v\n", err)
			os.Exit(1)
		}
		*proxyAuth = "Bearer " + token
		log.Println("✓ OIDC token acquired")
	}

	targetHost := flag.Arg(0)
	targetPort := flag.Arg(1)
	target := net.JoinHostPort(targetHost, targetPort)

	log.Printf("Connecting to %s via %s", target, *proxyURL)

	// Create dialer
	dialer, err := createDialer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dialer: %v\n", err)
		os.Exit(1)
	}

	// Connect through proxy
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", target, err)
		os.Exit(1)
	}
	defer conn.Close()

	log.Printf("Connected to %s", target)

	// Bidirectional copy between stdin/stdout and connection
	errCh := make(chan error, 2)

	// stdin -> connection
	go func() {
		buf := make([]byte, *bufferSize)
		_, err := io.CopyBuffer(conn, os.Stdin, buf)
		if err != nil {
			log.Printf("stdin->conn error: %v", err)
		}
		// Close write side when stdin closes
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
		errCh <- err
	}()

	// connection -> stdout
	go func() {
		buf := make([]byte, *bufferSize)
		_, err := io.CopyBuffer(os.Stdout, conn, buf)
		if err != nil {
			log.Printf("conn->stdout error: %v", err)
		}
		errCh <- err
	}()

	// Wait for either direction to finish
	err = <-errCh
	if err != nil && err != io.EOF {
		log.Printf("Connection error: %v", err)
		os.Exit(1)
	}

	log.Printf("Connection closed")
}

// createDialer creates a CONNECT dialer from the flags.
func createDialer() (connecttunnel.Dialer, error) {
	// Build client config
	clientCfg := &connecttunnel.ClientConfig{
		ProxyURL: *proxyURL,
	}

	// Add authentication header if provided
	if *proxyAuth != "" {
		clientCfg.Header = make(map[string][]string)
		clientCfg.Header.Set("Proxy-Authorization", *proxyAuth)
	}

	// Configure TLS if needed
	if *insecure {
		log.Println("Warning: TLS verification disabled")
		// clientCfg.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	// Determine dialer type
	dialerType := *proxyType
	if dialerType == "" {
		// Auto-detect from URL
		if len(*proxyURL) > 5 && (*proxyURL)[:5] == "https" {
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
