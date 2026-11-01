package scenarios

import "sysdesign/internal/engine"

// registerStorage registers the challenges about where bytes are kept and how fast they move back.
func (c *catalog) registerStorage() {
	// A 12 GB database is restored from backups in an object store. The restore streams through the slowest of: the
	// link to the bucket, the bucket's streams (one stream is 50 MB/s; multipart runs several at once) and the
	// database's own restore speed. The link is narrow AND the bucket is read as a single stream, so fixing either
	// alone moves the bottleneck to the other.
	restoreWindow := func(fixed bool) engine.Diagram {
		link, chunk := 400.0, 0.0
		if fixed {
			link, chunk = 2000, 1000
		}
		b := (&bld{}).
			n("users", engine.KClient, cols[0], rowMid, P{"rps": 20.0, "readPct": 50.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0, "retries": 0.0}).
			n("app", engine.KAppServer, cols[1], rowMid, P{"threads": 16.0}).
			n("db", engine.KDatabase, cols[2], rowTop, P{"connections": 32.0, "dataGB": 12.0, "backupEverySec": 10.0, "restoreMBps": 1000.0}).label("db", "Orders database").
			n("bucket", engine.KObjectStore, cols[2], rowBot, P{"multipartKB": chunk, "multipartParallel": 4.0}).label("bucket", "Backup bucket").
			e("users", "app").e("app", "db").e("db", "bucket")
		for k := range b.d.Edges {
			if b.d.Edges[k].ID == "db>bucket" {
				b.d.Edges[k].BandwidthMbps = link
			}
		}
		return b.done()
	}
	c.register(&Scenario{
		ID: "restore-window", Title: "The restore window", Difficulty: "Medium", ConfigOnly: true,
		Summary: "The backups are safe in the bucket, but getting them back takes four minutes.",
		Story:   "The orders database holds 12 GB and is backed up to a bucket every ten seconds. At 25 seconds a bad migration starts corrupting rows, and at 28 seconds someone restores the last good backup. The business says the database may be unavailable for at most 90 seconds while it is restored and may lose at most 15 seconds of writes. The database can write 1,000 MB a second, so it is not the slow part. In the last drill the restore ran for far longer than 90 seconds.",
		Diagram: restoreWindow(false), Solution: restoreWindow(true),
		Chaos:       []engine.ChaosEvent{{Type: "corrupt", NodeID: "db", AtSec: 25}, {Type: "restore", NodeID: "db", AtSec: 28}},
		Goals:       engine.Goals{MinSuccessRate: 0.5, MaxRpoSec: f(15), MaxRtoSec: f(90), MaxCostPerHour: f(1.3), StarCostPerHour: f(1.1), MaxP95Ms: f(400), StarP95Ms: f(250)},
		DurationSec: 150, WarmupSec: 0, Speed: 1,
		Hints: [3]string{
			"Estimate the restore time: the size of the database divided by the speed at which the backup comes back. What is that speed, and what limits it?",
			"The backup passes through the link to the bucket and is read from the bucket in streams. Whichever is slowest sets the speed, so widening one of them can just move the limit to the other.",
			"Of the things the backup passes through on its way back, which is the slowest now, and what becomes the slowest once you have fixed that one?",
		},
		Answer: "The answer: widen the link between the database and the backup bucket, and turn on multipart in the bucket with several parts at once, so that neither the link nor a single stream is the limit. Either change alone only moves the bottleneck.",
		Predictions: []Prediction{{ID: "p1", Question: "Raising only the link to the bucket to 10 Gbit/s will…", Options: []string{"Bring the restore inside the window, since the link was the limit", "Leave it slow, because one stream still tops out at its own speed", "Make it slower, as a wider link has more data to carry back"}, Answer: 1,
			Explain: "A single stream from the bucket tops out at its own speed however wide the link is. The link only matters once several streams run at once; until then the stream is the limit."}},
		Debrief: "A restore is a pipeline, and its speed is the speed of its slowest stage: the link, the bucket's streams, the database's write speed. Work out each stage's rate, widen the smallest one, then check which stage is now the smallest. Recovery time is data size divided by that rate, so measure it by practising the restore before the day it is needed.",
	})
}
