package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP4LuckySupplyPublication verifies a genuine new source publication and its bounded quota.
// Wallet/level are finite domain prerequisites; the native test obtains its coin through star awards.
func TestP4LuckySupplyPublication(t *testing.T) {
	f := p4Setup(t)
	for _, init := range []func() error{func() error { return f.s.InitializeP10Lucky(f.ctx) }, func() error { return f.s.InitializeP10Scene(f.ctx) }, func() error { return f.s.InitializeP10Channel(f.ctx) }} {
		if err := init(); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(70)
		p.Coin = proto.Int64(6000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := constUtil.P4LuckySupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	query := &pb.CSActivityShopInfo{ActivityId: proto.String(p4ExchangeKey(now))}
	before := p4Trade(t, f, query, 0, now).(*pb.SCActivityShopInfo)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil || cfg.Enabled || len(cfg.Frozen) != 0 {
		t.Fatal("default-off unissued catalog", cfg.Enabled, err)
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	closed := p4Trade(t, f, query, 0, now).(*pb.SCActivityShopInfo)
	if !proto.Equal(before, closed) {
		t.Fatal("closed downstream exposed source")
	}
	for _, target := range []struct {
		model  any
		values map[string]any
	}{{&db.P10LuckyPolicy{}, map[string]any{"fireworks_enabled": true, "drop_enabled": true}}, {&db.P10ScenePolicy{}, map[string]any{"enabled": true}}, {&db.P10ChannelPolicy{}, map[string]any{"world_enabled": true}}} {
		if err := db.GetIns().Model(target.model).Where("id=1").Updates(target.values).Error; err != nil {
			t.Fatal(err)
		}
	}
	opened := p4Trade(t, f, query, 0, now).(*pb.SCActivityShopInfo)
	items := opened.GetShopItemList()
	if len(items) != len(before.GetShopItemList())+1 {
		t.Fatal("single new tail product", len(items))
	}
	for i, item := range before.GetShopItemList() {
		if !proto.Equal(item, items[i]) {
			t.Fatal("existing product/indices changed")
		}
	}
	item := items[len(items)-1]
	if item.GetGetResource()[0].GetId() != "item_350" || item.GetName() != "七彩烟花" || item.GetCostResource()[0].GetNum() != 2000000 {
		t.Fatal("original visible identity or local quote", item)
	}
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil || len(cfg.Frozen) == 0 {
		t.Fatal("actual publication did not freeze", err)
	}
	request := &pb.CSBuyActivityShop{ActivityId: query.ActivityId, Index: item.Index, Num: proto.Int32(1), CostResources: item.CostResource, GetResources: item.GetResource}
	first := p4Trade(t, f, request, 8101, now)
	if !proto.Equal(first, p4Trade(t, f, request, 8101, now)) {
		t.Fatal("committed purchase repeat changed")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, request, 8102, now); err == nil {
		t.Fatal("daily cap bypass")
	}
	// Frozen settings remain authoritative after live edits, and the seed preserves operator values.
	var edited p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &edited); err != nil {
		t.Fatal(err)
	}
	edited.Products[0].Cost = "coin,1"
	raw, _ := json.Marshal(edited)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3; n++ {
		var row db.P4CommerceState
		if err := db.GetIns().First(&row, "player_id=? AND rule_id=?", f.id, key).Error; err != nil {
			t.Fatal(err)
		}
		var state p4CommerceProgress
		if err := json.Unmarshal(row.Payload, &state); err != nil {
			t.Fatal(err)
		}
		state.Day = "20000101"
		row.Payload, _ = json.Marshal(state)
		if err := db.GetIns().Save(&row).Error; err != nil {
			t.Fatal(err)
		}
		_, err := f.s.p4Commerce(f.ctx, f.id, request, int64(8200+n), now)
		if n < 2 && err != nil || n == 2 && err == nil {
			t.Fatal("month3 bounded source", n, err)
		}
	}
	p, g := f.profile(t)
	if p.GetCoin() != 0 || simpleOwned(p, g, "item_350") != 3 {
		t.Fatal("real three purchases do not conserve coin/items")
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, p4Trade(t, f, query, 0, now)) || !proto.Equal(first, p4Trade(t, f, request, 8101, now)) {
		t.Fatal("operator-off catalog or already committed purchase")
	}
	if err := f.s.seedP4LuckySupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil || cfg.Enabled || string(cfg.Rules) != string(raw) || !strings.Contains(cfg.Source, "原服投放/售价/窗口/限额未知") {
		t.Fatal("operator config/source provenance overwritten", err)
	}
}
