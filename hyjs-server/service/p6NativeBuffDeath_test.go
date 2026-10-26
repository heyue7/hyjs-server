package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"path/filepath"
	"testing"
)

type p6OriginalBuffDeadExport struct {
	Buff         model.CombatBuffExportSnapshot
	Source, Dead string
}

func TestP6NativeOriginalRoundEndLastAttackBuffDeadGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalLastAttackBuffDeadFixture(t, f[0], f[1]) })
	}
}
func p6OriginalLastAttackBuffDeadFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := readTestAnalysis75(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("standalone Buff load native SHA", err)
	}
	var d struct {
		Source    string `json:"sourceManifest"`
		SourceSHA string `json:"sourceManifestSHA256"`
		WireRoot  string `json:"wireSourceManifest"`
		WireSHA   string `json:"wireSourceManifestSHA256"`
		Cases     []struct {
			Name    string `json:"case"`
			Wire    string `json:"wire"`
			WireSHA string `json:"wireSHA256"`
			Points  []struct {
				Phase   string  `json:"phase"`
				Round   int32   `json:"round"`
				Time    float64 `json:"time"`
				Clock   string  `json:"clockEvidence"`
				Exports []struct {
					Source  string                           `json:"source"`
					Buff    *model.CombatBuffExportSnapshot  `json:"buff"`
					Loads   []model.CombatBuffExportSnapshot `json:"loads"`
					Values  []json.RawMessage                `json:"values"`
					Unloads []json.RawMessage                `json:"unloads"`
					Dead    []string                         `json:"dead"`
					Cast    string                           `json:"cast"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("standalone Buff load denominator", err)
	}
	for file, digest := range map[string]string{d.Source: d.SourceSHA, d.WireRoot: d.WireSHA} {
		body, readErr := readTestAnalysis75(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("standalone Buff load provenance", file, readErr)
		}
	}
	key := func(round int32, time float64) string {
		return fmt.Sprintf("%09d/%09d", round, int64(math.Round(time*10)))
	}
	s := p6OriginalTables(t)
	for _, c := range d.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, readErr := readTestAnalysis75(filepath.Join(root, c.Wire))
			if readErr != nil || p6Digest(wire) != c.WireSHA {
				t.Fatal("standalone Buff load PB", readErr)
			}
			p := &pb.SCFightPlayback{}
			if err := proto.Unmarshal(wire, p); err != nil {
				t.Fatal(err)
			}
			input, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor, Attacker: p.Attacker, Defender: p.Defender1, Monster: p.Defender2, Seed: p.GetRandSeed(), MaxRound: p.GetMaxRound(), BattleType: p.GetBattleType(), DefenderFirst: p.Attacker.GetInst().GetFirstOrder() == 0 && p.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if err != nil {
				t.Fatal(err)
			}
			native := map[string][]p6OriginalBuffDeadExport{}
			clocks := map[string]float64{}
			preciseBegin := false
			for _, pt := range c.Points {
				if pt.Phase == "round_begin" && pt.Clock == "explicit native callback" {
					preciseBegin = true
				}
				if pt.Phase != "tick" && pt.Phase != "round_end" && !(pt.Phase == "round_begin" && pt.Clock == "explicit native callback") {
					continue
				}
				if pt.Clock != "explicit native callback" && pt.Clock != "immediate native snapshot" {
					t.Fatal("standalone Buff load precise callback clock", pt.Clock)
				}
				k := key(pt.Round, pt.Time)
				for _, e := range pt.Exports {
					if e.Cast != "" || e.Buff == nil || len(e.Loads) > 0 || len(e.Unloads) > 0 || len(e.Dead) == 0 || len(e.Values) > 0 || e.Buff.Factor != "dieAfterLastAttack" {
						continue
					}
					if len(e.Dead) != 1 {
						t.Fatal("new original multiple Buff death needs complete list", e.Dead)
					}
					native[k] = append(native[k], p6OriginalBuffDeadExport{Buff: *e.Buff, Source: e.Source, Dead: e.Dead[0]})
					clocks[k] = pt.Time
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("standalone Buff load calculation", err)
			}
			actual := map[string][]p6OriginalBuffDeadExport{}
			for _, e := range out.Events {
				if e.Kind != "buff_dead_export" || e.OriginalBuffSnapshot == nil || e.OriginalExportCast != "" {
					continue
				}
				if e.Time == 0 && (!preciseBegin || e.Round == 0) {
					continue
				}
				k := key(e.Round, e.Time)
				if want, ok := clocks[k]; ok && math.Abs(e.Time-want) > 1e-8 {
					t.Fatal("standalone Buff load exact callback time", e.Time, want)
				}
				actual[k] = append(actual[k], p6OriginalBuffDeadExport{Buff: *e.OriginalBuffSnapshot, Source: e.Actor, Dead: e.Target})
			}
			count := 0
			for k, values := range native {
				got := actual[k]
				delete(actual, k)
				if len(values) != len(got) {
					t.Fatalf("standalone Buff load exact count clock%s native%d Go%d", k, len(values), len(got))
				}
				for i, n := range values {
					if n.Source != got[i].Source || n.Dead != got[i].Dead || !p6OriginalCastBuffMetadataEqual(n.Buff, got[i].Buff) {
						t.Fatalf("standalone Buff load original metadata/order clock%s index%d native=%+v Go=%+v", k, i, n, got[i])
					}
				}
				count += len(values)
			}
			if len(actual) != 0 {
				t.Fatal("extra calculated original standalone Buff load callbacks", len(actual))
			}
			t.Log("strict original load fullmetadata/listorder", count)
		})
	}
}
