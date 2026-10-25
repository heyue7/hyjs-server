package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// 有限初始排名只用于建立真实对手；必须真实获胜后才能取得活动资格。
func TestFeedback69ArenaRankRewardAllOriginalTiers(t *testing.T) {
	f, peer := p11ArenaScopedSetup(t)
	now := LocalPlayNow(f.ctx)
	loadOpenServerTaskFixture(t, f.s)
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	start := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if err := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", constUtil.OpenServerActivityID).Updates(map[string]any{"start_at": start.UnixMilli(), "end_at": start.AddDate(0, 0, 14).UnixMilli(), "reward_end_at": start.AddDate(0, 0, 15).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, peer, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 69001, now)
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", f.id).Update("position", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", peer).Update("position", 2).Error; err != nil {
		t.Fatal(err)
	}
	// 未发生获胜时，数字排名不产生奖励资格。
	check := func(want bool) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, peer).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.s.refreshOpenServerProgressAt(db.GetIns(), p, g, now); err != nil {
			t.Fatal(err)
		}
		for task := int32(23); task <= 30; task++ {
			var state db.OpenServerActivityState
			err := db.GetIns().First(&state, "player_id=? AND task_id=?", peer, task).Error
			if !want {
				if err == nil && state.Achieved {
					t.Fatal("unwitnessed numeric rank granted", task)
				}
				continue
			}
			if err != nil || !state.Achieved || state.Progress != 1 {
				t.Fatal("original rank tier unavailable", task, state, err)
			}
		}
	}
	check(false)
	p11ArenaQuery(t, f, peer, &pb.CSPvpFindMatch{}, 69002, now)
	won, err := f.s.p11ArenaScopedFightAt(f.ctx, peer, &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}, 69003, now)
	if err != nil || won[0].(*pb.SCPvpFightResult).GetWinPlayerId() != peer {
		t.Fatal("actual qualifying win", err)
	}
	check(true)
	var before db.Profile
	db.GetIns().First(&before, peer)
	for task := int32(23); task <= 30; task++ {
		req := &pb.CSGetOpenServerReward{ItemId: proto.Int32(task)}
		if _, err := f.s.openServerActivityAt(f.ctx, peer, req, 69010+int64(task), now); err != nil {
			t.Fatalf("tier%d claim: %v", task, err)
		}
		first := db.Profile{}
		db.GetIns().First(&first, peer)
		if _, err := f.s.openServerActivityAt(f.ctx, peer, req, 69010+int64(task), now); err != nil {
			t.Fatal("same receipt", err)
		}
		after := db.Profile{}
		db.GetIns().First(&after, peer)
		if *first.Player != *after.Player || *first.Game != *after.Game {
			t.Fatal("duplicate rank claim mutated profile")
		}
	}
	t.Log("all 8 original arena tiers: real winner rank1, claim/retry preserved")
}

func TestFeedback69TrialResetAbandonsPendingAndRetry(t *testing.T) {
	f := p7Setup(t)
	if err := db.GetIns().Model(&db.P7RushPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7ReRushPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	state.Floor = 26
	state.TotalStars = 75
	state.HighFloor = 25
	state.RecordStars = 75
	state.ActiveFight = "reset69-pending"
	raw := []byte(`{"id":"pending-evidence"}`)
	fight := db.P7RushFight{ID: state.ActiveFight, PlayerID: f.id, RunID: state.RunID, Kind: constUtil.P7RushTrial, TaskID: "local_rush_26", Status: constUtil.P7RushComputed, Day: state.Day, Rule: raw, RuleHash: p6Digest(raw), Start: raw, StartHash: p6Digest(raw)}
	if err := db.GetIns().Create(&fight).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveP7RushCalendar(db.GetIns(), state, time.Now()); err != nil {
		t.Fatal(err)
	}
	oldRun := state.RunID
	req := &pb.CSRushResetChallengeCount{}
	first, err := f.s.P7RushMutation(f.ctx, f.id, req, 69901)
	if err != nil {
		t.Fatal(err)
	}
	current, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil || current.RunID != oldRun+1 || current.ActiveFight != "" || current.Floor != 1 || current.TotalStars != 0 || current.HighFloor != 25 || current.RecordStars != 75 {
		t.Fatal("reset state", current, err)
	}
	var old db.P7RushFight
	if err := db.GetIns().First(&old, "id=?", fight.ID).Error; err != nil || old.Status != constUtil.P7RushAbandoned69 {
		t.Fatal("old pending evidence", old, err)
	}
	retry, err := f.s.P7RushMutation(f.ctx, f.id, req, 69901)
	if err != nil || !proto.Equal(first[0], retry[0]) {
		t.Fatal("reset retry", err)
	}
	current2, _ := p7State(db.GetIns(), f.id, time.Now())
	if current2.RunID != current.RunID || current2.FreeResetUsed != current.FreeResetUsed {
		t.Fatal("duplicate reset")
	}
	if _, err := f.s.p7End(f.ctx, f.id, &pb.CSRushFightEnd{FightId: proto.String(fight.ID)}); err == nil {
		t.Fatal("abandoned End accepted")
	}
	t.Log("pending floor26 reset archives old fight and preserves history; synthetic frozen pending fixture")
}

func TestFeedback69DailySeedUpgradeKeepsCustomAndFrozen(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeDailyTasks(f.ctx); err != nil {
		t.Fatal(err)
	}
	var rows []db.DailyTaskRule
	db.GetIns().Order("id").Find(&rows)
	total := int64(0)
	for _, r := range rows {
		var n int64
		if _, err := fmt.Sscanf(r.Reward, "diamond,%d", &n); err != nil {
			t.Fatal(r.Reward)
		}
		total += n
	}
	if len(rows) != 7 || total != 200 {
		t.Fatal("daily curve", len(rows), total)
	}
	if err := db.GetIns().Model(&db.DailyTaskRule{}).Where("id=?", "local_copy").Updates(map[string]any{"reward": "diamond,99", "source": "运营配置"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.DailyTaskRule{}).Where("id=?", "local_training").Updates(map[string]any{"reward": "coin,1000", "source": constUtil.DailyTask69LegacySource}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.DailyTaskRule{}).Where("id=?", "local_recruit").Updates(map[string]any{"total": 5, "reward": "coin,1000", "source": constUtil.DailyTask69LegacySource}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyTasks(f.ctx); err != nil {
		t.Fatal(err)
	}
	var custom, legacy, adjusted db.DailyTaskRule
	db.GetIns().First(&custom, "id=?", "local_copy")
	db.GetIns().First(&legacy, "id=?", "local_training")
	db.GetIns().First(&adjusted, "id=?", "local_recruit")
	if custom.Reward != "diamond,99" || legacy.Reward != "diamond,20" || adjusted.Reward != "coin,1000" || adjusted.Total != 5 {
		t.Fatal("seed upgrade overwrote custom or missed original", custom, legacy, adjusted)
	}
}

func TestFeedback69EquipmentActualClaimsAndStaleProgress(t *testing.T) {
	f := p1Setup(t)
	loadOpenServerTaskFixture(t, f.s)
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := LocalPlayNow(f.ctx)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if err := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", constUtil.OpenServerActivityID).Updates(map[string]any{"start_at": start.UnixMilli(), "end_at": start.AddDate(0, 0, 14).UnixMilli(), "reward_end_at": start.AddDate(0, 0, 15).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	// 有限装备前提；本测试通过原活动查询/领取事务验证条件与幂等。
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Formation = nil
		g.Warriors = nil
		g.Equipment = nil
		for i := int32(1); i <= 5; i++ {
			guid := fmt.Sprintf("gear69-%d", i)
			g.Warriors = append(g.Warriors, &pb.Warrior{Guid: proto.String(guid), Id: proto.String("hero_043")})
			slot := &pb.FormationItem{FormatPos: proto.Int32(i), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: proto.String(guid), Id: proto.String("hero_043")}}}
			for pos := int32(1); pos <= 4; pos++ {
				id := []string{"equip_015", "equip_016", "equip_017", "equip_018"}[pos-1]
				eg := fmt.Sprintf("gear69-%d-%d", i, pos)
				g.Equipment = append(g.Equipment, &pb.Equipment{Guid: proto.String(eg), EquipId: proto.String(id), Level: proto.Int32(40)})
				slot.Things = append(slot.Things, &pb.FormationThing{Pos: proto.Int32(pos), Guid: proto.String(eg), Id: proto.String(id)})
			}
			g.Formation = append(g.Formation, slot)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []int32{15, 16, 17} {
		if _, err := f.s.openServerActivityAt(f.ctx, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(1), Tap: proto.Int32(3)}, 0, now); err != nil {
			t.Fatal(err)
		}
		var state db.OpenServerActivityState
		if err := db.GetIns().First(&state, "player_id=? AND task_id=?", f.id, task).Error; err != nil || !state.Achieved {
			t.Fatal("qualified equipment task", task, state, err)
		}
		req := &pb.CSGetOpenServerReward{ItemId: proto.Int32(task)}
		first, err := f.s.openServerActivityAt(f.ctx, f.id, req, 69700+int64(task), now)
		if err != nil {
			t.Fatal("qualified claim", task, err)
		}
		replay, err := f.s.openServerActivityAt(f.ctx, f.id, req, 69700+int64(task), now)
		if err != nil || !proto.Equal(first[0], replay[0]) {
			t.Fatal("equipment receipt", task, err)
		}
	}
	// 未达到60级的任务先存1；缺槽时需准确变成0，不能永远显示1/60。
	var tasks []db.OpenServerActivityTask
	if err := db.GetIns().Where("task_type=6 AND target>40").Order("target").Find(&tasks).Error; err != nil || len(tasks) == 0 {
		t.Fatal(err)
	}
	target := tasks[0]
	if err := db.GetIns().Save(&db.OpenServerActivityState{PlayerID: f.id, TaskID: target.ID, Progress: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Formation[0].Things = g.Formation[0].Things[:1]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	db.GetIns().First(&row, f.id)
	p, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	// 当前日已开放所有第一周任务，单独对齐该task的开放日。
	later := start.AddDate(0, 0, int(target.OpenDay-1)).Add(time.Hour)
	if err = f.s.refreshOpenServerProgressAt(db.GetIns(), p, g, later); err != nil {
		t.Fatal(err)
	}
	var zero db.OpenServerActivityState
	if err := db.GetIns().First(&zero, "player_id=? AND task_id=?", f.id, target.ID).Error; err != nil || zero.Progress != 0 || zero.Achieved {
		t.Fatal("stale equipment progress", zero, err)
	}
}
