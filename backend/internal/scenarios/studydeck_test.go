package scenarios

import (
	"strings"
	"testing"
)

// The study deck is for two people: one reads the questions aloud, the other answers, and the measured evidence checks
// the answer. It is built only from what is stored: the story, the alternatives and what each measured.
func TestAStudyDeckAsksWhyNotForEachStoredAlternative(t *testing.T) {
	deck, ok := StudyDeck("sde3")
	if !ok {
		t.Fatal("sde3 is a level")
	}
	for _, want := range []string{"# Study deck: SDE3", "## The refund that fails", "Why not ", "Evidence:", "Answer:"} {
		if !strings.Contains(deck, want) {
			t.Errorf("deck lacks %q", want)
		}
	}
	for _, sc := range All() {
		if sc.Level == "sde3" && len(Alternatives(sc.ID)) > 0 && !strings.Contains(deck, "## "+sc.Title) {
			t.Errorf("%s has stored alternatives but no card", sc.ID)
		}
	}
}

func TestADeckSkipsChallengesWithNothingToDefend(t *testing.T) {
	deck, _ := StudyDeck("sde1")
	for _, sc := range All() {
		if sc.Level == "sde1" && len(Alternatives(sc.ID)) == 0 && strings.Contains(deck, "## "+sc.Title+"\n") {
			t.Errorf("%s has no alternatives, so it has no counter-questions", sc.ID)
		}
	}
}

func TestADeckForAnUnknownLevelIsRefused(t *testing.T) {
	if _, ok := StudyDeck("sde9"); ok {
		t.Error("an unknown level has no deck")
	}
}

func TestEveryCounterQuestionStatesItsMeasuredResult(t *testing.T) {
	deck, _ := StudyDeck("sde2")
	if strings.Count(deck, "Evidence:") == 0 || strings.Count(deck, "Evidence:") != strings.Count(deck, "Why not ") {
		t.Errorf("each question needs its evidence: %d evidence lines for %d questions", strings.Count(deck, "Evidence:"), strings.Count(deck, "Why not "))
	}
}

func TestTermMarkersBecomePlainWords(t *testing.T) {
	if got := plain("a {{ttl|time to live}} and {{p95|p95}} here"); got != "a time to live and p95 here" {
		t.Errorf("got %q", got)
	}
}
