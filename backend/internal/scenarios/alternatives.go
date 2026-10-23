package scenarios

import "sysdesign/internal/engine"

// An Alternative is a different single idea a learner might have tried on a challenge's starting design, with what
// the engine measured for it (seed 42). It is shown once the challenge is solved, next to the reference solution.
type Alternative struct {
	Label       string   `json:"label"`
	Changes     []Change `json:"changes"`
	Pass        bool     `json:"pass"`
	SuccessPct  float64  `json:"successPct"`
	P95Ms       float64  `json:"p95Ms"`
	CostPerHour float64  `json:"costPerHour"`
	Why         string   `json:"why"`
}

// Change sets one setting of one node.
type Change struct {
	Node  string `json:"node"`
	Param string `json:"param"`
	Value any    `json:"value"`
}

// Alternatives returns the stored alternatives for a challenge.
func Alternatives(id string) []Alternative { return alternatives[id] }

func applyChanges(d engine.Diagram, cs []Change) engine.Diagram {
	out := engine.Diagram{Nodes: append([]engine.Node(nil), d.Nodes...), Edges: append([]engine.Edge(nil), d.Edges...), Notes: d.Notes}
	for _, c := range cs {
		for i := range out.Nodes {
			if out.Nodes[i].ID != c.Node {
				continue
			}
			p := engine.Params{}
			for k, x := range out.Nodes[i].Params {
				p[k] = x
			}
			p[c.Param] = c.Value
			out.Nodes[i].Params = p
		}
	}
	return out
}

func alt(label string, pass bool, success, p95, cost float64, why string, cs ...Change) Alternative {
	return Alternative{Label: label, Changes: cs, Pass: pass, SuccessPct: success, P95Ms: p95, CostPerHour: cost, Why: why}
}

// The numbers are what the engine measured when they were written; TestAlternativesMatchTheEngine runs every one
// again, so they cannot drift from the simulator.
var alternatives = map[string][]Alternative{
	"gc-storm": {
		alt("Let clients retry twice", false, 99.1, 1013, 1.07, "Retries reach a healthy server, so nearly everything succeeds, but each rescued request has already waited out its timeout: the tail is a second.", Change{"users", "retries", 2.0}),
		alt("Give each server 32 threads", false, 91.6, 167, 2.62, "More threads do nothing for a server that is frozen, and they cost more.", Change{"app1", "threads", 32.0}, Change{"app2", "threads", 32.0}, Change{"app3", "threads", 32.0}),
	},
	"one-heavy-query": {
		alt("Raise the threads from 16 to 20", false, 97.9, 477, 1.45, "A little more room, but the heavy requests still take most of the thread-time; the light ones still queue behind them.", Change{"app", "threads", 20.0}),
		alt("Let clients retry twice", false, 84.0, 955, 1.34, "Retries add load to a server that has no spare threads.", Change{"users", "retries", 2.0}),
	},
	"reconnect-storm": {
		alt("Let clients retry three times", false, 68.3, 3196, 0.74, "Everyone retries together, so the herd arrives again and again.", Change{"users", "retries", 3.0}),
		alt("Spread reconnects over 2 seconds", false, 79.6, 831, 0.74, "Two seconds is still a spike for a server that sets up about 500 connections a second; thirty seconds is a trickle.", Change{"users", "reconnectJitterSec", 2.0}),
	},
	"hot-cell": {
		alt("Give the app server 64 threads", false, 0.0, 0, 2.60, "The wait is on the hot shard, not on the app server; the extra threads just wait there too, and cost more.", Change{"app", "threads", 64.0}),
		alt("Let clients retry twice", false, 0.0, 0, 1.14, "Retries go to the same shard.", Change{"users", "retries", 2.0}),
	},
	"bot-flood": {
		alt("Raise the shared limit to 400 requests a second", false, 58.1, 177, 2.53, "The bots use up the larger allowance too, so real customers are still turned away.", Change{"rl", "limitRps", 400.0}),
	},
	"retry-storm": {
		alt("Allow one immediate retry", false, 16.5, 204, 2.46, "One retry doubles the load on a system that is already struggling, so it never recovers.", Change{"users", "retries", 1.0}),
		alt("Allow no retries", true, 85.5, 200, 2.46, "Without the extra load the system recovers by itself; it passes, but does not protect the customers who got an error.", Change{"users", "retries", 0.0}),
	},
	"zone-outage": {
		alt("Spread only the servers over three zones", false, 56.2, 154, 1.92, "The balancer and the database are still in one zone, so losing it still takes the service down.", Change{"app2", "zone", "az-b"}, Change{"app3", "zone", "az-c"}),
	},
	"restore-drill": {
		alt("Back up every 5 seconds, nothing else", false, 35.7, 164, 0.80, "The data lost drops to 3 seconds, but the restore still takes 51 seconds: backups set the recovery point, not the recovery time.", Change{"db", "backupEverySec", 5.0}),
		alt("Restore at 600 MB/s, no backups", false, 77.9, 167, 0.74, "The database is back in 17 seconds, but with no backups the restore goes back to the very start: 28 seconds of writes lost.", Change{"db", "restoreMBps", 600.0}),
	},
	"lost-cart": {
		alt("Pass writes on in 50 ms instead of 300", false, 100.0, 177, 0.74, "A shorter window means fewer collisions (234 lost instead of 1,038 over the run), but any two writes to one cart inside it still lose one.", Change{"db", "syncLagMs", 50.0}),
		alt("Use a single leader", true, 100.0, 177, 0.74, "No conflicts at all, because every write goes through one place. It passes here; the price (not modelled in this run) is that the far region pays a cross-region round trip for every write.", Change{"db", "topology", "singleLeader"}),
	},
	"quorum-overlap": {
		alt("Wait for two copies on writes only (R=1, W=2)", false, 100.0, 216, 1.30, "1 + 2 is not more than 3 copies, so a read can still land on the one copy the write has not reached: 9 own-write misses.", Change{"kv", "writeQuorum", 2.0}),
		alt("Wait for two copies on reads only (R=2, W=1)", false, 100.0, 161, 1.30, "Closer, but 2 + 1 is still not more than 3: one read in the run still missed a write.", Change{"kv", "readQuorum", 2.0}),
		alt("Write to all three copies (R=1, W=3)", true, 100.0, 321, 1.30, "1 + 3 is more than 3, so it works, but every write waits for the slowest copy (p95 321 ms against 216) and would fail if any copy were down.", Change{"kv", "writeQuorum", 3.0}),
	},
	"lease-expired": {
		alt("Make the lock lease 1 second", true, 97.5, 2238, 2.71, "Passes this run, because every pause (600 ms) is shorter than the lease. It is a bet on how long a pause can be: with 3-second pauses it double books again (6 on this seed), where the conditional write still has none.", Change{"app1", "lockTtlMs", 1000.0}, Change{"app2", "lockTtlMs", 1000.0}, Change{"app3", "lockTtlMs", 1000.0}),
	},
	"add-a-shard": {
		alt("Spread keys over a ring with one point per shard", true, 100.0, 148, 3.79, "Still a ring, so only about a fifth of the keys move; with a single point per shard the split is less even, but the move is still small enough here to pass.", Change{"shards", "hashing", "ring"}, Change{"shards", "vnodes", 1.0}),
	},
	"heavy-tail": {
		alt("Give the app server 96 threads", false, 100.0, 264, 2.91, "The slow reads are slow by themselves, not queued: more threads leave the p95 where it was (264 ms) and cost more.", Change{"app", "threads", 96.0}),
		alt("Hedge after only 5 ms", true, 100.0, 164, 2.12, "It passes and has the best p95, but almost every read gets a second request (6,082 hedges for about 6,000 reads): the replicas carry twice the load for a gain that waiting longer gets most of.", Change{"app", "hedgeReadsMs", 5.0}),
	},
	"video-upload": {
		alt("Give the app server 64 threads", false, 4.9, 3744, 2.50, "The uploads share one saturated link; more threads only add more uploads to the same link.", Change{"app", "threads", 64.0}),
	},
}
