package service

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7ScopedFixture(t *testing.T, kind int32) (*p1Fixture, db.P7RushStageRule, *pb.CSRushFightStart) {
	t.Helper()
	f := p7Setup(t)
	t.Cleanup(func() { db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", false) })
	cfg := p7DiagnosticStage(t, f)
	if kind == constUtil.P7RushElite {
		cfg.Kind = kind
		cfg.Floor = 0
		cfg.Star = 0
	}
	cfg.Source = "prepublished isolated scoped-stage; original047 template; private server floor unknown"
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.formationList(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSRushFightStart{RushType: proto.Int32(kind), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(cfg.Star)}
	return f, cfg, request
}

func TestP7ScopedRushDefaultOffAndDiagnosticCannotSettle(t *testing.T) {
	f, _, request := p7ScopedFixture(t, constUtil.P7RushTrial)
	before, game := f.profile(t)
	if _, err := f.s.p7Start(f.ctx, f.id, request, 1400, false); err == nil {
		t.Fatal("default-off scope opened")
	}
	if p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 0 {
		t.Fatal("closed start left records")
	}
	after, again := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, game, again) {
		t.Fatal("closed start changed profile")
	}
	diagnostic, err := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, request, 1401)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	record, err := f.s.ComputeP7Diagnostic(f.ctx, f.id, diagnostic.GetFightId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p7End(f.ctx, f.id, &pb.CSRushFightEnd{FightId: diagnostic.FightId, Result: proto.Int32(1), OptRecord: record.Operates}); err == nil {
		t.Fatal("diagnostic promoted")
	}
	if p1Count(t, &db.P7RushScopedCalculation{}, "player_id=?", f.id) != 0 {
		t.Fatal("diagnostic acquired scoped proof")
	}
}

func TestP7ScopedRushActualPaidStartCompleteRecordEndAndColdOnce(t *testing.T) {
	for _, kind := range []int32{constUtil.P7RushTrial, constUtil.P7RushElite} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			f, cfg, request := p7ScopedFixture(t, kind)
			if err := db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			before, _ := f.profile(t)
			start, err := f.s.p7Start(f.ctx, f.id, request, 1410, false)
			if constUtil.P6RushStarterNativeProof == "" {
				if err == nil || p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 0 {
					t.Fatal("empty compiled proof opened")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var fight db.P7RushFight
			if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
				t.Fatal(err)
			}
			var row db.P7RushScopedCalculation
			if err = db.GetIns().First(&row, "fight_id=?", fight.ID).Error; err != nil {
				t.Fatal(err)
			}
			var terminal model.CombatFinalHPOutcome
			if err = json.Unmarshal(row.Terminal, &terminal); err != nil {
				t.Fatal(err)
			}
			if !terminal.Outcome.Authoritative {
				t.Fatal("public result lacks independent authority")
			}
			end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(0), OptRecord: proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)}
			if terminal.Outcome.Winner == 0 {
				end.Result = proto.Int32(1)
			}
			// Reject a same-winner altered operation clock before any paid-state/reward mutation.
			altered := proto.Clone(end).(*pb.CSRushFightEnd)
			altered.OptRecord.SkillOpts[0].RoundTime = proto.Float64(math.Nextafter(altered.OptRecord.SkillOpts[0].GetRoundTime(), math.Inf(1)))
			if _, err = f.s.p7End(f.ctx, f.id, altered); err == nil {
				t.Fatal("nonidentical complete record accepted")
			}
			var unchanged db.P7RushFight
			if err = db.GetIns().First(&unchanged, "id=?", fight.ID).Error; err != nil || unchanged.Status != constUtil.P7RushComputed {
				t.Fatal("rejected end changed paid state", err)
			}
			retry, err := f.s.p7Start(f.ctx, f.id, request, 1410, false)
			if err != nil || !proto.Equal(retry, start) {
				t.Fatal("paid start retry drift", err)
			}
			if _, err = f.s.p7Start(f.ctx, f.id, request, 1411, false); err == nil {
				t.Fatal("active paid fight replaced")
			}
			// Current policy/content changes stop new starts and preserve the accepted frozen run.
			if err = db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", cfg.ID).Update("reward", "point,999999").Error; err != nil {
				t.Fatal(err)
			}
			response, err := f.s.p7End(f.ctx, f.id, end)
			if err != nil {
				t.Fatal(err)
			}
			accepted := response[0].(*pb.SCRushFightEnd)
			if len(response) != 2 || response[1].(*pb.SCPlayer).Player.GetPlayerId() != f.id {
				t.Fatal("scoped end lacks current wallet sync")
			}
			expected := int32(0)
			if end.GetResult() == 1 {
				expected = 10
			}
			after, gameAfter := f.profile(t)
			if after.GetPoint() != before.GetPoint()+expected {
				t.Fatal("frozen reward mismatch", after.GetPoint(), expected)
			}
			var state db.P7RushState
			if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.ActiveFight != "" {
				t.Fatal("paid run did not close", err)
			}
			if kind == constUtil.P7RushElite && state.EliteUsed != 1 {
				t.Fatal("elite real budget was not consumed once")
			}
			if kind == constUtil.P7RushTrial && end.GetResult() == 1 && (state.Floor != 2 || state.TotalStars != 1) {
				t.Fatal("trial did not advance actual stars")
			}
			f.restart(t)
			repeated, err := f.s.p7End(f.ctx, f.id, end)
			if err != nil || !proto.Equal(repeated[0], accepted) {
				t.Fatal("cold accepted end changed", err)
			}
			again, gameAgain := f.profile(t)
			if !proto.Equal(after, again) || !p6CopySeriesJSONEqual(t, gameAfter, gameAgain) {
				t.Fatal("cold receipt granted twice")
			}
			if p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 1 || p1Count(t, &db.P7RushScopedCalculation{}, "player_id=?", f.id) != 1 {
				t.Fatal("retry duplicated source")
			}
			available, err := p7AuthorityAvailable(db.GetIns())
			if err != nil || available {
				t.Fatal("scoped settlement opened global parity", err)
			}
			if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushEliteBossBuyTime{}, 1412); err == nil {
				t.Fatal("scoped result opened uncertified commerce")
			}
		})
	}
}
