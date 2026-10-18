package service

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalPaidDaily50NoTargetStrictAndNamedCompatibility(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/p6/daily50-local-pve-no-target-finish-v1-input-20261014/001.pb")
	if err != nil || p6Digest(raw) != "f2497a9f8031b3b2c103a3db1aa1b4d57d267849758f960c6990aa392b0668ed" {
		t.Fatal("actual original paid native input changed", err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	strict, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if strict != nil || err == nil || !strings.Contains(err.Error(), constUtil.P6OriginalAINoTargetStall) {
		t.Fatal("original AI stall cannot manufacture a winner", strict, err)
	}
	input.PVECompatibilityRule = constUtil.P6LocalPVENoTargetFinish
	before, _ := json.Marshal(input)
	var (
		observer  func(*combatRoundEngine)
		tickFile  *os.File
		tickError error
	)
	if path := os.Getenv("HYJS_P6_LOCAL_PVE_COMPAT_TICKS_OUTPUT"); path != "" {
		tickFile, err = os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		encoder := json.NewEncoder(tickFile)
		observer = func(e *combatRoundEngine) {
			var players []map[string]any
			for _, player := range e.players {
				var actors []map[string]any
				for _, actor := range player.actors {
					actors = append(actors, map[string]any{"uid": actor.uid, "hp": actor.hp, "state": actor.state})
				}
				players = append(players, map[string]any{"anger": player.anger, "actors": actors})
			}
			if tickError == nil {
				tickError = encoder.Encode(map[string]any{"round": e.round, "time": e.time, "randomDraws": e.rng.Draws(), "players": players, "casts": e.diagnosticCasts})
			}
		}
	}
	out, err := s.computeFrozenBattleObserved(t.Context(), input, nil, observer)
	if tickFile != nil {
		closeErr := tickFile.Close()
		if tickError != nil || closeErr != nil {
			t.Fatal("completed compatibility tick output", tickError, closeErr)
		}
	}
	if err != nil || out.Authoritative || out.PVECompatibilityRule != constUtil.P6LocalPVENoTargetFinish {
		t.Fatal("named local compatibility computation", err)
	}
	after, _ := json.Marshal(input)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("controller recovery mutated frozen input")
	}
	var recovered bool
	for _, event := range out.Events {
		if event.Kind == "compat_no_target_finish" && event.Round == 4 && event.Time == 4.100000000000001 && event.Skill == "1071_1" && event.Value == 2.400000000000001 {
			recovered = true
		}
	}
	if !recovered {
		t.Fatal("actual failed1071 press was not recorded in controller action")
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) || out.Winner != replay.Winner {
		t.Fatal("named controller action replay drift", err)
	}
	if output := os.Getenv("HYJS_P6_LOCAL_PVE_COMPAT_OUTPUT"); output != "" {
		encoded, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(encoded, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual paid local compatibility only: finalRound=%d winner=%d RNG=%d events=%d; original native compatibility comparison pending", out.FinalRound, out.Winner, out.RandomDraws, len(out.Events))
}

func TestP6NamedPVENoTargetFinishRejectsMissingEvidenceAndUnfinishedSkills(t *testing.T) {
	for _, mode := range []string{"strict", "missing-press", "old-round", "wrong-camp", "non-nil-target", "active", "queued", "pending-ultimate", "unfinished-other", "valid"} {
		t.Run(mode, func(t *testing.T) {
			a := &combatRoundActor{camp: 1, uid: "1_actual-failed", hero: p6Actor("hero_047", "actual-failed", "047_1,047_2,047_3", 1, 100)}
			skill := &combatRoundSkill{actor: a, id: "failed", row: map[string]string{}, state: 0}
			a.skills = []*combatRoundSkill{skill}
			e := &combatRoundEngine{input: &model.FrozenBattleInput{Monster: &pb.BattleMonster{}, PVECompatibilityRule: constUtil.P6LocalPVENoTargetFinish}, round: 4, camp: 1, rng: NewCombatRandom(42), players: [2]*combatRoundPlayer{{}, {actors: []*combatRoundActor{a}}}, copyEnemyDealt: []*combatRoundSkill{skill}, pveNoTargetPresses: map[*combatRoundSkill]combatRoundNoTargetPress{skill: {round: 4, time: 2.4, camp: 1}}}
			switch mode {
			case "strict":
				e.input.PVECompatibilityRule = ""
			case "missing-press":
				e.pveNoTargetPresses = nil
			case "old-round":
				e.pveNoTargetPresses[skill] = combatRoundNoTargetPress{round: 3, time: 2.4, camp: 1}
			case "wrong-camp":
				e.pveNoTargetPresses[skill] = combatRoundNoTargetPress{round: 4, time: 2.4, camp: 0}
			case "non-nil-target":
				skill.target = &combatRoundActor{}
			case "active":
				a.current = skill
			case "queued":
				e.commands = []combatRoundCommand{{actor: a, skills: []*combatRoundSkill{skill}}}
			case "pending-ultimate":
				e.pendingUltimate = skill
			case "unfinished-other":
				a.skills = append(a.skills, &combatRoundSkill{state: 2})
			}
			before := e.rng.Draws()
			err := e.validateOriginalCopyEnemyRoundFinish()
			if (err == nil) != (mode == "valid") || e.rng.Draws() != before || skill.state != 0 || a.hp != 0 {
				t.Fatal("recovery safety or original state mutation", mode, err)
			}
		})
	}
}

func TestP6CompatibilityPolicyValidationAndLegacyEmptyJSON(t *testing.T) {
	for _, input := range []model.FrozenBattleInput{{PVECompatibilityRule: "unknown", Monster: &pb.BattleMonster{}}, {PVECompatibilityRule: constUtil.P6LocalPVENoTargetFinish}, {PVECompatibilityRule: constUtil.P6LocalPVENoTargetFinish, Monster: &pb.BattleMonster{}, Defender: &pb.BattlePlayer{}}, {PVECompatibilityRule: constUtil.P6LocalPVENoTargetFinish, Monster: &pb.BattleMonster{}, KFReplay: &pb.KFReplayData{}}, {PVECompatibilityRule: constUtil.P6LocalPVENoTargetFinish, Monster: &pb.BattleMonster{}, BattleType: 2}} {
		if ValidateP6PVECompatibility(input) == nil {
			t.Fatal("unknown/PVP/KF rule accepted")
		}
	}
	for _, input := range []model.FrozenBattleInput{{}, {Monster: &pb.BattleMonster{}, BattleType: 4, PVECompatibilityRule: constUtil.P6LocalPVENoTargetFinish}} {
		if err := ValidateP6PVECompatibility(input); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(model.FrozenBattleInput{})
	if strings.Contains(string(raw), "pveCompatibilityRule") {
		t.Fatal("empty historical JSON acquired a new policy field")
	}
}
