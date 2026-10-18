package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This optional source export uses actual P5 bare/training actors and records
// explicit finite HP100 overrides for four separate terminal mechanism probes.
// Pairing/seed/initiative are explicit mechanism probes, never paid Arena inputs.
// Both normal skills have zero anger and no effects, so initialAP0 is invariant:
// the source ultimate cannot become ready and no Buff interaction is introduced.
func TestP6ExportLinearPhysicalCandidateSources(t *testing.T) {
	folder := os.Getenv("HYJS_P6_LINEAR_SCOPE_SOURCE_OUTPUT")
	if folder == "" {
		t.Skip("explicit native candidate source export disabled")
	}
	analysis, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(folder)
	if err != nil || !strings.HasPrefix(absolute, analysis+string(os.PathSeparator)) {
		t.Fatal("candidate source path")
	}
	if _, err = os.Stat(filepath.Join(absolute, "manifest.json")); err == nil {
		t.Fatal("preserve immutable candidate input")
	}
	if err = os.MkdirAll(absolute, 0755); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	actors := map[string]*pb.BattleHero{}
	provenance := []map[string]string{}
	for _, stage := range []string{"breach", "training"} {
		name := "p5-" + stage + "-highest-actors-20261014.json"
		raw, e := os.ReadFile(filepath.Join(analysis, name))
		if e != nil {
			t.Fatal(e)
		}
		var source struct {
			Paths []struct {
				HeroID string          `json:"heroId"`
				Hero   json.RawMessage `json:"battleHero"`
			} `json:"characterPaths"`
		}
		if e = json.Unmarshal(raw, &source); e != nil {
			t.Fatal(e)
		}
		for _, row := range source.Paths {
			if row.HeroID != "hero_069" && row.HeroID != "hero_070" {
				continue
			}
			hero := &pb.BattleHero{}
			if e = json.Unmarshal(row.Hero, hero); e != nil {
				t.Fatal(e)
			}
			if len(hero.TalentList) != 0 || hero.GetAwakeStage() != 0 || hero.GetAwakenSkill() != "" || hero.Inst == nil || hero.Prop == nil {
				t.Fatal("actual P5 source is outside effect-free candidate")
			}
			actors[stage+"/"+row.HeroID] = hero
		}
		provenance = append(provenance, map[string]string{"path": "analysis/p6/" + name, "sha256": p6Digest(raw)})
	}
	pairs := [][2]string{{"breach/hero_069", "breach/hero_070"}, {"breach/hero_070", "breach/hero_069"}, {"training/hero_069", "breach/hero_070"}, {"breach/hero_069", "training/hero_070"}, {"training/hero_069", "training/hero_070"}, {"training/hero_070", "training/hero_069"}, {"breach/hero_069", "breach/hero_069"}, {"training/hero_070", "training/hero_070"}}
	seeds := []int64{42, 0, 1, -1, 2, 3, 7, 281474976710655}
	rows := []map[string]any{}
	for index, pair := range pairs {
		first := index%2 == 0
		left, right := proto.Clone(actors[pair[0]]).(*pb.BattleHero), proto.Clone(actors[pair[1]]).(*pb.BattleHero)
		panelProbe := index == 2 || index == 3 || index == 6 || index == 7
		if panelProbe {
			// Explicit bounded HP probes cover actual death/terminal callbacks.
			// Keep these separate from the unchanged real P5 panel sources.
			left.Prop.Hp, right.Prop.Hp = proto.Float64(100), proto.Float64(100)
			left.Inst.CurHp, right.Inst.CurHp = proto.Float64(100), proto.Float64(100)
		}
		a, b := int32(0), int32(1)
		if first {
			a, b = 1, 0
		}
		attacker := &pb.BattlePlayer{PlayerId: proto.Int64(1), ServerId: proto.Int32(1), Formation: []*pb.BattleHero{left}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0), FirstOrder: proto.Int32(a)}}
		defender := &pb.BattlePlayer{PlayerId: proto.Int64(2), ServerId: proto.Int32(1), Formation: []*pb.BattleHero{right}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0), FirstOrder: proto.Int32(b)}}
		name := fmt.Sprintf("%03d", index)
		wire := &pb.SCFightPlayback{Fightid: proto.String("linear-physical-candidate-" + name), Attacker: attacker, Defender1: defender, RandSeed: proto.Int64(seeds[index]), BattleType: proto.Int32(2), MaxRound: proto.Int32(40), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
		frozen, e := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: attacker, Defender: defender, Seed: seeds[index], BattleType: 2, MaxRound: 40, DefenderFirst: !first, AppliedProperties: map[string][]string{"scope": {fmt.Sprintf("candidate finite composition probe; actual P5 actor source; explicit finiteHP100=%t; not naturally paid Arena source", panelProbe)}}})
		if e != nil {
			t.Fatal(e)
		}
		out, e := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), frozen, nil)
		if e != nil || out.Outcome.Authoritative {
			t.Fatal("candidate calculation", e)
		}
		for _, event := range out.Outcome.Events {
			if event.Kind == "skill_press" && event.Value == 2 || event.Kind == "buff_load" || event.Kind == "anger" {
				t.Fatal("zero-anger/effect-free closure invariant failed", event)
			}
		}
		write := func(suffix string, value any) string {
			var raw []byte
			var e error
			if msg, ok := value.(proto.Message); ok {
				raw, e = proto.MarshalOptions{Deterministic: true}.Marshal(msg)
			} else {
				raw, e = json.MarshalIndent(value, "", "  ")
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(absolute, name+suffix), raw, 0644); e != nil {
				t.Fatal(e)
			}
			return p6Digest(raw)
		}
		wireSHA := write(".pb", wire)
		outcomeSHA := write(".go-terminal.json", out)
		rows = append(rows, map[string]any{"case": "linear-physical-candidate/" + strings.Join(pair[:], "-vs-") + fmt.Sprintf("/initiative%d", a), "wire": name + ".pb", "wireSHA256": wireSHA, "fightId": wire.GetFightid(), "seed": seeds[index], "battleType": 2, "firstCamp": b, "goTerminal": name + ".go-terminal.json", "goTerminalSHA256": outcomeSHA, "sourceActors": pair, "explicitFiniteHP100": panelProbe, "status": "awaiting-original-automatic-ai"})
	}
	manifest := map[string]any{"scope": "Candidate original PVP2 one actor per side, complete actual Hero069/070 skill groups, no Talent/Awake/Buff, initial anger0 invariant. Actual P5 source actors with exact per-case finiteHP100 overrides recorded; pair/seed/initiative are explicit mechanism probes, not naturally paid Arena source. Native proof and scoped settlement certification pending.", "engineVersion": constUtil.P6EngineVersion, "sourceProvenance": provenance, "cases": rows, "nativeCompared": false, "authoritative": false}
	raw, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(absolute, "manifest.json"), append(raw, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
}
