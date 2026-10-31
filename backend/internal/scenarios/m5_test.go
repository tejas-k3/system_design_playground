package scenarios

import (
	"strings"
	"testing"

	"sysdesign/internal/engine"
)

func TestM5TypeaheadBriefUsesIndexedFieldLookups(t *testing.T) {
	sc := Get("typeahead-index")
	if sc == nil {
		t.Fatal("typeahead-index is not registered")
	}
	if !sc.ConfigOnly || sc.Level != "sde2" {
		t.Fatalf("typeahead challenge should be an SDE2 fix challenge: configOnly=%v level=%q", sc.ConfigOnly, sc.Level)
	}
	client, db := m5Node(sc.Diagram, "users"), m5Node(sc.Diagram, "db")
	if client == nil || m5Float(client, "secondaryPct") != 80.0 || db == nil || m5Float(db, "indexes") != 0 {
		t.Fatal("starting design must issue alternate-field reads without a secondary index")
	}
	if len(Alternatives(sc.ID)) < 2 {
		t.Fatal("typeahead needs at least two measured alternatives")
	}
	if !strings.Contains(sc.Story, "prefix") || !strings.Contains(sc.Debrief, "secondary index") {
		t.Fatal("brief must explain its prefix-to-secondary-index model")
	}
}

func TestM5CrawlerBriefUsesFrontierDeduplication(t *testing.T) {
	sc := Get("crawl-dedup")
	if sc == nil {
		t.Fatal("crawl-dedup is not registered")
	}
	if sc.ConfigOnly || sc.Level != "sde2" {
		t.Fatalf("crawler should be a build challenge at SDE2: configOnly=%v level=%q", sc.ConfigOnly, sc.Level)
	}
	client := m5Node(sc.Solution, "users")
	if client == nil || client.Params["workload"] != "crawl" {
		t.Fatal("crawler witness must use the dedicated crawl workload")
	}
	if len(Alternatives(sc.ID)) < 2 {
		t.Fatal("crawler needs at least two measured alternatives")
	}
}

func TestM5StartingDesignsFailAndSolutionsPassAcrossSeeds(t *testing.T) {
	for _, id := range []string{"typeahead-index", "crawl-dedup"} {
		sc := Get(id)
		for _, seed := range []uint32{1, 2, 3} {
			if got := engine.RunToEnd(sc.Diagram, sc.Options(seed), sc.Goals); got.Pass {
				t.Errorf("%s starting design passes at seed %d", id, seed)
			}
			if got := engine.RunToEnd(sc.Solution, sc.Options(seed), sc.Goals); !got.Pass {
				t.Errorf("%s witness fails at seed %d: %+v", id, seed, got.Checks)
			}
		}
	}
}

func m5Node(d engine.Diagram, id string) *engine.Node {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

func m5Float(n *engine.Node, key string) float64 {
	if n == nil {
		return 0
	}
	v, _ := n.Params[key].(float64)
	return v
}
