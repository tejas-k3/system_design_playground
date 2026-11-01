package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"sysdesign/internal/engine"
)

// The characterisation snapshot is a stricter oracle than catalog.golden: it hashes the full Score JSON (every
// RunStats field) of every challenge's start and reference design on seeds 1-8, plus the reference under a fixed
// set of chaos events. It exists to prove that a structural refactor changed nothing. Regenerate with
// UPDATE_GOLDEN=1 only when a behaviour change is intended.

type charJob struct {
	label string
	d     engine.Diagram
	sc    *Scenario
	seed  uint32
	chaos []engine.ChaosEvent
}

func charChaos(d engine.Diagram) map[string][]engine.ChaosEvent {
	var ids []string
	for _, n := range d.Nodes {
		if n.Kind != engine.KClient {
			ids = append(ids, n.ID)
		}
	}
	out := map[string][]engine.ChaosEvent{}
	for i, id := range ids {
		if i >= 2 {
			break
		}
		out[fmt.Sprintf("crash:%s", id)] = []engine.ChaosEvent{{Type: "crash", NodeID: id, AtSec: 8, DownSec: 6}}
		out[fmt.Sprintf("slow:%s", id)] = []engine.ChaosEvent{{Type: "slow", NodeID: id, AtSec: 8, Factor: 4, DurationSec: 6}}
		out[fmt.Sprintf("partition:%s", id)] = []engine.ChaosEvent{{Type: "partition", NodeID: id, AtSec: 8, DurationSec: 6}}
	}
	return out
}

func characterisationLines(t *testing.T) []string {
	var jobs []charJob
	for _, sc := range All() {
		for seed := uint32(1); seed <= 8; seed++ {
			jobs = append(jobs, charJob{label: fmt.Sprintf("%s start seed=%d", sc.ID, seed), d: sc.Diagram, sc: sc, seed: seed})
			if len(sc.Solution.Nodes) > 0 {
				jobs = append(jobs, charJob{label: fmt.Sprintf("%s solution seed=%d", sc.ID, seed), d: sc.Solution, sc: sc, seed: seed})
			}
		}
		if len(sc.Solution.Nodes) > 0 {
			events := charChaos(sc.Solution)
			for _, name := range sortedKeys(events) {
				jobs = append(jobs, charJob{label: fmt.Sprintf("%s solution %s", sc.ID, name), d: sc.Solution, sc: sc, seed: 5, chaos: events[name]})
			}
		}
	}
	lines := make([]string, len(jobs))
	var wg sync.WaitGroup
	var next atomic.Int64
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(jobs) {
					return
				}
				j := jobs[i]
				opts := j.sc.Options(j.seed)
				if j.chaos != nil {
					opts.Chaos = append(append([]engine.ChaosEvent(nil), opts.Chaos...), j.chaos...)
				}
				b, err := roundedJSON(engine.RunToEnd(j.d, opts, j.sc.Goals))
				if err != nil {
					lines[i] = j.label + " marshal error: " + err.Error()
					continue
				}
				sum := sha256.Sum256(b)
				lines[i] = j.label + " " + hex.EncodeToString(sum[:8])
			}
		}()
	}
	wg.Wait()
	return lines
}

// roundedJSON marshals v with every float rounded to 5 significant digits, so the hash does not depend on the CPU
// architecture (arm64 fuses multiply-add, amd64 does not, which changes the last bits of a float).
func roundedJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return json.Marshal(roundFloats(tree))
}

func roundFloats(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			x[k] = roundFloats(c)
		}
	case []any:
		for i, c := range x {
			x[i] = roundFloats(c)
		}
	case float64:
		r, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 5, 64), 64)
		return r
	}
	return v
}

func sortedKeys(m map[string][]engine.ChaosEvent) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func TestCharacterisationSnapshot(t *testing.T) {
	got := strings.Join(characterisationLines(t), "\n") + "\n"
	const path = "testdata/characterisation.golden"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got == string(want) {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	shown := 0
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got  %s\n want %s", i+1, g, w)
			if shown++; shown >= 10 {
				break
			}
		}
	}
	t.Fatal("characterisation snapshot changed: a refactor must not change any run")
}
