package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"sysdesign/internal/designs"
	"sysdesign/internal/scenarios"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureLog redirects the standard logger for the test and returns what was written.
func captureLog(t *testing.T) *syncBuf {
	t.Helper()
	buf := &syncBuf{}
	oldW := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(oldW) })
	return buf
}

func doReq(t *testing.T, method, url string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestRequestLogNeverContainsADesignID(t *testing.T) {
	logs := captureLog(t)
	st, err := designs.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(Config{Designs: st}).Handler())
	t.Cleanup(ts.Close)

	var saved struct {
		ID string `json:"id"`
	}
	if code := post(t, ts.URL+"/api/designs", map[string]any{"diagram": scenarios.Get("hello-world").Solution}, &saved); code != 201 || saved.ID == "" {
		t.Fatalf("save: %d %q", code, saved.ID)
	}
	if code := doReq(t, "GET", ts.URL+"/api/designs/"+saved.ID); code != 200 {
		t.Fatalf("load: %d", code)
	}
	out := logs.String()
	if strings.Contains(out, saved.ID) {
		t.Fatalf("log leaks the design id:\n%s", out)
	}
	for _, want := range []string{"GET /api/designs/{id} 200 ", "POST /api/designs 201 "} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}

	if !regexp.MustCompile(`GET /api/designs/\{id\} 200 [0-9.]+(ms|s|µs)\n`).MatchString(out) {
		t.Errorf("log line has no duration:\n%s", out)
	}

	unknown := "0123456789abcdef0123456789abcdef"
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/designs/" + unknown},
		{"GET", "/api/designs/" + saved.ID + "/extra"},
		{"DELETE", "/api/designs/" + saved.ID},
		{"PUT", "/api/designs/" + unknown},
	} {
		doReq(t, c.method, ts.URL+c.path)
	}
	out = logs.String()
	for _, id := range []string{saved.ID, unknown} {
		if strings.Contains(out, id) {
			t.Fatalf("log leaks id %s:\n%s", id, out)
		}
	}
	if !strings.Contains(out, " 404 ") || !strings.Contains(out, " 405 ") {
		t.Errorf("expected 404 and 405 lines:\n%s", out)
	}
}

func TestSaveFailureLogsNoDesignID(t *testing.T) {
	logs := captureLog(t)
	dir := filepath.Join(t.TempDir(), "designs")
	st, err := designs.New(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(Config{Designs: st}).Handler())
	t.Cleanup(ts.Close)
	b, _ := json.Marshal(map[string]any{"diagram": scenarios.Get("hello-world").Solution})
	resp, err := http.Post(ts.URL+"/api/designs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d, want 500", resp.StatusCode)
	}
	out := logs.String()
	if !strings.Contains(out, "save design") {
		t.Fatalf("failure not logged:\n%s", out)
	}
	if strings.Contains(out, ".tmp") || strings.Contains(out, ".json") || strings.Contains(out, dir) {
		t.Fatalf("save failure log carries a path (and so the id):\n%s", out)
	}
}
