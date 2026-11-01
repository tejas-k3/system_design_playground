package engine

import (
	"fmt"
	"math"
)

// Goals are the pass criteria and star thresholds of a challenge. Pointers mean "not required".
type Goals struct {
	MinSuccessRate          float64  `json:"minSuccessRate"`
	MaxP50Ms                *float64 `json:"maxP50Ms,omitempty"`
	MaxP95Ms                *float64 `json:"maxP95Ms,omitempty"`
	MaxCostPerHour          *float64 `json:"maxCostPerHour,omitempty"`
	MinJobsCompleted        *float64 `json:"minJobsCompleted,omitempty"` // fraction 0..1
	MaxJobP95Ms             *float64 `json:"maxJobP95Ms,omitempty"`
	MaxConflicts            *int     `json:"maxConflicts,omitempty"`
	MaxStaleReadPct         *float64 `json:"maxStaleReadPct,omitempty"` // fraction 0..1
	MaxDuplicates           *int     `json:"maxDuplicates,omitempty"`   // duplicate writes, or duplicate pages fetched by a crawler
	MaxLostWrites           *int     `json:"maxLostWrites,omitempty"`
	MaxLostUpdates          *int     `json:"maxLostUpdates,omitempty"`          // transfers that silently overwrote each other
	MinCustomerSuccess      *float64 `json:"minCustomerSuccess,omitempty"`      // share of the real customers' requests that must succeed, bots set aside
	MaxJobsLost             *int     `json:"maxJobsLost,omitempty"`             // jobs given up with nowhere to go (not dead-lettered)
	MaxWriteSkew            *int     `json:"maxWriteSkew,omitempty"`            // on-call pairs left with the rule broken
	MaxRpoSec               *float64 `json:"maxRpoSec,omitempty"`               // seconds of data a restore may lose
	MaxRtoSec               *float64 `json:"maxRtoSec,omitempty"`               // seconds the database may be unavailable restoring
	MinDeliveriesPerMessage *float64 `json:"minDeliveriesPerMessage,omitempty"` // copies of each accepted message that were delivered (fan-out)
	MinWorstTenant          *float64 `json:"minWorstTenant,omitempty"`          // the worst-served of the quiet tenants must still succeed at least this often
	MaxInconsistent         *int     `json:"maxInconsistent,omitempty"`         // multi-step writes left half done
	MaxBlockedSec           *float64 `json:"maxBlockedSec,omitempty"`           // lock-seconds held waiting for a crashed coordinator
	MaxRepeatedOutputs      *int     `json:"maxRepeatedOutputs,omitempty"`      // stream outputs produced a second time after a crash
	MaxRecoverySec          *float64 `json:"maxRecoverySec,omitempty"`          // longest time a stream job took to catch up after a crash
	MaxOrderViolations      *int     `json:"maxOrderViolations,omitempty"`      // messages a consumer finished after a newer one of the same key
	MaxWrongAnswers         *int     `json:"maxWrongAnswers,omitempty"`         // wrong answers returned as correct
	MaxClockLost            *int     `json:"maxClockLost,omitempty"`            // newer writes dropped because a clock was behind
	MaxReauth               *int     `json:"maxReauth,omitempty"`               // requests that found their user signed out
	MaxUnchecked            *int     `json:"maxUnchecked,omitempty"`            // requests let through without a token check
	MaxInDoubt              *int     `json:"maxInDoubt,omitempty"`              // two-phase commits left waiting for a crashed coordinator
	MaxTTFTMs               *float64 `json:"maxTTFTMs,omitempty"`               // time to first token (p95) of LLM answers
	MinRecall               *float64 `json:"minRecall,omitempty"`               // recall of a vector search or crawl workload
	MinTopKRecall           *float64 `json:"minTopKRecall,omitempty"`           // share of the true top ten a stream job must find
	MaxStateKB              *float64 `json:"maxStateKB,omitempty"`              // memory a stream job needs to count its visitors
	MaxCountErrorPct        *float64 `json:"maxCountErrorPct,omitempty"`        // error of an approximate visitor count
	MaxLateDropped          *int     `json:"maxLateDropped,omitempty"`          // events a stream job dropped for arriving too late
	MaxResultDelaySec       *float64 `json:"maxResultDelaySec,omitempty"`       // how long after a window ends a stream job first emits it
	MaxLostEvents           *int     `json:"maxLostEvents,omitempty"`           // saved writes whose event never reached the queue
	MaxUnbalanced           *int     `json:"maxUnbalanced,omitempty"`           // ledger entries that do not balance: left half done, or saved and never announced
	MaxSessionViolations    *int     `json:"maxSessionViolations,omitempty"`    // violations of the client's selected session guarantee
	MaxCacheMessages        *int     `json:"maxCacheMessages,omitempty"`        // messages the origin sent to caches about writes
	StarCostPerHour         *float64 `json:"starCostPerHour,omitempty"`
	StarP95Ms               *float64 `json:"starP95Ms,omitempty"`
}

type Check struct {
	Label  string `json:"label"`
	Target string `json:"target"`
	Actual string `json:"actual"`
	Pass   bool   `json:"pass"`
}

// Score is the verdict for one finished run.
type Score struct {
	Pass        bool     `json:"pass"`
	Stars       int      `json:"stars"`
	Checks      []Check  `json:"checks"`
	Run         RunStats `json:"run"`
	CostPerHour float64  `json:"costPerHour"`
}

const eps = 1e-9

// WithStarTargets fills in the star targets a challenge did not set explicitly: two stars need a cost
// within 80% of the cost goal, three need a p95 within 60% of the p95 goal. Published with the goals so
// the learner can see the bar, and used by Evaluate so the two can never disagree.
func (g Goals) WithStarTargets() Goals {
	if g.StarCostPerHour == nil && g.MaxCostPerHour != nil {
		v := 0.8 * *g.MaxCostPerHour
		g.StarCostPerHour = &v
	}
	if g.StarP95Ms == nil && g.MaxP95Ms != nil {
		v := 0.6 * *g.MaxP95Ms
		g.StarP95Ms = &v
	}
	return g
}

func pct(f float64) string {
	if f > 0 && f < 0.0001 { // a limit this small must not read as zero either
		return "<0.01%"
	}
	if f > 0 && f < 0.001 { // under a tenth of a percent: two decimals
		return fmt.Sprintf("%.2f%%", math.Floor(f*10000+eps)/100)
	}
	return fmt.Sprintf("%.1f%%", math.Floor(f*1000+eps)/10)
}

func msStr(v float64) string {
	if v >= 10000 {
		return fmt.Sprintf("%.1f s", v/1000)
	}
	return fmt.Sprintf("%d ms", int(math.Round(v)))
}

// Unwired lists app servers that answer without any data store, and workers that never write.
func Unwired(d Diagram) []Node {
	kind := map[string]string{}
	for _, n := range d.Nodes {
		kind[n.ID] = n.Kind
	}
	isData := func(k string) bool {
		switch k {
		case KDatabase, KReplica, KCache, KRedis, KQueue, KShardRouter, KObjectStore, KIndex, KTopic, KLog, KKVStore, KCoordinator, KStream, KLLMServer, KVectorStore:
			return true
		}
		return false
	}
	hasData := false
	for _, n := range d.Nodes {
		if isData(n.Kind) {
			hasData = true
		}
	}
	if !hasData {
		return nil
	}
	var out []Node
	for _, n := range d.Nodes {
		switch n.Kind {
		case KAppServer:
			incoming, reaches := false, false
			for _, e := range d.Edges {
				if e.To == n.ID {
					incoming = true
				}
				if e.From == n.ID && isData(kind[e.To]) {
					reaches = true
				}
			}
			if incoming && !reaches {
				out = append(out, n)
			}
		case KWorker:
			fed, writes := false, false
			for _, e := range d.Edges {
				if e.To == n.ID && (kind[e.From] == KQueue || kind[e.From] == KTopic || kind[e.From] == KLog) {
					fed = true
				}
				if e.From == n.ID && (kind[e.To] == KDatabase || kind[e.To] == KShardRouter || kind[e.To] == KRedis) {
					writes = true
				}
			}
			if fed && !writes {
				out = append(out, n)
			}
		}
	}
	return out
}

// Evaluate checks a finished run against goals and awards 0-3 stars.
func Evaluate(g Goals, d Diagram, run RunStats, cost float64) Score {
	g = g.WithStarTargets()
	var checks []Check
	total := run.Resolved + run.Pending
	rate := 0.0
	if total > 0 {
		rate = float64(run.OK) / float64(total)
	}
	actual := "no requests"
	if total > 0 {
		actual = pct(rate)
	}
	checks = append(checks, Check{"Success rate", "≥ " + pct(g.MinSuccessRate), actual, rate+eps >= g.MinSuccessRate})
	hasOK := run.OK > 0
	if g.MaxP50Ms != nil {
		a := "—"
		if hasOK {
			a = msStr(run.P50)
		}
		checks = append(checks, Check{"Median latency", "≤ " + msStr(*g.MaxP50Ms), a, hasOK && run.P50 <= *g.MaxP50Ms+eps})
	}
	if g.MaxP95Ms != nil {
		a := "—"
		if hasOK {
			a = msStr(run.P95)
		}
		checks = append(checks, Check{"p95 latency", "≤ " + msStr(*g.MaxP95Ms), a, hasOK && run.P95 <= *g.MaxP95Ms+eps})
	}
	if g.MaxConflicts != nil {
		checks = append(checks, Check{"Double bookings", fmt.Sprintf("≤ %d", *g.MaxConflicts), fmt.Sprint(run.Conflicts), run.Conflicts <= *g.MaxConflicts})
	}
	if g.MaxStaleReadPct != nil {
		frac := 0.0
		a := "no fresh reads"
		if run.Fresh > 0 {
			frac = float64(run.Stale) / float64(run.Fresh)
			a = pct(frac)
		}
		checks = append(checks, Check{"Too-stale reads", "≤ " + pct(*g.MaxStaleReadPct), a, frac <= *g.MaxStaleReadPct+eps})
	}
	if g.MaxDuplicates != nil {
		got, label := run.Dups, "Duplicate writes"
		if run.Crawl != nil {
			got, label = run.Crawl.Refetched, "Duplicate pages fetched"
		}
		checks = append(checks, Check{label, fmt.Sprintf("≤ %d", *g.MaxDuplicates), fmt.Sprint(got), got <= *g.MaxDuplicates})
	}
	if g.MaxLostWrites != nil {
		checks = append(checks, Check{"Writes lost in a crash", fmt.Sprintf("≤ %d", *g.MaxLostWrites), fmt.Sprint(run.LostWrites), run.LostWrites <= *g.MaxLostWrites})
	}
	if g.MaxLostUpdates != nil {
		checks = append(checks, Check{"Lost updates", fmt.Sprintf("≤ %d", *g.MaxLostUpdates), fmt.Sprint(run.LostUpdates), run.LostUpdates <= *g.MaxLostUpdates})
	}
	if g.MinCustomerSuccess != nil {
		frac, a := 0.0, "no customers"
		if run.CustomerTotal > 0 {
			frac = float64(run.CustomerOK) / float64(run.CustomerTotal)
			a = pct(frac)
		}
		checks = append(checks, Check{"Customers served", "≥ " + pct(*g.MinCustomerSuccess), a, run.CustomerTotal > 0 && frac+eps >= *g.MinCustomerSuccess})
	}
	if g.MaxJobsLost != nil {
		lost := 0
		if run.Jobs != nil {
			lost = run.Jobs.Lost
		}
		checks = append(checks, Check{"Jobs lost", fmt.Sprintf("≤ %d", *g.MaxJobsLost), fmt.Sprint(lost), lost <= *g.MaxJobsLost})
	}
	if g.MaxWriteSkew != nil {
		checks = append(checks, Check{"Rule broken (write skew)", fmt.Sprintf("≤ %d", *g.MaxWriteSkew), fmt.Sprint(run.WriteSkew), run.WriteSkew <= *g.MaxWriteSkew})
	}
	if g.MinDeliveriesPerMessage != nil {
		a, ok := "no messages", false
		if run.OK > 0 && run.Jobs != nil {
			per := float64(run.Jobs.Completed) / float64(run.OK)
			a, ok = fmt.Sprintf("%.2f", per), per+eps >= *g.MinDeliveriesPerMessage
		}
		checks = append(checks, Check{"Deliveries per message", fmt.Sprintf("≥ %.1f", *g.MinDeliveriesPerMessage), a, ok})
	}
	if g.MinWorstTenant != nil {
		a, ok := "no tenants", false
		if run.Tenants > 1 {
			a, ok = pct(run.WorstTenantSuccess), run.WorstTenantSuccess+eps >= *g.MinWorstTenant
		}
		checks = append(checks, Check{"Worst-served quiet tenant", "≥ " + pct(*g.MinWorstTenant), a, ok})
	}
	if g.MaxInconsistent != nil {
		checks = append(checks, Check{"Orders left half done", fmt.Sprintf("≤ %d", *g.MaxInconsistent), fmt.Sprint(run.Inconsistent), run.Inconsistent <= *g.MaxInconsistent})
	}
	if g.MinRecall != nil {
		recall, label := run.Recall, "Vector search recall"
		if run.Crawl != nil {
			recall, label = run.Crawl.RecallPct/100, "Crawl recall"
		}
		checks = append(checks, Check{label, fmt.Sprintf("≥ %.0f%%", *g.MinRecall*100), fmt.Sprintf("%.0f%%", recall*100), recall+eps >= *g.MinRecall})
	}
	if g.MaxTTFTMs != nil {
		checks = append(checks, Check{"Time to first token (p95)", fmt.Sprintf("≤ %.0f ms", *g.MaxTTFTMs), fmt.Sprintf("%.0f ms", run.TTFTMs), run.TTFTMs <= *g.MaxTTFTMs+eps})
	}
	if g.MinTopKRecall != nil {
		checks = append(checks, Check{"Top ten found", fmt.Sprintf("≥ %.0f of 10", *g.MinTopKRecall*10), fmt.Sprintf("%.0f of 10", run.TopKRecall*10), run.TopKRecall+eps >= *g.MinTopKRecall})
	}
	if g.MaxStateKB != nil {
		checks = append(checks, Check{"Memory to count visitors", fmt.Sprintf("≤ %.0f KB", *g.MaxStateKB), fmt.Sprintf("%.1f KB", run.StateKB), run.StateKB <= *g.MaxStateKB+eps})
	}
	if g.MaxCountErrorPct != nil {
		checks = append(checks, Check{"Error of the visitor count", fmt.Sprintf("≤ %.0f%%", *g.MaxCountErrorPct), fmt.Sprintf("%.1f%%", run.CountErrorPct), run.CountErrorPct <= *g.MaxCountErrorPct+eps})
	}
	if g.MaxLateDropped != nil {
		checks = append(checks, Check{"Late events dropped", fmt.Sprintf("≤ %d", *g.MaxLateDropped), fmt.Sprint(run.LateDropped), run.LateDropped <= *g.MaxLateDropped})
	}
	if g.MaxResultDelaySec != nil {
		checks = append(checks, Check{"Wait for a window's first result", fmt.Sprintf("≤ %.0f s", *g.MaxResultDelaySec), fmt.Sprintf("%.0f s", run.ResultDelaySec), run.ResultDelaySec <= *g.MaxResultDelaySec+eps})
	}
	if g.MaxBlockedSec != nil {
		checks = append(checks, Check{"Lock-seconds held for a dead coordinator", fmt.Sprintf("≤ %.1f s", *g.MaxBlockedSec), fmt.Sprintf("%.1f s", run.BlockedSec), run.BlockedSec <= *g.MaxBlockedSec+eps})
	}
	if g.MaxRepeatedOutputs != nil {
		checks = append(checks, Check{"Outputs repeated after a crash", fmt.Sprintf("≤ %d", *g.MaxRepeatedOutputs), fmt.Sprint(run.RepeatedOutputs), run.RepeatedOutputs <= *g.MaxRepeatedOutputs})
	}
	if g.MaxRecoverySec != nil {
		checks = append(checks, Check{"Time to catch up after a crash", fmt.Sprintf("≤ %.1f s", *g.MaxRecoverySec), fmt.Sprintf("%.2f s", run.RecoverySec), run.RecoverySec <= *g.MaxRecoverySec+eps})
	}
	if g.MaxOrderViolations != nil {
		late := 0
		if run.Jobs != nil {
			late = run.Jobs.OrderViolations
		}
		checks = append(checks, Check{"Messages handled out of order", fmt.Sprintf("≤ %d", *g.MaxOrderViolations), fmt.Sprint(late), late <= *g.MaxOrderViolations})
	}
	if g.MaxWrongAnswers != nil {
		checks = append(checks, Check{"Wrong answers accepted", fmt.Sprintf("≤ %d", *g.MaxWrongAnswers), fmt.Sprint(run.WrongAnswers), run.WrongAnswers <= *g.MaxWrongAnswers})
	}
	if g.MaxClockLost != nil {
		checks = append(checks, Check{"Newer writes lost to a clock", fmt.Sprintf("≤ %d", *g.MaxClockLost), fmt.Sprint(run.ClockLost), run.ClockLost <= *g.MaxClockLost})
	}
	if g.MaxReauth != nil {
		checks = append(checks, Check{"Users signed out by a lost session", fmt.Sprintf("≤ %d", *g.MaxReauth), fmt.Sprint(run.Reauth), run.Reauth <= *g.MaxReauth})
	}
	if g.MaxUnchecked != nil {
		checks = append(checks, Check{"Requests let through unchecked", fmt.Sprintf("≤ %d", *g.MaxUnchecked), fmt.Sprint(run.Unchecked), run.Unchecked <= *g.MaxUnchecked})
	}
	if g.MaxInDoubt != nil {
		checks = append(checks, Check{"Transactions waiting on a dead coordinator", fmt.Sprintf("≤ %d", *g.MaxInDoubt), fmt.Sprint(run.InDoubt), run.InDoubt <= *g.MaxInDoubt})
	}
	if g.MaxLostEvents != nil {
		checks = append(checks, Check{"Events lost", fmt.Sprintf("≤ %d", *g.MaxLostEvents), fmt.Sprint(run.EventsLost), run.EventsLost <= *g.MaxLostEvents})
	}
	if g.MaxUnbalanced != nil {
		checks = append(checks, Check{"Entries that do not balance", fmt.Sprintf("≤ %d", *g.MaxUnbalanced), fmt.Sprint(run.Inconsistent + run.EventsLost), run.Inconsistent+run.EventsLost <= *g.MaxUnbalanced})
	}
	if g.MaxRpoSec != nil {
		checks = append(checks, Check{"Data lost (RPO)", fmt.Sprintf("≤ %.0f s", *g.MaxRpoSec), fmt.Sprintf("%.1f s", run.RpoSec), run.RpoSec <= *g.MaxRpoSec+eps})
	}
	if g.MaxRtoSec != nil {
		checks = append(checks, Check{"Time to restore (RTO)", fmt.Sprintf("≤ %.0f s", *g.MaxRtoSec), fmt.Sprintf("%.1f s", run.RtoSec), run.RtoSec <= *g.MaxRtoSec+eps})
	}
	if g.MaxCacheMessages != nil {
		checks = append(checks, Check{"Messages to caches", fmt.Sprintf("≤ %d", *g.MaxCacheMessages), fmt.Sprint(run.CacheMsgs), run.CacheMsgs <= *g.MaxCacheMessages})
	}
	if g.MaxSessionViolations != nil {
		broken := run.ReadYourWritesBroken + run.MonotonicBroken + run.MonoWritesBroken + run.FollowReadBroken + run.CausalBroken
		checks = append(checks, Check{"Per-client guarantees broken", fmt.Sprintf("≤ %d", *g.MaxSessionViolations), fmt.Sprint(broken), broken <= *g.MaxSessionViolations})
	}
	if g.MinJobsCompleted != nil {
		enq, frac := 0, 1.0
		if run.Jobs != nil {
			enq = run.Jobs.Enqueued
			if enq > 0 {
				frac = float64(run.Jobs.Completed) / float64(enq)
			}
		}
		a := "no jobs"
		if enq > 0 {
			a = pct(frac)
		}
		checks = append(checks, Check{"Jobs processed", "≥ " + pct(*g.MinJobsCompleted), a, frac+eps >= *g.MinJobsCompleted})
	}
	if g.MaxJobP95Ms != nil {
		ok, a := true, "no jobs"
		if run.Jobs != nil && run.Jobs.Completed > 0 {
			ok, a = run.Jobs.P95 <= *g.MaxJobP95Ms+eps, msStr(run.Jobs.P95)
		} else if run.Jobs != nil && run.Jobs.Enqueued > 0 {
			ok, a = false, "—"
		}
		checks = append(checks, Check{"Processing delay p95", "≤ " + msStr(*g.MaxJobP95Ms), a, ok})
	}
	if g.MaxCostPerHour != nil {
		checks = append(checks, Check{"Cost", fmt.Sprintf("≤ $%.2f/hr", *g.MaxCostPerHour), fmt.Sprintf("$%.2f/hr", cost), cost <= *g.MaxCostPerHour+eps})
	}
	if un := Unwired(d); len(un) > 0 {
		who := un[0].Label
		if len(un) > 1 {
			who += fmt.Sprintf(" and %d more", len(un)-1)
		}
		checks = append(checks, Check{"Wired to data", "every server", who + " never touches a data store", false})
	}
	pass := true
	for _, c := range checks {
		pass = pass && c.Pass
	}
	stars := 0
	if pass {
		stars = 1
		if g.StarCostPerHour == nil || cost <= *g.StarCostPerHour+eps {
			stars = 2
			switch {
			case g.StarP95Ms != nil:
				if hasOK && run.P95 <= *g.StarP95Ms+eps {
					stars = 3
				}
			case g.MaxP50Ms != nil:
				if hasOK && run.P50 <= 0.5**g.MaxP50Ms+eps {
					stars = 3
				}
			default:
				if rate >= 0.999-eps {
					stars = 3
				}
			}
		}
	}
	return Score{Pass: pass, Stars: stars, Checks: checks, Run: run, CostPerHour: cost}
}

// RunToEnd simulates a scenario headlessly until it finishes and scores it.
func RunToEnd(d Diagram, opts Options, g Goals) Score {
	d, _ = Sanitize(d)
	s := New(d, opts)
	limit := opts.StopTrafficAtMs + 60000
	if opts.StopTrafficAtMs <= 0 {
		limit = 60000
	}
	for !s.Finished() && s.Now() < limit {
		s.Advance(100)
	}
	return Evaluate(g, d, s.RunSnapshot(), s.CostPerHour())
}
