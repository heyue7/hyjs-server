package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Detail diagnostics preserve remaining original CD/Buff evidence separately
// from the completed core numerical fixture and never enable reward authority.
func TestP6NativePriorityCDAndBuffDetailsDiagnostic(t *testing.T) {
	output := os.Getenv("HYJS_P6_NATIVE_DETAIL_OUTPUT")
	if output == "" {
		t.Skip("explicit separate CD/Buff diagnostic output required")
	}
	p6NativePriorityDetails(t, output, false)
}

func TestP6NativePriorityOriginalCDAndBuffDetailsGolden(t *testing.T) {
	p6NativePriorityDetails(t, "", true)
}

func p6NativePriorityDetails(t *testing.T, output string, strict bool) {
	p6NativeDetailsFromFixture(t, output, strict, "native-pvp-priority-detail-golden-v48.json", "4a677c656c968a9c5e94f9fbcaa77e06b7e3ae672e8ecc5d64323c59bf0bb45f")
}

func p6NativeDetailsFromFixture(t *testing.T, output string, strict bool, file, expectedSHA string, expectedCases ...int) {
	t.Helper()
	raw, wireRoot := p6NativeDiagnosticFixture(t, file, expectedSHA, "HYJS_P6_NATIVE_NEXT_DETAIL_FIXTURE", strict)
	var err error
	type skillState struct {
		ID       string `json:"id"`
		State    int32  `json:"state"`
		Cooldown int32  `json:"cooldown"`
	}
	type buffState struct {
		ID     string  `json:"id"`
		Factor string  `json:"factor"`
		Timer  int32   `json:"timer"`
		Rounds int32   `json:"totalRound"`
		Attack int32   `json:"attCount"`
		Defend int32   `json:"defCount"`
		Loaded float64 `json:"loadBuffTime"`
	}
	type actorState struct {
		UID    string                 `json:"uid"`
		Skills []skillState           `json:"skills"`
		Buffs  map[string][]buffState `json:"buffs"`
		Shield map[string]float64     `json:"shield"`
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
			Name    string      `json:"case"`
			Wire    string      `json:"wire"`
			WireSHA string      `json:"wireSHA256"`
			Ticks   []tickState `json:"ticks"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &original); err != nil || len(original.Cases) != p6NativeDiagnosticCaseCount(t, expectedCases...) {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	var reports []map[string]any
	for _, c := range original.Cases {
		wire, err := os.ReadFile(filepath.Join(wireRoot, c.Wire))
		if err != nil || p6Digest(wire) != c.WireSHA {
			t.Fatal("original detail wire", err)
		}
		playback := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(wire, playback); err != nil {
			t.Fatal(err)
		}
		frozen, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
		if err != nil {
			t.Fatal(err)
		}
		wanted := map[string]tickState{}
		for _, tick := range c.Ticks {
			wanted[fmt.Sprintf("%d/%d", tick.Round, int64(math.Round(tick.Time*10)))] = tick
		}
		report := map[string]any{"case": c.Name, "sourceWireSHA256": c.WireSHA, "detailSourceSHA256": p6Digest(raw), "skillStateDrift": nil, "cooldownDrift": nil, "buffIdentityAndLifetimeDrift": nil, "shieldDrift": nil, "nativeCompared": false, "authoritative": false}
		matched := 0
		out, err := p6NativeGoldenComputeProjection(t, s, frozen, c.WireSHA, expectedSHA, playback.OptRecord, func(e *combatRoundEngine) {
			key := fmt.Sprintf("%d/%d", e.round, int64(math.Round(e.time*10)))
			tick, found := wanted[key]
			if !found {
				return
			}
			matched++
			for camp, player := range tick.Players {
				for i, expected := range player.Actors {
					a := e.players[camp].actors[i]
					if strict && (a.uid != expected.UID || len(a.skills) != len(expected.Skills)) {
						t.Fatalf("original actor/skill denominator %s %s", c.Name, expected.UID)
					}
					for _, wantedSkill := range expected.Skills {
						foundSkill := false
						for _, actualSkill := range a.skills {
							if actualSkill.id != wantedSkill.ID {
								continue
							}
							foundSkill = true
							for name, values := range map[string][2]int32{"skillStateDrift": {wantedSkill.State, actualSkill.state}, "cooldownDrift": {wantedSkill.Cooldown, actualSkill.cooldown}} {
								if values[0] != values[1] && report[name] == nil {
									report[name] = map[string]any{"tick": key, "actor": a.uid, "skill": actualSkill.id, "native": values[0], "go": values[1]}
								}
							}
						}
						if strict && !foundSkill {
							t.Fatalf("original skill missing %s %s", c.Name, wantedSkill.ID)
						}
					}
					buffs := map[string][]buffState{"buffs": {}, "onceCastbuffs": {}, "onceHitbuffs": {}}
					for _, b := range a.buffs {
						state := buffState{ID: b.row["Id"], Factor: b.row["Factor"], Timer: b.timer, Rounds: b.rounds, Attack: b.attacks, Defend: b.defenses, Loaded: b.loaded}
						// Native buffs contains every active identity; the once lists
						// additionally reference the same identities for cast/hit cleanup.
						buffs["buffs"] = append(buffs["buffs"], state)
						if b.once {
							bucket := "onceCastbuffs"
							if b.onceHit {
								bucket = "onceHitbuffs"
							}
							buffs[bucket] = append(buffs[bucket], state)
						}
					}
					if report["buffIdentityAndLifetimeDrift"] == nil && !reflect.DeepEqual(buffs, expected.Buffs) {
						report["buffIdentityAndLifetimeDrift"] = map[string]any{"tick": key, "actor": a.uid, "native": expected.Buffs, "go": buffs}
					}
					for kind, value := range expected.Shield {
						actual := a.shield(kind)
						if math.Abs(value-actual) > 1e-6 && report["shieldDrift"] == nil {
							report["shieldDrift"] = map[string]any{"tick": key, "actor": a.uid, "kind": kind, "native": value, "go": actual}
						}
					}
				}
			}
		})
		if err != nil || out.Authoritative || strict && matched != len(wanted) {
			t.Fatal("detail computation and clock denominator", c.Name, matched, len(wanted), err)
		}
		if strict {
			for _, name := range []string{"skillStateDrift", "cooldownDrift", "buffIdentityAndLifetimeDrift", "shieldDrift"} {
				if report[name] != nil {
					t.Fatalf("original detail %s %s: %v", c.Name, name, report[name])
				}
			}
		}
		report["matchedSampledTicks"] = matched
		report["nativeSampledTicks"] = len(wanted)
		report["missingSampledTicks"] = len(wanted) - matched
		reports = append(reports, report)
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
