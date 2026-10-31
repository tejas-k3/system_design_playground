package scenarios

import (
	"testing"

	"sysdesign/internal/engine"
)

func TestM1OpenBriefsHaveSde2RequirementsAndWeakPoints(t *testing.T) {
	ids := []string{"open-link-shortener", "open-notifications", "open-news-feed", "open-nearby-search", "open-chat"}
	for _, id := range ids {
		sc := Get(id)
		if sc == nil {
			t.Errorf("missing %s", id)
			continue
		}
		if !sc.Open || sc.Level != "sde2" || sc.Difficulty != "Medium" || len(sc.WeakPoints) < 2 || len(sc.WeakPoints) > 4 {
			t.Errorf("%s: open=%v level=%q difficulty=%q weak points=%d", id, sc.Open, sc.Level, sc.Difficulty, len(sc.WeakPoints))
		}
		if len(sc.Diagram.Nodes) != 1 || sc.Diagram.Nodes[0].Kind != engine.KClient || !sc.Diagram.Nodes[0].Locked {
			t.Errorf("%s: starting diagram is not a locked users node only", id)
		}
		if _, has := alternatives[id]; has {
			t.Errorf("%s: open challenge has stored alternatives", id)
		}
	}
}

func TestM1OpenBriefsNeedTheDomainFixAndStayInsideBudget(t *testing.T) {
	cases := []struct {
		id    string
		wrong func(engine.Diagram) engine.Diagram
	}{
		{"open-link-shortener", func(d engine.Diagram) engine.Diagram { return removeNode(d, "app2") }},
		{"open-notifications", func(d engine.Diagram) engine.Diagram { return setParam(d, "events", "deadLetter", false) }},
		{"open-news-feed", func(d engine.Diagram) engine.Diagram { return setParam(d, "app", "feedStrategy", "pushOnWrite") }},
		{"open-nearby-search", func(d engine.Diagram) engine.Diagram { return setParam(d, "db", "spatialIndex", false) }},
		{"open-chat", func(d engine.Diagram) engine.Diagram { return setKindParam(d, engine.KAppServer, "maxConns", 100.0) }},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			sc := Get(tc.id)
			if got := engine.RunToEnd(tc.wrong(sc.Solution), sc.Options(1), sc.Goals); got.Pass {
				t.Errorf("plausible but incomplete design passes: %+v", got.Checks)
			}
			overbuilt := setKindParam(sc.Solution, engine.KAppServer, "threads", 64.0)
			overbuilt = setKindParam(overbuilt, engine.KWorker, "concurrency", 64.0)
			if got := engine.RunToEnd(overbuilt, sc.Options(1), sc.Goals); got.Pass {
				t.Errorf("overbuilt design passes the cost cap: %+v", got.Checks)
			}
		})
	}
}

func cloneOpenDesign(d engine.Diagram) engine.Diagram {
	out := d
	out.Nodes = append([]engine.Node(nil), d.Nodes...)
	out.Edges = append([]engine.Edge(nil), d.Edges...)
	for i := range out.Nodes {
		params := engine.Params{}
		for k, v := range out.Nodes[i].Params {
			params[k] = v
		}
		out.Nodes[i].Params = params
	}
	return out
}

func setParam(d engine.Diagram, id, key string, value any) engine.Diagram {
	out := cloneOpenDesign(d)
	for i := range out.Nodes {
		if out.Nodes[i].ID == id {
			out.Nodes[i].Params[key] = value
		}
	}
	return out
}

func setKindParam(d engine.Diagram, kind, key string, value any) engine.Diagram {
	out := cloneOpenDesign(d)
	for i := range out.Nodes {
		if out.Nodes[i].Kind == kind {
			out.Nodes[i].Params[key] = value
		}
	}
	return out
}

func removeNode(d engine.Diagram, id string) engine.Diagram {
	out := cloneOpenDesign(d)
	nodes := out.Nodes[:0]
	for _, n := range out.Nodes {
		if n.ID != id {
			nodes = append(nodes, n)
		}
	}
	out.Nodes = nodes
	edges := out.Edges[:0]
	for _, e := range out.Edges {
		if e.From != id && e.To != id {
			edges = append(edges, e)
		}
	}
	out.Edges = edges
	return out
}
