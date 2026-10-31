package engine

import "testing"

// A group of members that vote. A member that stops answering is an omission or crash fault; one that answers wrongly
// is an arbitrary fault. Tolerating k crashes takes 2k+1 members; tolerating k members that lie takes 3k+1.
func voteRun(coord Params, chaos []ChaosEvent) RunStats {
	s := coordRun(coord, chaos, 30)
	return s.RunSnapshot()
}

func lies(members float64) []ChaosEvent {
	return []ChaosEvent{{Type: "memberLies", NodeID: "zk", AtSec: 5, DurationSec: 20, Factor: members}}
}

func TestAMemberThatLiesIsBelievedUnderTheCrashModel(t *testing.T) {
	clean := voteRun(Params{"nodes": 3.0}, nil)
	lying := voteRun(Params{"nodes": 3.0}, lies(1))
	t.Logf("no fault: %d wrong answers; one liar of 3: %d wrong, %d of %d ok", clean.WrongAnswers, lying.WrongAnswers, lying.OK, lying.Resolved)
	if clean.WrongAnswers != 0 {
		t.Errorf("no fault, no wrong answer: %d", clean.WrongAnswers)
	}
	if lying.WrongAnswers < 400 || lying.OK < lying.Resolved*9/10 {
		t.Errorf("the group trusts what it hears: requests succeed with wrong answers: %d wrong, %d ok of %d", lying.WrongAnswers, lying.OK, lying.Resolved)
	}
}

func TestVotingNeedsThreeKPlusOneMembersToOutvoteKLiars(t *testing.T) {
	arb := func(nodes float64, liars float64) RunStats {
		return voteRun(Params{"nodes": nodes, "faultModel": "arbitrary", "faultsTolerated": 1.0}, lies(liars))
	}
	four, three, two := arb(4, 1), arb(3, 1), arb(4, 2)
	t.Logf("4 members, 1 liar: %d wrong, %d ok of %d; 3 members, 1 liar: %d wrong, %d ok; 4 members, 2 liars: %d wrong, %d ok", four.WrongAnswers, four.OK, four.Resolved, three.WrongAnswers, three.OK, two.WrongAnswers, two.OK)
	if four.WrongAnswers != 0 || four.OK < four.Resolved*95/100 {
		t.Errorf("3k+1 members outvote k liars: %d wrong, %d ok of %d", four.WrongAnswers, four.OK, four.Resolved)
	}
	if three.WrongAnswers != 0 || three.OK > three.Resolved*60/100 {
		t.Errorf("one member short, the group cannot tell the liar from the truth and refuses: %d wrong, %d ok of %d", three.WrongAnswers, three.OK, three.Resolved)
	}
	if two.WrongAnswers == 0 {
		t.Errorf("more liars than the group is built for get through")
	}
}

func TestMoreMembersDoNotHelpWithoutVotingOnTheAnswer(t *testing.T) {
	five := voteRun(Params{"nodes": 5.0}, lies(1))
	if five.WrongAnswers < 400 {
		t.Errorf("five members under the crash model still believe the first answer: %d", five.WrongAnswers)
	}
}

func TestChecksumsCatchCorruptionButNotALieWithAValidChecksum(t *testing.T) {
	corrupt := []ChaosEvent{{Type: "memberLies", NodeID: "zk", AtSec: 5, DurationSec: 20, Factor: 1, Mode: "corrupt"}}
	plain := voteRun(Params{"nodes": 3.0}, corrupt)
	sums := voteRun(Params{"nodes": 3.0, "answerChecksums": true}, corrupt)
	lyingWithSums := voteRun(Params{"nodes": 3.0, "answerChecksums": true}, lies(1))
	t.Logf("corruption: %d wrong without checksums, %d with; a lie with checksums on: %d", plain.WrongAnswers, sums.WrongAnswers, lyingWithSums.WrongAnswers)
	if plain.WrongAnswers < 400 || sums.WrongAnswers != 0 {
		t.Errorf("a checksum shows a damaged answer, which is dropped like a missing one: %d against %d", plain.WrongAnswers, sums.WrongAnswers)
	}
	if lyingWithSums.WrongAnswers < 400 {
		t.Errorf("a lie arrives with a matching checksum: %d", lyingWithSums.WrongAnswers)
	}
}

func TestAMemberThatDropsMessagesLeavesTheGroupShortOfAnswers(t *testing.T) {
	mute := func(members float64, nodes float64) RunStats {
		return voteRun(Params{"nodes": nodes}, []ChaosEvent{{Type: "memberOmission", NodeID: "zk", AtSec: 5, DurationSec: 20, Factor: members}})
	}
	one, two := mute(1, 3), mute(2, 3)
	t.Logf("3 members, 1 mute: %d of %d ok; 2 mute: %d of %d ok", one.OK, one.Resolved, two.OK, two.Resolved)
	if one.OK < one.Resolved*95/100 {
		t.Errorf("a majority still answers: %d of %d", one.OK, one.Resolved)
	}
	if two.OK > two.Resolved*50/100 {
		t.Errorf("two silent members of three leave no majority for 20 of 30 seconds: %d of %d", two.OK, two.Resolved)
	}
}
