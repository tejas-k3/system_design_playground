package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"sysdesign/internal/scenarios"
)

// The wire snapshot pins the JSON the browser depends on (frontend/src/api/types.ts mirrors it): for each
// representative response, the sorted set of key paths with their JSON types, and a hash of the full body.
// Regenerate with UPDATE_GOLDEN=1 only when a wire change is intended.

func wirePaths(prefix string, v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		out[prefix+" object"] = true
		for k, c := range x {
			wirePaths(prefix+"."+k, c, out)
		}
	case []any:
		out[prefix+" array"] = true
		for _, c := range x {
			wirePaths(prefix+"[]", c, out)
		}
	case string:
		out[prefix+" string"] = true
	case float64:
		out[prefix+" number"] = true
	case bool:
		out[prefix+" bool"] = true
	case nil:
		out[prefix+" null"] = true
	}
}

func wireSection(t *testing.T, name string, body []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	paths := map[string]bool{}
	wirePaths("$", v, paths)
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sum := sha256.Sum256(body)
	return fmt.Sprintf("## %s sha256=%s\n%s\n", name, hex.EncodeToString(sum[:8]), strings.Join(keys, "\n"))
}

func TestWireSnapshot(t *testing.T) {
	ts := newTestServer(t)
	get := func(path string) []byte {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		return b
	}
	sc := scenarios.Get("hello-world")
	runBody, _ := json.Marshal(map[string]any{"diagram": sc.Solution, "scenarioId": "hello-world"})
	resp, err := http.Post(ts.URL+"/api/run", "application/json", bytes.NewReader(runBody))
	if err != nil {
		t.Fatal(err)
	}
	run, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	got := strings.Join([]string{
		wireSection(t, "GET /api/registry", get("/api/registry")),
		wireSection(t, "GET /api/scenarios", get("/api/scenarios")),
		wireSection(t, "GET /api/scenarios/hello-world", get("/api/scenarios/hello-world")),
		wireSection(t, "GET /api/scenarios/hello-world/solution", get("/api/scenarios/hello-world/solution")),
		wireSection(t, "POST /api/run hello-world", run),
	}, "")
	const path = "testdata/wire.golden"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		shown := 0
		for i := 0; i < len(gl) || i < len(wl); i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Errorf("line %d:\n got  %s\n want %s", i+1, g, w)
				if shown++; shown >= 10 {
					break
				}
			}
		}
		t.Fatal("wire snapshot changed: the JSON contract with the frontend must not change in a refactor")
	}
}
