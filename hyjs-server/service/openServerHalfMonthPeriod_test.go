package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Actual native payment confirmations and scoped event counters must be usable
// throughout the second week; the next accepted period cannot reuse those facts.
func TestOpeningHalfMonthPeriodFactsPaymentsAndRepeat(t *testing.T) {
	f, actor := reuseFixture(t, true)
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities, f.s.InitializePayments} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	role, err := LoginAccount(f.ctx, "demo-half-period-"+p6Digest([]byte(t.Name()))[:16], "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, role.Id, "半月真实条件", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = role.Id
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	if err = db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", today.AddDate(0, 0, -13).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var def model.AdminActivityDefinitionDTO
	for _, row := range definitions {
		if row.ID == "original_opening_14" {
			def = row
		}
	}
	def.Form.RepeatPolicy = "repeat"
	saved, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, def.ID, model.AdminActivityDefinitionSaveRequest{Name: def.Name, Form: def.Form, DefaultDays: 2, Enabled: true, Version: def.Version})
	if err != nil {
		t.Fatal(err)
	}
	first := reusePublish(t, f, actor, *saved, 1, 2, "character-day")
	second := reusePublish(t, f, actor, *saved, 3, 2, "character-day")
	stamp := int64(96000)
	bound := serverClockBindTest(t, f, f.id)
	query := func(taskID int32) *pb.OpenServer7DayItem {
		t.Helper()
		var task db.OpenServerActivityTask
		if err := db.GetIns().First(&task, taskID).Error; err != nil {
			t.Fatal(err)
		}
		messages, err := f.s.openServerActivityAt(bound, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(task.OpenDay), Tap: proto.Int32(task.Tap)}, 0, LocalPlayNow(bound))
		if err != nil {
			t.Fatal("native second-week tab", taskID, err)
		}
		for _, item := range messages[0].(*pb.SCNewOpenSevenDayByTitle).GetItemList() {
			if item.GetItemId() == taskID {
				return item
			}
		}
		t.Fatal("task missing", taskID)
		return nil
	}
	pay := func(productID string) {
		t.Helper()
		var product db.PaymentProduct
		if err := db.GetIns().First(&product, "id=?", productID).Error; err != nil {
			t.Fatal(err)
		}
		stamp++
		real := time.Now()
		messages, err := f.s.generatePayment(bound, f.id, &pb.CSGenerateOrder{PaymentId: proto.String(productID), Money: proto.Float64(float64(product.Cents) / 100), MoneyType: proto.String("RMB"), ChannelId: proto.String("gf_default"), ShopId: proto.String("common")}, stamp, real)
		if err != nil {
			t.Fatal("native order", err)
		}
		trusted := context.WithValue(bound, constUtil.P3TestPaymentContextKey, true)
		if _, err = f.s.confirmTestPaymentAt(trusted, messages[0].(*pb.SCGenerateOrder).GetOrderId(), fmt.Sprintf("half-payment-%d", stamp), product.Cents, constUtil.P3PaymentConfirmed, real); err != nil {
			t.Fatal("actual confirmation", err)
		}
	}
	event := func(prefix string, n int32) {
		t.Helper()
		if err := recordOpenServerCounterAt(db.GetIns().WithContext(bound), f.id, prefix+p1Day(LocalPlayNow(bound)), n, LocalPlayNow(bound)); err != nil {
			t.Fatal(err)
		}
	}
	// This explicitly named 15-yuan isolated product exercises single-order
	// semantics; no production product or reward is added by this fixture.
	var small db.PaymentProduct
	if err = db.GetIns().First(&small, "id=?", "djsy.wuyou.andp0030").Error; err != nil {
		t.Fatal(err)
	}
	small.ID = "fixture-half-15"
	small.Cents = 1500
	small.Charge = 150
	small.Free = 0
	small.FirstDouble = false
	if err = db.GetIns().Create(&small).Error; err != nil {
		t.Fatal(err)
	}
	pay(small.ID)
	pay(small.ID)
	if got := query(153); got.GetCurrentNum() != 15 || got.GetState() != 0 {
		t.Fatal("two fifteen-yuan orders became one thirty-yuan order", got)
	}
	event(constUtil.OpenServerActivityCounterLogin, 1)
	// A late joiner already has realm day seven unlocked; the original choice
	// request uses a real period login, without waiting for role day seven.
	if got := query(118); got.GetState() != 1 {
		t.Fatal("late day-seven character choice", got)
	}
	choice118 := &pb.CSGetOpenServerReward{ItemId: proto.Int32(118), HeroId: proto.String("hero_010")}
	choiceReceipt, err := f.s.openServerActivityAt(bound, f.id, choice118, 97000, LocalPlayNow(bound))
	if err != nil || len(choiceReceipt[0].(*pb.SCGetOpenServerReward).GetGet().GetWars()) != 1 || choiceReceipt[0].(*pb.SCGetOpenServerReward).GetGet().GetWars()[0].GetId() != "hero_010" {
		t.Fatal("native day-seven selected original hero", err)
	}
	if _, err = f.s.openServerActivityAt(bound, f.id, choice118, 97999, LocalPlayNow(bound)); err == nil {
		t.Fatal("day-seven choice claim limit reset")
	}
	event(constUtil.OpenServerActivityCounterCopyWin, 3)
	event(constUtil.OpenServerActivityCounterPredatory, 3)
	pay("djsy.wuyou.andp0030")
	// Simulate an already paid pre-upgrade role with no payment projection or
	// achievement; the immutable orders, not a new future payment, must repair it.
	if err = db.GetIns().Where("player_id=? AND occurrence_id=? AND counter_key LIKE ?", f.id, first.ID, constUtil.P3OpenServerSinglePaid+"%").Delete(&db.AdminOpeningCounter{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Where("player_id=? AND occurrence_id=? AND task_id=?", f.id, first.ID, 153).Delete(&db.AdminOpeningState{}).Error; err != nil {
		t.Fatal(err)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	event(constUtil.OpenServerActivityCounterCopyWin, 2)
	event(constUtil.OpenServerActivityCounterPredatory, 2)
	for _, id := range []int32{137, 153, 169, 139} {
		if got := query(id); got.GetState() != 1 {
			t.Fatal("period facts failed on a different date", id, got)
		}
	}
	// A first-week login remains attached to its original date.
	if got := query(21); got.GetState() == 1 {
		t.Fatal("first-week date policy expanded unintentionally", got)
	}
	claim := &pb.CSGetOpenServerReward{ItemId: proto.Int32(153)}
	firstReceipt, err := f.s.openServerActivityAt(bound, f.id, claim, 97001, LocalPlayNow(bound))
	if err != nil {
		t.Fatal("thirty-yuan original claim", err)
	}
	if _, err = f.s.openServerActivityAt(bound, f.id, claim, 97002, LocalPlayNow(bound)); err == nil {
		t.Fatal("claim limit reset")
	}
	loginChoice := &pb.CSGetOpenServerReward{ItemId: proto.Int32(137), HeroId: proto.String("frag_hero_001")}
	if _, err = f.s.openServerActivityAt(bound, f.id, loginChoice, 97003, LocalPlayNow(bound)); err != nil {
		t.Fatal("native login fragment choice", err)
	}
	f.restart(t)
	bound = serverClockBindTest(t, f, f.id)
	choiceReplay, err := f.s.openServerActivityAt(bound, f.id, choice118, 97000, LocalPlayNow(bound))
	if err != nil || !proto.Equal(choiceReceipt[0], choiceReplay[0]) {
		t.Fatal("day-seven choice cold receipt", err)
	}
	replay, err := f.s.openServerActivityAt(bound, f.id, claim, 97001, LocalPlayNow(bound))
	if err != nil || !proto.Equal(firstReceipt[0], replay[0]) {
		t.Fatal("cold original receipt", err)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if got := query(153); got.GetCurrentNum() != 0 || got.GetState() != 0 {
		t.Fatal("previous period payment leaked", got)
	}
	if got := query(139); got.GetCurrentNum() != 0 || got.GetState() != 0 {
		t.Fatal("previous period event leaked", got)
	}
	pay("djsy.wuyou.andp0030")
	if got := query(153); got.GetState() != 1 {
		t.Fatal("new period actual payment ignored", got)
	}
	if _, err = f.s.openServerActivityAt(bound, f.id, claim, 97004, LocalPlayNow(bound)); err != nil {
		t.Fatal("new period claim", err)
	}
	replay, err = f.s.openServerActivityAt(bound, f.id, claim, 97001, LocalPlayNow(bound))
	if err != nil || !proto.Equal(firstReceipt[0], replay[0]) {
		t.Fatal("new period rewrote old receipt", err)
	}
	var instances []db.AdminCharacterActivityInstance
	if err = db.GetIns().Where("player_id=? AND occurrence_id IN ?", f.id, []string{first.ID, second.ID}).Find(&instances).Error; err != nil || len(instances) != 2 {
		t.Fatal("repeat accepted identity", len(instances), err)
	}
	// Complete the second two-day window. New requests after TTL stay closed.
	reuseClockDay(t, f, actor)
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if _, err = f.s.openServerActivityAt(bound, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(9), Tap: proto.Int32(1)}, 0, LocalPlayNow(bound)); err == nil {
		t.Fatal("expired period admitted fresh completion")
	}
}
