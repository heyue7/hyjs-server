package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type memoryRequestSQLCounter struct {
	logger.Interface
	count atomic.Int64
}

func (counter *memoryRequestSQLCounter) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	counter.count.Add(1)
}

// 相同隔离库、角色和货架输入下测量完整 P4 协议入口至库存同步；可在两个源码版本复用。
func TestMemoryShrineMySQLSameInputRequestTiming(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	p4Fund(t, f)
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind=? AND enabled=?", "memory", true).First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.LiveEditable = true
	var product p4Product
	for _, candidate := range rules.Products {
		if candidate.Rewards == "item_013,1" {
			product = candidate
		}
	}
	if product.ID == "" || product.Stock != -1 || product.Cost != "light,10" {
		t.Fatal("benchmark fixture lacks its fixed product")
	}
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	now := time.Now()
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
	if _, err := f.s.p4Commerce(ctx, f.id, query, 0, now); err != nil {
		t.Fatal(err)
	}
	var stateRow db.P4CommerceState
	if err := db.GetIns().First(&stateRow, "player_id=? AND rule_id=?", f.id, cfg.ID).Error; err != nil {
		t.Fatal(err)
	}
	var state p4CommerceProgress
	if err := json.Unmarshal(stateRow.Payload, &state); err != nil {
		t.Fatal(err)
	}
	state.Offers = []memoryOffer55{{ID: fmt.Sprintf("memory55:%d:1:%s", state.Version, product.ID), ProductID: product.ID, Quote: &product}}
	state.Slots = nil
	state.FreeLeft = 1
	state.ResourceUsed = 0
	state.Bought = map[string]int32{}
	state.Total = map[string]int32{}
	if err := saveP4Commerce(db.GetIns(), &stateRow, &state); err != nil {
		t.Fatal(err)
	}
	listed, err := f.s.p4Commerce(ctx, f.id, query, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	items := listed[0].(*pb.SCRefreshShopQuery).GetShopItemList()
	if len(items) != 1 || items[0].GetPrice()[0].GetNum() != 10 {
		t.Fatal("benchmark slot differs from the requested input")
	}
	if err := db.GetIns().First(&stateRow, "player_id=? AND rule_id=?", f.id, cfg.ID).Error; err != nil {
		t.Fatal(err)
	}
	statePayload := append([]byte(nil), stateRow.Payload...)
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	// 恢复相同角色和货架快照的 SQL 不计入协议入口计时。
	reset := func() {
		t.Helper()
		if err := db.GetIns().Save(&profile).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&stateRow).Update("payload", statePayload).Error; err != nil {
			t.Fatal(err)
		}
	}
	type sample struct {
		timings []time.Duration
		sql     []int64
	}
	values := map[string]*sample{"query": {}, "refresh": {}, "buy": {}}
	counter := &memoryRequestSQLCounter{Interface: logger.Discard}
	priorLogger := db.GetIns().Logger
	db.GetIns().Logger = counter
	defer func() { db.GetIns().Logger = priorLogger }()
	for i := 0; i < 22; i++ {
		for j, name := range []string{"query", "refresh", "buy"} {
			reset()
			var request proto.Message
			switch name {
			case "query":
				request = query
			case "refresh":
				request = &pb.CSRefreshShopRefresh{ShopType: proto.Int32(3)}
			case "buy":
				request = &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: items[0].Id}
			}
			counter.count.Store(0)
			started := time.Now()
			messages, err := f.s.p4Commerce(context.WithValue(ctx, constUtil.GameRequestContextKey, request), f.id, request, int64(300000+i*3+j), now)
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if name == "refresh" && len(messages[0].(*pb.SCRefreshShopRefresh).GetCost()) != 0 {
				t.Fatal("benchmark refresh was not free")
			}
			if name == "buy" && messages[0].(*pb.SCRefreshShopBuy).GetCost()[0].GetNum() != 10 {
				t.Fatal("benchmark purchase used unexpected price")
			}
			if i >= 2 {
				values[name].timings = append(values[name].timings, elapsed)
				values[name].sql = append(values[name].sql, counter.count.Load())
			}
		}
	}
	for _, name := range []string{"query", "refresh", "buy"} {
		s := values[name]
		sort.Slice(s.timings, func(i, j int) bool { return s.timings[i] < s.timings[j] })
		sort.Slice(s.sql, func(i, j int) bool { return s.sql[i] < s.sql[j] })
		t.Logf("%s: 20 identical-input samples p50=%s p95=%s SQL p50=%d", name, s.timings[9], s.timings[18], s.sql[9])
	}
}
