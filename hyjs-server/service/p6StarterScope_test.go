package service

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestP6OriginalCompleteStarterSkillOrder(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_047", "complete-starter", "047_1,047_2,047_3", 1, 40000)
	input := p6Input(t, s, a, p6Actor("hero_047", "enemy", "047_1,047_2,047_3", 1, 40000), 16)
	input.Attacker.Inst.Anger = proto.Float64(300)
	input.Defender.Inst.Anger = proto.Float64(300)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int32]bool{}
	for _, operate := range out.Operates.SkillOpts {
		seen[operate.GetSkillIndex()] = true
	}
	if !seen[0] || !seen[1] || !seen[2] || out.Authoritative {
		t.Fatal("starter full skill sequence missing", seen)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("starter full sequence replay changed", err)
	}
}
