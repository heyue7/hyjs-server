package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

// TestP3PaymentDomain 可复用严格隔离MySQL；不会连接正式hyjs库或调用手机。
func TestP3PaymentDomain(t *testing.T) {
	ctx := context.Background()
	name := os.Getenv("HYJS_P3_MYSQL_DATABASE")
	path := filepath.Join(t.TempDir(), "p3.db")
	open := func() {
		t.Helper()
		var err error
		if name != "" {
			if !strings.HasPrefix(name, "hyjs_test_p3_") {
				t.Fatal("P3 database scope rejected")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name})
		} else {
			err = db.InitGame(ctx, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	initialize := func() {
		t.Helper()
		for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializePayments, s.InitializeMonthSign, s.InitializeGameplayPools, s.InitializeP2} {
			if err := init(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	initialize()
	protected, err := LoginAccount(ctx, "p3-protected-original", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, protected.Id, "保护角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	fresh := func(label string) int64 {
		t.Helper()
		row, e := LoginAccount(ctx, "demo-p3-"+label, "test-password")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = CreatePlayer(ctx, row.Id, "支付测试", "hero_043"); e != nil {
			t.Fatal(e)
		}
		return row.Id
	}
	profile := func(id int64) (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if e := db.GetIns().First(&row, id).Error; e != nil {
			t.Fatal(e)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		return p, g
	}
	stamp := int64(0)
	generate := func(id int64, product string, at time.Time) *db.PaymentOrder {
		t.Helper()
		var p db.PaymentProduct
		if e := db.GetIns().First(&p, "id = ?", product).Error; e != nil {
			t.Fatal(e)
		}
		stamp++
		req := &pb.CSGenerateOrder{PaymentId: proto.String(product), Money: proto.Float64(float64(p.Cents) / 100), MoneyType: proto.String("RMB"), ChannelId: proto.String("gf_default"), ShopId: proto.String("common")}
		messages, e := s.generatePayment(ctx, id, req, stamp, at)
		if e != nil {
			t.Fatal(e)
		}
		var o db.PaymentOrder
		if e = db.GetIns().First(&o, "id = ?", messages[0].(*pb.SCGenerateOrder).GetOrderId()).Error; e != nil {
			t.Fatal(e)
		}
		return &o
	}
	confirm := func(o *db.PaymentOrder, event string, at time.Time) *db.PaymentOrder {
		t.Helper()
		trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
		got, e := s.confirmTestPaymentAt(trusted, o.ID, event, o.Cents, constUtil.P3PaymentConfirmed, at)
		if e != nil {
			t.Fatal(e)
		}
		return got
	}
	call := func(id int64, req proto.Message) []proto.Message {
		t.Helper()
		raw, e := proto.Marshal(req)
		if e != nil {
			t.Fatal(e)
		}
		clone := req.ProtoReflect().Type().New().Interface()
		if e = proto.Unmarshal(raw, clone); e != nil {
			t.Fatal(e)
		}
		stamp++
		messages, handled, e := s.HandlePayment(ctx, id, clone, stamp)
		if e != nil || !handled {
			t.Fatalf("P3 %T handled=%v err=%v", req, handled, e)
		}
		return messages
	}
	now := time.Now()

	t.Run("native_AND0003_directory_and_generate_use_same_explicit_mapping", func(t *testing.T) {
		id := fresh("native-channel")
		catalog := func(channel string) *pb.SCGetPaymentInfo {
			return call(id, &pb.CSGetPaymentInfo{ChannelId: proto.String(channel), ShopId: proto.String("common")})[0].(*pb.SCGetPaymentInfo)
		}
		native, original := catalog("AND0003"), catalog("gf_default")
		if len(native.GetPaymentInfoList()) == 0 || !proto.Equal(native, original) {
			t.Fatal("native catalog mapping missing or different")
		}
		if len(catalog("AND0004").GetPaymentInfoList()) != 0 {
			t.Fatal("unknown native channel accepted")
		}
		for _, productID := range []string{"djsy.wuyou.andp0006", "local-p3-red-card", "local-p3-red-equipment"} {
			var product db.PaymentProduct
			if e := db.GetIns().First(&product, "id = ?", productID).Error; e != nil {
				t.Fatal(e)
			}
			if product.Channels != "gf_default" || !strings.Contains(product.Source, "AND0003") {
				t.Fatal("channel source was overwritten or compatibility undocumented")
			}
			req := &pb.CSGenerateOrder{PaymentId: proto.String(productID), Money: proto.Float64(float64(product.Cents) / 100), MoneyType: proto.String("RMB"), ChannelId: proto.String("AND0003"), ShopId: proto.String("common")}
			generated := call(id, req)[0].(*pb.SCGenerateOrder)
			if generated.GetOrderId() == "" || generated.GetTransactionData().GetPayStatus() != "PAY_STATUS_WAIT" {
				t.Fatal("native channel did not create pending order")
			}
			req.ChannelId = proto.String("AND0004")
			stamp++
			_, handled, e := s.HandlePayment(ctx, id, req, stamp)
			if !handled || e == nil {
				t.Fatal("unknown channel generated order")
			}
		}
		p, _ := profile(id)
		if p.GetDiamond() != 0 || p.GetChargeCount() != 0 {
			t.Fatal("native order generation credited resources")
		}
	})

	t.Run("generate_idempotency_and_first_double_are_separate_from_vip_charge", func(t *testing.T) {
		id := fresh("generate-double")
		if e := db.GetIns().Model(&db.PaymentProduct{}).Where("id = ?", "djsy.wuyou.andp0006").Update("first_double", true).Error; e != nil {
			t.Fatal(e)
		}
		stamp++
		req := &pb.CSGenerateOrder{PaymentId: proto.String("djsy.wuyou.andp0006"), Money: proto.Float64(6), MoneyType: proto.String("RMB"), ChannelId: proto.String("gf_default"), ShopId: proto.String("common")}
		first, e := s.generatePayment(ctx, id, req, stamp, now)
		if e != nil {
			t.Fatal(e)
		}
		replay, e := s.generatePayment(ctx, id, req, stamp, now)
		if e != nil || !proto.Equal(first[0], replay[0]) {
			t.Fatal("generation replay", e)
		}
		req.Money = proto.Float64(30)
		if _, e = s.generatePayment(ctx, id, req, stamp, now); e == nil {
			t.Fatal("request key payload changed")
		}
		var firstOrder db.PaymentOrder
		db.GetIns().First(&firstOrder, "id = ?", first[0].(*pb.SCGenerateOrder).GetOrderId())
		confirm(&firstOrder, "double-first", now)
		second := generate(id, "djsy.wuyou.andp0006", now)
		confirm(second, "double-second", now)
		p, _ := profile(id)
		if p.GetDiamond() != 192 || p.GetChargeDiamond() != 120 || p.GetChargeCount() != 2 {
			t.Fatal("double altered base charge", p)
		}
		if e := db.GetIns().Model(&db.PaymentProduct{}).Where("id = ?", "djsy.wuyou.andp0006").Update("first_double", false).Error; e != nil {
			t.Fatal(e)
		}
	})

	t.Run("client_claimed_success_is_only_order_query", func(t *testing.T) {
		id := fresh("client-forgery")
		order := generate(id, "djsy.wuyou.andp0006", now)
		result := call(id, &pb.CSCheckOrder{OrderId: proto.String(order.ID), PaymentId: proto.String(order.ProductID), PayInfo: &pb.SuperSDKPayInfo{PayStatus: proto.String("success"), Sign: proto.String("forged")}})[0].(*pb.SCCheckOrder)
		p, _ := profile(id)
		if p.GetDiamond() != 0 || p.GetChargeDiamond() != 0 || result.GetTransactionData().GetPayStatus() != "PAY_STATUS_WAIT" {
			t.Fatal("client assertion granted payment")
		}
		if _, e := s.ConfirmTestPayment(ctx, order.ID, "forged", 600, constUtil.P3PaymentConfirmed); e == nil {
			t.Fatal("missing trusted context accepted")
		}
	})
	t.Run("snapshot_parallel_confirmation_event_ownership_and_restart", func(t *testing.T) {
		id := fresh("parallel")
		order := generate(id, "djsy.wuyou.andp0030", now)
		if e := db.GetIns().Model(&db.PaymentProduct{}).Where("id = ?", order.ProductID).Updates(map[string]any{"charge": 9999, "free": 9999, "cents": 12345}).Error; e != nil {
			t.Fatal(e)
		}
		trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := s.confirmTestPaymentAt(trusted, order.ID, "same-event", 3000, constUtil.P3PaymentConfirmed, now)
				errs <- e
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		p, _ := profile(id)
		if p.GetDiamond() != 330 || p.GetChargeDiamond() != 300 || p.GetChargeCount() != 1 || p.GetVip() != 2 || p.GetWarriorBagSize() != 105 || p.GetEquipBagSize() != 105 {
			t.Fatal("frozen credit/VIP/capacity", p)
		}
		var facts int64
		db.GetIns().Model(&db.EventFact{}).Where("player_id = ?", id).Count(&facts)
		if facts != 2 {
			t.Fatal("payment events replayed", facts)
		}
		bad := generate(fresh("other-event"), "djsy.wuyou.andp0006", now)
		if _, e := s.confirmTestPaymentAt(trusted, bad.ID, "same-event", 600, constUtil.P3PaymentConfirmed, now); e == nil {
			t.Fatal("event reused across orders")
		}
		if _, e := s.confirmTestPaymentAt(trusted, order.ID, "same-event", 2999, constUtil.P3PaymentConfirmed, now); e == nil {
			t.Fatal("event amount changed")
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		initialize()
		confirm(order, "same-event", now)
		p, _ = profile(id)
		if p.GetDiamond() != 330 || p.GetChargeCount() != 1 {
			t.Fatal("restart duplicated credit")
		}
		if e := db.GetIns().Model(&db.PaymentProduct{}).Where("id = ?", order.ProductID).Updates(map[string]any{"charge": 300, "free": 30, "cents": 3000}).Error; e != nil {
			t.Fatal(e)
		}
	})
	t.Run("failed_expired_protected_and_resource_overflow_are_atomic", func(t *testing.T) {
		trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
		id := fresh("failures")
		failed := generate(id, "djsy.wuyou.andp0006", now)
		if _, e := s.confirmTestPaymentAt(trusted, failed.ID, "failure", 600, constUtil.P3PaymentFailed, now); e != nil {
			t.Fatal(e)
		}
		if _, e := s.confirmTestPaymentAt(trusted, failed.ID, "late-success", 600, constUtil.P3PaymentConfirmed, now); e == nil {
			t.Fatal("failure resurrected")
		}
		expired := generate(id, "djsy.wuyou.andp0006", now.Add(-time.Hour))
		if _, e := s.confirmTestPaymentAt(trusted, expired.ID, "expired", 600, constUtil.P3PaymentConfirmed, now); e == nil {
			t.Fatal("expired confirmed")
		}
		original := generate(protected.Id, "djsy.wuyou.andp0006", now)
		if _, e := s.confirmTestPaymentAt(trusted, original.ID, "protected", 600, constUtil.P3PaymentConfirmed, now); e == nil {
			t.Fatal("original charged by test adapter")
		}
		overflow := generate(id, "djsy.wuyou.andp0006", now)
		if e := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(math.MaxInt64); return nil }); e != nil {
			t.Fatal(e)
		}
		if _, e := s.confirmTestPaymentAt(trusted, overflow.ID, "overflow", 600, constUtil.P3PaymentConfirmed, now); e == nil {
			t.Fatal("overflow confirmed")
		}
		var persisted db.PaymentOrder
		db.GetIns().First(&persisted, "id = ?", overflow.ID)
		var events int64
		db.GetIns().Model(&db.PaymentConfirmation{}).Where("event_id = ?", "overflow").Count(&events)
		p, _ := profile(id)
		if persisted.Status != constUtil.P3PaymentPending || events != 0 || p.GetChargeCount() != 0 {
			t.Fatal("partial overflow mutation")
		}
	})
	t.Run("month_permanent_daily_and_renewal_boundary", func(t *testing.T) {
		id := fresh("cards")
		month := generate(id, "djsy.wuyou.month", now)
		confirm(month, "month", now)
		permanent := generate(id, "djsy.wuyou.permanent", now)
		confirm(permanent, "permanent", now)
		p, _ := profile(id)
		if p.GetDiamond() != 1230 || p.GetMonthCardEndTime() != now.UnixMilli()+constUtil.P3MonthCardDurationMS || p.GetPermanentCardValid() != 1 {
			t.Fatal("card initial credit", p)
		}
		claim := func(kind string, at time.Time) error {
			_, e := s.membership(ctx, id, &pb.CSClaimCard{CardType: proto.String(kind)}, at)
			return e
		}
		if e := claim("month", now); e != nil {
			t.Fatal(e)
		}
		if e := claim("permanent", now); e != nil {
			t.Fatal(e)
		}
		if e := claim("month", now); e == nil {
			t.Fatal("daily duplicated")
		}
		if e := claim("month", now.Add(30*24*time.Hour)); e == nil {
			t.Fatal("expired month paid")
		}
		if e := claim("permanent", now.Add(31*24*time.Hour)); e != nil {
			t.Fatal(e)
		}
		p, _ = profile(id)
		if p.GetDiamond() != 1530 {
			t.Fatal("daily amounts", p.GetDiamond())
		}
	})
	t.Run("first_single_total_claims_frozen_and_selection_validated", func(t *testing.T) {
		id := fresh("activities")
		for i := 0; i < 2; i++ {
			o := generate(id, "djsy.wuyou.andp0030", now)
			confirm(o, fmt.Sprintf("activity-%d", i), now)
		}
		for _, activity := range []string{"firstRecharge_local_p3", "rechargeSingle_local_p3", "rechargeTotal_local_p3"} {
			r := call(id, &pb.CSActivityInfo{ActivityId: proto.String(activity)})[0].(*pb.SCActivityInfo)
			if r.ItemList[0].GetStatus() != 1 {
				t.Fatal("paid condition not met", activity)
			}
			if e := db.GetIns().Model(&db.PaymentActivityRule{}).Where("id = ?", activity).Update("reward", "diamond,99999").Error; e != nil {
				t.Fatal(e)
			}
			req := &pb.CSActivityReward{ActivityId: proto.String(activity), Index: proto.Int32(1), GetResources: r.ItemList[0].RewardList}
			result := call(id, req)[0].(*pb.SCActivityReward)
			if activity == "firstRecharge_local_p3" && (len(result.GetGet().GetEquips()) != 1 || result.GetGet().Equips[0].GetEquipId() != "equip_013") {
				t.Fatal("first reward not original")
			}
			if _, _, e := s.HandlePayment(ctx, id, req, 1); e == nil {
				t.Fatal("repeat claimed")
			}
		}
		p, g := profile(id)
		if p.GetCoin() != 188888 || p.GetDiamond() != 690 || len(g.Equipment) != 1 {
			t.Fatal("frozen awards", p)
		}
	})
	t.Run("continuous_distinct_days_and_final_reward", func(t *testing.T) {
		id := fresh("continued")
		for i := 0; i < 3; i++ {
			at := now.Add(time.Duration(i-2) * 24 * time.Hour)
			order := generate(id, "djsy.wuyou.andp0006", at)
			confirm(order, fmt.Sprintf("continued-%d", i), at)
		}
		r := call(id, &pb.CSContinuedRechargeInfo{ActivityId: proto.String("continuedRecharge_local_p3")})[0].(*pb.SCContinuedRechargeInfo)
		if r.GetStatus() != 1 || len(r.ItemList) != 3 {
			t.Fatal("continued count")
		}
		for _, item := range r.ItemList {
			call(id, &pb.CSContinuedRechargeReward{ActivityId: r.ActivityId, Index: item.Index, Type: proto.Int32(0), GetResources: item.RewardList})
		}
		call(id, &pb.CSContinuedRechargeReward{ActivityId: r.ActivityId, Index: proto.Int32(-1), Type: proto.Int32(1), GetResources: r.BigReward})
		_, g := profile(id)
		found := false
		for _, item := range g.Items {
			found = found || (item.GetId() == "item_119" && item.GetNum() == 1)
		}
		if !found {
			t.Fatal("final red box absent")
		}
	})
	t.Run("vip_week_consumption_luxury_sign_and_red_box_gift", func(t *testing.T) {
		id := fresh("welfare")
		order := generate(id, "local-p3-red-card", now)
		confirm(order, "red-paid", now)
		r := call(id, &pb.CSGetVipBoonInfo{})[0].(*pb.SCGetVipBoonInfo)
		if r.Daily[0].GetStatus() != 1 || r.Week[0].GetStatus() != 1 {
			t.Fatal("VIP unavailable")
		}
		call(id, &pb.CSGetVipDailyBoonReward{})
		call(id, &pb.CSGetVipWeekBoonReward{Index: proto.Int32(1), CostResources: r.Week[0].CostResource, GetResources: r.Week[0].GetResource})
		if _, e := s.monthSignAt(ctx, id, &pb.CSReceiveLuxurySign{}, now); e != nil {
			t.Fatal(e)
		}
		if _, e := s.monthSignAt(ctx, id, &pb.CSReceiveLuxurySign{}, now); e == nil {
			t.Fatal("luxury duplicate")
		}
		call(id, &pb.CSConvertGiftCode{GiftCode: proto.String("HYJS-DEMO-P3")})
		if _, _, e := s.HandlePayment(ctx, id, &pb.CSConvertGiftCode{GiftCode: proto.String("HYJS-DEMO-P3")}, 1); e == nil {
			t.Fatal("gift duplicate")
		}
		p, g := profile(id)
		if p.GetDiamond() != 300 || p.GetCoin() != 3000 {
			t.Fatal("welfare funds", p)
		}
		boxes := map[string]int32{}
		for _, item := range g.Items {
			boxes[item.GetId()] = item.GetNum()
		}
		if boxes["item_023"] != 2 || boxes["item_119"] != 1 {
			t.Fatal("red test box chain", boxes)
		}
		// 已确认测试支付给原选择箱，再走P2原CSItemUse产生实例；没有直接写实例。
		use := func(key string, index int32, requestStamp int64) *pb.SCItemUse {
			t.Helper()
			messages, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(index)}, requestStamp)
			if e != nil {
				t.Fatal(e)
			}
			return messages[0].(*pb.SCItemUse)
		}
		if got := use("item_023", 5, 10001); got.GetGet().GetSimps()[0].GetId() != "item_053" {
			t.Fatal("original red selection category")
		}
		if got := use("item_053", 1, 10002); len(got.GetGet().GetWars()) != 1 || got.GetGet().GetWars()[0].GetId() != "hero_001" {
			t.Fatal("paid red hero not created")
		}
		if got := use("item_119", 1, 10003); len(got.GetGet().GetEquips()) != 1 || got.GetGet().GetEquips()[0].GetEquipId() != "equip_037" {
			t.Fatal("paid red equipment not created")
		}
	})
	t.Run("luxury_offer_requires_actual_chapter_and_window", func(t *testing.T) {
		id := fresh("luxury-offer")
		req := &pb.CSOpenLuxuryEquipment{Chapter: proto.Int32(6), Mission: proto.Int32(1)}
		if _, _, e := s.HandlePayment(ctx, id, req, 1); e == nil {
			t.Fatal("unpassed chapter opened")
		}
		// 仅隔离fixture模拟未来章节已完成，不能算手机原6-1已打通。
		if e := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
			g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String("6"), MissionId: proto.String("30"), Star: proto.Int32(5)})
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		call(id, req)
		info := call(id, &pb.CSLuxuryEquipmentInfo{})[0].(*pb.SCLuxuryEquipmentInfo)
		if info.GetId() != 2 || info.GetEndTime()-info.GetCurrentTime() > 12*3600*1000 {
			t.Fatal("original offer time")
		}
		order := generate(id, "djsy.wuyou.andp0288", time.Now())
		confirm(order, "luxury-paid", time.Now())
		call(id, &pb.CSLuxuryEquipmentReward{})
		_, g := profile(id)
		if len(g.Equipment) != 4 {
			t.Fatal("original suit not delivered")
		}
	})
	t.Run("vip_shop_day_quote_and_actual_consumption_activity", func(t *testing.T) {
		id := fresh("vip-shop")
		order := generate(id, "djsy.wuyou.andp0128", now)
		confirm(order, "vip-shop-paid", now)
		items := call(id, &pb.CSShopProductList{ShopType: proto.Int32(2)})[0].(*pb.SCShopProductList).GetShopItemList()
		if len(items) != 2 {
			t.Fatal("VIP catalogue", items)
		}
		for _, item := range items {
			// 按原 SetShopVipItem 的索引方式验证完整回包，避免残留未挂载的商品卡。
			if len(item.GetItem()) == 0 || len(item.GetPrice()) != 1 || len(item.GetDiscount()) != 1 || !proto.Equal(item.Price[0], item.Discount[0]) || !proto.Equal(item.Price[0], item.ExchangeList[0]) {
				t.Fatal("native VIP card requires original and discounted price", item.GetId())
			}
		}
		if e := db.GetIns().Model(&db.VIPShopProduct{}).Where("id = ?", "p3vip_training").Update("cost", 9999).Error; e != nil {
			t.Fatal(e)
		}
		call(id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String("p3vip_training"), BuyCount: proto.Int32(1)})
		p, _ := profile(id)
		if p.GetDiamond() != 1325 {
			t.Fatal("day quote changed", p.GetDiamond())
		}
		if _, _, e := s.HandlePayment(ctx, id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String("p3vip_training"), BuyCount: proto.Int32(1)}, 1); e == nil {
			t.Fatal("daily shop quota replay")
		}
		call(id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String("p3vip_red_card"), BuyCount: proto.Int32(1)})
		info := call(id, &pb.CSActivityInfo{ActivityId: proto.String("costTotal_local_p3")})[0].(*pb.SCActivityInfo)
		if info.ItemList[0].GetStatus() != 1 {
			t.Fatal("confirmed consumption not reached")
		}
		call(id, &pb.CSActivityReward{ActivityId: info.ActivityId, Index: proto.Int32(1), GetResources: info.ItemList[0].RewardList})
		p, _ = profile(id)
		if p.GetDiamond() != 1055 {
			t.Fatal("rebate altered paid metrics", p.GetDiamond())
		}
		var spent []db.EventFact
		db.GetIns().Where("player_id = ? AND kind = ?", id, constUtil.P1EventDiamondSpent).Find(&spent)
		total := int64(0)
		for _, fact := range spent {
			total += fact.Amount
		}
		if total != 400 {
			t.Fatal("consumption duplicated", total)
		}
	})
	t.Run("paid_vip_unblocks_month_sign_difference_and_original_fund", func(t *testing.T) {
		id := fresh("vip-month-difference")
		if _, e := s.monthSignAt(ctx, id, &pb.CSReceiveMonthSign{}, now); e != nil {
			t.Fatal(e)
		}
		order := generate(id, "djsy.wuyou.andp0030", now)
		confirm(order, "month-difference-pay", now)
		if _, e := s.monthSignAt(ctx, id, &pb.CSReceiveMonthSign{}, now); e != nil {
			t.Fatal(e)
		}
		if _, e := s.monthSignAt(ctx, id, &pb.CSReceiveMonthSign{}, now); e == nil {
			t.Fatal("VIP supplemental claim repeated")
		}
		p, _ := profile(id)
		if p.GetCoin() != 2000 || p.GetVip() != 2 {
			t.Fatal("paid VIP same-day difference", p)
		}
		if e := s.InitializeActivities(ctx); e != nil {
			t.Fatal(e)
		}
		fundID := fresh("paid-fund")
		if _, e := s.openServerFund(ctx, fundID, &pb.CSBuyOpenServerFund{}); e == nil {
			t.Fatal("VIP0 bought fund")
		}
		fundOrder := generate(fundID, "djsy.wuyou.andp0128", now)
		confirm(fundOrder, "fund-pay", now)
		if _, e := s.openServerFund(ctx, fundID, &pb.CSBuyOpenServerFund{}); e != nil {
			t.Fatal(e)
		}
		p, _ = profile(fundID)
		if p.GetDiamond() != 425 || p.GetVip() != 3 {
			t.Fatal("actual fund qualification", p)
		}
	})
	t.Run("gift_shared_stock_parallel_players_and_card_claim_atomicity", func(t *testing.T) {
		code := db.PaymentGiftCode{Hash: p3Hash([]byte("P3-STOCK-ONE")), Reward: "item_119,1", ExpiresAt: now.Add(time.Hour).UnixMilli(), Limit: 1, DemoOnly: true, Source: "isolated P3 finite stock regression"}
		if e := db.GetIns().Create(&code).Error; e != nil {
			t.Fatal(e)
		}
		ids := []int64{fresh("stock-a"), fresh("stock-b")}
		var wg sync.WaitGroup
		results := make(chan bool, 2)
		for _, id := range ids {
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				_, e := s.paymentOffer(ctx, id, &pb.CSConvertGiftCode{GiftCode: proto.String("P3-STOCK-ONE")}, now)
				results <- e == nil
			}(id)
		}
		wg.Wait()
		close(results)
		success := 0
		for ok := range results {
			if ok {
				success++
			}
		}
		if success != 1 {
			t.Fatal("shared stock over-issued", success)
		}
		var persisted db.PaymentGiftCode
		db.GetIns().First(&persisted, "hash = ?", code.Hash)
		if persisted.Used != 1 {
			t.Fatal("shared stock receipt", persisted.Used)
		}
		cardID := fresh("parallel-card")
		order := generate(cardID, "djsy.wuyou.month", now)
		confirm(order, "parallel-card-pay", now)
		claims := make(chan bool, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := s.membership(ctx, cardID, &pb.CSClaimCard{CardType: proto.String("month")}, now)
				claims <- e == nil
			}()
		}
		wg.Wait()
		close(claims)
		success = 0
		for ok := range claims {
			if ok {
				success++
			}
		}
		p, _ := profile(cardID)
		if success != 1 || p.GetDiamond() != 350 {
			t.Fatal("card day concurrency", success, p.GetDiamond())
		}
	})
	t.Run("original_14day_money_conditions_derive_only_from_confirmations", func(t *testing.T) {
		if e := s.InitializeOpenServerActivity(ctx); e != nil {
			t.Fatal(e)
		}
		local := now.In(time.FixedZone("UTC+8", 8*3600))
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		start := today.AddDate(0, 0, -8)
		if e := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id = ?", constUtil.OpenServerActivityID).Updates(map[string]any{"start_at": start.UnixMilli(), "end_at": start.AddDate(0, 0, 14).UnixMilli(), "reward_end_at": start.AddDate(0, 0, 15).UnixMilli()}).Error; e != nil {
			t.Fatal(e)
		}
		id := fresh("14day-paid")
		order := generate(id, "djsy.wuyou.andp0128", now)
		var count int64
		db.GetIns().Model(&db.OpenServerActivityCounter{}).Where("player_id = ? AND counter_key = ?", id, constUtil.P3OpenServerTotalPaid).Count(&count)
		if count != 0 {
			t.Fatal("pending order counted")
		}
		confirm(order, "14day-paid", now)
		var total, single db.OpenServerActivityCounter
		db.GetIns().First(&total, "player_id = ? AND counter_key = ?", id, constUtil.P3OpenServerTotalPaid)
		db.GetIns().First(&single, "player_id = ? AND counter_key = ?", id, constUtil.P3OpenServerSinglePaid+p3Day(now))
		if total.Value != 128 || single.Value != 128 {
			t.Fatal("payment metrics confused with diamonds", total, single)
		}
		var cfg db.OpenServerActivityRule
		db.GetIns().First(&cfg, "id = ?", constUtil.OpenServerActivityID)
		p, g := profile(id)
		values := map[string]int32{constUtil.P3OpenServerTotalPaid: total.Value, constUtil.P3OpenServerSinglePaid + p3Day(now): single.Value}
		for _, itemID := range []int32{2, 153} {
			var task db.OpenServerActivityTask
			db.GetIns().First(&task, itemID)
			value, e := s.openServerCurrent(p, g, &cfg, task, values)
			if e != nil || value != 128 {
				t.Fatal("shared original payment hunk missing", itemID, value, e)
			}
			messages, e := s.openServerActivity(ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(itemID)}, int64(90000+itemID))
			if e != nil || len(messages) == 0 {
				t.Fatal("paid 14day claim", itemID, e)
			}
		}
	})
	if name != "" {
		var count int64
		if e := db.GetIns().Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name LIKE 'p3_%' AND engine <> 'InnoDB'", name).Scan(&count).Error; e != nil || count != 0 {
			t.Fatal("P3 must use InnoDB", count, e)
		}
	}
	// Ensure test snapshots can be decoded after all transactions; evidence avoids credentials.
	var orders []db.PaymentOrder
	if e := db.GetIns().Find(&orders).Error; e != nil {
		t.Fatal(e)
	}
	for _, o := range orders {
		var p db.PaymentProduct
		if e := json.Unmarshal([]byte(o.Snapshot), &p); e != nil {
			t.Fatal(e)
		}
	}
}
