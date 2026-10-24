package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// resourceRecovery64Fixture exercises the real resourceProfile transaction with
// optional domains absent. The due energy cycle is never pre-consumed by saveProfile.
func resourceRecovery64Fixture(t *testing.T, migrateCapacity bool) (*GameService, db.Profile, *adminActivityQueryRecorder) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "recovery64.sqlite")); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	now := time.Now().UnixMilli()
	player := &pb.Player{PlayerId: proto.Int64(1), Energy: proto.Int32(10), Stamina: proto.Int32(constUtil.StaminaRecoveryLimit), EnergyLastRefreshTime: proto.Int64(now - constUtil.EnergyRecoveryIntervalMS - 1000), StaminaLastRefreshTime: proto.Int64(now)}
	game := &model.GameState{BagCapacityScale: constUtil.BagCapacityScale}
	if migrateCapacity {
		game.BagCapacityScale = 0
	}
	playerRaw, err := json.Marshal(player)
	if err != nil {
		t.Fatal(err)
	}
	gameRaw, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	playerText, gameText := string(playerRaw), string(gameRaw)
	row := db.Profile{Id: 1, Uid: "recovery64", Session: []byte("recovery64session"), Player: &playerText, Game: &gameText}
	if err := database.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	recorder := &adminActivityQueryRecorder{Interface: database.Config.Logger}
	database.Config.Logger = recorder
	return &GameService{}, row, recorder
}

func resourceRecovery64ProfileWrites(recorder *adminActivityQueryRecorder) []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	var writes []string
	for _, statement := range recorder.sqls {
		if strings.HasPrefix(statement, "UPDATE `profiles`") {
			writes = append(writes, statement)
		}
	}
	return writes
}

func TestResourceRecovery64PlayerOnlyUpdate(t *testing.T) {
	service, before, recorder := resourceRecovery64Fixture(t, false)
	player, _, err := service.resourceProfile(context.Background(), before.Id)
	if err != nil {
		t.Fatal(err)
	}
	if player.GetEnergy() != 11 {
		t.Fatalf("recovered energy=%d", player.GetEnergy())
	}
	writes := resourceRecovery64ProfileWrites(recorder)
	if len(writes) != 1 || !strings.Contains(writes[0], "`player`") || strings.Contains(writes[0], "`game`") {
		t.Fatalf("expected one player-only UPDATE, got %v", writes)
	}
	var saved db.Profile
	if err := db.GetIns().First(&saved, before.Id).Error; err != nil {
		t.Fatal(err)
	}
	if *saved.Game != *before.Game || *saved.Player == *before.Player {
		t.Fatal("recovery changed game bytes or failed to persist player")
	}
	// A second read must retain the same recovery cursor without another write.
	recorder.reset()
	again, _, err := service.resourceProfile(context.Background(), before.Id)
	if err != nil {
		t.Fatal(err)
	}
	if again.GetEnergy() != 11 || again.GetEnergyLastRefreshTime() != player.GetEnergyLastRefreshTime() || len(resourceRecovery64ProfileWrites(recorder)) != 0 {
		t.Fatal("repeated read advanced recovery or rewrote unchanged profile")
	}
}

func TestResourceRecovery64ChangedGameStillPersists(t *testing.T) {
	service, before, recorder := resourceRecovery64Fixture(t, true)
	player, game, err := service.resourceProfile(context.Background(), before.Id)
	if err != nil {
		t.Fatal(err)
	}
	if player.GetEnergy() != 11 || game.BagCapacityScale != constUtil.BagCapacityScale {
		t.Fatal("recovery or legacy capacity migration missing")
	}
	writes := resourceRecovery64ProfileWrites(recorder)
	if len(writes) != 1 || !strings.Contains(writes[0], "`player`") || !strings.Contains(writes[0], "`game`") {
		t.Fatalf("expected both changed columns, got %v", writes)
	}
	var saved db.Profile
	if err := db.GetIns().First(&saved, before.Id).Error; err != nil {
		t.Fatal(err)
	}
	var stored model.GameState
	if err := json.Unmarshal([]byte(*saved.Game), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.BagCapacityScale != constUtil.BagCapacityScale || *saved.Game == *before.Game {
		t.Fatal("changed game was not persisted")
	}
}

func TestResourceRecovery64UpdateFailureRollsBack(t *testing.T) {
	service, before, _ := resourceRecovery64Fixture(t, true)
	database := db.GetIns()
	if err := database.Exec("CREATE TRIGGER recovery64_abort BEFORE UPDATE ON profiles BEGIN SELECT RAISE(ABORT, 'recovery64 forced failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.resourceProfile(context.Background(), before.Id); err == nil || !strings.Contains(err.Error(), "recovery64 forced failure") {
		t.Fatalf("expected forced persistence failure, got %v", err)
	}
	var failed db.Profile
	if err := database.First(&failed, before.Id).Error; err != nil {
		t.Fatal(err)
	}
	if *failed.Player != *before.Player || *failed.Game != *before.Game {
		t.Fatal("failed transaction persisted recovery cursor or migration")
	}
	if err := database.Exec("DROP TRIGGER recovery64_abort").Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := service.resourceProfile(context.Background(), before.Id)
	if err != nil {
		t.Fatal(err)
	}
	if player.GetEnergy() != 11 || game.BagCapacityScale != constUtil.BagCapacityScale {
		t.Fatal("retry lost recovery cycle or game migration")
	}
}
