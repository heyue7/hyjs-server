package service

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalBasePercentageInputHasNoEngineConsumer(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/p6/base-property-percentage-readers-v90-original.json")
	if err != nil || p6Digest(raw) != "6bfae376fbec6a0d64f474644448439e30e2fdfdffbeef9fb150058a2342a583" {
		t.Fatal("complete original percentage getter audit", err)
	}
	var audit struct {
		DLL           string `json:"originalDLLSHA256"`
		Decoded       int    `json:"decodedMethods"`
		Errors        []any  `json:"errors"`
		Readers       []any  `json:"getterConsumers"`
		EngineReaders []any  `json:"engineGetterConsumers"`
	}
	if err = json.Unmarshal(raw, &audit); err != nil || audit.DLL != "ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f" || audit.Decoded != 26946 || len(audit.Errors) != 0 || len(audit.Readers) != 24 || len(audit.EngineReaders) != 0 {
		t.Fatal("original engine/actor constructor percentage reads not closed", err)
	}
	s := p6OriginalTables(t)
	player := p6Actor("hero_047", "percentage-source-player", "047_1", 3, 20000)
	enemy := p6Actor("hero_047", "percentage-source-enemy", "047_1", 7, 20000)
	baseline, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{player}, Inst: &pb.BattlePlayerInst{}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{enemy}, Inst: &pb.BattleMonsterInst{}}, Seed: 42, MaxRound: 4, BattleType: 4})
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.ComputeFrozenBattle(t.Context(), baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	variant, err := cloneP6CalculationInput(baseline)
	if err != nil {
		t.Fatal(err)
	}
	variant.Attacker.Formation[0].Prop.Peratt = proto.Float64(75)
	variant.Attacker.Formation[0].Prop.Perhp = proto.Float64(-13)
	variant.Monster.Formation[0].Prop.Perpdef = proto.Float64(125)
	variant.Monster.Formation[0].Prop.Permdef = proto.Float64(-9)
	before, err := json.Marshal(variant)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ComputeFrozenBattle(t.Context(), variant, nil)
	if err != nil || got.Winner != want.Winner || got.FinalRound != want.FinalRound || got.RandomDraws != want.RandomDraws || !proto.Equal(got.Operates, want.Operates) || !reflect.DeepEqual(got.Events, want.Events) || got.Authoritative {
		t.Fatal("upstream percentage metadata changed original flat-panel math or rejected source", err)
	}
	after, err := json.Marshal(variant)
	if err != nil || string(before) != string(after) || got.Attacker.Formation[0].Prop.GetPeratt() != 75 || got.Monster.Formation[0].Prop.GetPermdef() != -9 {
		t.Fatal("ignored original metadata must remain frozen and visible in source audit", err)
	}
	variant.Attacker.Formation[0].Prop.Perhp = proto.Float64(math.NaN())
	if partial, badErr := s.ComputeFrozenBattle(t.Context(), variant, nil); badErr == nil || partial != nil {
		t.Fatal("non-finite ignored metadata accepted into frozen calculation")
	}
}
