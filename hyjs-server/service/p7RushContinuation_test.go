package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7PublishContinuation(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P7RushContinuationPolicy{}).Where("id=?", constUtil.P7RushContinuationPolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("source=?", constUtil.P7RushContinuationSource).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.GetIns().Model(&db.P7RushContinuationPolicy{}).Where("id=?", constUtil.P7RushContinuationPolicyID).Update("enabled", false)
		db.GetIns().Model(&db.P7RushStageRule{}).Where("source=?", constUtil.P7RushContinuationSource).Update("enabled", false)
	})
}

func TestP7RushContinuationRealAdditionFourthBattleEndColdReplayAndReplayable(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	p7PublishRushAddition(t)
	p7PublishContinuation(t)
	// 未挑战的历史累计或目录开启都不能替代当前轮的三个真实胜利。
	for floor := int32(1); floor <= 3; floor++ {
		p7SectionPlayFloor(t, f, floor, 1, 18400+int64(floor))
	}
	options, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	option := options[0].(*pb.SCRushGetSectionAddition)
	if _, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAddition{Select: option.StarResult3, Star: proto.Int32(3)}, 18410, time.Now()); err != nil {
		t.Fatal(err)
	}
	before, game := f.profile(t)
	baseline, err := f.s.battlePlayer(before, game)
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tasks := current[0].(*pb.SCRushCurrentInfo).TaskInfoList
	if len(tasks) != 3 {
		t.Fatal("published fourth floor is missing original star choices")
	}
	for _, task := range tasks {
		if task.GetTaskId() != "local-continuation-4" || task.GetPoint() != 10 || task.GetCoin() != 0 {
			t.Fatal("fourth floor original Ready preview", task)
		}
	}
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-continuation-4"), Star: proto.Int32(1)}, 18411, false)
	if err != nil {
		t.Fatal(err)
	}
	if start.BattlePlayer.Formation[0].Prop.GetAtt() != math.Floor(baseline.Formation[0].Prop.GetAtt()*1.03) || start.BattlePlayer.Formation[0].GetGuid() != before.GetHeroWarId() {
		t.Fatal("paid real fourth Start did not use current legitimate roster and purchased addition")
	}
	if start.BattlePlayer.Formation[0].Prop.GetAtt() <= baseline.Formation[0].Prop.GetAtt() {
		t.Fatal("finite source should expose a nonzero real addition")
	}
	var calculation db.P7RushScopedCalculation
	if err = db.GetIns().First(&calculation, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal(calculation.Terminal, &terminal); err != nil {
		t.Fatal(err)
	}
	expected := int32(0)
	if terminal.Outcome.Winner == 0 {
		expected = 1
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(expected), OptRecord: terminal.OriginalOperates}
	// 目录关闭后重登仍可从冻结的同一fight恢复，其他星档继续拒绝。
	if err = db.GetIns().Model(&db.P7RushContinuationPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	pending, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil || len(pending[0].(*pb.SCRushCurrentInfo).TaskInfoList) != 1 || pending[0].(*pb.SCRushCurrentInfo).TaskInfoList[0].GetStar() != 1 {
		t.Fatal("cold pending original Ready entry", err)
	}
	if _, err = f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-continuation-4"), Star: proto.Int32(2)}, 18412, false); err == nil {
		t.Fatal("different star replaced pending battle")
	}
	resumed, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-continuation-4"), Star: proto.Int32(1)}, 18413, false)
	if err != nil || !proto.Equal(start, resumed) {
		t.Fatal("cold pending Start changed source", err)
	}
	if p1Count(t, &db.P7RushFight{}, "id=?", start.GetFightId()) != 1 {
		t.Fatal("resume created duplicate fight")
	}
	forged := proto.Clone(end).(*pb.CSRushFightEnd)
	forged.Result = proto.Int32(1 - expected)
	if _, err = f.s.p7End(f.ctx, f.id, forged); err == nil {
		t.Fatal("client-supplied winner replaced actual fourth calculation")
	}
	ended, err := f.s.p7End(f.ctx, f.id, end)
	if err != nil {
		t.Fatal(err)
	}
	result := ended[0].(*pb.SCRushFightEnd)
	if result.GetResult() != expected || result.GetStar() != expected {
		t.Fatal("actual fourth result not retained", result)
	}
	after, heldGame := f.profile(t)
	if after.GetPoint() != before.GetPoint()+expected*10 {
		t.Fatal("fourth reward differs from actual outcome")
	}
	var state db.P7RushState
	if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.TotalStars != 3+expected || state.ActiveFight != "" {
		t.Fatal("fourth actual progress", err)
	}
	if state.Floor != 4+expected {
		t.Fatal("fourth winner/loser progression", state.Floor, expected)
	}
	// 真实局创建后关闭、改动运营目录，已冻结End继续冷重试，不能重发奖励。
	f.restart(t)
	repeated, err := f.s.p7End(f.ctx, f.id, end)
	if err != nil || !proto.Equal(ended[0], repeated[0]) {
		t.Fatal("cold fourth End receipt", err)
	}
	still, stillGame := f.profile(t)
	if !proto.Equal(still, after) || !p6CopySeriesJSONEqual(t, heldGame, stillGame) {
		t.Fatal("cold fourth End duplicated reward")
	}
	if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 18420); err != nil {
		t.Fatal("original reset after real fourth result", err)
	}
	view, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAdditionView{}, 0, time.Now())
	if err != nil || view[0].(*pb.SCRushAdditionView).GetProp().GetPeratt() != 0 {
		t.Fatal("old run addition leaked into replay", err)
	}
	if _, err = f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(1)}, 18421, false); err != nil {
		t.Fatal("original new first-floor run cannot be played again", err)
	}
}

func TestP7RushContinuationCatalogueFullOriginalNPCAndActualRunGate(t *testing.T) {
	f := p7SectionFixture(t)
	p7PublishContinuation(t)
	rows, err := p7RushContinuationCatalogue(4, 6, true)
	if err != nil || len(rows) != 9 {
		t.Fatal("finite published catalogue", err)
	}
	baseRows, err := p7SectionCatalogue(true)
	if err != nil {
		t.Fatal(err)
	}
	base := &pb.BattleMonster{}
	if err = proto.Unmarshal(baseRows[0].Enemies, base); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile("../db/sql/86_p7_rush_stage_rules.seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range rows {
		if !strings.Contains(string(seed), "'"+stage.ID+"'") || !strings.Contains(string(seed), "0x"+hex.EncodeToString(stage.Enemies)) {
			t.Fatal("SQL seed drifted from catalogue", stage.ID)
		}
		monster := &pb.BattleMonster{}
		if err = proto.Unmarshal(stage.Enemies, monster); err != nil {
			t.Fatal(err)
		}
		if monster.Inst == nil || len(monster.Formation) != 1 || monster.Formation[0].GetSkillOrder() != base.Formation[0].GetSkillOrder() || monster.Formation[0].GetId() != base.Formation[0].GetId() || monster.Formation[0].GetLevel() != base.Formation[0].GetLevel() {
			t.Fatal("whole original NPC source changed", stage.ID)
		}
		wantHP := math.Floor(base.Formation[0].Prop.GetHp() * (1 + float64(stage.Floor-3)*0.5))
		if monster.Formation[0].Prop.GetHp() != wantHP || monster.Formation[0].Inst.GetCurHp() != wantHP {
			t.Fatal("local difficulty source differs", stage.ID)
		}
		if _, err = f.s.validateP7Stage(stage); err != nil {
			t.Fatal(err)
		}
		allowed, e := f.s.p7RushContinuationStageAvailable(db.GetIns().WithContext(f.ctx), &db.P7RushState{PlayerID: f.id, RunID: 1, Floor: stage.Floor, TotalStars: 9, RecordStars: 9}, stage)
		if e != nil || allowed {
			t.Fatal("unproven history opened continuation", e)
		}
		mutated := stage
		mutated.Enemies = append([]byte(nil), stage.Enemies...)
		mutated.Enemies[len(mutated.Enemies)-1] ^= 1
		if bytes.Equal(mutated.Enemies, stage.Enemies) {
			t.Fatal("negative source was not mutated")
		}
		if _, e = f.s.p7RushContinuationStageAvailable(db.GetIns().WithContext(f.ctx), &db.P7RushState{PlayerID: f.id, RunID: 1, Floor: stage.Floor}, mutated); e == nil {
			t.Fatal("changed stage source accepted")
		}
	}
}
