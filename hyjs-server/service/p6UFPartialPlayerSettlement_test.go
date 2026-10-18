package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"testing"
)

func TestP6UFPartialPlayerActualWarSourceScopeAndNoMutation(t *testing.T) {
	s := p6OriginalTables(t)
	for _, index := range []int{0, 1} {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/native-actual-war-wounded14-inputs-v196/%03d.SCUFFightRecordReplay.pb", index))
		if err != nil {
			t.Fatal(err)
		}
		typed := &pb.SCUFFightRecordReplay{}
		if proto.Unmarshal(raw, typed) != nil {
			t.Fatal("actual typed14 source")
		}
		input, err := s.FreezeP6UFPlayerInput(typed)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		if validateP6UFPartialPlayerScope(input) != nil || validateP6UFPlayerScope(input) == nil {
			t.Fatal("independent partial vs legacy fullHP boundary")
		}
		for _, foreign := range []string{"", constUtil.P6UFPlayerNativeProof, constUtil.P6StarterNativeProof, constUtil.P6ActualPartyNativeProof, constUtil.P6KFCanonicalNativeProof} {
			if p6KnownUFPartialPlayerSettlementProof(input, foreign) {
				t.Fatal("foreign capability")
			}
		}
		varied, _ := cloneP6CalculationInput(input)
		varied.Seed++
		if validateP6UFPartialPlayerScope(varied) != nil {
			t.Fatal("capability must not pick observed seed")
		}
		for _, kind := range []string{"zeroHP", "overHP", "nanHP", "negativeAP", "overAP", "nanAP", "factor0", "unknownskill", "mode", "party", "unknownmax"} {
			changed, _ := cloneP6CalculationInput(input)
			switch kind {
			case "zeroHP":
				changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(0)
			case "overHP":
				changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(changed.Attacker.Formation[0].Prop.GetHp() + 1)
			case "nanHP":
				changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(math.NaN())
			case "negativeAP":
				changed.Attacker.Inst.Anger = proto.Float64(-1)
			case "overAP":
				changed.Attacker.Inst.Anger = proto.Float64(301)
			case "nanAP":
				changed.Attacker.Inst.Anger = proto.Float64(math.NaN())
			case "factor0":
				changed.AttackerDamageFactor = proto.Float64(0)
			case "unknownskill":
				changed.Attacker.Formation[0].SkillOrder = proto.String("unknown")
			case "mode":
				changed.BattleType = 19
			case "party":
				changed.Attacker.Formation = append(changed.Attacker.Formation, proto.Clone(changed.Attacker.Formation[0]).(*pb.BattleHero))
			case "unknownmax":
				changed.Attacker.Formation[0].Prop.Perhp = proto.Float64(1)
			}
			if validateP6UFPartialPlayerScope(changed) == nil {
				t.Fatal("unknown partial branch", kind)
			}
		}
		if constUtil.P6UFPartialPlayerNativeProof == "" {
			if out, err := s.ComputeP6UFPartialPlayerSettlement(t.Context(), input); err == nil || out != nil {
				t.Fatal("empty proof accepted")
			}
		} else {
			out, err := s.ComputeP6UFPartialPlayerSettlement(t.Context(), input)
			if err != nil || out == nil || !out.Outcome.Authoritative || !p6KnownUFPartialPlayerSettlementProof(input, out.Outcome.CapabilityHash) || validateP6UFPartialPlayerTerminal(input, out) != nil {
				t.Fatal("independent current war capability", err)
			}
			out.FinalActors[0].CurrentHP++
			if validateP6UFPartialPlayerTerminal(input, out) == nil {
				t.Fatal("tampered currentHP")
			}
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("actual current source mutated")
		}
	}
}
