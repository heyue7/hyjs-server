package service

import (
	"encoding/hex"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
)

// TestP6NativeDailyID4Diagnostic compares an explicitly authorized read-only device record.
// A successful diagnostic run never marks a native mismatch as parity or opens rewards.
func TestP6NativeDailyID4Diagnostic(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") != "1" {
		t.Skip("explicit real-record diagnostic disabled")
	}
	p6CollectNativeDailyDiagnostic(t, "daily-id4-native-20261013.json", "95efecfe-ac5a-44c5-bf8c-69f7e859bb31", "daily-id4-native-diagnostic-20261013.json")
}

func TestP6NativeFoodDarkID4Diagnostic(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") != "1" {
		t.Skip("explicit real-record diagnostic disabled")
	}
	p6CollectNativeDailyDiagnostic(t, "daily-fooddark-id4-native-20261013.json", "42ac0bba-d5fd-472e-a54a-83fa53002dcd", "daily-fooddark-id4-native-diagnostic-20261013.json")
}

func p6CollectNativeDailyDiagnostic(t *testing.T, inputPath, fightID, outputPath string) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/" + inputPath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FightID  string `json:"fight_id"`
		PlayerID string `json:"player_id"`
		StartHex string `json:"start_hex"`
		EndHex   string `json:"end_hex"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.FightID != fightID || fixture.PlayerID != "4" {
		t.Fatal("unexpected native record")
	}
	startBytes, err := hex.DecodeString(fixture.StartHex)
	if err != nil {
		t.Fatal(err)
	}
	endBytes, err := hex.DecodeString(fixture.EndHex)
	if err != nil {
		t.Fatal(err)
	}
	start, end := &pb.SCDailyCopyFightStart{}, &pb.CSDailyCopyFightEnd{}
	if err = proto.Unmarshal(startBytes, start); err != nil {
		t.Fatal(err)
	}
	if err = proto.Unmarshal(endBytes, end); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: start.BattlePlayer, Monster: start.Monster, Seed: start.GetRandSeed(), MaxRound: start.GetMaxRoundNum(), BattleType: 4})
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"fightId": fixture.FightID, "playerId": 4, "nativeIsWin": end.GetIsWin(), "nativeOperate": end.OptRecord, "engineVersion": input.EngineVersion, "configurationHash": input.ConfigurationHash, "nativeCompared": false, "authoritative": false}
	computed, computeErr := s.ComputeFrozenBattle(t.Context(), input, nil)
	if computeErr != nil {
		report["computeError"] = computeErr.Error()
	} else {
		report["computed"] = computed
		report["automaticOperateEqualsNative"] = proto.Equal(computed.Operates, end.OptRecord)
	}
	matched, matchErr := s.ComputeFrozenBattle(t.Context(), input, end.OptRecord)
	if matchErr != nil {
		report["nativeOperationReplayError"] = matchErr.Error()
		report["nativePartialTimeline"] = p6NativePartialTimeline(t, input, end.OptRecord)
	} else {
		report["nativeOperationReplay"] = matched
		report["nativeOperationReplayEquals"] = proto.Equal(matched.Operates, end.OptRecord)
	}
	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("../../analysis/p6/"+outputPath, output, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("diagnostic collected; computeErr=%v, nativeReplayErr=%v; parity gate remains closed", computeErr, matchErr)
}

// Diagnostics retain the actual supplied-operation prefix when public computation
// rejects a later command. The frozen input is cloned and no stored battle is changed.
func p6NativePartialTimeline(t *testing.T, input *model.FrozenBattleInput, supplied *pb.BattleOperate) map[string]any {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var frozen model.FrozenBattleInput
	if err = json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	e := &combatRoundEngine{service: &GameService{Tables: frozen.Tables}, input: &frozen, rng: NewCombatRandom(frozen.Seed), operate: &pb.BattleOperate{}, supplied: supplied}
	err = e.initialize()
	if err == nil {
		for _, p := range e.players {
			for _, a := range p.actors {
				if a.state == 4 {
					continue
				}
				if err = e.talentTimer(a, nil, 1); err != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = e.validateOperates(supplied)
	}
	for e.round = 1; err == nil && e.round <= frozen.MaxRound; e.round++ {
		e.camp = (e.round - 1) % 2
		if frozen.DefenderFirst {
			e.camp = 1 - e.camp
		}
		if err = e.beginRound(); err != nil {
			break
		}
		for ticks := 0; ticks < 1200; ticks++ {
			if err = e.step(); err != nil {
				break
			}
			if !e.alive(0) || !e.alive(1) || e.canFinishRound() {
				break
			}
			e.time += .1
		}
		if err != nil {
			break
		}
		if err = e.endRound(); err != nil || !e.alive(0) || !e.alive(1) {
			break
		}
	}
	report := map[string]any{"round": e.round, "roundTime": e.time, "acceptedOperations": e.suppliedIndex, "randomDraws": e.rng.Draws(), "events": e.events, "nativeCompared": false, "authoritative": false}
	if err != nil {
		report["error"] = err.Error()
	}
	if e.players[0] != nil && e.players[1] != nil {
		report["anger"] = []float64{e.players[0].anger, e.players[1].anger}
		report["hp"] = []float64{e.totalHP(0), e.totalHP(1)}
	}
	return report
}

// The main-copy record uses its original frozen tables, retaining source version separately.
func p6NativeRedMainFixture(t *testing.T) (model.FrozenBattleInput, *pb.BattleOperate) {
	return p6NativeMainFixture(t, "red-main-id4-native-20261013.json", "red-main-id4-native-input-20261013.json", "f01d16a3288d2ddc9efdab3495f82b0fcb2d8733b6841172eeab4acd93abe7f5")
}

func p6NativeMainFixture(t *testing.T, manifestPath, inputPath, recordID string) (model.FrozenBattleInput, *pb.BattleOperate) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/" + manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ID          string `json:"id"`
		InputPath   string `json:"inputPath"`
		OperatesHex string `json:"operates_hex"`
		InputHash   string `json:"inputHash"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ID != recordID || fixture.InputPath != inputPath {
		t.Fatal("unexpected main native record")
	}
	raw, err = os.ReadFile("../../analysis/p6/" + fixture.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	if p6Digest(raw) != fixture.InputHash {
		t.Fatal("original snapshot hash changed")
	}
	var original model.FrozenBattleInput
	if err = json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	operateBytes, err := hex.DecodeString(fixture.OperatesHex)
	if err != nil {
		t.Fatal(err)
	}
	native := &pb.BattleOperate{}
	if err = proto.Unmarshal(operateBytes, native); err != nil {
		t.Fatal(err)
	}
	return original, native
}

func TestP6NativeRedMainID4Diagnostic(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") != "1" {
		t.Skip("explicit real-record diagnostic disabled")
	}
	original, native := p6NativeRedMainFixture(t)
	p6CollectNativeMainDiagnostic(t, original, native, "e6d9aecece5f5d8e3330566f", "red-main-id4-native-diagnostic-20261013.json")
}

func TestP6NativeRedMainTask5ID4Diagnostic(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") != "1" {
		t.Skip("explicit real-record diagnostic disabled")
	}
	original, native := p6NativeMainFixture(t, "red-main-task5-id4-native-20261013.json", "red-main-task5-id4-native-input-20261013.json", "5ca6c5562082cb222d35e2a50efc8c793757f56a5fc8a5bf022d1a38fdb6a3f5")
	p6CollectNativeMainDiagnostic(t, original, native, "e0e774cc6917e2309c08e71c", "red-main-task5-id4-native-diagnostic-20261013.json")
}

func p6CollectNativeMainDiagnostic(t *testing.T, original model.FrozenBattleInput, native *pb.BattleOperate, fightID, outputPath string) {
	t.Helper()
	s := &GameService{Tables: original.Tables}
	input, err := s.FreezeBattleInput(original)
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"fightId": fightID, "playerId": 4, "nativeOperate": native, "sourceEngineVersion": original.EngineVersion, "engineVersion": input.EngineVersion, "configurationHash": input.ConfigurationHash, "nativeCompared": false, "authoritative": false}
	computed, computeErr := s.ComputeFrozenBattle(t.Context(), input, nil)
	if computeErr != nil {
		report["computeError"] = computeErr.Error()
	} else {
		report["computed"] = computed
		report["automaticOperateEqualsNative"] = p6OriginalOperateEqual(computed.Operates, native)
	}
	matched, matchErr := s.ComputeFrozenBattle(t.Context(), input, native)
	if matchErr != nil {
		report["nativeOperationReplayError"] = matchErr.Error()
		report["nativePartialTimeline"] = p6NativePartialTimeline(t, input, native)
	} else {
		report["nativeOperationReplay"] = matched
		report["nativeOperationReplayEquals"] = p6OriginalOperateEqual(matched.Operates, native)
	}
	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("../../analysis/p6/"+outputPath, output, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("main native diagnostic; computeErr=%v replayErr=%v; parity gate remains closed", computeErr, matchErr)
}
