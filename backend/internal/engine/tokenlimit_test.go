package engine

import "testing"

// A limiter that counts tokens, not requests: every request carries 500 prompt tokens (a share of them 20 times as
// many) and the limiter holds a per-minute allowance of tokens, refilled steadily, with a burst of one minute.
func tokenLimitRun(lim Params, bigPct float64) RunStats {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0, "readPct": 0.0, "keySpace": 1000.0, "keyDist": "uniform", "timeoutMs": 2000.0,
		"requestTokens": 500.0, "bigRequestPct": bigPct, "retries": 0.0})
	lp := Params{}
	for k, v := range lim {
		lp[k] = v
	}
	b.node("l", KRateLimiter, lp)
	b.node("a", KAppServer, Params{"threads": 32.0})
	b.node("db", KDatabase, Params{"connections": 32.0})
	b.edge("u", "l").edge("l", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 14})
	s.Advance(600000)
	return s.RunSnapshot()
}

func TestATokenLimiterAdmitsByWeight(t *testing.T) {
	// 20 requests a second of 500 tokens is 10,000 tokens a second; over 600 s a limit of 900 a second (54,000 a minute)
	// lets through the one-minute burst plus 900 x 600: (54,000 + 540,000) / 6,000,000 = 9.9% of the tokens
	same := tokenLimitRun(Params{"unit": "tokens", "limitTokensPerMin": 54000.0}, 0)
	// by requests, the same 20 a second at a limit of 18 a second admits 90%
	byReq := tokenLimitRun(Params{"limitRps": 18.0, "burst": 5.0}, 0)
	t.Logf("limit of 54,000 tokens/min on 10,000 tokens/s: %d of %d admitted (%.1f%%); 18 requests/s limit: %.1f%%",
		same.OK, same.OK+same.Failed, 100*float64(same.OK)/float64(same.OK+same.Failed), 100*float64(byReq.OK)/float64(byReq.OK+byReq.Failed))
	share := float64(same.OK) / float64(same.OK+same.Failed)
	if share < 0.09 || share > 0.11 {
		t.Errorf("the burst plus 900 tokens a second is 9.9%% of the tokens: %.3f", share)
	}
}

func TestAFewHugeRequestsExhaustATokenAllowance(t *testing.T) {
	// 20 requests a second of 500 tokens is 10,000 tokens a second, under an allowance of 750,000 a minute (12,500 a
	// second). One request in twenty carrying 20 times as much (10,000 tokens) makes the mean 975 tokens and the rate
	// 19,500 a second: the huge ones are 5% of the requests but half of the tokens, and the allowance runs out.
	huge := tokenLimitRun(Params{"unit": "tokens", "limitTokensPerMin": 750000.0}, 5)
	small := tokenLimitRun(Params{"unit": "tokens", "limitTokensPerMin": 750000.0}, 0)
	t.Logf("allowance 12,500 tokens/s: all requests small, %d refused; 5%% huge, %d refused (of %d)", small.Failed, huge.Failed, huge.OK+huge.Failed)
	if small.Failed != 0 {
		t.Errorf("10,000 tokens a second fits an allowance of 12,500: %d refused", small.Failed)
	}
	// Over 600 s 11,700,000 tokens are asked for and 8,250,000 are allowed (the burst plus 12,500 a second): 3,450,000
	// too many. A small request (500 tokens) almost always finds that much in the bucket; a huge one (10,000) mostly
	// does not, so the refusals are nearly all huge requests: about 3,450,000 / 10,000 = 345 of the 600 or so that came.
	if huge.Failed < 300 || huge.Failed > 450 {
		t.Errorf("about 345 huge requests should be refused: %d", huge.Failed)
	}
	if share := float64(huge.OK) / float64(huge.OK+huge.Failed); share < 0.94 {
		t.Errorf("by count nearly everything gets through, because the refusals are the heavy ones: %.3f", share)
	}
}

func TestLimiterUnitDefaultsToRequests(t *testing.T) {
	a := tokenLimitRun(Params{"limitRps": 18.0, "burst": 5.0}, 0)
	b := tokenLimitRun(Params{"unit": "requests", "limitRps": 18.0, "burst": 5.0}, 0)
	if a.OK != b.OK {
		t.Errorf("requests is the default unit: %d vs %d", a.OK, b.OK)
	}
}
