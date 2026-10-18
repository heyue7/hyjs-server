package service

import (
	"encoding/json"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// These fixtures validate the projection/version arithmetic only. They never
// sign a real peer, create a qualifier, accept a battle or access any database.
func p14PromotionRankPureSnapshot() (*p14PromotionRankSnapshot, *db.P14AuditionQualification) {
	ids, _ := json.Marshal([]int64{1, 2})
	q := &db.P14AuditionQualification{SeasonIndex: 7, ServerID: 1, SourceHash: "qualified-pure-arithmetic", QualifiedAt: 10, Players: ids}
	offer := db.P14PromotionRankOffer{SeasonIndex: 7, ServerID: 1, Version: "local-test-published-v1", Source: constUtil.P14PromotionRankSource, RulesJSON: constUtil.P14PromotionRankRules, RulesHash: p6Digest([]byte(constUtil.P14PromotionRankRules)), PeerServersJSON: "[1,2]", CohortHash: p6Digest([]byte("[1,2]")), StageStart: 20, StageEnd: 200, PublishedAt: 5, FrozenAt: 15}
	offer.SourceHash = p14PromotionRankOfferHash(&offer)
	snapshot := &p14PromotionRankSnapshot{Offer: offer, QualificationHash: q.SourceHash, Revision: 1, ObservedAt: 30, Scores: []p14PromotionRankScore{{PlayerID: 1, Score: 1, LedgerHeadHash: p14PromotionRankInitialHead(q.SourceHash, 1)}, {PlayerID: 2, Score: 1, LedgerHeadHash: p14PromotionRankInitialHead(q.SourceHash, 2)}}}
	return snapshot, q
}
func TestP14PromotionRankFrozenLocalRuleRequiresEarlyPublicationAndRealCohort(t *testing.T) {
	snapshot, _ := p14PromotionRankPureSnapshot()
	if _, err := p14PromotionRankValidateOffer(&snapshot.Offer, time.UnixMilli(30)); err != nil {
		t.Fatal(err)
	}
	late := snapshot.Offer
	late.PublishedAt = 21
	late.FrozenAt = 22
	late.SourceHash = p14PromotionRankOfferHash(&late)
	if _, err := p14PromotionRankValidateOffer(&late, time.UnixMilli(30)); err == nil {
		t.Fatal("late tie-rule publication accepted")
	}
	for _, raw := range []string{"[1,1]", "[2,3]", "[1]", "[1,2]{}"} {
		changed := snapshot.Offer
		changed.PeerServersJSON = raw
		changed.CohortHash = p6Digest([]byte(raw))
		changed.SourceHash = p14PromotionRankOfferHash(&changed)
		if _, err := p14PromotionRankValidateOffer(&changed, time.UnixMilli(30)); err == nil {
			t.Fatal("invalid cohort", raw)
		}
	}
}
func TestP14PromotionRankOriginalThreeValueProjectionKeepsJointIdentity(t *testing.T) {
	rows := []p14PromotionRankProjection{{ServerID: 2, PlayerID: 1, Score: 16, Power: 400}, {ServerID: 1, PlayerID: 2, Score: 16, Power: 400}, {ServerID: 1, PlayerID: 1, Score: 16, Power: 400}, {ServerID: 1, PlayerID: 9, Score: 1, Power: 100000}}
	p14PromotionRankSort(rows)
	want := [][2]int64{{1, 1}, {1, 2}, {2, 1}, {1, 9}}
	for i, row := range rows {
		if row.ServerID != want[i][0] || row.PlayerID != want[i][1] {
			t.Fatal("same numeric IDs were aliased or unversioned order used", rows)
		}
	}
}
func TestP14PromotionRankInitialPointScoreAndSourceVersionCannotInventAcceptedWin(t *testing.T) {
	snapshot, q := p14PromotionRankPureSnapshot()
	if err := p14PromotionRankValidateScores(snapshot, q); err != nil {
		t.Fatal(err)
	}
	snapshot.Scores[0].Score = 16
	if p14PromotionRankValidateScores(snapshot, q) == nil {
		t.Fatal("initial qualification silently became a win")
	}
	snapshot.Scores[0].Score = 1
	snapshot.Scores[0].PlayerID = 3
	if p14PromotionRankValidateScores(snapshot, q) == nil {
		t.Fatal("outsider injected into true qualified cohort")
	}
	snapshot, q = p14PromotionRankPureSnapshot()
	snapshot.Closed = true
	if p14PromotionRankValidateScores(snapshot, q) == nil {
		t.Fatal("source sealed before actual phase7 end")
	}
}
func TestP14PromotionRankContiguousProgressAndFinalSealCannotBeReopened(t *testing.T) {
	previous, q := p14PromotionRankPureSnapshot()
	next := *previous
	next.Revision = 2
	next.PreviousHash = p6Digest([]byte("actual-prior-content-fixture"))
	next.ObservedAt = 201
	next.Closed = true
	if err := p14PromotionRankValidateScores(&next, q); err != nil {
		t.Fatal(err)
	}
	if err := p14PromotionRankValidateAdvance(previous, &next); err != nil {
		t.Fatal("close-only version", err)
	}
	same := *previous
	same.Revision = 2
	same.PreviousHash = next.PreviousHash
	same.ObservedAt = 40
	if p14PromotionRankValidateAdvance(previous, &same) == nil {
		t.Fatal("tick alone created a new score version")
	}
	reopen := next
	reopen.Revision = 3
	reopen.Closed = false
	reopen.ObservedAt = 202
	if p14PromotionRankValidateAdvance(&next, &reopen) == nil {
		t.Fatal("closed origin reopened")
	}
	gap := next
	gap.Revision = 3
	if p14PromotionRankValidateAdvance(previous, &gap) == nil {
		t.Fatal("missing version accepted")
	}
}

// Signed import and Final32 source reads share this qualification check. Delayed
// certificates remain historical facts without entering an earlier window.
func TestP14PromotionRankLateQualificationCannotBecomeClosedFinalSource(t *testing.T) {
	for _, at := range []int64{199, 200, 201} {
		snapshot, q := p14PromotionRankPureSnapshot()
		snapshot.ObservedAt = 202
		snapshot.Closed = true
		q.QualifiedAt = at
		before, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		err = p14PromotionRankValidateScores(snapshot, q)
		if (err == nil) != (at < snapshot.Offer.StageEnd) {
			t.Fatalf("qualification at %d, end %d: %v", at, snapshot.Offer.StageEnd, err)
		}
		after, err := json.Marshal(q)
		if err != nil || string(before) != string(after) {
			t.Fatal("historical qualification was changed", err)
		}
	}
}
