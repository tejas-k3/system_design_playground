package engine

import "testing"

func TestCrawlDuplicateGoalUsesRefetchedPages(t *testing.T) {
	max := 0
	score := Evaluate(Goals{MinSuccessRate: 1, MaxDuplicates: &max}, Diagram{}, RunStats{Crawl: &CrawlStats{Refetched: 7}}, 0)
	if score.Pass {
		t.Fatal("a crawl that refetched pages must fail a zero-duplicate goal")
	}
	last := score.Checks[len(score.Checks)-1]
	if last.Actual != "7" || last.Label != "Duplicate pages fetched" {
		t.Fatalf("crawl duplicate check = %+v", last)
	}
}

func TestCrawlRecallGoalUsesCrawlerRecall(t *testing.T) {
	min := 0.9
	score := Evaluate(Goals{MinSuccessRate: 1, MinRecall: &min}, Diagram{}, RunStats{Crawl: &CrawlStats{RecallPct: 80}}, 0)
	if score.Pass {
		t.Fatal("a crawl below its recall target must fail")
	}
	last := score.Checks[len(score.Checks)-1]
	if last.Actual != "80%" || last.Label != "Crawl recall" {
		t.Fatalf("crawl recall check = %+v", last)
	}
}

func TestStarTargetsAreDerivedFromTheGoals(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	g := Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1)}.WithStarTargets()
	if g.StarCostPerHour == nil || *g.StarCostPerHour != 0.8 {
		t.Errorf("star cost = %v, want 0.8 (80%% of the cost goal)", g.StarCostPerHour)
	}
	if g.StarP95Ms == nil || *g.StarP95Ms != 240 {
		t.Errorf("star p95 = %v, want 240 (60%% of the p95 goal)", g.StarP95Ms)
	}
	g = Goals{MinSuccessRate: 0.99, MaxP95Ms: f(400), MaxCostPerHour: f(1), StarCostPerHour: f(0.95), StarP95Ms: f(300)}.WithStarTargets()
	if *g.StarCostPerHour != 0.95 || *g.StarP95Ms != 300 {
		t.Errorf("explicit star targets were overwritten: %v %v", *g.StarCostPerHour, *g.StarP95Ms)
	}
	if g := (Goals{MinSuccessRate: 0.9}).WithStarTargets(); g.StarCostPerHour != nil || g.StarP95Ms != nil {
		t.Errorf("no cost or p95 goal, so no star targets: %+v", g)
	}
}
