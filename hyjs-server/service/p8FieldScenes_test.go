package service

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP8FieldAvailableScenesPreserveAreaSideDailyLimitsAndCold(t *testing.T) {
	s, ctx, path := p9IntimacyFixture(t)
	if err := s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(ctx, "p8-scenes", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "六图状态隔离", "hero_043"); err != nil {
		t.Fatal(err)
	}
	state := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var profile db.Profile
		if err := db.GetIns().First(&profile, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	transfer := func(scene string) {
		t.Helper()
		if _, err := s.fieldTransfer(ctx, account.Id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
			t.Fatal(err)
		}
	}
	if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(24); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.fieldTransfer(ctx, account.Id, &pb.CSTransferScene{Target: proto.String("field_002")}); err == nil {
		t.Fatal("original snowfield25 level gate bypassed")
	}
	if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	transfer("field_001")
	_, original := state()
	firstGUID := original.Field.Monsters[0].GetUuid()
	due := time.Now().Add(time.Hour).UnixMilli()
	if err = updateProfile(ctx, account.Id, func(_ *pb.Player, g *model.GameState) error {
		g.Field.Monsters[0].State = proto.Int32(3)
		g.Field.RespawnAt[firstGUID] = due
		g.Field.RewardKills = 7
		g.Field.Side.CurHp = proto.Float64(.25)
		g.Field.Side.Tiredness = proto.Int32(197)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, old := state()
	side := proto.Clone(old.Field.Side).(*pb.PlayerSide)
	transfer("field_002")
	_, snow := state()
	if snow.Field.Scene != "field_002" || !proto.Equal(side, snow.Field.Side) || snow.Field.RewardKills != 7 {
		t.Fatal("map switch reset shared HP/fatigue/reward counters")
	}
	for _, monster := range snow.Field.Monsters {
		if !strings.Contains(s.fieldSceneRow("field_002")["MonsterPoints"]+";", monster.GetMonsterPosId()+";") {
			t.Fatal("snowfield spawned a different map's original position")
		}
	}
	if _, err = s.fieldMonsterCheck(ctx, account.Id, &pb.CSSideMonsterCheck{Uuid: proto.String(firstGUID)}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.fieldFightStart(ctx, account.Id, &pb.CSSidePveFightStart{MonsterId: snow.Field.Monsters[0].Uuid}); err == nil {
		t.Fatal("missing original snow battle bundle silently enabled")
	}
	transfer("field_001")
	_, restored := state()
	if restored.Field.Monsters[0].GetUuid() != firstGUID || restored.Field.Monsters[0].GetState() != 3 || restored.Field.RespawnAt[firstGUID] != due || !proto.Equal(restored.Field.Side, side) || restored.Field.RewardKills != 7 {
		t.Fatal("map switching rerolled or respawned the previous area")
	}
	beforeP, beforeG := state()
	for _, scene := range []string{"field_003", "field_004", "field_005", "field_006", "field_999"} {
		if _, err = s.fieldTransfer(ctx, account.Id, &pb.CSTransferScene{Target: proto.String(scene)}); err == nil {
			t.Fatal("missing scene bundle was exposed", scene)
		}
	}
	afterP, afterG := state()
	if !proto.Equal(beforeP, afterP) || !reflect.DeepEqual(beforeG, afterG) {
		t.Fatal("unavailable scene mutated the account")
	}
	pool, _ := db.GetIns().DB()
	_ = pool.Close()
	p9IntimacyOpen(t, ctx, path)
	cold, _ := db.GetIns().DB()
	t.Cleanup(func() { cold.Close() })
	transfer("field_002")
	transfer("field_001")
	_, restored = state()
	if restored.Field.Monsters[0].GetUuid() != firstGUID || restored.Field.RespawnAt[firstGUID] != due || restored.Field.RewardKills != 7 || !proto.Equal(restored.Field.Side, side) {
		t.Fatal("cold switching lost persistent area or shared counters")
	}
}
