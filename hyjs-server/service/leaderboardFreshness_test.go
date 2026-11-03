package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type leaderboardFreshnessRole struct {
	id   int64
	name string
	hero string
}

type leaderboardFreshnessRow struct {
	id     int64
	values []int32
}

// These fixtures exercise real committed rows in an isolated P1 MySQL database.
// The pending transaction is read from another connection and then rolled back.
func TestLeaderboardFreshnessCSRankCommittedAndRollbackMySQL(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("isolated P1 MySQL database required")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") {
		t.Fatal("isolated P1 database required")
	}
	now := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	ctx := context.WithValue(context.Background(), constUtil.P13UnionFightClockContextKey{}, func() time.Time { return now })
	fixture := &p1Fixture{ctx: ctx, path: filepath.Join(t.TempDir(), "leaderboard-freshness.db")}
	fixture.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
	fixture.open(t)
	database := db.GetIns()
	t.Cleanup(func() {
		sqlDB, err := database.DB()
		if err != nil {
			t.Error(err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := database.AutoMigrate(&db.P13Guild{}, &db.P13GuildMember{}, &db.P7RushState{}, &db.P12WantedFight{}, &db.P12WantedInstance{}); err != nil {
		t.Fatal(err)
	}

	unique := time.Now().UnixNano()
	zone := int64(1000000000 + unique%900000000)
	var occupied int64
	if err := database.Model(&db.Profile{}).Where("server_id IN ?", []int64{zone, zone + 1}).Count(&occupied).Error; err != nil {
		t.Fatal(err)
	}
	if occupied != 0 {
		t.Fatal("freshness fixture server IDs already exist")
	}
	instanceIDs := []string{fmt.Sprintf("lf-%d-local", unique), fmt.Sprintf("lf-%d-foreign", unique)}
	var ids []int64
	t.Cleanup(func() {
		if err := database.Where("instance_id IN ?", instanceIDs).Delete(&db.P12WantedFight{}).Error; err != nil {
			t.Errorf("cleanup wanted fights: %v", err)
		}
		if err := database.Where("id IN ?", instanceIDs).Delete(&db.P12WantedInstance{}).Error; err != nil {
			t.Errorf("cleanup wanted instances: %v", err)
		}
		if len(ids) > 0 {
			if err := database.Where("player_id IN ?", ids).Delete(&db.P7RushState{}).Error; err != nil {
				t.Errorf("cleanup rush states: %v", err)
			}
			if err := database.Where("id IN ?", ids).Delete(&db.Profile{}).Error; err != nil {
				t.Errorf("cleanup profiles: %v", err)
			}
		}
	})

	createRole := func(label string, server int64) leaderboardFreshnessRole {
		t.Helper()
		uid := fmt.Sprintf("leaderboard-freshness-%d-%s", unique, label)
		row := db.Profile{Uid: uid, ServerID: server, Session: []byte{}}
		if err := database.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.Id)
		return leaderboardFreshnessRole{id: row.Id, name: label, hero: uid + "-hero"}
	}
	first := createRole("first", zone)
	second := createRole("second", zone)
	foreign := createRole("foreign", zone+1)

	writeRole := func(tx *gorm.DB, role leaderboardFreshnessRole, level, power, stars int32) {
		t.Helper()
		player := &pb.Player{PlayerId: proto.Int64(role.id), Name: proto.String(role.name), HeroWarId: proto.String(role.hero), Level: proto.Int32(level), FightPower: proto.Int32(power), WarriorBagSize: proto.Int32(100), EquipBagSize: proto.Int32(100), PetBagSize: proto.Int32(100)}
		game := &model.GameState{
			Warriors:  []*pb.Warrior{{Id: proto.String("hero_043"), Guid: proto.String(role.hero), Level: proto.Int32(1), Breached: &pb.Breached{Level: proto.Int32(1)}, Training: &pb.Training{}, TempTraining: &pb.Training{}, Awake: &pb.Awake{}}},
			Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: proto.String(role.hero), Id: proto.String("hero_043")}}}},
			Copy:      &pb.SCCopyPassedInfo{},
		}
		for mission := 1; stars > 0; mission++ {
			current := min(stars, int32(3))
			game.Copy.PassedCopyInfo = append(game.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String("1"), MissionId: proto.String(fmt.Sprint(mission)), Star: proto.Int32(current)})
			stars -= current
		}
		playerJSON, err := json.Marshal(player)
		if err != nil {
			t.Fatal(err)
		}
		gameJSON, err := json.Marshal(game)
		if err != nil {
			t.Fatal(err)
		}
		result := tx.Model(&db.Profile{}).Where("id=?", role.id).Updates(map[string]any{"player": string(playerJSON), "game": string(gameJSON)})
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("update role %d: rows=%d err=%v", role.id, result.RowsAffected, result.Error)
		}
	}
	writeRole(database, first, 20, 2000, 3)
	writeRole(database, second, 10, 1000, 1)
	writeRole(database, foreign, 90, 9000, 9)
	for _, state := range []db.P7RushState{{PlayerID: first.id, RecordStars: 10}, {PlayerID: second.id, RecordStars: 5}, {PlayerID: foreign.id, RecordStars: 99}} {
		if err := database.Create(&state).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, instance := range []db.P12WantedInstance{{ID: instanceIDs[0], ServerID: zone}, {ID: instanceIDs[1], ServerID: zone + 1}} {
		if err := database.Create(&instance).Error; err != nil {
			t.Fatal(err)
		}
	}
	fightIDs := []string{fmt.Sprintf("lf-%d-a", unique), fmt.Sprintf("lf-%d-b", unique), fmt.Sprintf("lf-%d-foreign", unique), fmt.Sprintf("lf-%d-cross", unique), fmt.Sprintf("lf-%d-pending", unique)}
	for _, fight := range []db.P12WantedFight{
		{ID: fightIDs[0], PlayerID: first.id, InstanceID: instanceIDs[0], Committed: true, CommittedAt: now.UnixMilli(), Performance: 10, AppliedDamage: 20},
		{ID: fightIDs[1], PlayerID: second.id, InstanceID: instanceIDs[0], Committed: false, CommittedAt: now.UnixMilli(), Performance: 1, AppliedDamage: 1},
		{ID: fightIDs[2], PlayerID: foreign.id, InstanceID: instanceIDs[1], Committed: true, CommittedAt: now.UnixMilli(), Performance: 999, AppliedDamage: 999},
		{ID: fightIDs[3], PlayerID: second.id, InstanceID: instanceIDs[1], Committed: true, CommittedAt: now.UnixMilli(), Performance: 999, AppliedDamage: 999},
		{ID: fightIDs[4], PlayerID: first.id, InstanceID: instanceIDs[0], Committed: false, CommittedAt: now.UnixMilli(), Performance: 500, AppliedDamage: 500},
	} {
		fight.StartProto, fight.EndProto = []byte{}, []byte{}
		if err := database.Create(&fight).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := map[int32][]leaderboardFreshnessRow{
		1: {{first.id, []int32{3, 2000}}, {second.id, []int32{1, 1000}}},
		2: {{first.id, []int32{2000}}, {second.id, []int32{1000}}},
		3: {{first.id, []int32{20}}, {second.id, []int32{10}}},
		4: {{first.id, []int32{20}}},
		5: {{first.id, []int32{10}}},
		6: {{first.id, []int32{10, 2000}}, {second.id, []int32{5, 1000}}},
	}
	after := map[int32][]leaderboardFreshnessRow{
		1: {{second.id, []int32{6, 4000}}, {first.id, []int32{3, 2000}}},
		2: {{second.id, []int32{4000}}, {first.id, []int32{2000}}},
		3: {{second.id, []int32{35}}, {first.id, []int32{20}}},
		4: {{second.id, []int32{40}}, {first.id, []int32{20}}},
		5: {{second.id, []int32{20}}, {first.id, []int32{10}}},
		6: {{second.id, []int32{25, 4000}}, {first.id, []int32{10, 2000}}},
	}
	service := &GameService{}
	assertRanks := func(readCtx context.Context, phase string, expected map[int32][]leaderboardFreshnessRow) {
		t.Helper()
		for kind := int32(1); kind <= 6; kind++ {
			out, handled, err := service.DispatchP10Social(readCtx, first.id, &pb.CSRank{Type: proto.Int32(kind)}, 1)
			if err != nil || !handled || len(out) != 1 {
				t.Fatalf("%s type %d: handled=%v replies=%d err=%v", phase, kind, handled, len(out), err)
			}
			response, ok := out[0].(*pb.SCRank)
			if !ok {
				t.Fatalf("%s type %d: unexpected reply %T", phase, kind, out[0])
			}
			rows := response.GetRankIno().GetList()
			if len(rows) != len(expected[kind]) {
				t.Fatalf("%s type %d: got %d rows, want %d", phase, kind, len(rows), len(expected[kind]))
			}
			for index, want := range expected[kind] {
				row := rows[index]
				if row.GetPid() != want.id || row.GetRank() != int32(index+1) || !reflect.DeepEqual(row.GetValues(), want.values) {
					t.Fatalf("%s type %d row %d: got %v, want player=%d values=%v", phase, kind, index, row, want.id, want.values)
				}
			}
		}
	}
	assertRanks(ctx, "before commit", before)

	committed := database.Begin()
	if committed.Error != nil {
		t.Fatal(committed.Error)
	}
	defer committed.Rollback()
	writeRole(committed, second, 35, 4000, 6)
	if result := committed.Model(&db.P7RushState{}).Where("player_id=?", second.id).Update("record_stars", 25); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("commit rush state: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if result := committed.Model(&db.P12WantedFight{}).Where("id=?", fightIDs[1]).Updates(map[string]any{"committed": true, "committed_at": now.UnixMilli(), "performance": 20, "applied_damage": 40.75}); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("commit wanted fight: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if err := committed.Commit().Error; err != nil {
		t.Fatal(err)
	}
	assertRanks(ctx, "after commit", after)

	pending := database.Begin()
	if pending.Error != nil {
		t.Fatal(pending.Error)
	}
	defer pending.Rollback()
	writeRole(pending, first, 99, 9900, 9)
	if result := pending.Model(&db.P7RushState{}).Where("player_id=?", first.id).Update("record_stars", 99); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("pending rush state: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if result := pending.Model(&db.P12WantedFight{}).Where("id=?", fightIDs[0]).Updates(map[string]any{"performance": 99, "applied_damage": 99}); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("pending wanted fight: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if result := pending.Model(&db.P12WantedFight{}).Where("id=?", fightIDs[4]).Update("committed", true); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("pending wanted committed flag: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	assertRanks(readCtx, "while uncommitted", after)
	if err := pending.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	assertRanks(ctx, "after rollback", after)
}

func TestLeaderboardFreshnessP11RankRefreshesOtherProfileMySQL(t *testing.T) {
	name := os.Getenv("HYJS_P11_MYSQL_DATABASE")
	if name == "" {
		t.Skip("isolated P11 MySQL database required")
	}
	if !strings.HasPrefix(name, "hyjs_test_p11_") {
		t.Fatal("isolated P11 database required")
	}
	fixture, peer, third := p11ArenaSetup(t)
	now := time.Now()
	for _, item := range []struct{ id, position int64 }{{fixture.id, 10}, {peer, 2}, {third, 3}} {
		result := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=? AND active=?", item.id, true).Update("position", item.position)
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("set member %d position: rows=%d err=%v", item.id, result.RowsAffected, result.Error)
		}
	}
	var frozen db.P11ArenaMember
	if err := db.GetIns().First(&frozen, "player_id=?", peer).Error; err != nil {
		t.Fatal(err)
	}
	frozenInfo := &pb.SimplePlayerInfo{}
	if err := proto.Unmarshal(frozen.Info, frozenInfo); err != nil {
		t.Fatal(err)
	}
	if frozenInfo.GetPlayerId() != peer || len(frozen.Battle) == 0 || frozen.SnapshotHash == "" {
		t.Fatal("peer has no valid frozen arena projection")
	}
	const newName = "竞技榜实时资料"
	if err := updateProfile(fixture.ctx, peer, func(player *pb.Player, game *model.GameState) error {
		player.Name = proto.String(newName)
		player.Level = proto.Int32(31)
		for _, warrior := range game.Warriors {
			if warrior.GetGuid() == player.GetHeroWarId() {
				warrior.Level = proto.Int32(warrior.GetLevel() + 10)
				return nil
			}
		}
		return fmt.Errorf("peer's formation hero is missing")
	}); err != nil {
		t.Fatal(err)
	}
	currentPlayer, currentGame := p10LuckyProfile(t, peer)
	currentPower, err := fixture.s.formationFightPower(currentPlayer, currentGame)
	if err != nil {
		t.Fatal(err)
	}
	if currentPower <= 0 || currentPower == frozenInfo.GetFightPower() || currentPlayer.GetName() == frozenInfo.GetPlayerName() || currentPlayer.GetLevel() == frozenInfo.GetLevel() {
		t.Fatalf("profile update did not change the frozen public values: current power=%d frozen=%d", currentPower, frozenInfo.GetFightPower())
	}

	listing := p11ArenaQuery(t, fixture, fixture.id, &pb.CSPvpRank{}, 0, now).(*pb.SCPvpRank)
	rows := listing.GetRankList()
	if len(rows) < 3 || rows[0].GetPvpRank() != 1 || !p11ArenaNPCID(rows[0].GetInfo().GetPlayerId()) || rows[1].GetPvpRank() != 2 || rows[1].GetInfo().GetPlayerId() != peer || rows[2].GetPvpRank() != 3 || rows[2].GetInfo().GetPlayerId() != third {
		t.Fatalf("top-three NPC and real positions changed: %v", rows)
	}
	info := rows[1].GetInfo()
	if info.GetPlayerName() != newName || info.GetLevel() != 31 || info.GetFightPower() != currentPower {
		t.Fatalf("peer rank did not use committed public profile: got %v want name=%s level=31 power=%d", info, newName, currentPower)
	}
	var after db.P11ArenaMember
	if err := db.GetIns().First(&after, "player_id=?", peer).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Info, frozen.Info) || !bytes.Equal(after.Battle, frozen.Battle) || after.SnapshotHash != frozen.SnapshotHash {
		t.Fatal("viewing peer's current information changed its historical member snapshot")
	}
}
