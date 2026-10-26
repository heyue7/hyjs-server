package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 有限50%伤/其它pose0仅负向投影准备，不制造真实war damage或获胜。
func TestP13UnionFightBattleProjectionActualShopWardOriginalPoseCurrentHPAndFightStates(t *testing.T) {
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
	if _, _, err = f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, id, now()); err == nil {
		t.Fatal("actual original resource mining state2 granted fighter benefit")
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

	t.Cleanup(func() { db.GetIns().Where("1=1").Delete(&db.P13UnionFightWarDrumSource{}) })
	row, frame := read(id)
	frame.State = proto.Int32(1)
	frame.FightInfo.Anger = proto.Float64(88.125)
	for i, hp := range frame.FightInfo.MaxHpPos {
		frame.FightInfo.HpPos[i] = hp * .5
		if i > 0 {
			frame.FightInfo.HpPos[i] = 0
		}
	}
	if err = p13UFFieldSaveActor(db.GetIns(), &row, frame); err != nil {
		t.Fatal(err)
	}
	live, applied, err := f.s.ProjectP13UnionFightFieldBattle(f.ctx, id, now())
	if err != nil {
		t.Fatal("original actor fightstate1 accepted", err)
	}
	if live.GetInst().GetAnger() != 88.125 {
		t.Fatal("current war AP was replaced by the entry snapshot", live.GetInst())
	}
	originalBattle := baseline(id)
	for i, hero := range live.Formation {
		if math.Abs(hero.Inst.GetCurHp()-frame.FightInfo.HpPos[i]) > 1e-8 || math.Abs(hero.Prop.GetAtt()-originalBattle.Formation[i].Prop.GetAtt()*1.2) > 1e-8 || len(applied[hero.GetGuid()]) < 2 {
			t.Fatal("exact livepose HP and ward audit", hero, applied)
		}
	}
	enemyBattle, enemyApplied, err := f.s.ProjectP13UnionFightFieldBattle(f.ctx, enemy, now())
	if err != nil {
		t.Fatal(err)
	}
	for guid, sources := range enemyApplied {
		applied[guid] = sources
	}
	frozen, err := f.s.FreezeBattleInput(model.FrozenBattleInput{Attacker: live, Defender: enemyBattle, Seed: 42, MaxRound: 40, BattleType: 14, AppliedProperties: applied})
	if err != nil {
		t.Fatal("original UF replaytype14 finite full input", err)
	}
	live.Formation[0].Inst.CurHp = proto.Float64(1)
	if frozen.Attacker.Formation[0].Inst.GetCurHp() == 1 {
		t.Fatal("frozen currentHP aliased")
	}
	if !proto.Equal(originalBattle, baseline(id)) {
		t.Fatal("projection altered frozen database battle")
	}
	frame.State = proto.Int32(6)
	if err = p13UFFieldSaveActor(db.GetIns(), &row, frame); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.ProjectP13UnionFightFieldBattle(f.ctx, id, now()); err != nil {
		t.Fatal("original FightState6 rejected", err)
	}
	frame.State = proto.Int32(2)
	if err = p13UFFieldSaveActor(db.GetIns(), &row, frame); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, id, now()); err == nil {
		t.Fatal("actual mining state2 treated as combat")
	}
	frame.State = proto.Int32(0)
	frame.FightInfo.MaxHpPos[0]++
	if err = p13UFFieldSaveActor(db.GetIns(), &row, frame); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.ProjectP13UnionFightFieldBattle(f.ctx, id, now()); err == nil {
		t.Fatal("unmatched original maxHp mapped into battle")
	}
	frame.FightInfo.MaxHpPos[0]--
	for i := range frame.FightInfo.HpPos {
		frame.FightInfo.HpPos[i] = 0
	}
	if err = p13UFFieldSaveActor(db.GetIns(), &row, frame); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.ProjectP13UnionFightFieldBattle(f.ctx, id, now()); err == nil {
		t.Fatal("dead pose resurrected to battle")
	}
	clock.Store(match.EndAt)
	if _, _, err = f.s.ProjectP13UnionFightFieldBattle(f.ctx, id, now()); err == nil {
		t.Fatal("End projection accepted")
	}
}

// Readonly input audit on dedicated schema; no combat outcome or warHP write.
func TestP13UnionFightBattleProjectionNativeRead(t *testing.T) {
	raw := os.Getenv("HYJS_P13_PROJECTION_NATIVE_PLAYER")
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
	at, err := strconv.ParseInt(os.Getenv("HYJS_P13_PROJECTION_NATIVE_AT"), 10, 64)
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
	projected, applied, err := f.s.ProjectP13UnionFightFieldBattle(f.ctx, id, time.UnixMilli(at))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		t.Fatal(err)
	}
	result := struct {
		PlayerID   int64     `json:"playerId"`
		Multiplier []float64 `json:"multiplier"`
		HPFraction []float64 `json:"hpFraction"`
		InputOnly  bool      `json:"inputOnly"`
		Applied    int       `json:"applied"`
	}{PlayerID: id, InputOnly: true}
	for i, hero := range projected.Formation {
		base := baseline.Formation[i]
		if base.Prop.GetAtt() <= 0 {
			t.Fatal("actual base attack missing")
		}
		result.Multiplier = append(result.Multiplier, hero.Prop.GetAtt()/base.Prop.GetAtt())
		result.HPFraction = append(result.HPFraction, hero.Inst.GetCurHp()/hero.Prop.GetHp())
		result.Applied += len(applied[hero.GetGuid()])
		found := false
		for j, pose := range frame.FightInfo.Index {
			if pose == hero.GetPose() {
				found = true
				if hero.Inst.GetCurHp() != frame.FightInfo.HpPos[j] {
					t.Fatal("actual live pose input mismatch")
				}
			}
		}
		if !found {
			t.Fatal("actual live pose missing")
		}
	}
	data, _ := json.Marshal(result)
	fmt.Println("GUILD_PROJECTION_NATIVE_INPUT " + string(data))
}
