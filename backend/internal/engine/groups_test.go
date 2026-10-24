package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeGroupsRoundTrip(t *testing.T) {
	in := Diagram{
		Nodes: []Node{{ID: "a", Kind: "client", X: 10, Y: 10}},
		Groups: []Group{
			{ID: "g1", Kind: "az", Label: "us-east-1a", X: 0, Y: 0, W: 300, H: 200},
			{ID: "g2", Kind: "bogus", W: 100, H: 100},
			{ID: "g3", Kind: "region", Label: "r", W: 5, H: 99999},
			{ID: "a", Kind: "vpc", W: 100, H: 100},
			{ID: "g5", Kind: "note", Label: strings.Repeat("é", 100), W: 100, H: 100},
		},
	}
	out, adj := Sanitize(in)
	if !adj {
		t.Fatal("expected adjustments")
	}
	if len(out.Groups) != 3 {
		t.Fatalf("want 3 groups (bad kind and duplicate id dropped), got %d: %+v", len(out.Groups), out.Groups)
	}
	if out.Groups[0] != in.Groups[0] {
		t.Fatalf("valid group changed: %+v", out.Groups[0])
	}
	if out.Groups[1].W != MinGroupSize || out.Groups[1].H != MaxGroupSize {
		t.Fatalf("sizes not clamped: %+v", out.Groups[1])
	}
	if len([]rune(out.Groups[2].Label)) != MaxGroupLabel {
		t.Fatalf("label not truncated")
	}
	again, adj2 := Sanitize(out)
	if adj2 {
		t.Fatal("sanitising a sanitised diagram must be a no-op")
	}
	b1, _ := json.Marshal(out)
	b2, _ := json.Marshal(again)
	if string(b1) != string(b2) {
		t.Fatal("not idempotent")
	}
}

func TestSanitizeGroupsCapAndOldDesigns(t *testing.T) {
	var gs []Group
	for i := 0; i < MaxGroups+5; i++ {
		gs = append(gs, Group{ID: "g" + strings.Repeat("x", i+1), Kind: "az", W: 100, H: 100})
	}
	out, adj := Sanitize(Diagram{Groups: gs})
	if !adj || len(out.Groups) != MaxGroups {
		t.Fatalf("cap: adj=%v n=%d", adj, len(out.Groups))
	}
	var old Diagram
	if err := json.Unmarshal([]byte(`{"nodes":[],"edges":[],"notes":[]}`), &old); err != nil {
		t.Fatal(err)
	}
	o, adj := Sanitize(old)
	if adj || len(o.Groups) != 0 {
		t.Fatalf("old design changed: %v %+v", adj, o)
	}
	b, _ := json.Marshal(o)
	if strings.Contains(string(b), "groups") {
		t.Fatalf("empty groups should be omitted: %s", b)
	}
}
