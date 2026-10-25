package engine

import (
	"math"
	"reflect"
	"testing"
)

// users -> app (network card of nicMbps) -> db. Everything the app sends (its requests to the database and its
// answers to the users) leaves through the one card.
func nicDesign(nicMbps, reqKB, respKB, rps float64) Diagram {
	b := &builder{}
	b.node("u", KClient, Params{"rps": rps, "readPct": 100.0, "reqKB": reqKB, "respKB": respKB, "timeoutMs": 60000.0, "retries": 0.0}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 5000.0, "nicMbps": nicMbps}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	return d
}

// 100 KB answers through an 80 Mbit/s card: 80e6 / 8 / 100,000 = 100 a second, however many are asked for (130 offered, so the queue grows and answers are late; counted over 20 s).
func TestACardCapsWhatAServerCanSend(t *testing.T) {
	r := run(nicDesign(80, 0, 100, 130), Options{Seed: 3}, 20).RunSnapshot()
	rate := float64(r.OK) / 20
	t.Logf("completed %.1f a second through an 80 Mbit/s card (theory 100)", rate)
	if math.Abs(rate-100)/100 > 0.05 {
		t.Fatalf("expected 100 a second, got %.1f", rate)
	}
}

// the card is shared by everything that leaves: 40 KB to the database plus 60 KB to the user is 100 KB a request
func TestTheCardIsSharedByRequestsAndAnswers(t *testing.T) {
	r := run(nicDesign(80, 40, 60, 130), Options{Seed: 3}, 20).RunSnapshot()
	rate := float64(r.OK) / 20
	t.Logf("completed %.1f a second (theory 100)", rate)
	if math.Abs(rate-100)/100 > 0.05 {
		t.Fatalf("expected 100 a second, got %.1f", rate)
	}
}

func TestACardBelowTheLoadChangesNothingButTheTransmitTime(t *testing.T) {
	base := run(nicDesign(0, 0, 125, 1), Options{Seed: 9}, 30).Snapshot().Global.Run.P50
	slow := run(nicDesign(10, 0, 125, 1), Options{Seed: 9}, 30).Snapshot().Global.Run.P50
	want := 125000.0 * 8 / 10e6 * 1000 // 100 ms
	if got := slow - base; math.Abs(got-want)/want > 0.05 {
		t.Fatalf("125 KB through a 10 Mbit/s card adds %.0f ms, want %.0f", got, want)
	}
}

func TestNoCardChangesNothing(t *testing.T) {
	b := &builder{}
	b.node("u", KClient, Params{"rps": 300.0, "readPct": 100.0, "reqKB": 0.0, "respKB": 100.0, "timeoutMs": 60000.0, "retries": 0.0}).
		node("a", KAppServer, Params{"threads": 64.0, "queueCap": 5000.0}).
		node("db", KDatabase, Params{"connections": 64.0})
	b.edge("u", "a").edge("a", "db")
	d, _ := Sanitize(b.d)
	if !reflect.DeepEqual(run(nicDesign(0, 0, 100, 300), Options{Seed: 3}, 20).RunSnapshot(), run(d, Options{Seed: 3}, 20).RunSnapshot()) {
		t.Fatal("nicMbps 0 must equal a design that never mentions it")
	}
}
