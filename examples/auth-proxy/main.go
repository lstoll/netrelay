// Package main demonstrates a CONNECT proxy with authentication.
package main

import (
	"context"
	"log"
	"net/http"
	"strings"

	"lds.li/funnelproxy/connecttunnel"
)

const (
	proxyUser     = "user"
	proxyPassword = "pass"
)

func main() {
	// Create handler with authentication
	handler := connecttunnel.NewHandler(&connecttunnel.ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			// Check Proxy-Authorization header
			auth := req.Header.Get("Proxy-Authorization")
			if !checkAuth(auth) {
				log.Printf("Authentication failed for %s", req.RemoteAddr)
				return connecttunnel.ErrTunnelRejected
			}

			log.Printf("Authenticated tunnel: %s -> %s", req.RemoteAddr, req.Host)
			return nil
		},
	})

	log.Println("Starting authenticated proxy server on :8080")
	log.Println("Use Proxy-Authorization: Basic dXNlcjpwYXNz")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

// checkAuth validates the Proxy-Authorization header.
// In production, use proper authentication libraries and secure credential storage.
func checkAuth(auth string) bool {
	// Basic authentication: "Basic dXNlcjpwYXNz" (base64 of "user:pass")
	const expected = "Basic dXNlcjpwYXNz"
	return strings.TrimSpace(auth) == expected
}
