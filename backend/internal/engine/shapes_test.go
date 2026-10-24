package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeShapesRoundTrip(t *testing.T) {
	in := Diagram{
		Nodes: []Node{{ID: "a", Kind: "client", X: 10, Y: 10}},
		Shapes: []Shape{
			{ID: "s1", Kind: "ellipse", Label: "hot path", X: 0, Y: 0, W: 200, H: 120},
			{ID: "s2", Kind: "arrow", Label: "calls", X: 10, Y: 10, W: -150, H: 40},
			{ID: "s3", Kind: "star", W: 100, H: 100},
			{ID: "a", Kind: "ellipse", W: 100, H: 100},
			{ID: "s5", Kind: "ellipse", W: 2, H: 99999},
			{ID: "s6", Kind: "arrow", W: -99999, H: 99999},
			{ID: "s7", Kind: "arrow", W: 0, H: 0},
			{ID: "s8", Kind: "ellipse", Label: strings.Repeat("é", 100), W: 100, H: 100},
		},
	}
	out, adj := Sanitize(in)
	if !adj {
		t.Fatal("expected adjustments")
	}
	if len(out.Shapes) != 6 {
		t.Fatalf("want 6 shapes (unknown kind and duplicate id dropped), got %d: %+v", len(out.Shapes), out.Shapes)
	}
	if out.Shapes[0] != in.Shapes[0] || out.Shapes[1] != in.Shapes[1] {
		t.Fatalf("valid shapes changed: %+v", out.Shapes[:2])
	}
	if s := out.Shapes[2]; s.W != MinShapeSize || s.H != MaxShapeSize {
		t.Fatalf("ellipse size not clamped: %+v", s)
	}
	if s := out.Shapes[3]; s.W != -MaxShapeSize || s.H != MaxShapeSize {
		t.Fatalf("arrow extent not clamped: %+v", s)
	}
	if s := out.Shapes[4]; s.W == 0 && s.H == 0 {
		t.Fatalf("a zero-length arrow must get a visible length: %+v", s)
	}
	if len([]rune(out.Shapes[5].Label)) != MaxShapeLabel {
		t.Fatal("label not truncated")
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

func TestSanitizeShapesShareIDSpaceWithGroups(t *testing.T) {
	out, _ := Sanitize(Diagram{
		Groups: []Group{{ID: "x", Kind: "az", W: 100, H: 100}},
		Shapes: []Shape{{ID: "x", Kind: "ellipse", W: 100, H: 100}},
	})
	if len(out.Groups) != 1 || len(out.Shapes) != 0 {
		t.Fatalf("a shape may not reuse a group id: %+v", out)
	}
}

func TestSanitizeShapesCapAndOldDesigns(t *testing.T) {
	var ss []Shape
	for i := 0; i < MaxShapes+5; i++ {
		ss = append(ss, Shape{ID: "s" + strings.Repeat("x", i+1), Kind: "ellipse", W: 100, H: 100})
	}
	out, adj := Sanitize(Diagram{Shapes: ss})
	if !adj || len(out.Shapes) != MaxShapes {
		t.Fatalf("cap: adj=%v n=%d", adj, len(out.Shapes))
	}
	var old Diagram
	if err := json.Unmarshal([]byte(`{"nodes":[],"edges":[],"notes":[]}`), &old); err != nil {
		t.Fatal(err)
	}
	o, adj := Sanitize(old)
	if adj || len(o.Shapes) != 0 {
		t.Fatalf("old design changed: %v %+v", adj, o)
	}
	b, _ := json.Marshal(o)
	if strings.Contains(string(b), "shapes") {
		t.Fatalf("empty shapes should be omitted: %s", b)
	}
}

func TestShapesDoNotChangeTheSimulationInput(t *testing.T) {
	d := Diagram{Nodes: []Node{{ID: "a", Kind: "client", X: 1, Y: 2}}, Shapes: []Shape{{ID: "s", Kind: "arrow", W: 50, H: 50}}}
	with, _ := Sanitize(d)
	d.Shapes = nil
	without, _ := Sanitize(d)
	if len(with.Nodes) != len(without.Nodes) || len(with.Edges) != len(without.Edges) {
		t.Fatal("shapes changed the components")
	}
}
