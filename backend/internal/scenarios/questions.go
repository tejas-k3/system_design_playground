package scenarios

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"sysdesign/internal/engine"
)

// Question modes beyond fixing a broken design. Each question is graded on the server and its answer key is
// never serialised, so nothing the client can fetch reveals it before the learner commits.
const (
	ModeEstimate = "estimate"
	ModeDiagnose = "diagnose"
	ModeTradeoff = "tradeoff"
	ModeReview   = "review"
	ModeMatch    = "match"
	ModeAsk      = "ask"
)

// Verdicts for one estimated number.
const (
	VerdictGood  = "good"  // within 30 % of the expected value
	VerdictClose = "close" // within a factor of two either way
	VerdictOff   = "off"
)

// EstimateField is one number the learner must produce. Label, Unit and Hint are shown with the question;
// Expected and Explain stay on the server and come back with the result.
type EstimateField struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Unit     string  `json:"unit"`
	Expected float64 `json:"-"`
	Explain  string  `json:"-"` // how the expected value follows from the facts
}

// Fact is one labelled number shown in the brief of a question.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// MatchItem is one need and the components that might meet it. The right option and the reason stay on the server.
type MatchItem struct {
	Need    string   `json:"need"`
	Options []string `json:"options"`
	answer  int
	why     string
}

// Question is one practice question as served to the learner; the answer and the explanations stay on the server.
type Question struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Mode       string          `json:"mode"`
	Difficulty string          `json:"difficulty"`
	Summary    string          `json:"summary"`
	Prompt     string          `json:"prompt"`
	Facts      []Fact          `json:"facts,omitempty"`
	Fields     []EstimateField `json:"fields,omitempty"`
	BuildHint  string          `json:"buildHint,omitempty"`  // estimate: how the numbers become a design, if the question can be built and tested
	Options    []string        `json:"options,omitempty"`    // diagnose: the candidate explanations
	Items      []MatchItem     `json:"items,omitempty"`      // match: several needs, each with its own options
	Asks       []AskOption     `json:"asks,omitempty"`       // ask: the clarifying questions on offer
	Picks      int             `json:"picks,omitempty"`      // ask: how many of them may be asked
	Symptoms   *Symptoms       `json:"symptoms,omitempty"`   // diagnose: what the learner gets to look at
	Candidates []Candidate     `json:"candidates,omitempty"` // tradeoff: designs to run and compare
	Diagram    *engine.Diagram `json:"diagram,omitempty"`    // review: the design to critique

	rule        func(RunSummary) bool // tradeoff: the stated requirement the right answer meets
	alsoPasses  []int                 // tradeoff: other candidates that meet it too, but lose on the stated tie-break
	build       *buildSpec            // estimate: turns the learner's numbers into a design and runs it
	defects     []Defect              // review: what is wrong with Diagram (never served)
	scenarioID  string                // tradeoff: the challenge whose traffic and failures every candidate faces
	source      diagSource            // where the symptoms come from, so a test can regenerate them
	answer      int                   // diagnose: index of the right option
	explanation string                // diagnose: why, shown only after answering
}

// Symptoms are what a monitoring page and one trace would show during an incident. They are generated from a
// seeded run of a real challenge's starting design (see generateSymptoms), never typed in.
type Symptoms struct {
	DurationSec float64              `json:"durationSec"`
	Series      []engine.SeriesPoint `json:"series"` // one point every 2 simulated seconds
	Errors      map[string]int       `json:"errors"` // failure counts by reason id
	Trace       *engine.TraceOut     `json:"trace"`  // the first failed request
}

type diagSource struct {
	ScenarioID  string
	Seed        uint32
	DurationSec float64
}

//go:embed fixtures/*.json
var fixtureFS embed.FS

func loadSymptoms(id string) *Symptoms {
	b, err := fixtureFS.ReadFile("fixtures/diagnose-" + id + ".json")
	if err != nil {
		return nil
	}
	var s Symptoms
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

// generateSymptoms runs a challenge's starting design with a seed and records what an operator would see.
func generateSymptoms(src diagSource) (*Symptoms, error) {
	sc := Get(src.ScenarioID)
	if sc == nil {
		return nil, fmt.Errorf("no scenario %q", src.ScenarioID)
	}
	sim := engine.New(sc.Diagram, sc.Options(src.Seed))
	sim.EnableTraces(true)
	for sim.Now() < src.DurationSec*1000 && !sim.Finished() {
		sim.Advance(100)
	}
	st := sim.Snapshot()
	out := &Symptoms{DurationSec: src.DurationSec, Errors: st.Global.Run.Errors}
	for i, p := range st.Series {
		if i%4 == 0 {
			out.Series = append(out.Series, p)
		}
	}
	if f := sim.Traces().Failed; len(f) > 0 {
		out.Trace = &f[0]
	}
	if out.Errors == nil {
		out.Errors = map[string]int{}
	}
	return out, nil
}

// ChoiceResult is the verdict on a multiple-choice answer.
type ChoiceResult struct {
	Correct bool   `json:"correct"`
	Answer  int    `json:"answer"`
	Explain string `json:"explain"`
	Stars   int    `json:"stars"`
}

// GradeChoice grades a multiple-choice diagnosis. The right answer is revealed with the result either way.
func (q *Question) GradeChoice(choice int) (ChoiceResult, error) {
	if q.Mode != ModeDiagnose && q.Mode != ModeTradeoff {
		return ChoiceResult{}, errors.New("not a multiple-choice question")
	}
	n := len(q.Options)
	if q.Mode == ModeTradeoff {
		n = len(q.Candidates)
	}
	if choice < 0 || choice >= n {
		return ChoiceResult{}, fmt.Errorf("choose one of the %d options", n)
	}
	r := ChoiceResult{Correct: choice == q.answer, Answer: q.answer, Explain: q.explanation}
	if r.Correct {
		r.Stars = 3
	}
	return r, nil
}

// MatchItemResult is the verdict on one need of a matching question.
type MatchItemResult struct {
	Correct bool   `json:"correct"`
	Answer  int    `json:"answer"`
	Explain string `json:"explain"`
}

// MatchResult is the verdict on a whole matching question.
type MatchResult struct {
	Items   []MatchItemResult `json:"items"`
	Correct int               `json:"correct"`
	Stars   int               `json:"stars"`
}

// GradeMatch grades one choice per item. Every item is graded, and the right option and its reason come back with it.
func (q *Question) GradeMatch(choices []int) (MatchResult, error) {
	if q.Mode != ModeMatch {
		return MatchResult{}, errors.New("not a matching question")
	}
	if len(choices) != len(q.Items) {
		return MatchResult{}, fmt.Errorf("answer all %d items", len(q.Items))
	}
	out := MatchResult{Items: make([]MatchItemResult, len(q.Items))}
	for i, it := range q.Items {
		if choices[i] < 0 || choices[i] >= len(it.Options) {
			return MatchResult{}, fmt.Errorf("item %d: choose one of the %d options", i+1, len(it.Options))
		}
		ok := choices[i] == it.answer
		out.Items[i] = MatchItemResult{Correct: ok, Answer: it.answer, Explain: it.why}
		if ok {
			out.Correct++
		}
	}
	out.Stars = matchStars(out.Correct, len(q.Items))
	return out, nil
}

// matchStars: three stars for all right, or for at most one miss when there are enough items for that to mean
// something; two for two thirds right; one for half.
func matchStars(correct, total int) int {
	switch {
	case total == 0:
		return 0
	case correct == total, total >= 3 && correct >= total-1:
		return 3
	case correct*3 >= total*2:
		return 2
	case correct*2 >= total:
		return 1
	}
	return 0
}

// FieldResult is the verdict on one number of an estimation question.
type FieldResult struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Unit     string  `json:"unit"`
	Given    float64 `json:"given"`
	Expected float64 `json:"expected"`
	Verdict  string  `json:"verdict"`
	Explain  string  `json:"explain"`
}

// EstimateResult is the verdict on a whole estimation question.
type EstimateResult struct {
	Fields []FieldResult `json:"fields"`
	Score  float64       `json:"score"` // good = 1, close = 0.5, off = 0, averaged
	Stars  int           `json:"stars"` // 3: everything good; 2: score >= 0.75; 1: score >= 0.5
}

// verdictFor grades one number: good within +/-30 %, close within a factor of two, off beyond or not a
// positive finite number.
func verdictFor(given, expected float64) string {
	if math.IsNaN(given) || math.IsInf(given, 0) || given <= 0 || expected <= 0 {
		return VerdictOff
	}
	switch r := given / expected; {
	case r >= 0.7 && r <= 1.3:
		return VerdictGood
	case r >= 0.5 && r <= 2:
		return VerdictClose
	}
	return VerdictOff
}

// GradeEstimate grades a complete set of answers. Missing and unknown fields are refused rather than
// guessed at, so a typo cannot quietly score as zero.
func (q *Question) GradeEstimate(answers map[string]float64) (EstimateResult, error) {
	if q.Mode != ModeEstimate {
		return EstimateResult{}, errors.New("not an estimate question")
	}
	known := map[string]bool{}
	for _, f := range q.Fields {
		known[f.Key] = true
		if _, ok := answers[f.Key]; !ok {
			return EstimateResult{}, fmt.Errorf("missing an answer for %q", f.Key)
		}
	}
	var extra []string
	for k := range answers {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		slices.Sort(extra)
		return EstimateResult{}, fmt.Errorf("unknown field %q", extra[0])
	}
	res := EstimateResult{}
	total, allGood := 0.0, true
	for _, f := range q.Fields {
		v := verdictFor(answers[f.Key], f.Expected)
		switch v {
		case VerdictGood:
			total++
		case VerdictClose:
			total += 0.5
			allGood = false
		default:
			allGood = false
		}
		res.Fields = append(res.Fields, FieldResult{Key: f.Key, Label: f.Label, Unit: f.Unit, Given: answers[f.Key], Expected: f.Expected, Verdict: v, Explain: f.Explain})
	}
	if n := len(q.Fields); n > 0 {
		res.Score = total / float64(n)
	}
	switch {
	case allGood:
		res.Stars = 3
	case res.Score >= 0.75:
		res.Stars = 2
	case res.Score >= 0.5:
		res.Stars = 1
	}
	return res, nil
}

func (c *catalog) registerQuestion(q *Question) {
	c.questions = append(c.questions, q)
	c.questionByID[q.ID] = q
}

// AllQuestions returns every question in registration order.
func AllQuestions() []*Question { return append([]*Question(nil), theCatalog().questions...) }

// Q returns a question by id, or nil.
func Q(id string) *Question { return theCatalog().questionByID[id] }

// Candidate is one design a trade-off question lets the learner run. Only its label and description are served;
// the diagram stays on the server.
type Candidate struct {
	Label   string `json:"label"`
	Detail  string `json:"detail"`
	diagram engine.Diagram
}

// RunSummary is what running a candidate shows: the numbers the choice turns on.
type RunSummary struct {
	Resolved     int     `json:"resolved"`
	Success      float64 `json:"success"` // share of requests that succeeded, 0..1
	P50          float64 `json:"p50"`
	P95          float64 `json:"p95"`
	P99          float64 `json:"p99"`
	CostPerHour  float64 `json:"costPerHour"`
	LostWrites   int     `json:"lostWrites"`
	StaleReadPct float64 `json:"staleReadPct"` // share of reads that were stale, 0..1
}

// RunCandidate simulates candidate i under the question's scenario (its traffic, failures and length) and
// summarises the run. The same seed always gives the same numbers.
func (q *Question) RunCandidate(i int, seed uint32) (RunSummary, error) {
	if q.Mode != ModeTradeoff {
		return RunSummary{}, errors.New("not a trade-off question")
	}
	if i < 0 || i >= len(q.Candidates) {
		return RunSummary{}, fmt.Errorf("choose one of the %d designs", len(q.Candidates))
	}
	sc := Get(q.scenarioID)
	if sc == nil {
		return RunSummary{}, fmt.Errorf("no scenario %q", q.scenarioID)
	}
	d := q.Candidates[i].diagram
	sim := engine.New(d, sc.Options(seed))
	limit := sc.DurationSec*1000 + 60000
	for !sim.Finished() && sim.Now() < limit {
		sim.Advance(100)
	}
	run := sim.RunSnapshot()
	out := RunSummary{Resolved: run.Resolved, P50: run.P50, P95: run.P95, P99: run.P99, CostPerHour: sim.CostPerHour(), LostWrites: run.LostWrites}
	if run.Resolved > 0 {
		out.Success = float64(run.OK) / float64(run.Resolved)
	}
	if reads := run.Fresh + run.Stale; reads > 0 {
		out.StaleReadPct = float64(run.Stale) / float64(reads)
	}
	return out, nil
}

// Defect is one thing wrong with a review design. Nodes are the components (or connection ids) that carry it;
// marking any one of them finds it. Rules names the design-lint rules that independently report the same flaw,
// which a test uses to keep the answer key honest.
type Defect struct {
	ID      string
	Nodes   []string
	Rules   []string
	Explain string
}

// DefectOut is one defect of a design under review, with the nodes it concerns.
type DefectOut struct {
	ID      string   `json:"id"`
	Nodes   []string `json:"nodes"`
	Explain string   `json:"explain"`
}

// ReviewResult is the verdict on a design review: the defects found and missed.
type ReviewResult struct {
	Found      []DefectOut `json:"found"`
	Missed     []DefectOut `json:"missed"`
	FalseMarks []string    `json:"falseMarks"` // marked components that carry no defect
	Precision  float64     `json:"precision"`  // marks on defective components / all marks
	Recall     float64     `json:"recall"`     // defects found / defects
	Score      float64     `json:"score"`      // (found - false marks) / defects, not below 0
	Stars      int         `json:"stars"`      // 3: all found, nothing false; 2: score >= 0.75; 1: >= 0.5
}

// GradeReview scores the components a learner marked as wrong. Unknown ids are refused; a repeated mark counts once.
func (q *Question) GradeReview(marked []string) (ReviewResult, error) {
	if q.Mode != ModeReview || q.Diagram == nil {
		return ReviewResult{}, errors.New("not a review question")
	}
	valid := map[string]bool{}
	for _, n := range q.Diagram.Nodes {
		valid[n.ID] = true
	}
	for _, e := range q.Diagram.Edges {
		valid[e.ID] = true
	}
	set := map[string]bool{}
	var order []string
	for _, m := range marked {
		if !valid[m] {
			return ReviewResult{}, fmt.Errorf("%q is not part of this design", m)
		}
		if !set[m] {
			set[m] = true
			order = append(order, m)
		}
	}
	carries := map[string]bool{}
	res := ReviewResult{Found: []DefectOut{}, Missed: []DefectOut{}, FalseMarks: []string{}}
	for _, d := range q.defects {
		out := DefectOut{ID: d.ID, Nodes: d.Nodes, Explain: d.Explain}
		hit := false
		for _, n := range d.Nodes {
			carries[n] = true
			if set[n] {
				hit = true
			}
		}
		if hit {
			res.Found = append(res.Found, out)
		} else {
			res.Missed = append(res.Missed, out)
		}
	}
	onDefect := 0
	for _, m := range order {
		if carries[m] {
			onDefect++
		} else {
			res.FalseMarks = append(res.FalseMarks, m)
		}
	}
	if len(order) > 0 {
		res.Precision = float64(onDefect) / float64(len(order))
	}
	total := float64(len(q.defects))
	if total > 0 {
		res.Recall = float64(len(res.Found)) / total
		res.Score = (float64(len(res.Found)) - float64(len(res.FalseMarks))) / total
		if res.Score < 0 {
			res.Score = 0
		}
	}
	switch {
	case len(res.Missed) == 0 && len(res.FalseMarks) == 0:
		res.Stars = 3
	case res.Score >= 0.75:
		res.Stars = 2
	case res.Score >= 0.5:
		res.Stars = 1
	}
	return res, nil
}

// buildSpec says how an estimate becomes a design: the number to size the fleet from, how much one server is assumed
// to handle, and the real load to test it against.
type buildSpec struct {
	field     string  // answer key whose value sizes the fleet
	perServer float64 // requests a second one app server is assumed to handle
	rps       float64 // the real peak load applied in the test
	readPct   float64
	seconds   float64
}

// The verdicts of a sizing question: fewer servers than needed, about right, or more than needed.
const (
	BuildUnder = "under"
	BuildRight = "right"
	BuildOver  = "over"
)

// BuildResult is the verdict on a sizing question: the servers chosen against the servers needed.
type BuildResult struct {
	Servers      int        `json:"servers"`
	RightServers int        `json:"rightServers"`
	Yours        RunSummary `json:"yours"`
	RightSized   RunSummary `json:"rightSized"`
	Verdict      string     `json:"verdict"` // under: it fails the load; over: it works but costs more than 1.5x the right-sized fleet
}

func (b *buildSpec) design(servers int) engine.Diagram {
	pp := P{"rps": b.rps, "readPct": b.readPct}
	d := (&bld{}).n("users", engine.KClient, cols[0], rowMid, pp).
		n("lb", engine.KLoadBalancer, cols[1], rowMid, nil).e("users", "lb").
		n("db", engine.KDatabase, cols[4], rowMid, P{"maxConnections": 1000.0, "connections": 48.0}).
		n("cache", engine.KCache, cols[3], rowTop, P{"ttlSec": 30.0, "ttlJitterPct": 20.0})
	return d.servers(servers, cols[2], rowTop, 70, P{"threads": 16.0, "callTimeoutMs": 400.0, "dbPool": 20.0}, "cache").done()
}

func (b *buildSpec) run(servers int, seed uint32) RunSummary {
	sim := engine.New(b.design(servers), engine.Options{Seed: seed, StopTrafficAtMs: b.seconds * 1000, WarmupMs: 3000})
	for !sim.Finished() && sim.Now() < b.seconds*1000+60000 {
		sim.Advance(100)
	}
	run := sim.RunSnapshot()
	out := RunSummary{Resolved: run.Resolved, P50: run.P50, P95: run.P95, P99: run.P99, CostPerHour: sim.CostPerHour()}
	if run.Resolved > 0 {
		out.Success = float64(run.OK) / float64(run.Resolved)
	}
	return out
}

// BuildAndTest builds a fleet sized from the learner's number (one server per perServer requests a second, between
// one and 32), runs it against the real peak load, and runs the fleet sized from the true value next to it.
func (q *Question) BuildAndTest(answers map[string]float64, seed uint32) (BuildResult, error) {
	b := q.build
	if q.Mode != ModeEstimate || b == nil {
		return BuildResult{}, errors.New("this question has nothing to build")
	}
	v, ok := answers[b.field]
	if !ok || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return BuildResult{}, fmt.Errorf("give a positive number for %q first", b.field)
	}
	size := func(x float64) int {
		n := int(math.Ceil(x / b.perServer))
		if n < 1 {
			n = 1
		}
		if n > 32 {
			n = 32
		}
		return n
	}
	var expected float64
	for _, f := range q.Fields {
		if f.Key == b.field {
			expected = f.Expected
		}
	}
	res := BuildResult{Servers: size(v), RightServers: size(expected)}
	res.Yours = b.run(res.Servers, seed)
	res.RightSized = b.run(res.RightServers, seed)
	switch {
	case res.Yours.Success < 0.97:
		res.Verdict = BuildUnder
	case res.Yours.CostPerHour > 1.5*res.RightSized.CostPerHour:
		res.Verdict = BuildOver
	default:
		res.Verdict = BuildRight
	}
	return res, nil
}
