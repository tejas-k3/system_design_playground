package engine

import (
	"reflect"
	"testing"
)

func feedDesign(strategy string, mean, skew, threshold float64, keyDist string, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 70.0, "workload": "feed", "keySpace": 5000.0, "keyDist": keyDist, "followersMean": mean, "followersSkew": skew, "timeoutMs": 10000.0}).
		node("a", KAppServer, Params{"threads": 64.0, "feedStrategy": strategy, "celebrityThreshold": threshold, "queueCap": 2000.0}).
		node("db", KDatabase, Params{"connections": 16.0, "queueCap": 2000.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

func TestFollowerCountsAverageToTheMeanAndAreSkewed(t *testing.T) {
	const K = 10000
	sum, top := 0.0, 0.0
	for k := 1; k <= K; k++ {
		f := followerCount(k, K, 100, 1)
		sum += f
		if k <= K/100 {
			top += f
		}
	}
	if mean := sum / K; mean < 95 || mean > 105 {
		t.Fatalf("the mean follower count should be the setting (100), got %.1f", mean)
	}
	if share := top / sum; share < 0.4 {
		t.Fatalf("with Zipf skew 1 the top 1%% of authors hold about half of all follows, got %.2f", share)
	}
	if followerCount(1, K, 100, 0) != followerCount(K, K, 100, 0) {
		t.Fatal("skew 0 means every author has the same number of followers")
	}
}

// A post in push-on-write mode inserts into every follower's timeline: the writes per post equal the author's followers.
func TestPushOnWriteAmplifiesEachPostByTheFollowerCount(t *testing.T) {
	// skew 0: every author has exactly the mean. (With skew the average over a few hundred posts is dominated by
	// whether a celebrity happened to post, so it is far too noisy to check; the distribution has its own test.)
	r := run(feedDesign("pushOnWrite", 100, 0, 1000, "uniform", 60), Options{Seed: 3}, 60).RunSnapshot()
	posts := float64(r.OK+r.Failed) * 0.3
	per := float64(r.FanoutWrites) / posts
	if per < 90 || per > 110 {
		t.Fatalf("100 timeline writes per post expected, got %.1f (%d writes, %.0f posts)", per, r.FanoutWrites, posts)
	}
}

func TestPullOnReadMovesTheCostToReaders(t *testing.T) {
	push := run(feedDesign("pushOnWrite", 100, 1, 1000, "uniform", 60), Options{Seed: 3}, 60).RunSnapshot()
	pull := run(feedDesign("pullOnRead", 100, 1, 1000, "uniform", 60), Options{Seed: 3}, 60).RunSnapshot()
	if pull.FanoutWrites != 0 || pull.FanoutReads == 0 {
		t.Fatalf("pull never pushes and reads from the people followed: writes %d reads %d", pull.FanoutWrites, pull.FanoutReads)
	}
	if push.FanoutReads != 0 {
		t.Fatalf("push reads one timeline: %d", push.FanoutReads)
	}
}

// With a few authors holding most followers, pushing a celebrity's post floods the database; the hybrid pulls
// celebrities instead and the spike disappears.
func TestHybridRemovesTheCelebritySpike(t *testing.T) {
	push := run(feedDesign("pushOnWrite", 50, 1.2, 2000, "zipf", 40), Options{Seed: 3}, 60).RunSnapshot()
	hyb := run(feedDesign("hybrid", 50, 1.2, 2000, "zipf", 40), Options{Seed: 3}, 60).RunSnapshot()
	t.Logf("push: p95 %.0f ok %d of %d; hybrid: p95 %.0f ok %d of %d", push.P95, push.OK, push.OK+push.Failed, hyb.P95, hyb.OK, hyb.OK+hyb.Failed)
	if hyb.P95 > push.P95/3 {
		t.Fatalf("hybrid should cut the p95 several-fold: push %.0f, hybrid %.0f", push.P95, hyb.P95)
	}
	if float64(hyb.OK) < 0.97*float64(hyb.OK+hyb.Failed) {
		t.Fatalf("hybrid should keep serving: %d of %d", hyb.OK, hyb.OK+hyb.Failed)
	}
}

func TestOtherWorkloadsAreUntouchedByFeedSettings(t *testing.T) {
	plain := &builder{}
	plain.node("u", KClient, Params{"rps": 60.0}).node("a", KAppServer, Params{"threads": 16.0}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	withFeed := &builder{}
	withFeed.node("u", KClient, Params{"rps": 60.0, "followersMean": 500.0}).node("a", KAppServer, Params{"threads": 16.0, "feedStrategy": "hybrid"}).node("db", KDatabase, nil).edge("u", "a").edge("a", "db")
	d1, _ := Sanitize(plain.d)
	d2, _ := Sanitize(withFeed.d)
	if !reflect.DeepEqual(run(d1, Options{Seed: 3}, 20).RunSnapshot(), run(d2, Options{Seed: 3}, 20).RunSnapshot()) {
		t.Fatal("feed settings only matter for the feed workload")
	}
}
