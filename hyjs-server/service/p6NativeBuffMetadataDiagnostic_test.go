package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original metadata comparisons keep the exact source identity separate from remaining shields.
func TestP6NativePriorityBuffSourceAndValuesDiagnostic(t *testing.T) {
	output := os.Getenv("HYJS_P6_NATIVE_BUFF_METADATA_OUTPUT")
	if output == "" {
		t.Skip("explicit original Buff metadata diagnostic output required")
	}
	p6NativePriorityBuffSourceAndValues(t, output, false)
}

func TestP6NativePriorityOriginalBuffSourceAndValuesGolden(t *testing.T) {
	p6NativePriorityBuffSourceAndValues(t, "", true)
}

func p6NativePriorityBuffSourceAndValues(t *testing.T, output string, strict bool) {
	p6NativeBuffSourceAndValuesFromFixture(t, output, strict, "native-pvp-priority-buff-metadata-golden-v51.json", "18c484b091f159a3d0986afbf7395cc86235e04646b00eb40e8c604b19a40492")
}

func p6NativeBuffSourceAndValuesFromFixture(t *testing.T, output string, strict bool, file, expectedSHA string, expectedCases ...int) {
	t.Helper()
	raw, wireRoot := p6NativeDiagnosticFixture(t, file, expectedSHA, "HYJS_P6_NATIVE_NEXT_METADATA_FIXTURE", strict)
	var err error
	type actorState struct {
		UID   string           `json:"uid"`
		Buffs []map[string]any `json:"fullBuffMetadata"`
	}
	type tickState struct {
		Round   int32   `json:"round"`
		Time    float64 `json:"time"`
		Players []struct {
			Actors []actorState `json:"actors"`
		} `json:"players"`
	}
	var original struct {
		Cases []struct {
			Name  string      `json:"case"`
			Wire  string      `json:"wire"`
			SHA   string      `json:"wireSHA256"`
			Ticks []tickState `json:"ticks"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &original); err != nil || len(original.Cases) != p6NativeDiagnosticCaseCount(t, expectedCases...) {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	var reports []map[string]any
	for _, c := range original.Cases {
		wire, readErr := readTestAnalysis75(filepath.Join(wireRoot, c.Wire))
		if readErr != nil || p6Digest(wire) != c.SHA {
			t.Fatal("original Buff metadata wire", readErr)
		}
		playback := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(wire, playback); err != nil {
			t.Fatal(err)
		}
		input, freezeErr := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}, c.SHA)
		if freezeErr != nil {
			t.Fatal(freezeErr)
		}
		wanted := map[string]tickState{}
		for _, tick := range c.Ticks {
			wanted[fmt.Sprintf("%d/%d", tick.Round, int64(math.Round(tick.Time*10)))] = tick
		}
		matched := 0
		drifts := map[string]any{}
		out, computeErr := p6NativeGoldenComputeProjection(t, s, input, c.SHA, expectedSHA, playback.OptRecord, func(e *combatRoundEngine) {
			key := fmt.Sprintf("%d/%d", e.round, int64(math.Round(e.time*10)))
			tick, found := wanted[key]
			if !found {
				return
			}
			matched++
			for camp, p := range tick.Players {
				for index, aNative := range p.Actors {
					a := e.players[camp].actors[index]
					if a.uid != aNative.UID {
						t.Fatal("original metadata actor/buff denominator")
					}
					if len(a.buffs) != len(aNative.Buffs) {
						if strict {
							t.Fatal("original metadata Buff denominator")
						}
						if drifts["buffCount"] == nil {
							drifts["buffCount"] = map[string]any{"tick": key, "actor": a.uid, "native": len(aNative.Buffs), "go": len(a.buffs)}
						}
					}
					for index, b := range a.buffs {
						if index >= len(aNative.Buffs) {
							break
						}
						native := aNative.Buffs[index]
						actual := map[string]any{"id": b.row["Id"], "factor": b.row["Factor"], "group": b.row["Group"], "param": b.row["Param"], "isDebuff": b.row["IsDebuff"] == "1", "canBeCleared": b.row["CanBeCleared"] == "1", "shouldCover": b.row["ShouldCover"] == "1", "onceData": b.once, "val": p6OriginalBuffMetadataVal(b), "val2": p6OriginalBuffMetadataVal2(b), "loadBuffRound": float64(b.loadedRound), "sourceTalent": b.origin.talent, "sourceSkill": b.origin.skill, "sourceCast": b.origin.cast, "sourceActor": b.source.uid}
						for key, tableKey := range map[string]string{"args1": "Args1", "args2": "Args2", "args3": "Args3", "totalCount": "TotalCount", "totalTime": "TotalTime"} {
							actual[key] = tableNumber(b.row, tableKey)
						}
						// Buff.Init separates the original decimal class digit and
						// its one-use flag before retaining exported metadata.
						class := int32(tableNumber(b.row, "BuffClass"))
						actual["buffClass"] = float64(class % 10)
						actual["buffFlag"] = float64(class / 10)
						for field, want := range native {
							have, found := actual[field]
							same := found
							if numeric, isNumber := want.(float64); isNumber {
								value, isFloat := have.(float64)
								same = same && isFloat && math.Abs(numeric-value) <= 1e-6
							} else {
								same = same && want == have
							}
							if !same && drifts[field] == nil {
								drifts[field] = map[string]any{"tick": key, "actor": a.uid, "buff": b.row["Id"], "native": want, "go": have}
							}
						}
					}
				}
			}
		})
		if computeErr != nil || strict && matched != len(wanted) || out.Authoritative {
			t.Fatal("metadata clock denominator", computeErr, matched, len(wanted))
		}
		if strict && len(drifts) != 0 {
			t.Fatalf("original buff source/values %s %v", c.Name, drifts)
		}
		reports = append(reports, map[string]any{"case": c.Name, "matchedTicks": matched, "nativeTicks": len(wanted), "missingTicks": len(wanted) - matched, "firstDrifts": drifts, "originalFixtureSHA256": p6Digest(raw), "authoritative": false, "nativeCompared": false})
	}
	if output == "" {
		return
	}
	encoded, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

// These observed factory branches explicitly assign Buff.val. Direct scalar modifiers
// use Args fields and leave the base Buff.val at zero; remaining shield is separate.
func p6OriginalBuffMetadataVal(b *combatRoundBuff) float64 {
	if p6HPTransferKey(b.row["Factor"]) != "" {
		return b.initialValue * .01
	}
	if int(tableNumber(b.row, "BuffClass"))%10 == 1 {
		return b.initialValue
	}
	switch b.row["Factor"] {
	case "poisonDam", "fireDam", "recoverBySelfRound", "reliveAndinvisible", "permaxHpBySelfRound", "perhp", "permaxHp", "minusMaxHpPercent":
		return b.initialValue
	case "iceYNT":
		return tableNumber(parseP6Condition(b.row["Param"]), "damageAdd") * .01
	}
	return 0
}

func p6OriginalBuffMetadataVal2(b *combatRoundBuff) float64 {
	if b.row["Factor"] == "iceYNT" {
		return tableNumber(parseP6Condition(b.row["Param"]), "treatAdd") * .01
	}
	return 0
}
