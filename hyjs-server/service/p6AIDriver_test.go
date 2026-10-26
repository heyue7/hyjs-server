package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalAIDriverUsesBattleTypeAndPreservesDiagnosticSource(t *testing.T) {
	raw, err := readTestAnalysis75("../../analysis/p6/ai-driver-selection-v93-original.il.txt")
	if err != nil || p6Digest(raw) != "e5b72c71143248e1280c5539792a930a223e07574959ea9d152cba461885346d" {
		t.Fatal("original mode driver source", err)
	}
	for _, term := range []string{"01e4 ldc.i4.s 14", "01e6 beq 509", "01fd newobj Battle.Engine.AI.EnginePlayerAI::.ctor", "0212 newobj Battle.Engine.AI.EnginePlayerAI::.ctor", "0131 newobj Battle.Engine.AI.EngineCopyEnemyAI::.ctor"} {
		if !strings.Contains(string(raw), term) {
			t.Fatal("original UF14 both PlayerAI proof", term)
		}
	}
	s := p6OriginalTables(t)
	player := p6Actor("hero_047", "driver-player", "047_1,047_2,047_3", 3, 80000)
	monster := p6Actor("monster_016", "driver-monster", "1016_1", 3, 1000)
	f, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{player}, Inst: &pb.BattlePlayerInst{}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{monster}, Inst: &pb.BattleMonsterInst{}}, BattleType: 14, Seed: 42, MaxRound: 8, AttackerDamageFactor: proto.Float64(0), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(f)
	actual, err := s.ComputeFrozenBattle(t.Context(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := cloneP6CalculationInput(f)
	if err != nil {
		t.Fatal(err)
	}
	baseline.Defender = &pb.BattlePlayer{PlayerId: proto.Int64(43), Formation: baseline.Monster.Formation, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(baseline.Monster.Inst.GetAnger())}}
	baseline.Monster = nil
	expected, err := s.ComputeFrozenBattle(t.Context(), baseline, nil)
	if err != nil || !proto.Equal(actual.Operates, expected.Operates) || actual.RandomDraws != expected.RandomDraws || !reflect.DeepEqual(actual.Events, expected.Events) || actual.Authoritative {
		t.Fatal("UF14 Monster payload selected CopyEnemy rather than original PlayerAI", err)
	}
	historical, err := cloneP6CalculationInput(f)
	if err != nil {
		t.Fatal(err)
	}
	historical.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticUF14CopyAI
	old, err := s.ComputeFrozenBattle(t.Context(), historical, nil)
	if err != nil || old.Authoritative {
		t.Fatal("explicit historical guard diagnostic driver", err)
	}
	if reflect.DeepEqual(actual.Events, old.Events) {
		t.Fatal("UF14 original/diagnostic drivers were silently conflated")
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), f, actual.Operates)
	if err != nil || !p6CopySeriesJSONEqual(t, actual, replay) {
		t.Fatal("original UF14 automatic whole replay", err)
	}
	after, _ := json.Marshal(f)
	if string(before) != string(after) {
		t.Fatal("AI driver selection mutated frozen source")
	}
	for _, kind := range []int32{0, 1, 4, 6, 7, 8, 9, 10, 13, 16} {
		e := &combatRoundEngine{input: &model.FrozenBattleInput{BattleType: kind}}
		if e.usesCopyEnemyAI(0) || !e.usesCopyEnemyAI(1) {
			t.Fatal("original CopyAI mode", kind)
		}
	}
	for _, kind := range []int32{2, 3, 5, 11, 12, 14, 15, 17, 18, 19, 20} {
		e := &combatRoundEngine{input: &model.FrozenBattleInput{BattleType: kind, Monster: &pb.BattleMonster{}}}
		if e.usesCopyEnemyAI(0) || e.usesCopyEnemyAI(1) {
			t.Fatal("original both PlayerAI mode", kind)
		}
	}
	bad, err := cloneP6CalculationInput(historical)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"unknown", "wrong-type", "no-monster", "compat"} {
		copy, err := cloneP6CalculationInput(bad)
		if err != nil {
			t.Fatal(err)
		}
		switch variant {
		case "unknown":
			copy.DiagnosticAIDriverPolicy = "unknown"
		case "wrong-type":
			copy.BattleType = 1
		case "no-monster":
			copy.Monster = nil
		case "compat":
			copy.PVECompatibilityRule = constUtil.P6LocalPVENoTargetFinish
		}
		if ValidateP6AIDriverPolicy(copy) == nil {
			t.Fatal("unbound diagnostic driver policy", variant)
		}
	}
}

func TestP6NativeGuardDiagnosticDriverRequiresExactFixtureAndPB(t *testing.T) {
	s := p6OriginalTables(t)
	f, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{p6Actor("hero_047", "driver-scope-a", "047_1", 3, 80000)}, Inst: &pb.BattlePlayerInst{}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{p6Actor("monster_016", "driver-scope-d", "1016_1", 3, 1000)}, Inst: &pb.BattleMonsterInst{}}, BattleType: 14, Seed: 42, MaxRound: 4})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := s.ComputeFrozenBattle(t.Context(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"3d69ec109c3d37c6817c871c8e83d14b5924ad96eb843163838b56fd75b5aea8", "new-original-wrapper-fixture"}, {"different-PB", "03b850f2f0b2026b1b4726b441371804445cdcb85aff714d5c2b9b54dcff4d78"}} {
		result, err := p6NativeGoldenComputeProjection(t, s, f, pair[0], pair[1], nil, nil)
		if err != nil || !p6CopySeriesJSONEqual(t, baseline, result) {
			t.Fatal("PB/fixture partial identity activated diagnostic driver", err)
		}
	}
}
