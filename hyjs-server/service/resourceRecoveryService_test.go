package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestResourceRecoveryOfflineAndRemainder(t *testing.T) {
	var (
		nowMS       int64 = 1800000000000
		energyLast        = nowMS - 2*constUtil.EnergyRecoveryIntervalMS - 12345
		staminaLast       = nowMS - 3*constUtil.StaminaRecoveryIntervalMS - 54321
		player            = &pb.Player{Energy: proto.Int32(10), Stamina: proto.Int32(20), EnergyLastRefreshTime: proto.Int64(energyLast), StaminaLastRefreshTime: proto.Int64(staminaLast)}
	)
	changed, err := settleResourceRecovery(player, nowMS)
	if err != nil || !changed || player.GetEnergy() != 12 || player.GetStamina() != 23 {
		t.Fatalf("offline recovery: %v %v %v", player, changed, err)
	}
	if player.GetEnergyLastRefreshTime() != nowMS-12345 || player.GetStaminaLastRefreshTime() != nowMS-54321 {
		t.Fatal("partial interval was discarded")
	}
	if changed, err = settleResourceRecovery(player, nowMS); err != nil || changed {
		t.Fatalf("same instant recovered twice: %v %v", changed, err)
	}
	// 恢复在边界时精确增加一点，后续请求继续保留新的周期起点。
	if _, err = settleResourceRecovery(player, nowMS-54321+constUtil.StaminaRecoveryIntervalMS); err != nil || player.GetStamina() != 24 {
		t.Fatalf("exact interval boundary: %v %v", player.GetStamina(), err)
	}
}

func TestResourceRecoveryOverCapAndConsumption(t *testing.T) {
	var (
		nowMS  int64 = 1800000000000
		last         = nowMS - 24*60*60*1000
		player       = &pb.Player{Energy: proto.Int32(100), Stamina: proto.Int32(120), EnergyLastRefreshTime: proto.Int64(last), StaminaLastRefreshTime: proto.Int64(last)}
	)
	if changed, err := settleResourceRecovery(player, nowMS); err != nil || changed || player.GetEnergy() != 100 || player.GetStamina() != 120 {
		t.Fatalf("over-cap balance changed: %v %v %v", player, changed, err)
	}
	player.Stamina = proto.Int32(99)
	if err := finalizeResourceRecovery(player, 100, 120, nowMS); err != nil || player.GetStaminaLastRefreshTime() != nowMS {
		t.Fatalf("consume below cap must restart: %v %v", player, err)
	}
	if _, err := settleResourceRecovery(player, nowMS+constUtil.StaminaRecoveryIntervalMS-1); err != nil || player.GetStamina() != 99 {
		t.Fatal("old full-resource time granted recovery immediately")
	}
	if _, err := settleResourceRecovery(player, nowMS+constUtil.StaminaRecoveryIntervalMS); err != nil || player.GetStamina() != 100 {
		t.Fatal("missing first tick after consume")
	}
	// 未满时继续消费保留计时，不随每次消费重新延后。
	player.Stamina = proto.Int32(70)
	player.StaminaLastRefreshTime = proto.Int64(nowMS - 12345)
	if err := finalizeResourceRecovery(player, 80, 70, nowMS); err != nil || player.GetStaminaLastRefreshTime() != nowMS-12345 {
		t.Fatal("below-cap consumption reset the running timer")
	}
}

func TestResourceRecoveryMidnightCapAndLegacy(t *testing.T) {
	var (
		zone   = time.FixedZone("Asia/Shanghai", 8*60*60)
		nowMS  = time.Date(2026, 10, 13, 0, 2, 0, 0, zone).UnixMilli()
		player = &pb.Player{Energy: proto.Int32(29), Stamina: proto.Int32(98), EnergyLastRefreshTime: proto.Int64(nowMS - 7200000), StaminaLastRefreshTime: proto.Int64(nowMS - 7200000)}
	)
	if _, err := settleResourceRecovery(player, nowMS); err != nil || player.GetEnergy() != 30 || player.GetStamina() != 100 {
		t.Fatalf("offline midnight recovery exceeded cap: %v %v", player, err)
	}
	player.EnergyLastRefreshTime = nil
	player.StaminaLastRefreshTime = proto.Int64(nowMS + 5000)
	player.Energy = proto.Int32(5)
	player.Stamina = proto.Int32(50)
	if _, err := settleResourceRecovery(player, nowMS); err != nil || player.GetEnergy() != 5 || player.GetStamina() != 50 || player.GetEnergyLastRefreshTime() != nowMS || player.GetStaminaLastRefreshTime() != nowMS {
		t.Fatalf("legacy/future timestamps must initialize without rewards: %v %v", player, err)
	}
	player.Stamina = proto.Int32(-1)
	if _, err := settleResourceRecovery(player, nowMS); err == nil {
		t.Fatal("negative resource balance accepted")
	}
}

// TestResourceRecoveryMySQLParallelQueries 使用独立 MySQL 测试库验证游标只推进一次。
func TestResourceRecoveryMySQLParallelQueries(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("isolated MySQL test database required")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	row, err := LoginAccount(ctx, fmt.Sprintf("resource-query-%d", time.Now().UnixNano()), "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, row.Id, "resource-test", "hero_047"); err != nil {
		t.Fatal(err)
	}
	nowMS := time.Now().UnixMilli()
	// 直接写隔离测试存档，防止业务钩子提前消费用于并发查询的三个恢复周期。
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var saved db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&saved, row.Id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&saved)
		if err != nil {
			return err
		}
		player.Energy = proto.Int32(10)
		player.Stamina = proto.Int32(50)
		player.EnergyLastRefreshTime = proto.Int64(nowMS - 2*constUtil.EnergyRecoveryIntervalMS - 10000)
		player.StaminaLastRefreshTime = proto.Int64(nowMS - 3*constUtil.StaminaRecoveryIntervalMS - 10000)
		return saveProfile(tx, &saved, player, game)
	})
	if err != nil {
		t.Fatal(err)
	}
	var (
		wg   sync.WaitGroup
		fail = make(chan error, 16)
		svc  = &GameService{}
	)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			player, _, err := svc.resourceProfile(ctx, row.Id)
			if err == nil && (player.GetEnergy() != 12 || player.GetStamina() != 53) {
				err = fmt.Errorf("duplicate recovery: energy=%d stamina=%d", player.GetEnergy(), player.GetStamina())
			}
			fail <- err
		}()
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		if err != nil {
			t.Fatal(err)
		}
	}
	player, _, err := svc.resourceProfile(ctx, row.Id)
	if err != nil || player.GetEnergyLastRefreshTime() != nowMS-10000 || player.GetStaminaLastRefreshTime() != nowMS-10000 {
		t.Fatalf("cursor not persisted: %v %v", player, err)
	}
	// 查询与消费共用相同行锁，交错执行后每次扣减均可在重读存档中核实。
	fail = make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, err := svc.resourceProfile(ctx, row.Id)
			fail <- err
		}()
		go func() {
			defer wg.Done()
			fail <- updateProfile(ctx, row.Id, func(p *pb.Player, _ *model.GameState) error {
				p.Stamina = proto.Int32(p.GetStamina() - 1)
				return nil
			})
		}()
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		if err != nil {
			t.Fatal(err)
		}
	}
	player, _, err = svc.resourceProfile(ctx, row.Id)
	if err != nil || player.GetStamina() != 37 || player.GetEnergy() != 12 || player.GetStaminaLastRefreshTime() != nowMS-10000 {
		t.Fatalf("query/consume lost balance or interval: %v %v", player, err)
	}
}
