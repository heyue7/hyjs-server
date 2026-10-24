package service

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func reWelfarePublish(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, mode string, at time.Time) (model.AdminActivityInstance, string) {
	t.Helper()
	for _, form := range reOperationsWelfareForms() {
		if form.Mode != mode {
			continue
		}
		form.StartAt = at.Add(-time.Hour).UnixMilli()
		form.EndAt = at.AddDate(0, 0, 7).UnixMilli()
		form.ClaimEndAt = form.EndAt
		created, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
		if err != nil {
			t.Fatal("publish actual Re welfare", err)
		}
		_, key, _ := strings.Cut(created.ID, ":")
		return *created, key
	}
	t.Fatal("missing mode", mode)
	return model.AdminActivityInstance{}, ""
}

func TestReFortuneCatNativeCostsDailyReplayAndCold(t *testing.T) {
	f, actor := adminModesFixture(t)
	at := time.Now()
	created, key := reWelfarePublish(t, f, actor, "fortune-cat", at)
	if len(created.Form.Milestones) != 5 || created.Form.Milestones[4].Goal != 3000 || created.Form.Milestones[4].Rewards[0].Amount != 3150 {
		t.Fatal("backend lost actual cost and return", created.Form)
	}
	tabs, err := f.s.reOperationsWelfareTabs(db.GetIns().WithContext(f.ctx), at)
	if err != nil || len(tabs) != 1 || tabs[0].GetActivityId() != "fortuneCat_re_current~100,105~300,315~800,840~1500,1575~3000,3150" {
		t.Fatal("frozen native payout quote omitted", tabs, err)
	}
	// This finite wallet funds real native cat debits; no paid-order or consumption fact is injected.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Diamond = proto.Int64(10000)
		p.Coin = proto.Int64(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := info[0].(*pb.SCFortuneCatInfo).GetInfo(); got.GetTodayLeft() != 5 || got.GetCoinPool() != 100 || got.GetTotalStatus() != 0 {
		t.Fatal("native 5-step cat info", got)
	}
	var first proto.Message
	for i, want := range []int32{105, 315, 840, 1575, 3150} {
		req := &pb.CSFortuneCatReward{Type: proto.Int32(0)}
		out, err := f.s.p4Welfare(f.ctx, f.id, req, int64(500+i), at)
		if err != nil {
			t.Fatal("actual native cat debit", i, err)
		}
		reward := out[0].(*pb.SCFortuneCatReward)
		if len(reward.GetGet().GetSimps()) != 1 || reward.GetGet().GetSimps()[0].GetId() != "coin" || reward.GetGet().GetSimps()[0].GetNum() != want || reward.GetInfo().GetTodayLeft() != int32(4-i) {
			t.Fatal("wrong native full return", reward)
		}
		if i == 0 {
			first = out[0]
		}
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 4300 || p.GetCoin() != 5985 {
		t.Fatal("five actual debits/returns not conserved", p.GetDiamond())
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent) != 0 {
		t.Fatal("Re cat fabricated ordinary spending")
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 505, at); err == nil {
		t.Fatal("sixth cat accepted")
	}
	f.restart(t)
	old, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 500, at.AddDate(0, 0, 1))
	if err != nil || !proto.Equal(first, old[0]) {
		t.Fatal("cold native receipt changed", err)
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(1)}, 500, at); err == nil {
		t.Fatal("same stamp changed cat type")
	}
	var validLive db.P4WelfareRule
	if err = db.GetIns().First(&validLive, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P4WelfareRule{}).Where("id=?", key).Update("rules", []byte(`{"tiers":[]}`)).Error; err != nil {
		t.Fatal(err)
	}
	// New Info must validate the latest quote, while old receipts stay immutable.
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at.AddDate(0, 0, 1)); err == nil {
		t.Fatal("invalid live quote accepted")
	}
	if err = db.GetIns().Model(&db.P4WelfareRule{}).Where("id=?", key).Update("rules", validLive.Rules).Error; err != nil {
		t.Fatal(err)
	}
	next, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at.AddDate(0, 0, 1))
	if err != nil || next[0].(*pb.SCFortuneCatInfo).GetInfo().GetTodayLeft() != 5 {
		t.Fatal("validated live quote or new day reset", err)
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 506, at.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	back, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at)
	if err != nil || back[0].(*pb.SCFortuneCatInfo).GetInfo().GetTodayLeft() != 0 {
		t.Fatal("calendar rewind reset old daily uses", err)
	}
	// A reward overflow must roll back both the debit and the daily ordinal.
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Diamond = proto.Int64(math.MaxInt64)
		p.Coin = proto.Int64(math.MaxInt64)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 507, at.AddDate(0, 0, 1)); err == nil {
		t.Fatal("overflow return committed")
	}
	p, _ = f.profile(t)
	if p.GetDiamond() != math.MaxInt64 {
		t.Fatal("failed cat debit leaked")
	}
	if err = db.GetIns().Model(&db.P4WelfareRule{}).Where("id=?", key).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at); err == nil {
		t.Fatal("disabled cat query still active")
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 500, at); err != nil {
		t.Fatal("disable erased historical receipt", err)
	}
}

func TestReContinuedSpendNativeSevenRealDaysAndFinal(t *testing.T) {
	f, actor := adminModesFixture(t)
	// The trusted event producer rejects future dates. Replay seven bounded historical test dates.
	at := time.Now().AddDate(0, 0, -6)
	_, key := reWelfarePublish(t, f, actor, "continued-spend", at)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(100000); return nil }); err != nil {
		t.Fatal(err)
	}
	// Real commerce transactions produce each day's diamond facts. Advancing at is a finite calendar fixture.
	shop := adminModeForm("discount-shop")
	shop.StartAt = at.Add(-time.Hour).UnixMilli()
	shop.EndAt = at.AddDate(0, 0, 8).UnixMilli()
	shop.ClaimEndAt = shop.EndAt
	for _, amount := range []int64{2999, 1, 3000} {
		shop.Products = append(shop.Products, model.AdminActivityProduct{ID: "re-consume-" + strconv.FormatInt(amount, 10), Name: "有限消费验证商品", Index: int64(len(shop.Products) + 1), Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 1}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: amount}, DailyLimit: 10, TotalLimit: 100, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1})
	}
	created, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: shop, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, shopKey, _ := strings.Cut(created.ID, ":")
	buy := func(cost int32, stamp int64, day time.Time) {
		t.Helper()
		index := int32(3)
		if cost == 2999 {
			index = 1
		} else if cost == 1 {
			index = 2
		}
		_, e := f.s.p4Commerce(f.ctx, f.id, &pb.CSGetActivityShopReward{ActivityId: proto.String(shopKey), Index: proto.Int32(index), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(cost)}}, GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1)}}}, stamp, day)
		if e != nil {
			t.Fatal("real native diamond purchase", cost, e)
		}
	}
	query := &pb.CSContinuedRechargeInfo{ActivityId: proto.String(key)}
	initial, handled, err := f.s.HandlePayment(f.ctx, f.id, query, 0)
	if err != nil || !handled {
		t.Fatal("native continued dispatch", err)
	}
	if got := initial[0].(*pb.SCContinuedRechargeInfo); got.GetParam() != "3000" || got.GetLevel() != 7 || len(got.GetItemList()) != 7 || got.GetBigReward()[0].GetNum() != 9000 {
		t.Fatal("actual continued config missing", got)
	}
	if _, err = f.s.reContinuedWelfare(f.ctx, f.id, &pb.CSContinuedRechargeReward{ActivityId: proto.String(key), Type: proto.Int32(1), Index: proto.Int32(-1), GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(9000)}}}, 600, at); err == nil {
		t.Fatal("unearned big reward")
	}
	var first proto.Message
	for day := 1; day <= 7; day++ {
		now := at.AddDate(0, 0, day-1)
		if day == 1 {
			buy(2999, 6000, now)
			out, e := f.s.reContinuedWelfare(f.ctx, f.id, query, 0, now)
			if e != nil || out[0].(*pb.SCContinuedRechargeInfo).GetItemList()[0].GetStatus() != 0 {
				t.Fatal("2999 qualifies", e)
			}
			buy(1, 6001, now)
		} else {
			buy(3000, int64(6000+day), now)
		}
		out, e := f.s.reContinuedWelfare(f.ctx, f.id, query, 0, now)
		if e != nil {
			t.Fatal(e)
		}
		info := out[0].(*pb.SCContinuedRechargeInfo)
		if info.GetCurrentIndex() != int32(day) || info.GetItemList()[day-1].GetStatus() != 1 {
			t.Fatal("wrong next earned native day", day, info)
		}
		req := &pb.CSContinuedRechargeReward{ActivityId: proto.String(key), Type: proto.Int32(0), Index: proto.Int32(int32(day)), GetResources: info.GetItemList()[day-1].GetRewardList()}
		claimed, e := f.s.reContinuedWelfare(f.ctx, f.id, req, int64(7000+day), now)
		if e != nil {
			t.Fatal("actual day reward", day, e)
		}
		if day == 1 {
			first = claimed[0]
		}
		if _, e = f.s.reContinuedWelfare(f.ctx, f.id, req, int64(7100+day), now); e == nil {
			t.Fatal("day reward duplicated", day)
		}
		if day < 7 {
			nextReq := &pb.CSContinuedRechargeReward{ActivityId: proto.String(key), Type: proto.Int32(0), Index: proto.Int32(int32(day + 1)), GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(int32((day + 1) * 900))}}}
			if _, e = f.s.reContinuedWelfare(f.ctx, f.id, nextReq, int64(7200+day), now); e == nil {
				t.Fatal("same actual day advanced twice")
			}
		}
		if day == 3 {
			f.restart(t)
		}
	}
	before, _ := f.profile(t)
	if before.GetDiamond() != 104200 {
		t.Fatal("7 real 3000 debits and day rewards", before.GetDiamond())
	}
	old, err := f.s.reContinuedWelfare(f.ctx, f.id, &pb.CSContinuedRechargeReward{ActivityId: proto.String(key), Type: proto.Int32(0), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(900)}}}, 7001, at.AddDate(0, 0, 6))
	if err != nil || !proto.Equal(first, old[0]) {
		t.Fatal("cold continued receipt", err)
	}
	final := &pb.CSContinuedRechargeReward{ActivityId: proto.String(key), Type: proto.Int32(1), Index: proto.Int32(-1), GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(9000)}}}
	if _, err = f.s.reContinuedWelfare(f.ctx, f.id, final, 8000, at.AddDate(0, 0, 6)); err != nil {
		t.Fatal("earned final reward", err)
	}
	if _, err = f.s.reContinuedWelfare(f.ctx, f.id, final, 8001, at.AddDate(0, 0, 6)); err == nil {
		t.Fatal("final duplicated")
	}
	after, _ := f.profile(t)
	if after.GetDiamond() != 113200 {
		t.Fatal("actual final payout", after.GetDiamond())
	}
	var frozen db.P4WelfareState
	if err = db.GetIns().First(&frozen, "player_id=? AND rule_id=?", f.id, key).Error; err != nil {
		t.Fatal(err)
	}
	var saved reOperationsWelfareProgress
	if json.Unmarshal(frozen.Payload, &saved) != nil || !saved.FinalClaimed || len(saved.ClaimDays) != 7 {
		t.Fatal("actual claim history missing")
	}
}

func TestReFortuneCatConcurrentOriginalStamp(t *testing.T) {
	f, actor := adminModesFixture(t)
	at := time.Now()
	reWelfarePublish(t, f, actor, "fortune-cat", at)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 999, at)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent original stamp", err)
		}
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 900 || p1Count(t, &db.P4Receipt{}, "player_id=? AND domain=?", f.id, constUtil.ReWelfareCatKind) != 1 {
		t.Fatal("race duplicated paid cat", p.GetDiamond())
	}
}

func TestReFortuneCatConfiguredPayoutMatchesNativeQuote(t *testing.T) {
	f, actor := adminModesFixture(t)
	at := time.Now()
	created, key := reWelfarePublish(t, f, actor, "fortune-cat", at)
	created.Form.Milestones[0].Goal = 123
	created.Form.Milestones[0].Rewards[0].Amount = 777
	if _, err := f.s.AdminSaveTypedActivity(f.ctx, actor, created.ID, model.AdminTypedActivityRequest{AdminActivityForm: created.Form, Version: created.Version, Enabled: true}); err != nil {
		t.Fatal("operator payout update", err)
	}
	tabs, err := f.s.reOperationsWelfareTabs(db.GetIns().WithContext(f.ctx), at)
	if err != nil || len(tabs) != 1 || !strings.HasPrefix(tabs[0].GetActivityId(), "fortuneCat_re_current~123,777~") {
		t.Fatal("native quote does not reflect saved configuration", tabs, err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(123); return nil }); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 10001, at)
	if err != nil || out[0].(*pb.SCFortuneCatReward).GetGet().GetSimps()[0].GetNum() != 777 {
		t.Fatal("configured native full return", err)
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 0 {
		t.Fatal("actual configured cost was not deducted", p.GetDiamond())
	}
	var cfg db.P4WelfareRule
	if err = db.GetIns().First(&cfg, "id=?", key).Error; err != nil || len(cfg.Frozen) == 0 {
		t.Fatal("visible quote was not frozen", err)
	}
}

// Existing frozen diamond rows change only future payout resource; counts and committed receipts survive.
func TestReTreasureLegacyFrozenPayoutCorrection(t *testing.T) {
	f, actor := adminModesFixture(t)
	at := time.Now()
	_, key := reWelfarePublish(t, f, actor, "fortune-cat", at)
	var cfg db.P4WelfareRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	cfg.Rules = []byte(strings.ReplaceAll(string(cfg.Rules), "coin,", "diamond,"))
	frozen := adminJSON(cfg)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": cfg.Rules, "frozen": frozen}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Diamond = proto.Int64(1000)
		p.Coin = proto.Int64(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 88001, at)
	if err != nil {
		t.Fatal(err)
	}
	r := first[0].(*pb.SCFortuneCatReward)
	if r.GetGet().GetSimps()[0].GetId() != "coin" || r.GetInfo().GetTodayLeft() != 4 {
		t.Fatal(r)
	}
	f.restart(t)
	replay, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(0)}, 88001, at)
	if err != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("cold receipt", err)
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 900 || p.GetCoin() != 105 {
		t.Fatal("payout correction duplicated or refunded diamonds", p)
	}
	if err = db.GetIns().First(&cfg, "id=?", key).Error; err != nil || string(cfg.Frozen) != string(frozen) {
		t.Fatal("historical frozen configuration rewritten", err)
	}
}
