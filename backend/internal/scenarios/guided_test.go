package scenarios

import (
	"strings"
	"testing"

	"sysdesign/internal/engine"
)

func guided(t *testing.T) *Scenario {
	t.Helper()
	sc := Get(guidedID)
	if sc == nil {
		t.Fatal("the guided first-design challenge is not registered")
	}
	return sc
}

func TestGuidedChallengeIsAnOpenSde1BriefWithAStarterChecklist(t *testing.T) {
	sc := guided(t)
	if !sc.Open || sc.Level != "sde1" || sc.Difficulty != "Easy" {
		t.Errorf("open=%v level=%q difficulty=%q, want an open Easy sde1 brief", sc.Open, sc.Level, sc.Difficulty)
	}
	if len(sc.Starter) != 3 {
		t.Fatalf("%d starter steps, want 3 (users, app server, database)", len(sc.Starter))
	}
	kinds := map[string]bool{}
	for _, st := range sc.Starter {
		kinds[st.Kind] = true
		if st.Need == "" || st.Nudge == "" {
			t.Errorf("step %q needs both a need and a nudge", st.Kind)
		}
	}
	for _, k := range []string{engine.KClient, engine.KAppServer, engine.KDatabase} {
		if !kinds[k] {
			t.Errorf("no starter step for %s", k)
		}
	}
	if got := sc.Public().Starter; len(got) != 3 {
		t.Errorf("the served challenge carries %d starter steps", len(got))
	}
}

// The checklist names what is needed in plain words. Saying "add an App Server" would hand over the answer.
func TestStarterStepsNameTheNeedNotTheComponent(t *testing.T) {
	for _, sc := range All() {
		for _, st := range sc.Starter {
			for _, sp := range engine.Specs() {
				name := strings.ToLower(sp.Name)
				for _, text := range []string{st.Need, st.Nudge} {
					if strings.Contains(strings.ToLower(text), name) {
						t.Errorf("%s: %q gives away the component %q", sc.ID, text, name)
					}
				}
			}
		}
	}
}

// Too few servers fail the crash, too many break the budget, and one spare over the peak passes.
func TestGuidedChallengeNeedsEnoughServersAndNoMore(t *testing.T) {
	sc := guided(t)
	for _, seed := range []uint32{1, 2, 3} {
		for servers, want := range map[int]bool{1: false, 2: false, 3: true, 4: false} {
			if got := engine.RunToEnd(guidedDesign(servers), sc.Options(seed), sc.Goals).Pass; got != want {
				t.Errorf("seed %d, %d servers: pass=%v, want %v", seed, servers, got, want)
			}
		}
	}
}
