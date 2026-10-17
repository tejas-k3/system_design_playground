package scenarios

import (
	"testing"

	"sysdesign/internal/engine"
)

func lintRules(d engine.Diagram) map[string]string {
	m := map[string]string{}
	for _, f := range engine.Lint(d) {
		m[f.Rule] = f.Severity
	}
	return m
}

// A rule that calls a reference solution broken is a bug in the rule.
func TestNoReferenceSolutionHasALintError(t *testing.T) {
	for _, s := range All() {
		for _, f := range engine.Lint(s.Solution) {
			if f.Severity == "error" {
				t.Errorf("%s solution: error %s on %v", s.ID, f.Rule, f.Nodes)
			}
		}
	}
}

func TestLintFindsTheFaultEachChallengeTeaches(t *testing.T) {
	want := map[string][]string{
		"retry-pileup":          {"retries-without-backoff", "retry-amplification", "no-call-timeout"},
		"cut-off":               {"no-call-timeout"},
		"connection-crunch":     {"pool-exceeds-db"},
		"fence-the-old-primary": {"unfenced-failover"},
		"stale-prices":          {"synchronised-expiry"},
		"cache-stampede":        {"synchronised-expiry"},
		"charged-twice":         {"retries-without-backoff"},
		"opening-night":         {"spof-database"},
	}
	for id, rules := range want {
		got := lintRules(Get(id).Diagram)
		for _, r := range rules {
			if _, ok := got[r]; !ok {
				t.Errorf("%s start should trigger %s, got %v", id, r, got)
			}
		}
	}
	// and the fix removes the finding that names it
	gone := map[string]string{"retry-pileup": "retries-without-backoff", "connection-crunch": "pool-exceeds-db", "fence-the-old-primary": "unfenced-failover", "cut-off": "no-call-timeout", "cache-stampede": "synchronised-expiry"}
	for id, r := range gone {
		if _, ok := lintRules(Get(id).Solution)[r]; ok {
			t.Errorf("%s solution still triggers %s", id, r)
		}
	}
}
