package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"path/filepath"
	"sort"
	"testing"
)

func TestP6NativeOriginalStandaloneBuffNumericalOrderGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalOrderedBuffFixture(t, f[0], f[1]) })
	}
}

// Keep each standalone Buff export's value[] intact, including AP loss/gain
// before compound HP. Independent repeated Buff instances retain separate lists.
func p6OriginalOrderedBuffFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := readTestAnalysis75(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("ordered Buff fixture SHA", err)
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
					Source string `json:"source"`
					Cast   string `json:"cast"`
					Buff   *struct {
						ID    string `json:"id"`
						Owner string `json:"target"`
					} `json:"buff"`
					Values []p6OrderedNativeValue `json:"values"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("ordered Buff original denominator", err)
	}
	for file, digest := range map[string]string{d.Source: d.SourceSHA, d.WireRoot: d.WireSHA} {
		body, readErr := readTestAnalysis75(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("ordered Buff provenance", file, readErr)
		}
	}
	key := func(round int32, time float64, source, buff, owner string) string {
		return fmt.Sprintf("%09d/%09d/%s/%s/%s", round, int64(math.Round(time*10)), source, buff, owner)
	}
	s := p6OriginalTables(t)
	for _, c := range d.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, readErr := readTestAnalysis75(filepath.Join(root, c.Wire))
			if readErr != nil || p6Digest(wire) != c.WireSHA {
				t.Fatal("ordered Buff PB SHA", readErr)
			}
			p := &pb.SCFightPlayback{}
			if err := proto.Unmarshal(wire, p); err != nil {
				t.Fatal(err)
			}
			input, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor, Attacker: p.Attacker, Defender: p.Defender1, Monster: p.Defender2, Seed: p.GetRandSeed(), MaxRound: p.GetMaxRound(), BattleType: p.GetBattleType(), DefenderFirst: p.Attacker.GetInst().GetFirstOrder() == 0 && p.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if err != nil {
				t.Fatal(err)
			}
			preciseBegin := false
			native := map[string][][]p6OrderedNativeValue{}
			for _, pt := range c.Points {
				if pt.Phase == "round_begin" && pt.Clock == "explicit native callback" {
					preciseBegin = true
				}
				if pt.Phase != "tick" && pt.Phase != "round_end" && !(pt.Phase == "round_begin" && pt.Clock == "explicit native callback") {
					continue
				}
				for _, e := range pt.Exports {
					if e.Cast != "" || e.Buff == nil || len(e.Values) == 0 {
						continue
					}
					if pt.Clock != "explicit native callback" && pt.Clock != "immediate native snapshot" {
						t.Fatal("ordered Buff exact original clock", pt.Clock)
					}
					k := key(pt.Round, pt.Time, e.Source, e.Buff.ID, e.Buff.Owner)
					values := []p6OrderedNativeValue{}
					for _, v := range e.Values {
						if v.Class != 1 && v.Class != 2 {
							t.Fatal("unsupported original standalone Buff class", v)
						}
						v.Time = pt.Time
						values = append(values, v)
					}
					native[k] = append(native[k], values)
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("ordered Buff real calculation", err)
			}
			actual := map[string]map[int64][]p6OrderedNativeValue{}
			for _, e := range out.Events {
				if e.Kind != "buff_hp_export" && e.Kind != "buff_value_export" {
					continue
				}
				if e.Time == 0 && (!preciseBegin || e.Round == 0) {
					continue
				}
				if e.OriginalBuffExport <= 0 || e.OriginalBuffOwner == "" {
					t.Fatal("missing original Buff export identity", e)
				}
				v := p6OrderedNativeValue{Class: e.OriginalValueClass, Number: e.Value, Extra: e.OriginalExtraValue, Source: e.Actor, Target: e.Target, Time: e.Time}
				if e.Kind == "buff_hp_export" {
					if e.OriginalHPValue == nil || e.OriginalHPType == nil {
						t.Fatal("missing original Buff HP nominal/type")
					}
					v.Class = 1
					v.Number = *e.OriginalHPValue
					v.Kind = *e.OriginalHPType
				}
				k := key(e.Round, e.Time, e.Actor, e.Buff, e.OriginalBuffOwner)
				if actual[k] == nil {
					actual[k] = map[int64][]p6OrderedNativeValue{}
				}
				actual[k][e.OriginalBuffExport] = append(actual[k][e.OriginalBuffExport], v)
			}
			count := 0
			for k, groups := range native {
				got := actual[k]
				delete(actual, k)
				ids := []int64{}
				for id := range got {
					ids = append(ids, id)
				}
				sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
				if len(ids) != len(groups) {
					t.Fatalf("ordered Buff group count %s original%d Go%d native=%+v go=%+v", k, len(groups), len(ids), groups, got)
				}
				for group, values := range groups {
					gvalues := got[ids[group]]
					if len(values) != len(gvalues) {
						t.Fatal("ordered Buff list length", k, group, values, gvalues)
					}
					for index, n := range values {
						g := gvalues[index]
						if n.Class != g.Class || n.Kind != g.Kind || n.Source != g.Source || n.Target != g.Target || math.Abs(n.Time-g.Time) > 1e-8 || math.Abs(n.Number-g.Number) > 1e-6 || math.Abs(n.Extra-g.Extra) > 1e-6 {
							t.Fatalf("ordered Buff %s group%d index%d native=%+v Go=%+v", k, group, index, values, gvalues)
						}
						count++
					}
				}
			}
			if len(actual) != 0 {
				t.Fatal("extra Go standalone Buff exports", len(actual), actual)
			}
			t.Log("strict original standalone values", count)
		})
	}
}
