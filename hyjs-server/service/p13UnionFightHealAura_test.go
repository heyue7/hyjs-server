package service

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightHealAuraActual 实际矿币购买法阵、五秒逐tick团队恢复；受伤仅明确有限测试准备，真实伤害producer待P6。
func TestP13UnionFightHealAuraActualMineShopTeamTicksColdConcurrentClampDeadPoseAndExit(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
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
		// Team aura eligibility uses the original authenticated TCP session binding.
		p10SceneConnect(t, f, player, &p10SceneRecorder{})
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
	for i, item := range []string{"ufitem_013", "ufitem_013", "ufitem_011"} {
		if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(p13UFTestShopQuote(t, shopQuotes, item)), NpcId: proto.String(shop)}, 600+int64(i), now()); err != nil {
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
	replies, err := f.s.p13UFItemUse(f.ctx, id, request, 800, now())
	if err != nil {
		t.Fatal(err)
	}
	on := replies[1].(*pb.SCUFElementBuffOn)
	if len(on.TargetGuid) != 1 || on.TargetGuid[0] != id || on.Buff[0].GetBuffId() != "uf_buff_009" || on.Buff[0].GetMountGuid() != id {
		t.Fatal("original aura self emitter", on)
	}
	percent := func(player int64, value float64) {
		t.Helper()
		_, frame := read(player)
		for i, hp := range frame.FightInfo.HpPos {
			want := frame.FightInfo.MaxHpPos[i] * value
			if i > 0 {
				want = 0
			}
			if math.Abs(hp-want) > 1e-8 {
				t.Fatal("actual team HP/keep dead pose", player, hp, want)
			}
		}
	}
	percent(id, .1)
	percent(ids[11], .1)
	clock.Store(first + 33999)
	if err = f.s.TickP13UnionFightHealAura(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	percent(id, .1)
	concurrent := func() {
		t.Helper()
		if f.mysql.Database == "" {
			if err := f.s.TickP13UnionFightHealAura(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
			return
		}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightHealAura(f.ctx, now()) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	clock.Store(first + 34000)
	concurrent()
	percent(id, .3)
	percent(ids[11], .3)
	percent(ids[10], .1)
	percent(enemy, .1)
	if _, err = f.s.p13UFItemUse(f.ctx, id, &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_011")}, 801, now()); err == nil {
		t.Fatal("shared diamond5s bypass")
	}
	var state db.P13UnionFightHealAuraEffect
	if err = db.GetIns().First(&state, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if state.NextTickAt != first+35000 {
		t.Fatal("eight workers multiplied a tick", state)
	}
	db.GetIns().Model(&db.P13UnionFightHealAuraSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "hp_percent": 1, "radius": 1, "tick_millis": 5000, "authority_mask": 1})
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	cold, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSUFInit{}, now())
	if err != nil || len(cold[0].(*pb.SCUnionFightTransfer).Player.Buffs) != 1 {
		t.Fatal("cold original active aura", err)
	}
	clock.Store(first + 35000)
	concurrent()
	percent(id, .5)
	percent(ids[11], .5)
	// 原3米之外移动及时停止该友方后续tick，法阵中心仍随真实caster位置。
	row, _ := read(ids[11])
	point = &pb.PointNew{X: proto.Float32(row.X + direction*10), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)}
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[11], &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 38000)
	concurrent()
	percent(id, 1)
	percent(ids[11], .5)
	percent(ids[10], .1)
	percent(enemy, .1)
	if err = db.GetIns().First(&state, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if state.EndedAt != first+38000 || state.NextTickAt != first+39000 {
		t.Fatal("final5 ticks/expiry", state)
	}
	_, expired := read(id)
	if len(expired.Buffs) != 0 {
		t.Fatal("expired aura still present")
	}
	_, err = f.s.p13UFItemUse(f.ctx, id, request, 800, now())
	if err != nil {
		t.Fatal("legacy exact aura receipt", err)
	}
	percent(id, 1)
	clock.Store(first + 39000)
	if _, err = f.s.p13UFItemUse(f.ctx, id, request, 802, now()); err != nil {
		t.Fatal("actual second frozen source inventory", err)
	}
	if _, err = f.s.p13UFFieldHook(f.ctx, id, &pb.CSUFOnHook{OnHook: proto.Int32(1)}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 40000)
	concurrent()
	var second db.P13UnionFightHealAuraEffect
	if err = db.GetIns().First(&second, "match_id=? AND started_at=?", match.ID, first+39000).Error; err != nil {
		t.Fatal(err)
	}
	if second.EndedAt != first+39000 {
		t.Fatal("actual caster Hook did not revoke aura", second)
	}
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFItemUse(f.ctx, id, request, 800, now()); err == nil {
		t.Fatal("End original aura receipt")
	}
}

// TestP13UnionFightHealAuraNativePrepareHP 仅专属schema有限准备10%受伤，不授予库存/奖励或声称自然P6伤害。
func TestP13UnionFightHealAuraNativePrepareHP(t *testing.T) {
	raw := os.Getenv("HYJS_P13_HEAL_AURA_NATIVE_PLAYER")
	if raw == "" {
		t.Skip("显式原帧有限受伤准备")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_") {
		t.Fatal("native heal preparation schema guard")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		t.Fatal("explicit real target required")
	}
	ctx := context.Background()
	if err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	defer sql.Close()
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var row db.P13UnionFightFieldPlayer
		if err := tx.First(&row, "player_id=? AND left_at=0", id).Error; err != nil {
			return err
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			return err
		}
		if frame.GetState() != 0 && frame.GetState() != 5 {
			return fmt.Errorf("bounded alive preparation only")
		}
		if len(frame.GetFightInfo().GetMaxHpPos()) == 0 {
			return fmt.Errorf("actual frozen HP missing")
		}
		for i, maxHP := range frame.FightInfo.MaxHpPos {
			if maxHP <= 0 || math.IsNaN(maxHP) || math.IsInf(maxHP, 0) {
				return fmt.Errorf("invalid actual frozen maxHP")
			}
			frame.FightInfo.HpPos[i] = maxHP * .1
		}
		return p13UFFieldSaveActor(tx, &row, frame)
	})
	if err != nil {
		t.Fatal(err)
	}
}
