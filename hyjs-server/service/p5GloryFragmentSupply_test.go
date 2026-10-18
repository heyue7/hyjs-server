package service

import (
	"context"
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

func p5GloryFragmentFixture(t *testing.T) (*p1Fixture, string, time.Time) {
	t.Helper()
	f := p4Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeP2, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	now := p4At(3, 12)
	if err := f.s.seedP5GloryFragmentSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	return f, constUtil.P5GloryFragmentSupplyPrefix + now.Format("200601"), now
}

func TestP5GloryFragmentSupplyClosedAndExactOriginalTargets(t *testing.T) {
	f, key, now := p5GloryFragmentFixture(t)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || len(cfg.Frozen) != 0 || cfg.Source != constUtil.P5GloryFragmentSupplySource {
		t.Fatal("default-off source changed")
	}
	products, err := f.s.p5GloryFragmentProducts()
	if err != nil || len(products) != 4 {
		t.Fatal(products, err)
	}
	for i, p := range products {
		if p.Index != int32(i+1) || p.Cost != "coin,100000" || p.Rewards != p.ID+",3" || p.MinLevel != 60 || p.DailyLimit != 5 || p.TotalLimit != 150 {
			t.Fatal("named defaults", p)
		}
		if f.s.table("Frag", "id", p.ID)["num"] != "60" {
			t.Fatal("original combine cost changed")
		}
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now); err == nil {
		t.Fatal("unpublished source visible")
	}
	for _, row := range f.s.Tables["Frag"] {
		if row["id"] == "frag_equip_044" {
			old := row["num"]
			row["num"] = "59"
			if _, err = f.s.p5GloryFragmentProducts(); err == nil {
				t.Fatal("changed original combine accepted")
			}
			row["num"] = old
		}
	}
}

// Capital/level are finite test qualifications. No fragments, equipment, gems,
// forge result or GUID is injected; all four bodies are actual sixty-fragment
// combines after five service dates of paid quota-limited source transactions.
func TestP5GloryFragmentSupplyFourActualCombineForgeWearRefundAndCold(t *testing.T) {
	f, key, now := p5GloryFragmentFixture(t)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(59)
		p.Coin = proto.Int64(10000000)
		p.Diamond = proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	outer := p4ExchangeKey(now)
	base := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(outer)}, 0, now).(*pb.SCActivityShopInfo)
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(outer)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.ShopItemList) != len(base.ShopItemList)+4 {
		t.Fatal("four late global addresses not appended")
	}
	for i, old := range base.ShopItemList {
		if !proto.Equal(old, listing.ShopItemList[i]) {
			t.Fatal("earlier index changed", i)
		}
	}
	quotes := listing.ShopItemList[len(base.ShopItemList):]
	makeBuy := func(q *pb.ActivityShopItem, n int32) *pb.CSBuyActivityShop {
		return &pb.CSBuyActivityShop{ActivityId: proto.String(outer), Index: q.Index, Num: proto.Int32(n), CostResources: q.CostResource, GetResources: q.GetResource}
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, makeBuy(quotes[0], 1), 998001, now); err == nil {
		t.Fatal("source60 gate bypass")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(60); return nil }); err != nil {
		t.Fatal(err)
	}
	stamp := int64(998010)
	for day := 0; day < 5; day++ {
		n := int32(5)
		if day == 4 {
			n = 1
		}
		for _, q := range quotes {
			stamp++
			buy := makeBuy(q, n)
			at := now.AddDate(0, 0, day)
			out := p4Trade(t, f, buy, stamp, at)
			if !proto.Equal(out, p4Trade(t, f, buy, stamp, at)) {
				t.Fatal("source replay changed")
			}
			if day == 0 {
				if _, err := f.s.p4Commerce(f.ctx, f.id, makeBuy(q, 1), stamp+10000, at); err == nil {
					t.Fatal("per-product daily5 bypass")
				}
			}
		}
	}
	p, g := f.profile(t)
	if p.GetCoin() != 1600000 {
		t.Fatal("four sources did not charge exactly8400000", p.GetCoin())
	}
	requestCtx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	for i, id := range []string{"equip_041", "equip_042", "equip_043", "equip_044"} {
		frag := "frag_" + id
		if simpleOwned(p, g, frag) != 63 {
			t.Fatal("actual paid fragment quantity", frag)
		}
		combine := &pb.CSCombine{TargetId: proto.String(id)}
		combined, err := f.s.redCombine(requestCtx(combine), f.id, combine, int64(999000+i))
		if err != nil {
			t.Fatal("original sixty-fragment combine", err)
		}
		if again, e := f.s.redCombine(requestCtx(combine), f.id, combine, int64(999000+i)); e != nil || !proto.Equal(again[0], combined[0]) {
			t.Fatal("combine receipt", e)
		}
		gear := combined[0].(*pb.SCCombine).GetGet().GetEquips()[0]
		if gear.GetEquipId() != id || gear.GetGuid() == "" {
			t.Fatal("actual unique original body")
		}
		p, g = f.profile(t)
		if simpleOwned(p, g, frag) != 3 {
			t.Fatal("original sixty-fragment deduction")
		}
		costs, odds, err := f.s.equipmentForgeCosts(gear)
		if err != nil || odds != 100 || len(costs) != 2 || costs[0].GetId() != frag || costs[0].GetNum() != 3 || costs[1].GetId() != "coin" || costs[1].GetNum() != 1000 {
			t.Fatal("original first forge contract", costs, odds, err)
		}
		forge := &pb.CSEquipForge{Guid: gear.Guid, Cost: costs}
		forged, err := f.s.equipmentForge(requestCtx(forge), f.id, forge, int64(999100+i))
		if err != nil {
			t.Fatal("real original guaranteed first step", err)
		}
		if forged[0].(*pb.SCEquipForge).GetResult() != 1 || forged[0].(*pb.SCEquipForge).GetEquip().GetForgeLevel() != 1 {
			t.Fatal("original first100-percent roll")
		}
		if _, err = f.s.redCombine(requestCtx(combine), f.id, combine, int64(999000+i)); err == nil {
			t.Fatal("old full-body combine replay overwrote grown GUID")
		}
		f.restart(t)
		if replay, e := f.s.equipmentForge(requestCtx(forge), f.id, forge, int64(999100+i)); e != nil || !proto.Equal(replay[0], forged[0]) {
			t.Fatal("cold exact forge receipt", e)
		}
		p, g = f.profile(t)
		before, err := f.s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.equipmentFormationOn(f.ctx, f.id, &pb.CSFormationOn{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(1)}); err != nil {
			t.Fatal(err)
		}
		p, g = f.profile(t)
		after, err := f.s.battlePlayer(p, g)
		if err != nil || proto.Equal(before.GetFormation()[0].GetProp(), after.GetFormation()[0].GetProp()) {
			t.Fatal("actual worn source has no static battle-panel effect", err)
		}
		if _, err = f.s.equipmentFormationOff(f.ctx, f.id, &pb.CSFormationOff{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(1)}); err != nil {
			t.Fatal(err)
		}
		reset := &pb.CSEquipReBuild{Guid: gear.Guid}
		if _, err = f.s.p5EquipmentReset(requestCtx(reset), f.id, reset, int64(999200+i)); err != nil {
			t.Fatal("actual paid forge investment refund", err)
		}
		p, g = f.profile(t)
		if simpleOwned(p, g, frag) != 3 {
			t.Fatal("original tracked3 fragments not refunded")
		}
		decompose := &pb.CSEquipDecompose{Guid: []string{gear.GetGuid()}}
		out, err := f.s.p5EquipmentDecompose(requestCtx(decompose), f.id, decompose, int64(999300+i))
		if err != nil {
			t.Fatal(err)
		}
		if again, e := f.s.p5EquipmentDecompose(requestCtx(decompose), f.id, decompose, int64(999300+i)); e != nil || !proto.Equal(out[0], again[0]) {
			t.Fatal("decompose once receipt", e)
		}
		if _, err = f.s.redCombine(requestCtx(combine), f.id, combine, int64(999000+i)); err == nil {
			t.Fatal("removed GUID resurrected by old combine")
		}
		p, g = f.profile(t)
	}
	if p.GetPoint() != 120000 || p.GetCoin() != 1604000 || p.GetDiamond() != 800 {
		t.Fatal("exact paid/refund/original decompose balances", p.GetPoint(), p.GetCoin(), p.GetDiamond())
	}
	beforeP, beforeGame := proto.Clone(p).(*pb.Player), p5GloryJSON(t, g)
	f.restart(t)
	p, g = f.profile(t)
	if !proto.Equal(beforeP, p) || string(beforeGame) != string(p5GloryJSON(t, g)) {
		t.Fatal("cold four-source lifecycle changed")
	}
}

func p5GloryJSON(t *testing.T, g *model.GameState) []byte {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestP5GloryFragmentSupplyPersonalRefreshKeepsOldDirectory(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	now := LocalPlayNow(f.ctx)
	tx := db.GetIns().WithContext(f.ctx)
	if err := f.s.seedP5GloryFragmentSupply(tx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "99700000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	key, err := localPlayExchangeKey(tx, now)
	if err != nil {
		t.Fatal(err)
	}
	directory, bindings, err := f.s.p4ExchangeDirectory(tx, key, now)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash, beforeRaw := directory.InputHash, string(directory.Bindings)
	global := constUtil.P5GloryFragmentSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", global).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p5AppendGloryFragmentBinding(tx, directory, &bindings, now); err != nil || directory.InputHash != beforeHash || string(directory.Bindings) != beforeRaw {
		t.Fatal("late enabled supply mutated private frozen directory", err)
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitExchange", "99700000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	newKey, err := localPlayExchangeKey(tx, now)
	if err != nil || newKey == key {
		t.Fatal("new personal cycle identity", err)
	}
	_, fresh, err := f.s.p4ExchangeDirectory(tx, newKey, now)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, binding := range fresh {
		if strings.HasPrefix(binding.ChildID, constUtil.P5GloryFragmentSupplyPrefix) {
			count++
			if !strings.HasPrefix(binding.ProductID, "frag_equip_04") {
				t.Fatal("unexpected new source product")
			}
		}
	}
	if count != 4 {
		t.Fatal("explicit refresh did not adopt all four glory-fragment sources", count)
	}
	var old db.P4ResourceExchangeDirectory
	if err := tx.First(&old, "id=?", key).Error; err != nil || old.InputHash != beforeHash || string(old.Bindings) != beforeRaw {
		t.Fatal("old directory history changed", err)
	}
}

func TestP5GloryFragmentSupplyMonthlyFrozenPriceAndShutdown(t *testing.T) {
	f, key, now := p5GloryFragmentFixture(t)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.Products[0].Cost = "coin,600000"
	cfg.Rules, _ = json.Marshal(rules)
	cfg.Enabled = true
	cfg.Frozen, _ = json.Marshal(cfg)
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	// Later live edits cannot replace the published frozen quote.
	if err := db.GetIns().Model(&cfg).Update("rules", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	next := now.AddDate(0, 1, 0)
	if err := f.s.seedP5GloryFragmentSupply(db.GetIns(), next); err != nil {
		t.Fatal(err)
	}
	var inherited db.P4CommerceRule
	nextKey := constUtil.P5GloryFragmentSupplyPrefix + next.Format("200601")
	if err := db.GetIns().First(&inherited, "id=?", nextKey).Error; err != nil {
		t.Fatal(err)
	}
	_, actual, err := f.s.p4CommerceSnapshot(inherited)
	if err != nil || !inherited.Enabled || actual.Products[0].Cost != "coin,600000" {
		t.Fatal("next month lost legitimate frozen publication", err)
	}
	if err = db.GetIns().Model(&inherited).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{next, next.AddDate(0, 1, 0)} {
		if err = f.s.seedP5GloryFragmentSupply(db.GetIns(), at); err != nil {
			t.Fatal(err)
		}
		var row db.P4CommerceRule
		if err = db.GetIns().First(&row, "id=?", constUtil.P5GloryFragmentSupplyPrefix+at.Format("200601")).Error; err != nil || row.Enabled {
			t.Fatal("restart or next month reenabled operator shutdown", err)
		}
	}
	// Private enabled clones do not turn global publication back on.
	clone := cfg
	clone.ID, clone.Frozen = key+"_lp-00000000000000000000000000", nil
	clone.StartAt, clone.EndAt = next.AddDate(0, 2, 0).UnixMilli(), next.AddDate(0, 3, 0).UnixMilli()
	clone.Enabled = true
	if !strings.Contains(clone.ID, "lp-") || db.GetIns().Create(&clone).Error != nil {
		t.Fatal("private continuity fixture")
	}
	future := next.AddDate(0, 3, 0)
	if err = f.s.seedP5GloryFragmentSupply(db.GetIns(), future); err != nil {
		t.Fatal(err)
	}
	var off db.P4CommerceRule
	if err = db.GetIns().First(&off, "id=?", constUtil.P5GloryFragmentSupplyPrefix+future.Format("200601")).Error; err != nil || off.Enabled {
		t.Fatal("private clone became global publication", err)
	}
}
