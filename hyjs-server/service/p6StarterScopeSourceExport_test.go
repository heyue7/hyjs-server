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

// Export actual allowed starter profiles through the normal roster projection.
// Levels are explicitly finite isolation setup; every original main GUID remains
// in its own formation. Only the last two rows alter HP for terminal probes.
func TestP6ExportStarterFriendCandidateSources(t *testing.T) {
	folder := os.Getenv("HYJS_P6_STARTER_SCOPE_SOURCE_OUTPUT")
	if folder == "" {
		t.Skip("explicit starter source export disabled")
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
		t.Fatal("preserve immutable candidate source")
	}
	if err = os.MkdirAll(absolute, 0755); err != nil {
		t.Fatal(err)
	}
	f, other, _ := p11ArenaSetup(t)
	project := func(id int64, level int32) *pb.BattlePlayer {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(level); return nil }); err != nil {
			t.Fatal(err)
		}
		p, g := p10LuckyProfile(t, id)
		b, err := f.s.battlePlayer(p, g)
		if err != nil || len(b.GetFormation()) != 1 || b.Formation[0].GetGuid() != p.GetHeroWarId() || b.Formation[0].GetId() != "hero_047" {
			t.Fatal("actual legal main roster", err, b)
		}
		return b
	}
	pairs := [][2]int32{{1, 1}, {20, 20}, {100, 100}, {100, 1}, {1, 100}, {20, 100}, {20, 20}, {100, 100}}
	seeds := []int64{42, 0, 1, -1, 2, 3, 7, 281474976710655}
	rows := []map[string]any{}
	for index, pair := range pairs {
		a, b := project(f.id, pair[0]), project(other, pair[1])
		finite := index >= 6
		if finite {
			hp := float64(100)
			if index == 7 {
				hp = 1000000
			}
			for _, p := range []*pb.BattlePlayer{a, b} {
				p.Formation[0].Prop.Hp = proto.Float64(hp)
				p.Formation[0].Inst.CurHp = proto.Float64(hp)
			}
		}
		defenderFirst := index%2 == 1
		if err = SetP6FirstOrder(a, b, defenderFirst); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("%03d", index)
		fightID := "starter-friend-candidate-" + name
		wire := &pb.SCFightPlayback{Fightid: proto.String(fightID), Attacker: a, Defender1: b, RandSeed: proto.Int64(seeds[index]), BattleType: proto.Int32(11), MaxRound: proto.Int32(40), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
		callback := &pb.SCFriendFight{FightId: proto.String(fightID), RoomId: proto.String("map_forest_001"), RandSeed: proto.Int64(seeds[index]), PlayerList: []*pb.BattlePlayer{a, b}, MaxRoundNum: proto.Int32(40)}
		frozen, e := f.s.FreezeBattleInput(model.FrozenBattleInput{Attacker: a, Defender: b, Seed: seeds[index], BattleType: 11, MaxRound: 40, DefenderFirst: defenderFirst, AppliedProperties: map[string][]string{"source": {"actual isolated allowed CreatePlayer(hero_047) main roster; finite level setup; no main removed", fmt.Sprintf("explicit finite HP override=%t", finite)}}})
		if e != nil {
			t.Fatal(e)
		}
		result, e := f.s.ComputeOriginalP6ProtocolFinalHP(t.Context(), frozen, nil)
		if e != nil || result.Outcome.Authoritative {
			t.Fatal("candidate compute", e)
		}
		write := func(suffix string, v any) string {
			var raw []byte
			var err error
			if msg, ok := v.(proto.Message); ok {
				raw, err = proto.MarshalOptions{Deterministic: true}.Marshal(msg)
			} else {
				raw, err = json.MarshalIndent(v, "", "  ")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(absolute, name+suffix), raw, 0644); err != nil {
				t.Fatal(err)
			}
			return p6Digest(raw)
		}
		sha := write(".pb", wire)
		callbackSHA := write(".SCFriendFight.pb", callback)
		inputSHA := write(".frozen-input.json", frozen)
		terminalSHA := write(".go-terminal.json", result)
		stats := map[string]int{}
		for _, e := range result.Outcome.Events {
			stats[e.Kind]++
		}
		rows = append(rows, map[string]any{"case": fmt.Sprintf("legal-starter047-friend11/levels%d-vs-%d/first%d/finiteHP%t", pair[0], pair[1], b.Inst.GetFirstOrder(), finite), "wire": name + ".pb", "wireSHA256": sha, "fightId": fightID, "seed": seeds[index], "battleType": 11, "firstCamp": int32(0), "originalCallback": name + ".SCFriendFight.pb", "originalCallbackSHA256": callbackSHA, "frozenInput": name + ".frozen-input.json", "frozenInputSHA256": inputSHA, "goTerminal": name + ".go-terminal.json", "goTerminalSHA256": terminalSHA, "explicitFiniteHP": finite, "goObservedEvents": stats, "goFinalRound": result.Outcome.FinalRound, "goWinner": result.Outcome.Winner, "status": "awaiting-original-automatic-ai"})
		if defenderFirst {
			rows[len(rows)-1]["firstCamp"] = int32(1)
		}
	}
	manifest := map[string]any{"scope": "Original allowed starter047 actual isolated profile roster through battlePlayer, mandatory HeroWarId retained. Level1/20/100 are explicit finite setup; two HP overrides are mechanism probes. Pair/seed/initiative are finite diagnostic sources, not paid/private-server fights. No result supplied to original callback; native comparison and capability proof pending.", "engineVersion": constUtil.P6EngineVersion, "cases": rows, "nativeCompared": false, "authoritative": false}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(absolute, "manifest.json"), append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
