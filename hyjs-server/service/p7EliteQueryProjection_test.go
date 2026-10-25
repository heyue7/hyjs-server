package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP7EliteQueryProjectionContinuousSourcesAndBoundedReads(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	for floor := int32(1); floor <= 3; floor++ {
		p7SectionPlayFloor(t, f, floor, 1, 72200+int64(floor))
	}
	var state db.P7RushState
	var elite db.P7RushStageRule
	if err := db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&elite, "id=?", "elite:01").Error; err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	name := "p7-elite-projection-count"
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) { counts[tx.Statement.Table]++ }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Query().Remove(name) })
	for i := 0; i < 3; i++ {
		if got, err := f.s.p7StagePrerequisite(db.GetIns(), f.id, elite); err != nil || !got {
			t.Fatal("original qualification", got, err)
		}
	}
	originalSources := counts["p6_battle_records"]
	counts = map[string]int{}
	ctx := p7WithRushHistoricalQuery(p7WithRushReadRequestCache(f.ctx))
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q, e := p7NewEliteQueryProjection(tx, &state)
		if e != nil {
			return e
		}
		for i := 0; i < 3; i++ {
			got, e := q.prerequisite(f.s, elite)
			if e != nil || !got {
				t.Fatalf("complete same-run source: %v %v", got, e)
			}
		}
		if q.verified != 3 {
			t.Fatalf("source validated %d times, want 3", q.verified)
		}
		if originalSources <= counts["p6_battle_records"] {
			t.Fatalf("source reads did not improve: original=%d projection=%d", originalSources, counts["p6_battle_records"])
		}

		for _, table := range []string{"p7_rush_progress", "p7_rush_fights", "p7_rush_manual_calculations", "p7_rush_scoped_calculations", "p6_battle_records", "p7_re_rush_sweeps"} {
			if counts[table] != 1 {
				t.Fatalf("%s read %d times, want one bulk read", table, counts[table])
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var first db.P7RushFight
	if err := db.GetIns().Where("player_id=? AND task_id=?", f.id, "local-section-1").First(&first).Error; err != nil {
		t.Fatal(err)
	}
	check := func(want bool, wantErr bool) {
		t.Helper()
		q, e := p7NewEliteQueryProjection(db.GetIns(), &state)
		if e != nil {
			t.Fatal(e)
		}
		got, e := q.prerequisite(f.s, elite)
		if got != want || (e != nil) != wantErr {
			t.Fatalf("got=%v err=%v", got, e)
		}
	}
	if err := db.GetIns().Model(&first).Update("run_id", first.RunID+1).Error; err != nil {
		t.Fatal(err)
	}
	check(false, false)
	if err := db.GetIns().Model(&first).Update("run_id", state.RunID).Error; err != nil {
		t.Fatal(err)
	}
	check(true, false)
	if err := db.GetIns().Model(&first).Update("diagnostic", true).Error; err != nil {
		t.Fatal(err)
	}
	check(false, false)
	if err := db.GetIns().Model(&first).Update("diagnostic", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", first.ReplayID).Update("authoritative", false).Error; err != nil {
		t.Fatal(err)
	}
	check(false, true)
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", first.ReplayID).Update("authoritative", true).Error; err != nil {
		t.Fatal(err)
	}
	before, beforeGame := f.profile(t)
	facts := p1Count(t, &db.P7RushProgress{}, "player_id=?", f.id)
	fights := p1Count(t, &db.P7RushFight{}, "player_id=?", f.id)
	if _, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, beforeGame, afterGame) || facts != p1Count(t, &db.P7RushProgress{}, "player_id=?", f.id) || fights != p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) {
		t.Fatal("directory query changed facts or rewards")
	}
}

// The sweep is produced by the original Start/End/reset/sweep services from an
// actual frozen roster and original engine result, never by inserting a victory.
func TestP7EliteQueryProjectionReSweepSourcesAndLazyHighFloor(t *testing.T) {
	f := p7PartyBusinessFixture(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local_rush_1"), Star: proto.Int32(1)}, 72301, false)
	if err != nil {
		t.Fatal(err)
	}
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil || client.Outcome.Winner != 0 || client.Outcome.Authoritative {
		t.Fatal("actual finite roster must win independently", err)
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: client.OriginalOperates}
	if _, err := f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 72302); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushToTop{}, 72303); err != nil {
		t.Fatal(err)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	check := func(wantErr bool) {
		t.Helper()
		q, e := p7NewEliteQueryProjection(db.GetIns(), state)
		if e != nil {
			t.Fatal(e)
		}
		if e = q.load(); e != nil {
			t.Fatal(e)
		}
		wins, e := q.runWins(1)
		if (e != nil) != wantErr || !wantErr && (len(wins) != 1 || wins[1].Fight != start.GetFightId()) {
			t.Fatalf("sweep provenance: %v %v", wins, e)
		}
	}
	check(false)
	if err := db.GetIns().Model(&db.P7ReRushSweep{}).Where("player_id=? AND run_id=? AND floor=1", f.id, state.RunID).Update("source_hash", "wrong").Error; err != nil {
		t.Fatal(err)
	}
	check(true)
	// A bad higher-floor row is outside the floor-one query and must stay lazy.
	q, err := p7NewEliteQueryProjection(db.GetIns(), state)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.load(); err != nil {
		t.Fatal(err)
	}
	q.sweeps = nil
	q.fights = append(q.fights, db.P7RushFight{ID: "unneeded-high-floor", PlayerID: f.id, TaskID: "local_rush_99", RunID: state.RunID, Rule: []byte("bad")})
	wins, err := q.acceptedWins(0, 1, 1, nil)
	if err != nil || len(wins) != 1 {
		t.Fatal("irrelevant high floor changed lower proof", err)
	}
}
