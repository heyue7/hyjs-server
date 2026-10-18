package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP13UnionFightFactoryActual 覆盖真实400矿来源消耗、原角色/NPC/120s/8.67s、冷恢复和首车唯一实例。
func TestP13UnionFightFactoryActualResource400OriginalTimingRoleColdAndUniqueCar(t *testing.T) {
	p13GuildFactoryActual400(t, false)
}

func p13GuildFactoryActual400(t *testing.T, mission bool) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if mission {
		t.Cleanup(func() { db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("1=1").Update("enabled", false) })
		if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "local-factory-task-v1", "source": "original desc09 and real400 field factory; local task prestige5", "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "factory_mission_prestige": 5, "win_prestige": 50, "lose_prestige": 25, "published_at": now().UnixMilli() - 1}).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.s.TickP13UnionFightSettlement(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离原4车厂；明确本地出生和HP"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for factory := int64(1); factory <= 4; factory++ {
		if err = db.GetIns().Create(&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: factory, Enabled: true, ChariotHP: 1000 + float64(factory), PublishedAt: now().UnixMilli(), Source: "明确本地首辆战车HP1000+原厂ID，未当原公式/结算"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for camp, npc := range map[int64]string{0: "2", 1: "5"} {
		point, err := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 2; index++ {
			if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("factory-mining-%d-%d", camp, index), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "隔离原矿型1/原回收NPC位置/明确本地产量50和双点，不声称原spawn"}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, id := range []int64{ids[0], ids[10], ids[11]} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	if mission {
		frames, err := f.s.p13UFMissionQuery(f.ctx, ids[10], &pb.CSUFUnionMissionList{}, now())
		if err != nil {
			t.Fatal(err)
		}
		infos := frames[0].(*pb.SCUFUnionMissionList).GetMissionInfos()
		if len(infos) != 1 || infos[0].GetIsComplete() != 0 || infos[0].GetProgress() != "0/1" || infos[0].GetRewardNum() != 5 {
			t.Fatal("original unpublished activity has no completed factory task", infos)
		}
	}
	var actor db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	recycler, npc, factory := "2", "1", int64(3)
	if actor.Camp == 1 {
		recycler, npc, factory = "5", "4", 4
	}
	guid := p13UFFieldGUID(match.ID, "factory", fmt.Sprint(factory))
	quote, err := f.s.p13UFFactoryCost(f.ctx, ids[0], &pb.CSUFOpenFactoryCost{FactoryId: proto.Int64(guid)}, now())
	if err != nil || quote[0].(*pb.SCUFOpenFactoryCost).GetCost().GetResourceNum() != 400 {
		t.Fatal("original400 cost actually takes factory GUID", err)
	}
	if _, err = f.s.p13UFFactoryCost(f.ctx, ids[10], &pb.CSUFOpenFactoryCost{FactoryId: proto.Int64(guid)}, now()); err == nil {
		t.Fatal("ordinary cost permission")
	}
	if _, err = f.s.p13UFFactoryCost(f.ctx, ids[0], &pb.CSUFOpenFactoryCost{FactoryId: proto.Int64(factory)}, now()); err == nil {
		t.Fatal("tableID mistaken for scene GUID")
	}
	request := &pb.CSUFFieldCariotFactoryOpen{FactoryGuid: proto.Int64(guid), NpcId: proto.String(npc)}
	if _, err = f.s.p13UFFactoryOpen(f.ctx, ids[0], request, 800, now()); err == nil {
		t.Fatal("opened duringReady/no actual400")
	}
	clock.Store(first + 20000)
	point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		index := 0
		if i >= 5 {
			index = 1
		}
		resource := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("factory-mining-%d-%d", actor.Camp, index))
		if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(resource)}, now()); err != nil {
			t.Fatal(err)
		}
		clock.Add(3000)
		if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFResourceDeposit(f.ctx, ids[0], &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 900+int64(i), now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.p13UFFactoryOpen(f.ctx, ids[0], request, 800, now()); err == nil {
		t.Fatal("opened far from own originalNPC")
	}
	clock.Add(20000)
	point, _ = fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	openedAt := clock.Load()
	replies, err := f.s.p13UFFactoryOpen(f.ctx, ids[0], request, 800, now())
	if err != nil {
		t.Fatal(err)
	}
	opened := replies[0].(*pb.SCUFCariotFactoryOpen)
	if opened.GetFactory().GetIsOpen() != 1 || replies[1].(*pb.SCUFCampResourceUpdate).GetResource().GetResourceNum() != 0 {
		t.Fatal("actual400 not spent once")
	}
	duplicate, err := f.s.p13UFFactoryOpen(f.ctx, ids[0], request, 800, now())
	if err != nil || !proto.Equal(opened, duplicate[0]) {
		t.Fatal("persistent exact original open receipt", err)
	}
	if _, err = f.s.p13UFFactoryOpen(f.ctx, ids[11], request, 0, now()); err == nil {
		t.Fatal("second officer opened already opened factory")
	}
	var camp db.P13UnionFightCampResource
	if err = db.GetIns().First(&camp, "match_id=? AND guild_id=?", match.ID, actor.GuildID).Error; err != nil || camp.Amount != 0 {
		t.Fatal("camp resource400 accounting", err, camp)
	}
	if mission {
		frames, err := f.s.p13UFMissionQuery(f.ctx, ids[10], &pb.CSUFUnionMissionList{}, now())
		if err != nil {
			t.Fatal(err)
		}
		info := frames[0].(*pb.SCUFUnionMissionList).GetMissionInfos()[0]
		if info.GetIsComplete() != 1 || info.GetMissionBalanceType() != 0 || info.GetMissionDesc() != "desc_battel_mission_09" || info.GetProgress() != "1/1" {
			t.Fatal("original real opened factory task", info)
		}
		if p1Count(t, &db.P13UnionFightGuildMissionFact{}, "match_id=? AND guild_id=?", match.ID, actor.GuildID) != 1 {
			t.Fatal("actual task fact not unique")
		}
		var prestige db.P13UnionFightSettlementPrestige
		if err = db.GetIns().First(&prestige, "guild_id=?", actor.GuildID).Error; err != nil || prestige.Score != 5 {
			t.Fatal("actual task prestige once", prestige, err)
		}
		if err = db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": false, "factory_mission_prestige": 999}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var stored db.P13UnionFightFactoryState
	read := func() {
		t.Helper()
		if err := db.GetIns().First(&stored, "match_id=? AND guid=?", match.ID, guid).Error; err != nil {
			t.Fatal(err)
		}
	}
	read()
	if stored.DueAt != openedAt+120000 || stored.AnimationAt != openedAt+111330 {
		t.Fatal("original120s/8.67s timing", stored)
	}
	originalDue := stored.DueAt
	if err = db.GetIns().Model(&stored).Update("due_at", originalDue-1).Error; err != nil {
		t.Fatal(err)
	}
	read()
	if _, err = p13UFFactoryFrame(&stored); err == nil {
		t.Fatal("tampered persistent deadline bypassed full state SHA")
	}
	if err = db.GetIns().Model(&stored).Update("due_at", originalDue).Error; err != nil {
		t.Fatal(err)
	}
	read()
	clock.Store(stored.AnimationAt - 1)
	if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	read()
	frame, err := p13UFFactoryFrame(&stored)
	if err != nil || frame.GetState() != 0 {
		t.Fatal("early original animation", err)
	}
	clock.Add(1)
	if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	read()
	frame, _ = p13UFFactoryFrame(&stored)
	if frame.GetState() != 1 || stored.CompletedAt != 0 {
		t.Fatal("original prepare complete state1")
	}
	if err = db.GetIns().Model(&db.P13UnionFightFactorySource{}).Where("day=?", match.Day).Update("chariot_hp", 999999).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(stored.DueAt - 1)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if mission {
		frames, err := f.s.p13UFMissionQuery(f.ctx, ids[10], &pb.CSUFUnionMissionList{}, now())
		if err != nil {
			t.Fatal(err)
		}
		info := frames[0].(*pb.SCUFUnionMissionList).GetMissionInfos()[0]
		if info.GetRewardNum() != 5 || info.GetIsComplete() != 1 {
			t.Fatal("cold source closure overwrote task publication", info)
		}
	}
	read()
	if stored.CompletedAt != 0 || stored.ChariotHP != 1000+float64(factory) {
		t.Fatal("cold1ms or current source overwrote frozen source")
	}
	clock.Add(1)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightFactory(f.ctx, now()) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	} else {
		for i := 0; i < 8; i++ {
			if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	read()
	frame, err = p13UFFactoryFrame(&stored)
	if err != nil || frame.GetState() != 3 || frame.GetCariotGuid() == 0 || stored.CompletedAt != stored.DueAt {
		t.Fatal("original complete/unique car", err)
	}
	car := &pb.InstUFCariot{}
	if proto.Unmarshal(stored.CarFrame, car) != nil || car.GetMaxHp() != 1000+float64(factory) || car.GetHp() != car.GetMaxHp() || car.GetFactoryGuid() != guid || car.GetCariotId() != int32(factory) {
		t.Fatal("exact original car identity/frozen localHP")
	}
	query, err := f.s.p13UFFieldElements(f.ctx, ids[0], []int64{car.GetGuid()}, now())
	if err != nil || len(query) != 1 || query[0].(*pb.SCUFCaropt).GetCariot()[0].GetGuid() != car.GetGuid() {
		t.Fatal("original exact car GUID query", err)
	}
	frames, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSUFInit{}, now())
	if err != nil {
		t.Fatal(err)
	}
	full := frames[0].(*pb.SCUnionFightTransfer)
	found := false
	for _, live := range full.FactoryInfos {
		if live.GetGuid() == guid {
			found = proto.Equal(live, frame)
		}
	}
	if !found {
		t.Fatal("full init lost persisted original factory")
	}
	duplicate, err = f.s.p13UFFactoryOpen(f.ctx, ids[0], request, 800, now())
	if err != nil || !proto.Equal(opened, duplicate[0]) {
		t.Fatal("cold completed canonical open receipt", err)
	}
}
