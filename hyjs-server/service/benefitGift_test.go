package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func benefitGiftInfo(t *testing.T, f *p1Fixture, now time.Time) *pb.SCBenefitGiftInfo {
	t.Helper()
	result, err := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftInfo{}, 0, now)
	if err != nil || len(result) != 1 {
		t.Fatal("native info", result, err)
	}
	return result[0].(*pb.SCBenefitGiftInfo)
}

func benefitGiftBalance(t *testing.T, f *p1Fixture, diamond int64) {
	t.Helper()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(diamond); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestBenefitGiftOriginalPageSharedQuotaConcurrentReceiptAndCold(t *testing.T) {
	f := p4Setup(t)
	at := time.Now()
	before, bg := f.profile(t)
	info := benefitGiftInfo(t, f, at)
	if info.GetTargetId() != "p4Flash_1" || info.GetBuyType() != "diamond" || info.GetPrice() != 20 || info.GetDiscountPrice() != 20 || info.GetRewardState() != 0 || !p1SameResources(info.Reward, []*pb.BaseResource{{Id: proto.String("item_013"), Num: proto.Int32(1)}}) {
		t.Fatal("not the existing frozen sale", info)
	}
	queried, qg := f.profile(t)
	if queried.GetDiamond() != before.GetDiamond() || simpleOwned(queried, qg, "item_013") != simpleOwned(before, bg, "item_013") {
		t.Fatal("query fabricated resources")
	}
	state, err := f.s.BenefitGiftOpenState(f.ctx, f.id, at)
	if err != nil || state.GetType() != "benefitGiftActivity" || state.GetIsOpen() != 1 {
		t.Fatal(state, err)
	}
	if _, err = f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 7001, at); err == nil {
		t.Fatal("unfunded purchase succeeded")
	}
	benefitGiftBalance(t, f, 20)
	var wg sync.WaitGroup
	replies := make(chan proto.Message, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 7001, at)
			if e != nil {
				failures <- e
				return
			}
			if len(out) != 1 {
				failures <- fmt.Errorf("extra native callback frame")
				return
			}
			replies <- out[0]
		}()
	}
	wg.Wait()
	close(replies)
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	var original proto.Message
	for reply := range replies {
		if original == nil {
			original = reply
		} else if !proto.Equal(original, reply) {
			t.Error("different same-stamp receipts")
		}
	}
	if original == nil {
		t.Fatal("no successful native response")
	}
	award := original.(*pb.SCBenefitGiftReward)
	if award.GetRewardState() != 2 || !p1SameResources(award.Cost, []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(20)}}) {
		t.Fatal(award)
	}
	p, g := f.profile(t)
	if p.GetDiamond() != 0 || simpleOwned(p, g, "item_013") != simpleOwned(before, bg, "item_013")+1 {
		t.Fatal("purchase not once")
	}
	if benefitGiftInfo(t, f, at).GetRewardState() != 2 {
		t.Fatal("native state not persisted")
	}
	state, err = f.s.BenefitGiftOpenState(f.ctx, f.id, at)
	if err != nil || state.GetIsOpen() != 0 {
		t.Fatal("sold native icon remains enabled", err)
	}
	key := "limitSale_p4_" + at.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	var stock db.P4CommerceStock
	if err = db.GetIns().First(&stock, "rule_id=? AND product_id=?", key, info.GetTargetId()).Error; err != nil || stock.Used != 1 {
		t.Fatal("failed charge or concurrent replay consumed shared stock", stock.Used, err)
	}
	request := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: award.Cost, GetResources: info.Reward}
	if _, err = f.s.p4Commerce(f.ctx, f.id, request, 7002, at); err == nil {
		t.Fatal("generic shop bypassed shared native quota")
	}
	f.restart(t)
	if err = f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 7001, at.AddDate(0, 1, 0))
	if err != nil || len(out) != 1 || !proto.Equal(out[0], original) {
		t.Fatal("cold cross-cycle receipt changed", out, err)
	}
	after, ag := f.profile(t)
	if after.GetDiamond() != p.GetDiamond() || simpleOwned(after, ag, "item_013") != simpleOwned(p, g, "item_013") {
		t.Fatal("cold replay spent again")
	}
}

func TestBenefitGiftNoOfferPolicyMissingQuoteAndGenericPurchase(t *testing.T) {
	f := p4Setup(t)
	at := time.Now()
	if _, err := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 8001, at); err == nil {
		t.Fatal("purchase without shown quote")
	}
	info := benefitGiftInfo(t, f, at)
	benefitGiftBalance(t, f, 40)
	key := "limitSale_p4_" + at.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: info.DiscountPrice}}, GetResources: info.Reward}, 8001, at); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 8002, at); err == nil {
		t.Fatal("native page bypassed generic quota")
	}
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if got := benefitGiftInfo(t, f, at); got.GetTargetId() != "" || got.GetRewardState() != 2 {
		t.Fatal("operator disabled offer exposed", got)
	}
	s := &GameService{}
	if got, err := s.BenefitGiftOpenState(f.ctx, f.id, at); err != nil || got.GetIsOpen() != 0 {
		t.Fatal("uninitialized fastpath", got, err)
	}
	if _, err := s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftInfo{}, 0, at); err == nil {
		t.Fatal("uninitialized endpoint queried tables")
	}
}

func TestBenefitGiftPrivateRefreshStaleQuoteAndOldReceipt(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	benefitGiftBalance(t, f, 60)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitSail", "a2000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	at := LocalPlayNow(f.ctx)
	info := benefitGiftInfo(t, f, at)
	if info.GetTargetId() == "" {
		t.Fatal("private offer absent")
	}
	first, err := f.s.benefitGiftAt(f.ctx, id, &pb.CSBenefitGiftReward{}, 9001, at)
	if err != nil {
		t.Fatal(err)
	}
	oldCycle := localPlayCycleID(t, f, id, "TimeLimitSail")
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "TimeLimitSail", "a2000000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	at = LocalPlayNow(f.ctx)
	if oldCycle == localPlayCycleID(t, f, id, "TimeLimitSail") {
		t.Fatal("fixture did not create a new private cycle")
	}
	if state, e := f.s.BenefitGiftOpenState(f.ctx, id, at); e != nil || state.GetIsOpen() != 1 {
		t.Fatal("new offer icon", state, e)
	}
	if _, err = f.s.benefitGiftAt(f.ctx, id, &pb.CSBenefitGiftReward{}, 9002, at); err == nil {
		t.Fatal("new cycle or HUD refresh accepted old page quote")
	}
	benefitGiftInfo(t, f, at)
	if _, err = f.s.benefitGiftAt(f.ctx, id, &pb.CSBenefitGiftReward{}, 9002, at); err != nil {
		t.Fatal(err)
	}
	old, err := f.s.benefitGiftAt(f.ctx, id, &pb.CSBenefitGiftReward{}, 9001, at)
	if err != nil || !proto.Equal(first[0], old[0]) {
		t.Fatal("refresh lost old original receipt", err)
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 20 {
		t.Fatal("private cycles spent wrong amount", p.GetDiamond())
	}
	var progress db.P4CommerceState
	if err = db.GetIns().First(&progress, "player_id=? AND rule_id=?", id, "limitSale_p4_"+oldCycle).Error; err != nil {
		t.Fatal(err)
	}
	var state p4CommerceProgress
	if json.Unmarshal(progress.Payload, &state) != nil || state.Total[info.GetTargetId()] != 1 {
		t.Fatal("old cycle state overwritten")
	}
	var count int64
	if err = db.GetIns().Model(&db.P4Receipt{}).Where("player_id=? AND domain=?", id, constUtil.BenefitGiftReceiptDomain).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("native receipts not persistent once", count, err)
	}
}
