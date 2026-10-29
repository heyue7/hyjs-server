package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func trialRefresh82Fixture(t *testing.T, now time.Time) (*p1Fixture, db.P4CommerceRule, p4CommerceRules) {
	t.Helper()
	f := p4Setup(t)
	p4Fund(t, f)
	if err := db.GetIns().AutoMigrate(&db.LocalPlayActivityOperation{}); err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	fragment := p4BaseProduct("trial82_fragment", "frag_equip_037,5", "point,50", 1, 1)
	fragment.MinTrialStars, fragment.DailyLimit, fragment.TotalLimit = 135, 1, 1
	purple := p4BaseProduct("trial82_purple", "frag_equip_013,1", "point,70", 2, 1)
	orange := p4BaseProduct("trial82_orange", "frag_equip_025,1", "point,80", 3, 1)
	red := p4BaseProduct("trial82_red", "equip_041,1", "point,90", 4, 1)
	rules := p4CommerceRules{TrialRefreshVersion: 1, LiveEditable: true, Slots: 6, FreeCap: 10, FreeSeconds: 7200, ResourceRefreshCap: 15, RefreshItem: "item_013,1", RefreshCost: "diamond,20", Products: []p4Product{fragment, purple, orange, red}}
	cfg.Rules, _ = json.Marshal(rules)
	cfg.Frozen = nil
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": cfg.Rules, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 100}).Error; err != nil {
		t.Fatal(err)
	}
	_ = now
	return f, cfg, rules
}

func trialRefresh82Groups(t *testing.T, items []*pb.ShopItem) map[int32][]*pb.ShopItem {
	t.Helper()
	groups := map[int32][]*pb.ShopItem{}
	for _, item := range items {
		groups[item.GetPriority()/1000] = append(groups[item.GetPriority()/1000], item)
	}
	return groups
}

func trialRefresh82Cold(t *testing.T, f *p1Fixture) {
	t.Helper()
	connection, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
}

func TestTrialRefresh82SlotsLifetimeQuotesAndColdReplay(t *testing.T) {
	now := time.Now()
	f, cfg, rules := trialRefresh82Fixture(t, now)
	legacy := &p4CommerceProgress{Day: p1Day(now), Bought: map[string]int32{"trial82_fragment": 1, "trial82_purple": 1}, Total: map[string]int32{"trial82_fragment": 1, "trial82_purple": 1}, Version: 1, FreeLeft: 1, FreeAt: now.UnixMilli()}
	legacyRow := &db.P4CommerceState{PlayerID: f.id, RuleID: cfg.ID}
	if err := saveP4Commerce(db.GetIns(), legacyRow, legacy); err != nil {
		t.Fatal(err)
	}
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(5)}
	initial := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	groups := trialRefresh82Groups(t, initial.GetShopItemList())
	if initial.GetShopType() != 5 || initial.GetFreeLeftTimes() != 1 || initial.GetFreeMaxTimes() != 10 || initial.GetResLeftTimes() != 15 || len(groups[1]) != 6 || len(groups[2]) != 1 || len(groups[3]) != 1 || len(groups[4]) != 1 {
		t.Fatal("trial refresh projection", initial)
	}
	seen := map[string]bool{}
	for pos, item := range groups[1] {
		if seen[item.GetId()] || item.GetPriority() != int32(1001+pos) || item.GetCanBuy() != 0 || item.GetCondition().GetValue() != 135 || item.GetItem()[0].GetNum() != 5 {
			t.Fatal("slot identity or star requirement", item)
		}
		seen[item.GetId()] = true
	}
	if groups[2][0].GetCanBuy() != 2 {
		t.Fatal("legacy purple lifetime lost", groups[2][0])
	}
	plainList := p4Trade(t, f, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, now).(*pb.SCShopProductList)
	if plainList.GetShopType() != 5 || len(plainList.GetShopItemList()) != len(initial.GetShopItemList()) {
		t.Fatal("normal trial listing lost a tab", plainList)
	}
	firstSlot := groups[1][0].GetId()
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(firstSlot), BuyCount: proto.Int32(1)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 1, now); err == nil {
		t.Fatal("missing stars accepted")
	}
	if err := db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", f.id).Update("record_stars", 135).Error; err != nil {
		t.Fatal(err)
	}
	// A live operator price/quantity edit applies on the next draw; this accepted
	// generation keeps the exact previously displayed price and amount.
	rules.Products[0].Cost, rules.Products[0].Rewards = "point,90", "frag_equip_037,6"
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	updated := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	updatedGroups := trialRefresh82Groups(t, updated.GetShopItemList())
	if updatedGroups[1][0].GetId() != firstSlot || updatedGroups[1][0].GetPrice()[0].GetNum() != 50 || updatedGroups[1][0].GetItem()[0].GetNum() != 5 {
		t.Fatal("live edit changed frozen quote", updatedGroups[1][0])
	}
	first := p4Trade(t, f, buy, 1, now).(*pb.SCShopBuy)
	if first.GetCost()[0].GetNum() != 50 {
		t.Fatal("wrong frozen cost", first.GetCost())
	}
	if !proto.Equal(first, p4Trade(t, f, buy, 1, now)) {
		t.Fatal("same stamp replay changed")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 2, now); err == nil {
		t.Fatal("slot purchased twice")
	}
	trialRefresh82Cold(t, f)
	if !proto.Equal(first, p4Trade(t, f, buy, 1, now)) {
		t.Fatal("cold receipt changed")
	}
	second := &pb.CSRefreshShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(updatedGroups[1][1].GetId())}
	rules.Products[0].MinTrialStars = 140
	raw, _ = json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, second, 2, now); err == nil {
		t.Fatal("stale eligibility accepted")
	}
	rules.Products[0].MinTrialStars = 135
	raw, _ = json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	if result := p4Trade(t, f, second, 2, now).(*pb.SCRefreshShopBuy); result.GetShopType() != 5 {
		t.Fatal("refresh-buy response shop type", result)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("trial82_fragment"), BuyCount: proto.Int32(1)}, 3, now); err == nil {
		t.Fatal("unquoted source SKU accepted")
	}
	oldQuote := adminLiveP4SKU(cfg, rules.Products[0], nil)
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(oldQuote), BuyCount: proto.Int32(1)}, 3, now); err == nil {
		t.Fatal("old source quote accepted")
	}
	orange := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(groups[3][0].GetId()), BuyCount: proto.Int32(1)}
	p4Trade(t, f, orange, 3, now)
	if _, err := f.s.p4Commerce(f.ctx, f.id, orange, 4, now); err == nil {
		t.Fatal("fixed orange lifetime lost")
	}
	p, game := f.profile(t)
	if p.GetPoint() != 820 || simpleOwned(p, game, "frag_equip_037") != 10 {
		t.Fatal("frozen quotes charged or granted incorrectly", p.GetPoint(), simpleOwned(p, game, "frag_equip_037"))
	}
	_, lifetime, _, err := loadPointShopLifetime(db.GetIns(), f.id)
	if err != nil || lifetime.Total["trial82_fragment"] != 1 || lifetime.Total["trial82_purple"] != 1 || lifetime.Total["trial82_orange"] != 1 {
		t.Fatal("legacy lifetime modified", lifetime, err)
	}
	refresh := &pb.CSRefreshShopRefresh{ShopType: proto.Int32(5)}
	refreshed := p4Trade(t, f, refresh, 4, now).(*pb.SCRefreshShopRefresh)
	newGroups := trialRefresh82Groups(t, refreshed.GetShopItemList())
	if refreshed.GetShopType() != 5 || len(refreshed.GetCost()) != 0 || len(newGroups[1]) != 6 || newGroups[1][0].GetId() == firstSlot || newGroups[1][0].GetPrice()[0].GetNum() != 90 || newGroups[1][0].GetItem()[0].GetNum() != 6 || newGroups[3][0].GetCanBuy() != 2 {
		t.Fatal("refresh did not preserve fixed tabs or use new quote", refreshed)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(updatedGroups[1][2].GetId()), BuyCount: proto.Int32(1)}, 5, now); err == nil {
		t.Fatal("old generation accepted")
	}
	disabled := false
	rules.Products[0].Enabled = &disabled
	raw, _ = json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(newGroups[1][0].GetId()), BuyCount: proto.Int32(1)}, 5, now); err == nil {
		t.Fatal("disabled live source accepted")
	}
}

func TestTrialRefresh82QuotaCalendarAndFreeTiming(t *testing.T) {
	now := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	now = time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())
	f, cfg, _ := trialRefresh82Fixture(t, now)
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(5)}
	refresh := &pb.CSRefreshShopRefresh{ShopType: proto.Int32(5)}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(21); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", f.id).Update("record_stars", 135).Error; err != nil {
		t.Fatal(err)
	}
	locked := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	if trialRefresh82Groups(t, locked.GetShopItemList())[1][0].GetCanBuy() != 0 {
		t.Fatal("trial refresh slot opened below level 22")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, refresh, 100, now); err == nil {
		t.Fatal("trial refresh opened below level 22")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(22); return nil }); err != nil {
		t.Fatal(err)
	}
	opened := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	if trialRefresh82Groups(t, opened.GetShopItemList())[1][0].GetCanBuy() != 1 {
		t.Fatal("trial refresh slot did not open at level 22")
	}
	first := p4Trade(t, f, refresh, 100, now).(*pb.SCRefreshShopRefresh)
	if first.GetFreeLeftTimes() != 0 || len(first.GetCost()) != 0 {
		t.Fatal("first refresh not free", first)
	}
	for i := 0; i < 15; i++ {
		result := p4Trade(t, f, refresh, int64(101+i), now).(*pb.SCRefreshShopRefresh)
		want := "diamond"
		if i == 0 {
			want = "item_013"
		}
		if result.GetCost()[0].GetId() != want || result.GetResLeftTimes() != int32(14-i) {
			t.Fatal("paid refresh order or quota", i, result)
		}
		if i == 0 && !proto.Equal(result, p4Trade(t, f, refresh, int64(101+i), now)) {
			t.Fatal("refresh retry changed")
		}
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, refresh, 116, now); err == nil {
		t.Fatal("sixteenth paid refresh accepted")
	}
	// The same offer remains bought across midnight until an explicit refresh.
	current := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	groups := trialRefresh82Groups(t, current.GetShopItemList())
	if err := db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", f.id).Update("record_stars", 135).Error; err != nil {
		t.Fatal(err)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(groups[1][0].GetId()), BuyCount: proto.Int32(1)}
	p4Trade(t, f, buy, 116, now)
	nextDay := now.Add(24 * time.Hour)
	next := p4Trade(t, f, query, 0, nextDay).(*pb.SCRefreshShopQuery)
	nextGroups := trialRefresh82Groups(t, next.GetShopItemList())
	if next.GetResLeftTimes() != 15 || next.GetFreeLeftTimes() != 10 || nextGroups[1][0].GetCanBuy() != 2 {
		t.Fatal("daily reset or slot ownership", next)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 117, nextDay); err == nil {
		t.Fatal("midnight reset the purchased slot")
	}
	// At the cap, elapsed complete periods advance the anchor as in memory shop.
	spentAt := nextDay.Add(time.Hour)
	used := p4Trade(t, f, refresh, 117, spentAt).(*pb.SCRefreshShopRefresh)
	if used.GetFreeLeftTimes() != 9 || used.GetNextUpdateTime() != nextDay.Add(2*time.Hour).UnixMilli() {
		t.Fatal("free recovery anchor differs from memory shop", used)
	}
	trialRefresh82Cold(t, f)
	rewound := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	if rewound.GetResLeftTimes() != 0 || rewound.GetFreeLeftTimes() != 9 {
		t.Fatal("rewind recovered paid quota or free uses", rewound)
	}
	again := p4Trade(t, f, query, 0, nextDay).(*pb.SCRefreshShopQuery)
	if again.GetResLeftTimes() != 15 || again.GetFreeLeftTimes() != 9 {
		t.Fatal("revisit reset quota or recovery", again)
	}
	// A different shop window cannot issue another daily resource allowance.
	var row db.P4CommerceState
	if err := db.GetIns().First(&row, "player_id=? AND rule_id=?", f.id, constUtil.TrialRefresh82StateID).Error; err != nil {
		t.Fatal(err)
	}
	var state trialRefresh82State
	if err := json.Unmarshal(row.Payload, &state); err != nil {
		t.Fatal(err)
	}
	state.ResourceUsed = 7
	if err := trialRefresh82Save(db.GetIns(), &row, &state); err != nil {
		t.Fatal(err)
	}
	window := cfg
	window.ID = "trial82_next_window"
	window.StartAt = nextDay.Add(time.Hour).UnixMilli()
	window.EndAt = nextDay.Add(48 * time.Hour).UnixMilli()
	window.ClaimEndAt = window.EndAt
	if err := db.GetIns().Create(&window).Error; err != nil {
		t.Fatal(err)
	}
	currentID := trialRefresh82Groups(t, again.GetShopItemList())[1][0].GetId()
	newWindow := p4Trade(t, f, query, 0, nextDay.Add(2*time.Hour)).(*pb.SCRefreshShopQuery)
	if newWindow.GetResLeftTimes() != 8 || trialRefresh82Groups(t, newWindow.GetShopItemList())[1][0].GetId() == currentID {
		t.Fatal("window reset paid quota or reused slot identity", newWindow)
	}
}

func TestTrialRefresh82PriorityAndWeightValidation(t *testing.T) {
	f, _, rules := trialRefresh82Fixture(t, time.Now())
	for _, id := range []string{"frag_equip_037", "frag_equip_038", "frag_equip_039", "frag_equip_040"} {
		if group := f.s.trialRefresh82Priority(1001, []*pb.BaseResource{{Id: proto.String(id)}}) / 1000; group != 1 {
			t.Fatal(id, group)
		}
	}
	for id, want := range map[string]int32{"frag_equip_013": 2, "frag_equip_025": 3, "equip_041": 4, "item_119": 4} {
		if got := f.s.trialRefresh82Priority(1001, []*pb.BaseResource{{Id: proto.String(id)}}) / 1000; got != want {
			t.Fatal(fmt.Sprintf("%s: %d, want %d", id, got, want))
		}
	}
	rules.Products[0].Weight = 0
	if err := f.s.trialRefresh82Draw(db.P4CommerceRule{ID: "invalid"}, rules, &trialRefresh82State{}); err == nil {
		t.Fatal("zero weight accepted")
	}
}

func TestTrialRefresh82BoundCalendarKeepsRealCooldown(t *testing.T) {
	now := time.Now()
	f, cfg, rules := trialRefresh82Fixture(t, now)
	contextAt := func(offset int64) context.Context {
		ctx := context.WithValue(f.ctx, constUtil.ServerGameClockBoundKey, true)
		return context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, offset)
	}
	load := func(offset int64) (*db.P4CommerceState, *trialRefresh82State) {
		t.Helper()
		row, state, err := f.s.trialRefresh82Load(db.GetIns().WithContext(contextAt(offset)), f.id, cfg, rules, now.AddDate(0, 0, int(offset)))
		if err != nil {
			t.Fatal(err)
		}
		return row, state
	}
	row, state := load(0)
	state.FreeLeft, state.ResourceUsed = 0, 15
	if err := trialRefresh82Save(db.GetIns(), row, state); err != nil {
		t.Fatal(err)
	}
	row, state = load(1)
	if state.FreeLeft != 0 || state.ResourceUsed != 0 || trialRefresh82Next(db.GetIns().WithContext(contextAt(1)), rules, state) != now.Add(26*time.Hour).UnixMilli() {
		t.Fatal("calendar jump granted elapsed free time", state)
	}
	if err := trialRefresh82Save(db.GetIns(), row, state); err != nil {
		t.Fatal(err)
	}
	row, state = load(0)
	if state.FreeLeft != 0 || state.ResourceUsed != 15 {
		t.Fatal("rewind granted refreshed quota", state)
	}
	if err := trialRefresh82Save(db.GetIns(), row, state); err != nil {
		t.Fatal(err)
	}
	_, state = load(1)
	if state.FreeLeft != 0 || state.ResourceUsed != 0 {
		t.Fatal("return to next day reset cooldown", state)
	}
}
