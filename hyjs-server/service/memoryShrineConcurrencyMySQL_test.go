package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func memoryMySQLFixture(t *testing.T) (*p1Fixture, int64) {
	t.Helper()
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	peer, err := LoginAccount(f.ctx, fmt.Sprintf("memory-concurrency-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, peer.Id, "圣坛并发角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, peer.Id} {
		memoryFundMySQL(t, f.ctx, id, 1000)
	}
	return f, peer.Id
}

func memoryFundMySQL(t *testing.T, ctx context.Context, id int64, light int32) {
	t.Helper()
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Light = proto.Int32(light)
		p.Diamond = proto.Int64(100000)
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_013"), Num: proto.Int32(1)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func memoryMakeLiveMySQL(t *testing.T) (db.P4CommerceRule, p4CommerceRules) {
	t.Helper()
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind=? AND enabled=?", "memory", true).First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.LiveEditable = true
	cfg.Rules, _ = json.Marshal(rules)
	cfg.Frozen = nil
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": cfg.Rules, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	return cfg, rules
}

// 手工持锁只隔离 P4 目录；真实请求仍走角色存档、消费、回执与同步。
func TestMemoryShrineMySQLIndependentLocksAndAdminGuard(t *testing.T) {
	f, peer := memoryMySQLFixture(t)
	cfg, rules := memoryMakeLiveMySQL(t)
	now := time.Now()
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now); err != nil {
		t.Fatal(err)
	}
	peerList, err := f.s.p4Commerce(f.ctx, peer, query, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	items := peerList[0].(*pb.SCRefreshShopQuery).GetShopItemList()
	if len(items) == 0 {
		t.Fatal("missing peer offers")
	}
	var point db.P4CommerceRule
	if err := db.GetIns().Where("kind=? AND id NOT LIKE ?", "point", "shop_schedule_%").Order("start_at DESC,id DESC").First(&point).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	defer cancel()
	tx := db.GetIns().WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	for _, key := range []string{constUtil.P4PopularityRankRulePrefix + "local-v1"} {
		var locked db.P4CommerceRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", key).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&point, "id=?", point.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := f.s.loadP4CommerceForMemory(tx, f.id, "", 3, now, true, false, errors.New("retry")); err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan error, 1)
	go func() {
		_, err := f.s.p4Commerce(ctx, peer, &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: items[0].Id}, 99101, now)
		if err == nil {
			_, err = f.s.p4Commerce(ctx, peer, &pb.CSRefreshShopRefresh{ShopType: proto.Int32(3)}, 99102, now)
		}
		requestDone <- err
	}()
	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("another player's purchase/refresh waited on unrelated or shared directory locks")
	}
	// 运营保存同一目录必须等待读者提交，避免提交后的交易使用旧规则。
	rules.FreeCap--
	updated, _ := json.Marshal(rules)
	writeStarted := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		close(writeStarted)
		writeDone <- db.GetIns().WithContext(ctx).Model(&db.P4CommerceRule{}).Where("id=?", cfg.ID).Update("rules", updated).Error
	}()
	<-writeStarted
	select {
	case err := <-writeDone:
		t.Fatalf("operator update bypassed shared directory lock: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("operator update remained blocked after transaction")
	}
}

func TestMemoryShrineMySQLFirstFreezeSerializes(t *testing.T) {
	f, peer := memoryMySQLFixture(t)
	cfg, rules := memoryMakeLiveMySQL(t)
	rules.LiveEditable = false
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	defer cancel()
	tx := db.GetIns().WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if _, _, _, _, err := f.s.loadP4CommerceForMemory(tx, f.id, "", 3, time.Now(), true, false, errors.New("retry")); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.s.p4Commerce(ctx, peer, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, time.Now())
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("concurrent request bypassed first freeze: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("request remained blocked after first freeze")
	}
	var actual db.P4CommerceRule
	if err := db.GetIns().First(&actual, "id=?", cfg.ID).Error; err != nil || len(actual.Frozen) == 0 {
		t.Fatalf("first freeze was not committed: %v", err)
	}
	var snapshot db.P4CommerceRule
	if err := json.Unmarshal(actual.Frozen, &snapshot); err != nil || !reflect.DeepEqual(snapshot.Rules, raw) {
		t.Fatal("first freeze did not preserve the operator rules", err)
	}
}

func TestMemoryShrineMySQLConcurrentEditRequiresFreezeRetry(t *testing.T) {
	f, _ := memoryMySQLFixture(t)
	cfg, rules := memoryMakeLiveMySQL(t)
	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	defer cancel()
	tx := db.GetIns().WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	// 提前建立一致读快照，模拟预读后运营将目录切换为待冻结版本。
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", cfg.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("snapshot: count=%d error=%v", count, err)
	}
	rules.LiveEditable = false
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().WithContext(ctx).Model(&db.P4CommerceRule{}).Where("id=?", cfg.ID).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	retry := errors.New("retry freeze")
	_, _, _, _, err := f.s.loadP4CommerceForMemory(tx, f.id, "", 3, time.Now(), true, false, retry)
	if !errors.Is(err, retry) {
		t.Fatalf("changed directory did not request transaction rollback: %v", err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	var frozen db.P4CommerceRule
	if err := db.GetIns().First(&frozen, "id=?", cfg.ID).Error; err != nil || len(frozen.Frozen) == 0 {
		t.Fatalf("exclusive retry failed to freeze latest directory: %v", err)
	}
	var snapshot db.P4CommerceRule
	if err := json.Unmarshal(frozen.Frozen, &snapshot); err != nil || !reflect.DeepEqual(snapshot.Rules, raw) {
		t.Fatal("frozen rules differ from the committed operator edit", err)
	}
}

func TestMemoryShrineMySQLConcurrentEditAutoRetries(t *testing.T) {
	f, _ := memoryMySQLFixture(t)
	cfg, rules := memoryMakeLiveMySQL(t)
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	entered := make(chan struct{})
	resume := make(chan struct{})
	var resumed sync.Once
	defer resumed.Do(func() { close(resume) })
	var observed atomic.Bool
	callback := db.GetIns().Callback().Query()
	const hook = "memory-test:after-unlocked-preview"
	if err := callback.After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table != "p4_commerce_rules" || len(tx.Statement.Selects) != 2 || tx.Statement.Selects[0] != "rules" || tx.Statement.Selects[1] != "frozen" || !observed.CompareAndSwap(false, true) {
			return
		}
		close(entered)
		select {
		case <-resume:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove(hook)
	done := make(chan error, 1)
	go func() {
		_, err := f.s.p4Commerce(ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, time.Now())
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("request ended before its unlocked preview: %v", err)
	case <-ctx.Done():
		t.Fatal("request did not reach the unlocked preview")
	}
	rules.LiveEditable = false
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().WithContext(ctx).Model(&db.P4CommerceRule{}).Where("id=?", cfg.ID).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	resumed.Do(func() { close(resume) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("outer transaction did not retry: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("outer transaction retry did not finish")
	}
	var frozen db.P4CommerceRule
	if err := db.GetIns().First(&frozen, "id=?", cfg.ID).Error; err != nil || len(frozen.Frozen) == 0 {
		t.Fatalf("automatic retry did not freeze the latest directory: %v", err)
	}
	var snapshot db.P4CommerceRule
	if err := json.Unmarshal(frozen.Frozen, &snapshot); err != nil || !reflect.DeepEqual(snapshot.Rules, raw) {
		t.Fatal("automatic retry froze stale rules", err)
	}
}

func TestMemoryShrineMySQLLastStockRollbackAndReplay(t *testing.T) {
	f, peer := memoryMySQLFixture(t)
	cfg, rules := memoryMakeLiveMySQL(t)
	var product p4Product
	for i := range rules.Products {
		if rules.Products[i].Rewards == "item_013,1" {
			rules.Products[i].Stock = 1
			product = rules.Products[i]
		}
	}
	if product.ID == "" {
		t.Fatal("missing finite-stock fixture product")
	}
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
	requests := map[int64]*pb.CSRefreshShopBuy{}
	for _, id := range []int64{f.id, peer} {
		if _, err := f.s.p4Commerce(f.ctx, id, query, 0, now); err != nil {
			t.Fatal(err)
		}
		var row db.P4CommerceState
		if err := db.GetIns().First(&row, "player_id=? AND rule_id=?", id, cfg.ID).Error; err != nil {
			t.Fatal(err)
		}
		var state p4CommerceProgress
		if err := json.Unmarshal(row.Payload, &state); err != nil {
			t.Fatal(err)
		}
		quote := product
		state.Offers = []memoryOffer55{{ID: fmt.Sprintf("memory55:%d:1:%s", state.Version, product.ID), ProductID: product.ID, Quote: &quote}}
		state.Slots = nil
		if err := saveP4Commerce(db.GetIns(), &row, &state); err != nil {
			t.Fatal(err)
		}
		list, err := f.s.p4Commerce(f.ctx, id, query, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		items := list[0].(*pb.SCRefreshShopQuery).GetShopItemList()
		if len(items) != 1 {
			t.Fatal("fixture did not retain one slot")
		}
		requests[id] = &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: items[0].Id}
	}
	// 余额不足时，首次补建的库存行、次数和回执都必须随事务回滚。
	memoryFundMySQL(t, f.ctx, f.id, 0)
	if _, err := f.s.p4Commerce(f.ctx, f.id, requests[f.id], 99200, now); err == nil {
		t.Fatal("unfunded purchase succeeded")
	}
	var count int64
	if err := db.GetIns().Model(&db.P4CommerceStock{}).Where("rule_id=? AND product_id=?", cfg.ID, product.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed purchase left stock behind: %d %v", count, err)
	}
	if err := db.GetIns().Model(&db.P4Receipt{}).Where("player_id=? AND domain=? AND timestamp=?", f.id, "commerce:3", 99200).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed purchase left a receipt: %d %v", count, err)
	}
	memoryFundMySQL(t, f.ctx, f.id, 1000)
	type result struct {
		id       int64
		messages []proto.Message
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, id := range []int64{f.id, peer} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			messages, err := f.s.p4Commerce(ctx, id, requests[id], 99201, now)
			results <- result{id: id, messages: messages, err: err}
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	var winner result
	successes := 0
	var loser error
	for result := range results {
		if result.err == nil {
			winner = result
			successes++
		} else {
			loser = result.err
		}
	}
	if successes != 1 || loser == nil || !strings.Contains(loser.Error(), "共享库存不足") {
		t.Fatalf("expected one success and one stock rejection, got %d successes and %v", successes, loser)
	}
	var stock db.P4CommerceStock
	if err := db.GetIns().First(&stock, "rule_id=? AND product_id=?", cfg.ID, product.ID).Error; err != nil || stock.Used != 1 {
		t.Fatalf("incorrect committed stock: %+v %v", stock, err)
	}
	before := map[int64]int32{}
	for _, id := range []int64{f.id, peer} {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, _, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		before[id] = p.GetLight()
		if id == winner.id && p.GetLight() != 990 || id != winner.id && p.GetLight() != 1000 {
			t.Fatalf("unexpected charge for player %d: %d", id, p.GetLight())
		}
	}
	replay, err := f.s.p4Commerce(f.ctx, winner.id, requests[winner.id], 99201, now)
	if err != nil || !proto.Equal(replay[0], winner.messages[0]) {
		t.Fatal("committed receipt did not replay", err)
	}
	if _, err := f.s.p4Commerce(f.ctx, winner.id, &pb.CSRefreshShopRefresh{ShopType: proto.Int32(3)}, 99201, now); err == nil {
		t.Fatal("same timestamp with different request was accepted")
	}
	var row db.Profile
	if err := db.GetIns().First(&row, winner.id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&row)
	if err != nil || p.GetLight() != before[winner.id] {
		t.Fatal("receipt replay charged again", err)
	}
}

func TestMemoryShrineMySQLFrozenPriceSamePlayerAndColdReplay(t *testing.T) {
	f, _ := memoryMySQLFixture(t)
	cfg, rules := memoryMakeLiveMySQL(t)
	for i := range rules.Products {
		rules.Products[i].QuantityTiers = []model.AdminShopQuantityTier{{Amount: 2, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: "light", Amount: 29}}}
	}
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
	first, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	items := first[0].(*pb.SCRefreshShopQuery).GetShopItemList()
	if len(items) == 0 || len(items[0].GetItem()) != 1 || items[0].GetItem()[0].GetNum() != 2 || items[0].GetPrice()[0].GetNum() != 29 {
		t.Fatal("quantity tier was not shown")
	}
	quoted := proto.Clone(items[0]).(*pb.ShopItem)
	for i := range rules.Products {
		rules.Products[i].Cost = "light,91"
		rules.Products[i].QuantityTiers = []model.AdminShopQuantityTier{{Amount: 3, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: "light", Amount: 91}}}
	}
	raw, _ = json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	// 冷重连后仍按本次刷新已经展示的数量和价格购买。
	f.restart(t)
	listed, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	current := listed[0].(*pb.SCRefreshShopQuery).GetShopItemList()
	if len(current) == 0 || current[0].GetId() != quoted.GetId() || current[0].GetItem()[0].GetNum() != 2 || current[0].GetPrice()[0].GetNum() != 29 {
		t.Fatal("operator edit changed a saved quote")
	}
	buy := &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: quoted.Id}
	type result struct {
		messages []proto.Message
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			messages, err := f.s.p4Commerce(ctx, f.id, buy, 99301, now)
			results <- result{messages: messages, err: err}
		}()
	}
	close(start)
	var replies []proto.Message
	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			replies = append(replies, result.messages[0])
		case <-ctx.Done():
			t.Fatal("same-player retry did not complete")
		}
	}
	if !proto.Equal(replies[0], replies[1]) {
		t.Fatal("concurrent identical requests returned different receipts")
	}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&row)
	if err != nil || p.GetLight() != 971 {
		t.Fatalf("frozen price was not charged exactly once: %v", err)
	}
	var receipts int64
	if err := db.GetIns().Model(&db.P4Receipt{}).Where("player_id=? AND domain=? AND timestamp=?", f.id, "commerce:3", 99301).Count(&receipts).Error; err != nil || receipts != 1 {
		t.Fatalf("same-player purchase produced %d receipts: %v", receipts, err)
	}
	refresh, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSRefreshShopRefresh{ShopType: proto.Int32(3)}, 99302, now)
	if err != nil {
		t.Fatal(err)
	}
	newItems := refresh[0].(*pb.SCRefreshShopRefresh).GetShopItemList()
	if len(newItems) == 0 || newItems[0].GetItem()[0].GetNum() != 3 || newItems[0].GetPrice()[0].GetNum() != 91 {
		t.Fatal("new refresh did not use current operator terms")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 99303, now); err == nil {
		t.Fatal("old slot remained purchasable after refresh")
	}
}

func TestMemoryShrineMySQLSeedOnlyCurrentMonth(t *testing.T) {
	f, _ := memoryMySQLFixture(t)
	now := time.Now().AddDate(0, 2, 0)
	start := time.Date(now.In(time.FixedZone("UTC+8", 8*3600)).Year(), now.In(time.FixedZone("UTC+8", 8*3600)).Month(), 1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	expected, err := p4CommerceSeedRule("memory", "p4:memory:", 3, f.s.p4DefaultMemoryRules(), start, start.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	startTogether := make(chan struct{})
	result := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-startTogether
			result <- db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { return f.s.seedP4Memory(tx, now) })
		}()
	}
	close(startTogether)
	for i := 0; i < 2; i++ {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent monthly seeding did not finish")
		}
	}
	var actual db.P4CommerceRule
	if err := db.GetIns().First(&actual, "id=?", expected.ID).Error; err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("monthly memory seed differs from original default: %v", err)
	}
	var unrelated int64
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", "p4:ordinary:"+start.Format("200601")).Count(&unrelated).Error; err != nil || unrelated != 0 {
		t.Fatalf("memory seed initialized unrelated monthly directory: %d %v", unrelated, err)
	}
}

type memorySQLCounter struct {
	logger.Interface
	count atomic.Int64
}

func (counter *memorySQLCounter) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	counter.count.Add(1)
}

// 本地同输入的目录加载对照；这不包含其他协议处理和线上资源竞争。
func TestMemoryShrineMySQLSameInputDirectoryComparison(t *testing.T) {
	f, _ := memoryMySQLFixture(t)
	memoryMakeLiveMySQL(t)
	now := time.Now()
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}, 0, now); err != nil {
		t.Fatal(err)
	}
	counter := &memorySQLCounter{Interface: logger.Discard}
	database := db.GetIns().Session(&gorm.Session{Logger: counter})
	rollback := errors.New("rollback comparison")
	type measurements struct {
		durations []time.Duration
		queries   []int64
	}
	var old, fast measurements
	measure := func(useFast bool, target *measurements) {
		t.Helper()
		counter.count.Store(0)
		start := time.Now()
		err := database.WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			if useFast {
				_, _, _, _, err = f.s.loadP4CommerceForMemory(tx, f.id, "", 3, now, true, false, errors.New("retry"))
			} else {
				_, _, _, _, err = f.s.loadP4Commerce(tx, f.id, "", 3, now)
			}
			if err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		target.durations = append(target.durations, time.Since(start))
		target.queries = append(target.queries, counter.count.Load())
	}
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			measure(false, &old)
			measure(true, &fast)
		} else {
			measure(true, &fast)
			measure(false, &old)
		}
	}
	for _, sample := range []*measurements{&old, &fast} {
		sort.Slice(sample.durations, func(i, j int) bool { return sample.durations[i] < sample.durations[j] })
		sort.Slice(sample.queries, func(i, j int) bool { return sample.queries[i] < sample.queries[j] })
	}
	t.Logf("same memory directory, 20 samples: original p50=%s p95=%s SQL=%d; fast p50=%s p95=%s SQL=%d", old.durations[9], old.durations[18], old.queries[9], fast.durations[9], fast.durations[18], fast.queries[9])
	if fast.queries[9] >= old.queries[9] {
		t.Fatalf("memory directory SQL count did not decrease: original=%d fast=%d", old.queries[9], fast.queries[9])
	}
}
