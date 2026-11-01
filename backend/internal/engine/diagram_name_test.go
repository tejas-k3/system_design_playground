package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeDiagramName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		adj  bool
	}{
		{"plain", "Checkout service", "Checkout service", false},
		{"trimmed", "  Checkout  ", "Checkout", true},
		{"control characters removed", "a\x00b\tc\nd\x7f", "abcd", true},
		{"blank becomes empty", " \t ", "", true},
		{"long is cut to the limit", strings.Repeat("é", maxDiagramName+10), strings.Repeat("é", maxDiagramName), true},
		{"leading blanks do not count against the limit", "   " + strings.Repeat("a", maxDiagramName), strings.Repeat("a", maxDiagramName), true},
		{"cut then trimmed", strings.Repeat("a", maxDiagramName-1) + "   b", strings.Repeat("a", maxDiagramName-1), true},
	}
	for _, c := range cases {
		out, adj := Sanitize(Diagram{Name: c.in})
		if out.Name != c.want || adj != c.adj {
			t.Errorf("%s: got %q adjusted=%v, want %q adjusted=%v", c.name, out.Name, adj, c.want, c.adj)
		}
		again, adj2 := Sanitize(out)
		if adj2 || again.Name != out.Name {
			t.Errorf("%s: sanitising twice changed the result", c.name)
		}
	}
}

func TestDiagramNameRoundTripAndOldDesigns(t *testing.T) {
	out, adj := Sanitize(Diagram{Name: "My design", Groups: []Group{{ID: "g", Kind: "az", W: 100, H: 100, Collapsed: true}}})
	if adj {
		t.Fatal("a valid name and a collapsed box must not be adjusted")
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var back Diagram
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != "My design" || len(back.Groups) != 1 || !back.Groups[0].Collapsed {
		t.Fatalf("name or collapsed flag lost: %s", b)
	}
	var old Diagram
	if err := json.Unmarshal([]byte(`{"nodes":[],"edges":[],"notes":[]}`), &old); err != nil {
		t.Fatal(err)
	}
	o, adj := Sanitize(old)
	if adj || o.Name != "" {
		t.Fatalf("old design changed: %v %+v", adj, o)
	}
	ob, _ := json.Marshal(o)
	if strings.Contains(string(ob), `"name"`) {
		t.Fatalf("an empty name should be omitted: %s", ob)
	}
}
