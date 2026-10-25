package engine

import (
	"math"
	"reflect"
	"testing"
)

// users -> app -> db with a limited link between users and app, response sizes fixed or lognormal.
func sizeDistDesign(dist string, sigma, mbps, respKB, rps float64) Diagram {
	b := &builder{}
	p := Params{"rps": rps, "readPct": 100.0, "respKB": respKB, "timeoutMs": 30000.0, "retries": 0.0}
	if dist != "" {
		p["sizeDist"] = dist
		p["sizeSigma"] = sigma
	}
	b.node("u", KClient, p).node("a", KAppServer, Params{"threads": 64.0, "queueCap": 5000.0}).node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	for i := range b.d.Edges {
		if b.d.Edges[i].ID == "u>a" {
			b.d.Edges[i].BandwidthMbps = mbps
		}
	}
	d, _ := Sanitize(b.d)
	return d
}

// The same mean size, a heavier tail: at 80% link utilisation the big responses make the ones behind them wait, so the
// tail of the latency is worse and so is the mean (the Pollaczek-Khinchine effect of a larger size variance).
func TestLognormalSizesGiveAHeavierTailAtTheSameMean(t *testing.T) {
	// 100 KB responses, 110 a second on 100 Mbit/s (8 ms each): mean utilisation 0.88
	fixed := run(sizeDistDesign("fixed", 0.5, 100, 100, 110), Options{Seed: 7}, 400).Snapshot().Global.Run
	logn := run(sizeDistDesign("lognormal", 0.5, 100, 100, 110), Options{Seed: 7}, 400).Snapshot().Global.Run
	// Pollaczek-Khinchine: the extra mean wait of a size spread with squared coefficient of variation c2 is
	// rho * c2 / (2 (1 - rho)) x the mean transmission time; c2 = exp(sigma^2) - 1 = 0.284 for sigma 0.5
	want := 0.88 * (math.Exp(0.25) - 1) / (2 * (1 - 0.88)) * 8
	t.Logf("fixed: mean %.1f p95 %.0f p99 %.0f ms; lognormal: mean %.1f p95 %.0f p99 %.0f ms; extra mean wait %.1f ms (P-K %.1f)", fixed.Mean, fixed.P95, fixed.P99, logn.Mean, logn.P95, logn.P99, logn.Mean-fixed.Mean, want)
	if extra := logn.Mean - fixed.Mean; math.Abs(extra-want) > 0.3*want { // a single 400 s run at 88% load: sampling noise of a few ms
		t.Errorf("extra mean wait %.1f ms, Pollaczek-Khinchine says %.1f", extra, want)
	}
	if !(logn.P99 > fixed.P99*1.05) {
		t.Fatalf("lognormal sizes should have a clearly worse p99: %.0f vs %.0f", logn.P99, fixed.P99)
	}
	if !(logn.Mean > fixed.Mean) {
		t.Fatalf("a larger size variance raises the mean wait too: %.0f vs %.0f", logn.Mean, fixed.Mean)
	}
}

// Below saturation the bytes carried follow the mean size: the distribution does not change the average load.
func TestLognormalSizesCarryTheSameMeanBytes(t *testing.T) {
	carried := func(dist string) float64 {
		s := run(sizeDistDesign(dist, 0.5, 2000, 100, 400), Options{Seed: 7}, 100)
		return s.Snapshot().Edges["u>a"].Mbps
	}
	f, l := carried("fixed"), carried("lognormal")
	t.Logf("carried: fixed %.1f Mbit/s, lognormal %.1f Mbit/s (400 x 100 KB = 320)", f, l)
	if math.Abs(f-320)/320 > 0.02 || math.Abs(l-320)/320 > 0.05 {
		t.Fatalf("the mean load is 320 Mbit/s either way: %.1f and %.1f", f, l)
	}
}

func TestFixedSizesNeedNoDrawAndChangeNothing(t *testing.T) {
	a := run(sizeDistDesign("", 0, 80, 100, 50), Options{Seed: 7}, 30).RunSnapshot()
	b := run(sizeDistDesign("fixed", 0.9, 80, 100, 50), Options{Seed: 7}, 30).RunSnapshot()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("sizeDist fixed (whatever the sigma) must equal a design that never mentions it")
	}
}

// the hash-based draw: the sample mean over 100,000 requests is the configured mean within 2%, the log has the
// configured sigma, and the same request always gets the same size.
func TestTheLognormalDrawHasTheConfiguredMean(t *testing.T) {
	for _, sigma := range []float64{0.5, 1.0} {
		sum, sumLog, sumLog2 := 0.0, 0.0, 0.0
		const n = 100000
		for id := 1; id <= n; id++ {
			f := sizeFactor(7, id, 1, sigma)
			if f != sizeFactor(7, id, 1, sigma) {
				t.Fatal("the same request must always get the same size")
			}
			sum += f
			l := math.Log(f)
			sumLog += l
			sumLog2 += l * l
		}
		mean := sum / n
		sd := math.Sqrt(sumLog2/n - (sumLog/n)*(sumLog/n))
		t.Logf("sigma %.1f: sample mean %.4f, sd of ln %.4f", sigma, mean, sd)
		tol := 0.02
		if sigma > 0.7 {
			tol = 0.03 // the sample mean of a fatter lognormal is noisier (about 0.5% sd at sigma 1)
		}
		if math.Abs(mean-1) > tol || math.Abs(sd-sigma)/sigma > 0.02 {
			t.Fatalf("sigma %.1f: mean %.4f, sd %.4f", sigma, mean, sd)
		}
	}
}

// 100 KB on average against a 256 KB message limit: P(size > 256 KB) = P(z > (ln 2.56 + sigma^2/2) / sigma) = 1.7% at sigma 0.5.
func TestSomeLognormalMessagesExceedTheQueueLimit(t *testing.T) {
	d := uploadFlow(100, 256, false)
	for i := range d.Nodes {
		if d.Nodes[i].ID == "u" {
			d.Nodes[i].Params["sizeDist"] = "lognormal"
			d.Nodes[i].Params["sizeSigma"] = 0.5
			d.Nodes[i].Params["rps"] = 200.0
		}
	}
	d, _ = Sanitize(d)
	r := run(d, Options{Seed: 5}, 100).Snapshot().Global.Run
	share := float64(r.Errors["messageTooLarge"]) / float64(r.Resolved)
	t.Logf("refused %d of %d = %.2f%% (theory 1.65%%)", r.Errors["messageTooLarge"], r.Resolved, share*100)
	if share < 0.0125 || share > 0.021 {
		t.Fatalf("expected about 1.7%% over the limit, got %.2f%%", share*100)
	}
}
