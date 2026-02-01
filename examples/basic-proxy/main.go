// Package main demonstrates a basic HTTP/1.1 and HTTP/2 CONNECT proxy server.
package main

import (
	"context"
	"log"
	"net/http"

	"lds.li/funnelproxy/connecttunnel"
)

func main() {
	// Create a unified handler that supports both HTTP/1.1 and HTTP/2
	handler := connecttunnel.NewHandler(&connecttunnel.ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			// Log tunnel requests
			log.Printf("Tunnel requested: %s -> %s", req.RemoteAddr, req.Host)

			// Optional: Add authentication, rate limiting, or filtering here
			// Return an error to reject the tunnel

			return nil
		},
	})

	// Start HTTP server
	log.Println("Starting proxy server on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
