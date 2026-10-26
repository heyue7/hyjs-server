package service

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6NativeOriginalBuffInstanceAliasingAcrossLifecycle(t *testing.T) {
	const file = "native-next-real-skills-buff-aliases-v100.json"
	raw, err := readTestAnalysis75("../../analysis/p6/" + file)
	if err != nil {
		t.Fatal(err)
	}
	// The immutable native sidecar SHA is filled only after verified extraction.
	if p6Digest(raw) != "22bdfb036417691dfe3fba7c034a599848622df966a6491b859dc52c1869d24d" {
		t.Fatal("native Buff alias source SHA")
	}
	var d struct {
		Source     string `json:"sourceManifest"`
		SourceSHA  string `json:"sourceManifestSHA256"`
		WireSource string `json:"wireSourceManifest"`
		WireSHA    string `json:"wireSourceManifestSHA256"`
		Cases      []struct {
			Name         string `json:"case"`
			Wire         string `json:"wire"`
			WireSHA      string `json:"wireSHA256"`
			Observations []struct {
				ID, Source, Target, Talent, Skill, Cast, Role string
				Round                                         int32
				Time                                          float64
				NativeUID, OriginalName, OriginalType         string
				HeaderSource                                  string
				HeaderCast                                    string
			}
		}
	}
	if err = json.Unmarshal(raw, &d); err != nil || len(d.Cases) != 14 {
		t.Fatal("exact source14", err)
	}
	for _, binding := range [][2]string{{d.Source, d.SourceSHA}, {d.WireSource, d.WireSHA}} {
		body, e := readTestAnalysis75(filepath.Join("../..", binding[0]))
		if e != nil || p6Digest(body) != binding[1] {
			t.Fatal("original alias provenance", e)
		}
	}
	s := p6OriginalTables(t)
	type ref struct {
		uid      string
		instance int64
	}
	key := func(round int32, time float64, role, headerSource, headerCast string, b *model.CombatBuffExportSnapshot) string {
		return fmt.Sprintf("%09d/%09d/%s/%s/%s/%s/%s/%s/%s/%s/%s", round, int64(math.Round(time*10)), role, headerSource, headerCast, b.ID, b.Source, b.Target, b.Talent, b.Skill, b.Cast)
	}
	for _, c := range d.Cases {
		t.Run(c.Name, func(t *testing.T) {
			body, e := readTestAnalysis75(filepath.Join("../..", c.Wire))
			if e != nil || p6Digest(body) != c.WireSHA {
				t.Fatal("original alias wire", e)
			}
			wire := &pb.SCFightPlayback{}
			if e = proto.Unmarshal(body, wire); e != nil {
				t.Fatal(e)
			}
			input, e := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor}, c.WireSHA)
			if e != nil {
				t.Fatal(e)
			}
			native := map[string][]ref{}
			nativeSeen := map[string]bool{}
			for _, o := range c.Observations {
				row := s.table("Buff", "Id", o.ID)
				typ := row["Factor"]
				if shield := (&combatRoundBuff{row: row}).shieldKind(); shield != "" {
					typ = shield
				}
				if row == nil || row["Name"] != o.OriginalName || typ != o.OriginalType || o.NativeUID == "" {
					t.Fatal("original raw Buff name/type/UID", o.ID, o.OriginalName, o.OriginalType)
				}
				b := &model.CombatBuffExportSnapshot{ID: o.ID, Source: o.Source, Target: o.Target, Talent: o.Talent, Skill: o.Skill, Cast: o.Cast}
				k := key(o.Round, o.Time, o.Role, o.HeaderSource, o.HeaderCast, b)
				seen := k + "/" + o.NativeUID
				if !nativeSeen[seen] {
					native[k] = append(native[k], ref{uid: o.NativeUID})
					nativeSeen[seen] = true
				}
			}
			out, e := p6NativeGoldenComputeProjection(t, s, input, c.WireSHA, p6Digest(raw), wire.OptRecord, nil)
			if e != nil || out.Authoritative {
				t.Fatal(e)
			}
			calculated := map[string][]ref{}
			calculatedSeen := map[string]bool{}
			add := func(event model.CombatEvent, role string, b *model.CombatBuffExportSnapshot, instance int64) {
				if b == nil {
					return
				}
				if instance <= 0 {
					t.Fatal("calculated Buff snapshot lost its real object identity", event.Kind)
				}
				headerSource, headerCast := b.Source, event.OriginalExportCast
				if headerCast != "" {
					headerSource = event.OriginalExportSource
				}
				k := key(event.Round, event.Time, role, headerSource, headerCast, b)
				seen := fmt.Sprintf("%s/%d", k, instance)
				if !calculatedSeen[seen] {
					calculated[k] = append(calculated[k], ref{instance: instance})
					calculatedSeen[seen] = true
				}
			}
			for _, event := range out.Events {
				role := ""
				switch event.Kind {
				case "buff_load":
					role = "load"
				case "buff_unload":
					role = "unload"
				case "buff_empty_export":
					role = "empty"
				case "buff_dead_export":
					role = "dead"
				default:
					if event.OriginalValueClass > 0 || event.OriginalHPValue != nil {
						role = "value"
					}
				}
				if role != "" {
					add(event, role, event.OriginalBuffSnapshot, event.OriginalBuffInstance)
				}
				if event.OriginalBuffParentSnapshot != nil {
					add(event, "parentload", event.OriginalBuffParentSnapshot, event.OriginalBuffParentInstance)
				}
			}
			if len(native) != len(calculated) {
				for k := range native {
					if _, ok := calculated[k]; !ok {
						t.Log("native-only", k)
					}
				}
				for k := range calculated {
					if _, ok := native[k]; !ok {
						t.Log("Go-only", k)
					}
				}
				t.Fatal("Buff alias observation key count", len(native), len(calculated))
			}
			toGo := map[string]int64{}
			toNative := map[int64]string{}
			observations := 0
			for k, want := range native {
				got := calculated[k]
				if len(want) != len(got) {
					t.Fatalf("Buff alias roles differ %s native%d Go%d", k, len(want), len(got))
				}
				for i, n := range want {
					g := got[i].instance
					if old, ok := toGo[n.uid]; ok && old != g {
						t.Fatal("original same Buff mapped to different calculated objects", k, n.uid, old, g)
					}
					if old, ok := toNative[g]; ok && old != n.uid {
						t.Fatal("different original Buff objects merged", k, old, n.uid, g)
					}
					toGo[n.uid] = g
					toNative[g] = n.uid
					observations++
				}
			}
			t.Log("actual original object alias bijection", len(toGo), "references", observations)
		})
	}
}
