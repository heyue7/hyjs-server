package service

import (
	"context"
	"encoding/json"
	"errors"
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

// TestP3VIP9GrowthActualTestOrdersAndOriginalBook 从空新角色实际确认五笔测试订单取得VIP9，再购买原成长礼包。
// 本地价格和关闭目录显式区分于缺失原购买价格；不向手机或正式角色写入资格。
func TestP3VIP9GrowthActualTestOrdersAndOriginalBook(t *testing.T) {
	ctx := context.Background()
	name := os.Getenv("HYJS_P3_MYSQL_DATABASE")
	path := filepath.Join(t.TempDir(), "vip9.db")
	open := func() {
		t.Helper()
		var err error
		if name == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(name, "hyjs_test_p3_") {
				t.Fatal("isolated P3 schema required")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadPaymentTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializePayments, s.InitializeP5, s.InitCooking, s.InitializeP8Life, s.InitializeP8Gather} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	protected, err := LoginAccount(ctx, "protected-p3-vip9-original", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, protected.Id, "保护角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(ctx, "demo-p3-vip9", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	id := account.Id
	if _, err = CreatePlayer(ctx, id, "原会员礼包", "hero_043"); err != nil {
		t.Fatal(err)
	}
	state := func() (*pb.Player, *model.GameState) {
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
	now := time.Now()
	list := func() []*pb.ShopItem {
		t.Helper()
		r, e := s.vipShop(ctx, id, &pb.CSShopProductList{ShopType: proto.Int32(2)}, now)
		if e != nil {
			t.Fatal(e)
		}
		return r[0].(*pb.SCShopProductList).GetShopItemList()
	}
	for _, row := range list() {
		if strings.HasPrefix(row.GetId(), "p3vip") && strings.HasSuffix(row.GetId(), "_growth_local") {
			t.Fatal("defaultoff product visible")
		}
	}
	var cfg db.VIPShopProduct
	if err = db.GetIns().First(&cfg, "id=?", constUtil.P3VIP9GrowthProduct).Error; err != nil || cfg.Enabled || cfg.MinVIP != 9 || cfg.Cost != 1000 || cfg.Reward != "item_043,1" {
		t.Fatal("defaultoff local quote")
	}
	if err = db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(vipShopSKU(cfg)), BuyCount: proto.Int32(1)}
	if _, err = s.vipShop(ctx, id, buy, now); err == nil {
		t.Fatal("unpaid fresh player accepted")
	}
	// 所有资格和钻石均经现有可信隔离测试支付适配器及原VIP表实际累计。
	for i := int64(1); i <= 5; i++ {
		req := &pb.CSGenerateOrder{PaymentId: proto.String("djsy.wuyou.andp2000"), Money: proto.Float64(2000), MoneyType: proto.String("RMB"), ChannelId: proto.String("AND0003"), ShopId: proto.String("common")}
		r, e := s.generatePayment(ctx, id, req, i, now)
		if e != nil {
			t.Fatal(e)
		}
		trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
		if _, e = s.confirmTestPaymentAt(trusted, r[0].(*pb.SCGenerateOrder).GetOrderId(), "isolated-vip9-"+string(rune('0'+i)), 200000, constUtil.P3PaymentConfirmed, now); e != nil {
			t.Fatal(e)
		}
	}
	p, g := state()
	if p.GetVip() != 9 || p.GetChargeDiamond() != 100000 || p5LifeItemCount(g, "item_043") != 0 {
		t.Fatal("VIP9 was not derived from five real confirmed test orders")
	}
	before := p.GetDiamond()
	for _, row := range list() {
		if row.GetId() == vipShopSKU(cfg) && (row.GetCanBuy() != 1 || len(row.Price) != 1 || len(row.Discount) != 1 || !proto.Equal(row.Price[0], row.Discount[0]) || row.Price[0].GetNum() != 1000) {
			t.Fatal("original member card price/discount/qualification")
		}
	}
	// A stale member quote must reject without a debit; refreshing publishes the current real SKU.
	if err = db.GetIns().Model(&cfg).Update("cost", 9999).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.vipShop(ctx, id, buy, now); !errors.Is(err, constUtil.ErrShopQuoteChanged) {
		t.Fatal("stale quote accepted", err)
	}
	if err = db.GetIns().Model(&cfg).Update("cost", 1000).Error; err != nil {
		t.Fatal(err)
	}
	list()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.vipShop(ctx, id, buy, now); errs <- e }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	p, g = state()
	if success != 1 || p.GetDiamond() != before-1000 || p5LifeItemCount(g, "item_043") != 1 {
		t.Fatal("member quote/quota/once debit", success)
	}
	use := &pb.CSItemUse{CostItem: p2Item("item_043", 1), GetItemIndex: proto.Int32(0)}
	if _, err = s.p5BoxUse(ctx, id, use, 9001); err != nil {
		t.Fatal(err)
	}
	_, g = state()
	if p5LifeItemCount(g, "item_043") != 0 || p5LifeItemCount(g, "item_065") != 1 {
		t.Fatal("actual original growth box did not produce skill book")
	}
	// 生活开放等级仅为隔离有界资格；礼包、书、会员及钻石来源全部由上述真实业务取得。
	if err = updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(int32(tableNumber(s.table("GameConfigure", "StaticName", "OutdoorLevel"), "NameValue")))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.p8SkillBook(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_065", 1)}, 9002); err != nil {
		t.Fatal(err)
	}
	// 另五笔实际确认的原测试订单自然累积VIP10资格；原VIP10包才包含第二本宝石技能书。
	for i := int64(6); i <= 10; i++ {
		req := &pb.CSGenerateOrder{PaymentId: proto.String("djsy.wuyou.andp2000"), Money: proto.Float64(2000), MoneyType: proto.String("RMB"), ChannelId: proto.String("AND0003"), ShopId: proto.String("common")}
		r, e := s.generatePayment(ctx, id, req, i, now)
		if e != nil {
			t.Fatal(e)
		}
		trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
		if _, e = s.confirmTestPaymentAt(trusted, r[0].(*pb.SCGenerateOrder).GetOrderId(), "isolated-vip10-"+string(rune('A'+i)), 200000, constUtil.P3PaymentConfirmed, now); e != nil {
			t.Fatal(e)
		}
	}
	p, _ = state()
	if p.GetVip() != 10 || p.GetChargeDiamond() != 200000 {
		t.Fatal("actual VIP10 qualification")
	}
	// 保留所有原包内容，VIP8/10原箱实际得到对应技能书。
	for i, spec := range []struct{ product, box, book, skill string }{
		{constUtil.P3VIP8GrowthProduct, "item_042", "item_063", "LifeSkill_085"},
		{constUtil.P3VIP10GrowthProduct, "item_044", "item_064", "LifeSkill_086"},
	} {
		if err = db.GetIns().Model(&db.VIPShopProduct{}).Where("id=?", spec.product).Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.vipShop(ctx, id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(spec.product), BuyCount: proto.Int32(1)}, now); err != nil {
			t.Fatal(err)
		}
		if _, err = s.p5BoxUse(ctx, id, &pb.CSItemUse{CostItem: p2Item(spec.box, 1)}, int64(9010+2*i)); err != nil {
			t.Fatal(err)
		}
		if _, err = s.p8SkillBook(ctx, id, &pb.CSItemUse{CostItem: p2Item(spec.book, 1)}, int64(9011+2*i)); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err = db.GetIns().Model(&db.P8LifeSkill{}).Where("player_id=? AND skill_id=? AND book_id=?", id, spec.skill, spec.book).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("original advanced gem skill source")
		}
	}
	p, g = state()
	beforeCold, _ := json.Marshal(g)
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	_, after := state()
	afterCold, _ := json.Marshal(after)
	if string(beforeCold) != string(afterCold) || p5LifeItemCount(after, "item_065") != 0 {
		t.Fatal("book consumption cold persistence")
	}
	if _, err = s.vipShop(ctx, id, buy, now); err == nil {
		t.Fatal("cold daily quota bypass")
	}
	var count int64
	if err = db.GetIns().Model(&db.P8LifeSkill{}).Where("player_id=? AND skill_id=? AND book_id=?", id, "SkillFishItem_051", "item_065").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("original learned skill cold persistence")
	}
}
