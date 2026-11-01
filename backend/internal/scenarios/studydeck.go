package scenarios

import (
	"fmt"
	"strings"
)

// maxQuestions is how many counter-questions one card carries; a longer list stops being a conversation.
const maxQuestions = 3

// StudyDeck is a printable deck for a study partner, one card for each challenge of a level that has stored alternatives:
// the prompt, the answer to look for, and up to three counter-questions ("why not ...?") each with the evidence the
// simulator measured for it. It reports false for a level that does not exist.
func StudyDeck(level string) (string, bool) {
	if !validLevel(level) {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Study deck: %s\n\nOne person reads a card, the other answers out loud, then you compare with the evidence. Every number was measured by the simulator.\n", strings.ToUpper(level))
	for _, sc := range All() {
		alts := Alternatives(sc.ID)
		if sc.Level != level || len(alts) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n**Prompt:** %s\n\n", sc.Title, plain(sc.Story))
		if sc.Answer != "" {
			fmt.Fprintf(&b, "**Answer:** %s\n\n", plain(strings.TrimPrefix(sc.Answer, "The answer: ")))
		}
		for i, a := range alts {
			if i == maxQuestions {
				break
			}
			verdict := "it does not meet the goals"
			if a.Pass {
				verdict = "it meets the goals"
			}
			fmt.Fprintf(&b, "%d. Why not \"%s\"?\n   - Evidence: %.1f%% of requests succeed, p95 %.0f ms, $%.2f an hour; %s.\n   - Answer: %s\n", i+1, a.Label, a.SuccessPct, a.P95Ms, a.CostPerHour, verdict, plain(a.Why))
		}
	}
	return b.String(), true
}

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
