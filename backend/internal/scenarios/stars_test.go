package scenarios

import "testing"

// A star target that equals the pass limit gives the extra star for nothing: any run that passes already meets it.
func TestStarTargetsAskForMoreThanThePassLimits(t *testing.T) {
	for _, sc := range All() {
		g := sc.Goals
		if g.StarP95Ms != nil && g.MaxP95Ms != nil && *g.StarP95Ms >= *g.MaxP95Ms {
			t.Errorf("%s: the p95 star target (%.0f ms) is not below the pass limit (%.0f ms)", sc.ID, *g.StarP95Ms, *g.MaxP95Ms)
		}
		if g.StarCostPerHour != nil && g.MaxCostPerHour != nil && *g.StarCostPerHour >= *g.MaxCostPerHour {
			t.Errorf("%s: the cost star target ($%.2f) is not below the cap ($%.2f)", sc.ID, *g.StarCostPerHour, *g.MaxCostPerHour)
		}
	}
}
