package scenarios

import (
	"math"
	"testing"

	"sysdesign/internal/engine"
)

// The restore window has two bottlenecks in series: a 400 Mbit/s link (50 MB/s) and one 50 MB/s stream out of the
// bucket. Widening only one of them leaves the other as the limit, so only both together meet the 90 s goal.
func TestRestoreWindowNeedsBothTheLinkAndTheStreams(t *testing.T) {
	sc := Get("restore-window")
	with := func(linkMbps, chunkKB, parallel float64) engine.Score {
		d := applyChanges(sc.Diagram, []Change{{"bucket", "multipartKB", chunkKB}, {"bucket", "multipartParallel", parallel}})
		for i := range d.Edges {
			if d.Edges[i].ID == "db>bucket" {
				e := d.Edges[i]
				e.BandwidthMbps = linkMbps
				d.Edges = append([]engine.Edge(nil), d.Edges...)
				d.Edges[i] = e
			}
		}
		return engine.RunToEnd(d, sc.Options(1), sc.Goals)
	}
	cases := []struct {
		name                  string
		link, chunk, parallel float64
		rto                   float64 // by hand: 30 ms + 12,288 MB / min(link/8, 50 x streams)
		pass                  bool
	}{
		{"as given", 400, 0, 4, 0.03 + 12288.0/50, false},
		{"link only, 10 Gbit/s", 10000, 0, 4, 0.03 + 12288.0/50, false},
		{"streams only, 8 parts", 400, 1000, 8, 0.03 + 12288.0/50, false},
		{"link 1 Gbit/s and 4 parts: link-bound at 125 MB/s", 1000, 1000, 4, 0.03 + 12288.0/125, false},
		{"link 2 Gbit/s and 4 parts: stream-bound at 200 MB/s", 2000, 1000, 4, 0.03 + 12288.0/200, true},
	}
	for _, c := range cases {
		r := with(c.link, c.chunk, c.parallel)
		t.Logf("%s: RTO %.1f s (by hand %.1f), pass %v, %d stars", c.name, r.Run.RtoSec, c.rto, r.Pass, r.Stars)
		if math.Abs(r.Run.RtoSec-c.rto)/c.rto > 0.05 {
			t.Errorf("%s: RTO %.1f s, by hand %.1f s", c.name, r.Run.RtoSec, c.rto)
		}
		if r.Pass != c.pass {
			t.Errorf("%s: pass=%v, want %v (%+v)", c.name, r.Pass, c.pass, r.Checks)
		}
	}
}
