package scenarios

import "testing"

// Every challenge says which interview band it is for, and each band has enough challenges to work through.
func TestEveryScenarioHasALevel(t *testing.T) {
	count := map[string]int{}
	for _, sc := range All() {
		if !validLevel(sc.Level) {
			t.Errorf("%s has level %q, want one of %v", sc.ID, sc.Level, Levels)
			continue
		}
		count[sc.Level]++
	}
	for _, l := range Levels {
		if count[l] < 8 {
			t.Errorf("level %s has %d challenges, want at least 8", l, count[l])
		}
	}
	for id := range levelOf {
		if Get(id) == nil {
			t.Errorf("level given for %q, which is not a challenge", id)
		}
	}
}

func TestPublicScenarioCarriesItsLevel(t *testing.T) {
	sc := Get("hello-world")
	if sc.Level != "sde1" || sc.Public().Level != "sde1" {
		t.Fatalf("hello-world level %q / %q", sc.Level, sc.Public().Level)
	}
}
