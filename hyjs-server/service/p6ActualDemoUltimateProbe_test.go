package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Mirror already saved complete real P5 parties under explicit local player
// identity/initiative/seed diagnostics. Every actor, skill, item-derived panel,
// talent, HP and GUID is retained. This does not claim a real paid opponent.
func TestP6ProbeActualDemoFullPartyUltimateExecutions(t *testing.T) {
	output := os.Getenv("HYJS_P6_DEMO_ULTIMATE_PROBE_OUTPUT")
	if output == "" {
		t.Skip("explicit offline original-source mechanism probe only")
	}
	root, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	output, err = filepath.Abs(output)
	if err != nil || filepath.Dir(output) != root {
		t.Fatal("probe summary must be directly under analysis/p6")
	}
	source, err := readTestAnalysis75("../../analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.frozen-input.json")
	if err != nil || p6Digest(source) != "d0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94" {
		t.Fatal("immutable actual source", err)
	}
	var actual model.FrozenBattleInput
	if err = json.Unmarshal(source, &actual); err != nil {
		t.Fatal(err)
	}
	s := &GameService{Tables: actual.Tables}
	var rows []map[string]any
	exportFolder := os.Getenv("HYJS_P6_DEMO_ULTIMATE_SOURCE_OUTPUT")
	var exportRows []map[string]any
	if exportFolder != "" {
		exportFolder, err = filepath.Abs(exportFolder)
		if err != nil || filepath.Dir(exportFolder) != root {
			t.Fatal("new source folder directly under analysis/p6 required")
		}
		if _, err = os.Stat(exportFolder); !os.IsNotExist(err) {
			t.Fatal("preserve existing source folder", err)
		}
		if err = os.Mkdir(exportFolder, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for partyIndex, party := range []*pb.BattlePlayer{actual.Attacker, actual.Defender} {
		for _, seed := range []int64{7, 314159} {
			for _, defenderFirst := range []bool{false, true} {
				input, err := cloneP6CalculationInput(&actual)
				if err != nil {
					t.Fatal(err)
				}
				input.Attacker = proto.Clone(party).(*pb.BattlePlayer)
				input.Defender = proto.Clone(party).(*pb.BattlePlayer)
				input.Attacker.PlayerId = proto.Int64(4)
				input.Defender.PlayerId = proto.Int64(3)
				input.Seed = seed
				input.DefenderFirst = defenderFirst
				if err = SetP6FirstOrder(input.Attacker, input.Defender, defenderFirst); err != nil {
					t.Fatal(err)
				}
				input.AppliedProperties = map[string][]string{"immutableActualActorSource": {p6Digest(source)}, "explicitLocalDiagnostic": {fmt.Sprintf("complete party %d mirrored; no actor edits; identity4/3 first%v seed%d", partyIndex, defenderFirst, seed)}}
				input, err = s.FreezeBattleInput(*input)
				if err != nil {
					t.Fatal(err)
				}
				row := map[string]any{"partyIndex": partyIndex, "seed": seed, "defenderFirst": defenderFirst, "actualActorSourceSHA256": p6Digest(source)}
				result, computeErr := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
				if computeErr != nil {
					row["computeError"] = computeErr.Error()
				} else {
					begins := map[string]int{}
					queued := map[string]int{}
					buffs := map[string]int{}
					for _, event := range result.Outcome.Events {
						if event.Kind == "skill_begin" {
							begins[event.Skill]++
						}
						if event.Kind == "skill_press" {
							queued[event.Skill]++
						}
						if event.Buff != "" {
							buffs[event.Buff]++
						}
					}
					row["skillBegins"] = begins
					row["queuedPresses"] = queued
					row["buffs"] = buffs
					row["finalRound"] = result.Outcome.FinalRound
					row["winner"] = result.Outcome.Winner
					row["RNGCalls"] = result.Outcome.RandomDraws
					row["authoritative"] = result.Outcome.Authoritative
				}
				rows = append(rows, row)
				// Four candidates keep both complete actual parties and exercise
				// both seed/initiative directions without eight redundant mirrors.
				if exportFolder != "" && ((seed == 7 && !defenderFirst) || (seed == 314159 && defenderFirst)) {
					name := fmt.Sprintf("%03d", len(exportRows))
					fightID := "readonly-actual-ultimate-mirror-" + name
					callback := &pb.SCFriendFight{FightId: proto.String(fightID), RandSeed: proto.Int64(seed), RoomId: proto.String("map_forest_001"), MaxRoundNum: proto.Int32(40), PlayerList: []*pb.BattlePlayer{input.Attacker, input.Defender}}
					wire := &pb.SCFightPlayback{Fightid: proto.String(fightID), RandSeed: proto.Int64(seed), BattleType: proto.Int32(11), Attacker: input.Attacker, Defender1: input.Defender, MaxRound: proto.Int32(40), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
					write := func(suffix string, value any) string {
						var data []byte
						var e error
						if message, ok := value.(proto.Message); ok {
							data, e = proto.MarshalOptions{Deterministic: true}.Marshal(message)
						} else {
							data, e = json.MarshalIndent(value, "", "  ")
						}
						if e != nil {
							t.Fatal(e)
						}
						if e = os.WriteFile(filepath.Join(exportFolder, name+suffix), data, 0644); e != nil {
							t.Fatal(e)
						}
						return p6Digest(data)
					}
					first := 0
					if defenderFirst {
						first = 1
					}
					exportRows = append(exportRows, map[string]any{"case": fmt.Sprintf("complete-actual-party%d-mirror/seed%d/defenderFirst%v", partyIndex, seed, defenderFirst), "fightId": fightID, "seed": seed, "firstCamp": first, "battleType": 11, "wire": name + ".pb", "wireSHA256": write(".pb", wire), "originalCallback": name + ".SCFriendFight.pb", "originalCallbackSHA256": write(".SCFriendFight.pb", callback), "frozenInput": name + ".frozen-input.json", "frozenInputSHA256": write(".frozen-input.json", input), "goCandidate": row, "nativeScope": "original-pvp-automatic-ai", "status": "awaiting-original-automatic-ai"})
				}
			}
		}
	}
	if exportFolder != "" {
		manifest := map[string]any{"scope": "Immutable actual128b complete P5 parties mirrored as explicit local diagnostic opponents; only player IDs4/3, seed and initiative selected. Every actual actor/GUID/three skills/gear-derived panel/talent/currentHP retained. No original private opponent, actual paid match, formal SELECT/write, expected winner or ultimate execution proof claimed. Go candidate begins are predictions to be tested by original runtime.", "sourceActualInput": "analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.frozen-input.json", "sourceActualInputSHA256": p6Digest(source), "cases": exportRows, "nativeCompared": false, "authoritative": false}
		data, e := json.MarshalIndent(manifest, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(exportFolder, "manifest.json"), append(data, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
	raw, err := json.MarshalIndent(map[string]any{"scope": "offline complete actual P5 party mirrors; candidate mechanism observations only, not original runtime or paid match proof", "cases": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
