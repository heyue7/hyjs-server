package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13ZeroFightActualTimedRoundAndLocalInbox(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, false, false)
}

func TestP13UFFinalizationWorkerRealTimedRoundWithoutQuery(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, false, false, false, false, false, false, true)
}

func p13ZeroFightActualTimedRoundAndLocalInbox(t *testing.T, transportMVP bool, tiedMVP bool, naturalMissions ...bool) {
	missions := len(naturalMissions) > 0 && naturalMissions[0]
	weekly := len(naturalMissions) > 1 && naturalMissions[1]
	pendingWeek := len(naturalMissions) > 2 && naturalMissions[2]
	unmatchedWeek := len(naturalMissions) > 3 && naturalMissions[3]
	fastWorker := len(naturalMissions) > 4 && naturalMissions[4]
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(clock.Load())
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated real scene; explicit local HP/spawns, no combat"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "enabled": true, "version": "local-archive-reviewed-items311-v1", "source": "isolated local operator publication: archived guild prestige50/25, original current UI item311; quantities1/2 locally chosen, not current private-server awards", "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "published_at": now.UnixMilli() - 1}).Error; err != nil {
		t.Fatal(err)
	}
	if transportMVP {
		if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Update("transport_mvp_rewards", "item_311,4").Error; err != nil {
			t.Fatal(err)
		}
	}
	if missions {
		if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"personal_missions": `[{"kind":1,"target":2,"score":5},{"kind":2,"target":2,"score":7}]`, "task_rewards": "item_311,6"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if weekly {
		t.Cleanup(func() { db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("1=1").Update("enabled", false) })
		for _, table := range []any{&db.P13UnionFightWeeklyPolicy{}, &db.P13UnionFightWeeklyOffer{}, &db.P13UnionFightWeeklyResult{}} {
			if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
				t.Fatal(err)
			}
		}
		day, _ := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
		if err := db.GetIns().Create(&db.P13UnionFightWeeklyPolicy{WeekStart: p13UFWeek(day), Group: match.Group, Enabled: true, Version: "local-week-real-v1", Source: "explicit local weekly participant award, original Sunday; quantities8/4 local", Rewards: `[{"rank":1,"rewards":"item_311,8"},{"rank":2,"rewards":"item_311,4"}]`, PublishedAt: now.UnixMilli() - 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.TickP13UnionFightSettlement(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("real roster")
	}
	participants := []int64{roster.Left[0].PlayerID, roster.Right[0].PlayerID}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	// These publication keys belong only to this fixture. MySQL cases reuse one
	// schema; old fields retain their frozen nodes while the next case publishes.
	if err = db.GetIns().Where("source_key LIKE ?", "zero-real-mine-%").Delete(&db.P13UnionFightResourceSource{}).Error; err != nil {
		t.Fatal(err)
	}
	for camp, npc := range map[int64]string{0: "2", 1: "5"} {
		point, err := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("zero-real-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 2, PublishedAt: now.UnixMilli(), Source: "local published node at original recycle NPC, actual3s mine"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range participants {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, point := range []struct {
		at    int64
		stage int32
	}{{first - 60000, 3}, {first, 4}} {
		clock.Store(point.at)
		frames, err := f.s.p13UFQuery(f.ctx, participants[0], &pb.CSUFCurrentStageInfo{}, time.UnixMilli(point.at))
		if err != nil || frames[0].(*pb.SCUFCurrentStageInfo).GetStage() != point.stage {
			t.Fatal("original prepared/fighting stage", err, frames)
		}
	}
	clock.Store(first + 20000)
	for _, id := range participants {
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	var miner db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&miner, "match_id=? AND player_id=?", match.ID, participants[0]).Error; err != nil {
		t.Fatal(err)
	}
	npc := "2"
	if miner.Camp == 1 {
		npc = "5"
	}
	point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
	clock.Add(20000)
	if _, err = f.s.p13UFFieldMove(f.ctx, miner.PlayerID, &pb.CSMove{PointNew: point}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	guid := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("zero-real-mine-%d", miner.Camp))
	if _, err = f.s.p13UFResourceBegin(f.ctx, miner.PlayerID, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(guid)}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if missions {
		frames, handled, err := f.s.DispatchP13UnionFight(f.ctx, miner.PlayerID, &pb.CSUFPlayerMissionList{}, 0)
		if err != nil || !handled || len(frames[0].(*pb.SCUFPlayerMissionList).GetMissionInfos()) != 2 {
			t.Fatal("original empty socket task list", err, frames)
		}
		if frames[0].(*pb.SCUFPlayerMissionList).GetMissionInfos()[0].GetIsComplete() != 0 {
			t.Fatal("incomplete gather cannot complete task")
		}
	}
	clock.Add(3000)
	if err = f.s.TickP13UnionFightResource(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if missions {
		frames, err := f.s.p13UFMissionQuery(f.ctx, miner.PlayerID, &pb.CSUFPlayerMissionList{}, time.UnixMilli(clock.Load()))
		if err != nil {
			t.Fatal(err)
		}
		infos := frames[0].(*pb.SCUFPlayerMissionList).GetMissionInfos()
		if infos[0].GetIsComplete() != 1 || infos[1].GetIsComplete() != 0 {
			t.Fatal("actual gather only", infos)
		}
	}
	deposit, err := f.s.p13UFResourceDeposit(f.ctx, miner.PlayerID, &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 910, time.UnixMilli(clock.Load()))
	if err != nil {
		t.Fatal(err)
	}
	if deposit[0].(*pb.SCUFPlayerUpResource).GetResourceNum() != 2 {
		t.Fatal("real mined2 deposit")
	}
	if missions {
		if len(deposit) != 2 || deposit[1].(*pb.SCUFMissionPush).GetPlayerFightScore() != 12 {
			t.Fatal("actual natural two-task points", deposit)
		}
		frames, err := f.s.p13UFMissionQuery(f.ctx, miner.PlayerID, &pb.CSUFPlayerMissionList{}, time.UnixMilli(clock.Load()))
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range frames[0].(*pb.SCUFPlayerMissionList).GetMissionInfos() {
			if item.GetIsComplete() != 1 || item.GetProgress() != "2/2" {
				t.Fatal("actual two complete tasks", item)
			}
		}
	}
	if tiedMVP {
		var other db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&other, "match_id=? AND player_id=?", match.ID, participants[1]).Error; err != nil {
			t.Fatal(err)
		}
		npc := "2"
		if other.Camp == 1 {
			npc = "5"
		}
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if _, err = f.s.p13UFFieldMove(f.ctx, other.PlayerID, &pb.CSMove{PointNew: point}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		guid := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("zero-real-mine-%d", other.Camp))
		if _, err = f.s.p13UFResourceBegin(f.ctx, other.PlayerID, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(guid)}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		clock.Add(3000)
		if err = f.s.TickP13UnionFightResource(f.ctx, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFResourceDeposit(f.ctx, other.PlayerID, &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 911, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	if transportMVP {
		// Original HTTP receipt retries never add another deposit or ore statistic.
		for i := 0; i < 8; i++ {
			if _, err = f.s.p13UFResourceDeposit(f.ctx, miner.PlayerID, &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 910, time.UnixMilli(clock.Load())); err != nil {
				t.Fatal(err)
			}
		}
		var stats db.P13UnionFightTransportStat
		if err = db.GetIns().First(&stats, "match_id=? AND player_id=?", match.ID, miner.PlayerID).Error; err != nil || stats.Ore != 2 || stats.Deposits != 1 {
			t.Fatal("actual ore statistic, no receipt duplication", stats, err)
		}
	}
	if err = f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(match.EndAt-1)); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightSettlementResult{}).Where("match_id=?", match.ID).Count(&count)
	if count != 0 {
		t.Fatal("early result")
	}
	if fastWorker {
		// The slow cadence last ran before End. Only the actual fast worker
		// advances the real round at End; no query or direct full Tick does it.
		clock.Store(match.EndAt - 1)
		workerCtx, cancel := context.WithCancel(f.ctx)
		calls := 0
		err = serveP13UFFinalization(workerCtx, time.Millisecond, func(ctx context.Context, at time.Time) error {
			calls++
			e := f.s.TickP13UnionFightFinalization(ctx, at)
			if calls == 1 {
				clock.Store(match.EndAt)
			} else {
				cancel()
			}
			return e
		}, func(e error, _ int64) { t.Error(e) })
		cancel()
		if err != nil || calls != 2 {
			t.Fatal("fast worker cadence", calls, err)
		}
	} else {
		clock.Store(match.EndAt)
		var workers sync.WaitGroup
		failures := make(chan error, 8)
		for i := 0; i < 8; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				failures <- f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load()))
			}()
		}
		workers.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var result db.P13UnionFightSettlementResult
	if err = db.GetIns().First(&result, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	winner := match.LeftGuildID
	if result.RightPower < result.LeftPower {
		winner = match.RightGuildID
	}
	if result.WinnerGuildID != winner || result.LeftPower == result.RightPower || result.DeliveredAt != 0 {
		t.Fatal("real lower frozen power only at end", result)
	}
	for _, id := range participants {
		reply, err := f.s.p13UFSettlementQuery(f.ctx, id, &pb.CSUFFightResult{}, time.UnixMilli(clock.Load()))
		if err != nil {
			t.Fatal(err)
		}
		if transportMVP {
			result := reply[0].(*pb.SCUnionFightResult).GetResult()
			if (!tiedMVP && (len(result.GetMvpInfos()) != 1 || result.GetMvpInfos()[0].GetType() != 2 || result.GetMvpInfos()[0].GetPlayerId() != miner.PlayerID || result.GetMvpInfos()[0].GetValue() != 2)) || (tiedMVP && len(result.GetMvpInfos()) != 0) {
				t.Fatal("original transport MVP slot and actual ore", result)
			}
			expectedAchieves := 3
			if missions {
				expectedAchieves++
			}
			if len(result.GetSelfAchieves()) != expectedAchieves || result.GetSelfAchieves()[2].GetType() != 2 || result.GetSelfAchieves()[2].GetIsSuccess() != func() int32 {
				if id == miner.PlayerID && !tiedMVP {
					return 1
				}
				return 0
			}() {
				t.Fatal("one personal MVP award only", result)
			}
		}
		if missions {
			award := reply[0].(*pb.SCUnionFightResult).GetResult().GetSelfAchieves()[3]
			if award.GetType() != 3 || (award.GetIsSuccess() == 1) != (id == miner.PlayerID) {
				t.Fatal("original task slot3", award)
			}
		}
		if reply[0].(*pb.SCUnionFightResult).GetResult().GetWinUnionId() != winner {
			t.Fatal("original result winner")
		}
	}
	// A side-effect-free rejected request leaves no combat identity. A separately
	// committed audit still blocks peaceful terminal and pending-week rewards.
	var second db.P13UnionFightMatch
	if err = db.GetIns().First(&second, "round=2 AND (left_guild_id=? OR right_guild_id=?)", match.LeftGuildID, match.LeftGuildID).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(second.StartAt - 60000)
	if !weekly || pendingWeek {
		zero, one := float64(0), float64(1)
		if err = db.GetIns().Save(&db.P13UnionFightBattleAuditSource{Day: second.Day, Enabled: true, SeedSalt: 42, AttackerDamageFactor: &zero, DefenderDamageFactor: &one, PublishedAt: clock.Load() - 1, Source: "isolated audit-only source; actual frozen PVE14/0-1; never war authority"}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err = f.s.TickP13UnionFightField(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var secondRoster p13UFMatchRoster
	if json.Unmarshal(second.Roster, &secondRoster) != nil {
		t.Fatal("round2 roster")
	}
	pair := []int64{secondRoster.Left[0].PlayerID, secondRoster.Right[0].PlayerID}
	for _, id := range pair {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	clock.Store(second.StartAt + 20000)
	for _, id := range pair {
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	if !weekly || pendingWeek {
		if _, handled, err := f.s.DispatchP13UnionFight(f.ctx, pair[0], &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(pair[1])}, 0); !handled || err == nil {
			t.Fatal("real nonauthority attack unexpectedly succeeded", err)
		}
		if count := p1Count(t, &db.P13UnionFightSettlementIntent{}, "match_id=?", second.ID); count != 0 {
			t.Fatal("side-effect-free refusal polluted match", count)
		}
		// Check eligibility at EndAt inside a rolled-back transaction, then keep
		// the live field before EndAt for a genuine committed audit request.
		func() {
			tx := db.GetIns().Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			var offer db.P13UnionFightSettlementOffer
			if e := tx.First(&offer, "match_id=?", second.ID).Error; e != nil {
				t.Fatal(e)
			}
			terminal, e := f.s.p13UFSettleZeroFight(tx, &second, &offer, time.UnixMilli(second.EndAt))
			if e != nil || terminal == nil {
				t.Fatal("harmless refused request prevented peaceful terminal", e)
			}
		}()
		clock.Store(second.StartAt + 60000)
		var target db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&target, "match_id=? AND player_id=?", second.ID, pair[1]).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, pair[0], &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(target.X), Y: proto.Float32(target.Y), Z: proto.Float32(target.Z)}}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal("real audit approach", err)
		}
		if _, handled, e := f.s.DispatchP13UnionFight(f.ctx, pair[0], &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(pair[1])}, 987); !handled || e == nil || e.Error() != constUtil.P13UnionFightBattleAuditRefusal {
			t.Fatal("actual audit committed refusal", handled, e)
		}
		var audit db.P13UnionFightBattleAuditAttempt
		if err = db.GetIns().First(&audit, "match_id=? AND player_id=? AND stamp=?", second.ID, pair[0], 987).Error; err != nil {
			t.Fatal("missing committed audit", err)
		}
		if audit.SourceHash != p13UFBattleAuditAttemptHash(&audit) {
			t.Fatal("committed audit identity changed")
		}
		if err = p13UFBattleAuditVerifyRecord(db.GetIns(), &audit); err != nil {
			t.Fatal("actual audit source invalid", err)
		}
		clock.Store(second.EndAt)
		if err = f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		if count := p1Count(t, &db.P13UnionFightSettlementResult{}, "match_id=?", second.ID); count != 0 {
			t.Fatal("refused battle manufactured zero-combat victory")
		}
		if _, err := f.s.p13UFSettlementQuery(f.ctx, pair[0], &pb.CSUFFightResult{}, time.UnixMilli(clock.Load())); err == nil {
			t.Fatal("pending round replayed an earlier round result")
		}
	} else {
		clock.Store(second.EndAt)
		if err = f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		if p1Count(t, &db.P13UnionFightSettlementResult{}, "match_id=?", second.ID) != 1 {
			t.Fatal("real second peaceful round did not settle")
		}
	}
	if weekly {
		if unmatchedWeek {
			day, _ := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
			declare := day.AddDate(0, 0, 1).Add(12 * time.Hour)
			p13UFCall(t, f, participants[0], &pb.CSUFUnionApply{}, 1010, declare)
			p13UFCall(t, f, participants[0], &pb.CSUFPlayerApply{}, 1011, declare)
			clock.Store(day.AddDate(0, 0, 2).Add(31 * time.Minute).UnixMilli())
			if err = f.s.TickP13UnionFight(f.ctx, time.UnixMilli(clock.Load())); err != nil {
				t.Fatal(err)
			}
		}
		p13WeeklyActualSundayClaims(t, f, match, participants, clock, pendingWeek || unmatchedWeek, !pendingWeek)
		return
	}
	for _, point := range []struct {
		at    int64
		stage int32
	}{{match.EndAt, 5}, {second.StartAt, 6}, {second.EndAt, 7}} {
		frames, err := f.s.p13UFQuery(f.ctx, participants[0], &pb.CSUFCurrentStageInfo{}, time.UnixMilli(point.at))
		if err != nil {
			t.Fatal(err)
		}
		stage := frames[0].(*pb.SCUFCurrentStageInfo)
		if stage.GetStage() != point.stage || stage.GetFirstFightInfo().GetFightResult() == 0 || stage.GetSecondFightInfo().GetFightResult() != 0 {
			t.Fatal("original stage/results projection", stage)
		}
	}
	if err = f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(result.AwardAt-1)); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "p13-war:"+match.ID).Count(&count)
	if count != 0 {
		t.Fatal("23:30 early delivery")
	}
	// An already frozen publication remains owed when new offers are switched off.
	if err = db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	clock.Store(result.AwardAt)
	if err = f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "p13-war:"+match.ID).Count(&count)
	if count != 2 {
		t.Fatal("actual participants one delivery each", count)
	}
	for _, id := range participants {
		itemCount := func() int64 {
			var row db.Profile
			if err := db.GetIns().First(&row, id).Error; err != nil {
				t.Fatal(err)
			}
			_, state, err := decodeProfile(&row)
			if err != nil {
				t.Fatal(err)
			}
			var n int64
			for _, item := range state.Items {
				if item.GetId() == "item_311" {
					n += int64(item.GetNum())
				}
			}
			return n
		}
		before := itemCount()
		messages, err := f.s.RewardCenter(f.ctx, id, &pb.CSQueryUserReward{})
		if err != nil {
			t.Fatal(err)
		}
		var rewardIDs []int64
		for _, item := range messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList() {
			rewardIDs = append(rewardIDs, item.GetRewardId())
		}
		if len(rewardIDs) != 1 {
			t.Fatal("one real inbox grant", len(rewardIDs))
		}
		if _, err = f.s.RewardCenter(f.ctx, id, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.RewardCenter(f.ctx, id, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs}); err != nil {
			t.Fatal(err)
		}
		var member db.P13GuildMember
		if err = db.GetIns().First(&member, "player_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(1)
		if member.GuildID == winner {
			want = 3
		}
		if missions && id == miner.PlayerID {
			want += 6
		}
		if transportMVP && !tiedMVP && id == miner.PlayerID {
			want += 4
		}
		if got := itemCount() - before; got != want {
			t.Fatal("P1 actual item311 once", got, want)
		}
	}
	var excluded int64
	db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=? AND player_id NOT IN ?", "p13-war:"+match.ID, participants).Count(&excluded)
	if excluded != 0 {
		t.Fatal("nonparticipants rewarded", ids)
	}
}

// Undefined original ties, absent opponents and a changed frozen roster do not
// produce a result or reward. Each case uses real guild signup and actual entry.
func TestP13ZeroFightUndefinedRosterAndParticipationStayUnresolved(t *testing.T) {
	for _, scenario := range []string{"missing-side", "withdrawn", "equal-power"} {
		t.Run(scenario, func(t *testing.T) {
			f, ids, first, clock, match := p13UFFieldSetup(t)
			t.Cleanup(func() { db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("1=1").Update("enabled", false) })
			if err := f.s.InitializeP6(f.ctx); err != nil {
				t.Fatal(err)
			}
			if scenario == "equal-power" {
				var candidates []db.P13UnionFightMatch
				if err := db.GetIns().Where("round=1").Find(&candidates).Error; err != nil {
					t.Fatal(err)
				}
				found := false
				for i := range candidates {
					var left, right pb.UFInfoPreView
					if proto.Unmarshal(candidates[i].LeftPreview, &left) == nil && proto.Unmarshal(candidates[i].RightPreview, &right) == nil && left.GetFightPower() == right.GetFightPower() {
						match = &candidates[i]
						found = true
						break
					}
				}
				if !found {
					t.Fatal("actual equal power roster not found")
				}
			}
			now := time.UnixMilli(clock.Load())
			if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated undefined round"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "local-negative-v1", "source": "explicit isolated local publication", "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "published_at": now.UnixMilli() - 1}).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.TickP13UnionFightSettlement(f.ctx, now); err != nil {
				t.Fatal(err)
			}
			var roster p13UFMatchRoster
			if json.Unmarshal(match.Roster, &roster) != nil {
				t.Fatal("real roster")
			}
			pair := []int64{roster.Left[0].PlayerID, roster.Right[0].PlayerID}
			if scenario == "missing-side" {
				pair = pair[:1]
			}
			for _, id := range pair {
				if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
					t.Fatal(err)
				}
			}
			clock.Store(first + 20000)
			for _, id := range pair {
				if _, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "withdrawn" {
				p13GuildCall(t, f, ids[10], &pb.CSQuitUnion{}, 730, time.UnixMilli(clock.Load()))
			}
			clock.Store(match.EndAt)
			if err := f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
				t.Fatal(err)
			}
			if n := p1Count(t, &db.P13UnionFightSettlementResult{}, "match_id=?", match.ID); n != 0 {
				t.Fatal("undefined original branch settled", scenario)
			}
			if n := p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-war:"+match.ID); n != 0 {
				t.Fatal("undefined original branch rewarded", scenario)
			}
		})
	}
}
