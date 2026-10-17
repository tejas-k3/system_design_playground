package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthURL(t *testing.T) {
	for addr, want := range map[string]string{
		":8090":          "http://127.0.0.1:8090/api/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/api/healthz",
		"localhost:8091": "http://localhost:8091/api/healthz",
		"garbage":        "http://127.0.0.1:8090/api/healthz",
	} {
		if got := healthURL(addr); got != want {
			t.Errorf("healthURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestCheckHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	if err := checkHealth(ok.URL); err != nil {
		t.Fatalf("a 200 answer is healthy: %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer bad.Close()
	if err := checkHealth(bad.URL); err == nil {
		t.Fatal("a 503 answer must be unhealthy")
	}
	if err := checkHealth("http://127.0.0.1:1/api/healthz"); err == nil {
		t.Fatal("a refused connection must be unhealthy")
	}
}
