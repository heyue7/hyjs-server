package service

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightCarItemActual 首车真实原120秒timer→真实矿币店购→大小炸弹消耗/破车/cold，车HP为具名本地来源。
func TestP13UnionFightCarItemActualNaturalFirstCarsMineShopBombsColdRaceDeadAndSHA(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	id := ids[0]
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, table := range []any{&db.P13UnionFightCarItemSource{}, &db.P13UnionFightCarItemOffer{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightItemState{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightFactorySource{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "炸弹实际来源隔离原地图/本地初始HP"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range []string{"ufitem_017", "ufitem_018"} {
		if err = db.GetIns().Create(&db.P13UnionFightCarItemSource{Day: match.Day, ItemID: item, Enabled: true, HPPercent: []int64{20, 60}[i], PublishedAt: now().UnixMilli(), Source: "隔离当地20/60%大小炸弹；原数额未知"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: "ufitem_016", Enabled: true, HPPercent: 20, PublishedAt: now().UnixMilli(), Source: "普通瓶实际共CD当地20%"}).Error; err != nil {
		t.Fatal(err)
	}
	for factoryID := int64(1); factoryID <= 2; factoryID++ {
		if err = db.GetIns().Create(&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: factoryID, Enabled: true, ChariotHP: 1000 + float64(factoryID), PublishedAt: now().UnixMilli(), Source: "隔离主厂首车初始HP1000+原ID，非原公式"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []string{"ufitem_017", "ufitem_018", "ufitem_016"} {
		if err = db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: item, ItemID: item, Enabled: true, Price: 10, Quantity: 1, BuyCount: 2, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "真实矿币本地报价10/2库存"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("car-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "原矿1/回收NPC位置/当地50矿"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
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
	point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	clock.Store(first + 20000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("car-mine-%d", camp)))}, now()); err != nil {
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
		t.Fatal("natural50 to150 war currency")
	}
	clock.Add(10000)
	point, _ = fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(shop)}, now()); err != nil {
		t.Fatal(err)
	}
	for i, item := range []string{"ufitem_017", "ufitem_018", "ufitem_016"} {
		count := 2
		if item == "ufitem_016" {
			count = 1
		}
		for n := 0; n < count; n++ {
			if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(item), NpcId: proto.String(shop)}, 200+int64(i*2+n), now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if read().GetOwnToken() != 100 || len(read().Items) != 3 {
		t.Fatal("actual5 bought items cost50")
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true)
	req := func(item string) *pb.CSUFPlayerUseItem { return &pb.CSUFPlayerUseItem{ItemId: proto.String(item)} }
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now()); err == nil {
		t.Fatal("bomb used before actual first car")
	}
	clock.Store(first + 119999)
	if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightFactoryState{}).Where("match_id=? AND completed_at>0", match.ID).Count(&count)
	if count != 0 {
		t.Fatal("timer early cars")
	}
	clock.Store(first + 120000)
	if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.P13UnionFightFactoryState{}).Where("match_id=? AND completed_at>0", match.ID).Count(&count)
	if count != 2 {
		t.Fatal("original both main first cars", count)
	}
	var target db.P13UnionFightFactoryState
	if err = db.GetIns().First(&target, "match_id=? AND completed_at>0 AND camp<>?", match.ID, camp).Error; err != nil {
		t.Fatal(err)
	}
	car, err := p13UFCarItemRead(&target)
	if err != nil {
		t.Fatal(err)
	}
	initial := car.GetHp()
	// 本方出生点和未到敌车范围均拒绝；当前位置是当前服务器确实未移动的原出生点。
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now()); err == nil {
		t.Fatal("remote car targeted outside5")
	}
	point, _ = fieldPoint(original.table("UnionFightFactory", "UFFId", fmt.Sprint(target.FactoryID))["CariotBornPos"])
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_016"), 801, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now()); err == nil {
		t.Fatal("bomb bypassed normal shared CD")
	}
	clock.Add(5000)
	small, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now())
	if err != nil {
		t.Fatal(err)
	}
	if len(small) != 3 || math.Abs(small[2].(*pb.SCUFCaropt).Cariot[0].GetHp()-initial*.8) > 1e-8 || small[1].(*pb.SCUFElementTickExport).TargetGuid[0] != car.GetGuid() {
		t.Fatal("actual enemy first small effect")
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now())
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
		t.Fatal("same receipt consumed again")
	}
	db.GetIns().Model(&db.P13UnionFightCarItemSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "hp_percent": 1})
	clock.Add(5000)
	large, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_018"), 802, now())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(large[2].(*pb.SCUFCaropt).Cariot[0].GetHp()-initial*.2) > 1e-8 {
		t.Fatal("frozen60 source drifted")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	live, err := f.s.p13UFFactoryCars(db.GetIns(), match, map[int64]bool{car.GetGuid(): true})
	if err != nil || math.Abs(live[0].(*pb.SCUFCaropt).Cariot[0].GetHp()-initial*.2) > 1e-8 {
		t.Fatal("cold partial car damage reset", err)
	}
	clock.Add(5000)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		wins := make(chan []proto.Message, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(stamp int64) {
				defer wg.Done()
				replies, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_018"), stamp, now())
				if err == nil {
					wins <- replies
				}
			}(900 + int64(i))
		}
		wg.Wait()
		close(wins)
		count := 0
		for replies := range wins {
			count++
			large = replies
		}
		if count != 1 {
			t.Fatal("eight different final car requests", count)
		}
	} else {
		large, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_018"), 900, now())
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(large) != 4 || large[2].(*pb.SCUFCaropt).Cariot[0].GetHp() != 0 || large[3].(*pb.SCUFCariotCrash).GetGuid() != car.GetGuid() {
		t.Fatal("actual destroyed car original GUID")
	}
	if math.Abs(large[1].(*pb.SCUFElementTickExport).HpDelta[0]+initial*.2) > 1e-8 {
		t.Fatal("actual car overkill unclamped")
	}
	live, err = f.s.p13UFFactoryCars(db.GetIns(), match, map[int64]bool{car.GetGuid(): true})
	if err != nil || len(live) != 2 || live[1].(*pb.SCUFCariotCrash).GetGuid() != car.GetGuid() {
		t.Fatal("cold originalRefresh created living zeroHP car", err)
	}
	clock.Add(5000)
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 1000, now()); err == nil {
		t.Fatal("bomb targeted actual destroyed car")
	}
	if read().Items[0].GetLeftUseNum() != 1 {
		t.Fatal("no target consumed unused small bomb")
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", false)
	retry, err := f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now())
	if err != nil || !proto.Equal(retry[0], small[0]) {
		t.Fatal("hot off car receipt changed", err)
	}
	db.GetIns().Model(&db.P13UnionFightFactoryState{}).Where("match_id=? AND guid=?", match.ID, target.GUID).Update("car_hash", "bad")
	if _, err = f.s.p13UFFactoryCars(db.GetIns(), match, nil); err == nil {
		t.Fatal("car frame/state SHA corrupt displayed")
	}
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFItemUse(f.ctx, id, req("ufitem_017"), 800, now()); err == nil {
		t.Fatal("End old bomb receipt")
	}
}
