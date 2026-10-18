package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func p12WorldBossSetup(t *testing.T) (*p1Fixture, int64) {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p12.db")}
	if name := os.Getenv("HYJS_P12_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p12_") {
			t.Fatal("P12 isolation guard")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P12_MYSQL_HOST"), User: os.Getenv("HYJS_P12_MYSQL_USER"), Password: os.Getenv("HYJS_P12_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP12WorldBossTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for index := 0; index < 2; index++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p12-%s-%d-%d", t.Name(), index, time.Now().UnixNano()), "local-isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("真实首领查询%d", index), "hero_047"); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	f.id = ids[0]
	if err := f.s.InitializeP12WorldBoss(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P12WorldBossCycle{}, &db.P12WorldBossPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP12WorldBoss(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, ids[1]
}
func p12WorldBossCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, now time.Time) proto.Message {
	t.Helper()
	out, err := f.s.p12WorldBossAt(f.ctx, id, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return out[0]
}
func p12WorldBossDay() time.Time {
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 0, -2)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}
func TestP12WorldBossOriginalDefinitionsConcurrentRealCycleAndRestart(t *testing.T) {
	f, b := p12WorldBossSetup(t)
	now := p12WorldBossDay().Add(12*time.Hour + 30*time.Minute)
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossInfo{}, now); err == nil {
		t.Fatal("default world boss query open")
	}
	var cycles int64
	db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&cycles)
	if cycles != 0 {
		t.Fatal("disabled query created instances")
	}
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan *pb.SCWorldBossInfo, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			out, err := f.s.p12WorldBossAt(f.ctx, id, &pb.CSWorldBossInfo{}, now)
			errs <- err
			if err == nil {
				replies <- out[0].(*pb.SCWorldBossInfo)
			}
		}([]int64{f.id, b}[i%2])
	}
	wg.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var original *pb.SCWorldBossInfo
	for r := range replies {
		if original == nil {
			original = r
		}
		if !proto.Equal(original, r) {
			t.Fatal("one calendar window yielded different instances")
		}
	}
	info := original.GetWorldBossShowInfo()
	preview := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossReward{}, now).(*pb.SCWorldBossReward)
	if len(preview.RewardInfos) != 6 || preview.RewardInfos[0].Rewards[0].GetId() != "coin" || preview.RewardInfos[0].Rewards[0].GetNum() != 10000 {
		t.Fatal("original level one six-tier preview", preview)
	}
	if info.GetBossState() != 1 || info.GetBossLevel() != 1 || info.GetBossPicPath() != "monster_041" || info.GetScene() != "field_002" || info.GetBossuuid() == "" || info.GetLeftRefreshTime() != p12WorldBossDay().Add(21*time.Hour+40*time.Minute).UnixMilli() || len(info.Props) != 4 {
		t.Fatal("original metadata/current absolute next refresh", info)
	}
	for _, prop := range info.Props {
		if prop.GetCost() != "diamond,150" {
			t.Fatal("original price")
		}
	}
	db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&cycles)
	if cycles != 1 {
		t.Fatal("concurrent window duplicated", cycles)
	}
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Updates(map[string]any{"level": 2, "position_id": 2}).Error; err != nil {
		t.Fatal(err)
	}
	restart := &GameService{}
	if err := restart.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restart.LoadP12WorldBossTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restart.InitializeP12WorldBoss(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s = restart
	if !proto.Equal(original, p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, now)) {
		t.Fatal("same actual definition changed on restart/level policy")
	}
	if !proto.Equal(preview, p12WorldBossCall(t, f, b, &pb.CSWorldBossReward{}, now)) {
		t.Fatal("reward preview followed future policy instead of frozen instance")
	}
	rank := p12WorldBossCall(t, f, b, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	if len(rank.DamageInfos) != 0 || rank.MyDamageInfo != nil || rank.LastDamageInfo != nil || rank.GetBossHpPercent() != 1 {
		t.Fatal("query manufactured damage/ranks", rank)
	}
	for _, request := range []proto.Message{&pb.CSWorldBossPropPurchase{PropId: proto.String("wordboss_1")}, &pb.CSWorldBossFight{BossId: proto.String(info.GetBossuuid())}} {
		if _, err := f.s.p12WorldBossAt(f.ctx, f.id, request, now); err == nil {
			t.Fatal("unapplied boost or non-native fight awarded")
		}
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("metadata query/closed fight changed original resources")
	}
}

func TestP12WorldBossOriginalAllFiftyLevelsFixedPreviewProbabilityGateAndRollback(t *testing.T) {
	f, _ := p12WorldBossSetup(t)
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	def := p12WorldBossDefinition{Tables: f.s.Tables}
	for level := int64(1); level <= 50; level++ {
		preview, err := p12WorldBossRewardPreview(def, level)
		if level >= 30 {
			if err == nil {
				t.Fatal("probability pool presented as guaranteed rewards", level)
			}
			continue
		}
		if err != nil || len(preview.GetRewardInfos()) != 6 {
			t.Fatalf("original fixed level %d incomplete: %v", level, err)
		}
		for kind, row := range preview.RewardInfos {
			if row.GetType() != int32(kind) || len(row.Rewards) == 0 {
				t.Fatal("original type ordering/attachment missing", level, row)
			}
		}
	}
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := p12WorldBossDay().Add(12 * time.Hour)
	for index := 0; index < 3; index++ {
		p12WorldBossCall(t, f, f.id, &pb.CSWorldBossReward{}, now)
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("repeated preview granted resources")
	}
	var cfg db.P12WorldBossPolicy
	if err := db.GetIns().First(&cfg, 1).Error; err != nil {
		t.Fatal(err)
	}
	// 保留合法hash但破坏完整档数，下一周期查询必须整笔回滚，不能提交新实例。
	for _, mode := range []string{"missing", "duplicate", "invalid-resource"} {
		var corrupt p12WorldBossDefinition
		json.Unmarshal([]byte(cfg.DefinitionJSON), &corrupt)
		switch mode {
		case "missing":
			corrupt.Tables["WorldBossReward"] = corrupt.Tables["WorldBossReward"][1:]
		case "duplicate":
			corrupt.Tables["WorldBossReward"] = append(corrupt.Tables["WorldBossReward"], corrupt.Tables["WorldBossReward"][0])
		case "invalid-resource":
			corrupt.Tables["WorldBossReward"][0]["reward"] = "coin,-1"
		}
		raw, err := json.Marshal(corrupt)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Updates(map[string]any{"definition_json": string(raw), "definition_hash": p6Digest(raw)}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossReward{}, now.Add(24*time.Hour)); err == nil {
			t.Fatal("invalid six-tier snapshot accepted", mode)
		}
		var count int64
		db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&count)
		if count != 1 {
			t.Fatal("invalid preview committed new window", mode, count)
		}
	}
	// 旧定义没有明确的仅ID投影来源，新增能力不得把旧概率档自动打开。
	var legacy p12WorldBossDefinition
	if err := json.Unmarshal([]byte(cfg.DefinitionJSON), &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.WeightedRewardPreviewSource = ""
	legacyRaw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Updates(map[string]any{"definition_json": string(legacyRaw), "definition_hash": p6Digest(legacyRaw), "level": 30}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossReward{}, now.Add(24*time.Hour)); err == nil {
		t.Fatal("probability preview was accepted by original request")
	}
	var count int64
	db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&count)
	if count != 1 {
		t.Fatal("probability preview committed unconfirmed new window", count)
	}
}
func TestP12WorldBossAbsoluteOriginalWindowsScheduledActiveEndedAndNoDuplicate(t *testing.T) {
	f, _ := p12WorldBossSetup(t)
	day := p12WorldBossDay()
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	before := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, day.Add(11*time.Hour+59*time.Minute)).(*pb.SCWorldBossInfo).WorldBossShowInfo
	if before.GetBossState() != 0 || before.GetLeftRefreshTime() != day.Add(12*time.Hour).UnixMilli() {
		t.Fatal("pre-start absolute clock", before)
	}
	active := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, day.Add(12*time.Hour)).(*pb.SCWorldBossInfo).WorldBossShowInfo
	if active.GetBossuuid() != before.GetBossuuid() || active.GetBossState() != 1 {
		t.Fatal("planned UUID changed at birth window")
	}
	next := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, day.Add(13*time.Hour)).(*pb.SCWorldBossInfo).WorldBossShowInfo
	if next.GetBossuuid() == before.GetBossuuid() || next.GetBossState() != 0 || next.GetLeftRefreshTime() != day.Add(21*time.Hour+40*time.Minute).UnixMilli() {
		t.Fatal("end boundary became fake kill or missed next window", next)
	}
	var ended db.P12WorldBossCycle
	db.GetIns().First(&ended, "id=?", before.GetBossuuid())
	if ended.State != "ended" || ended.EndedAt != day.Add(13*time.Hour).UnixMilli() {
		t.Fatal("expired instance terminal state", ended)
	}
	evening := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, day.Add(21*time.Hour+40*time.Minute)).(*pb.SCWorldBossInfo).WorldBossShowInfo
	if evening.GetBossuuid() != next.GetBossuuid() || evening.GetBossState() != 1 {
		t.Fatal("second original window changed")
	}
	tomorrow := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, day.Add(22*time.Hour+40*time.Minute)).(*pb.SCWorldBossInfo).WorldBossShowInfo
	if tomorrow.GetLeftRefreshTime() != day.Add(36*time.Hour).UnixMilli() || tomorrow.GetBossState() != 0 {
		t.Fatal("next original day", tomorrow)
	}
	var count int64
	db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&count)
	if count != 3 {
		t.Fatal("calendar windows cloned", count)
	}
}
func TestP12WorldBossCorruptDefinitionRollsBackWithoutNewCycle(t *testing.T) {
	f, _ := p12WorldBossSetup(t)
	now := p12WorldBossDay().Add(12 * time.Hour)
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	info := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, now).(*pb.SCWorldBossInfo).WorldBossShowInfo
	var original db.P12WorldBossCycle
	db.GetIns().First(&original, "id=?", info.GetBossuuid())
	t.Cleanup(func() {
		db.GetIns().Model(&db.P12WorldBossCycle{}).Where("id=?", original.ID).Update("definition_hash", original.DefinitionHash)
	})
	if err := db.GetIns().Model(&db.P12WorldBossCycle{}).Where("id=?", original.ID).Update("definition_hash", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossInfo{}, now); err == nil {
		t.Fatal("corrupt stored actor/config snapshot accepted")
	}
	var count int64
	db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&count)
	if count != 1 {
		t.Fatal("corrupt recovery created substitute boss")
	}
}
