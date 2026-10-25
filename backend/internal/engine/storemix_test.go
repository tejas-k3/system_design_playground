package engine

import (
	"math"
	"testing"
)

// There is no separate PUT/GET setting on the object store: the mix is the users' own "Reads" share (readPct). A
// write with a body is a PUT (the claim check), a read with a response size is a GET (a download). This test pins
// that down so the control keeps working: at 70% reads, 70% of the store's requests are GETs.
func TestTheStoreSeesTheUsersReadShareAsItsGetShare(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 100.0, "readPct": 70.0, "reqKB": 100.0, "respKB": 100.0, "keySpace": 1000.0, "timeoutMs": 10000.0}).
		node("a", KAppServer, Params{"threads": 64.0}).
		node("s", KObjectStore, nil).node("q", KQueue, nil).node("w", KWorker, nil).node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "s").edge("a", "q").edge("q", "w").edge("w", "db").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := run(d, Options{Seed: 6}, 120)
	st := s.nodes["s"]
	gets := st.egressBytes / 100000
	puts := float64(st.osRequests) - gets
	share := gets / (gets + puts)
	t.Logf("store requests: %.0f GET, %.0f PUT, GET share %.3f (readPct 70)", gets, puts, share)
	if math.Abs(share-0.7) > 0.7*0.05 {
		t.Fatalf("GET share %.3f, want 0.70 +/- 5%%", share)
	}
}
