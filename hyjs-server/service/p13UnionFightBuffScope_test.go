package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestP13UnionFightBuffScopeActualMineShopCasterExitReenterColdAndEarnedMovement(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	for _, table := range []any{&db.P13UnionFightShopSource{}, &db.P13UnionFightResourceSource{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, model := range []any{&db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightSpeedItemOffer{}, &db.P13UnionFightSpeedItemEffect{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离原冲锋地图/本地HP"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightSpeedItemSource{Day: match.Day, ItemID: "ufitem_015", Enabled: true, Multiplier: 2, AuthorityMask: 4, PublishedAt: now().UnixMilli(), Source: "原倍率/UseCondition1未知，隔离local2倍/current冻结Chair角色2可用"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: "ufitem_011", Enabled: true, HPPercent: 20, PublishedAt: now().UnixMilli(), Source: "原比例未知/local20共享CD"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"ufitem_015", "ufitem_011"} {
		if err := db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: item, ItemID: item, Enabled: true, Price: 10, Quantity: 1, BuyCount: 3, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "原报价未知/local矿币10买1"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("speed-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "真实矿1/原回收点/local50"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var roster p13UFMatchRoster
	if err = json.Unmarshal(match.Roster, &roster); err != nil {
		t.Fatal(err)
	}
	guild := p13GuildID(t, f, id, now())
	enemy := int64(0)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			if fighter.GuildID != guild {
				enemy = fighter.PlayerID
				break
			}
		}
	}
	if enemy <= 0 {
		t.Fatal("actual opposing roster missing")
	}
	for _, player := range []int64{id, ids[11], ids[10], enemy} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, player, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, player, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(player int64) (db.P13UnionFightFieldPlayer, *pb.InstUFPlayer) {
		t.Helper()
		var row db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&row, "match_id=? AND player_id=?", match.ID, player).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			t.Fatal(err)
		}
		return row, frame
	}
	actor, _ := read(id)
	direction := float32(1)
	if actor.Camp == 1 {
		direction = -1
	}
	recycler, shop := "2", "3"
	if actor.Camp == 1 {
		recycler, shop = "5", "6"
	}
	point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	clock.Store(first + 20000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("speed-mine-%d", actor.Camp)))}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(3000)
	if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 500, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(10000)
	point, _ = fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	for i, player := range []int64{id, ids[11], ids[10], enemy} {
		target := proto.Clone(point).(*pb.PointNew)
		if i == 1 {
			target.X = proto.Float32(target.GetX() + direction*2)
		}
		if i == 2 {
			target.X = proto.Float32(target.GetX() + direction*10)
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, player, &pb.CSMove{PointNew: target}, now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(shop)}, now()); err != nil {
		t.Fatal(err)
	}
	for i, item := range []string{"ufitem_015", "ufitem_015", "ufitem_011"} {
		if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(item), NpcId: proto.String(shop)}, 600+int64(i), now()); err != nil {
			t.Fatal(err)
		}
	}
	_, inventory := read(id)
	if inventory.GetOwnToken() != 120 {
		t.Fatal("actual mining150 bought30", inventory.GetOwnToken())
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true)
	request := &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_015")}
	replies, err := f.s.p13UFItemUse(f.ctx, id, request, 800, now())
	if err != nil {
		t.Fatal(err)
	}
	on := replies[1].(*pb.SCUFElementBuffOn)
	if len(on.TargetGuid) != 2 {
		t.Fatal("actual ally3m selector", on)
	}
	for _, player := range []int64{id, ids[11]} {
		_, frame := read(player)
		if len(frame.Buffs) != 1 || frame.Buffs[0].GetBuffId() != "uf_buff_008" || frame.Buffs[0].GetMountGuid() != player {
			t.Fatal("actual original buff mount", frame.Buffs)
		}
	}
	for _, player := range []int64{ids[10], enemy} {
		_, frame := read(player)
		if len(frame.Buffs) != 0 {
			t.Fatal("far or opposing target buffed")
		}
	}

	clock.Store(first + 35000)
	left, err := f.s.p13UFFieldLeave(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now())
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 || len(left[1].(*pb.SCUFElementBuffOff).BuffGuid) != 2 {
		t.Fatal("caster exit must revoke original friend and self GUIDs", left)
	}
	ally, allyFrame := read(ids[11])
	if len(allyFrame.Buffs) != 0 {
		t.Fatal("ally retained exited caster buff")
	}
	base := float64(5)
	earned, rate, err := p13UFSpeedMoveAllowance(db.GetIns(), &ally, base, first+37000)
	if err != nil || earned != base*6 || rate != base {
		t.Fatal("early end earned2s boost plus2s base", earned, rate, err)
	}
	var effects []db.P13UnionFightSpeedItemEffect
	if err = db.GetIns().Where("match_id=?", match.ID).Find(&effects).Error; err != nil {
		t.Fatal(err)
	}
	for _, effect := range effects {
		if effect.EndedAt != first+35000 {
			t.Fatal("effect not atomically closed", effect)
		}
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	self, selfFrame := read(id)
	if len(selfFrame.Buffs) != 0 {
		t.Fatal("sameclock reentry resurrected speed")
	}
	_, rate, err = p13UFSpeedMoveAllowance(db.GetIns(), &self, base, first+36000)
	if err != nil || rate != base {
		t.Fatal("sameclock reentry inherited bonus", rate, err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightSpeedItem(f.ctx, time.UnixMilli(first+39000)); err != nil {
		t.Fatal(err)
	}
	_, cold := read(ids[11])
	if len(cold.Buffs) != 0 {
		t.Fatal("cold peer buff resurrected")
	}
}

func TestP13UnionFightBuffScopeActualAuraFastExitReenterNoFurtherHealing(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	for _, table := range []any{&db.P13UnionFightShopSource{}, &db.P13UnionFightResourceSource{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, model := range []any{&db.P13UnionFightHealAuraSource{}, &db.P13UnionFightHealAuraOffer{}, &db.P13UnionFightHealAuraEffect{}, &db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离原回复法阵地图/本地HP"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightHealAuraSource{Day: match.Day, ItemID: "ufitem_013", Enabled: true, HPPercent: 20, Radius: 3, TickMillis: 1000, AuthorityMask: 4, PublishedAt: now().UnixMilli(), Source: "原数额/radius/tick/UseCondition2未知，隔离local20%/3m/1s/current冻结Chair资格"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: "ufitem_011", Enabled: true, HPPercent: 20, PublishedAt: now().UnixMilli(), Source: "原比例未知/local20共享CD"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"ufitem_013", "ufitem_011"} {
		if err := db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: item, ItemID: item, Enabled: true, Price: 10, Quantity: 1, BuyCount: 3, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "原报价未知/local矿币10买1"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("speed-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "真实矿1/原回收点/local50"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var roster p13UFMatchRoster
	if err = json.Unmarshal(match.Roster, &roster); err != nil {
		t.Fatal(err)
	}
	guild := p13GuildID(t, f, id, now())
	enemy := int64(0)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			if fighter.GuildID != guild {
				enemy = fighter.PlayerID
				break
			}
		}
	}
	if enemy <= 0 {
		t.Fatal("actual opposing roster missing")
	}
	for _, player := range []int64{id, ids[11], ids[10], enemy} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, player, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, player, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(player int64) (db.P13UnionFightFieldPlayer, *pb.InstUFPlayer) {
		t.Helper()
		var row db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&row, "match_id=? AND player_id=?", match.ID, player).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			t.Fatal(err)
		}
		return row, frame
	}
	actor, _ := read(id)
	direction := float32(1)
	if actor.Camp == 1 {
		direction = -1
	}
	recycler, shop := "2", "3"
	if actor.Camp == 1 {
		recycler, shop = "5", "6"
	}
	point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	clock.Store(first + 20000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("speed-mine-%d", actor.Camp)))}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(3000)
	if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 500, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(10000)
	point, _ = fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	for i, player := range []int64{id, ids[11], ids[10], enemy} {
		target := proto.Clone(point).(*pb.PointNew)
		if i == 1 {
			target.X = proto.Float32(target.GetX() + direction*2)
		}
		if i == 2 {
			target.X = proto.Float32(target.GetX() + direction*10)
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, player, &pb.CSMove{PointNew: target}, now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(shop)}, now()); err != nil {
		t.Fatal(err)
	}
	for i, item := range []string{"ufitem_013", "ufitem_013", "ufitem_011"} {
		if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(item), NpcId: proto.String(shop)}, 600+int64(i), now()); err != nil {
			t.Fatal(err)
		}
	}
	_, inventory := read(id)
	if inventory.GetOwnToken() != 120 {
		t.Fatal("actual mining150 bought30", inventory.GetOwnToken())
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true)
	request := &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_013")}
	// 只准备有限受伤帧，不伪造原P6战场胜负或实际伤害producer。
	prepare := func(player int64) {
		t.Helper()
		row, frame := read(player)
		for i, maxHP := range frame.FightInfo.MaxHpPos {
			frame.FightInfo.HpPos[i] = maxHP * .1
			if i > 0 {
				frame.FightInfo.HpPos[i] = 0
			}
		}
		if err := p13UFFieldSaveActor(db.GetIns(), &row, frame); err != nil {
			t.Fatal(err)
		}
	}
	for _, player := range []int64{id, ids[11], ids[10], enemy} {
		prepare(player)
	}

	if _, err = f.s.p13UFItemUse(f.ctx, id, request, 800, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 34000)
	if err = f.s.TickP13UnionFightHealAura(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	_, before := read(id)
	left, err := f.s.p13UFFieldLeave(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now())
	if err != nil || len(left) != 2 {
		t.Fatal("aura leave must return original off", left, err)
	}
	if len(left[1].(*pb.SCUFElementBuffOff).BuffGuid) != 1 {
		t.Fatal("aura off GUID", left)
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	_, reentered := read(id)
	if len(reentered.Buffs) != 0 {
		t.Fatal("fast reentry resurrected aura")
	}
	clock.Store(first + 36000)
	if err = f.s.TickP13UnionFightHealAura(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	_, after := read(id)
	if !proto.Equal(before.FightInfo, after.FightInfo) {
		t.Fatal("revoked aura healed after reentry")
	}
	var effect db.P13UnionFightHealAuraEffect
	if err = db.GetIns().First(&effect, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if effect.EndedAt != first+34000 || effect.NextTickAt != first+35000 {
		t.Fatal("cancel advanced unseen healing", effect)
	}
}
