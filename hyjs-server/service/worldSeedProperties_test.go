package service

import (
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func worldSeedFixture(t *testing.T) (*GameService, *pb.Player, *model.GameState) {
	t.Helper()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	player := &pb.Player{PlayerId: proto.Int64(3), Level: proto.Int32(5), HeroWarId: proto.String("main"), CurrentWorldSeedId: proto.Int32(1)}
	game := &model.GameState{
		Warriors: []*pb.Warrior{
			{Guid: proto.String("main"), Id: proto.String("hero_047"), Level: proto.Int32(1), Talent: proto.Int32(1), Breached: &pb.Breached{Level: proto.Int32(1)}},
			{Guid: proto.String("partner"), Id: proto.String("hero_025"), Level: proto.Int32(3), Breached: &pb.Breached{Level: proto.Int32(1)}},
		},
		Equipment: []*pb.Equipment{{Guid: proto.String("weapon"), EquipId: proto.String("equip_001"), Level: proto.Int32(7)}},
		Formation: []*pb.FormationItem{
			{FormatPos: proto.Int32(1), BattlePos: proto.Int32(3), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: proto.String("main"), Id: proto.String("hero_047")}, {Pos: proto.Int32(1), Guid: proto.String("weapon"), Id: proto.String("equip_001")}}},
			{FormatPos: proto.Int32(2), BattlePos: proto.Int32(7), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: proto.String("partner"), Id: proto.String("hero_025")}}},
		},
	}
	return s, player, game
}

func TestWorldSeedStarterMatchesNativePanelAndAllTeamMembers(t *testing.T) {
	s, player, game := worldSeedFixture(t)
	party, err := s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(party.GetFormation()) != 2 {
		t.Fatalf("team size=%d", len(party.GetFormation()))
	}
	main := party.GetFormation()[0].GetProp()
	// 实机同装备、天赋和等级：171攻、1518血、56物防、46魔防。
	if main.GetAtt() != 171 || main.GetHp() != 1518 || main.GetPdef() != 56 || main.GetMdef() != 46 {
		t.Fatalf("native starter panel mismatch: %v", main)
	}
	player.CurrentWorldSeedId = nil
	without, err := s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	for index, actor := range party.GetFormation() {
		base := without.GetFormation()[index].GetProp()
		if actor.GetProp().GetPdef()-base.GetPdef() != 10 || actor.GetProp().GetMdef() != base.GetMdef() {
			t.Fatalf("world seed did not apply equally to team member %s", actor.GetId())
		}
		if actor.GetInst().GetCurHp() != actor.GetProp().GetHp() {
			t.Fatal("initial HP differs from final battle property")
		}
	}
}

func TestWorldSeedUsesOriginalCumulativePrefixAndIgnoresRewardNodes(t *testing.T) {
	s, _, _ := worldSeedFixture(t)
	for _, test := range []struct {
		level int32
		want  map[string]float64
	}{
		{0, map[string]float64{}},
		{-1, map[string]float64{}},
		{1, map[string]float64{"pdef": 10}},
		{4, map[string]float64{"pdef": 10, "mdef": 10, "hp": 170, "att": 15}},
		{5, map[string]float64{"pdef": 10, "mdef": 10, "hp": 170, "att": 15}},
		{10, map[string]float64{"pdef": 30, "mdef": 30, "hp": 530, "att": 45}},
	} {
		values := map[string]float64{}
		if err := s.addWorldSeedProperties(&pb.Player{CurrentWorldSeedId: proto.Int32(test.level)}, values); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"att", "hp", "pdef", "mdef"} {
			if values[key] != test.want[key] {
				t.Fatalf("seed=%d %s=%v want=%v", test.level, key, values[key], test.want[key])
			}
		}
	}
	values := map[string]float64{}
	if err := s.addWorldSeedProperties(nil, values); err != nil || len(values) != 0 {
		t.Fatalf("empty seed changed attributes: %v %v", values, err)
	}
	// 原 DAO 越界时截到表尾；重复构造属性不改变角色或配置。
	var previous map[string]float64
	for _, level := range []int32{int32(len(s.Tables["WorldSeed"])), math.MaxInt32} {
		values := map[string]float64{}
		if err := s.addWorldSeedProperties(&pb.Player{CurrentWorldSeedId: proto.Int32(level)}, values); err != nil {
			t.Fatal(err)
		}
		if previous != nil {
			for key, value := range previous {
				if values[key] != value {
					t.Fatalf("past-end seed changed %s", key)
				}
			}
		}
		previous = values
	}
}

func TestWorldSeedIsAddedBeforePercentAndLeavesMonstersUnchanged(t *testing.T) {
	s, player, game := worldSeedFixture(t)
	game.Warriors[0].Breached.Level = proto.Int32(2)
	player.CurrentWorldSeedId = nil
	before, err := s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	row := s.Tables["CopyEnemy"][0]
	monster, err := s.actor(row, "monster", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	player.CurrentWorldSeedId = proto.Int32(1)
	after, err := s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	want := 10 * (1 + tableNumber(s.table("HeroBreached", "id", "2"), "PDefAddPercent")/100)
	if delta := after.GetFormation()[0].GetProp().GetPdef() - before.GetFormation()[0].GetProp().GetPdef(); math.Abs(delta-want) > 1e-8 {
		t.Fatalf("world seed percent order delta=%v want=%v", delta, want)
	}
	unchanged, err := s.actor(row, "monster", 3, true)
	if err != nil || !proto.Equal(monster, unchanged) {
		t.Fatalf("player world seed changed monster: %v", err)
	}
}

func TestWorldSeedConfigurationErrorsDoNotInventBattleProperties(t *testing.T) {
	for _, value := range []string{"pdef", "pdef,NaN", "pdef,+Inf", "pdef,-10", "unknown,10"} {
		s := &GameService{Tables: map[string][]map[string]string{"WorldSeed": {{"type": "1", "get": value}}}}
		if err := s.addWorldSeedProperties(&pb.Player{CurrentWorldSeedId: proto.Int32(1)}, map[string]float64{}); err == nil {
			t.Fatalf("invalid original-table value accepted: %s", value)
		}
	}
}
