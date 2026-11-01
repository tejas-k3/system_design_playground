package engine

import (
	"math"
	"strconv"
	"strings"
	"unicode"
)

// The limits Sanitize clamps a connection to.
const (
	MinLatencyMs     = 1
	MaxLatencyMs     = 200
	maxBandwidthMbps = 100000.0
	maxJitterMs      = 1000.0
)

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func clamp(v, lo, hi float64) float64 { return min(hi, max(lo, v)) }

// sanitizeParams keeps only known keys with valid values, clamping numbers into range.
// It reports whether anything had to be changed.
func sanitizeParams(kind string, in Params) (Params, bool) {
	out := Params{}
	adjusted := false
	spec := Spec(kind)
	if spec == nil {
		return out, in != nil
	}
	for k, raw := range in {
		var ps *ParamSpec
		for i := range spec.Params {
			if spec.Params[i].Key == k {
				ps = &spec.Params[i]
			}
		}
		if ps == nil {
			adjusted = true
			continue
		}
		switch ps.Type {
		case "number":
			var f float64
			ok := true
			switch v := raw.(type) {
			case float64:
				f = v
			case int:
				f = float64(v)
			case string:
				p, err := strconv.ParseFloat(v, 64)
				f, ok = p, err == nil
			default:
				ok = false
			}
			if !ok || !finite(f) {
				adjusted = true
				continue
			}
			c := clamp(f, ps.Min, ps.Max)
			if c != f {
				adjusted = true
			}
			out[k] = c
		case "select":
			s, _ := raw.(string)
			valid := false
			for _, o := range ps.Options {
				if o.Value == s {
					valid = true
				}
			}
			if !valid {
				adjusted = true
				continue
			}
			out[k] = s
		case "boolean":
			b, ok := raw.(bool)
			if !ok {
				adjusted = true
				continue
			}
			out[k] = b
		}
	}
	return out, adjusted
}

var knownProtocol = map[string]bool{"http": true, "grpc": true, "tcp": true, "ws": true, "sql": true, "custom": true}

// Sanitize makes a diagram safe to run: unknown kinds, duplicate ids and illegal edges are dropped, and
// parameters and latencies are clamped. It reports whether anything had to be changed.
func Sanitize(in Diagram) (Diagram, bool) {
	adjusted := false
	seen := map[string]bool{}
	claim := func(id string) bool {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	out := Diagram{Nodes: []Node{}, Edges: []Edge{}, Notes: []Note{}}
	var nameAdj bool
	out.Name, nameAdj = sanitizeName(in.Name)
	adjusted = adjusted || nameAdj
	kinds := map[string]string{}
	for _, n := range in.Nodes {
		spec := Spec(n.Kind)
		if spec == nil || !finite(n.X) || !finite(n.Y) || !claim(n.ID) {
			adjusted = true
			continue
		}
		p, adj := sanitizeParams(n.Kind, n.Params)
		if adj {
			adjusted = true
		}
		label := n.Label
		if label == "" {
			label = spec.Name
		}
		out.Nodes = append(out.Nodes, Node{ID: n.ID, Kind: n.Kind, X: n.X, Y: n.Y, Label: label, Params: p, Locked: n.Locked})
		kinds[n.ID] = n.Kind
	}
	pairs := map[string]bool{}
	for _, e := range in.Edges {
		fk, ok1 := kinds[e.From]
		tk, ok2 := kinds[e.To]
		key := e.From + "\n" + e.To
		if !ok1 || !ok2 || e.From == e.To || !CanConnect(fk, tk) || pairs[key] || !claim(e.ID) {
			adjusted = true
			continue
		}
		pairs[key] = true
		lat := e.LatencyMs
		if !finite(lat) || lat == 0 {
			lat = DefaultLatency(fk, tk)
			adjusted = true
		}
		if c := clamp(lat, MinLatencyMs, MaxLatencyMs); c != lat {
			lat = c
			adjusted = true
		}
		bw := e.BandwidthMbps
		if !finite(bw) || bw < 0 {
			bw = 0
			adjusted = true
		} else if bw > maxBandwidthMbps {
			bw = maxBandwidthMbps
			adjusted = true
		}
		loss, jit := e.LossPct, e.JitterMs
		if !finite(loss) || loss < 0 {
			loss, adjusted = 0, true
		} else if loss > 100 {
			loss, adjusted = 100, true
		}
		if !finite(jit) || jit < 0 {
			jit, adjusted = 0, true
		} else if jit > maxJitterMs {
			jit, adjusted = maxJitterMs, true
		}
		proto := e.Protocol
		if proto != "" && !knownProtocol[proto] {
			proto, adjusted = "", true
		}
		out.Edges = append(out.Edges, Edge{ID: e.ID, From: e.From, To: e.To, LatencyMs: lat, BandwidthMbps: bw, LossPct: loss, JitterMs: jit, Protocol: proto, Async: e.Async})
	}
	for _, n := range in.Notes {
		if !finite(n.X) || !finite(n.Y) || !claim(n.ID) {
			adjusted = true
			continue
		}
		fs := n.FontSize
		if !finite(fs) || fs <= 0 {
			fs = 0
		}
		out.Notes = append(out.Notes, Note{ID: n.ID, X: n.X, Y: n.Y, Text: n.Text, FontSize: fs})
	}
	for _, g := range in.Groups {
		if len(out.Groups) >= maxGroups {
			adjusted = true
			break
		}
		if !knownGroupKind[g.Kind] || !finite(g.X) || !finite(g.Y) || !finite(g.W) || !finite(g.H) || !claim(g.ID) {
			adjusted = true
			continue
		}
		w, h := clamp(g.W, minGroupSize, maxGroupSize), clamp(g.H, minGroupSize, maxGroupSize)
		if w != g.W || h != g.H {
			adjusted = true
		}
		label := g.Label
		if r := []rune(label); len(r) > maxGroupLabel {
			label, adjusted = string(r[:maxGroupLabel]), true
		}
		out.Groups = append(out.Groups, Group{ID: g.ID, Kind: g.Kind, Label: label, X: g.X, Y: g.Y, W: w, H: h, Collapsed: g.Collapsed})
	}
	for _, sh := range in.Shapes {
		if len(out.Shapes) >= maxShapes {
			adjusted = true
			break
		}
		if !knownShapeKind[sh.Kind] || !finite(sh.X) || !finite(sh.Y) || !finite(sh.W) || !finite(sh.H) || !claim(sh.ID) {
			adjusted = true
			continue
		}
		var w, h float64
		if sh.Kind == "arrow" {
			w, h = clamp(sh.W, -maxShapeSize, maxShapeSize), clamp(sh.H, -maxShapeSize, maxShapeSize)
			if math.Hypot(w, h) < 1 { // an arrow needs a visible length
				w = minShapeSize
			}
		} else {
			w, h = clamp(sh.W, minShapeSize, maxShapeSize), clamp(sh.H, minShapeSize, maxShapeSize)
		}
		if w != sh.W || h != sh.H {
			adjusted = true
		}
		label := sh.Label
		if r := []rune(label); len(r) > maxShapeLabel {
			label, adjusted = string(r[:maxShapeLabel]), true
		}
		out.Shapes = append(out.Shapes, Shape{ID: sh.ID, Kind: sh.Kind, Label: label, X: sh.X, Y: sh.Y, W: w, H: h})
	}
	return out, adjusted
}

// maxDiagramName is the longest design title kept, in characters.
const maxDiagramName = 60

// sanitizeName cleans a design title: control characters removed, trimmed, cut to MaxDiagramName characters, trimmed again.
func sanitizeName(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	clean = strings.TrimSpace(clean) // leading blanks must not eat into the length limit
	if r := []rune(clean); len(r) > maxDiagramName {
		clean = string(r[:maxDiagramName])
	}
	clean = strings.TrimSpace(clean)
	return clean, clean != s
}

// Limits on the visual annotation shapes (ellipses and arrows).
const (
	maxShapes     = 40
	minShapeSize  = 20
	maxShapeSize  = 4000
	maxShapeLabel = 60
)

var knownShapeKind = map[string]bool{"ellipse": true, "arrow": true}

// Limits on the visual group boxes.
const (
	maxGroups     = 30
	minGroupSize  = 60
	maxGroupSize  = 4000
	maxGroupLabel = 60
)

var knownGroupKind = map[string]bool{"az": true, "region": true, "vpc": true, "service": true, "note": true}
