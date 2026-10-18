package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestMySQLTurntableTransactions 在显式隔离 MySQL 内验证全局锁、共享库存和持久化防重。
func TestMySQLTurntableTransactions(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("isolated MySQL test database required")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	defer sqlDB.Close()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeTurntable(ctx); err != nil {
		t.Fatal(err)
	}
	// 测试数据库名称已验证，清除仅本测试使用的转盘状态，允许在同一隔离库重复运行。
	for _, table := range []string{"turntable_records", "turntable_states"} {
		if err := db.GetIns().WithContext(ctx).Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	key := "turntable_20261011"
	now := time.Now().UnixMilli()
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id = ?", key).Updates(map[string]any{"enabled": true, "start_at": now - 60000, "end_at": now + 3600000, "reward_end_at": now + 7200000, "jackpot_per_million": 0, "single_cost": "diamond,50", "ten_cost": "diamond,488", "coin_per_draw": 10000, "min_level": 1, "free_per_day": -1, "free_seconds": 86400}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.TurntableRules{}).Where("pool_id = ?", key).Updates(map[string]any{"score_per_draw": 1, "top_prize": "item_008,100", "advance_prize": "item_008,20", "advance_places": 10}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id = ?", key).Updates(map[string]any{"resource_id": "item_011", "amount": 1, "stock": 1, "weight": 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.TurntableGlobal{}).Where("pool_id = ?", key).Updates(map[string]any{"diamond_pool": 10000, "contribution_remainder": 0, "used_stock": []byte("{}"), "settled": false, "final_ranks": nil}).Error; err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for index := 0; index < 9; index++ {
		row, err := LoginAccount(ctx, fmt.Sprintf("turn-%d-%d", now, index), "test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, fmt.Sprintf("真人%d", index), "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(player *pb.Player, g *model.GameState) error { player.Diamond = proto.Int64(1000); return nil }); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.Id)
	}
	if _, err := s.playBigTurn(ctx, ids[0], &pb.CSPlayBigTurn{Count: proto.Int32(10)}, 100); err == nil {
		t.Fatal("ten draws exceeded nine finite prizes")
	}
	var records int64
	if err := db.GetIns().Model(&db.TurntableRecord{}).Count(&records).Error; err != nil || records != 0 {
		t.Fatalf("rollback records=%d err=%v", records, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 9)
	results := make(chan *pb.SCPlayBigTurn, 9)
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			messages, err := s.playBigTurn(ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 100)
			errs <- err
			if err == nil {
				results <- messages[0].(*pb.SCPlayBigTurn)
			}
		}(id)
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[int32]bool{}
	for result := range results {
		if seen[result.GetRewardItemId()] {
			t.Fatal("shared prize awarded twice")
		}
		seen[result.GetRewardItemId()] = true
	}
	if len(seen) != 9 {
		t.Fatal("missing shared awards")
	}
	var global db.TurntableGlobal
	if err := db.GetIns().Where("pool_id = ?", key).First(&global).Error; err != nil {
		t.Fatal(err)
	}
	stock := map[int32]int32{}
	if err := json.Unmarshal(global.UsedStock, &stock); err != nil {
		t.Fatal(err)
	}
	for index := int32(0); index < 9; index++ {
		if stock[index] != 1 {
			t.Fatalf("stock[%d]=%d", index, stock[index])
		}
	}
	if _, err := s.playBigTurn(ctx, ids[0], &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 101); err == nil {
		t.Fatal("exhausted shared pool accepted")
	}
	// 反复发送最后一个包只能返回回执，不重复扣费、增加积分或记录。
	for retry := 0; retry < 6; retry++ {
		if _, err := s.playBigTurn(ctx, ids[0], &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.TurntableRecord{}).Count(&records).Error; err != nil || records != 9 {
		t.Fatalf("records=%d err=%v", records, err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&row)
	if err != nil || player.GetCoin() != 10000 || player.GetDiamond() != 1000 {
		t.Fatalf("draw changed twice: %v %v", player, err)
	}
	// 两个并发结束查询共用全局锁，排名奖励整批只提交一次。
	if err = db.GetIns().Model(&db.GameplayPool{}).Where("id = ?", key).Update("end_at", time.Now().UnixMilli()-1).Error; err != nil {
		t.Fatal(err)
	}
	settled := make(chan error, 2)
	for _, id := range ids[:2] {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); _, e := s.bigTurnRank(ctx, id); settled <- e }(id)
	}
	wg.Wait()
	close(settled)
	for e := range settled {
		if e != nil {
			t.Fatal(e)
		}
	}
	if err = db.GetIns().First(&row, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	owned := int32(0)
	for _, item := range game.Items {
		if item.GetId() == "item_008" {
			owned += item.GetNum()
		}
	}
	if owned != 100 {
		t.Fatalf("top rank reward=%d", owned)
	}
}
