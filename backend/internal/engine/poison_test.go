package engine

import (
	"reflect"
	"testing"
)

func poisonDesign(maxAttempts float64, deadLetter bool, poisonPct float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 0.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).
		node("q", KQueue, Params{"capacity": 5000.0, "maxAttempts": maxAttempts, "deadLetter": deadLetter}).
		node("w", KWorker, Params{"concurrency": 8.0, "processMs": 50.0, "poisonPct": poisonPct}).
		node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}

func jobs(d Diagram) *JobStats { return run(d, Options{Seed: 8}, 60).RunSnapshot().Jobs }

func TestPoisonJobsAreRetriedThenGivenUp(t *testing.T) {
	j := jobs(poisonDesign(3, false, 10))
	poison := j.Enqueued - j.Completed - j.Backlog
	if poison < j.Enqueued/12 || poison > j.Enqueued/8 {
		t.Fatalf("10%% of the jobs can never succeed: %d of %d did not complete", poison, j.Enqueued)
	}
	if j.Lost != poison {
		t.Fatalf("without a dead-letter queue the exhausted jobs are simply lost: lost %d of %d poison", j.Lost, poison)
	}
	// attempts 1 and 2 are followed by another try, attempt 3 is the last (a few jobs are mid-backoff when the run ends)
	if d := j.Retried - 2*poison; d < -8 || d > 8 {
		t.Fatalf("each poison job is retried twice with 3 attempts: %d retries for %d poison jobs", j.Retried, poison)
	}
}

func TestAMoreGenerousAttemptLimitMultipliesTheWastedWork(t *testing.T) {
	few, many := jobs(poisonDesign(3, false, 10)), jobs(poisonDesign(8, false, 10))
	if many.Retried < few.Retried*3 {
		t.Fatalf("8 attempts waste far more work than 3: %d vs %d retries", many.Retried, few.Retried)
	}
}

func TestADeadLetterQueueKeepsWhatWouldHaveBeenLost(t *testing.T) {
	j := jobs(poisonDesign(3, true, 10))
	if j.Lost != 0 || j.Dead == 0 {
		t.Fatalf("exhausted jobs are parked, not lost: lost %d dead %d", j.Lost, j.Dead)
	}
	plain := jobs(poisonDesign(3, false, 10))
	if j.Dead != plain.Lost {
		t.Fatalf("the same jobs end up there: dead %d, lost without a DLQ %d", j.Dead, plain.Lost)
	}
}

func TestDefaultsAreWhatTheyAlwaysWere(t *testing.T) {
	if !reflect.DeepEqual(run(poisonDesign(3, false, 0), Options{Seed: 8}, 30).RunSnapshot(), run(plainQueueDesign(), Options{Seed: 8}, 30).RunSnapshot()) {
		t.Fatal("maxAttempts 3, no dead-letter queue and no poison must equal a design that never mentions them")
	}
}

func plainQueueDesign() Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 40.0, "readPct": 0.0, "timeoutMs": 5000.0}).
		node("a", KAppServer, Params{"threads": 32.0}).node("q", KQueue, Params{"capacity": 5000.0}).
		node("w", KWorker, Params{"concurrency": 8.0, "processMs": 50.0}).node("db", KDatabase, nil)
	b.edge("u", "a").edge("a", "q").edge("q", "w").edge("w", "db")
	d, _ := Sanitize(b.d)
	return d
}
