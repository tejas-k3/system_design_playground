package engine

import (
	"strings"
	"testing"
)

// purposeProblem says what is wrong with a component's purpose line, or "" when it is fine. The library shows
// the line on hover and focus and a learner reads it before using the component, so a missing one or a
// paragraph would leave a gap there.
func purposeProblem(sp NodeSpec) string {
	d := sp.Description
	switch {
	case d == "" || strings.TrimSpace(d) != d:
		return "missing or has stray spaces"
	case len(d) < 20 || len(d) > 240:
		return "should be 20 to 240 characters"
	case !strings.HasSuffix(d, "."):
		return "should be a sentence ending in a full stop"
	case strings.ContainsAny(d, "\n\t"):
		return "must be a single line"
	case sp.Lesson == "":
		return "has no lesson line"
	}
	return ""
}

func TestEveryComponentHasAOneLinePurpose(t *testing.T) {
	for _, sp := range Specs() {
		if p := purposeProblem(sp); p != "" {
			t.Errorf("%s: purpose line %s: %q", sp.Kind, p, sp.Description)
		}
	}
}

func TestThePurposeCheckCatchesTheGaps(t *testing.T) {
	ok := NodeSpec{Description: "Buffers work so servers can answer fast and workers catch up later.", Lesson: "A queue turns a spike into a backlog."}
	if p := purposeProblem(ok); p != "" {
		t.Fatalf("a good line was refused: %s", p)
	}
	for name, mutate := range map[string]func(*NodeSpec){
		"missing":      func(s *NodeSpec) { s.Description = "" },
		"stray space":  func(s *NodeSpec) { s.Description += " " },
		"too short":    func(s *NodeSpec) { s.Description = "A box." },
		"a paragraph":  func(s *NodeSpec) { s.Description = strings.Repeat("It does a thing. ", 20) },
		"no full stop": func(s *NodeSpec) { s.Description = "Buffers work so servers can answer fast" },
		"two lines":    func(s *NodeSpec) { s.Description = "Buffers work so servers\ncan answer fast and catch up." },
		"no lesson":    func(s *NodeSpec) { s.Lesson = "" },
	} {
		bad := ok
		mutate(&bad)
		if purposeProblem(bad) == "" {
			t.Errorf("%s: the check let it through", name)
		}
	}
}
