package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This is the SHA-bound original-host playback used by the counterstrike
// oracle, frozen against the complete original combat table fixture.
func p6ReadOnlyCalculationTablesRound2Fixture(tb testing.TB) (*GameService, *model.FrozenBattleInput) {
	tb.Helper()
	encoded := "Cidjb3VudGVyc3RyaWtlLXNjYWxhci1vcmlnaW5hbC1vcmFjbGUtcjIQBxgCIucBCGUQARgBIh1maW5pdGUgb3JpZ2luYWwgY291bnRlcnN0cmlrZSq0AQoI" +
		"aGVyb18wNDcSEGNvdW50ZXItYXR0YWNrZXIaCHRhbF8zMDQwIgcwNDdfMSwsKmMJAAAAAACIw0ARAAAAAICELkEZAAAAAAAAAAAhAAAAAAAAAABJAAAAAAAA" +
		"JEBRAAAAAADAV0BZAAAAAAAAAABhAAAAAAAAFEBpAAAAAAAAAABxAAAAAACAQUB5AAAAAAAASUAwATgDQgkJAAAAAICELkFQAVkAAAAAAABZQGABagBwADIJ" +
		"CQAAAAAAAAAAKuMBCGYQARgBIh1maW5pdGUgb3JpZ2luYWwgY291bnRlcnN0cmlrZSqwAQoIaGVyb18wNDcSDWNvdW50ZXItb3duZXIaB3RhbF8zNTMiBzA0" +
		"N18xLCwqYwkAAAAAAEB/QBEAAAAAgIQuQRkAAAAAAAAAACEAAAAAAAAAAEkAAAAAAAAkQFEAAAAAAMBXQFkAAAAAAAAAAGEAAAAAAAAUQGkAAAAAAAAAAHEA" +
		"AAAAAAAAAHkAAAAAAABJQDABOANCCQkAAAAAgIQuQVABWQAAAAAAAFlAYAFqAHAAMgkJAAAAAAAAAABZAAAAAAAA8D9hAAAAAAAA8D9oDA=="
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || p6Digest(raw) != "8c9233bb5d10b8774467281d3ddd534c212db51699a75732c55be45b32fe46ca" {
		tb.Fatalf("original-host playback changed: %v", err)
	}
	wire := &pb.SCFightPlayback{}
	if err := proto.Unmarshal(raw, wire); err != nil {
		tb.Fatal(err)
	}
	p6OriginalTestCache.once.Do(func() {
		p6OriginalTestCache.tables, p6OriginalTestCache.err = readP6OriginalTestTables()
	})
	if p6OriginalTestCache.err != nil {
		tb.Fatal(p6OriginalTestCache.err)
	}
	s := &GameService{Tables: cloneP6OriginalTestTables(p6OriginalTestCache.tables)}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{
		Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(),
		MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(),
		AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor,
	})
	if err != nil {
		tb.Fatal(err)
	}
	if len(input.Tables["Talent"]) != 780 || len(input.Tables["SkillCast"]) != 2470 {
		tb.Fatalf("original table fixture changed: Talent=%d SkillCast=%d", len(input.Tables["Talent"]), len(input.Tables["SkillCast"]))
	}
	return s, input
}

func p6ReadOnlyCalculationTablesRound2JSON(tb testing.TB, value any) []byte {
	tb.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		tb.Fatal(err)
	}
	return raw
}

func TestP6ReadOnlyCalculationTablesRound2CloneIsolation(t *testing.T) {
	_, input := p6ReadOnlyCalculationTablesRound2Fixture(t)
	input.WinConditions = []*pb.BattleWinCondition{{WinKey: proto.Int32(0), WinValue: proto.Int32(7)}, nil}
	input.Monster = &pb.BattleMonster{Formation: []*pb.BattleHero{proto.Clone(input.Defender.Formation[0]).(*pb.BattleHero)}, Inst: &pb.BattleMonsterInst{}}
	input.KFReplay = &pb.KFReplayData{PlayerList: []*pb.BattlePlayer{input.Attacker, input.Defender}}
	input.KFReplayViewer = &model.FrozenP6KFViewer{PlayerID: 42, ServerID: 7}
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(2.5)
	input.AttackerDamageFactor, input.DefenderDamageFactor = proto.Float64(0), proto.Float64(1)
	input.AppliedProperties = map[string][]string{"owner": {"hp", "attack"}, "nil": nil, "empty": {}}
	input.Tables["__nil"] = nil
	input.Tables["__empty"] = []map[string]string{}
	input.Tables["__rows"] = []map[string]string{nil, {"value": "original"}}
	input.Attacker.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})

	first, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Tables, input.Tables) || !reflect.DeepEqual(first.AppliedProperties, input.AppliedProperties) || !proto.Equal(first.Attacker, input.Attacker) {
		t.Fatal("full clone changed frozen table or metadata shapes")
	}
	first.Tables["__rows"][1]["value"] = "full-clone-only"
	first.Tables["__empty"] = append(first.Tables["__empty"], map[string]string{"new": "row"})
	if input.Tables["__rows"][1]["value"] != "original" || second.Tables["__rows"][1]["value"] != "original" || len(input.Tables["__empty"]) != 0 || len(second.Tables["__empty"]) != 0 {
		t.Fatal("full clones share table maps or slices")
	}

	metadata, err := cloneP6CalculationMetadata(input)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(metadata.Attacker, input.Attacker) || !proto.Equal(metadata.Defender, input.Defender) || !proto.Equal(metadata.Monster, input.Monster) || !proto.Equal(metadata.KFReplay, input.KFReplay) || !proto.Equal(metadata.WinConditions[0], input.WinConditions[0]) || metadata.WinConditions[1] != nil || !reflect.DeepEqual(metadata.AppliedProperties, input.AppliedProperties) {
		t.Fatal("metadata clone changed protobuf or nil/empty values")
	}
	metadata.Tables["__shared"] = []map[string]string{{"probe": "shared"}}
	if _, ok := input.Tables["__shared"]; !ok {
		t.Fatal("metadata clone did not reuse the frozen table map")
	}
	delete(metadata.Tables, "__shared")
	metadata.Tables["__rows"][1]["value"] = "shared-row"
	if input.Tables["__rows"][1]["value"] != "shared-row" {
		t.Fatal("metadata clone did not reuse the frozen row")
	}
	input.Tables["__rows"][1]["value"] = "original"

	beforeAttacker := proto.Clone(input.Attacker)
	beforeDefender := proto.Clone(input.Defender)
	beforeMonster := proto.Clone(input.Monster)
	beforeReplay := proto.Clone(input.KFReplay)
	metadata.Attacker.Formation[0].Inst.CurHp = proto.Float64(1)
	metadata.Defender.Formation[0].Inst.CurHp = proto.Float64(2)
	metadata.Monster.Formation[0].Inst.CurHp = proto.Float64(3)
	metadata.KFReplay.PlayerList[0].Formation[0].Inst.CurHp = proto.Float64(4)
	metadata.KFReplayViewer.PlayerID = 99
	metadata.WinConditions[0].WinValue = proto.Int32(8)
	*metadata.DiagnosticManualAutoStartRoundTime = 9
	*metadata.AttackerDamageFactor = 5
	*metadata.DefenderDamageFactor = 6
	metadata.AppliedProperties["owner"][0] = "changed"
	if !proto.Equal(input.Attacker, beforeAttacker) || !proto.Equal(input.Defender, beforeDefender) || !proto.Equal(input.Monster, beforeMonster) || !proto.Equal(input.KFReplay, beforeReplay) || input.KFReplayViewer.PlayerID != 42 || input.WinConditions[0].GetWinValue() != 7 || *input.DiagnosticManualAutoStartRoundTime != 2.5 || *input.AttackerDamageFactor != 0 || *input.DefenderDamageFactor != 1 || input.AppliedProperties["owner"][0] != "hp" {
		t.Fatal("metadata clone aliases per-calculation mutable state")
	}
	for _, clone := range []func(*model.FrozenBattleInput) (*model.FrozenBattleInput, error){cloneP6CalculationInput, cloneP6CalculationMetadata} {
		if _, err := clone(nil); err == nil {
			t.Fatal("nil input was accepted")
		}
		empty, err := clone(&model.FrozenBattleInput{})
		if err != nil || empty.Tables != nil || empty.AppliedProperties != nil || empty.WinConditions != nil {
			t.Fatalf("nil collections were not preserved: %v", err)
		}
	}
}

func TestP6ReadOnlyCalculationTablesRound2RejectNonfinite(t *testing.T) {
	inputs := []*model.FrozenBattleInput{
		{AttackerDamageFactor: proto.Float64(math.NaN())},
		{KFReplay: &pb.KFReplayData{PlayerList: []*pb.BattlePlayer{{Formation: []*pb.BattleHero{{Inst: &pb.BattleHeroInst{CurHp: proto.Float64(math.NaN())}}}}}}},
	}
	for _, input := range inputs {
		for _, clone := range []func(*model.FrozenBattleInput) (*model.FrozenBattleInput, error){cloneP6CalculationInput, cloneP6CalculationMetadata} {
			if _, err := clone(input); err == nil {
				t.Fatal("nonfinite metadata was accepted")
			}
		}
	}
}

func TestP6ReadOnlyCalculationTablesRound2NativeInputUnchanged(t *testing.T) {
	s, input := p6ReadOnlyCalculationTablesRound2Fixture(t)
	before := p6ReadOnlyCalculationTablesRound2JSON(t, input)
	base, err := s.ComputeFrozenBattle(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, p6ReadOnlyCalculationTablesRound2JSON(t, input)) {
		t.Fatal("core calculation changed the frozen input")
	}
	terminal, err := s.ComputeFrozenBattleFinalHP(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, p6ReadOnlyCalculationTablesRound2JSON(t, input)) {
		t.Fatal("final HP calculation changed the frozen input")
	}
	if terminal.Outcome == nil || base.Winner != terminal.Outcome.Winner || base.RandomDraws != terminal.Outcome.RandomDraws || !proto.Equal(base.Operates, terminal.Outcome.Operates) || !proto.Equal(base.Attacker, terminal.Outcome.Attacker) || !proto.Equal(base.Defender, terminal.Outcome.Defender) {
		t.Fatal("core and final HP calculations disagree on the same original playback")
	}
	var hits int
	for _, event := range base.Events {
		if event.Kind == "counterstrike" && event.Round >= 7 {
			hits++
			if math.Float64bits(event.Value) != math.Float64bits(65) {
				t.Fatal("counterstrike differs from the original host")
			}
		}
	}
	if hits != 11 {
		t.Fatalf("original-host counterstrike count changed: %d", hits)
	}
}

func TestP6ReadOnlyCalculationTablesRound2Concurrent(t *testing.T) {
	s, input := p6ReadOnlyCalculationTablesRound2Fixture(t)
	before := p6ReadOnlyCalculationTablesRound2JSON(t, input)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type outcome struct {
		value *model.CombatFinalHPOutcome
		err   error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, err := s.ComputeFrozenBattleFinalHP(ctx, input, nil)
			results <- outcome{value: result, err: err}
		}()
	}
	defer workers.Wait()
	close(start)
	var actual [2]*model.CombatFinalHPOutcome
	for i := range actual {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			actual[i] = result.value
		case <-ctx.Done():
			t.Fatal("concurrent original playback timed out")
		}
	}
	second := p6ReadOnlyCalculationTablesRound2JSON(t, actual[1])
	if !bytes.Equal(p6ReadOnlyCalculationTablesRound2JSON(t, actual[0]), second) || !bytes.Equal(before, p6ReadOnlyCalculationTablesRound2JSON(t, input)) {
		t.Fatal("concurrent calculations differ or changed the frozen input")
	}
	if actual[0].Outcome == nil || actual[1].Outcome == nil || actual[0].Outcome.Attacker == actual[1].Outcome.Attacker || len(actual[0].FinalActors) == 0 {
		t.Fatal("concurrent calculations did not create independent actors")
	}
	actual[0].Outcome.Attacker.Formation[0].Inst.CurHp = proto.Float64(-1)
	actual[0].FinalActors[0].CurrentHP = -1
	if !bytes.Equal(second, p6ReadOnlyCalculationTablesRound2JSON(t, actual[1])) || !bytes.Equal(before, p6ReadOnlyCalculationTablesRound2JSON(t, input)) {
		t.Fatal("changing one result altered the other result or the input")
	}
}

var p6ReadOnlyCalculationTablesRound2BenchmarkSink *model.FrozenBattleInput

func BenchmarkP6ReadOnlyCalculationTablesRound2(b *testing.B) {
	_, input := p6ReadOnlyCalculationTablesRound2Fixture(b)
	b.Logf("original table configuration=%s Talent=%d SkillCast=%d", input.ConfigurationHash, len(input.Tables["Talent"]), len(input.Tables["SkillCast"]))
	for _, test := range []struct {
		name  string
		clone func(*model.FrozenBattleInput) (*model.FrozenBattleInput, error)
	}{{"full_tables", cloneP6CalculationInput}, {"metadata_only", cloneP6CalculationMetadata}} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				p6ReadOnlyCalculationTablesRound2BenchmarkSink, err = test.clone(input)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
