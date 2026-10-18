package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func growthRecruitPreservePolicy(t *testing.T) {
	t.Helper()
	var before db.GrowthRecruitPolicy
	existed := db.GetIns().Migrator().HasTable(&before) && db.GetIns().First(&before, 1).Error == nil
	t.Cleanup(func() {
		if existed {
			if err := db.GetIns().Save(&before).Error; err != nil {
				t.Error(err)
			}
		} else if db.GetIns().Migrator().HasTable(&before) {
			if err := db.GetIns().Where("id=1").Delete(&before).Error; err != nil {
				t.Error(err)
			}
		}
	})
}

// This checks the actual SQL rotation readers, without forcing any random draw to a selected hero.
func TestGrowthRecruitAll74PartnerChannels(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, day := range []int{19, 22} { // Monday and Thursday in the same original UTC+8 week.
		at := time.Date(2026, 10, day, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
		for _, kind := range []int32{1, 2} {
			if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				rewards, _, err := f.s.loadP4Rotation(tx, kind, at)
				for _, reward := range rewards {
					covered[reward.ResourceID] = true
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(covered) != 58 {
		t.Fatal("ordinary/strong real two-window coverage", len(covered))
	}
	_, king, err := f.s.loadGameplayPool(db.GetIns(), "recruit_3")
	if err != nil {
		t.Fatal(err)
	}
	red := map[string]bool{}
	for space := int32(1); space <= 3; space++ {
		rewards, reds := f.s.kingRewards(king, space)
		for _, reward := range rewards {
			covered[reward.ResourceID] = true
		}
		for _, id := range reds {
			red[id] = true
		}
	}
	if len(red) != 13 || len(covered) != 71 {
		t.Fatal("actual king three-world coverage", len(red), len(covered))
	}
	for _, id := range []string{"hero_077", "hero_078", "hero_079"} {
		frag := f.s.table("Frag", "id", "frag_"+id)
		if frag == nil || frag["num"] != "150" || f.s.table("Hero", "id", id)["quality"] != "23" {
			t.Fatal("original OS fragment consumer", id)
		}
		covered[id] = true
	}
	count := 0
	for _, hero := range f.s.Tables["Hero"] {
		if hero["heroClass"] == "0" {
			count++
			if !covered[hero["id"]] {
				t.Fatal("unobtainable partner", hero["id"])
			}
		}
	}
	if count != 74 || len(covered) != count {
		t.Fatal("original complete partner set", count, len(covered))
	}
}

func TestGrowthRecruitPolicyMonthlyStopAndColdPreservation(t *testing.T) {
	f := p4Setup(t)
	growthRecruitPreservePolicy(t)
	at := time.Now()
	key := constUtil.P15RedOSSupplyPrefix + at.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	if err := f.s.InitializeGrowthRecruit(f.ctx); err != nil {
		t.Fatal(err)
	}
	allowed, err := p15RedOSSupplyAllowed(db.GetIns())
	if err != nil || allowed {
		t.Fatal("default-off publication", allowed, err)
	}
	if err = f.s.GrowthRecruitPublishCurrent(f.ctx, at); err != nil {
		t.Fatal(err)
	}
	var before db.P4CommerceRule
	if err = db.GetIns().First(&before, "id=?", key).Error; err != nil || !before.Enabled {
		t.Fatal("publish existing current month", err)
	}
	// Freeze through the original list, then preserve this exact catalog over stop/restart.
	p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(p4ExchangeKey(at))}, 0, at)
	if err = db.GetIns().First(&before, "id=?", key).Error; err != nil || len(before.Frozen) == 0 {
		t.Fatal("actual list freezes catalog", err)
	}
	if err = db.GetIns().Model(&before).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.GrowthRecruitPublishCurrent(f.ctx, at); err != nil {
		t.Fatal(err)
	}
	next := at.AddDate(0, 1, 0)
	if err = f.s.seedP15RedOSSupply(db.GetIns(), next); err != nil {
		t.Fatal(err)
	}
	var future db.P4CommerceRule
	if err = db.GetIns().First(&future, "id=?", constUtil.P15RedOSSupplyPrefix+next.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")).Error; err != nil || !future.Enabled {
		t.Fatal("next month inherits published policy", err)
	}
	f.restart(t)
	if err = f.s.InitializeGrowthRecruit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.seedP15RedOSSupply(db.GetIns(), at); err != nil {
		t.Fatal(err)
	}
	var after db.P4CommerceRule
	if err = db.GetIns().First(&after, "id=?", key).Error; err != nil || after.Enabled || !bytes.Equal(before.Frozen, after.Frozen) || !bytes.Equal(before.Rules, after.Rules) {
		t.Fatal("cold publication reset a stopped/frozen month", err)
	}
	if err = db.GetIns().Model(&db.GrowthRecruitPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.GrowthRecruitPublishCurrent(f.ctx, at); err != nil {
		t.Fatal(err)
	}
	allowed, err = p15RedOSSupplyAllowed(db.GetIns())
	if err != nil || allowed {
		t.Fatal("policy stop", allowed, err)
	}
	if err = f.s.seedP15RedOSSupply(db.GetIns(), at.AddDate(0, 2, 0)); err != nil {
		t.Fatal(err)
	}
	after = db.P4CommerceRule{}
	if err = db.GetIns().First(&after, "id=?", constUtil.P15RedOSSupplyPrefix+at.AddDate(0, 2, 0).In(time.FixedZone("UTC+8", 8*3600)).Format("200601")).Error; err != nil || after.Enabled {
		t.Fatal("stopped policy reopened a new month", err)
	}
}

func growthRecruitOffset(ctx context.Context, days int64) context.Context {
	return context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, days)
}

func growthRecruitInfo(t *testing.T, f *p1Fixture, ctx context.Context, kind int32) *pb.WarriorRecruit {
	t.Helper()
	response, err := f.s.recruitInfo(ctx, f.id, &pb.CSWarriorRecruitGet{CostType: []int32{kind}})
	if err != nil {
		t.Fatal(err)
	}
	entries := response[0].(*pb.SCWarriorRecruitGet).GetRecruit()
	if len(entries) != 1 {
		t.Fatal("original single-pool projection", entries)
	}
	return entries[0]
}

func TestGrowthRecruitCalendarRewindPreservesActualFreeUses(t *testing.T) {
	f, _ := localPlayActivitiesFixture(t)
	base := growthRecruitOffset(f.ctx, 0)
	next := growthRecruitOffset(f.ctx, 1)
	request := &pb.CSWarriorRecruit{CostType: proto.Int32(1), NumType: proto.Int32(1), Free: proto.Int32(1)}
	before, beforeGame := f.profile(t)
	for i, ctx := range []context.Context{base, next} {
		replies, err := f.s.warriorRecruit(ctx, f.id, request, int64(8100+i))
		if err != nil || len(replies[0].(*pb.SCWarriorRecruit).GetCost()) != 0 {
			t.Fatal("actual original free recruit", err)
		}
	}
	_, after := f.profile(t)
	if len(after.Warriors) != len(beforeGame.Warriors)+2 {
		t.Fatal("free draws did not create two real unique instances")
	}
	last := after.Recruit[1]
	nextFree, guarantee := last.NextFree, last.SinceGuarantee
	for _, ctx := range []context.Context{base, next, base} {
		if entry := growthRecruitInfo(t, f, ctx, 1); entry.GetNum() != 2 {
			t.Fatal("calendar revisit restored a spent free use", entry)
		}
	}
	f.restart(t)
	entry := growthRecruitInfo(t, f, next, 1)
	p, cold := f.profile(t)
	if entry.GetNum() != 2 || cold.Recruit[1].NextFree != nextFree || cold.Recruit[1].SinceGuarantee != guarantee || len(cold.Warriors) != len(after.Warriors) || p.GetDiamond() != before.GetDiamond() {
		t.Fatal("cold calendar changed cooldown, guarantee, inventory or price")
	}
}

func TestGrowthRecruitKingDailyCapSurvivesCalendarRevisit(t *testing.T) {
	f, _ := localPlayActivitiesFixture(t)
	base := growthRecruitOffset(f.ctx, 0)
	next := growthRecruitOffset(f.ctx, 1)
	// Finite level/funds and 49 historical attempts isolate the original 50th
	// operation and calendar boundary. This is not a natural currency source.
	p4Fund(t, f)
	state := db.KingRecruitState{PlayerID: f.id, Day: p1Day(LocalPlayNow(base)), Draws: 49}
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSWarriorRecruit{CostType: proto.Int32(3), NumType: proto.Int32(1), Free: proto.Int32(0)}
	if _, err := f.s.warriorRecruit(base, f.id, request, 8201); err != nil {
		t.Fatal(err)
	}
	if growthRecruitInfo(t, f, base, 3).GetLeftTimes() != 0 || growthRecruitInfo(t, f, next, 3).GetLeftTimes() != 50 || growthRecruitInfo(t, f, base, 3).GetLeftTimes() != 0 {
		t.Fatal("king calendar recreated original daily attempts")
	}
	before, beforeGame := f.profile(t)
	if _, err := f.s.warriorRecruit(base, f.id, request, 8202); err == nil {
		t.Fatal("51st king attempt accepted after calendar rewind")
	}
	after, afterGame := f.profile(t)
	if before.GetDiamond() != after.GetDiamond() || len(beforeGame.Warriors) != len(afterGame.Warriors) {
		t.Fatal("rejected king attempt consumed currency or awarded a warrior")
	}
}

func TestGrowthRecruitOSPrivateRefreshKeepsOldBytesAndMonthlyQuota(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	growthRecruitPreservePolicy(t)
	if err := f.s.InitializeGrowthRecruit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GrowthRecruitPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "published_at": 0}).Error; err != nil {
		t.Fatal(err)
	}
	now := LocalPlayNow(f.ctx)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "00000000-0000-4000-8000-000000008301"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	oldKey, err := localPlayExchangeKey(db.GetIns().WithContext(f.ctx), now)
	if err != nil {
		t.Fatal(err)
	}
	var old db.P4ResourceExchangeDirectory
	if err := db.GetIns().First(&old, "id=?", oldKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.GrowthRecruitPublishCurrent(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	// Explicit finite qualification/funds exercise the real original trade;
	// normal level85 and coin sourcing remain separate growth-chain evidence.
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.Level, p.Coin = proto.Int32(85), proto.Int64(5000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(oldKey)}, 0, now)
	var unchanged db.P4ResourceExchangeDirectory
	if err := db.GetIns().First(&unchanged, "id=?", oldKey).Error; err != nil || old.InputHash != unchanged.InputHash || !bytes.Equal(old.Bindings, unchanged.Bindings) {
		t.Fatal("publication rewrote the existing private directory", err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitExchange", "00000000-0000-4000-8000-000000008302"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	key, err := localPlayExchangeKey(db.GetIns().WithContext(f.ctx), now)
	if err != nil || key == oldKey {
		t.Fatal("explicit refresh did not create a new directory", err)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	var chosen *pb.ActivityShopItem
	count := 0
	for _, item := range listing.GetShopItemList() {
		for _, reward := range item.GetGetResource() {
			if reward.GetId() == "frag_hero_077" || reward.GetId() == "frag_hero_078" || reward.GetId() == "frag_hero_079" {
				count++
				if reward.GetId() == "frag_hero_077" {
					chosen = item
				}
			}
		}
	}
	if count != 3 || chosen == nil {
		t.Fatal("new private directory omitted original OS three fragments", count)
	}
	buy := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: chosen.Index, Num: proto.Int32(3), CostResources: chosen.CostResource, GetResources: chosen.GetResource}
	// Original quote fields are per-unit; the domain computes the 3x price.
	p4Trade(t, f, buy, 8303, now)
	child := constUtil.P15RedOSSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	var quotaBefore db.P4CommerceState
	if err := db.GetIns().First(&quotaBefore, "player_id=? AND rule_id=?", id, child).Error; err != nil {
		t.Fatal(err)
	}
	var progress p4CommerceProgress
	if json.Unmarshal(quotaBefore.Payload, &progress) != nil || progress.Total["frag_hero_077"] != 3 || progress.Bought["frag_hero_077"] != 3 {
		t.Fatal("OS original monthly/daily state owner", string(quotaBefore.Payload))
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitExchange", "00000000-0000-4000-8000-000000008304"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	newKey, err := localPlayExchangeKey(db.GetIns().WithContext(f.ctx), now)
	if err != nil {
		t.Fatal(err)
	}
	listing = p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(newKey)}, 0, now).(*pb.SCActivityShopInfo)
	for _, item := range listing.GetShopItemList() {
		if len(item.GetGetResource()) > 0 && item.GetGetResource()[0].GetId() == "frag_hero_077" {
			buy.ActivityId, buy.Index, buy.Num = proto.String(newKey), item.Index, proto.Int32(1)
			buy.CostResources, buy.GetResources = item.CostResource, item.GetResource
		}
	}
	if _, err := f.s.p4Commerce(f.ctx, id, buy, 8305, now); err == nil {
		t.Fatal("refresh reopened original spent daily OS quota")
	}
	var quotaAfter db.P4CommerceState
	if err := db.GetIns().First(&quotaAfter, "player_id=? AND rule_id=?", id, child).Error; err != nil || !bytes.Equal(quotaBefore.Payload, quotaAfter.Payload) {
		t.Fatal("refresh changed original monthly/daily OS progress", err)
	}
	if err := db.GetIns().First(&unchanged, "id=?", oldKey).Error; err != nil || old.InputHash != unchanged.InputHash || !bytes.Equal(old.Bindings, unchanged.Bindings) {
		t.Fatal("new private period rewrote old directory", err)
	}
}

func TestGrowthRecruitFreeThenRealGrowthRejectsOldWholeWarrior(t *testing.T) {
	f := p4Setup(t)
	// Strong recruitment is open at original level1 and every legitimate outcome has quality>12.
	request := &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(1)}
	first, err := f.s.warriorRecruit(f.ctx, f.id, request, 7001)
	if err != nil {
		t.Fatal(err)
	}
	reply := first[0].(*pb.SCWarriorRecruit)
	if len(reply.GetCost()) != 0 || len(reply.GetGet().GetWars()) != 1 {
		t.Fatal("free acquisition contract")
	}
	hero := reply.GetGet().GetWars()[0]
	if again, err := f.s.warriorRecruit(f.ctx, f.id, request, 7001); err != nil || !proto.Equal(first[0], again[0]) {
		t.Fatal("unconsumed exact receipt", err)
	}
	// Finite level/material/coin prerequisites verify the growth consumer; this is not a natural resource-source proof.
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(30)
		_, err := f.s.GrantFixedReward(p, g, "hero_exp_003,1;coin,1000000")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	var material *pb.Warrior
	for _, warrior := range g.Warriors {
		if warrior.GetId() == "hero_exp_003" {
			material = warrior
		}
	}
	if material == nil {
		t.Fatal("finite material was not an inventory instance")
	}
	_, err = f.s.warriorStrengthen(f.ctx, f.id, &pb.CSWarriorStrengthen{Guid: hero.Guid, Cost: []*pb.BaseResource{{Id: material.Id, Guid: material.Guid}}})
	if err != nil {
		t.Fatal(err)
	}
	_, g = f.profile(t)
	current, err := p5Warrior(g, hero.GetGuid())
	if err != nil || current.GetLevel() <= 1 {
		t.Fatal("actual recruit/growth inventory", err)
	}
	f.restart(t)
	if _, err = f.s.warriorRecruit(f.ctx, f.id, request, 7001); err == nil || !strings.Contains(err.Error(), constUtil.P5GrowthReceiptChanged) {
		t.Fatal("old complete warrior response survived real growth/cold reload", err)
	}
	_, cold := f.profile(t)
	persisted, err := p5Warrior(cold, hero.GetGuid())
	if err != nil || !proto.Equal(current, persisted) {
		t.Fatal("stale replay changed current inventory", err)
	}
}
