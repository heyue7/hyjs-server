package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightWarDrumActual 实际矿币购买原014，具名local模型/Atk20%/3m/10秒与完整P6冻结输入深克隆。
func TestP13UnionFightWarDrumActualMineShopTotemFrozenAttackColdRadiusOverlapAndExit(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	for _, model := range []any{&db.P13UnionFightWarDrumSource{}, &db.P13UnionFightWarDrumOffer{}, &db.P13UnionFightWarDrumEffect{}, &db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightHealAuraSource{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离原冲锋地图/本地HP"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightWarDrumSource{Day: match.Day, ItemID: "ufitem_014", Enabled: true, AttackPercent: 20, Radius: 3, DurationMillis: 10000, AuthorityMask: 4, ModelName: constUtil.P13UnionFightWarDrumLocalModel, ModelScale: 1, PublishedAt: now().UnixMilli(), Source: "原model/totem1数字/UseCondition2未知，隔离local已核shield-prefab/Atk20%/3m/10s/Chair2资格"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: "ufitem_011", Enabled: true, HPPercent: 20, PublishedAt: now().UnixMilli(), Source: "原比例未知/local20共享CD"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"ufitem_014", "ufitem_011"} {
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
	for i, item := range []string{"ufitem_014", "ufitem_014", "ufitem_011"} {
		if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(p13UFTestShopQuote(t, shopQuotes, item)), NpcId: proto.String(shop)}, 600+int64(i), now()); err != nil {
			t.Fatal(err)
		}
	}
	_, inventory := read(id)
	if inventory.GetOwnToken() != 120 {
		t.Fatal("actual mining150 bought30", inventory.GetOwnToken())
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true)
	request := &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_014")}
	baseline := func(player int64) *pb.BattlePlayer {
		t.Helper()
		row, _ := read(player)
		battle := &pb.BattlePlayer{}
		if err := proto.Unmarshal(row.Battle, battle); err != nil {
			t.Fatal(err)
		}
		return battle
	}
	check := func(player int64, multiplier float64) (*pb.BattlePlayer, map[string][]string) {
		t.Helper()
		before := baseline(player)
		projected, applied, err := f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, player, now())
		if err != nil {
			t.Fatal(err)
		}
		for i, hero := range projected.Formation {
			if math.Abs(hero.Prop.GetAtt()-before.Formation[i].Prop.GetAtt()*multiplier) > 1e-8 || !proto.Equal(hero.Inst, before.Formation[i].Inst) {
				t.Fatal("explicit Att/currentHP mapping boundary", player, hero.Prop.GetAtt())
			}
			if multiplier == 1 && len(applied[hero.GetGuid()]) != 0 {
				t.Fatal("no eligible benefit got audit")
			}
			if multiplier > 1 && len(applied[hero.GetGuid()]) == 0 {
				t.Fatal("real benefit missing source provenance")
			}
		}
		return projected, applied
	}
	for _, player := range []int64{id, ids[11], ids[10], enemy} {
		check(player, 1)
	}
	used, err := f.s.p13UFItemUse(f.ctx, id, request, 800, now())
	if err != nil {
		t.Fatal(err)
	}
	created := used[1].(*pb.SCUFTotemCreate)
	if created.Totem.GetTotemId() != "1" || created.Totem.GetModelName() != constUtil.P13UnionFightWarDrumLocalModel || created.Totem.GetModelScale() != 1 || created.Totem.GetLastTime() != 10 || created.Totem.GetSourcePlayerGuid() != id {
		t.Fatal("actual local totem instance", created)
	}
	boosted, applied := check(id, 1.2)
	check(ids[11], 1.2)
	check(ids[10], 1)
	enemyInput, _ := check(enemy, 1)
	frozen, err := f.s.FreezeBattleInput(model.FrozenBattleInput{Attacker: boosted, Defender: enemyInput, Seed: 42, MaxRound: 40, BattleType: 2, AppliedProperties: applied})
	if err != nil {
		t.Fatal("stable generic P6 frozen input", err)
	}
	boosted.Formation[0].Prop.Att = proto.Float64(1)
	if frozen.Attacker.Formation[0].Prop.GetAtt() == 1 {
		t.Fatal("P6 frozen input aliased mutable battle")
	}
	check(id, 1.2)
	if _, err = f.s.p13UFItemUse(f.ctx, id, &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_011")}, 801, now()); err == nil {
		t.Fatal("original diamond class5s bypass")
	}
	db.GetIns().Model(&db.P13UnionFightWarDrumSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "attack_percent": 1, "radius": 1, "duration_millis": 1000, "model_scale": 2, "authority_mask": 1})
	clock.Store(first + 36000)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	cold, err := p13UFDrumFrames(db.GetIns(), match.ID, nil, now())
	if err != nil || len(cold) != 1 || cold[0].(*pb.SCUFTotemCreate).Totem.GetLastTime() != 7 || !proto.Equal(cold[0].(*pb.SCUFTotemCreate).Pos, created.Pos) {
		t.Fatal("cold totem moved/extended/local source drift", err)
	}
	check(id, 1.2)
	clock.Store(first + 38000)
	second, err := f.s.p13UFItemUse(f.ctx, id, request, 802, now())
	if err != nil {
		t.Fatal("actual second frozen stock", err)
	}
	if second[1].(*pb.SCUFTotemCreate).Totem.GetGuid() == created.Totem.GetGuid() {
		t.Fatal("second original instance GUID reused")
	}
	check(id, 1.2)
	row, _ := read(ids[11])
	outside := &pb.PointNew{X: proto.Float32(row.X + direction*10), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)}
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[11], &pb.CSMove{PointNew: outside}, now()); err != nil {
		t.Fatal(err)
	}
	check(ids[11], 1)
	clock.Store(first + 43000)
	if err = f.s.TickP13UnionFightWarDrum(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	cold, err = p13UFDrumFrames(db.GetIns(), match.ID, nil, now())
	if err != nil || len(cold) != 1 || cold[0].(*pb.SCUFTotemCreate).Totem.GetLastTime() != 5 {
		t.Fatal("independent expiry/cold remaining", err)
	}
	check(id, 1.2)
	leaving, err := f.s.p13UFFieldLeave(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now())
	if err != nil || len(leaving) != 2 || leaving[1].(*pb.SCUFTotemDestroy).GetGuid() != second[1].(*pb.SCUFTotemCreate).Totem.GetGuid() {
		t.Fatal("actual exit did not atomically revoke totem", err)
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	check(id, 1)
	cold, err = p13UFDrumFrames(db.GetIns(), match.ID, nil, now())
	if err != nil || len(cold) != 0 {
		t.Fatal("fast exit/reentry resurrected spent totem", err)
	}
	clock.Store(match.EndAt)
	if _, _, err = f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, id, now()); err == nil {
		t.Fatal("End stale benefit")
	}
	// 这里只验证来源明确属性进入完整冻结结构，没有原ward模型映射，也没有提前计算或结算战争胜者。
}

// TestP13UnionFightWarDrumNativeBenefit 只读专属真实HTTP产生的图腾/当前位置/原阵容，核对明确属性进入完整冻结结构。
func TestP13UnionFightWarDrumNativeBenefit(t *testing.T) {
	raw := os.Getenv("HYJS_P13_WAR_DRUM_NATIVE_PLAYER")
	if raw == "" {
		t.Skip("显式原图腾受益查询")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_") {
		t.Fatal("native ward benefit schema guard")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		t.Fatal("actual native target required")
	}
	at, err := strconv.ParseInt(os.Getenv("HYJS_P13_WAR_DRUM_NATIVE_AT"), 10, 64)
	if err != nil || at <= 0 {
		t.Fatal("explicit native scope clock required")
	}
	f := &p1Fixture{ctx: context.WithValue(context.Background(), constUtil.P13UnionFightClockContextKey{}, func() time.Time { return time.UnixMilli(at) }), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}, s: &GameService{}}
	f.open(t)
	defer func() { sql, _ := db.GetIns().DB(); sql.Close() }()
	if err = f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&actor, "player_id=? AND left_at=0", id).Error; err != nil {
		t.Fatal(err)
	}
	baseline := &pb.BattlePlayer{}
	if err = proto.Unmarshal(actor.Battle, baseline); err != nil {
		t.Fatal(err)
	}
	projected, applied, err := f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, id, time.UnixMilli(at))
	if err != nil {
		t.Fatal(err)
	}
	result := struct {
		PlayerID           int64     `json:"playerId"`
		Multiplier         []float64 `json:"multiplier"`
		Applied            int       `json:"applied"`
		CurrentHPUnchanged bool      `json:"currentHPUnchanged"`
	}{PlayerID: id, CurrentHPUnchanged: true}
	for i, hero := range projected.Formation {
		base := baseline.Formation[i]
		if base.Prop.GetAtt() <= 0 {
			t.Fatal("actual base attack missing")
		}
		result.Multiplier = append(result.Multiplier, hero.Prop.GetAtt()/base.Prop.GetAtt())
		result.Applied += len(applied[hero.GetGuid()])
		result.CurrentHPUnchanged = result.CurrentHPUnchanged && proto.Equal(hero.Inst, base.Inst)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("WAR_DRUM_NATIVE_BENEFIT " + string(data))
}
