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

func TestP6NativeOriginalStandaloneBuffHPExportGoldens(t *testing.T) {
	for _, fixture := range p6OriginalHitExportFixtures {
		t.Run(fixture[0], func(t *testing.T) { p6OriginalBuffHPFixture(t, fixture[0], fixture[1]) })
	}
}

func TestP6NativeOriginalStandaloneBuffAPExportGoldens(t *testing.T) {
	for _, fixture := range p6OriginalHitExportFixtures {
		t.Run(fixture[0], func(t *testing.T) { p6OriginalBuffHPFixture(t, fixture[0], fixture[1], 2) })
	}
}

func p6OriginalBuffHPFixture(t *testing.T, name, sha string, classes ...int32) {
	wantedClass := int32(1)
	if len(classes) > 0 {
		wantedClass = classes[0]
	}
	t.Helper()
	preciseRoundBegin := name == "native-hit-exports-golden-v81-v66abatch.json" || name == "native-hit-exports-golden-v76-v65abatch.json" || name == "native-hit-exports-golden-v73-v64batch.json" || name == "native-hit-exports-golden-v63-v63batch.json" || name == "native-hit-exports-golden-v62-v62batch.json" || name == "native-hit-exports-golden-v58-v58batch.json" || name == "native-hit-exports-golden-v59-v59batch.json" || name == "native-hit-exports-golden-v60-v60batch.json" || name == "native-hit-exports-golden-v61-v61batch.json"
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "analysis", "p6", name))
	if err != nil || p6Digest(raw) != sha {
		t.Fatal("original standalone Buff HP export digest", err)
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
					Buff   *struct {
						ID string `json:"id"`
					} `json:"buff"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &original); err != nil || len(original.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("original standalone Buff HP denominator", err)
	}
	for file, digest := range map[string]string{original.Source: original.SourceSHA, original.WireRoot: original.WireSHA} {
		body, readErr := os.ReadFile(filepath.Join(root, file))
		if readErr != nil || p6Digest(body) != digest {
			t.Fatal("original standalone Buff HP provenance", file, readErr)
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
				t.Fatal("original standalone Buff HP input SHA", readErr)
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
				if point.Phase != "tick" && point.Phase != "round_end" && !(preciseRoundBegin && point.Phase == "round_begin") {
					continue
				}
				for _, exported := range point.Exports {
					if exported.Cast != "" || exported.Buff == nil {
						continue
					}
					for _, v := range exported.Values {
						if v.Class != wantedClass {
							continue
						}
						if point.Clock != "immediate native snapshot" && point.Clock != "explicit native callback" || v.Extra != 0 {
							t.Fatal("original standalone Buff HP precise clock/extra denominator", point.Clock, v)
						}
						native = append(native, hit{Key: key(point.Round, point.Time, "", exported.Buff.ID, v.Source, v.Target), Time: point.Time, Number: v.Number, Extra: v.Extra, Kind: v.Kind})
					}
				}
			}
			out, computeErr := p6NativeGoldenComputeProjection(t, s, frozen, c.WireSHA, sha, playback.OptRecord, nil)
			if computeErr != nil || out.Authoritative {
				t.Fatal("original standalone Buff HP calculation", computeErr)
			}
			actual := []hit{}
			for _, event := range out.Events {
				if wantedClass == 2 && event.Kind == "buff_value_export" && event.OriginalValueClass == 2 && (event.Time > 0 || preciseRoundBegin && event.Round > 0) {
					actual = append(actual, hit{Key: key(event.Round, event.Time, "", event.Buff, event.Actor, event.Target), Time: event.Time, Number: event.Value, Kind: 0})
				}
				if wantedClass == 1 && event.Kind == "buff_hp_export" && (event.Time > 0 || preciseRoundBegin && event.Round > 0) && event.OriginalHPValue != nil {
					if event.OriginalHPType == nil {
						t.Fatal("original standalone Buff HP type missing")
					}
					actual = append(actual, hit{Key: key(event.Round, event.Time, "", event.Buff, event.Actor, event.Target), Time: event.Time, Number: *event.OriginalHPValue, Kind: *event.OriginalHPType})
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
				extra := []hit{}
				for _, h := range actual {
					if counts[h.Key] > 0 {
						extra = append(extra, h)
						counts[h.Key]--
					}
				}
				t.Fatalf("original standalone Buff HP count %d/%d missing=%+v extra=%+v", len(native), len(actual), missing, extra)
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
					t.Fatalf("original standalone Buff HP value #%d native=%+v go=%+v", i, n, g)
				}
			}
		})
	}
}
