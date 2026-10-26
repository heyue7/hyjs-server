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

type p6OrderedNativeValue struct {
	Class  int32   `json:"exportValClass"`
	Kind   int32   `json:"damageType"`
	Extra  float64 `json:"extraValue"`
	Number float64 `json:"value"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Order  int64   `json:"-"`
	Time   float64 `json:"-"`
}

// Compare every retained numerical value in the exact order within its original
// FExportOnceData. Native callback collection ordering stays independently open.
func TestP6NativeOriginalCastNumericalOrderGoldens(t *testing.T) {
	for _, fixture := range p6OriginalHitExportFixtures {
		t.Run(fixture[0], func(t *testing.T) { p6OriginalOrderedCastFixture(t, fixture[0], fixture[1]) })
	}
}

func p6OriginalOrderedCastFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := readTestAnalysis75(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("ordered native fixture SHA", err)
	}
	var original struct {
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
					Source string                 `json:"source"`
					Cast   string                 `json:"cast"`
					Skill  string                 `json:"skill"`
					Values []p6OrderedNativeValue `json:"values"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &original); err != nil || len(original.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("ordered original denominator", err)
	}
	for file, digest := range map[string]string{original.Source: original.SourceSHA, original.WireRoot: original.WireSHA} {
		body, readErr := readTestAnalysis75(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("ordered original provenance", file, readErr)
		}
	}
	key := func(round int32, time float64, source, skill, cast string) string {
		return fmt.Sprintf("%09d/%09d/%s/%s/%s", round, int64(math.Round(time*10)), source, skill, cast)
	}
	s := p6OriginalTables(t)
	for _, c := range original.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, readErr := readTestAnalysis75(filepath.Join(root, c.Wire))
			if readErr != nil || p6Digest(wire) != c.WireSHA {
				t.Fatal("ordered native PB SHA", readErr)
			}
			playback := &pb.SCFightPlayback{}
			if err := proto.Unmarshal(wire, playback); err != nil {
				t.Fatal(err)
			}
			frozen, freezeErr := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			native := map[string][]p6OrderedNativeValue{}
			for _, p := range c.Points {
				if p.Phase != "tick" {
					continue
				}
				for _, e := range p.Exports {
					if e.Cast == "" {
						continue
					}
					if p.Clock != "immediate native snapshot" && p.Clock != "explicit native callback" {
						t.Fatal("ordered original clock", p.Clock)
					}
					k := key(p.Round, p.Time, e.Source, e.Skill, e.Cast)
					if _, exists := native[k]; exists {
						t.Fatal("ambiguous duplicate original cast", k)
					}
					native[k] = []p6OrderedNativeValue{}
					for _, v := range e.Values {
						if v.Class < 1 || v.Class > 6 || v.Class <= 4 && v.Source != e.Source || v.Class == 1 && v.Kind > 8 && v.Kind != 12 && v.Kind != 13 && v.Kind != 16 {
							t.Fatal("unsupported original cast numerical value", k, v)
						}
						v.Time = p.Time
						native[k] = append(native[k], v)
					}
				}
			}
			out, computeErr := p6NativeGoldenComputeProjection(t, s, frozen, c.WireSHA, sha, playback.OptRecord, nil)
			if computeErr != nil || out.Authoritative {
				t.Fatal("ordered native calculation", computeErr)
			}
			actual := map[string][]p6OrderedNativeValue{}
			for _, e := range out.Events {
				if e.Cast == "" {
					continue
				}
				v := p6OrderedNativeValue{Class: e.OriginalValueClass, Extra: e.OriginalExtraValue, Number: e.Value, Source: e.Actor, Target: e.Target, Order: e.OriginalValueOrder, Time: e.Time}
				if (e.Kind == "release" || e.Kind == "hp_export") && e.OriginalHPValue != nil {
					if e.OriginalHPType == nil {
						t.Fatal("ordered primary HP type")
					}
					v.Class = 1
					v.Number = *e.OriginalHPValue
					v.Kind = *e.OriginalHPType
				}
				if v.Class < 1 || v.Class > 6 {
					continue
				}
				if e.OriginalValueSource != "" {
					v.Source = e.OriginalValueSource
				}
				if e.OriginalValueTarget != "" {
					v.Target = e.OriginalValueTarget
				}
				if v.Order <= 0 {
					t.Fatal("missing original cast value order", e)
				}
				k := key(e.Round, e.Time, e.Actor, e.Skill, e.Cast)
				actual[k] = append(actual[k], v)
			}
			count := 0
			for k, values := range native {
				got := actual[k]
				delete(actual, k)
				sort.Slice(got, func(i, j int) bool { return got[i].Order < got[j].Order })
				if len(values) != len(got) {
					t.Fatalf("ordered cast %s count native=%d go=%d native=%+v go=%+v", k, len(values), len(got), values, got)
				}
				for i, n := range values {
					g := got[i]
					if g.Order != int64(i+1) {
						t.Fatal("duplicate original ordinal", k, got)
					}
					if math.Abs(n.Time-g.Time) > 1e-8 || n.Class != g.Class || n.Kind != g.Kind || n.Source != g.Source || n.Target != g.Target || math.Abs(n.Number-g.Number) > 1e-6 || math.Abs(n.Extra-g.Extra) > 1e-6 {
						t.Fatalf("ordered cast %s index%d native=%+v go=%+v fullnative=%+v fullgo=%+v", k, i, n, g, values, got)
					}
					count++
				}
			}
			if len(actual) != 0 || count == 0 {
				t.Fatal("ordered cast extra Go rows or empty values", len(actual), count)
			}
			t.Logf("strict within-cast ordered numerical values=%d", count)
		})
	}
}
