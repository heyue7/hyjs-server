package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestTurntableScreenshotSourceKeepsOldAndRedConfiguration(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	var old, red []db.GameplayPoolReward
	if err := db.GetIns().Where("pool_id=?", "turntable_20261011").Order("id").Find(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("pool_id=?", constUtil.P2RedWheelPool).Order("id").Find(&red).Error; err != nil {
		t.Fatal(err)
	}
	pool, prizes, _, err := f.s.turntableConfigForPool(db.GetIns(), constUtil.TurntableScreenshotPool)
	if err != nil {
		t.Fatal(err)
	}
	if pool.SingleCost != "diamond,50" || pool.TenCost != "diamond,488" || pool.CoinPerDraw != 10000 {
		t.Fatal("原图价格不一致", pool)
	}
	for id, expected := range map[int32]string{0: "item_340,1", 1: "coin,50000", 2: "item_001,3", 3: "pet_066,1", 4: "item_027,3", 5: "item_138,3", 6: "MineItem_079,1", 7: "item_127,3", 8: "item_139,3"} {
		if value := prizes[id]; value.ResourceID+","+strconv.FormatInt(int64(value.Amount), 10) != expected || value.Weight != old[id].Weight {
			t.Fatal("明确奖位/原本地概率不一致", value, expected)
		}
	}
	if prizes[0].Stock != 3 || prizes[3].Stock != 2 || prizes[6].Stock != 8 || pool.Source != constUtil.TurntableScreenshotSource {
		t.Fatal("用户最终精灵/库存决定不一致", prizes)
	}
	// 已存在的本轮partial占位可升级；人工改过的格子保持原值。
	var migrated db.GameplayPoolReward
	for i, source := range []string{constUtil.TurntableScreenshotPartialSource, constUtil.TurntableScreenshotSQLPartialSource} {
		if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", pool.ID).Update("source", source).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=3", pool.ID).Updates(map[string]any{"resource_id": "item_008", "amount": 1, "stock": 0}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=4", pool.ID).Updates(map[string]any{"resource_id": "item_005", "amount": 20}).Error; err != nil {
			t.Fatal(err)
		}
		var frozenCycle string
		if i == 0 {
			localPlayNonRedWheelSource(t)
			if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "BigTurnTable", "00000000-0000-4000-8000-000000004004"); err != nil {
				t.Fatal(err)
			}
			frozenCycle = localPlayCycleID(t, f, id, "BigTurnTable")
		}
		var globalBefore db.TurntableGlobal
		if err := db.GetIns().First(&globalBefore, "pool_id=?", pool.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.s.InitializeTurntable(f.ctx); err != nil {
			t.Fatal(err)
		}
		migrated = db.GameplayPoolReward{}
		if err := db.GetIns().First(&migrated, "pool_id=? AND id=3", pool.ID).Error; err != nil || migrated.ResourceID != "pet_066" || migrated.Amount != 1 || migrated.Stock != 2 {
			t.Fatal("旧partial占位未迁到用户暗之幽梦兽", err)
		}
		migrated = db.GameplayPoolReward{}
		if err := db.GetIns().First(&migrated, "pool_id=? AND id=4", pool.ID).Error; err != nil || migrated.ResourceID != "item_027" || migrated.Amount != 3 {
			t.Fatal("旧partial占位未迁到用户装备箱", err)
		}
		var globalAfter db.TurntableGlobal
		if err := db.GetIns().First(&globalAfter, "pool_id=?", pool.ID).Error; err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(globalBefore)
		after, _ := json.Marshal(globalAfter)
		if !bytes.Equal(before, after) {
			t.Fatal("保护迁移重置全局库存/钻石池")
		}
		if frozenCycle != "" {
			migrated = db.GameplayPoolReward{}
			if err := db.GetIns().First(&migrated, "pool_id=? AND id=3", frozenCycle).Error; err != nil || migrated.ResourceID != "item_008" || migrated.Amount != 1 || migrated.Stock != 0 {
				t.Fatal("用户旧个人冻结奖位被迁写", err)
			}
		}
	}
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", pool.ID).Update("source", constUtil.TurntableScreenshotPartialSource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=3", pool.ID).Updates(map[string]any{"resource_id": "pet_051", "amount": 2, "stock": 7}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=4", pool.ID).Updates(map[string]any{"resource_id": "item_008", "amount": 7}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeTurntable(f.ctx); err != nil {
		t.Fatal(err)
	}
	migrated = db.GameplayPoolReward{}
	if err := db.GetIns().First(&migrated, "pool_id=? AND id=4", pool.ID).Error; err != nil || migrated.ResourceID != "item_008" || migrated.Amount != 7 {
		t.Fatal("启动迁移覆写人工奖位", err)
	}
	migrated = db.GameplayPoolReward{}
	if err := db.GetIns().First(&migrated, "pool_id=? AND id=3", pool.ID).Error; err != nil || migrated.ResourceID != "pet_051" || migrated.Amount != 2 || migrated.Stock != 7 {
		t.Fatal("用户暗宠决定覆写既有人工精灵奖位", err)
	}
	if err := db.GetIns().Save(&prizes[3]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&prizes[4]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", pool.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Model(&db.GameplayPool{}).Where("id=?", pool.ID).Update("enabled", pool.Enabled) })
	if err := f.s.InitializeTurntable(f.ctx); err != nil {
		t.Fatal(err)
	}
	var after db.GameplayPool
	if err := db.GetIns().First(&after, "id=?", pool.ID).Error; err != nil || after.Enabled {
		t.Fatal("重启覆写运营开关", err)
	}
	var oldAfter, redAfter []db.GameplayPoolReward
	if err := db.GetIns().Where("pool_id=?", "turntable_20261011").Order("id").Find(&oldAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("pool_id=?", constUtil.P2RedWheelPool).Order("id").Find(&redAfter).Error; err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(old)
	b, _ := json.Marshal(oldAfter)
	c, _ := json.Marshal(red)
	d, _ := json.Marshal(redAfter)
	if !bytes.Equal(a, b) || !bytes.Equal(c, d) {
		t.Fatal("旧普通盘或红武盘配置被覆盖")
	}
	// 原红武选择保持独立，不能被普通截图奖池替换。
	var priorKing db.KingRecruitRules
	if err := db.GetIns().First(&priorKing, 1).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetIns().Save(&priorKing).Error; err != nil {
			t.Error(err)
		}
	})
	if err := db.GetIns().Model(&db.KingRecruitRules{}).Where("id=1").Update("wheel_pool", constUtil.P2RedWheelPool).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "BigTurnTable", "00000000-0000-4000-8000-000000004000"); err != nil {
		t.Fatal(err)
	}
	redCycle := localPlayCycleID(t, f, id, "BigTurnTable")
	var redClone []db.GameplayPoolReward
	if err := db.GetIns().Where("pool_id=?", redCycle).Order("id").Find(&redClone).Error; err != nil {
		t.Fatal(err)
	}
	if len(redClone) != len(red) {
		t.Fatal("红武克隆目录缺失")
	}
	for i := range redClone {
		redClone[i].PoolID = red[i].PoolID
	}
	originalRed, _ := json.Marshal(red)
	clonedRed, _ := json.Marshal(redClone)
	if !bytes.Equal(originalRed, clonedRed) {
		t.Fatal("红武刷新被截图普通盘替换")
	}
	// 此隔离库普通源未直接抽取，个人期用独立ID冻结；验证关闭的旧普通源首次建新源也保持关闭。
	var base db.GameplayPool
	if err := db.GetIns().First(&base, "id=?", "turntable_20261011").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetIns().Save(&base).Error; err != nil {
			t.Error(err)
		}
	})
	for _, table := range []any{&db.GameplayPoolReward{}, &db.TurntableRules{}, &db.TurntableGlobal{}} {
		if err := db.GetIns().Where("pool_id=?", constUtil.TurntableScreenshotPool).Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Delete(&db.GameplayPool{}, "id=?", constUtil.TurntableScreenshotPool).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", base.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeTurntable(f.ctx); err != nil {
		t.Fatal("关闭旧普通源拖累启动", err)
	}
	var disabled db.GameplayPool
	if err := db.GetIns().First(&disabled, "id=?", constUtil.TurntableScreenshotPool).Error; err != nil || disabled.Enabled {
		t.Fatal("新源绕过旧运营关闭", err)
	}
}

func TestTurntableScreenshotRefreshLatestKeepsFrozenPeriodAndColdReceipt(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	localPlayNonRedWheelSource(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "BigTurnTable", "00000000-0000-4000-8000-000000004001"); err != nil {
		t.Fatal(err)
	}
	first := localPlayCycleID(t, f, id, "BigTurnTable")
	var frozen []db.GameplayPoolReward
	if err := db.GetIns().Where("pool_id=?", first).Order("id").Find(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 9 || frozen[0].ResourceID != "item_340" || frozen[1].Amount != 50000 || frozen[6].ResourceID != "MineItem_079" {
		t.Fatal("钱包首次召唤没有使用截图普通源", frozen)
	}
	request := &pb.CSPlayBigTurn{Count: proto.Int32(1)}
	draw, err := f.s.playBigTurn(f.ctx, id, request, 64001)
	if err != nil {
		t.Fatal(err)
	}
	// 有限运营编辑仅用于证明刷新读最新源，旧期仍保持冻结目录。
	if err = db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=1", constUtil.TurntableScreenshotPool).Update("amount", 50001).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=1", constUtil.TurntableScreenshotPool).Update("amount", 50000)
	})
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "BigTurnTable", "00000000-0000-4000-8000-000000004002"); err != nil {
		t.Fatal(err)
	}
	second := localPlayCycleID(t, f, id, "BigTurnTable")
	var oldCoin, newCoin db.GameplayPoolReward
	if err = db.GetIns().First(&oldCoin, "pool_id=? AND id=1", first).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&newCoin, "pool_id=? AND id=1", second).Error; err != nil {
		t.Fatal(err)
	}
	if first == second || oldCoin.Amount != 50000 || newCoin.Amount != 50001 {
		t.Fatal("新期未读最新源或旧冻结被覆盖", oldCoin, newCoin)
	}
	before, game := f.profile(t)
	beforeGame, _ := json.Marshal(game)
	f.restart(t)
	again, err := f.s.playBigTurn(f.ctx, id, request, 64001)
	after, current := f.profile(t)
	afterGame, _ := json.Marshal(current)
	if err != nil || !proto.Equal(again[0], draw[0]) || !proto.Equal(before, after) || !bytes.Equal(beforeGame, afterGame) {
		t.Fatal("刷新后冷重试改变旧奖励或重复扣发", err)
	}
}

// 本条使用独立有限测试期的明确红宠047，不代表用户已选择右上幽梦兽光/暗。
func TestTurntableScreenshotPetConfiguredSourceAtomicColdAndCapacity(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	localPlayNonRedWheelSource(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "BigTurnTable", "00000000-0000-4000-8000-000000004003"); err != nil {
		t.Fatal(err)
	}
	pool := localPlayCycleID(t, f, id, "BigTurnTable")
	// 原独立期提供九个同值红宠奖位且无大奖，仅验证已有P9实例消费者，未改默认奖池或挑选随机结果。
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", pool).Update("jackpot_per_million", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=?", pool).Updates(map[string]any{"resource_id": "pet_047", "amount": 1, "weight": 1, "stock": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.PetBagSize = proto.Int32(1)
		p.Diamond = proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSPlayBigTurn{Count: proto.Int32(1)}
	messages, err := f.s.playBigTurn(f.ctx, id, request, 65001)
	if err != nil {
		t.Fatal(err)
	}
	result := messages[0].(*pb.SCPlayBigTurn)
	if len(result.GetRewards().GetPets()) != 1 || result.GetRewards().GetPets()[0].GetId() != "pet_047" || len(result.GetRewards().GetPets()[0].GetPetEquipList()) != 4 {
		t.Fatal("原P9精灵完整实例没有发出", result)
	}
	pets, err := P9PetList(db.GetIns(), id)
	if err != nil || len(pets) != 1 || !proto.Equal(pets[0], result.GetRewards().GetPets()[0]) {
		t.Fatal("精灵归属/真实实例不符", err)
	}
	before, game := f.profile(t)
	beforeGame, _ := json.Marshal(game)
	var beforeGlobal db.TurntableGlobal
	if err = db.GetIns().First(&beforeGlobal, "pool_id=?", pool).Error; err != nil {
		t.Fatal(err)
	}
	beforeGlobalBytes, _ := json.Marshal(beforeGlobal)
	f.restart(t)
	again, err := f.s.playBigTurn(f.ctx, id, request, 65001)
	if err != nil || !proto.Equal(again[0], result) || p1Count(t, &db.P9PetInstance{}, "player_id=?", id) != 1 {
		t.Fatal("冷回执重复创建精灵", err)
	}
	if _, err = f.s.playBigTurn(f.ctx, id, request, 65002); err == nil {
		t.Fatal("满精灵背包仍抽入实例")
	}
	after, current := f.profile(t)
	afterGame, _ := json.Marshal(current)
	var afterGlobal db.TurntableGlobal
	if err = db.GetIns().First(&afterGlobal, "pool_id=?", pool).Error; err != nil {
		t.Fatal(err)
	}
	afterGlobalBytes, _ := json.Marshal(afterGlobal)
	if !proto.Equal(before, after) || !bytes.Equal(beforeGame, afterGame) || !bytes.Equal(beforeGlobalBytes, afterGlobalBytes) || p1Count(t, &db.P9PetInstance{}, "player_id=?", id) != 1 {
		t.Fatal("满包或回执重试造成扣费/库存/精灵半状态")
	}
}
