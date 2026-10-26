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

type p6NativeStandaloneMixedHeader struct {
	Snapshot *model.CombatBuffExportSnapshot
	Source   string
	Talent   string
	Values   int
	Loads    int
	Unloads  int
	Dead     int
}

func TestP6NativeOriginalStandaloneMixedCollectionGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, f[0], f[1]) })
	}
}
func p6OriginalStandaloneMixedCollectionFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := readTestAnalysis75(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("standalone mixed exports native SHA", err)
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
					Talent  string                           `json:"talent"`
					Buff    *model.CombatBuffExportSnapshot  `json:"buff"`
					Loads   []model.CombatBuffExportSnapshot `json:"loads"`
					Unloads []model.CombatBuffExportSnapshot `json:"unloads"`
					Dead    []json.RawMessage                `json:"dead"`
					Values  []json.RawMessage                `json:"values"`
					Cast    string                           `json:"cast"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("standalone mixed exports denominator", err)
	}
	for file, digest := range map[string]string{d.Source: d.SourceSHA, d.WireRoot: d.WireSHA} {
		body, readErr := readTestAnalysis75(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("standalone mixed exports provenance", file, readErr)
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
				t.Fatal("standalone mixed exports PB", readErr)
			}
			p := &pb.SCFightPlayback{}
			if err := proto.Unmarshal(wire, p); err != nil {
				t.Fatal(err)
			}
			input, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: p.AttackerFactor, DefenderDamageFactor: p.DefenderFactor, Attacker: p.Attacker, Defender: p.Defender1, Monster: p.Defender2, Seed: p.GetRandSeed(), MaxRound: p.GetMaxRound(), BattleType: p.GetBattleType(), DefenderFirst: p.Attacker.GetInst().GetFirstOrder() == 0 && p.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if err != nil {
				t.Fatal(err)
			}
			native := map[string][]p6NativeStandaloneMixedHeader{}
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
					t.Fatal("standalone mixed exports precise callback clock", pt.Clock)
				}
				k := key(pt.Round, pt.Time)
				for _, e := range pt.Exports {
					if e.Cast != "" {
						continue
					}
					native[k] = append(native[k], p6NativeStandaloneMixedHeader{Snapshot: e.Buff, Source: e.Source, Talent: e.Talent, Values: len(e.Values), Loads: len(e.Loads), Unloads: len(e.Unloads), Dead: len(e.Dead)})
					clocks[k] = pt.Time
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("standalone mixed exports calculation", err)
			}
			// Reconstruct actual export groups from stable identities and complete each
			// group at its last calculation event, keeping inner side-effects before it.
			type entry struct {
				header   p6NativeStandaloneMixedHeader
				sequence int64
			}
			grouped := map[string]map[string]*entry{}
			for _, e := range out.Events {
				if e.OriginalExportCast != "" || e.OriginalBuffSnapshot == nil && e.Kind != "standalone_load_export" {
					continue
				}
				if e.Time == 0 && (!preciseBegin || e.Round == 0) {
					continue
				}
				k := key(e.Round, e.Time)
				if want, ok := clocks[k]; ok && math.Abs(e.Time-want) > 1e-8 {
					t.Fatal("standalone mixed exact callback time", e.Time, want)
				}
				identity := ""
				h := p6NativeStandaloneMixedHeader{Snapshot: e.OriginalBuffSnapshot}
				if e.OriginalBuffSnapshot != nil {
					h.Source = e.OriginalBuffSnapshot.Source
				}
				switch e.Kind {
				case "standalone_load_export":
					identity = fmt.Sprintf("load/%d", e.OriginalStandaloneLoadGroup)
					if grouped[k] == nil || grouped[k][identity] == nil {
						t.Fatal("standalone group finalized without actual persistent loads", e)
					}
					grouped[k][identity].sequence = e.Sequence
					continue
				case "buff_load":
					identity = fmt.Sprintf("load/%d", e.OriginalStandaloneLoadGroup)
					if e.OriginalStandaloneLoadGroup <= 0 {
						t.Fatal("missing true standalone load group", e)
					}
					h.Snapshot = e.OriginalBuffParentSnapshot
					h.Talent = e.OriginalBuffSnapshot.Talent
					if h.Snapshot != nil {
						h.Source = h.Snapshot.Source
						h.Talent = ""
					}
					h.Loads = 1
				case "buff_unload":
					identity = fmt.Sprintf("unload/%d", e.Sequence)
					h.Unloads = 1
				case "buff_hp_export", "buff_value_export":
					identity = fmt.Sprintf("buff/%d/%s", e.OriginalBuffExport, e.OriginalBuffOwner)
					h.Values = 1
				case "buff_round_dead_export", "buff_dead_export", "buff_value_dead_export":
					identity = fmt.Sprintf("buff/%d/%s", e.OriginalBuffExport, e.OriginalBuffOwner)
					h.Dead = 1
				case "buff_empty_export":
					identity = fmt.Sprintf("empty/%d", e.Sequence)
				default:
					continue
				}
				if grouped[k] == nil {
					grouped[k] = map[string]*entry{}
				}
				old := grouped[k][identity]
				if old == nil {
					old = &entry{header: h, sequence: e.Sequence}
					grouped[k][identity] = old
				} else {
					old.header.Values += h.Values
					old.header.Loads += h.Loads
					old.header.Unloads += h.Unloads
					old.header.Dead += h.Dead
					old.sequence = e.Sequence
				}
			}
			actual := map[string][]p6NativeStandaloneMixedHeader{}
			for k, m := range grouped {
				items := []*entry{}
				for _, x := range m {
					items = append(items, x)
				}
				sort.Slice(items, func(i, j int) bool { return items[i].sequence < items[j].sequence })
				for _, x := range items {
					actual[k] = append(actual[k], x.header)
				}
			}

			count := 0
			for k, values := range native {
				got := actual[k]
				delete(actual, k)
				if len(values) != len(got) {
					t.Fatalf("standalone mixed exports exact count clock%s native%d Go%d", k, len(values), len(got))
				}
				for i, n := range values {
					if n.Source != got[i].Source || n.Talent != got[i].Talent || n.Values != got[i].Values || n.Loads != got[i].Loads || n.Unloads != got[i].Unloads || n.Dead != got[i].Dead || (n.Snapshot == nil) != (got[i].Snapshot == nil) || n.Snapshot != nil && !p6OriginalCastBuffMetadataEqual(*n.Snapshot, *got[i].Snapshot) {
						t.Fatalf("standalone mixed exports original metadata/order clock%s index%d native=%+v Go=%+v", k, i, n, got[i])
					}
				}
				count += len(values)
			}
			if len(actual) != 0 {
				t.Fatal("extra calculated original standalone mixed exports callbacks", len(actual))
			}
			t.Log("strict original complete standalone export headers/payload/count/listorder", count)
		})
	}
}
