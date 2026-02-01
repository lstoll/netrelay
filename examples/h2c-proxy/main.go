// Package main demonstrates an HTTP/2 cleartext (h2c) CONNECT proxy server.
package main

import (
	"context"
	"log"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"lds.li/funnelproxy/connecttunnel"
)

func main() {
	// Create tunnel handler
	handler := connecttunnel.NewHandler(&connecttunnel.ServerConfig{
		OnTunnel: func(ctx context.Context, req *http.Request) error {
			log.Printf("h2c Tunnel requested: %s -> %s", req.RemoteAddr, req.Host)
			return nil
		},
	})

	// Wrap with h2c handler
	h2s := &http2.Server{}
	h1s := &http.Server{
		Addr:    ":8080",
		Handler: h2c.NewHandler(handler, h2s),
	}

	log.Println("Starting h2c proxy server on :8080")
	log.Fatal(h1s.ListenAndServe())
}
