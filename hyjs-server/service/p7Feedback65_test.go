package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Reuse the original scoped actor/engine fixture; no supplied winner creates a win.
func p7Feedback65EliteFixture(t *testing.T) (*p1Fixture, db.P7RushStageRule, *pb.CSRushFightStart) {
	t.Helper()
	f, cfg, request := p7ScopedFixture(t, constUtil.P7RushElite)
	cfg.FirstReward, cfg.Reward = "point,2000", "coin,40000"
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", constUtil.P7ScopedRushPolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// Keep this fixture on its existing original automatic scoped consumer.
	if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id IN ?", []int64{constUtil.P7RushManualDriverPolicyID, constUtil.P7RushEliteManualDriverPolicyID}).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	return f, cfg, request
}

func p7Feedback65WinningEnd(t *testing.T, start *pb.SCRushFightStart) *pb.CSRushFightEnd {
	t.Helper()
	var calculation db.P7RushScopedCalculation
	if err := db.GetIns().First(&calculation, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if err := json.Unmarshal(calculation.Terminal, &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.Winner != 0 || terminal.OriginalOperates == nil {
		t.Fatal("original scoped fixture did not actually win; preserve the real outcome")
	}
	return &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}
}

func TestP7Feedback65ElitePendingNewStampColdAndClosedPolicy(t *testing.T) {
	f, cfg, request := p7Feedback65EliteFixture(t)
	start, err := f.s.p7Start(f.ctx, f.id, request, 65001, false)
	if err != nil {
		t.Fatal(err)
	}
	var state db.P7RushState
	if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.EliteUsed != 1 {
		t.Fatal("first elite budget", state, err)
	}
	for _, altered := range []*pb.CSRushFightStart{
		{RushType: proto.Int32(constUtil.P7RushTrial), TaskId: request.TaskId, Star: request.Star},
		{RushType: request.RushType, TaskId: proto.String("other-elite"), Star: request.Star},
		{RushType: request.RushType, TaskId: request.TaskId, Star: proto.Int32(1)},
	} {
		if _, err = f.s.p7Start(f.ctx, f.id, altered, 65002, false); err == nil {
			t.Fatal("different kind/task/star resumed the elite", altered)
		}
	}
	foreign := state
	foreign.PlayerID++
	if _, _, _, err = f.s.p7PendingRushContinuation(db.GetIns().WithContext(f.ctx), &foreign); err == nil {
		t.Fatal("another owner read the pending elite")
	}
	foreign = state
	foreign.RunID++
	if _, _, _, err = f.s.p7PendingRushContinuation(db.GetIns().WithContext(f.ctx), &foreign); err == nil {
		t.Fatal("another run read the pending elite")
	}
	resumed, err := f.s.p7Start(f.ctx, f.id, request, 65003, false)
	if err != nil || !proto.Equal(start, resumed) {
		t.Fatal("new stamp did not restore frozen elite", err)
	}
	// Ordinary info retains its own floor/tasks while an elite is pending.
	if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("source=?", constUtil.P7SectionStageSource).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	info, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
	if err != nil || info[0].(*pb.SCGetRushInfo).GetCurFloor() != state.Floor {
		t.Fatal("elite changed ordinary floor projection", err)
	}
	current, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tasks := current[0].(*pb.SCRushCurrentInfo).TaskInfoList
	if len(tasks) == 0 {
		t.Fatal("ordinary tasks disappeared behind pending elite")
	}
	for _, task := range tasks {
		if task.GetTaskId() == cfg.TaskID || task.GetStar() == 0 {
			t.Fatal("elite was projected as an ordinary task", task)
		}
	}
	for _, policy := range []any{&db.P7RushPolicy{}, &db.P7RushScopedPolicy{}} {
		if err = db.GetIns().Model(policy).Where("id=1").Update("enabled", false).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.restart(t)
	resumed, err = f.s.p7Start(f.ctx, f.id, request, 65004, false)
	if err != nil || !proto.Equal(start, resumed) {
		t.Fatal("cold/closed policy lost previously accepted elite", err)
	}
	if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.EliteUsed != 1 || state.ActiveFight != start.GetFightId() || p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 1 {
		t.Fatal("resume charged again or replaced the paid fight", state, err)
	}
}

func TestP7Feedback65EliteFirstRewardQueryAndRepeatedEnd(t *testing.T) {
	f, cfg, request := p7Feedback65EliteFixture(t)
	before, _ := f.profile(t)
	for attempt := 0; attempt < 2; attempt++ {
		start, err := f.s.p7Start(f.ctx, f.id, request, 65020+int64(attempt), false)
		if err != nil {
			t.Fatal(err)
		}
		end := p7Feedback65WinningEnd(t, start)
		accepted, err := f.s.p7End(f.ctx, f.id, end)
		if err != nil {
			t.Fatal(err)
		}
		result := accepted[0].(*pb.SCRushFightEnd)
		if result.GetResult() != 1 || result.ResultObj == nil {
			t.Fatal("actual elite win lost its result/reward")
		}
		awards := map[string]int32{}
		for _, item := range result.ResultObj.GetSimps() {
			awards[item.GetId()] += item.GetNum()
		}
		firstPoint := int32(0)
		if attempt == 0 {
			firstPoint = 2000
		}
		if awards["coin"] != 40000 || awards["point"] != firstPoint || len(awards) != 2-attempt {
			t.Fatal("elite receipt did not distinguish first and repeated wins", attempt, awards)
		}
		after, game := f.profile(t)
		if after.GetCoin() != before.GetCoin()+int64(attempt+1)*40000 || after.GetPoint() != before.GetPoint()+2000 {
			t.Fatal("first/regular elite wallet reward mismatch", attempt, after.GetCoin(), after.GetPoint())
		}
		f.restart(t)
		repeated, err := f.s.p7End(f.ctx, f.id, end)
		if err != nil || !proto.Equal(repeated[0], result) {
			t.Fatal("cold repeated elite receipt changed", err)
		}
		again, againGame := f.profile(t)
		if !proto.Equal(after, again) || !p6CopySeriesJSONEqual(t, game, againGame) {
			t.Fatal("repeated elite End granted again")
		}
		messages, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, boss := range messages[0].(*pb.SCGetRushEliteBossInfo).BossInfoList {
			if boss.GetEliteTaskId() == cfg.TaskID {
				found = true
				if boss.GetIsSuccess() != 1 || len(boss.FirstRewardList) != 0 || len(boss.GeneralRewardList) != 1 || boss.GeneralRewardList[0].GetId() != "coin" || boss.GeneralRewardList[0].GetNum() != 40000 {
					t.Fatal("completed elite still advertises an unclaimed first reward", boss)
				}
			}
		}
		if !found {
			t.Fatal("elite disappeared after winning")
		}
	}
	if p1Count(t, &db.P7RushProgress{}, "player_id=? AND kind=? AND task_id=?", f.id, constUtil.P7RushElite, cfg.TaskID) != 1 {
		t.Fatal("elite first-clear ledger duplicated")
	}
}
