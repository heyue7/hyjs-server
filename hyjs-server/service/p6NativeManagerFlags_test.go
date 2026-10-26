package service

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Native BuffClass flags survive effect unload. Completed-Tick suppression
// lists are checked separately from active and once-cast/hit identities.
func TestP6NativeOriginalBuffManagerIdentitiesGolden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v57.json", "2c42de90b3a07d5b7b9610609b85a09aa10d7cb3470f1377dc4611c3881c64e3", 4)
}
func p6NativeOriginalManagerFixture(t *testing.T, file, expectedSHA string, groups int, expectedCases ...int) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := readTestAnalysis75(filepath.Join(root, "analysis", "p6", file))
	if err != nil || p6Digest(raw) != expectedSHA {
		t.Fatal("original manager identities golden digest", err)
	}
	type actorState struct {
		UID        string   `json:"uid"`
		Flags      []string `json:"flags"`
		Suppressed []string `json:"suppressed"`
	}
	type tickState struct {
		Round   int32          `json:"round"`
		Time    float64        `json:"time"`
		Players [][]actorState `json:"players"`
	}
	var fixture struct {
		Groups []struct {
			Manifest    string `json:"sourceManifest"`
			ManifestSHA string `json:"sourceManifestSHA256"`
			WireSource  string `json:"wireSourceManifest"`
			WireSHA     string `json:"wireSourceManifestSHA256"`
			Cases       []struct {
				Name    string      `json:"case"`
				Wire    string      `json:"wire"`
				WireSHA string      `json:"wireSHA256"`
				Ticks   []tickState `json:"ticks"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Groups) != groups {
		t.Fatal("original manager group denominator", err)
	}
	s := p6OriginalTables(t)
	for _, group := range fixture.Groups {
		for name, sha := range map[string]string{group.Manifest: group.ManifestSHA, group.WireSource: group.WireSHA} {
			body, readErr := readTestAnalysis75(filepath.Join(root, name))
			if readErr != nil || p6Digest(body) != sha {
				t.Fatal("original manager source digest", name, readErr)
			}
		}
		if len(group.Cases) != p6NativeDiagnosticCaseCount(t, expectedCases...) {
			t.Fatal("original manager case denominator")
		}
		for _, c := range group.Cases {
			t.Run(c.Name, func(t *testing.T) {
				wire, readErr := readTestAnalysis75(filepath.Join(root, c.Wire))
				if readErr != nil || p6Digest(wire) != c.WireSHA {
					t.Fatal("original manager wire digest", readErr)
				}
				playback := &pb.SCFightPlayback{}
				if err = proto.Unmarshal(wire, playback); err != nil {
					t.Fatal(err)
				}
				frozen, freezeErr := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
				if freezeErr != nil {
					t.Fatal(freezeErr)
				}
				wanted := map[string]tickState{}
				for _, tick := range c.Ticks {
					key := fmt.Sprintf("%d/%d", tick.Round, int64(math.Round(tick.Time*10)))
					if _, exists := wanted[key]; exists {
						t.Fatal("duplicate original manager completed tick", key)
					}
					wanted[key] = tick
				}
				matched := 0
				_, computeErr := p6NativeGoldenComputeProjection(t, s, frozen, c.WireSHA, expectedSHA, playback.OptRecord, func(e *combatRoundEngine) {
					key := fmt.Sprintf("%d/%d", e.round, int64(math.Round(e.time*10)))
					tick, found := wanted[key]
					if !found {
						return
					}
					matched++
					if len(tick.Players) != len(e.players) {
						t.Fatal("original manager player denominator")
					}
					for camp, player := range tick.Players {
						if len(player) != len(e.players[camp].actors) {
							t.Fatal("original manager actor denominator")
						}
						for i, expected := range player {
							a := e.players[camp].actors[i]
							if a.uid != expected.UID {
								t.Fatal("original manager actor identity")
							}
							flags, suppressed := []string{}, []string{}
							for id, present := range a.flags {
								if present {
									flags = append(flags, id)
								}
							}
							for _, b := range a.buffs {
								if b.suppressed {
									suppressed = append(suppressed, b.row["Id"])
								}
							}
							sort.Strings(flags)
							sort.Strings(expected.Flags)
							if !reflect.DeepEqual(flags, expected.Flags) || !reflect.DeepEqual(suppressed, expected.Suppressed) {
								t.Fatalf("original manager identities %s %s: flags native=%v go=%v suppressed native=%v go=%v", key, a.uid, expected.Flags, flags, expected.Suppressed, suppressed)
							}
						}
					}
				})
				if computeErr != nil || matched != len(c.Ticks) {
					t.Fatalf("original manager completed ticks %d/%d: %v", matched, len(c.Ticks), computeErr)
				}
			})
		}
	}
}
