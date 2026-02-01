// Package main demonstrates an HTTP/2 cleartext (h2c) CONNECT proxy client.
package main

import (
	"context"
	"fmt"
	"io"
	"log"

	"lds.li/funnelproxy/connecttunnel"
)

func main() {
	// Create an h2c dialer
	dialer := connecttunnel.NewH2CDialer(&connecttunnel.ClientConfig{
		ProxyURL: "http://localhost:8080",
	})

	// Connect to a target through the proxy
	ctx := context.Background()
	conn, err := dialer.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		log.Fatalf("Failed to connect through proxy: %v", err)
	}
	defer conn.Close()

	// Send HTTP request through the tunnel
	request := "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		log.Fatalf("Failed to write request: %v", err)
	}

	// Read response
	response, err := io.ReadAll(conn)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	fmt.Printf("Response:\n%s\n", response)
}
