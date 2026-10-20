package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func p4Fund(t *testing.T, f *p1Fixture) {
	t.Helper()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Diamond = proto.Int64(100000)
		p.Light = proto.Int32(1000)
		p.Point = proto.Int32(1000)
		p.Popularity = proto.Int32(1000)
		p.Score = proto.Int32(1000)
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_013"), Num: proto.Int32(1)}, {Id: proto.String("item_021"), Num: proto.Int32(10)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func p4Trade(t *testing.T, f *p1Fixture, b proto.Message, stamp int64, at time.Time) proto.Message {
	t.Helper()
	r, e := f.s.p4Commerce(f.ctx, f.id, b, stamp, at)
	if e != nil {
		t.Fatal(e)
	}
	return r[0]
}

func TestP4OrdinaryOriginalTierPriceAndReceiptLimits(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	now := time.Now()
	expected, e := f.s.p4TierCost("itemShopId_1", 1, 6)
	if e != nil {
		t.Fatal(e)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_1"), BuyCount: proto.Int32(6)}
	first := p4Trade(t, f, buy, 1, now).(*pb.SCShopBuy)
	if !p1SameResources(first.GetCost(), []*pb.BaseResource{expected}) {
		t.Fatal("original tier sum changed")
	}
	// The six available variable tiers and eight fixed slots include the recovered
	// original item004 slot12. Verify the complete directory and its actual terms.
	catalogue := map[string]*pb.ShopItem{}
	for _, item := range first.GetShopItemList() {
		if catalogue[item.GetId()] != nil {
			t.Fatal("duplicate ordinary shop slot", item.GetId())
		}
		catalogue[item.GetId()] = item
	}
	for _, suffix := range []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 16} {
		if catalogue[fmt.Sprintf("itemShopId_%d", suffix)] == nil {
			t.Fatal("missing original or recovered ordinary shop slot", suffix)
		}
	}
	if catalogue["itemShopId_2"] != nil {
		t.Fatal("direct stamina product remained visible")
	}
	wanted := catalogue["itemShopId_12"]
	if len(catalogue) != 14 || !p1SameResources(wanted.GetItem(), []*pb.BaseResource{{Id: proto.String("item_004"), Num: proto.Int32(1)}}) || !p1SameResources(wanted.GetPrice(), []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(200)}}) || wanted.GetDayTotalTimes() != 10 || wanted.GetDayLeftTimes() != 10 {
		t.Fatal("recovered wanted slot or full ordinary directory changed", wanted)
	}
	before, _ := f.profile(t)
	again := p4Trade(t, f, buy, 1, now)
	after, _ := f.profile(t)
	if !proto.Equal(first, again) || before.GetDiamond() != after.GetDiamond() {
		t.Fatal("tier retry charged")
	}
	buy.BuyCount = proto.Int32(15)
	if _, e := f.s.p4Commerce(f.ctx, f.id, buy, 2, now); e == nil {
		t.Fatal("tier day quota")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent) != 1 {
		t.Fatal("variable price missing/duplicate actual spend")
	}
}
func TestP4MemoryRefreshFreeTicketDiamondAndWindow(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	now := time.Now().Add(-6 * time.Hour)
	q := &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
	r := p4Trade(t, f, q, 0, now).(*pb.SCRefreshShopQuery)
	if len(r.GetShopItemList()) != 4 || r.GetFreeLeftTimes() != 1 {
		t.Fatal("initial memory catalogue")
	}
	refresh := &pb.CSRefreshShopRefresh{ShopType: proto.Int32(3)}
	one := p4Trade(t, f, refresh, 1, now).(*pb.SCRefreshShopRefresh)
	if len(one.GetCost()) != 0 || one.GetFreeLeftTimes() != 0 {
		t.Fatal("free refresh")
	}
	two := p4Trade(t, f, refresh, 2, now).(*pb.SCRefreshShopRefresh)
	if two.GetCost()[0].GetId() != "item_013" {
		t.Fatal("ticket priority")
	}
	three := p4Trade(t, f, refresh, 3, now).(*pb.SCRefreshShopRefresh)
	if three.GetCost()[0].GetId() != "diamond" || three.GetCost()[0].GetNum() != 20 {
		t.Fatal("diamond fallback")
	}
	current := p4Trade(t, f, q, 0, now.Add(4*time.Hour)).(*pb.SCRefreshShopQuery)
	if current.GetFreeLeftTimes() != 2 {
		t.Fatal("free regeneration")
	}
	slot := current.GetShopItemList()[0]
	purchase := &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: slot.Id}
	p4Trade(t, f, purchase, 4, now.Add(4*time.Hour))
	if _, e := f.s.p4Commerce(f.ctx, f.id, purchase, 5, now.Add(4*time.Hour)); e == nil {
		t.Fatal("same slot bought twice")
	}
	p4Trade(t, f, refresh, 5, now.Add(4*time.Hour))
	p, _ := f.profile(t)
	if p.GetDiamond() != 99980 {
		t.Fatal("refresh charged unexpectedly")
	}
}
func TestP4RealmCurrenciesAndRedEssenceExistingInstances(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	if _, e := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("p4Point_2"), BuyCount: proto.Int32(1)}, 1, now); e == nil {
		t.Fatal("unearned red exchange")
	}
	p4Fund(t, f)
	for i, spec := range []struct {
		typ int32
		sku string
	}{{4, "p4Popularity_1"}, {5, "p4Point_1"}, {7, "p4Score_1"}, {5, "p4Point_2"}} {
		p4Trade(t, f, &pb.CSShopBuy{ShopType: proto.Int32(spec.typ), ShopItemId: proto.String(spec.sku), BuyCount: proto.Int32(1)}, int64(i+1), now)
	}
	p, g := f.profile(t)
	if p.GetPopularity() != 980 || p.GetPoint() != 900 || p.GetScore() != 950 || simpleOwned(p, g, "item_021") != 1 || len(g.Equipment) != 1 || g.Equipment[0].GetEquipId() != "equip_037" {
		t.Fatal("actual alternate currencies or original red instance")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "spend:score") != 1 {
		t.Fatal("score spend missing")
	}
}
func TestP4ActivitySharedStockFrozenQuotesAndMystic(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	now := time.Now()
	month := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	key := "limitSale_p4_" + month
	// 本例显式开放独立测试库中的限时特卖和神秘小屋，生产模板继续默认关闭。
	opened := db.GetIns().Model(&db.P4CommerceRule{}).Where("id IN ?", []string{key, "mysticDiscount_p4_" + month}).Update("enabled", true)
	if opened.Error != nil || opened.RowsAffected != 2 {
		t.Fatal("fixture activities were not both enabled", opened.Error, opened.RowsAffected)
	}
	var cfg db.P4CommerceRule
	db.GetIns().Where("id=?", key).First(&cfg)
	var rules p4CommerceRules
	json.Unmarshal(cfg.Rules, &rules)
	rules.Products[0].Stock = 1
	raw, _ := json.Marshal(rules)
	db.GetIns().Model(&cfg).Update("rules", raw)
	q := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	item := q.GetShopItemList()[0]
	bad := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(1)}}, GetResources: item.GetGetResource()}
	if _, e := f.s.p4Commerce(f.ctx, f.id, bad, 1, now); e == nil {
		t.Fatal("quote tamper")
	}
	account, e := LoginAccount(f.ctx, "p4-second-"+fmt.Sprint(now.UnixNano()), "isolated-password")
	if e != nil {
		t.Fatal(e)
	}
	CreatePlayer(f.ctx, account.Id, "共享库存", "hero_043")
	second := *f
	second.id = account.Id
	p4Fund(t, &second)
	buy := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: item.GetCostResource(), GetResources: item.GetGetResource()}
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for _, id := range []int64{f.id, second.id} {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); _, e := f.s.p4Commerce(f.ctx, id, buy, 1, now); out <- e }(id)
	}
	wg.Wait()
	close(out)
	success := 0
	for e := range out {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("shared stock oversold", success)
	}
	info := p4Trade(t, f, &pb.CSMysticDiscountInfo{}, 0, now).(*pb.SCMysticDiscountInfo)
	draw := &pb.CSMysticDiscount{ActivityId: info.ActivityId}
	first := p4Trade(t, f, draw, 1, now).(*pb.SCMysticDiscount)
	again := p4Trade(t, f, draw, 2, now).(*pb.SCMysticDiscount)
	if first.GetDiscount() != again.GetDiscount() || first.GetDiscount() < 0.5 || first.GetDiscount() > 0.8 {
		t.Fatal("mystic reroll")
	}
}

func TestP4RecruitRotationPreviewAndGuaranteeFrozen(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	var one, two []db.GameplayPoolReward
	err := db.GetIns().Transaction(func(tx *gorm.DB) error { var err error; one, _, err = f.s.loadP4Rotation(tx, 2, now); return err })
	if err != nil {
		t.Fatal(err)
	}
	var cfg db.P4RecruitRotation
	db.GetIns().Where("kind=? AND start_at<=? AND end_at>?", 2, now.UnixMilli(), now.UnixMilli()).First(&cfg)
	db.GetIns().Model(&cfg).Update("rewards", []byte(`[]`))
	err = db.GetIns().Transaction(func(tx *gorm.DB) error { var e error; _, two, e = f.s.loadGameplayPool(tx, "recruit_2"); return e })
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(one)
	b, _ := json.Marshal(two)
	if string(a) != string(b) {
		t.Fatal("preview actual mismatch")
	}
	guaranteed, e := choosePoolReward(two, true)
	if e != nil || !guaranteed.Guaranteed || f.s.table("Hero", "id", guaranteed.ResourceID)["quality"] != "20" {
		t.Fatal("rotated guarantee")
	}
	next := time.UnixMilli(cfg.EndAt).Add(time.Minute)
	var rotated []db.GameplayPoolReward
	err = db.GetIns().Transaction(func(tx *gorm.DB) error { var e error; rotated, _, e = f.s.loadP4Rotation(tx, 2, next); return e })
	if err != nil {
		t.Fatal(err)
	}
	r, _ := json.Marshal(rotated)
	if string(a) == string(r) {
		t.Fatal("rotation never changed")
	}
}
func TestP4ActualRebateDailyCapReplayAndFacts(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	key := "consumeRet_p4_" + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	f.event(t, "diamond:actual", constUtil.P1EventDiamondSpent, 1234, now)
	r, e := f.s.p4Rebate(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)}, 0, now)
	if e != nil || r[0].(*pb.SCActivityInfo).GetItemList()[0].GetRewardList()[0].GetNum() != 100 {
		t.Fatal("actual cap", e)
	}
	request := &pb.CSConsumeReturnReward{ActivityId: proto.String(key)}
	if _, e = f.s.p4Rebate(f.ctx, f.id, request, 1, now); e != nil {
		t.Fatal(e)
	}
	f.event(t, "diamond:later", constUtil.P1EventDiamondSpent, 100, now)
	if _, e = f.s.p4Rebate(f.ctx, f.id, request, 2, now); e != nil {
		t.Fatal(e)
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 100 || p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key LIKE ?", f.id, "rebate:%") != 1 {
		t.Fatal("daily rebate amplified")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent) != 2 {
		t.Fatal("rebate fabricated spend")
	}
}

func TestP4RegistryAndRealPowerCollection(t *testing.T) {
	f := p4Setup(t)
	if p1Count(t, &db.P4ActivityRegistry{}, "1=1") != 36 {
		t.Fatal("36 registry")
	}
	for i := 0; i < 2; i++ {
		if e := f.s.RecordP4Login(f.ctx, f.id); e != nil {
			t.Fatal(e)
		}
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindPower) != 1 {
		t.Fatalf("initial actual stats duplicated collect=%d power=%d", p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect), p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindPower))
	}
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var row db.Profile
		tx.First(&row, f.id)
		if _, err := f.s.GrantFixedReward(p, g, "hero_001,1"); err != nil {
			return err
		}
		return f.s.RecordP4Save(tx, &row, p, g)
	})
	if err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect) != 2 {
		t.Fatal("new real hero not recorded")
	}
	// Type9 now consumes only accepted modern Arena challenger wins. Registry
	// readiness must preserve the default-off policy and cannot manufacture a win.
	var arena db.P4ActivityRegistry
	if err := db.GetIns().First(&arena, 9).Error; err != nil {
		t.Fatal(err)
	}
	if arena.Owner != "P4/P11+P1" || !strings.Contains(arena.Status, "实际modern23101挑战胜者事实") || !strings.Contains(arena.Status, "默认off") || !strings.Contains(arena.Evidence, "VsGetItem原胜利次数") {
		t.Fatal("verified Arena Type9 registration missing source boundary", arena)
	}
	var activity db.PeriodInstance
	if err := db.GetIns().Where("id LIKE ?", constUtil.P4ArenaWinPeriodPrefix+"%").First(&activity).Error; err != nil {
		t.Fatal(err)
	}
	if activity.Enabled || activity.Kind != constUtil.P4ArenaWinKind || activity.Source != constUtil.P4ArenaWinSource || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4ArenaWinKind) != 0 {
		t.Fatal("registry/collection granted an unverified Arena win or opened the local policy", activity)
	}
	// Type10 uses only actual accepted independent type3 challenges. Reading
	// the registry or collection cannot open a local month or generate attempts.
	var plunder db.P4ActivityRegistry
	if err := db.GetIns().First(&plunder, 10).Error; err != nil {
		t.Fatal(err)
	}
	if plunder.Owner != "P4/P11+P1" || !strings.Contains(plunder.Status, "实际type3挑战次数") || !strings.Contains(plunder.Status, "默认off") || !strings.Contains(plunder.Evidence, "原RobTrea次数") {
		t.Fatal("verified original Type10 source boundary", plunder)
	}
	var plunderMonth db.PeriodInstance
	if err := db.GetIns().Where("id LIKE ?", constUtil.P4PlunderActivityPrefix+"%").First(&plunderMonth).Error; err != nil {
		t.Fatal(err)
	}
	if plunderMonth.Enabled || plunderMonth.Kind != constUtil.P4PlunderActivityKind || plunderMonth.Source != constUtil.P4PlunderActivitySource || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4PlunderActivityKind) != 0 {
		t.Fatal("registry/collection manufactured an attempt or opened the local policy", plunderMonth)
	}
	// Neighboring unverified producers retain their original pending boundary.
	for _, spec := range []struct {
		id    int32
		owner string
	}{{11, "P12"}, {12, "P12"}} {
		var pending db.P4ActivityRegistry
		if err := db.GetIns().First(&pending, spec.id).Error; err != nil {
			t.Fatal(err)
		}
		if pending.Owner != spec.owner || !strings.Contains(pending.Status, "待") {
			t.Fatal("unverified activity producer no longer pending", pending)
		}
	}
}

// TestP4NewPowerWindowReceivesUnchangedActualPower 验证当前战力进入新实例，重连不重复参与。
func TestP4NewPowerWindowReceivesUnchangedActualPower(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.RecordP4Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	power, err := f.s.formationFightPower(p, g)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cfg := db.PeriodInstance{ID: fmt.Sprintf("kaiFuJingSai_p4_entry_%d", now.UnixNano()), Enabled: true, Title: "新窗口战力", Kind: constUtil.P4KindPower, Mode: "single", Goal: math.MaxInt32, Rewards: "coin,1", StartAt: now.Add(-time.Minute).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), RankPlaces: 1, RankRewards: "coin,1", Source: "隔离当前真实战力回归"}
	if err = db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = f.s.RecordP4Login(f.ctx, f.id); err != nil {
			t.Fatal(err)
		}
	}
	var progress db.PeriodProgress
	if err = db.GetIns().Where("player_id=? AND instance_id=?", f.id, cfg.ID).First(&progress).Error; err != nil || progress.Value != int64(power) {
		t.Fatal("unchanged actual power missing in new window", err)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "power-entry:"+cfg.ID) != 1 {
		t.Fatal("new window entry duplicated")
	}
}

// TestP4TabsUseFrozenCommerceAndWelfareEnable 验证入口投放使用冻结窗口并尊重运营启停。
func TestP4TabsUseFrozenCommerceAndWelfareEnable(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	month := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	key := "halfDiscountSale_p4_" + month
	p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now)
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Updates(map[string]any{"rules": []byte(`{}`), "end_at": now.Add(-time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P4WelfareRule{}).Where("kind IN ?", []string{constUtil.P4WelfareCat, constUtil.P4WelfareRebate}).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	tabs, err := f.s.P4ActivityTabs(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tab := range tabs {
		found = found || tab.GetActivityId() == key
		if strings.HasPrefix(tab.GetActivityId(), "fortuneCat_") || strings.HasPrefix(tab.GetActivityId(), "consumeRet_") {
			t.Fatal("disabled welfare still advertised")
		}
	}
	if !found {
		t.Fatal("frozen live shop hidden after mutable edit")
	}
	p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now)
}

// TestP4OriginalRecruitGiftAndRealPowerCutoff 验证成功招募驱动实际活动奖励，真实战力截止快照走公共投递。
func TestP4OriginalRecruitGiftAndRealPowerCutoff(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	for _, initialize := range []func(context.Context) error{f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	request := &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(2)}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, request)
	if _, err := f.s.warriorRecruit(ctx, f.id, request, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.warriorRecruit(ctx, f.id, request, 100); err != nil {
		t.Fatal(err)
	}
	key := "warriorRescruitTimes_p4_" + time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	rows, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
	if err != nil {
		t.Fatal(err)
	}
	item := rows[0].(*pb.SCActivityInfo).GetItemList()[0]
	if item.GetStatus() != 1 || item.GetLeft() != 10 {
		t.Fatal("actual recruit gift", item)
	}
	if _, err = f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1), GetResources: item.GetRewardList()}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1), GetResources: item.GetRewardList()}); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_048") != 1 {
		t.Fatal("recruit gift repeated")
	}
	var facts []db.EventFact
	db.GetIns().Where("player_id=? AND kind=?", f.id, constUtil.P1EventRecruit).Find(&facts)
	if len(facts) != 1 || facts[0].Amount != 10 {
		t.Fatal("recruit receipt produced duplicate fact")
	}
	// 新短窗口通过真实阵容变更生成新峰值来源，保留截止后邮件与领奖中心同一奖。
	cfg := db.PeriodInstance{ID: fmt.Sprintf("kaiFuJingSai_p4_test_%d", time.Now().UnixNano()), Enabled: true, Title: "实际战力截止", Kind: constUtil.P4KindPower, Mode: "single", Goal: math.MaxInt32, Rewards: "coin,1", StartAt: time.Now().Add(-time.Minute).UnixMilli(), EndAt: time.Now().Add(time.Second).UnixMilli(), RewardEndAt: time.Now().Add(time.Hour).UnixMilli(), RankPlaces: 1, RankRewards: "coin,17", Source: "隔离真实战力排名回归"}
	if err = db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var row db.Profile
		tx.First(&row, f.id)
		// 主角成长由Player.Level驱动，与真实升级保存路径一致。
		p.Level = proto.Int32(76)
		return f.s.RecordP4Save(tx, &row, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	power, err := f.s.formationFightPower(p, g)
	if err != nil {
		t.Fatal(err)
	}
	var progress db.PeriodProgress
	if err = db.GetIns().Where("player_id=? AND instance_id=?", f.id, cfg.ID).First(&progress).Error; err != nil || progress.Value != int64(power) {
		t.Fatal("server actual power", err, progress.Value, power)
	}
	time.Sleep(time.Until(time.UnixMilli(cfg.EndAt)) + 30*time.Millisecond)
	if err = f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, "rank:"+cfg.ID) != 1 || p1Count(t, &db.MailMessage{}, "player_id=? AND source_key=?", f.id, "rank:"+cfg.ID) != 1 {
		t.Fatal("actual CP cutoff delivery missing")
	}
}
