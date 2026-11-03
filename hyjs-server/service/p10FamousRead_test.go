package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10FamousTestServer(t *testing.T, f *p1Fixture, ids ...int64) int64 {
	t.Helper()
	serverID := int64(100000) + f.id
	if err := db.GetIns().Model(&db.Profile{}).Where("id IN ?", ids).Update("server_id", serverID).Error; err != nil {
		t.Fatal(err)
	}
	return serverID
}

func p10FamousTestRead(t *testing.T, f *p1Fixture, ctx context.Context, id int64, kind int32, now time.Time) *pb.SCFamous {
	t.Helper()
	out, err := f.s.p10Worship(ctx, id, &pb.CSFamous{Type: proto.Int32(kind)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	return out[0].(*pb.SCFamous)
}

func p10FamousTestExpire(t *testing.T, s *GameService) {
	t.Helper()
	cache := &s.p10ClientOwner().famousRankCache
	cache.mu.Lock()
	hasSnapshot := cache.snapshot != nil
	if hasSnapshot {
		cache.snapshot.ExpiresAt = time.Now().Add(-time.Second)
	}
	cache.mu.Unlock()
	if !hasSnapshot {
		t.Fatal("expected a populated public rank cache")
	}
}

func p10FamousTraceStatements(recorder *adminActivityQueryRecorder) []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]string(nil), recorder.sqls...)
}

func TestP10FamousRefreshAndPersonalWorship(t *testing.T) {
	f, ids := p10WorshipSetup(t)
	serverID := p10FamousTestServer(t, f, append([]int64{f.id}, ids...)...)
	if err := db.GetIns().Model(&db.P10WorshipPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	foreign, err := LoginAccount(f.ctx, fmt.Sprintf("p10-foreign-%s-%d", t.Name(), time.Now().UnixNano()), "isolated-local-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, foreign.Id, "跨服角色", "hero_047"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", foreign.Id).Update("server_id", serverID+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, foreign.Id, func(player *pb.Player, game *model.GameState) error {
		player.Level = proto.Int32(99)
		game.Warriors[0].Level = proto.Int32(90)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	oldLevel := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	oldPower := p10FamousTestRead(t, f, f.ctx, f.id, 0, now)
	if len(oldLevel.GetPlayerList()) != 3 || len(oldPower.GetPlayerList()) != 3 || oldLevel.GetPlayerList()[0].GetPlayerId() != ids[0] || oldPower.GetPlayerList()[0].GetPlayerId() != ids[0] || oldLevel.GetLeftAddGood() != 3 {
		t.Fatal("initial same-server ranks", oldLevel, oldPower)
	}
	if err := updateProfile(f.ctx, ids[3], func(player *pb.Player, game *model.GameState) error {
		player.Level = proto.Int32(95)
		player.FightPower = proto.Int32(1)
		game.Warriors[0].Level = proto.Int32(80)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stale := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	if !proto.Equal(oldLevel, stale) {
		t.Fatal("public list changed before cache expiry", oldLevel, stale)
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(ids[2])}, 730, now); err == nil {
		t.Fatal("stale cached target still qualified for worship")
	}
	// The newly qualified target is absent from the still-cached public list.
	if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(ids[3])}, 731, now); err != nil {
		t.Fatalf("live eligible target rejected while public list was cached: %v", err)
	}
	afterGood := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	if afterGood.GetLeftAddGood() != 2 || len(afterGood.GetGoodList()) != 1 || afterGood.GetGoodList()[0] != ids[3] || afterGood.GetPlayerList()[0].GetPlayerId() != ids[0] {
		t.Fatal("personal worship fact did not refresh independently", afterGood)
	}
	p10FamousTestExpire(t, f.s)
	updated := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	updatedPower := p10FamousTestRead(t, f, f.ctx, f.id, 0, now)
	if updated.GetPlayerList()[0].GetPlayerId() != ids[3] || updated.GetPlayerList()[0].GetLevel() != 95 || updatedPower.GetPlayerList()[0].GetPlayerId() != ids[3] || updatedPower.GetPlayerList()[0].GetFightPower() <= oldPower.GetPlayerList()[0].GetFightPower() || updatedPower.GetPlayerList()[0].GetFightPower() == 1 {
		t.Fatal("expiry did not publish current level and computed power", updated, updatedPower)
	}
	for _, rank := range []*pb.SCFamous{updated, updatedPower} {
		for _, entry := range rank.GetPlayerList() {
			if entry.GetPlayerId() == foreign.Id || entry.GetServerId() != int32(serverID) {
				t.Fatal("foreign server entered local rank", entry)
			}
		}
	}
	updated.PlayerList[0].Level = proto.Int32(1)
	updated.PlayerList[0].PlayerName = proto.String("mutated-response")
	updated.PlayerList[0].FightPower = proto.Int32(1)
	again := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	againPower := p10FamousTestRead(t, f, f.ctx, f.id, 0, now)
	if again.GetPlayerList()[0].GetLevel() != 95 || again.GetPlayerList()[0].GetPlayerName() == "mutated-response" || againPower.GetPlayerList()[0].GetFightPower() <= 1 || again.GetLeftAddGood() != 2 || len(again.GetGoodList()) != 1 || again.GetGoodList()[0] != ids[3] {
		t.Fatal("response mutation affected cached public data or personal facts", again, againPower)
	}
	foreignRank := p10FamousTestRead(t, f, f.ctx, foreign.Id, 1, now)
	if len(foreignRank.GetPlayerList()) != 1 || foreignRank.GetPlayerList()[0].GetPlayerId() != foreign.Id {
		t.Fatal("foreign server used another server's cached ranks", foreignRank)
	}
}

func TestP10FamousMySQLReadUnderCallerWriteLock(t *testing.T) {
	if os.Getenv("HYJS_P10_MYSQL_DATABASE") == "" {
		t.Skip("requires isolated HYJS_P10_MYSQL_DATABASE")
	}
	f, ids := p10WorshipSetup(t)
	p10FamousTestServer(t, f, append([]int64{f.id}, ids...)...)
	now := time.Now()
	old := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	if len(old.GetPlayerList()) != 3 || old.GetPlayerList()[0].GetPlayerId() != ids[0] {
		t.Fatal("initial committed rank", old)
	}
	var caller db.Profile
	if err := db.GetIns().Select("id", "player", "game").First(&caller, f.id).Error; err != nil || caller.Player == nil || caller.Game == nil {
		t.Fatal("caller fixture", err)
	}
	var player pb.Player
	var game model.GameState
	if err := json.Unmarshal([]byte(*caller.Player), &player); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*caller.Game), &game); err != nil || len(game.Warriors) == 0 {
		t.Fatal("caller game fixture", err)
	}
	player.Level = proto.Int32(95)
	player.FightPower = proto.Int32(1)
	game.Warriors[0].Level = proto.Int32(80)
	playerJSON, err := json.Marshal(&player)
	if err != nil {
		t.Fatal(err)
	}
	gameJSON, err := model.MarshalProfileGame86(&game)
	if err != nil {
		t.Fatal(err)
	}
	tx := db.GetIns()
	writer := tx.Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	t.Cleanup(func() { writer.Rollback() })
	if err := writer.Model(&db.Profile{}).Where("id=?", f.id).Updates(map[string]any{"player": string(playerJSON), "game": string(gameJSON)}).Error; err != nil {
		t.Fatal(err)
	}
	p10FamousTestExpire(t, f.s)
	recorder := &adminActivityQueryRecorder{Interface: tx.Config.Logger}
	oldLogger := tx.Config.Logger
	tx.Config.Logger = recorder
	defer func() { tx.Config.Logger = oldLogger }()
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	underLock := p10FamousTestRead(t, f, ctx, f.id, 1, now)
	if len(underLock.GetPlayerList()) != 3 || underLock.GetPlayerList()[0].GetPlayerId() != ids[0] || !proto.Equal(old, underLock) {
		t.Fatal("famous rank read uncommitted caller data", underLock)
	}
	for _, query := range p10FamousTraceStatements(recorder) {
		if strings.Contains(strings.ToLower(query), "for update") {
			t.Fatalf("famous read locked a row: %s", query)
		}
	}
	if err := writer.Commit().Error; err != nil {
		t.Fatal(err)
	}
	p10FamousTestExpire(t, f.s)
	updated := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	updatedPower := p10FamousTestRead(t, f, f.ctx, f.id, 0, now)
	if updated.GetPlayerList()[0].GetPlayerId() != f.id || updated.GetPlayerList()[0].GetLevel() != 95 || updatedPower.GetPlayerList()[0].GetPlayerId() != f.id || updatedPower.GetPlayerList()[0].GetFightPower() <= 1 {
		t.Fatal("expiry did not show newly committed player values", updated, updatedPower)
	}
}

func TestP10FamousColdAndWarmQueriesUseCurrentPower(t *testing.T) {
	f, ids := p10WorshipSetup(t)
	serverID := p10FamousTestServer(t, f, f.id)
	var template db.Profile
	if err := db.GetIns().Select("id", "player", "game").First(&template, ids[0]).Error; err != nil || template.Player == nil || template.Game == nil {
		t.Fatal("real role template", err)
	}
	var templatePlayer pb.Player
	var templateGame model.GameState
	if err := json.Unmarshal([]byte(*template.Player), &templatePlayer); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*template.Game), &templateGame); err != nil || len(templateGame.Warriors) == 0 {
		t.Fatal("real formation template", err)
	}
	gameJSON := make(map[int32]string)
	for _, level := range []int32{1, 70, 80} {
		templateGame.Warriors[0].Level = proto.Int32(level)
		raw, err := model.MarshalProfileGame86(&templateGame)
		if err != nil {
			t.Fatal(err)
		}
		gameJSON[level] = string(raw)
	}
	profiles := make([]db.Profile, 128)
	seed := time.Now().UnixNano()
	for index := range profiles {
		profiles[index] = db.Profile{Uid: fmt.Sprintf("p10-famous-%d-%d", seed, index), ServerID: serverID, Session: []byte{}}
	}
	if err := db.GetIns().CreateInBatches(&profiles, 64).Error; err != nil {
		t.Fatal(err)
	}
	for index, profile := range profiles {
		player := proto.Clone(&templatePlayer).(*pb.Player)
		player.PlayerId = proto.Int64(profile.Id)
		player.Name = proto.String(fmt.Sprintf("排行角色%03d", index))
		player.Level = proto.Int32(80)
		player.FightPower = proto.Int32(0)
		warriorLevel := int32(70)
		if index == 0 {
			player.Level, player.FightPower, warriorLevel = proto.Int32(99), proto.Int32(1), 80
		} else if index == 1 {
			player.Level, player.FightPower, warriorLevel = proto.Int32(98), proto.Int32(2000000000), 1
		}
		raw, err := json.Marshal(player)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Updates(map[string]any{"player": string(raw), "game": gameJSON[warriorLevel]}).Error; err != nil {
			t.Fatal(err)
		}
	}
	tx := db.GetIns()
	recorder := &adminActivityQueryRecorder{Interface: tx.Config.Logger}
	oldLogger := tx.Config.Logger
	tx.Config.Logger = recorder
	defer func() { tx.Config.Logger = oldLogger }()
	now := time.Now()
	coldStart := time.Now()
	power := p10FamousTestRead(t, f, f.ctx, f.id, 0, now)
	coldElapsed := time.Since(coldStart)
	coldSQL := p10FamousTraceStatements(recorder)
	if len(coldSQL) > 40 {
		t.Fatalf("cold rank issued %d SQL statements for 129 roles", len(coldSQL))
	}
	// The account main hero uses Player.Level (99, 98, then the ID-ordered
	// level-80 ties), even when its stored Warrior.Level differs. The fake
	// saved power of two billion must not become first.
	if len(power.GetPlayerList()) != 3 || power.GetPlayerList()[0].GetPlayerId() != profiles[0].Id || power.GetPlayerList()[0].GetFightPower() <= 1 || power.GetPlayerList()[1].GetPlayerId() != profiles[1].Id || power.GetPlayerList()[2].GetPlayerId() != profiles[2].Id {
		t.Fatal("power order followed saved fightPower or lost tie ordering", power)
	}
	for _, query := range coldSQL {
		if strings.Contains(strings.ToLower(query), "for update") {
			t.Fatalf("cold rank acquired a write lock: %s", query)
		}
	}
	recorder.reset()
	warmStart := time.Now()
	level := p10FamousTestRead(t, f, f.ctx, f.id, 1, now)
	warmElapsed := time.Since(warmStart)
	warmSQL := p10FamousTraceStatements(recorder)
	if len(level.GetPlayerList()) != 3 || level.GetPlayerList()[0].GetPlayerId() != profiles[0].Id || level.GetPlayerList()[0].GetLevel() != 99 || level.GetPlayerList()[1].GetPlayerId() != profiles[1].Id || level.GetPlayerList()[1].GetLevel() != 98 || level.GetPlayerList()[2].GetPlayerId() != profiles[2].Id {
		t.Fatal("current level order or tie ordering", level)
	}
	profileReads := 0
	for _, query := range warmSQL {
		lower := strings.ToLower(query)
		if strings.Contains(lower, "for update") {
			t.Fatalf("warm rank acquired a write lock: %s", query)
		}
		if index := strings.Index(lower, "from `profiles`"); index >= 0 {
			profileReads++
			selection := lower[:index]
			if strings.Contains(selection, "select *") || strings.Contains(selection, "`player`") || strings.Contains(selection, "`game`") {
				t.Fatalf("warm rank reloaded a full profile: %s", query)
			}
		}
	}
	if profileReads != 1 || len(warmSQL) >= len(coldSQL) {
		t.Fatalf("warm rank reloaded candidates: profile reads=%d, SQL=%d, cold SQL=%d", profileReads, len(warmSQL), len(coldSQL))
	}
	t.Logf("129 roles: cold queries=%d elapsed=%s, warm queries=%d elapsed=%s", len(coldSQL), coldElapsed, len(warmSQL), warmElapsed)
}
