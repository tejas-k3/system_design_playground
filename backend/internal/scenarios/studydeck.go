package scenarios

import (
	"fmt"
	"strings"
)

// maxQuestions is how many counter-questions one card carries; a longer list stops being a conversation.
const maxQuestions = 3

// StudyDeck is a printable deck for a study partner, one card for each challenge of a level that has stored alternatives:
// the prompt, the answer to look for, and up to three counter-questions ("why not ...?") each with the evidence the
// simulator measured for it. It reports false for a level that does not exist or has no card.
func StudyDeck(level string) (string, bool) {
	if !validLevel(level) {
		return "", false
	}
	var b strings.Builder
	cards := 0
	fmt.Fprintf(&b, "# Study deck: %s\n\nOne person reads a card, the other answers out loud, then you compare with the evidence. The numbers were measured by the simulator (seed 42) and the tests re-run them, so they match the engine.\n", strings.ToUpper(level))
	for _, sc := range All() {
		if sc.Level != level {
			continue
		}
		alts := Alternatives(sc.ID)
		if len(alts) == 0 {
			continue
		}
		cards++
		fmt.Fprintf(&b, "\n## %s\n\n**Prompt:** %s\n\n", oneLine(sc.Title), oneLine(plain(sc.Story)))
		if sc.Answer != "" {
			fmt.Fprintf(&b, "**Answer:** %s\n\n", oneLine(plain(strings.TrimPrefix(sc.Answer, "The answer: "))))
		}
		for i, a := range alts {
			if i == maxQuestions {
				break
			}
			verdict := "it does not meet the goals"
			if a.Pass {
				verdict = "it meets the goals"
			}
			fmt.Fprintf(&b, "%d. Why not \"%s\"?\n   - Evidence: %.1f%% of requests succeed, p95 %.0f ms, $%.2f an hour; %s.\n   - Answer: %s\n", i+1, strings.ReplaceAll(oneLine(a.Label), "\"", "'"), a.SuccessPct, a.P95Ms, a.CostPerHour, verdict, oneLine(plain(a.Why)))
		}
	}
	if cards == 0 {
		return "", false
	}
	return b.String(), true
}

// oneLine puts a text on one line, so that it cannot break the structure of a card.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// plain turns the {{term|words}} markers of the challenge text into the words alone.
func plain(s string) string {
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			return s
		}
		inner := s[i+2 : i+j]
		if k := strings.Index(inner, "|"); k >= 0 {
			inner = inner[k+1:]
		}
		s = s[:i] + inner + s[i+j+2:]
	}
}
