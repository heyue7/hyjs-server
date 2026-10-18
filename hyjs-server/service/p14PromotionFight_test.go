package service

import (
	"context"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"

	"hyjs-server/db"
)

// These are pure arithmetic fixtures of previously accepted spends. They do
// not produce a battle result, grant authority, open a database, or run DDL.
func p14PromotionPointArithmeticRows() (*db.P14PromotionState, []db.P14PromotionFight) {
	state := &db.P14PromotionState{SeasonIndex: 1, ServerID: 1, PlayerID: 7, QualificationHash: "actual-qualification", Score: 1, Points: 20, LastGrantAt: 100, EligibleAt: 50, StageEnd: 100000}
	var rows []db.P14PromotionFight
	score := int64(1)
	for i := int64(0); i < 20; i++ {
		delta := int64(0)
		if i%3 == 0 {
			delta = 15
		}
		rows = append(rows, db.P14PromotionFight{SeasonIndex: state.SeasonIndex, ServerID: state.ServerID, PlayerID: state.PlayerID, QualificationHash: state.QualificationHash, MatchIndex: i + 1, GrantAt: 100, CreatedAt: 101 + i, PointsBefore: 20 - i, PointsAfter: 19 - i, ScoreBefore: score, ScoreAfter: score + delta, ScoreDelta: delta})
		score += delta
	}
	return state, rows
}

func TestP14PromotionFightNoonCapResetsSpentPointsAndRetainsRealScore(t *testing.T) {
	state, rows := p14PromotionPointArithmeticRows()
	points, score, err := p14PromotionFightTotals(state, rows)
	if err != nil || points != 0 || score != 106 {
		t.Fatal("first twenty actual spends", points, score, err)
	}
	state.LastGrantAt = 1000
	points, score, err = p14PromotionFightTotals(state, rows)
	if err != nil || points != 20 || score != 106 {
		t.Fatal("next actual noon must restore cap20 without erasing score", points, score, err)
	}
	rows = append(rows, db.P14PromotionFight{SeasonIndex: state.SeasonIndex, ServerID: state.ServerID, PlayerID: state.PlayerID, QualificationHash: state.QualificationHash, MatchIndex: 21, GrantAt: 1000, CreatedAt: 1001, PointsBefore: 20, PointsAfter: 19, ScoreBefore: 106, ScoreAfter: 121, ScoreDelta: 15})
	points, score, err = p14PromotionFightTotals(state, rows)
	if err != nil || points != 19 || score != 121 || state.Points != 20 || state.Score != 1 || rows[0].PointsAfter != 19 {
		t.Fatal("new spend must not rewrite grant-only legacy state or old spend", points, score, err)
	}
}

func TestP14PromotionFightNoonCannotHideMissingOrOverdrawnSpend(t *testing.T) {
	state, rows := p14PromotionPointArithmeticRows()
	state.LastGrantAt = 1000
	missing := append(append([]db.P14PromotionFight{}, rows[:7]...), rows[8:]...)
	if _, _, err := p14PromotionFightTotals(state, missing); err == nil {
		t.Fatal("later noon hid a missing accepted spend")
	}
	last := rows[len(rows)-1]
	last.MatchIndex++
	last.CreatedAt++
	last.PointsBefore, last.PointsAfter = 0, -1
	last.ScoreBefore, last.ScoreAfter, last.ScoreDelta = 106, 106, 0
	if _, _, err := p14PromotionFightTotals(state, append(rows, last)); err == nil {
		t.Fatal("old grant overdraft hidden by current noon")
	}
	wrongIdentity := append([]db.P14PromotionFight{}, rows...)
	wrongIdentity[0].ServerID = 2
	if _, _, err := p14PromotionFightTotals(state, wrongIdentity); err == nil {
		t.Fatal("another realm's same local player ID spent these points")
	}
}

func TestP14PromotionFightLocalScorePolicyRequiresExactPublishedRules(t *testing.T) {
	raw := `{"matchCost":1,"maxRound":40,"winScore":15,"lossScore":0,"minimumScore":1}`
	if _, err := p14PromotionFightDecode(raw, p6Digest([]byte(raw))); err != nil {
		t.Fatal("explicit local scoring policy", err)
	}
	for _, invalid := range []string{raw + `{}`, `{"matchCost":0,"maxRound":40,"winScore":15,"lossScore":0,"minimumScore":1}`, `{"matchCost":1,"maxRound":40,"winScore":15,"lossScore":0,"minimumScore":1,"privateOdds":1}`} {
		if _, err := p14PromotionFightDecode(invalid, p6Digest([]byte(invalid))); err == nil {
			t.Fatal("implicit or unrecognized local rule accepted", invalid)
		}
	}
}

// An unrelated request avoids DB work while checking the internal dispatch
// clock contract. No public request parameter is mapped to the context key.
func TestP14PromotionFightDispatchReadsInternalClockOnce(t *testing.T) {
	calls := 0
	ctx := context.WithValue(context.Background(), constUtil.P14CrossClockContextKey{}, func() time.Time { calls++; return time.UnixMilli(1700000000000) })
	if _, handled, err := (&GameService{}).DispatchP14Cross(ctx, 7, &pb.CSPlayer{}, 1); handled || err != nil || calls != 1 {
		t.Fatal("single internal clock read", handled, err, calls)
	}
	if _, handled, err := (&GameService{}).DispatchP14Cross(context.Background(), 7, &pb.CSPlayer{}, 1); handled || err != nil {
		t.Fatal("ordinary context", handled, err)
	}
}
