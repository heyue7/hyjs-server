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
	"testing"
)

type p6OriginalCastHeader struct{ Source, Skill, Cast string }

func TestP6NativeOriginalCastCollectionOrderGoldens(t *testing.T) {
	for _, f := range p6OriginalHitExportFixtures {
		t.Run(f[0], func(t *testing.T) { p6OriginalCastCollectionFixture(t, f[0], f[1]) })
	}
}
func p6OriginalCastCollectionFixture(t *testing.T, name, sha string) {
	t.Helper()
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "analysis", "p6", name))
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
					Source string `json:"source"`
					Skill  string `json:"skill"`
					Cast   string `json:"cast"`
				} `json:"exports"`
			} `json:"points"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != p6NativeOriginalPerHitCaseCount(name) {
		t.Fatal("standalone Buff load denominator", err)
	}
	for file, digest := range map[string]string{d.Source: d.SourceSHA, d.WireRoot: d.WireSHA} {
		body, readErr := os.ReadFile(filepath.Join(root, file))
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
			wire, readErr := os.ReadFile(filepath.Join(root, c.Wire))
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
			native := map[string][]p6OriginalCastHeader{}
			clocks := map[string]float64{}
			for _, pt := range c.Points {
				if pt.Phase != "tick" {
					continue
				}
				if pt.Clock != "explicit native callback" && pt.Clock != "immediate native snapshot" {
					t.Fatal("original cast collection callback clock", pt.Clock)
				}
				k := key(pt.Round, pt.Time)
				for _, e := range pt.Exports {
					if e.Cast != "" {
						native[k] = append(native[k], p6OriginalCastHeader{e.Source, e.Skill, e.Cast})
						clocks[k] = pt.Time
					}
				}
			}
			out, err := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, sha, p.OptRecord, nil)
			if err != nil || out.Authoritative {
				t.Fatal("standalone Buff load calculation", err)
			}
			actual := map[string][]p6OriginalCastHeader{}
			for _, e := range out.Events {
				if e.Kind != "cast_export" {
					continue
				}
				k := key(e.Round, e.Time)
				if want, ok := clocks[k]; ok && math.Abs(e.Time-want) > 1e-8 {
					t.Fatal("original cast collection exactclock", e.Time, want)
				}
				actual[k] = append([]p6OriginalCastHeader{{e.Actor, e.Skill, e.Cast}}, actual[k]...)
			}
			count := 0
			for k, values := range native {
				got := actual[k]
				delete(actual, k)
				if len(values) != len(got) {
					t.Fatalf("standalone Buff load exact count clock%s native%d Go%d", k, len(values), len(got))
				}
				for i, n := range values {
					if n != got[i] {
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
