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

type p6StandaloneLoadGroup struct {
	Group          int64
	Snapshot       model.CombatBuffExportSnapshot
	Source, Talent string
	Parent         *model.CombatBuffExportSnapshot
}

func TestP6NativeOriginalStandaloneBuffLoadGroupGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalStandaloneGroupFixture(t, f[0], f[1]) })
	}
}
func p6OriginalStandaloneGroupFixture(t *testing.T, name, sha string) {
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
					Source string                           `json:"source"`
					Talent string                           `json:"talent"`
					Buff   *model.CombatBuffExportSnapshot  `json:"buff"`
					Loads  []model.CombatBuffExportSnapshot `json:"loads"`
					Cast   string                           `json:"cast"`
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
			native := map[string][]p6StandaloneLoadGroup{}
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
				for index, e := range pt.Exports {
					if e.Cast != "" {
						continue
					}
					for _, b := range e.Loads {

						native[k] = append(native[k], p6StandaloneLoadGroup{Group: int64(index + 1), Snapshot: b, Source: e.Source, Talent: e.Talent, Parent: e.Buff})
						clocks[k] = pt.Time
					}
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("standalone Buff load calculation", err)
			}
			actual := map[string][]p6StandaloneLoadGroup{}
			for _, e := range out.Events {
				if e.Kind != "buff_load" || e.OriginalBuffSnapshot == nil || e.OriginalExportCast != "" {
					continue
				}
				if e.Time == 0 && (!preciseBegin || e.Round == 0) {
					continue
				}
				k := key(e.Round, e.Time)
				if want, ok := clocks[k]; ok && math.Abs(e.Time-want) > 1e-8 {
					t.Fatal("standalone Buff load exact callback time", e.Time, want)
				}
				header := p6StandaloneLoadGroup{Group: e.OriginalStandaloneLoadGroup, Snapshot: *e.OriginalBuffSnapshot, Source: e.OriginalBuffSnapshot.Source, Talent: e.OriginalBuffSnapshot.Talent, Parent: e.OriginalBuffParentSnapshot}
				if header.Parent != nil {
					header.Source = header.Parent.Source
					header.Talent = ""
				}
				actual[k] = append(actual[k], header)
			}
			count := 0
			for k, values := range native {
				got := actual[k]
				delete(actual, k)
				if len(values) != len(got) {
					t.Fatalf("standalone Buff load exact count clock%s native%d Go%d", k, len(values), len(got))
				}
				for i, n := range values {
					if got[i].Group == 0 || i > 0 && ((n.Group == values[i-1].Group) != (got[i].Group == got[i-1].Group)) {
						t.Fatalf("original standalone load group boundary clock%s index%d native%v got%v", k, i, values, got)
					}
					if n.Source != got[i].Source || n.Talent != got[i].Talent || (n.Parent == nil) != (got[i].Parent == nil) || !p6OriginalCastBuffMetadataEqual(n.Snapshot, got[i].Snapshot) || n.Parent != nil && !p6OriginalCastBuffMetadataEqual(*n.Parent, *got[i].Parent) {
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
