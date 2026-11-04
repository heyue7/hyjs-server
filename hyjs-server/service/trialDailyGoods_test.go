package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestTrialDailyGoodsAdminAndDailyPurchases(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/trial-daily-goods-before.json")
	if err != nil {
		t.Fatal(err)
	}
	var before p4CommerceRules
	if err = json.Unmarshal(raw, &before); err != nil || len(before.Products) != 49 || before.TrialRefreshVersion != 1 {
		t.Fatal("unexpected original trial catalogue", len(before.Products), err)
	}
	originals := make(map[string]p4Product)
	var liberation p4Product
	for _, product := range before.Products {
		if product.isEnabled() && strings.HasPrefix(product.Rewards, "frag_trea_") {
			originals[product.ID] = product
		}
		if product.isEnabled() && product.Rewards == "item_005,25" {
			liberation = product
		}
	}
	if len(originals) != 24 || liberation.ID == "" {
		t.Fatal("expected the 24 original accessory rewards and liberation stone", len(originals))
	}
	// The fixture owns this duplicate, including its original identifier and quota.
	duplicate := liberation
	duplicate.ID, duplicate.Index = "fixture_duplicate_liberation", 50
	before.Products = append(before.Products, duplicate)

	var cfg db.P4CommerceRule
	if err = db.GetIns().Where("kind=? AND id NOT LIKE ?", "point", "shop_schedule_%").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = db.GetIns().Model(&cfg).Updates(map[string]any{
		"rules": adminJSON(before), "frozen": nil,
		"start_at": now.AddDate(0, 0, -2).UnixMilli(), "end_at": now.AddDate(0, 0, 2).UnixMilli(),
		"claim_end_at": now.AddDate(0, 0, 2).UnixMilli(), "enabled": true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// This isolated server clock is the source of both request time and P1 event
	// validation; advance it from yesterday to today before the second purchase.
	f.s.ServerID = f.id + 1000000
	if err = f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	setClock := func(offset, revision int64) {
		t.Helper()
		if err := db.GetIns().Model(&db.ServerGameClock{}).Where("server_id=?", f.s.ServerID).Updates(map[string]any{"offset_days": offset, "revision": revision}).Error; err != nil {
			t.Fatal(err)
		}
	}
	setClock(-1, 1)
	bound := serverClockBindTest(t, f, f.id)
	firstDay := LocalPlayNow(bound)

	current, err := f.s.AdminShop(f.ctx, "point")
	if err != nil {
		t.Fatal(err)
	}
	applied, err := f.s.ApplyTrialDailyGoods(f.ctx, actor, current.Version)
	if err != nil || applied.Version == "" || applied.Version == current.Version {
		t.Fatal("versioned administrator application", err)
	}
	if _, err = f.s.ApplyTrialDailyGoods(f.ctx, actor, current.Version); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale catalogue version accepted", err)
	}
	if err = db.GetIns().First(&cfg, "id=?", cfg.ID).Error; err != nil {
		t.Fatal(err)
	}
	var after p4CommerceRules
	if err = json.Unmarshal(cfg.Rules, &after); err != nil || after.TrialRefreshVersion != 2 || len(after.Products) != 74 || !after.LiveEditable || after.RefreshEnabled == nil || *after.RefreshEnabled || after.Slots != 0 {
		t.Fatal("fixed full catalogue version", len(after.Products), err)
	}
	byID := make(map[string]p4Product)
	dailyByReward := make(map[string]p4Product)
	var enabledCount int
	for _, product := range after.Products {
		byID[product.ID] = product
		if product.isEnabled() {
			enabledCount++
		}
		if strings.HasPrefix(product.ID, constUtil.TrialDailyGoodsPrefix) {
			if _, exists := dailyByReward[product.Rewards]; exists {
				t.Fatal("duplicate daily accessory", product.Rewards)
			}
			dailyByReward[product.Rewards] = product
		}
	}
	if p := byID[duplicate.ID]; p.isEnabled() || p.Index != duplicate.Index || p.Rewards != duplicate.Rewards {
		t.Fatal("duplicate liberation product was not retained disabled", p)
	}
	if p := byID[liberation.ID]; !p.isEnabled() || p.Rewards != "item_005,250" || p.DailyLimit != 2 || p.TotalLimit != 0 {
		t.Fatal("liberation product", p)
	}
	for _, id := range []string{"item_006", "item_008", "item_009", "item_010", "item_012"} {
		found := false
		for _, product := range after.Products {
			if product.isEnabled() && strings.HasPrefix(product.Rewards, id+",") {
				found = product.DailyLimit == 2 && product.TotalLimit == 0
			}
		}
		if !found {
			t.Fatal("daily limit missing for material", id)
		}
	}
	var sword p4Product
	for _, product := range after.Products {
		if product.isEnabled() && product.Rewards == "frag_equip_037,5" {
			sword = product
		}
	}
	if sword.ID == "" || sword.Priority/1000 != 4 || sword.DailyLimit != 1 || sword.TotalLimit != 1 {
		t.Fatal("holy sword must remain a once-only equipment reward", sword)
	}
	if len(dailyByReward) != 24 {
		t.Fatal("daily accessory count", len(dailyByReward))
	}
	for id, original := range originals {
		retained, exists := byID[id]
		if !exists || retained.Rewards != original.Rewards || retained.Cost != original.Cost || retained.TotalLimit != original.TotalLimit || retained.Index != original.Index {
			t.Fatal("original accessory reward or identity changed", id, retained)
		}
		daily, exists := dailyByReward[original.Rewards]
		if !exists || daily.ID == original.ID || !daily.isEnabled() || daily.Priority/1000 != 1 || daily.Cost != "point,30000" || daily.MinTrialStars != 135 || daily.DailyLimit != 1 || daily.TotalLimit != 0 {
			t.Fatal("daily accessory terms", original.Rewards, daily)
		}
	}

	if err = db.GetIns().Create(&db.P7RushState{PlayerID: f.id, Day: p1Day(firstDay), RunID: 1, Floor: 46, HighFloor: 45, TotalStars: 135, RecordStars: 135}).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level, p.Point = proto.Int32(75), proto.Int32(150000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	list := func(at time.Time) map[string]*pb.ShopItem {
		t.Helper()
		out, err := f.s.p4Commerce(bound, f.id, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, at)
		if err != nil {
			t.Fatal("trial catalogue", err)
		}
		items := make(map[string]*pb.ShopItem)
		for _, item := range out[0].(*pb.SCShopProductList).GetShopItemList() {
			_, id, ok := adminLiveShopParseSKU(item.GetId())
			if !ok || items[id] != nil {
				t.Fatal("invalid or duplicate client SKU", item.GetId())
			}
			items[id] = item
		}
		return items
	}
	items := list(firstDay)
	if len(items) != enabledCount {
		t.Fatal("fixed catalogue did not publish every enabled product", len(items), enabledCount)
	}
	refresh, err := f.s.p4Commerce(bound, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(5)}, 0, firstDay)
	if err != nil {
		t.Fatal("daily client query", err)
	}
	query := refresh[0].(*pb.SCRefreshShopQuery)
	if query.GetFreeLeftTimes() != 0 || query.GetFreeMaxTimes() != 0 || query.GetResLeftTimes() != 0 || query.GetResMaxTimes() != 0 || query.GetNextUpdateTime() != trialDailyGoodsNext(firstDay) || len(query.GetShopItemList()) != enabledCount {
		t.Fatal("daily client rejected refresh counters or catalogue", query)
	}
	for _, product := range after.Products {
		item := items[product.ID]
		if product.isEnabled() != (item != nil) {
			t.Fatal("catalogue visibility", product.ID)
		}
		if strings.HasPrefix(product.ID, constUtil.TrialDailyGoodsPrefix) && (item.GetPriority()/1000 != 1 || item.GetCondition().GetType() != "star" || item.GetCondition().GetValue() != 135 || item.GetCanBuy() != 1 || item.GetDayLeftTimes() != 1) {
			t.Fatal("daily accessory client projection", item)
		}
	}
	if items[sword.ID] == nil || items[sword.ID].GetPriority()/1000 != 4 {
		t.Fatal("holy sword client reward tab", items[sword.ID])
	}
	material := items[liberation.ID]
	if material.GetCanBuy() != 1 || material.GetDayTotalTimes() != 2 || material.GetDayLeftTimes() != 2 {
		t.Fatal("daily material client state", material)
	}
	original := p4Product{}
	for _, p := range originals {
		if p.Rewards == "frag_trea_015_1,1" {
			original = p
		}
	}
	if original.ID == "" {
		t.Fatal("representative original accessory missing")
	}
	daily := dailyByReward[original.Rewards]
	buy := func(item *pb.ShopItem, stamp int64, at time.Time) ([]proto.Message, error) {
		t.Helper()
		return f.s.p4Commerce(bound, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(item.GetId()), BuyCount: proto.Int32(1)}, stamp, at)
	}
	if _, err = buy(items[original.ID], 919001, firstDay); err != nil {
		t.Fatal("original accessory purchase", err)
	}
	if _, err = buy(items[original.ID], 919002, firstDay); err == nil {
		t.Fatal("original accessory purchased twice")
	}
	first, err := buy(items[daily.ID], 919003, firstDay)
	if err != nil {
		t.Fatal("daily accessory purchase", err)
	}
	paid, inventory := f.profile(t)
	if paid.GetPoint() != 112000 || simpleOwned(paid, inventory, "frag_trea_015_1") != 2 {
		t.Fatal("original and daily purchase amounts", paid.GetPoint())
	}
	replay, err := buy(items[daily.ID], 919003, firstDay)
	afterReplay, replayInventory := f.profile(t)
	if err != nil || !proto.Equal(first[0], replay[0]) || afterReplay.GetPoint() != paid.GetPoint() || simpleOwned(afterReplay, replayInventory, "frag_trea_015_1") != 2 {
		t.Fatal("same-stamp replay charged or granted twice", err)
	}
	if _, err = buy(items[daily.ID], 919004, firstDay); err == nil {
		t.Fatal("daily accessory exceeded daily limit")
	}
	for i := 0; i < 2; i++ {
		if _, err = buy(material, 919010+int64(i), firstDay); err != nil {
			t.Fatal("daily liberation purchase", i, err)
		}
		updated := list(firstDay)[liberation.ID]
		wantState := int32(1)
		if i == 1 {
			wantState = 2
		}
		if updated.GetDayLeftTimes() != int32(1-i) || updated.GetCanBuy() != wantState {
			t.Fatal("daily material count or client state", i, updated)
		}
	}
	if _, err = buy(material, 919012, firstDay); err == nil {
		t.Fatal("daily liberation limit exceeded")
	}

	setClock(0, 2)
	bound = serverClockBindTest(t, f, f.id)
	secondDay := LocalPlayNow(bound)
	if p1Day(firstDay) == p1Day(secondDay) {
		t.Fatal("isolated clock did not cross the business day", firstDay, secondDay)
	}
	items = list(secondDay)
	if items[original.ID].GetCanBuy() != 2 || items[daily.ID].GetCanBuy() != 1 || items[daily.ID].GetDayLeftTimes() != 1 || items[liberation.ID].GetCanBuy() != 1 || items[liberation.ID].GetDayLeftTimes() != 2 {
		t.Fatal("next day did not restore daily-only quotas", items[original.ID], items[daily.ID], items[liberation.ID])
	}
	if _, err = buy(items[original.ID], 919020, secondDay); err == nil {
		t.Fatal("new day reset original accessory lifetime limit")
	}
	if _, err = buy(items[daily.ID], 919021, secondDay); err != nil {
		t.Fatal("next-day accessory purchase", err)
	}
	final, finalInventory := f.profile(t)
	if final.GetPoint() != 80400 || simpleOwned(final, finalInventory, "frag_trea_015_1") != 3 || simpleOwned(final, finalInventory, "item_005") != 500 {
		t.Fatal(fmt.Sprintf("final balances: points=%d accessory=%d liberation=%d", final.GetPoint(), simpleOwned(final, finalInventory, "frag_trea_015_1"), simpleOwned(final, finalInventory, "item_005")))
	}
}
