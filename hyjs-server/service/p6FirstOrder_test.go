package service

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalFirstOrderFlagsAndRecordedFirstCamp(t *testing.T) {
	s := p6OriginalTables(t)
	for _, pair := range [][2]int32{{1, 0}, {0, 1}, {0, 0}, {1, 1}, {2, -1}} {
		input := p6Input(t, s, p6Actor("hero_047", "first-player", "047_1", 1, 20000), p6Actor("hero_047", "second-player", "047_1", 1, 20000), 4)
		input.Attacker.Inst.FirstOrder, input.Defender.Inst.FirstOrder = proto.Int32(pair[0]), proto.Int32(pair[1])
		input.DefenderFirst = pair[0] == 0 && pair[1] == 1
		before, _ := json.Marshal(input)
		out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
		if err != nil || len(out.Operates.GetSkillOpts()) == 0 {
			t.Fatal("original initiative pair failed", pair, err)
		}
		firstCamp := int32(0)
		if input.DefenderFirst {
			firstCamp = 1
		}
		if out.Operates.SkillOpts[0].GetCamp() != firstCamp || out.Operates.SkillOpts[0].GetRoundNum() != 1 {
			t.Fatal("original initiative disagreed with recorded first operation", pair)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("historical original initiative flags normalized")
		}
		input.DefenderFirst = !input.DefenderFirst
		if _, err = s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
			t.Fatal("internal initiative inconsistent with original wire accepted", pair)
		}
	}
}

func TestP6NewFirstOrderPreservesOriginalAPAndRejectsMonsterInitiative(t *testing.T) {
	for _, first := range []bool{false, true} {
		a := &pb.BattlePlayer{Inst: &pb.BattlePlayerInst{Anger: proto.Float64(43)}}
		d := &pb.BattlePlayer{Inst: &pb.BattlePlayerInst{Anger: proto.Float64(27)}}
		if err := SetP6FirstOrder(a, d, first); err != nil || a.GetInst().GetAnger() != 43 || d.GetInst().GetAnger() != 27 || a.Inst.GetFirstOrder()+d.Inst.GetFirstOrder() != 1 || ValidateP6FirstOrder(&model.FrozenBattleInput{Attacker: a, Defender: d, DefenderFirst: first}) != nil {
			t.Fatal("new original initiative changed live AP or flags", first, err)
		}
	}
	if SetP6FirstOrder(nil, &pb.BattlePlayer{}, false) == nil || ValidateP6FirstOrder(&model.FrozenBattleInput{Attacker: &pb.BattlePlayer{}, Monster: &pb.BattleMonster{}, DefenderFirst: true}) == nil {
		t.Fatal("absent original defending player acquired initiative")
	}
}
