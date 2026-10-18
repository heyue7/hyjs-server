package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestCopyActualDailyPendingTwoRecovery reopens the complete actual paid state.
// It verifies the original committed first source and the unconsumed second
// result independently of the device caller, without generating a winner.
func TestCopyActualDailyPendingTwoRecovery(t *testing.T) {
	if os.Getenv("HYJS_COPY_DAILY_PENDING_RECOVERY_SQLITE") != "1" {
		t.Skip("explicit actual original daily checkpoint recovery")
	}
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "actual-daily-pending-two.db")); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeOpenServerActivity, s.InitializeDailyTasks, s.InitializeCopyDrops, s.InitializeP6, s.InitializeDailyCopy, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	path := "../../analysis/p6/copy-pet-daily-native-20261014/001-start-paid-state.json"
	digest := "34163d4cdc43e9d64b3788ad5f9e892b431b26050691c61b84f18003cda5dc3b"
	restoreCopySourceCheckpoint(t, ctx, s, path, digest)
	var quota db.DailyCopyState
	if err := db.GetIns().First(&quota, "player_id=? AND day=? AND type=?", 1, "2026-10-14", "PetEquip").Error; err != nil || quota.Used != 2 || quota.Limit != 3 {
		t.Fatal("actual paid quota changed", err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, 1).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if simpleOwned(player, game, "item_379") != 10 || simpleOwned(player, game, "item_384") != 5 {
		t.Fatal("actual first winner's source changed")
	}
	raw, err := os.ReadFile("../../analysis/p6/copy-pet-daily-native-20261014/001.pb")
	if err != nil || p6Digest(raw) != "f2497a9f8031b3b2c103a3db1aa1b4d57d267849758f960c6990aa392b0668ed" {
		t.Fatal("actual original playback changed", err)
	}
	var expected pb.SCFightPlayback
	if err := proto.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 14, 13, 45, 0, 0, time.FixedZone("UTC+8", 8*3600))
	resumed, err := s.dailyCopyAt(ctx, 1, &pb.CSDailyCopyFightStart{ChapterId: proto.String("50")}, 1800002500010, now)
	if err != nil {
		t.Fatal(err)
	}
	start := resumed[0].(*pb.SCDailyCopyFightStart)
	actual := &pb.SCFightPlayback{Fightid: start.FightId, RandSeed: start.RandSeed, BattleType: proto.Int32(4), Attacker: start.BattlePlayer, Defender2: start.Monster, MaxRound: start.MaxRoundNum, AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
	if !proto.Equal(actual, &expected) {
		t.Fatal("same actual paid pending identity/seed/party/monster changed")
	}
	var after db.Profile
	if err := db.GetIns().First(&after, 1).Error; err != nil || *row.Player != *after.Player || *row.Game != *after.Game {
		t.Fatal("resuming changed actual paid inventory", err)
	}
	if err := db.GetIns().First(&quota, "player_id=? AND day=? AND type=?", 1, "2026-10-14", "PetEquip").Error; err != nil || quota.Used != 2 {
		t.Fatal("same pending consumed another attempt", err)
	}
	var pending db.DailyCopyFight
	if err := db.GetIns().First(&pending, "fight_id=?", start.GetFightId()).Error; err != nil || pending.Settled {
		t.Fatal("unavailable original terminal result was falsely settled", err)
	}
	var committed db.DailyCopyFight
	if err := db.GetIns().First(&committed, "fight_id=?", "1d21ab4c-1cec-4d4e-af14-8a7df51a909e").Error; err != nil || !committed.Settled || len(committed.Response) == 0 {
		t.Fatal("actual first original settlement disappeared", err)
	}
	var result pb.SCDailyCopyFightEnd
	if err := proto.Unmarshal(committed.Response, &result); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.GetGet())
	t.Logf("actual first source remains %s; same second paid pending has used=2/3 and zero fabricated outcome", encoded)
}
