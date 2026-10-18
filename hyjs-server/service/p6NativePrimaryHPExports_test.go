package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This covers cast HP values including primary hits, shared damage, retaliation,
// bleeding and lifesteal, plus zero-effect casts and capped/fully shielded values.
// Standalone Buff HP, AP/SPE/magic-stone values and full export ordering stay separate.
var p6OriginalHitExportFixtures = [][2]string{
	{"native-hit-exports-golden-v81-v66abatch.json", "3d31b9ef5c2f5986af091637cdbfa39311b976e9572a3ec56425decfed4dd942"},
	{"native-hit-exports-golden-v76-v65abatch.json", "5e03947ec05450ab0dac9960a652cf96ad0084967a7401db837f3706a5c9527c"},
	{"native-hit-exports-golden-v73-v64batch.json", "c4fe0019a25ecc7b322145d4c27bf6366262cb1d75483c1d4db6f0dbde7aa28a"},
	{"native-hit-exports-golden-v57-old14.json", "0e8a053f7499e6289ead59d6967f80bb219b8350684d839224999dc207ce89bc"},
	{"native-hit-exports-golden-v57-v54batch.json", "db1f4355db8e39c8b929e380d5e26873c423764ce90ac4d57a5ae4d9c8623853"},
	{"native-hit-exports-golden-v57-v55batch.json", "f69ebaff6d19a8a1b3896fa42a6ab8096babc9813476c03e516e450f42f67f15"},
	{"native-hit-exports-golden-v57-v56batch.json", "6e80fe9322dd049cb0552c1aebd0c1197f77128d4ef68d23fddbe1343b9b98cf"},
	{"native-hit-exports-golden-v57-v57batch.json", "1f9d4a4d9258c76ffd943fe4a6b92826549bce1d59270f5578dc496857fd7a4b"},
	{"native-hit-exports-golden-v58-v58batch.json", "b755b6fcaf78f48edc8324aeeebcd79f88c515a262a80916106bab2bde797f81"},
	{"native-hit-exports-golden-v59-v59batch.json", "c76967629ee4abb5326bd4720a803e3f195b3ba42c7a22dbe91a79840d280b52"},
	{"native-hit-exports-golden-v60-v60batch.json", "5c3ce906437e90841b8047daf10fefbb0bf1531957bd57ca5d89f6831d356fe7"},
	{"native-hit-exports-golden-v61-v61batch.json", "07e392bb506bc61415be5b7ae32200e650bf772496259050d383e9598e92e401"},
	{"native-hit-exports-golden-v62-v62batch.json", "b7d3471c85e1ae7d0940e12ab2bb1f12fd748250fbca1a20b627d6e164f80fac"},
	{"native-hit-exports-golden-v63-v63batch.json", "01b836e3b8794d206b599d11bc81b024fd9308283aadb9cbf0806e8c799d9140"},
}

func TestP6NativeOriginalPrimaryHPExportGoldens(t *testing.T) {
	for _, fixture := range p6OriginalHitExportFixtures {
		t.Run(fixture[0], func(t *testing.T) { p6OriginalPrimaryHPFixture(t, fixture[0], fixture[1]) })
	}
}

func TestP6NativeOriginalCastComboExportGoldens(t *testing.T) {
	for _, fixture := range p6OriginalHitExportFixtures {
		t.Run(fixture[0], func(t *testing.T) { p6OriginalPrimaryHPFixture(t, fixture[0], fixture[1], 3) })
	}
}

func TestP6NativeOriginalCastControlExportGoldens(t *testing.T) {
	for _, class := range []int32{2, 4, 5, 6} {
		t.Run(fmt.Sprint(class), func(t *testing.T) {
			for _, fixture := range p6OriginalHitExportFixtures {
				t.Run(fixture[0], func(t *testing.T) { p6OriginalPrimaryHPFixture(t, fixture[0], fixture[1], class) })
			}
		})
	}
}

func p6OriginalPrimaryHPFixture(t *testing.T, name, sha string, classes ...int32) {
	wantedClass := int32(1)
	if len(classes) != 0 {
		wantedClass = classes[0]
	}
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("original primary HP export digest", err)
	}
	type value struct {
		Class  int32   `json:"exportValClass"`
		Kind   int32   `json:"damageType"`
		Extra  float64 `json:"extraValue"`
		Number float64 `json:"value"`
		Source string  `json:"source"`
		Target string  `json:"target"`
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
					Source string  `json:"source"`
					Cast   string  `json:"cast"`
					Skill  string  `json:"skill"`
					Values []value `json:"values"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &original); err != nil || len(original.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("original primary HP denominator", err)
	}
	for file, digest := range map[string]string{original.Source: original.SourceSHA, original.WireRoot: original.WireSHA} {
		body, readErr := os.ReadFile(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("original primary HP provenance", file, readErr)
		}
	}
	type hit struct {
		Key    string
		Time   float64
		Number float64
		Extra  float64
		Kind   int32
	}
	key := func(round int32, time float64, skill, cast, source, target string) string {
		return fmt.Sprintf("%09d/%09d/%s/%s/%s/%s", round, int64(math.Round(time*10)), skill, cast, source, target)
	}
	s := p6OriginalTables(t)
	for _, c := range original.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, readErr := os.ReadFile(filepath.Join(root, c.Wire))
			if readErr != nil || p6Digest(wire) != c.WireSHA {
				t.Fatal("original primary HP input SHA", readErr)
			}
			playback := &pb.SCFightPlayback{}
			if err = proto.Unmarshal(wire, playback); err != nil {
				t.Fatal(err)
			}
			frozen, freezeErr := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: playback.AttackerFactor, DefenderDamageFactor: playback.DefenderFactor, Attacker: playback.Attacker, Defender: playback.Defender1, Monster: playback.Defender2, Seed: playback.GetRandSeed(), MaxRound: playback.GetMaxRound(), BattleType: playback.GetBattleType(), DefenderFirst: playback.Attacker.GetInst().GetFirstOrder() == 0 && playback.Defender1.GetInst().GetFirstOrder() == 1}, c.WireSHA)
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			native := []hit{}
			for _, point := range c.Points {
				if point.Phase != "tick" {
					continue
				}
				for _, exported := range point.Exports {
					if exported.Cast == "" {
						continue
					}
					for _, v := range exported.Values {
						if v.Class != wantedClass || wantedClass <= 4 && v.Source != exported.Source || wantedClass == 1 && v.Kind > 8 && v.Kind != 12 && v.Kind != 13 && v.Kind != 16 {
							continue
						}
						if point.Clock != "immediate native snapshot" && point.Clock != "explicit native callback" || wantedClass != 4 && v.Extra != 0 {
							t.Fatal("original primary HP precise clock/extra denominator", point.Clock, v)
						}
						native = append(native, hit{Key: key(point.Round, point.Time, exported.Skill, exported.Cast, v.Source, v.Target), Time: point.Time, Number: v.Number, Extra: v.Extra, Kind: v.Kind})
					}
				}
			}
			out, computeErr := p6NativeGoldenComputeProjection(t, s, frozen, c.WireSHA, sha, playback.OptRecord, nil)
			if computeErr != nil || out.Authoritative || (wantedClass == 1 || wantedClass == 3) && len(native) == 0 {
				t.Fatal("original primary HP calculation", computeErr)
			}
			actual := []hit{}
			for _, event := range out.Events {
				if wantedClass != 1 && event.Cast != "" && event.OriginalValueClass == wantedClass {
					source, target := event.Actor, event.Target
					if event.OriginalValueSource != "" {
						source = event.OriginalValueSource
					}
					if event.OriginalValueTarget != "" {
						target = event.OriginalValueTarget
					}
					actual = append(actual, hit{Key: key(event.Round, event.Time, event.Skill, event.Cast, source, target), Time: event.Time, Number: event.Value, Extra: event.OriginalExtraValue, Kind: 0})
				}
				if wantedClass == 1 && (event.Kind == "release" || event.Kind == "hp_export") && event.OriginalHPValue != nil {
					if event.OriginalHPType == nil {
						t.Fatal("original primary HP type missing")
					}
					actual = append(actual, hit{Key: key(event.Round, event.Time, event.Skill, event.Cast, event.Actor, event.Target), Time: event.Time, Number: *event.OriginalHPValue, Kind: *event.OriginalHPType})
				}
			}
			if len(native) != len(actual) {
				counts := map[string]int{}
				for _, h := range actual {
					counts[h.Key]++
				}
				missing := []hit{}
				for _, h := range native {
					if counts[h.Key] == 0 {
						missing = append(missing, h)
					} else {
						counts[h.Key]--
					}
				}
				t.Fatalf("original primary HP count %d/%d missing=%+v", len(native), len(actual), missing)
			}
			// Native batches preserve the full raw order. Compare primary values
			// by their original Tick/cast/actor/target/type, retaining duplicate counts.
			sort.SliceStable(native, func(i, j int) bool {
				return native[i].Key < native[j].Key || native[i].Key == native[j].Key && native[i].Kind < native[j].Kind
			})
			sort.SliceStable(actual, func(i, j int) bool {
				return actual[i].Key < actual[j].Key || actual[i].Key == actual[j].Key && actual[i].Kind < actual[j].Kind
			})
			for i, n := range native {
				g := actual[i]
				if n.Key != g.Key || n.Kind != g.Kind || math.Abs(n.Time-g.Time) > 1e-8 || math.Abs(n.Number-g.Number) > 1e-6 || math.Abs(n.Extra-g.Extra) > 1e-6 {
					t.Fatalf("original primary HP value #%d native=%+v go=%+v", i, n, g)
				}
			}
		})
	}
}
