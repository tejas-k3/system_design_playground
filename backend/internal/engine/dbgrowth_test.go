package engine

import (
	"math"
	"testing"
)

func growthStats(db Params) map[string]float64 {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 20.0})
	b.node("a", KAppServer, Params{"threads": 16.0})
	b.node("db", KDatabase, db)
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	s := New(d, Options{Seed: 1})
	s.Advance(3000)
	return s.Snapshot().Nodes["db"].Extra
}

// 100 GB growing 10 GB a day at $0.10 per GB-month: the next year's average size is 100 + 10 x 365 / 2 = 1,925 GB,
// which costs 1,925 x 0.10 x 12 = $2,310.
func TestDatabaseGrowthProjectsAYearOfStorageCost(t *testing.T) {
	ex := growthStats(Params{"dataGB": 100.0, "growthGBPerDay": 10.0, "storagePerGBMonth": 0.10, "bufferPoolGB": 200.0})
	if got := ex["yearStorageCost"]; math.Abs(got-2310) > 1 {
		t.Errorf("year storage cost should be $2,310, got %.2f", got)
	}
	if ex["storedGB"] != 100 {
		t.Errorf("stored GB: %.0f", ex["storedGB"])
	}
	// the data outgrows the 200 GB buffer pool after (200 - 100) / 10 = 10 days: reads then start going to disk
	if got := ex["daysUntilBufferPoolFull"]; math.Abs(got-10) > 0.01 {
		t.Errorf("days until the data no longer fits in memory should be 10, got %.2f", got)
	}
}

func TestNoGrowthNoProjection(t *testing.T) {
	ex := growthStats(Params{"dataGB": 100.0})
	if _, ok := ex["yearStorageCost"]; ok && ex["yearStorageCost"] != 100*0.115*12 {
		t.Errorf("without growth the year costs the current size only: %.2f", ex["yearStorageCost"])
	}
	if _, ok := ex["daysUntilBufferPoolFull"]; ok {
		t.Errorf("no growth means it never outgrows memory")
	}
}

func TestADatabaseWithoutDataShowsNothing(t *testing.T) {
	ex := growthStats(nil)
	if _, ok := ex["yearStorageCost"]; ok {
		t.Errorf("a database with no data size set has no storage figures: %v", ex)
	}
}
