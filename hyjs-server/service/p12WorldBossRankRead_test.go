package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p12WorldBossRankReadSetup(t *testing.T) (*p1Fixture, int64, string, time.Time) {
	t.Helper()
	f, other := p12WorldBossSetup(t)
	p12WorldBossRankResetBattlePolicy(t)
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	day := p12WorldBossDay()
	cycleID := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, day.Add(11*time.Hour+59*time.Minute)).(*pb.SCWorldBossInfo).GetWorldBossShowInfo().GetBossuuid()
	if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "rewards_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	return f, other, cycleID, day.Add(12*time.Hour + 10*time.Minute)
}

func p12WorldBossRankResetBattlePolicy(t *testing.T) {
	t.Helper()
	tx := db.GetIns()
	var policy db.P12WorldBossBattlePolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Updates(map[string]any{"enabled": policy.Enabled, "rewards_enabled": policy.RewardsEnabled}).Error; err != nil {
			t.Error(err)
		}
	})
	if err := tx.Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "rewards_enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
}

func p12WorldBossRankReceipt(fightID, instanceID string, playerID, damage int64, committed bool) db.P12WorldBossBattleReceipt {
	return db.P12WorldBossBattleReceipt{
		FightID: fightID, InstanceID: instanceID, PlayerID: playerID, AppliedDamage: damage,
		Committed: committed, StartProto: []byte{},
	}
}

func TestP12WorldBossRankReadOnlyWindowAndCommittedDamage(t *testing.T) {
	f, other := p12WorldBossSetup(t)
	p12WorldBossRankResetBattlePolicy(t)
	now := p12WorldBossDay().Add(12*time.Hour + 10*time.Minute)
	if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossRank{}, now); err == nil {
		t.Fatal("disabled policy allowed rank")
	}
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	missing := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	if missing.GetState() != 0 || missing.GetBossHpPercent() != 1 || len(missing.GetDamageInfos()) != 0 {
		t.Fatal("missing cycle must remain an empty scheduled view", missing)
	}
	var count int64
	if err := db.GetIns().Model(&db.P12WorldBossCycle{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rank created a cycle", count, err)
	}
	cycleID := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, p12WorldBossDay().Add(11*time.Hour+59*time.Minute)).(*pb.SCWorldBossInfo).GetWorldBossShowInfo().GetBossuuid()
	if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "rewards_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	beforeStart := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, p12WorldBossDay().Add(11*time.Hour+59*time.Minute)).(*pb.SCWorldBossRank)
	noInstance := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	if beforeStart.GetState() != 0 || noInstance.GetState() != 1 || noInstance.GetBossHpPercent() != 1 {
		t.Fatal("scheduled or missing-instance state", beforeStart, noInstance)
	}
	if err := db.GetIns().Model(&db.P12WorldBossBattleInstance{}).Where("id=?", cycleID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rank created a battle instance", count, err)
	}
	instance := db.P12WorldBossBattleInstance{ID: cycleID, TotalHP: 1000, CurrentHP: 0, KillerID: other, KilledAt: now.UnixMilli(), PolicyName: constUtil.P12WorldBossBattlePolicy}
	if err := db.GetIns().Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	receipts := []db.P12WorldBossBattleReceipt{
		p12WorldBossRankReceipt(cycleID+"-a", cycleID, f.id, 60, true),
		p12WorldBossRankReceipt(cycleID+"-b", cycleID, f.id, 40, true),
		p12WorldBossRankReceipt(cycleID+"-c", cycleID, other, 100, true),
		p12WorldBossRankReceipt(cycleID+"-negative", cycleID, f.id, -50, true),
		p12WorldBossRankReceipt(cycleID+"-pending", cycleID, other, 1000, false),
		p12WorldBossRankReceipt(cycleID+"-foreign", "another-cycle", other, 1000, true),
	}
	if err := db.GetIns().Create(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	var beforeCycle, afterCycle db.P12WorldBossCycle
	var beforeInstance, afterInstance db.P12WorldBossBattleInstance
	var beforeProfiles, afterProfiles []db.Profile
	var beforeReceipts, afterReceipts []db.P12WorldBossBattleReceipt
	var beforeAwards, afterAwards int64
	readSnapshot := func(cycle *db.P12WorldBossCycle, boss *db.P12WorldBossBattleInstance, profiles *[]db.Profile, battles *[]db.P12WorldBossBattleReceipt, awards *int64) {
		t.Helper()
		tx := db.GetIns()
		for _, err := range []error{
			tx.First(cycle, "id=?", cycleID).Error,
			tx.First(boss, "id=?", cycleID).Error,
			tx.Where("id IN ?", []int64{f.id, other}).Order("id").Find(profiles).Error,
			tx.Where("fight_id IN ?", []string{cycleID + "-a", cycleID + "-b", cycleID + "-c", cycleID + "-negative", cycleID + "-pending", cycleID + "-foreign"}).Order("fight_id").Find(battles).Error,
			tx.Model(&db.P12WorldBossAward{}).Where("instance_id=?", cycleID).Count(awards).Error,
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	readSnapshot(&beforeCycle, &beforeInstance, &beforeProfiles, &beforeReceipts, &beforeAwards)
	tx := db.GetIns()
	trace := &playerRank48Trace{Interface: tx.Config.Logger}
	oldLogger := tx.Config.Logger
	tx.Config.Logger = trace
	rank := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	cutoff := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, p12WorldBossDay().Add(13*time.Hour)).(*pb.SCWorldBossRank)
	tx.Config.Logger = oldLogger
	if len(rank.GetDamageInfos()) != 2 || rank.GetState() != 2 || rank.GetBossHpPercent() != 0 || rank.GetDamageInfos()[0].GetDamage() != 100 || rank.GetDamageInfos()[1].GetDamage() != 100 || rank.GetDamageInfos()[0].GetPlayerName() != "真实首领查询0" || rank.GetDamageInfos()[1].GetPlayerName() != "真实首领查询1" || rank.GetMyDamageInfo().GetRank() != 1 || rank.GetLastDamageInfo().GetRank() != 2 {
		t.Fatal("committed damage, tie, own rank or killer incorrect", rank)
	}
	if cutoff.GetState() != 0 || cutoff.GetBossHpPercent() != 1 || len(cutoff.GetDamageInfos()) != 0 {
		t.Fatal("expired window still displayed", cutoff)
	}
	for _, query := range trace.sql {
		lower := strings.ToLower(strings.TrimSpace(query))
		if strings.Contains(lower, "for update") || strings.HasPrefix(lower, "update ") || strings.HasPrefix(lower, "insert ") || strings.HasPrefix(lower, "delete ") {
			t.Fatalf("rank acquired a write lock or wrote data: %s", query)
		}
		if strings.Contains(lower, "from `profiles`") && (strings.Contains(lower, "select *") || strings.Contains(lower, "`game`") || strings.Contains(lower, "json_extract(game")) {
			t.Fatalf("rank fetched a complete profile: %s", query)
		}
	}
	lockedRank, err := p12WorldBossBattleRank(db.GetIns(), cycleID, f.id)
	if err != nil || !proto.Equal(rank, lockedRank) {
		t.Fatal("read and battle rank projections disagree", err)
	}
	readSnapshot(&afterCycle, &afterInstance, &afterProfiles, &afterReceipts, &afterAwards)
	if !reflect.DeepEqual(beforeCycle, afterCycle) || !reflect.DeepEqual(beforeInstance, afterInstance) || !reflect.DeepEqual(beforeProfiles, afterProfiles) || !reflect.DeepEqual(beforeReceipts, afterReceipts) || beforeAwards != afterAwards || beforeCycle.State != constUtil.P12WorldBossScheduled {
		t.Fatal("rank changed a cycle, instance, profile, receipt or award")
	}
}

func TestP12WorldBossRankReadRejectsInvalidOwnershipAndSnapshots(t *testing.T) {
	f, _, cycleID, now := p12WorldBossRankReadSetup(t)
	tx := db.GetIns()
	var caller db.Profile
	var policy db.P12WorldBossPolicy
	var cycle db.P12WorldBossCycle
	if err := tx.First(&caller, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if caller.Player == nil {
		t.Fatal("fixture has no player")
	}
	if err := tx.First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&cycle, "id=?", cycleID).Error; err != nil {
		t.Fatal(err)
	}
	var player pb.Player
	if err := json.Unmarshal([]byte(*caller.Player), &player); err != nil {
		t.Fatal(err)
	}
	player.PlayerId = proto.Int64(f.id + 1000000)
	wrongOwner, err := json.Marshal(&player)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{nil, string(wrongOwner), "{"} {
		if err := tx.Model(&db.Profile{}).Where("id=?", f.id).Update("player", value).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossRank{}, now); err == nil {
			t.Fatal("rank accepted missing, malformed, or foreign player identity")
		}
	}
	if err := tx.Model(&db.Profile{}).Where("id=?", f.id).Update("player", *caller.Player).Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		column string
		bad    any
		good   any
	}{
		{"source", "source", "", policy.Source},
		{"hash", "definition_hash", "invalid", policy.DefinitionHash},
		{"level", "level", 0, policy.Level},
		{"position", "position_id", 0, policy.PositionID},
	} {
		if err := tx.Model(&db.P12WorldBossPolicy{}).Where("id=1").Update(test.column, test.bad).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossRank{}, now); err == nil {
			t.Fatalf("rank accepted invalid policy %s", test.name)
		}
		if err := tx.Model(&db.P12WorldBossPolicy{}).Where("id=1").Update(test.column, test.good).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name   string
		column string
		bad    any
		good   any
	}{
		{"start", "starts_at", cycle.StartsAt + 1, cycle.StartsAt},
		{"end", "ends_at", cycle.EndsAt + 1, cycle.EndsAt},
		{"hash", "definition_hash", "invalid", cycle.DefinitionHash},
		{"level projection", "level", 999, cycle.Level},
		{"position projection", "position_id", 999, cycle.PositionID},
	} {
		if err := tx.Model(&db.P12WorldBossCycle{}).Where("id=?", cycleID).Update(test.column, test.bad).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p12WorldBossAt(f.ctx, f.id, &pb.CSWorldBossRank{}, now); err == nil {
			t.Fatalf("rank accepted invalid cycle %s", test.name)
		}
		if err := tx.Model(&db.P12WorldBossCycle{}).Where("id=?", cycleID).Update(test.column, test.good).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestP12WorldBossRankReadIdentityBatches(t *testing.T) {
	f, _, cycleID, now := p12WorldBossRankReadSetup(t)
	instance := db.P12WorldBossBattleInstance{ID: cycleID, TotalHP: 1000, CurrentHP: 743, PolicyName: constUtil.P12WorldBossBattlePolicy}
	if err := db.GetIns().Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	profiles := make([]db.Profile, 257)
	seed := time.Now().UnixNano()
	for index := range profiles {
		player, err := json.Marshal(map[string]string{"name": fmt.Sprintf("batch-%03d", index), "heroWarId": fmt.Sprintf("hero-%03d", index)})
		if err != nil {
			t.Fatal(err)
		}
		profiles[index] = db.Profile{Uid: fmt.Sprintf("p12-rank-%d-%d", seed, index), Session: []byte{}, Player: proto.String(string(player))}
	}
	if err := db.GetIns().CreateInBatches(&profiles, 100).Error; err != nil {
		t.Fatal(err)
	}
	receipts := make([]db.P12WorldBossBattleReceipt, len(profiles))
	for index, profile := range profiles {
		receipts[index] = p12WorldBossRankReceipt(fmt.Sprintf("%s-%03d", cycleID, index), cycleID, profile.Id, int64(len(profiles)-index), true)
	}
	if err := db.GetIns().CreateInBatches(&receipts, 100).Error; err != nil {
		t.Fatal(err)
	}
	tx := db.GetIns()
	trace := &playerRank48Trace{Interface: tx.Config.Logger}
	oldLogger := tx.Config.Logger
	tx.Config.Logger = trace
	rank := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	tx.Config.Logger = oldLogger
	if len(rank.GetDamageInfos()) != 257 || rank.GetDamageInfos()[0].GetPlayerName() != "batch-000" || rank.GetDamageInfos()[256].GetPlayerName() != "batch-256" || rank.GetBossHpPercent() != 0.743 {
		t.Fatal("batched rank result", len(rank.GetDamageInfos()), rank.GetBossHpPercent())
	}
	checkPages := func() {
		t.Helper()
		var pages []int64
		for index, query := range trace.sql {
			lower := strings.ToLower(query)
			if strings.Contains(lower, "from `profiles`") && strings.Contains(lower, "json_extract") {
				if strings.Contains(lower, "for update") || strings.Contains(lower, "json_extract(game") || strings.Contains(lower, "$.level") || strings.Contains(lower, "select *") {
					t.Fatalf("participant lookup read a full profile or locked rows: %s", query)
				}
				pages = append(pages, trace.rows[index])
			}
		}
		if !reflect.DeepEqual(pages, []int64{128, 128, 1}) {
			t.Fatalf("participant query batch sizes: %v", pages)
		}
	}
	checkPages()
	trace.sql, trace.rows = nil, nil
	tx.Config.Logger = trace
	lockedRank, err := p12WorldBossBattleRank(tx, cycleID, f.id)
	tx.Config.Logger = oldLogger
	if err != nil || !proto.Equal(rank, lockedRank) {
		t.Fatal("battle transaction rank differs from read projection", err)
	}
	checkPages()
}

func TestP12WorldBossRankReadMySQLSnapshotUnderWriteLocks(t *testing.T) {
	if os.Getenv("HYJS_P12_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated HYJS_P12_MYSQL_DATABASE")
	}
	f, other, cycleID, now := p12WorldBossRankReadSetup(t)
	instance := db.P12WorldBossBattleInstance{ID: cycleID, TotalHP: 1000, CurrentHP: 900, PolicyName: constUtil.P12WorldBossBattlePolicy}
	receipt := p12WorldBossRankReceipt(cycleID+"-committed", cycleID, f.id, 100, true)
	if err := db.GetIns().Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	var caller db.Profile
	if err := db.GetIns().Select("id", "player").First(&caller, f.id).Error; err != nil || caller.Player == nil {
		t.Fatal("fixture caller", err)
	}
	writer := db.GetIns().Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	t.Cleanup(func() { writer.Rollback() })
	for _, err := range []error{
		writer.Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", false).Error,
		writer.Model(&db.P12WorldBossCycle{}).Where("id=?", cycleID).Update("state", constUtil.P12WorldBossEnded).Error,
		writer.Model(&db.P12WorldBossBattleInstance{}).Where("id=?", cycleID).Updates(map[string]any{"current_hp": 0, "killer_id": other}).Error,
		writer.Model(&db.P12WorldBossBattleReceipt{}).Where("fight_id=?", receipt.FightID).Update("applied_damage", 900).Error,
		writer.Model(&db.Profile{}).Where("id=?", f.id).Update("player", `{"playerId":0,"name":"uncommitted"}`).Error,
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	pending := p12WorldBossRankReceipt(cycleID+"-uncommitted", cycleID, other, 500, true)
	if err := writer.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	out, err := f.s.p12WorldBossAt(ctx, f.id, &pb.CSWorldBossRank{}, now)
	if err != nil {
		t.Fatalf("rank blocked behind held write locks: %v", err)
	}
	rank := out[0].(*pb.SCWorldBossRank)
	if rank.GetState() != 1 || rank.GetBossHpPercent() != 0.9 || len(rank.GetDamageInfos()) != 1 || rank.GetMyDamageInfo().GetDamage() != 100 || rank.GetMyDamageInfo().GetPlayerName() != "真实首领查询0" || rank.GetLastDamageInfo() != nil {
		t.Fatal("rank observed uncommitted writer state", rank)
	}
	for _, err := range []error{
		writer.Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error,
		writer.Model(&db.P12WorldBossCycle{}).Where("id=?", cycleID).Update("state", constUtil.P12WorldBossScheduled).Error,
		writer.Model(&db.Profile{}).Where("id=?", f.id).Update("player", *caller.Player).Error,
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Commit().Error; err != nil {
		t.Fatal(err)
	}
	updated := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	if updated.GetState() != 2 || updated.GetBossHpPercent() != 0 || len(updated.GetDamageInfos()) != 2 || updated.GetMyDamageInfo().GetDamage() != 900 || updated.GetMyDamageInfo().GetRank() != 1 || updated.GetLastDamageInfo().GetDamage() != 500 || updated.GetLastDamageInfo().GetRank() != 2 {
		t.Fatal("rank did not refresh to the newly committed result", updated)
	}
}
