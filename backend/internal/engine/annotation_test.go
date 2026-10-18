package engine

import (
	"reflect"
	"testing"
)

func annotated(protocol string, async bool) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 50.0}).node("a", KAppServer, nil).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "db")
	b.d.Edges[1].Protocol, b.d.Edges[1].Async = protocol, async
	return b.d
}

func TestEdgeProtocolAndAsyncSurviveSanitising(t *testing.T) {
	d, adjusted := Sanitize(annotated("grpc", true))
	if adjusted || d.Edges[1].Protocol != "grpc" || !d.Edges[1].Async {
		t.Fatalf("a known protocol and the async flag are kept: %+v adjusted=%v", d.Edges[1], adjusted)
	}
}

func TestUnknownProtocolIsDropped(t *testing.T) {
	d, adjusted := Sanitize(annotated("carrier-pigeon", false))
	if !adjusted || d.Edges[1].Protocol != "" {
		t.Fatalf("an unknown protocol is removed and reported: %+v adjusted=%v", d.Edges[1], adjusted)
	}
}

// The labels are notes for the reader: they must not change what the simulation does.
func TestAnnotationsDoNotChangeTheSimulation(t *testing.T) {
	plain, _ := Sanitize(annotated("", false))
	marked, _ := Sanitize(annotated("sql", true))
	if !reflect.DeepEqual(run(plain, Options{Seed: 5}, 20).RunSnapshot(), run(marked, Options{Seed: 5}, 20).RunSnapshot()) {
		t.Fatal("protocol and async are annotations only")
	}
}
