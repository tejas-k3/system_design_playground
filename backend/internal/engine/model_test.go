package engine

import (
	"reflect"
	"testing"
)

func modelDesign(model string, secondaryPct float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 60.0, "readPct": 50.0, "secondaryPct": secondaryPct, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("db", KDatabase, Params{"connections": 32.0, "model": model})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func modelMean(d Diagram) float64 { return run(d, Options{Seed: 6}, 60).RunSnapshot().Mean }

func modelWrites(model string) float64 {
	d := modelDesign(model, 0)
	for i := range d.Nodes {
		if d.Nodes[i].Kind == KClient {
			d.Nodes[i].Params["readPct"] = 0.0
		}
	}
	return modelMean(d)
}

// A preset is a coarse summary of what a data model is good at: it changes the cost of the same requests.
func TestAppendFriendlyModelsMakeWritesCheaper(t *testing.T) {
	rel, doc, wide, ts := modelWrites("relational"), modelWrites("document"), modelWrites("wideColumn"), modelWrites("timeSeries")
	t.Logf("write-only mean latency: relational %.1f document %.1f wide-column %.1f time-series %.1f", rel, doc, wide, ts)
	if !(ts < wide && wide < doc && doc < rel) {
		t.Fatalf("expected relational > document > wide-column > time-series for writes: %.1f %.1f %.1f %.1f", rel, doc, wide, ts)
	}
}

// Both scan on a lookup by another field, but a wide-column store has no secondary access path: its scan is 3x as big.
func TestWideColumnLookupsByAnotherFieldCostThreeTimesAsMuch(t *testing.T) {
	extra := func(model string) float64 {
		return modelMean(modelDesign(model, 20)) - modelMean(modelDesign(model, 0))
	}
	rel, wide := extra("relational"), extra("wideColumn")
	t.Logf("extra mean latency from 20%% lookups by another field: relational +%.1f, wide-column +%.1f", rel, wide)
	if ratio := wide / rel; ratio < 2.5 || ratio > 3.5 {
		t.Fatalf("expected about 3x the extra cost, got %.2fx (+%.1f vs +%.1f)", ratio, wide, rel)
	}
}

func TestADocumentStoreAnswersKeyLookupsFaster(t *testing.T) {
	rel, doc := modelMean(modelDesign("relational", 0)), modelMean(modelDesign("document", 0))
	if doc >= rel {
		t.Fatalf("single-key operations should be cheaper in a document store: %.1f vs %.1f", doc, rel)
	}
}

func TestRelationalIsTheDefaultAndChangesNothing(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 60.0, "readPct": 50.0, "secondaryPct": 20.0, "timeoutMs": 5000.0}).node("a", KAppServer, Params{"threads": 32.0}).node("db", KDatabase, Params{"connections": 32.0}).edge("u", "a").edge("a", "db")
	d, _ := Sanitize(plain.d)
	if !reflect.DeepEqual(run(modelDesign("relational", 20), Options{Seed: 6}, 20).RunSnapshot(), run(d, Options{Seed: 6}, 20).RunSnapshot()) {
		t.Fatal("model relational must equal a design that never mentions it")
	}
}
