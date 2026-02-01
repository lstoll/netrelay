// Package main demonstrates chaining multiple proxies together.
package main

import (
	"context"
	"fmt"
	"io"
	"log"

	"lds.li/funnelproxy/connecttunnel"
)

func main() {
	// Create first proxy dialer
	proxy1 := connecttunnel.NewH1Dialer(&connecttunnel.ClientConfig{
		ProxyURL: "http://localhost:8080",
	})

	// Create second proxy dialer that uses the first proxy to connect
	proxy2 := connecttunnel.NewH1Dialer(&connecttunnel.ClientConfig{
		ProxyURL:    "http://localhost:8081",
		DialContext: proxy1.DialContext, // Use proxy1 to connect to proxy2
	})

	// Connect to final target through both proxies
	ctx := context.Background()
	conn, err := proxy2.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		log.Fatalf("Failed to connect through chained proxies: %v", err)
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

	fmt.Printf("Response (via 2 proxies):\n%s\n", response)
}
