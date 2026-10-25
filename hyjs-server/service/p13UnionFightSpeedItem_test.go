package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightSpeedItemActual 真实矿币购买冲锋，友方3米/5秒、共享钻石CD及跨过期移动距离积分保持一致。
func TestP13UnionFightSpeedItemActualMineShopFriendlyRadiusFiveSecondsColdRaceAndMovement(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
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
	shopQuotes := p13UFTestShopQuotes(t, f, id, shop, now())
	for i, item := range []string{"ufitem_015", "ufitem_015", "ufitem_011"} {
		if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(p13UFTestShopQuote(t, shopQuotes, item)), NpcId: proto.String(shop)}, 600+int64(i), now()); err != nil {
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
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := f.s.p13UFItemUse(f.ctx, id, request, 800, now()); errs <- err }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var effectCount int64
	db.GetIns().Model(&db.P13UnionFightSpeedItemEffect{}).Where("match_id=?", match.ID).Count(&effectCount)
	if effectCount != 2 {
		t.Fatal("same receipt recast effects", effectCount)
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_011")}, 801, now()); err == nil {
		t.Fatal("original diamond class sharedCD bypass")
	}
	cfg := original.table("GameConfigure", "StaticName", "UnionFightPlayerMoveSpeed")
	base, err := strconv.ParseFloat(cfg["NameValue"], 64)
	if err != nil {
		t.Fatal(err)
	}
	clock.Add(1000)
	for _, player := range []int64{id, ids[11]} {
		row, _ := read(player)
		target := &pb.PointNew{X: proto.Float32(row.X + direction*float32(base*2)), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)}
		moved, err := f.s.p13UFFieldMove(f.ctx, player, &pb.CSMove{PointNew: target}, now())
		if err != nil || moved[len(moved)-1].(*pb.SCUFElementMove).Speed[0] != float32(base*2) {
			t.Fatal("actual boosted move", err)
		}
	}
	for _, player := range []int64{ids[10], enemy} {
		row, _ := read(player)
		target := &pb.PointNew{X: proto.Float32(row.X + direction*float32(base*2)), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)}
		if _, err = f.s.p13UFFieldMove(f.ctx, player, &pb.CSMove{PointNew: target}, now()); err == nil {
			t.Fatal("nonselected player got boosted allowance")
		}
	}
	db.GetIns().Model(&db.P13UnionFightSpeedItemSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "multiplier": 9, "authority_mask": 1})
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	_, cold := read(id)
	if len(cold.Buffs) != 1 {
		t.Fatal("cold active buff absent")
	}
	clock.Store(first + 38800)
	row, _ := read(id)
	earned, currentRate, err := p13UFSpeedMoveAllowance(db.GetIns(), &row, base, clock.Load())
	if err != nil || math.Abs(earned-base*8.8) > 1e-8 || currentRate != base {
		t.Fatal("expiry split integral", earned, currentRate, err)
	}
	target := &pb.PointNew{X: proto.Float32(row.X + direction*float32(earned)), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)}
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: target}, now()); err != nil {
		t.Fatal("valid earned preexpiry distance rejected", err)
	}
	if err = f.s.TickP13UnionFightSpeedItem(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	_, expired := read(id)
	if len(expired.Buffs) != 0 {
		t.Fatal("expired persisted originalbuff")
	}
	clock.Store(first + 39000)
	row, _ = read(id)
	target.X = proto.Float32(row.X + direction*float32(base*2))
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: target}, now()); err == nil {
		t.Fatal("expired move still doubled")
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", false)
	retry, err := f.s.p13UFItemUse(f.ctx, id, request, 800, now())
	if err != nil || len(retry) != 1 || !proto.Equal(retry[0], replies[0]) {
		t.Fatal("cold exact receipt changed/reapplied", err)
	}
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFItemUse(f.ctx, id, request, 800, now()); err == nil {
		t.Fatal("End speed old receipt")
	}
}
