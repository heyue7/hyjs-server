package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

// TestSkillSpecialtyOriginalAttributes 对照原等级/强化表检查固定属性、门槛和未知作用拒绝。
func TestSkillSpecialtyOriginalAttributes(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var (
		player  = &pb.Player{Level: proto.Int32(75), HeroWarId: proto.String("main")}
		warrior = &pb.Warrior{Id: proto.String("hero_001"), Guid: proto.String("main"), Level: proto.Int32(1)}
	)
	if err := s.specialtyGate(player, warrior); err != nil {
		t.Fatal(err)
	}
	player.Level = proto.Int32(74)
	if s.specialtyGate(player, warrior) == nil {
		t.Fatal("low main bypassed gate")
	}
	player.Level = proto.Int32(75)
	warrior.Guid = proto.String("companion")
	if s.specialtyGate(player, warrior) == nil {
		t.Fatal("low companion bypassed gate")
	}
	warrior.Level = proto.Int32(75)
	if err := s.specialtyGate(player, warrior); err != nil {
		t.Fatal(err)
	}
	warrior.Id = proto.String("hero_043")
	if s.specialtyGate(player, warrior) == nil {
		t.Fatal("unsupported quality bypassed gate")
	}
	warrior.Id = proto.String("hero_001")
	warrior.SkillRefine = []*pb.SkillRefine{{RoleGuid: proto.String("companion"), Type: proto.Int32(1), Level: proto.Int32(10), StrengthLevel: proto.Int32(2)}}
	values := map[string]float64{}
	if err := s.AddSkillSpecialtyProperties(warrior, values); err != nil {
		t.Fatal(err)
	}
	if values["hp"] != 6000 {
		t.Fatal("original level and strength sum", values)
	}
	warrior.SkillRefine = append(warrior.SkillRefine, &pb.SkillRefine{RoleGuid: proto.String("companion"), Type: proto.Int32(5), Level: proto.Int32(2), StrengthLevel: proto.Int32(1)})
	values = map[string]float64{}
	if err := s.AddSkillSpecialtyProperties(warrior, values); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cri", "antiCri", "hit", "dodge"} {
		if values[key] != 0.6 {
			t.Fatal("original special attribute", values)
		}
	}
	row, err := s.specialtyTable("SkillSpecialty", "level", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	row["propAdd"] = "unknown,500"
	if s.AddSkillSpecialtyProperties(warrior, map[string]float64{}) == nil {
		t.Fatal("unknown effect silently accepted")
	}
	row["propAdd"] = "hp,NaN"
	if s.AddSkillSpecialtyProperties(warrior, map[string]float64{}) == nil {
		t.Fatal("NaN effect accepted")
	}
	row["propAdd"] = "hp,4500"
	warrior.SkillRefine = append(warrior.SkillRefine, proto.Clone(warrior.SkillRefine[0]).(*pb.SkillRefine))
	if s.AddSkillSpecialtyProperties(warrior, map[string]float64{}) == nil {
		t.Fatal("duplicate specialty accepted")
	}
}
