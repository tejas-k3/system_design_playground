package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// healthURL turns a listen address such as ":8090" or "0.0.0.0:8090" into the local health URL.
func healthURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://127.0.0.1:8090/api/healthz"
	}
	if host == "" || host == "0.0.0.0" || host == "::" || strings.HasPrefix(host, "[") {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/api/healthz"
}

// checkHealth fetches url and returns nil only for a 200 answer. It lets the binary act as its own
// container health probe, since a minimal image has no shell or curl.
func checkHealth(url string) error {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check answered %d", resp.StatusCode)
	}
	return nil
}
