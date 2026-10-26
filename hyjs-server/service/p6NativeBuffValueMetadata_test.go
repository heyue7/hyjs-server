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

func TestP6NativeOriginalStandaloneBuffValueMetadataGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalStandaloneBuffValueMetadataFixture(t, f[0], f[1]) })
	}
}
func p6OriginalStandaloneBuffValueMetadataFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := readTestAnalysis75(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("standalone Buff value native SHA", err)
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
					Buff   *model.CombatBuffExportSnapshot  `json:"buff"`
					Loads  []model.CombatBuffExportSnapshot `json:"loads"`
					Values []json.RawMessage                `json:"values"`
					Cast   string                           `json:"cast"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("standalone Buff value denominator", err)
	}
	for file, digest := range map[string]string{d.Source: d.SourceSHA, d.WireRoot: d.WireSHA} {
		body, readErr := readTestAnalysis75(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("standalone Buff value provenance", file, readErr)
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
				t.Fatal("standalone Buff value PB", readErr)
			}
			p := &pb.SCFightPlayback{}
			if err := proto.Unmarshal(wire, p); err != nil {
				t.Fatal(err)
			}
			input, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor, Attacker: p.Attacker, Defender: p.Defender1, Monster: p.Defender2, Seed: p.GetRandSeed(), MaxRound: p.GetMaxRound(), BattleType: p.GetBattleType(), DefenderFirst: p.Attacker.GetInst().GetFirstOrder() == 0 && p.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if err != nil {
				t.Fatal(err)
			}
			native := map[string][]model.CombatBuffExportSnapshot{}
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
					t.Fatal("standalone Buff value precise callback clock", pt.Clock)
				}
				k := key(pt.Round, pt.Time)
				for _, e := range pt.Exports {
					if e.Cast != "" || e.Buff == nil || len(e.Values) == 0 {
						continue
					}
					for range e.Values {
						native[k] = append(native[k], *e.Buff)
					}
					clocks[k] = pt.Time
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("standalone Buff value calculation", err)
			}
			actual := map[string][]model.CombatBuffExportSnapshot{}
			for _, e := range out.Events {
				if e.Kind != "buff_hp_export" && e.Kind != "buff_value_export" || e.OriginalBuffSnapshot == nil || e.OriginalExportCast != "" {
					continue
				}
				if e.Time == 0 && (!preciseBegin || e.Round == 0) {
					continue
				}
				k := key(e.Round, e.Time)
				if want, ok := clocks[k]; ok && math.Abs(e.Time-want) > 1e-8 {
					t.Fatal("standalone Buff value exact callback time", e.Time, want)
				}
				actual[k] = append(actual[k], *e.OriginalBuffSnapshot)
			}
			count := 0
			for k, values := range native {
				got := actual[k]
				delete(actual, k)
				if len(values) != len(got) {
					t.Fatalf("standalone Buff value exact count clock%s native%d Go%d", k, len(values), len(got))
				}
				for i, n := range values {
					if !p6OriginalCastBuffMetadataEqual(n, got[i]) {
						t.Fatalf("standalone Buff value original metadata/order clock%s index%d native=%+v Go=%+v", k, i, n, got[i])
					}
				}
				count += len(values)
			}
			if len(actual) != 0 {
				t.Fatal("extra calculated original standalone Buff value callbacks", len(actual))
			}
			t.Log("strict original value owner fullmetadata/listorder", count)
		})
	}
}
