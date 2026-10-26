package service

import (
	"encoding/json"
	"gorm.io/gorm"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// These explicit, finite non-red operating sources exist only in the isolated
// local-play fixture. Default-disabled production seeds remain unchanged.
func localPlaySourcesFixture75(t *testing.T, f *p1Fixture) {
	t.Helper()
	// One finite non-red exchange makes the original personal directory refresh real.
	now := LocalPlayNow(f.ctx)
	exchangeStart, exchangeEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	products, _ := json.Marshal(p4CommerceRules{Products: []p4Product{p4BaseProduct("item_005", "item_005,1", "coin,1000", 1, 1)}})
	exchange := db.P4CommerceRule{ID: "exchange_p4_fixture75", Kind: "exchange", Enabled: true, Rules: products, Source: "explicit finite isolated material exchange fixture75", StartAt: exchangeStart.UnixMilli(), EndAt: exchangeEnd.UnixMilli(), ClaimEndAt: exchangeEnd.UnixMilli()}
	if err := db.GetIns().Where("id=?", exchange.ID).FirstOrCreate(&exchange).Error; err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{constUtil.P4WelfareLogin, constUtil.P4WelfareLimit, constUtil.P4WelfareRebate} {
		localWelfareSourceFixture75(t, kind)
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	for _, spec := range []struct {
		id   string
		goal int64
	}{{"totalLogin_p1_fixture75_welcome", 1}, {"totalLogin_p1_fixture75_three", 3}} {
		cfg := db.PeriodInstance{ID: spec.id, Enabled: true, Title: "隔离登录检查", Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: spec.goal, Rewards: "coin,1000", StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), RewardEndAt: start.AddDate(0, 1, 7).UnixMilli(), Source: "explicit isolated non-red local-play fixture"}
		if err := db.GetIns().Create(&cfg).Error; err != nil {
			t.Fatal(err)
		}
	}
	copyProgress := db.PeriodInstance{ID: "mainCopyWinTimes_p4_fixture75", Enabled: true, Title: "隔离主线检查", Kind: constUtil.P1EventCopyWin, Mode: "cumulative", Goal: 10, Rewards: "coin,17", StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), RewardEndAt: start.AddDate(0, 1, 7).UnixMilli(), Source: "explicit isolated copy-fact fixture"}
	if err := db.GetIns().Create(&copyProgress).Error; err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"rechargeSingle", "rechargeTotal", "continuedRecharge", "costTotal"} {
		var rule db.PaymentActivityRule
		if err := db.GetIns().First(&rule, "id=?", prefix+"_local_p3").Error; err != nil {
			t.Fatal(err)
		}
		rule.ID, rule.Enabled = prefix+"_fixture75_local_p3", true
		rule.Reward, rule.Source = "coin,17", "explicit isolated non-red payment-metric fixture"
		if err := db.GetIns().Create(&rule).Error; err != nil {
			t.Fatal(err)
		}
	}
	var copySource db.P4CopyBenefitRule
	if err := db.GetIns().Order("start_at DESC,id DESC").First(&copySource).Error; err != nil {
		t.Fatal(err)
	}
	copySource.ID, copySource.Source = "allCopyDouble_p4_fixture75", "explicit isolated original2x copy-benefit fixture"
	copySource.Enabled, copySource.Frozen = true, nil
	if err := db.GetIns().Create(&copySource).Error; err != nil {
		t.Fatal(err)
	}
}

func localWelfareSourceFixture75(t *testing.T, kind string) {
	t.Helper()
	var source db.P4WelfareRule
	if err := db.GetIns().Where("kind=? AND id NOT LIKE ? AND id NOT LIKE ?", kind, "%lp-%", "%fixture75%").Order("start_at DESC,id DESC").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	source.ID = "p4:" + kind + ":fixture75"
	source.Enabled, source.Frozen = true, nil
	source.Source = "explicit isolated non-red local-play fixture; original welfare consumer"
	if err := db.GetIns().Where("id=?", source.ID).FirstOrCreate(&source).Error; err != nil {
		t.Fatal(err)
	}
}

func p3DomainOperationsFixture75(t *testing.T) {
	t.Helper()
	if err := db.GetIns().AutoMigrate(&db.ActivityRules{}, &db.ActivityReward{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	luxury := db.ActivityRules{ID: constUtil.P3LuxurySignActivityID, Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.AddDate(0, 1, 0).UnixMilli(), RewardEndAt: now.AddDate(0, 1, 0).UnixMilli(), Source: "explicit isolated six-yuan luxury-sign fixture"}
	if err := db.GetIns().Where("id=?", luxury.ID).FirstOrCreate(&luxury).Error; err != nil {
		t.Fatal(err)
	}
	luxuryReward := db.ActivityReward{ActivityID: luxury.ID, RewardType: constUtil.P3LuxurySignRewardType, Reward: "item_005,1"}
	if err := db.GetIns().Where("activity_id=? AND reward_type=? AND threshold=0", luxury.ID, luxuryReward.RewardType).FirstOrCreate(&luxuryReward).Error; err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"rechargeSingle", "rechargeTotal", "continuedRecharge", "costTotal"} {
		var cfg db.PaymentActivityRule
		if err := db.GetIns().First(&cfg, "id=?", prefix+"_local_p3").Error; err != nil {
			t.Fatal(err)
		}
		cfg.ID, cfg.Enabled = prefix+"_fixture75_local_p3", true
		cfg.Source += "; explicit isolated P3 activity fixture"
		if err := db.GetIns().Where("id=?", cfg.ID).FirstOrCreate(&cfg).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, product := range []db.VIPShopProduct{
		{ID: "fixture75_training", MinVIP: 1, Cost: 100, Reward: "item_007,20", Enabled: true, Source: "explicit isolated original VIP material fixture"},
		{ID: "fixture75_growth_material", MinVIP: 2, Cost: 300, Reward: "item_005,5", Enabled: true, Source: "explicit isolated original VIP material fixture"},
	} {
		if err := db.GetIns().Where("id=?", product.ID).FirstOrCreate(&product).Error; err != nil {
			t.Fatal(err)
		}
	}
	code := db.PaymentGiftCode{Hash: p3Hash([]byte("P3-FIXTURE75")), Reward: "item_023,1;item_119,1", ExpiresAt: time.Now().AddDate(0, 1, 0).UnixMilli(), Limit: 1, DemoOnly: true, Source: "explicit isolated single-use P3 code fixture"}
	if err := db.GetIns().Where("hash=?", code.Hash).FirstOrCreate(&code).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"local-p3-red-card", "local-p3-red-equipment", "p3vip_red_card"} {
		var enabled bool
		table := any(&db.PaymentProduct{})
		if id == "p3vip_red_card" {
			table = &db.VIPShopProduct{}
		}
		if err := db.GetIns().Model(table).Select("enabled").Where("id=?", id).Scan(&enabled).Error; err != nil || enabled {
			t.Fatal("isolated fixture opened original disabled direct red product", id, err)
		}
	}
}

// A finite two-player ladder is an admission prerequisite for title tests.
// No best-win rank, winner, title, battle receipt or reward fact is supplied.
func arenaTitleLadderFixture75(t *testing.T, f *p1Fixture, challenger int64, now time.Time) {
	t.Helper()
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		if err = f.s.p11SyncArenaMembers(tx, cfg, now); err != nil {
			return err
		}
		for i, id := range []int64{f.id, challenger} {
			if err = tx.Model(&db.P11ArenaMember{}).Where("player_id=? AND best_won_rank=0", id).Update("position", int64(i+1)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Red-capable original boxes are earned through a real confirmed native recharge
// activity in these tests; forbidden ordinary-shop red boxes remain closed.
func fundedRechargeRewardFixture75(t *testing.T, f *p1Fixture, key, reward string) {
	t.Helper()
	now := time.Now()
	activity := db.PeriodInstance{ID: "rechargeTotal_p4_" + key + "75", Enabled: true, Title: "隔离实际充值奖励", Kind: "payment_amount", Mode: "cumulative", Goal: 3000, Rewards: reward, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Source: "finite isolated actual-funded-payment activity fixture"}
	if err := db.GetIns().Create(&activity).Error; err != nil {
		t.Fatal(err)
	}
	p3DemoProviderFund(t, f, f.id, key+"75-funded", 3000)
	f.s.P3AutoConfirmDemoPayments = true
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0030")
	if _, err := f.s.generatePayment(f.ctx, f.id, request, 93001, now); err != nil {
		t.Fatal(err)
	}
	resources, err := p1RewardResources(reward)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(activity.ID), Index: proto.Int32(1), GetResources: resources}); err != nil {
		t.Fatal(err)
	}
}
