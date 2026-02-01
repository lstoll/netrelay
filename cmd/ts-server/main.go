// Package main implements a Tailscale Funnel-enabled CONNECT proxy server.
//
// This server uses Tailscale as a library (tsnet) to create a listener that
// supports Tailscale Funnel, allowing external internet access to the proxy.
// It handles both HTTP/1.1 and HTTP/2 cleartext (h2c) since Tailscale
// terminates TLS.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tink-crypto/tink-go/v2/jwt"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"lds.li/funnelproxy/connecttunnel"
	"lds.li/oauth2ext/provider"
	"tailscale.com/tsnet"
)

var (
	hostname   = flag.String("hostname", "", "Tailscale hostname (default: generates one)")
	port       = flag.String("port", "443", "Port to listen on (default: 443 for Funnel)")
	authKey    = flag.String("authkey", "", "Tailscale auth key (optional, uses existing auth if not provided)")
	stateDir   = flag.String("statedir", "", "Directory to store Tailscale state (default: .tsnet-state)")

	// Authentication options
	enableAuth = flag.Bool("auth", false, "Enable simple bearer token authentication")
	authToken  = flag.String("auth-token", "", "Authentication token (required if -auth is set)")

	// OIDC authentication options
	oidcIssuer   = flag.String("oidc-issuer", "", "OIDC issuer URL (e.g., https://accounts.google.com)")
	oidcAudience = flag.String("oidc-audience", "", "OIDC audience/client ID (required if -oidc-issuer is set)")

	verbose = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	// Validate authentication flags
	if *enableAuth && *authToken == "" {
		log.Fatal("Error: -auth-token is required when -auth is set")
	}

	if *oidcIssuer != "" && *oidcAudience == "" {
		log.Fatal("Error: -oidc-audience is required when -oidc-issuer is set")
	}

	if *enableAuth && *oidcIssuer != "" {
		log.Fatal("Error: cannot use both -auth and -oidc-issuer (choose one authentication method)")
	}

	// Initialize OIDC provider if configured
	var oidcProvider *provider.Provider
	if *oidcIssuer != "" {
		log.Printf("Initializing OIDC provider: %s", *oidcIssuer)
		var err error
		oidcProvider, err = provider.DiscoverOIDCProvider(context.Background(), *oidcIssuer)
		if err != nil {
			log.Fatalf("Failed to initialize OIDC provider: %v", err)
		}
		log.Printf("✓ OIDC provider initialized (issuer: %s)", oidcProvider.Issuer())
	}

	// Create Tailscale server
	srv := &tsnet.Server{
		Hostname: *hostname,
		Logf:     log.Printf,
	}

	if *authKey != "" {
		srv.AuthKey = *authKey
	}

	if *stateDir != "" {
		srv.Dir = *stateDir
	}

	// Start Tailscale
	log.Println("Starting Tailscale...")
	defer srv.Close()

	// Get local client for Funnel configuration
	lc, err := srv.LocalClient()
	if err != nil {
		log.Fatalf("Failed to get local client: %v", err)
	}

	// Wait for Tailscale to be ready
	ctx := context.Background()
	status, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		log.Fatalf("Failed to get Tailscale status: %v", err)
	}

	log.Printf("Tailscale node: %s", status.Self.DNSName)
	log.Printf("Tailscale addresses: %v", status.Self.TailscaleIPs)

	// Create the CONNECT proxy handler
	proxyHandler := connecttunnel.NewHandler(&connecttunnel.ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			// Extract target for logging
			target := req.Host
			if target == "" {
				target = req.RequestURI
			}

			// OIDC authentication
			if oidcProvider != nil {
				token, err := extractBearerToken(req)
				if err != nil {
					if *verbose {
						log.Printf("No valid bearer token from %s: %v", req.RemoteAddr, err)
					}
					return connecttunnel.ErrTunnelRejected
				}

				// Create validator with audience
				issuer := oidcProvider.Issuer()
				validator, err := jwt.NewValidator(&jwt.ValidatorOpts{
					ExpectedAudience: oidcAudience,
					ExpectedIssuer:   &issuer,
				})
				if err != nil {
					log.Printf("Failed to create validator: %v", err)
					return connecttunnel.ErrTunnelRejected
				}

				// Verify and decode the token
				verifiedJWT, err := oidcProvider.VerifyAndDecodeContext(ctx, token, validator)
				if err != nil {
					if *verbose {
						log.Printf("Token verification failed from %s: %v", req.RemoteAddr, err)
					}
					return connecttunnel.ErrTunnelRejected
				}

				// Extract subject and email for logging
				subject, _ := verifiedJWT.Subject()

				// Try to get email from custom claims
				email, _ := verifiedJWT.StringClaim("email")

				// Log authenticated tunnel with user identity
				user := email
				if user == "" {
					user = subject
				}
				log.Printf("Tunnel: %s (%s) -> %s (proto: %s)", req.RemoteAddr, user, target, req.Proto)
				return nil
			}

			// Simple bearer token authentication
			if *enableAuth {
				token := req.Header.Get("Proxy-Authorization")
				expectedToken := "Bearer " + *authToken
				if token != expectedToken {
					if *verbose {
						log.Printf("Authentication failed from %s (got: %q)", req.RemoteAddr, token)
					}
					return connecttunnel.ErrTunnelRejected
				}
			}

			// Log tunnel requests (unauthenticated or simple auth)
			log.Printf("Tunnel: %s -> %s (proto: %s)", req.RemoteAddr, target, req.Proto)
			return nil
		},
		ErrorLog: log.Default(),
	})

	// Wrap with h2c handler for HTTP/2 cleartext support
	// This is necessary because Tailscale Funnel terminates TLS
	h2s := &http2.Server{}
	handler := h2c.NewHandler(proxyHandler, h2s)

	// Create HTTP server
	httpServer := &http.Server{
		Handler: handler,
		ErrorLog: log.Default(),
	}

	// Listen on Tailscale with Funnel
	listener, err := srv.ListenFunnel("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("Failed to listen with Funnel on port %s: %v", *port, err)
	}
	defer listener.Close()

	// Get the Funnel URL
	funnelURL := "https://" + status.Self.DNSName + ":" + *port
	log.Printf("✓ Tailscale Funnel enabled")
	log.Printf("✓ CONNECT proxy listening on: %s", funnelURL)
	log.Printf("✓ Supports: HTTP/1.1 CONNECT and HTTP/2 CONNECT (h2c)")

	if oidcProvider != nil {
		log.Printf("✓ Authentication: OIDC enabled (issuer: %s, audience: %s)", *oidcIssuer, *oidcAudience)
	} else if *enableAuth {
		log.Printf("✓ Authentication: bearer token enabled (use Proxy-Authorization: Bearer %s)", *authToken)
	} else {
		log.Println("⚠ Authentication: disabled (use -auth or -oidc-issuer to enable)")
	}

	log.Println("✓ Server ready - press Ctrl+C to stop")

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("\nShutting down gracefully...")
		httpServer.Close()
	}()

	// Start serving
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}

// extractBearerToken extracts a bearer token from Authorization or Proxy-Authorization headers.
func extractBearerToken(req *http.Request) (string, error) {
	// Try Proxy-Authorization first (standard for CONNECT)
	auth := req.Header.Get("Proxy-Authorization")
	if auth == "" {
		// Fall back to Authorization
		auth = req.Header.Get("Authorization")
	}

	if auth == "" {
		return "", fmt.Errorf("no authorization header")
	}

	// Extract bearer token
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", fmt.Errorf("not a bearer token")
	}

	return strings.TrimPrefix(auth, prefix), nil
}
