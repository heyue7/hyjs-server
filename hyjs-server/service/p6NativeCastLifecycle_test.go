package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type p6OriginalCastBuffIdentity = model.CombatBuffExportSnapshot

func TestP6NativeOriginalCastPersistentLoadsAndDeadOrderGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalCastLifecycleFixture(t, f[0], f[1]) })
	}
}

// Original per-cast list order is independent from event audit chronology.
// UnloadBuff owns a separate Buff export; these original cast unload lists are empty.
func p6OriginalCastLifecycleFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("cast lifecycle native SHA", err)
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
					Source  string                       `json:"source"`
					Skill   string                       `json:"skill"`
					Cast    string                       `json:"cast"`
					Loads   []p6OriginalCastBuffIdentity `json:"loads"`
					Unloads []p6OriginalCastBuffIdentity `json:"unloads"`
					Dead    []string                     `json:"dead"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("cast lifecycle original denominator", err)
	}
	for file, digest := range map[string]string{d.Source: d.SourceSHA, d.WireRoot: d.WireSHA} {
		b, readErr := os.ReadFile(filepath.Join(root, file))
		if readErr != nil || p6Digest(b) != digest {
			t.Fatal("cast lifecycle provenance", file, readErr)
		}
	}
	key := func(round int32, time float64, source, skill, cast string) string {
		return fmt.Sprintf("%09d/%09d/%s/%s/%s", round, int64(math.Round(time*10)), source, skill, cast)
	}
	type lists struct {
		Loads []p6OriginalCastBuffIdentity
		Dead  []string
	}
	s := p6OriginalTables(t)
	for _, c := range d.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, readErr := os.ReadFile(filepath.Join(root, c.Wire))
			if readErr != nil || p6Digest(wire) != c.WireSHA {
				t.Fatal("cast lifecycle original PB", readErr)
			}
			p := &pb.SCFightPlayback{}
			if err := proto.Unmarshal(wire, p); err != nil {
				t.Fatal(err)
			}
			input, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor, Attacker: p.Attacker, Defender: p.Defender1, Monster: p.Defender2, Seed: p.GetRandSeed(), MaxRound: p.GetMaxRound(), BattleType: p.GetBattleType(), DefenderFirst: p.Attacker.GetInst().GetFirstOrder() == 0 && p.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if err != nil {
				t.Fatal(err)
			}
			native := map[string]lists{}
			for _, pt := range c.Points {
				if pt.Phase != "tick" {
					continue
				}
				for _, e := range pt.Exports {
					if e.Cast == "" {
						continue
					}
					if pt.Clock != "explicit native callback" && pt.Clock != "immediate native snapshot" {
						t.Fatal("cast lifecycle exact clock", pt.Clock)
					}
					if len(e.Unloads) > 0 {
						t.Fatal("new original per-cast unload branch needs explicit collector", e)
					}
					k := key(pt.Round, pt.Time, e.Source, e.Skill, e.Cast)
					if _, ok := native[k]; ok {
						t.Fatal("duplicate original cast", k)
					}
					native[k] = lists{Loads: e.Loads, Dead: e.Dead}
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("cast lifecycle real calculation", err)
			}
			actual := map[string]lists{}
			for _, e := range out.Events {
				if e.Kind == "buff_load" && e.OriginalExportCast != "" {
					k := key(e.Round, e.Time, e.OriginalExportSource, e.OriginalExportSkill, e.OriginalExportCast)
					a := actual[k]
					if e.OriginalBuffSnapshot == nil {
						t.Fatal("missing completed Tick Buff snapshot", e)
					}
					a.Loads = append(a.Loads, *e.OriginalBuffSnapshot)
					actual[k] = a
				}
				if e.Kind == "cast_dead_export" {
					k := key(e.Round, e.Time, e.Actor, e.Skill, e.Cast)
					a := actual[k]
					a.Dead = append(a.Dead, e.Target)
					actual[k] = a
				}
			}
			loads, deaths := 0, 0
			for k, n := range native {
				a := actual[k]
				delete(actual, k)
				if len(n.Loads) != len(a.Loads) || len(n.Dead) != len(a.Dead) {
					t.Fatalf("cast lists %s native=%+v go=%+v", k, n, a)
				}
				for i, b := range n.Loads {
					if !p6OriginalCastBuffMetadataEqual(b, a.Loads[i]) {
						t.Fatalf("cast load original sequence %s index%d native=%+v go=%+v", k, i, n.Loads, a.Loads)
					}
				}
				for i, uid := range n.Dead {
					if uid != a.Dead[i] {
						t.Fatal("cast final dead sequence", k, n.Dead, a.Dead)
					}
				}
				loads += len(n.Loads)
				deaths += len(n.Dead)
			}
			if len(actual) != 0 {
				t.Fatal("extra Go cast lifecycle", actual)
			}
			t.Log("original cast persistent loads", loads, "deaths", deaths)
		})
	}
}

func p6OriginalCastBuffMetadataEqual(a, b model.CombatBuffExportSnapshot) bool {
	var n, g map[string]any
	ar, _ := json.Marshal(a)
	br, _ := json.Marshal(b)
	json.Unmarshal(ar, &n)
	json.Unmarshal(br, &g)
	for k, v := range n {
		if number, ok := v.(float64); ok {
			limit := 1e-6
			if k == "loadBuffTime" {
				limit = 1e-8
			}
			other, ok := g[k].(float64)
			if !ok || math.Abs(number-other) > limit {
				return false
			}
		} else if !reflect.DeepEqual(v, g[k]) {
			return false
		}
	}
	return len(n) == len(g)
}
