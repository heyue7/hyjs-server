package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP4Feedback66ScoreProjectionUsesNativeStateAndRetainsQuota(t *testing.T) {
	f := p4Setup(t)
	for _, tc := range []struct {
		name          string
		shopType      int32
		bought        int32
		level         int32
		totalLimit    int32
		total         int32
		stock         int32
		used          int32
		wantCan       int32
		wantLeft      int32
		wantTotalLeft int32
	}{
		{"twenty", 7, 0, 75, 0, 0, -1, 0, 1, 20, 2147483647},
		{"nineteen", 7, 1, 75, 0, 0, -1, 0, 1, 19, 2147483647},
		{"one", 7, 19, 75, 0, 0, -1, 0, 1, 1, 2147483647},
		{"daily-exhausted", 7, 20, 75, 0, 0, -1, 0, 0, 0, 2147483647},
		{"level-locked", 7, 0, 34, 0, 0, -1, 0, 0, 20, 2147483647},
		{"total-cap", 7, 0, 75, 2, 1, -1, 0, 1, 1, 1},
		{"total-exhausted", 7, 0, 75, 2, 2, -1, 0, 0, 0, 0},
		{"stock-exhausted", 7, 0, 75, 0, 0, 3, 3, 0, 0, 2147483647},
		{"point-unchanged", 5, 0, 75, 0, 0, -1, 0, 20, 20, 2147483647},
		{"ordinary-unchanged", 1, 0, 75, 0, 0, -1, 0, 20, 20, 2147483647},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := db.P4CommerceRule{ID: "feedback66-" + tc.name, Kind: "score", ShopType: tc.shopType}
			product := p4BaseProduct("feedback66-score", "item_005,5", "score,50", 1, 35)
			product.TotalLimit, product.Stock = tc.totalLimit, tc.stock
			state := &p4CommerceProgress{Bought: map[string]int32{product.ID: tc.bought}, Total: map[string]int32{product.ID: tc.total}}
			if tc.stock >= 0 {
				if err := db.GetIns().Create(&db.P4CommerceStock{RuleID: cfg.ID, ProductID: product.ID, Used: tc.used}).Error; err != nil {
					t.Fatal(err)
				}
			}
			items, activity, err := f.s.p4Projection(db.GetIns(), &pb.Player{Level: proto.Int32(tc.level)}, cfg, p4CommerceRules{Products: []p4Product{product}}, state)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || len(activity) != 1 {
				t.Fatal("projection directory", items, activity)
			}
			item := items[0]
			if item.GetCanBuy() != tc.wantCan || item.GetDayLeftTimes() != tc.wantLeft || item.GetDayTotalTimes() != 20 || item.GetTotalLeftTimes() != tc.wantTotalLeft {
				t.Fatal("native eligibility and independent quantities", item)
			}
			wantStatus := int32(0)
			if tc.wantCan > 0 {
				wantStatus = 1
			}
			if activity[0].GetStatus() != wantStatus || activity[0].GetLeft() != tc.wantLeft || activity[0].GetTotal() != 20 {
				t.Fatal("activity projection changed", activity[0])
			}
			// Changing the native state enum must retain the authoritative price curve and every other field.
			referenceCfg := cfg
			referenceCfg.ShopType = 1
			reference, _, err := f.s.p4Projection(db.GetIns(), &pb.Player{Level: proto.Int32(tc.level)}, referenceCfg, p4CommerceRules{Products: []p4Product{product}}, state)
			if err != nil {
				t.Fatal(err)
			}
			reference[0].CanBuy = item.CanBuy
			if !proto.Equal(reference[0], item) {
				t.Fatal("quota, price or curve changed with eligibility", reference[0], item)
			}
		})
	}
}

func TestP4Feedback66ScoreSinglePurchaseAndHistoricalBatchReplay(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "score").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	for i := range rules.Products {
		if rules.Products[i].ID == "p4Score_1" {
			rules.Products[i].DailyLimit = 2
		}
	}
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"enabled": true, "rules": adminJSON(rules)}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	query := &pb.CSShopProductList{ShopType: proto.Int32(7)}
	listed := p4Trade(t, f, query, 0, now).(*pb.SCShopProductList)
	if len(listed.ShopItemList) != 2 {
		t.Fatal("score catalogue", listed)
	}
	item := p4PopularityFind(t, listed.ShopItemList, "p4Score_1")
	if item.GetCanBuy() != 1 || item.GetDayLeftTimes() != 2 || item.GetDayTotalTimes() != 2 {
		t.Fatal("native visible state or daily quota changed", item)
	}
	curve := func(item *pb.ShopItem) []int32 {
		t.Helper()
		var prices []int32
		for raw := item.ProtoReflect().GetUnknown(); len(raw) > 0; {
			field, kind, n := protowire.ConsumeTag(raw)
			if n < 0 || field != 100 || kind != protowire.BytesType {
				t.Fatal("malformed price extension")
			}
			payload, size := protowire.ConsumeBytes(raw[n:])
			if size < 0 {
				t.Fatal("malformed price")
			}
			var price pb.BaseResource
			if err := proto.Unmarshal(payload, &price); err != nil {
				t.Fatal(err)
			}
			prices = append(prices, price.GetNum())
			raw = raw[n+size:]
		}
		return prices
	}
	if prices := curve(item); len(prices) != 1 || prices[0] != 50 {
		t.Fatal("existing bulk selector must receive one exact price", prices)
	}
	ordinary := p4Trade(t, f, &pb.CSShopProductList{ShopType: proto.Int32(1)}, 0, now).(*pb.SCShopProductList)
	if prices := curve(p4PopularityFind(t, ordinary.ShopItemList, "itemShopId_1")); len(prices) < 2 {
		t.Fatal("other shop batch prices were limited", prices)
	}
	// This preexisting immutable receipt represents an already completed old batch.
	legacy := &pb.CSShopBuy{ShopType: proto.Int32(7), ShopItemId: proto.String("p4Score_1"), BuyCount: proto.Int32(2)}
	legacyReceipt := &pb.SCShopBuy{ShopType: proto.Int32(7), Cost: []*pb.BaseResource{{Id: proto.String("score"), Num: proto.Int32(100)}}, Get: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("item_005"), Num: proto.Int32(10)}}}}
	if err := p4SaveReceipt(db.GetIns(), f.id, "commerce:7", 66000, legacy, legacyReceipt); err != nil {
		t.Fatal(err)
	}
	before, beforeGame := f.profile(t)
	owned := simpleOwned(before, beforeGame, "item_005")
	if replay := p4Trade(t, f, legacy, 66000, now); !proto.Equal(replay, legacyReceipt) {
		t.Fatal("legacy batch receipt changed", replay)
	}
	conflict := proto.Clone(legacy).(*pb.CSShopBuy)
	conflict.BuyCount = proto.Int32(1)
	if _, err := f.s.p4Commerce(f.ctx, f.id, conflict, 66000, now); err == nil {
		t.Fatal("changed historical request reused receipt")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, legacy, 66001, now); err == nil {
		t.Fatal("new batch purchase accepted")
	}
	unchanged, unchangedGame := f.profile(t)
	if !proto.Equal(before, unchanged) || !p6CopySeriesJSONEqual(t, beforeGame, unchangedGame) {
		t.Fatal("legacy replay or rejected batch changed assets")
	}
	bought := int32(0)
	for index, count := range []int32{1, 1} {
		request := &pb.CSShopBuy{ShopType: proto.Int32(7), ShopItemId: proto.String("p4Score_1"), BuyCount: proto.Int32(count)}
		wire, err := proto.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.CSShopBuy{}
		if err = proto.Unmarshal(wire, decoded); err != nil {
			t.Fatal(err)
		}
		stamp := int64(66002 + index)
		result := p4Trade(t, f, decoded, stamp, now).(*pb.SCShopBuy)
		bought += count
		if len(result.GetCost()) != 1 || result.GetCost()[0].GetId() != "score" || result.GetCost()[0].GetNum() != count*50 || len(result.GetGet().GetSimps()) != 1 || result.GetGet().GetSimps()[0].GetId() != "item_005" || result.GetGet().GetSimps()[0].GetNum() != count*5 {
			t.Fatal("single purchase amount changed", result)
		}
		current, game := f.profile(t)
		if current.GetScore() != before.GetScore()-bought*50 || simpleOwned(current, game, "item_005") != owned+int64(bought)*5 {
			t.Fatal("actual wallet or item quantity", current.GetScore(), game.Items)
		}
		item := p4PopularityFind(t, result.ShopItemList, "p4Score_1")
		wantCan := int32(1)
		if bought == 2 {
			wantCan = 0
		}
		if item.GetCanBuy() != wantCan || item.GetDayLeftTimes() != 2-bought {
			t.Fatal("post-buy native state and quota", item)
		}
		replay := p4Trade(t, f, decoded, stamp, now)
		afterReplay, replayGame := f.profile(t)
		if !proto.Equal(result, replay) || afterReplay.GetScore() != current.GetScore() || simpleOwned(afterReplay, replayGame, "item_005") != simpleOwned(current, game, "item_005") {
			t.Fatal("same original stamp charged or granted twice")
		}
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(7), ShopItemId: proto.String("p4Score_1"), BuyCount: proto.Int32(1)}, 66004, now); err == nil {
		t.Fatal("daily quota bypass")
	}
	f.restart(t)
	if replay := p4Trade(t, f, legacy, 66000, now); !proto.Equal(replay, legacyReceipt) {
		t.Fatal("cold historical receipt changed", replay)
	}
}
