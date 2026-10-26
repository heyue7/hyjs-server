package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Broader diagnostics expose first original actor/property drifts without opening authority.
func TestP6NativePriorityOriginalActorPropertiesDiagnostic(t *testing.T) {
	output := os.Getenv("HYJS_P6_NATIVE_ACTOR_OUTPUT")
	if output == "" {
		t.Skip("separate original actor/property diagnostic output required")
	}
	p6NativePriorityActorProperties(t, output, false)
}

func TestP6NativePriorityOriginalActorPropertiesGolden(t *testing.T) {
	p6NativePriorityActorProperties(t, "", true)
}

func p6NativePriorityActorProperties(t *testing.T, output string, strict bool) {
	p6NativeActorPropertiesFromFixture(t, output, strict, "native-pvp-priority-actor-golden-v50.json", "d1e71ef02ce2c72f5e8d1585d82f7f1d8379da3615d4993330f80e4867f2afe1")
}

func p6NativeActorPropertiesFromFixture(t *testing.T, output string, strict bool, file, expectedSHA string, expectedCases ...int) {
	t.Helper()
	raw, wireRoot := p6NativeDiagnosticFixture(t, file, expectedSHA, "HYJS_P6_NATIVE_NEXT_ACTOR_FIXTURE", strict)
	var err error
	type actorState struct {
		UID       string         `json:"uid"`
		State     int32          `json:"actorState"`
		Special   int32          `json:"specialState"`
		Unable    []int32        `json:"unableStates"`
		Attribute map[string]any `json:"attribute"`
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
			t.Fatal("original property wire", readErr)
		}
		playback := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(wire, playback); err != nil {
			t.Fatal(err)
		}
		frozen, freezeErr := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}, c.SHA)
		if freezeErr != nil {
			t.Fatal(freezeErr)
		}
		wanted := map[string]tickState{}
		for _, tick := range c.Ticks {
			wanted[fmt.Sprintf("%d/%d", tick.Round, int64(math.Round(tick.Time*10)))] = tick
		}
		drifts := map[string]any{}
		matched := 0
		out, computeErr := p6NativeGoldenComputeProjection(t, s, frozen, c.SHA, expectedSHA, playback.OptRecord, func(e *combatRoundEngine) {
			key := fmt.Sprintf("%d/%d", e.round, int64(math.Round(e.time*10)))
			tick, found := wanted[key]
			if !found {
				return
			}
			matched++
			for camp, p := range tick.Players {
				for index, expected := range p.Actors {
					a := e.players[camp].actors[index]
					if a.uid != expected.UID {
						t.Fatal("actor identity denominator")
					}
					record := func(name string, want, actual any) {
						if !reflect.DeepEqual(want, actual) && drifts[name] == nil {
							drifts[name] = map[string]any{"tick": key, "actor": a.uid, "native": want, "go": actual}
						}
					}
					record("actorState", expected.State, a.state)
					record("specialState", expected.Special, a.speState)
					unable := []int32{}
					for value, on := range a.unableStates {
						if on {
							unable = append(unable, value)
						}
					}
					sort.Slice(unable, func(i, j int) bool { return unable[i] < unable[j] })
					nativeUnable := append([]int32{}, expected.Unable...)
					sort.Slice(nativeUnable, func(i, j int) bool { return nativeUnable[i] < nativeUnable[j] })
					record("unableStates", nativeUnable, unable)
					// DataAttribute.get_value = baseValue*rate+increment (original max is Double.MaxValue).
					attributes := []string{"att", "patt", "matt", "pdef", "mdef", "maxHp", "cure", "heal", "hp", "hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce", "pdamageReduce", "mdamageReduce", "treatAdd", "bleeding", "block", "angerRate", "hpChangeInRound"}
					attributes = append(attributes, "lifestealHp", "peranger", "minusAddperhp", "minusDebuffOdds", "minusDebuffperOdds", "poison", "antiPoison", "fire", "antiFire", "cripple", "blockDam", "hpMinusVal", "magicStoneNum")
					attributes = append(attributes, "angerTohpAF", "angerTohpHF", "perhpreduceToangerAF", "perhpreduceToangerHF", "counterStrikeDam", "counterStrikeSelect", "damShareFactor", "sameDamForRandom")
					record("counterStrikeReal", expected.Attribute["counterStrikeReal"], a.counterReal)
					for _, name := range attributes {
						source, exists := expected.Attribute[name]
						if !exists {
							t.Fatal("original property missing", name)
						}
						native := 0.0
						if data, isData := source.(map[string]any); isData {
							native = data["baseValue"].(float64)*data["rate"].(float64) + data["increment"].(float64)
						} else {
							native = source.(float64)
						}
						actual := a.attribute(name)
						switch name {
						case "hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce", "pdamageReduce", "mdamageReduce", "treatAdd", "bleeding", "block":
							actual *= .01
						case "angerRate":
							actual = 1 + actual*.01
						case "lifestealHp", "peranger", "minusAddperhp", "minusDebuffOdds", "minusDebuffperOdds", "poison", "antiPoison", "fire", "antiFire", "cripple", "blockDam":
							actual *= .01
						case "hpMinusVal":
							actual = a.hpLossAccumulator
						case "magicStoneNum":
							actual = float64(a.magicStones)
						case "angerTohpAF":
							actual = float64(a.angerToHPAP)
						case "angerTohpHF":
							actual = a.angerToHPAmount
						case "perhpreduceToangerAF":
							actual = float64(a.lossToAPAP)
						case "perhpreduceToangerHF":
							actual = a.lossToAPHP
						case "counterStrikeDam", "sameDamForRandom":
							actual *= .01
						case "counterStrikeSelect":
							actual = float64(a.counterSelect)
						case "damShareFactor":
							actual = a.shareFactor
						case "hp":
							actual = a.hp
						case "hpChangeInRound":
							actual = a.hpChangeInRound
						}
						if math.Abs(native-actual) > 1e-6 && drifts[name] == nil {
							drifts[name] = map[string]any{"tick": key, "actor": a.uid, "native": native, "go": actual}
						}
					}
				}
			}
		})
		if computeErr != nil || out.Authoritative || strict && matched != len(wanted) {
			t.Fatal("original actor tick denominator", computeErr, matched, len(wanted))
		}
		if strict && len(drifts) != 0 {
			t.Fatalf("original actor/property drift %s %v", c.Name, drifts)
		}
		reports = append(reports, map[string]any{"case": c.Name, "matchedTicks": matched, "nativeTicks": len(wanted), "missingTicks": len(wanted) - matched, "firstDrifts": drifts, "nativeCompared": false, "authoritative": false})
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
