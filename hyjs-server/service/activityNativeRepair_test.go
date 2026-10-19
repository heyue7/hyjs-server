package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestKingNativeEntrance(t *testing.T) {
	f, actor, form := adminKingFixture(t)
	definition := adminKingDefinition(t, f, actor, form)
	slot := reusePublish(t, f, actor, definition, 1, 1, "calendar")
	ctx := serverClockBindTest(t, f, f.id)
	state, err := f.s.kingRecruitOpenState(ctx, f.id, LocalPlayNow(ctx))
	if err != nil || state.GetType() != "heroRecuritKing" || state.GetIsOpen() != 1 {
		t.Fatal("published king native entrance", state, err)
	}
	if err = updateProfile(ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(19); return nil }); err != nil {
		t.Fatal(err)
	}
	state, err = f.s.kingRecruitOpenState(ctx, f.id, LocalPlayNow(ctx))
	if err != nil || state.GetIsOpen() != 0 {
		t.Fatal("original level20 gate bypassed", state, err)
	}
	p4Fund(t, f)
	current, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range current.Items {
		if current.Items[i].ID == slot.ID {
			current.Items[i].Enabled = false
		}
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: current.Version, Items: current.Items}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	ctx = serverClockBindTest(t, f, f.id)
	state, err = f.s.kingRecruitOpenState(ctx, f.id, LocalPlayNow(ctx))
	if err != nil || state.GetIsOpen() != 0 {
		t.Fatal("stopped king reopened after cold", state, err)
	}
}

func TestFirstNativeEmptySelection(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePayments(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "demo-native-first", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "原生首充", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	at := time.Now()
	req := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	out, err := f.s.generatePayment(f.ctx, f.id, req, 771, at)
	if err != nil {
		t.Fatal(err)
	}
	order := out[0].(*pb.SCGenerateOrder).GetOrderId()
	trusted := context.WithValue(f.ctx, constUtil.P3TestPaymentContextKey, true)
	if _, err = f.s.confirmTestPaymentAt(trusted, order, "native-first-paid", 600, constUtil.P3PaymentConfirmed, at); err != nil {
		t.Fatal(err)
	}
	info, err := f.s.paymentActivity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String("firstRecharge_local_p3")}, at)
	if err != nil {
		t.Fatal(err)
	}
	item := info[0].(*pb.SCActivityInfo).GetItemList()[0]
	if item.GetStatus() != 1 {
		t.Fatal("confirmed order not eligible", item)
	}
	bad := &pb.CSActivityReward{ActivityId: proto.String("firstRecharge_local_p3"), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(999999)}}}
	if _, err = f.s.paymentActivity(f.ctx, f.id, bad, at); err == nil {
		t.Fatal("forged nonempty first reward accepted")
	}
	native := &pb.CSActivityReward{ActivityId: proto.String("firstRecharge_local_p3"), Index: proto.Int32(1)}
	raw, _ := proto.Marshal(native)
	native.Reset()
	if err = proto.Unmarshal(raw, native); err != nil {
		t.Fatal(err)
	}
	out, err = f.s.paymentActivity(f.ctx, f.id, native, at)
	if err != nil {
		t.Fatal("original first native empty selection", err)
	}
	gain := out[0].(*pb.SCActivityReward).GetGet()
	if len(gain.GetEquips()) != 1 || gain.GetEquips()[0].GetEquipId() != "equip_013" {
		t.Fatal("original first reward changed", gain)
	}
	f.restart(t)
	if _, err = f.s.paymentActivity(f.ctx, f.id, native, at); err == nil {
		t.Fatal("first reward regranted after cold")
	}
}

func TestFundBenefitSequential(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.InitializeActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Represent one pre-upgrade legitimate purchase and a previously claimed original10-person tier.
	if err := db.GetIns().Create(&db.ActivityState{ActivityID: "open_server_fund", PlayerID: f.id, Bought: true, Claims: []byte(`{"everyoneBenefit:10":true}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.ActivityReward{}).Where("activity_id=? AND reward_type=?", "open_server_fund", "everyoneBenefit").Update("required_count", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundInfo{})
	if err != nil {
		t.Fatal(err)
	}
	list := out[0].(*pb.SCOpenServerFundInfo).GetEveryoneBenefitList()
	if len(list) != 12 {
		t.Fatal("original benefit catalogue lost", len(list))
	}
	for i, item := range list {
		if item.GetIndex() != int32(i+1) {
			t.Fatal("benefit count is not sequential", item)
		}
	}
	if list[0].GetStatus() != 2 || list[1].GetStatus() != 0 {
		t.Fatal("old claim or real count changed", list[0], list[1])
	}
	if _, err = f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundReward{RewardType: proto.String("everyoneBenefit"), Index: proto.Int32(1)}); err == nil {
		t.Fatal("old claim regranted")
	}
	account, err := LoginAccount(f.ctx, "benefit-second-buyer", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "第二位购买者", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, account.Id, func(p *pb.Player, _ *model.GameState) error {
		p.Vip = proto.Int32(2)
		p.Diamond = proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.openServerFund(f.ctx, account.Id, &pb.CSBuyOpenServerFund{}); err != nil {
		t.Fatal(err)
	}
	claim := &pb.CSOpenServerFundReward{RewardType: proto.String("everyoneBenefit"), Index: proto.Int32(2)}
	if _, err = f.s.openServerFund(f.ctx, f.id, claim); err != nil {
		t.Fatal("two actual purchases unlock second tier", err)
	}
	f.restart(t)
	if err = f.s.InitializeActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.openServerFund(f.ctx, f.id, claim); err == nil {
		t.Fatal("sequential tier regranted after cold")
	}
	var state db.ActivityState
	if err = db.GetIns().First(&state, "activity_id=? AND player_id=?", "open_server_fund", f.id).Error; err != nil {
		t.Fatal(err)
	}
	claims := map[string]bool{}
	if err = json.Unmarshal(state.Claims, &claims); err != nil {
		t.Fatal(err)
	}
	if !claims["everyoneBenefit:10"] || !claims["everyoneBenefit:30"] || claims["everyoneBenefit:2"] {
		t.Fatal("historical identity was rewritten", claims)
	}
}
