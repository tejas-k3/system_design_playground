package engine

import (
	"reflect"
	"testing"
)

// A legitimate crowd that the cache serves well, plus a scraper that asks for one new item after another.
func botDesign(botRps float64, limit float64) Diagram {
	b := &builder{}
	b.node("users", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0}).
		node("bot", KClient, Params{"rps": botRps, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "retries": 2.0, "abuse": botRps > 0}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 500.0}).
		node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true}).
		node("db", KDatabase, Params{"connections": 16.0, "readMs": 20.0, "queueCap": 500.0})
	if limit > 0 {
		b.node("rl", KRateLimiter, Params{"limitRps": limit})
		b.edge("users", "rl").edge("bot", "rl").edge("rl", "a")
	} else {
		b.edge("users", "a").edge("bot", "a")
	}
	b.edge("a", "c").edge("c", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestAScraperAsksForItemsNobodyHasCachedAndLoadsTheDatabase(t *testing.T) {
	quiet := run(botDesign(0, 0), Options{Seed: 5}, 40)
	bot := run(botDesign(800, 0), Options{Seed: 5}, 40)
	hq, hb := quiet.Snapshot().Nodes["c"].HitRate, bot.Snapshot().Nodes["c"].HitRate
	arr := bot.Snapshot().Nodes["db"].Arr
	t.Logf("cache hit rate %.3f without the bot, %.3f with it; database arrivals %.0f/s; p95 %.0f vs %.0f ms", hq, hb, arr, quiet.RunSnapshot().P95, bot.RunSnapshot().P95)
	if hq < 0.95 || hb > 0.2 {
		t.Fatalf("the crowd hits the cache, the bot's never-seen items do not: %.3f without, %.3f with", hq, hb)
	}
	if arr < 700 {
		t.Fatalf("almost every bot request reaches the database (about 800/s offered): %.0f/s", arr)
	}
	if bot.RunSnapshot().P95 < quiet.RunSnapshot().P95*4 {
		t.Fatalf("the saturated database slows everybody down: p95 %.0f vs %.0f", bot.RunSnapshot().P95, quiet.RunSnapshot().P95)
	}
}

// A rate limiter in front of everything protects the database, but it cannot tell the bot from the customers.
func TestAGlobalRateLimiterProtectsTheDatabaseButTurnsAwayRealUsersToo(t *testing.T) {
	open := run(botDesign(800, 0), Options{Seed: 5}, 40)
	limited := run(botDesign(800, 300), Options{Seed: 5}, 40)
	r := limited.RunSnapshot()
	if limited.Snapshot().Nodes["db"].Arr > open.Snapshot().Nodes["db"].Arr*0.5 {
		t.Fatalf("the limiter should cut the database load by more than half: %.0f vs %.0f", limited.Snapshot().Nodes["db"].Arr, open.Snapshot().Nodes["db"].Arr)
	}
	if r.P95 > 250 {
		t.Fatalf("what gets through is served quickly: p95 %.0f", r.P95)
	}
	if r.Errors["rateLimited"] < 10000 {
		t.Fatalf("most of the offered traffic is turned away (and the customers are in it): %d rejected", r.Errors["rateLimited"])
	}
}

func TestAbuseFlagOffChangesNothing(t *testing.T) {
	plain := &builder{}
	plain.node("users", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 500.0}).node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true}).
		node("db", KDatabase, Params{"connections": 16.0, "readMs": 20.0, "queueCap": 500.0})
	plain.edge("users", "a").edge("a", "c").edge("c", "db")
	d, _ := Sanitize(plain.d)
	withOff := &builder{}
	withOff.node("users", KClient, Params{"rps": 100.0, "readPct": 100.0, "keySpace": 200.0, "keyDist": "zipf", "timeoutMs": 3000.0, "abuse": false}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 500.0}).node("c", KCache, Params{"capacityItems": 1000.0, "ttlSec": 0.0, "startWarm": true}).
		node("db", KDatabase, Params{"connections": 16.0, "readMs": 20.0, "queueCap": 500.0})
	withOff.edge("users", "a").edge("a", "c").edge("c", "db")
	d2, _ := Sanitize(withOff.d)
	if !reflect.DeepEqual(run(d, Options{Seed: 5}, 20).RunSnapshot(), run(d2, Options{Seed: 5}, 20).RunSnapshot()) {
		t.Fatal("abuse false must change nothing")
	}
}
