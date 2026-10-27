package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func TestPointShopLifetimeStarsPurchaseAndColdReplay(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind = ?", "point").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	product := p4BaseProduct("trialLifetime", "frag_equip_037,10", "point,150", 1, 1)
	product.MinTrialStars = 135
	raw, _ := json.Marshal(p4CommerceRules{Products: []p4Product{product}})
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	query := &pb.CSShopProductList{ShopType: proto.Int32(5)}
	list := p4Trade(t, f, query, 0, now).(*pb.SCShopProductList).GetShopItemList()
	if len(list) != 1 || list[0].GetCanBuy() != 0 || list[0].GetCondition().GetType() != "star" || list[0].GetCondition().GetValue() != 135 {
		t.Fatal("missing locked star projection", list)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(product.ID), BuyCount: proto.Int32(1)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 1001, now); err == nil {
		t.Fatal("insufficient stars accepted")
	}
	p, g := f.profile(t)
	if p.GetPoint() != 1000 || simpleOwned(p, g, "frag_equip_037") != 0 {
		t.Fatal("failed qualification mutated assets")
	}
	if err := db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 135}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	type purchaseResult struct {
		stamp    int64
		response proto.Message
		err      error
	}
	results := make(chan purchaseResult, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(stamp int64) {
			defer wg.Done()
			messages, err := f.s.p4Commerce(f.ctx, f.id, buy, stamp, now)
			result := purchaseResult{stamp: stamp, err: err}
			if err == nil {
				result.response = messages[0]
			}
			results <- result
		}(int64(1001 + i))
	}
	wg.Wait()
	close(results)
	var first proto.Message
	var successfulStamp int64
	successes := 0
	for result := range results {
		if result.err == nil {
			successes++
			first = result.response
			successfulStamp = result.stamp
		}
	}
	if successes != 1 {
		t.Fatal("concurrent distinct stamps oversold", successes)
	}
	list = p4Trade(t, f, query, 0, now).(*pb.SCShopProductList).GetShopItemList()
	if len(list) != 1 || list[0].GetCanBuy() != 2 || list[0].GetTotalLeftTimes() != 0 {
		t.Fatal("purchased item must display the original sold state", list)
	}
	if !proto.Equal(first, p4Trade(t, f, buy, successfulStamp, now)) {
		t.Fatal("warm receipt changed")
	}
	f.open(t)
	if !proto.Equal(first, p4Trade(t, f, buy, successfulStamp, now)) {
		t.Fatal("cold receipt changed")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 1010, now); err == nil {
		t.Fatal("second purchase accepted")
	}
	// Replace the directory identity with an independently published next window.
	next := cfg
	next.ID = "trialLifetimeNextWeek"
	next.StartAt = now.Add(-time.Hour).UnixMilli()
	next.EndAt = now.Add(15 * 24 * time.Hour).UnixMilli()
	next.ClaimEndAt = next.EndAt
	next.Rules = raw
	next.Frozen = nil
	if err := db.GetIns().Create(&next).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 1003, now.Add(8*24*time.Hour)); err == nil {
		t.Fatal("new week reset lifetime")
	}
	p, g = f.profile(t)
	if p.GetPoint() != 850 || simpleOwned(p, g, "frag_equip_037") != 10 {
		t.Fatal("purchase/retry assets", p.GetPoint(), simpleOwned(p, g, "frag_equip_037"))
	}
	account, err := LoginAccount(f.ctx, "trial-other-"+t.Name(), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "试炼独立角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	second := *f
	second.id = account.Id
	p4Fund(t, &second)
	if err = db.GetIns().Create(&db.P7RushState{PlayerID: second.id, RecordStars: 135}).Error; err != nil {
		t.Fatal(err)
	}
	p4Trade(t, &second, buy, 1001, now)
	secondPlayer, secondGame := second.profile(t)
	if secondPlayer.GetPoint() != 850 || simpleOwned(secondPlayer, secondGame, "frag_equip_037") != 10 {
		t.Fatal("second role could not independently purchase")
	}
	// Other shops retain their original repeat-purchase quotas.
	ordinary := &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_1"), BuyCount: proto.Int32(1)}
	p4Trade(t, f, ordinary, 2001, now)
	p4Trade(t, f, ordinary, 2002, now)
	if err := db.GetIns().Migrator().DropTable(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now); err == nil {
		t.Fatal("trial database failure was swallowed")
	}
}

func TestPointShopLifetimeImportsClosedHistoryAndSeparatesPlayers(t *testing.T) {
	f := p4Setup(t)
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind = ?", "point").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	cfg.ID = "closedTrialHistory"
	cfg.Enabled = false
	cfg.EndAt = 1
	cfg.ClaimEndAt = 1
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p4CommerceProgress{Total: map[string]int32{"trialA": 1}})
	if err := db.GetIns().Create(&db.P4CommerceState{PlayerID: f.id, RuleID: cfg.ID, Payload: raw}).Error; err != nil {
		t.Fatal(err)
	}
	_, state, _, err := loadPointShopLifetime(db.GetIns(), f.id)
	if err != nil || state.Total["trialA"] != 1 {
		t.Fatal("closed history omitted", err, state)
	}
	_, other, stars, err := loadPointShopLifetime(db.GetIns(), f.id+999)
	if err != nil || other.Total["trialA"] != 0 || stars != 0 {
		t.Fatal("player history leaked", err, other, stars)
	}
	p4Fund(t, f)
	current := cfg
	current.ID = "trialHistoryCurrent"
	current.Enabled = true
	current.StartAt = time.Now().Add(-time.Hour).UnixMilli()
	current.EndAt = time.Now().Add(time.Hour).UnixMilli()
	current.ClaimEndAt = current.EndAt
	current.Frozen = nil
	current.Rules, _ = json.Marshal(p4CommerceRules{Products: []p4Product{p4BaseProduct("trialA", "item_021,1", "point,100", 1, 1)}})
	if err := db.GetIns().Create(&current).Error; err != nil {
		t.Fatal(err)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("trialA"), BuyCount: proto.Int32(1)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 3001, time.Now()); err == nil {
		t.Fatal("historically bought product purchased again")
	}
	player, _ := f.profile(t)
	if player.GetPoint() != 1000 {
		t.Fatal("history refusal charged points")
	}
	// New fixed state is authoritative; historic rows are preserved and never recounted.
	_, again, _, err := loadPointShopLifetime(db.GetIns(), f.id)
	if err != nil || again.Total["trialA"] != 1 {
		t.Fatal("history counted twice", err, again)
	}
}

func TestPointShopLegacyHundredStarRequirementMatchesPurchase(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind = ?", "point").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	product := p4BaseProduct("legacyTrial", "item_021,1", "point,100", 1, 1)
	raw, _ := json.Marshal(p4CommerceRules{Products: []p4Product{product}})
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	query := &pb.CSShopProductList{ShopType: proto.Int32(5)}
	list := p4Trade(t, f, query, 0, now).(*pb.SCShopProductList).GetShopItemList()
	if len(list) != 1 || list[0].GetCanBuy() != 0 || list[0].GetCondition().GetType() != "star" || list[0].GetCondition().GetValue() != 100 {
		t.Fatal("default 100-star gate not visible", list)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(product.ID), BuyCount: proto.Int32(1)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 8811, now); err == nil {
		t.Fatal("sub-100-star purchase accepted")
	}
	if err := db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 8812, now); err != nil {
		t.Fatal("100-star purchase rejected", err)
	}
}
