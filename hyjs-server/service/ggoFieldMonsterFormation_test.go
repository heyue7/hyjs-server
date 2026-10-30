package service

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Preserve the tier statistics and use the actual main-story group models,
// positions and skills. The frozen P6 input must own an independent copy.
func TestGGOMonsterFormationOriginalFieldBaselines(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, tier := range defaultGGORuntime().MonsterTiers {
		t.Run(fmt.Sprint(tier.Level), func(t *testing.T) {
			node := &ggoMonsterNode{UUID: "formation-fixture", HeroID: tier.HeroID, Level: tier.Level, MonID: ggoMonsterGroup(tier.HeroID)}
			rows := s.adminFieldCombatRows("FieldMonster", "MonId", node.MonID, constUtil.GGOFieldScene)
			if len(rows) != 4 {
				t.Fatalf("original field group %s has %d members", node.MonID, len(rows))
			}
			original := map[int32]bool{}
			for _, row := range rows {
				pose := int32(tableNumber(row, "Location"))
				if original[pose] || tableNumber(row, "GroupIndex") != 1 || tableNumber(row, "Level") != float64(tier.Level) ||
					tableNumber(row, "Hp") != float64(tier.HP) || tableNumber(row, "Attack") != float64(tier.Attack) ||
					tableNumber(row, "PhyDefend") != float64(tier.PhyDefend) || tableNumber(row, "MagicDefend") != float64(tier.MagicDefend) {
					t.Fatalf("desert default differs from original group member: %v", row)
				}
				original[pose] = true
			}
			monster, err := s.ggoMonsterBattleFormation(node, &tier)
			if err != nil || len(monster.GetFormation()) != 4 || monster.GetTaskId() != node.MonID {
				t.Fatal("desert battle group", err)
			}
			task := map[int64]string{35: "123", 45: "208", 60: "200"}[tier.Level]
			members := map[int32]map[string]string{}
			for _, row := range s.Tables["CopyEnemy"] {
				if row["TaskId"] == task && row["GroupIndex"] == "0" {
					members[int32(tableNumber(row, "Location"))] = row
				}
			}
			seen := map[string]bool{}
			models := map[string]bool{}
			for _, actor := range monster.Formation {
				row := members[actor.GetPose()]
				if row == nil {
					t.Fatal("missing source pose", actor.GetPose())
				}
				skills := row["SkillId1"] + "," + row["SkillId2"] + "," + row["UltSkillId"]
				if !original[actor.GetPose()] || seen[actor.GetGuid()] || actor.GetGuid() == "" || actor.GetId() != row["HeroId"] ||
					actor.GetLevel() != int32(tier.Level) || actor.GetProp().GetHp() != float64(tier.HP) ||
					actor.GetProp().GetAtt() != float64(tier.Attack) || actor.GetProp().GetPdef() != float64(tier.PhyDefend) ||
					actor.GetProp().GetMdef() != float64(tier.MagicDefend) || actor.GetInst().GetCurHp() != float64(tier.HP) ||
					actor.GetSkillOrder() != skills || len(actor.GetTalentList()) != 0 {
					t.Fatal("invalid desert actor", actor)
				}
				delete(original, actor.GetPose())
				seen[actor.GetGuid()] = true
				models[actor.GetId()] = true
			}
			if len(original) != 0 || len(members) != 4 || len(models) < 2 || !models[tier.HeroID] {
				t.Fatal("original field position or mixed models missing", original, models)
			}
			party := &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{p6Actor("hero_047", "owner", "047_1", 3, 80000)}, Inst: &pb.BattlePlayerInst{}}
			frozen, err := s.freezeAndQualifyP6GenericMechanisms(context.Background(), model.FrozenBattleInput{
				Attacker: party, Monster: monster, Seed: 42, MaxRound: 10, BattleType: 6,
				AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1),
			})
			if err != nil || len(frozen.Monster.GetFormation()) != 4 {
				t.Fatal("desert formation rejected by P6", err)
			}
			monster.Formation[0].Prop.Hp = proto.Float64(1)
			if frozen.Monster.Formation[0].GetProp().GetHp() != float64(tier.HP) {
				t.Fatal("frozen input shares mutable actor")
			}
		})
	}
}
