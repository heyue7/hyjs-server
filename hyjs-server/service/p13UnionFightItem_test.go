package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightItemActual 使用真实采集/上交/商店来源，HP受伤/死亡仅有限准备，不宣称完整共享战斗。
func TestP13UnionFightItemActualMiningShopThreeLifeCooldownColdRaceAndEnd(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	for _, table := range []any{&db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightItemState{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	id := ids[0]
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "生命物品测试明确本地出生HP"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range []string{"ufitem_011", "ufitem_012", "ufitem_016"} {
		percent := []int64{50, 100, 20}[i]
		if err = db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: item, Enabled: true, HPPercent: percent, PublishedAt: now().UnixMilli(), Source: fmt.Sprintf("隔离当地恢复%d%%，非原服数额", percent)}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: item, Enabled: true, ItemID: item, Price: 10, Quantity: 1, BuyCount: 2, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "隔离三生命物品真实采矿币消费；当地价格10，每种2次"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("life-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "隔离实际50资源矿与原回收NPC"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	req := func(item string) *pb.CSUFPlayerUseItem { return &pb.CSUFPlayerUseItem{ItemId: proto.String(item)} }
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 900, now()); err == nil {
		t.Fatal("preparation/loading used item")
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	read := func() *pb.InstUFPlayer {
		t.Helper()
		var row db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&row, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	camp := read().GetCamp()
	recycler, shop := "2", "3"
	if camp == 1 {
		recycler, shop = "5", "6"
	}
	clock.Store(first + 20000)
	recyclerPoint, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: recyclerPoint}, now()); err != nil {
		t.Fatal(err)
	}
	guid := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("life-mine-%d", camp))
	if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(guid)}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(3000)
	if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 100, now()); err != nil {
		t.Fatal(err)
	}
	if read().GetOwnToken() != 150 {
		t.Fatal("actual50 mining to150 war token")
	}
	clock.Add(10000)
	shopPoint, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: shopPoint}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(shop)}, now()); err != nil {
		t.Fatal(err)
	}
	for i, item := range []string{"ufitem_011", "ufitem_012", "ufitem_016"} {
		for n := 0; n < 2; n++ {
			if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(item), NpcId: proto.String(shop)}, 200+int64(i*2+n), now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if read().GetOwnToken() != 90 || len(read().Items) != 3 {
		t.Fatal("actual bought3 kinds2each cost60")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 900, now()); err == nil {
		t.Fatal("default closed new item use")
	}
	if err = db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	prepareHP := func(percent float64, state int32) {
		t.Helper()
		err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var row db.P13UnionFightFieldPlayer
			if err := tx.First(&row, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
				return err
			}
			frame, err := p13UFFieldReadActor(&row)
			if err != nil {
				return err
			}
			for i, maxHP := range frame.FightInfo.MaxHpPos {
				frame.FightInfo.HpPos[i] = maxHP * percent / 100
			}
			frame.State = proto.Int32(state)
			return p13UFFieldSaveActor(tx, &row, frame)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	prepareHP(10, 0)
	replies, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 900, now())
	if err != nil {
		t.Fatal(err)
	}
	receipt := proto.Clone(replies[0])
	frame := read()
	if len(replies) != 2 || frame.Items[0].GetLeftUseNum() != 1 || frame.GetUseDiamondItemCD() != 5 || frame.GetUseNormalItemCD() != 0 {
		t.Fatal("item inventory/independent5sCD", frame)
	}
	for i, hp := range frame.FightInfo.HpPos {
		if math.Abs(hp-frame.FightInfo.MaxHpPos[i]*.6) > 1e-8 {
			t.Fatal("frozen local50percent effect")
		}
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 900, now())
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if read().Items[0].GetLeftUseNum() != 1 {
		t.Fatal("retry double consumed")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 901, now()); err == nil {
		t.Fatal("same kind original cooldown")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_012"), 902, now()); err == nil {
		t.Fatal("revive shared diamond cooldown bypass")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_016"), 903, now()); err != nil {
		t.Fatal("independent normal not blocked by diamond", err)
	}
	if read().Items[2].GetLeftUseNum() != 1 {
		t.Fatal("normal actual inventory")
	}
	clock.Add(2000)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSUFInit{}, now())
	if err != nil {
		t.Fatal(err)
	}
	cold := frames[0].(*pb.SCUnionFightTransfer).Player
	if cold.GetUseDiamondItemCD() != 3 || cold.GetUseNormalItemCD() != 3 {
		t.Fatal("cold absolute cooldown reset/expired", cold)
	}
	db.GetIns().Model(&db.P13UnionFightItemSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "hp_percent": 1})
	clock.Add(3000)
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_012"), 904, now()); err == nil {
		t.Fatal("alive used resurrection")
	}
	prepareHP(0, 3)
	revived, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_012"), 904, now())
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 3 || read().GetState() != 0 || read().Items[1].GetLeftUseNum() != 1 {
		t.Fatal("actual revive changed state/inventory")
	}
	for i, hp := range read().FightInfo.HpPos {
		if hp != read().FightInfo.MaxHpPos[i] {
			t.Fatal("frozen local100percent changed by source1")
		}
	}
	clock.Add(5000)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		wins := make(chan bool, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(stamp int64) {
				defer wg.Done()
				_, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), stamp, now())
				wins <- err == nil
			}(1000 + int64(i))
		}
		wg.Wait()
		close(wins)
		count := 0
		for ok := range wins {
			if ok {
				count++
			}
		}
		if count != 1 {
			t.Fatal("different frames consumed last potion", count)
		}
	} else {
		if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 1000, now()); err != nil {
			t.Fatal(err)
		}
	}
	clock.Add(5000)
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 1100, now()); err == nil {
		t.Fatal("zero actual inventory")
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", false)
	retry, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 900, now())
	if err != nil || !proto.Equal(retry[0], receipt) {
		t.Fatal("hot closed exact original receipt", err)
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 1200, now()); err == nil {
		t.Fatal("unknown effect returned successful use")
	}
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_011"), 900, now()); err == nil {
		t.Fatal("End receipt kept item authority")
	}
}

// TestP13UnionFightItemNativePrepareHP 只为专属原帧测试准备受伤/死亡状态，不冒充战斗结果或资源来源。
func TestP13UnionFightItemNativePrepareHP(t *testing.T) {
	raw := os.Getenv("HYJS_P13_ITEM_NATIVE_PLAYER")
	if raw == "" {
		t.Skip("只启用显式专属道具HP准备")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_") {
		t.Fatal("native HP preparation schema guard")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		t.Fatal("explicit actual player missing")
	}
	state, err := strconv.ParseInt(os.Getenv("HYJS_P13_ITEM_NATIVE_STATE"), 10, 32)
	if err != nil || (state != 0 && state != 3) {
		t.Fatal("explicit bounded state")
	}
	percent := float64(10)
	if state == 3 {
		percent = 0
	}
	ctx := context.Background()
	if err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	defer sql.Close()
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.P13UnionFightFieldPlayer
		if err := tx.First(&row, "player_id=? AND left_at=0", id).Error; err != nil {
			return err
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			return err
		}
		if len(frame.Items) != 3 {
			return fmt.Errorf("native actual bought3 inventory missing")
		}
		for i, maximum := range frame.FightInfo.MaxHpPos {
			frame.FightInfo.HpPos[i] = maximum * percent / 100
		}
		frame.State = proto.Int32(int32(state))
		return p13UFFieldSaveActor(tx, &row, frame)
	})
	if err != nil {
		t.Fatal(err)
	}
}
