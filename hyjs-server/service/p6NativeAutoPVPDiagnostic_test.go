package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Diagnostics retain expected Go outputs against the original captured automatic
// AI inputs. A successful computation never asserts native parity by itself.
func TestP6NativeAutomaticPVPPriorityExpectedTimelines(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_PVP_AUTO_TEST") != "1" {
		t.Skip("explicit original automatic PVP diagnostic disabled")
	}
	output := os.Getenv("HYJS_P6_NATIVE_PVP_AUTO_OUTPUT")
	if output == "" {
		t.Fatal("separate diagnostic output directory required")
	}
	p6AutomaticPVPExpectedTimelines(t, "../../analysis/p6/native-pvp-auto-priority-v45-20261014/manifest.json", output, 14, false)
}

func TestP6OriginalDamageFactorExpectedTimelines(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_FACTOR_AUTO_TEST") != "1" {
		t.Skip("explicit factor calibration preparation disabled")
	}
	output := os.Getenv("HYJS_P6_NATIVE_FACTOR_AUTO_OUTPUT")
	if output == "" {
		t.Fatal("separate factor output required")
	}
	p6AutomaticPVPExpectedTimelines(t, "../../analysis/p6/native-damage-factor-input-v49/manifest.json", output, 4, true)
}

func TestP6OriginalNextRealCoverageExpectedTimelines(t *testing.T) {
	if os.Getenv("HYJS_P6_NATIVE_NEXT_AUTO_TEST") != "1" {
		t.Skip("explicit next original real-input calibration preparation disabled")
	}
	output := os.Getenv("HYJS_P6_NATIVE_NEXT_AUTO_OUTPUT")
	if output == "" {
		t.Fatal("separate next real-input output required")
	}
	manifest := os.Getenv("HYJS_P6_NATIVE_NEXT_INPUT_MANIFEST")
	if manifest == "" {
		manifest = "../../analysis/p6/native-next-real-inputs-v52/manifest.json"
	}
	p6AutomaticPVPExpectedTimelines(t, manifest, output, 14, false)
}

func p6AutomaticPVPExpectedTimelines(t *testing.T, manifestPath, output string, count int, variedFactors bool) {
	t.Helper()
	raw, err := readTestAnalysis75(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Source string `json:"sourceManifest"`
		Cases  []struct {
			Name      string `json:"case"`
			Wire      string `json:"wire"`
			WireSHA   string `json:"wireSHA256"`
			BatchWire string `json:"batchWire"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &manifest); err != nil || len(manifest.Cases) != count {
		t.Fatal("immutable original priority cases changed", err)
	}
	s := p6OriginalTables(t)
	if err = os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	var proofs []map[string]any
	for _, c := range manifest.Cases {
		// A local real-input manifest stores its exact protobuf next to itself;
		// older collected manifests retain their independent source directory.
		wireRoot := filepath.Dir(manifest.Source)
		if manifest.Source == "" {
			wireRoot = filepath.Dir(manifestPath)
		}
		wire, err := readTestAnalysis75(filepath.Join(wireRoot, c.Wire))
		if err != nil || p6Digest(wire) != c.WireSHA {
			t.Fatal("original raw priority wire hash", c.Name, err)
		}
		original := &pb.SCFightPlayback{}
		if err = proto.Unmarshal(wire, original); err != nil {
			t.Fatal(err)
		}
		if original.GetBattleType() != 2 || original.Defender1 == nil || (!variedFactors && (original.GetAttackerFactor() != 1 || original.GetDefenderFactor() != 1)) {
			t.Fatal("original supported PVP input changed", c.Name)
		}
		frozen, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: original.AttackerFactor, DefenderDamageFactor: original.DefenderFactor, Attacker: original.Attacker, Defender: original.Defender1, Monster: original.Defender2, Seed: original.GetRandSeed(), MaxRound: original.GetMaxRound(), BattleType: original.GetBattleType(), DefenderFirst: original.Attacker.GetInst().GetFirstOrder() == 0 && original.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
		if err != nil {
			t.Fatal(err)
		}
		stem := strings.TrimSuffix(c.BatchWire, ".pb")
		if c.BatchWire == "" {
			stem = strings.TrimSuffix(c.Wire, ".pb")
		}
		tickPath := filepath.Join(output, stem+".ticks.jsonl")
		ticks, err := os.Create(tickPath)
		if err != nil {
			t.Fatal(err)
		}
		encoder := json.NewEncoder(ticks)
		var tickErr error
		out, err := s.computeFrozenBattleObserved(t.Context(), frozen, nil, func(e *combatRoundEngine) {
			var players []map[string]any
			for _, p := range e.players {
				var actors []map[string]any
				for _, a := range p.actors {
					var skills []map[string]any
					var waiting []string
					current := ""
					if a.current != nil {
						current = a.current.id
					}
					for _, skill := range a.skills {
						skills = append(skills, map[string]any{"id": skill.id, "state": skill.state, "cooldown": skill.cooldown})
					}
					for _, skill := range a.waiting {
						waiting = append(waiting, skill.id)
					}
					actors = append(actors, map[string]any{"uid": a.uid, "hp": a.hp, "state": a.state, "skills": skills, "waiting": waiting, "current": current})
				}
				players = append(players, map[string]any{"anger": p.anger, "actors": actors})
			}
			if tickErr == nil {
				tickErr = encoder.Encode(map[string]any{"round": e.round, "time": e.time, "randomDraws": e.rng.Draws(), "players": players, "casts": e.diagnosticCasts})
			}
		})
		closeErr := ticks.Close()
		if tickErr != nil || closeErr != nil {
			t.Fatal("exact completed tick diagnostic write", tickErr, closeErr)
		}
		if err != nil || out.Authoritative {
			t.Fatal("original PVP diagnostic computation", c.Name, err)
		}
		bytes, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		name := stem + ".go.json"
		if err = os.WriteFile(filepath.Join(output, name), append(bytes, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
		tickBytes, readErr := readTestAnalysis75(tickPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		proofs = append(proofs, map[string]any{"case": c.Name, "sourceWireSHA256": c.WireSHA, "goTimeline": name, "goTimelineSHA256": p6Digest(append(bytes, '\n')), "goTicks": filepath.Base(tickPath), "goTicksSHA256": p6Digest(tickBytes), "engineVersion": constUtil.P6EngineVersion, "nativeCompared": false, "authoritative": false})
	}
	bytes, err := json.MarshalIndent(map[string]any{"engineVersion": constUtil.P6EngineVersion, "cases": proofs, "nativeCompared": false, "authoritative": false}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "manifest.json"), append(bytes, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
