package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestWorldBossFiveParty76OriginalMechanismAndReceipt(t *testing.T) {
	s := p6OriginalTables(t)
	input := p12WorldBossPartyScopeInput(t, s)
	grown := p6GenericGrownActors(t)
	ids := []string{"hero_001", "hero_002", "hero_003", "hero_007", "hero_008"}
	poses := []int32{3, 7, 8, 9, 13}
	input.Attacker.Formation = nil
	for i, id := range ids {
		if grown[id] == nil {
			t.Fatal("fixture missing actual actor", id)
		}
		actor := proto.Clone(grown[id]).(*pb.BattleHero)
		actor.Guid = proto.String(fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1))
		actor.Pose = proto.Int32(poses[i])
		input.Attacker.Formation = append(input.Attacker.Formation, actor)
	}
	out, err := s.computeP12WorldBossCertifiedFinalHP(t.Context(), input)
	if err != nil {
		t.Fatal("original five-member WorldBoss", err)
	}
	if len(out.FinalActors) != 9 || out.Outcome == nil || !out.Outcome.Authoritative {
		t.Fatal("incomplete five-member terminal")
	}
	if err = validateP12WorldBossCertifiedReceipt(input, out); err != nil {
		t.Fatal("original receipt", err)
	}
	sixth := proto.Clone(input.Attacker.Formation[0]).(*pb.BattleHero)
	sixth.Guid = proto.String("00000000-0000-4000-8000-000000000006")
	sixth.Pose = proto.Int32(1)
	input.Attacker.Formation = append(input.Attacker.Formation, sixth)
	if _, err = s.computeP12WorldBossCertifiedFinalHP(t.Context(), input); err == nil {
		t.Fatal("six-member formation accepted")
	}
}
