package service

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6KFRaw(t *testing.T, s *GameService, ids ...int64) *pb.KFReplayData {
	t.Helper()
	base := p6Input(t, s, p6Actor("hero_047", "kf-global0", "047_1", 1, 1000000), p6Actor("hero_047", "kf-global1", "047_1", 1, 1000000), 1)
	base.Attacker.PlayerId = proto.Int64(ids[0])
	base.Defender.PlayerId = proto.Int64(ids[1])
	raw := &pb.KFReplayData{WarId: proto.String("original-kf-war"), RandSeed: proto.Int64(42), AppVersion: proto.String("10056262"), Time: proto.Int64(1700000000), PlayerList: []*pb.BattlePlayer{base.Attacker, base.Defender}, WinPlayerId: proto.Int64(ids[1]), MaxRoundNum: proto.Int32(1), RoomId: proto.String("map_kfarena_001"), Score: proto.Int32(31)}
	if len(ids) > 2 {
		p := proto.Clone(base.Attacker).(*pb.BattlePlayer)
		p.PlayerId = proto.Int64(ids[2])
		p.Formation[0].Guid = proto.String("kf-global2")
		raw.PlayerList = append(raw.PlayerList, p)
	}
	return raw
}

func TestP6OriginalKFGlobalFirstOrderAndLocalProjection(t *testing.T) {
	s := p6OriginalTables(t)
	for _, flags := range [][2]int32{{1, 0}, {0, 1}, {0, 0}, {1, 1}, {2, -1}} {
		for _, viewer := range []int64{42, 43, 44, 999} {
			t.Run(fmt.Sprintf("flags%d_%d/view%d", flags[0], flags[1], viewer), func(t *testing.T) {
				raw := p6KFRaw(t, s, 42, 43, 44)
				raw.PlayerList[0].Inst.FirstOrder = proto.Int32(flags[0])
				raw.PlayerList[1].Inst.FirstOrder = proto.Int32(flags[1])
				before, _ := proto.Marshal(raw)
				frozen, err := s.FreezeP6KFReplayInput(raw, viewer)
				if err != nil {
					t.Fatal(err)
				}
				index := 0
				if viewer == 43 {
					index = 1
				}
				if viewer == 44 {
					index = 2
				}
				if frozen.BattleType != 17 || frozen.IsAttToDef != (index == 1) || frozen.DefenderFirst != (flags[0] == 0 && flags[1] == 1) || !proto.Equal(frozen.Attacker, raw.PlayerList[index]) || !proto.Equal(frozen.Defender, raw.PlayerList[(index+1)%2]) || !proto.Equal(frozen.KFReplay, raw) {
					t.Fatal("original global first/local pair projection changed")
				}
				out, err := s.ComputeFrozenBattle(t.Context(), frozen, nil)
				if err != nil {
					t.Fatal(err)
				}
				first := out.Operates.GetSkillOpts()[0]
				expectedCamp := int32(0)
				if frozen.DefenderFirst {
					expectedCamp = 1
				}
				expectedActor := frozen.Attacker.Formation[0].GetGuid()
				if expectedCamp == 1 {
					expectedActor = frozen.Defender.Formation[0].GetGuid()
				}
				if first.GetCamp() != expectedCamp || first.GetActorUid() != expectedActor || first.GetRoundNum() != 1 {
					t.Fatal("original KF attackFirst was inverted after local swap", first)
				}
				replay, err := s.ComputeFrozenBattle(t.Context(), frozen, out.Operates)
				if err != nil || out.Authoritative || out.RandomDraws != replay.RandomDraws || !proto.Equal(out.Operates, replay.Operates) || !reflect.DeepEqual(out.Events, replay.Events) {
					t.Fatal("original KF first-pair replay", err)
				}
				after, _ := proto.Marshal(raw)
				if string(before) != string(after) {
					t.Fatal("original global KF transport mutated")
				}
				raw.RoomId = proto.String("caller-change")
				if frozen.KFReplay.GetRoomId() != "map_kfarena_001" {
					t.Fatal("original KF raw aliased caller")
				}
				broken := *frozen
				broken.IsAttToDef = !frozen.IsAttToDef
				if ValidateP6FirstOrder(&broken) == nil {
					t.Fatal("KF direction mismatch accepted")
				}
				broken = *frozen
				broken.Seed++
				if ValidateP6FirstOrder(&broken) == nil {
					t.Fatal("KF seed mismatch accepted")
				}
			})
		}
	}
	raw := p6KFRaw(t, s, 42, 43)
	raw.MaxRoundNum = proto.Int32(0)
	f, err := s.FreezeP6KFReplayInput(raw, 999)
	if err != nil || f.MaxRound != 40 || f.KFReplay.GetMaxRoundNum() != 0 {
		t.Fatal("original KF default40 normalized raw field", err)
	}
	for _, bad := range []*pb.KFReplayData{nil, {}, {PlayerList: []*pb.BattlePlayer{raw.PlayerList[0]}}, {PlayerList: []*pb.BattlePlayer{raw.PlayerList[0], nil}}} {
		if _, err = s.FreezeP6KFReplayInput(bad, 42); err == nil {
			t.Fatal("malformed KF accepted")
		}
	}
}

func TestP6MySQLKFGlobalProjectionColdTransportAndMetadataRetry(t *testing.T) {
	dsn := os.Getenv("HYJS_MYSQL_P6_DSN")
	if dsn == "" {
		t.Skip("isolated original KF cold transport disabled")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(cfg.DBName, "hyjs_test_p6_") {
		t.Fatal("dedicated KF schema required")
	}
	ctx := t.Context()
	open := func() {
		t.Helper()
		if e := db.InitGameMySQL(ctx, db.MySQLConfig{Host: cfg.Addr, User: cfg.User, Password: cfg.Passwd, Database: cfg.DBName}); e != nil {
			t.Fatal(e)
		}
	}
	open()
	conn, _ := db.GetIns().DB()
	defer func() { conn.Close() }()
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	ids := []int64{dailyCopyTestPlayer(t, ctx, "kf-global0", 100), dailyCopyTestPlayer(t, ctx, "kf-global1", 100), dailyCopyTestPlayer(t, ctx, "kf-global2", 100)}
	var stored []struct {
		id    string
		owner int64
		raw   *pb.KFReplayData
		out   *model.CombatOutcome
	}
	for index, owner := range ids {
		raw := p6KFRaw(t, s, ids...)
		raw.PlayerList[0].Inst.FirstOrder = proto.Int32(1)
		raw.PlayerList[1].Inst.FirstOrder = proto.Int32(0)
		frozen, e := s.FreezeP6KFReplayInput(raw, owner)
		if e != nil {
			t.Fatal(e)
		}
		fight := fmt.Sprintf("kf-original-view-%d", index)
		var rid string
		var out *model.CombatOutcome
		if e = p6Tx(ctx, owner, func(tx *gorm.DB) error {
			var x error
			rid, x = s.CaptureBattleReplay(ctx, tx, owner, "p6_kf_projection", fight, *frozen)
			if x != nil {
				return x
			}
			out, x = s.ComputeBattleReplay(ctx, tx, owner, "p6_kf_projection", fight, nil)
			return x
		}); e != nil || out.Authoritative {
			t.Fatal("KF first-pair persistence", e)
		}
		altered := *frozen
		altered.KFReplay = proto.Clone(frozen.KFReplay).(*pb.KFReplayData)
		altered.KFReplay.RoomId = proto.String("forged-room")
		if e = p6Tx(ctx, owner, func(tx *gorm.DB) error {
			_, x := s.CaptureBattleReplay(ctx, tx, owner, "p6_kf_projection", fight, altered)
			return x
		}); e == nil {
			t.Fatal("KF metadata-only retry replaced original")
		}
		stored = append(stored, struct {
			id    string
			owner int64
			raw   *pb.KFReplayData
			out   *model.CombatOutcome
		}{rid, owner, raw, out})
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	open()
	conn, _ = db.GetIns().DB()
	for _, item := range stored {
		var row db.P6BattleRecord
		if err = db.GetIns().WithContext(ctx).Where("id = ? AND player_id = ?", item.id, item.owner).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		frozen, e := decodeP6Record(&row)
		if e != nil || !proto.Equal(frozen.KFReplay, item.raw) || len(frozen.KFReplay.GetPlayerList()) != 3 {
			t.Fatal("cold full original KF fields/groups changed", e)
		}
		wire, e := proto.Marshal(frozen.KFReplay)
		if e != nil {
			t.Fatal(e)
		}
		decoded := &pb.KFReplayData{}
		if e = proto.Unmarshal(wire, decoded); e != nil || !proto.Equal(decoded, item.raw) {
			t.Fatal("cold original KF protobuf transport", e)
		}
		out, e := s.ComputeFrozenBattle(ctx, frozen, item.out.Operates)
		if e != nil || !proto.Equal(out.Operates, item.out.Operates) || !reflect.DeepEqual(out.Events, item.out.Events) || out.RandomDraws != item.out.RandomDraws {
			t.Fatal("cold KF first-pair deterministic replay", e)
		}
		original, _ := json.Marshal(frozen.KFReplay)
		if len(original) == 0 || row.Authoritative {
			t.Fatal("KF original scope lost")
		}
	}
}
