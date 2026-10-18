package service

import (
	"encoding/hex"
	"encoding/json"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func p6NativeDailyInput(t *testing.T, fixturePath string) (*model.FrozenBattleInput, *pb.BattleOperate) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/" + fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		PlayerID string `json:"player_id"`
		StartHex string `json:"start_hex"`
		EndHex   string `json:"end_hex"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || fixture.PlayerID != "4" {
		t.Fatal("original player4 fixture required", err)
	}
	startRaw, err := hex.DecodeString(fixture.StartHex)
	if err != nil {
		t.Fatal(err)
	}
	endRaw, err := hex.DecodeString(fixture.EndHex)
	if err != nil {
		t.Fatal(err)
	}
	start, end := &pb.SCDailyCopyFightStart{}, &pb.CSDailyCopyFightEnd{}
	if err = proto.Unmarshal(startRaw, start); err != nil {
		t.Fatal(err)
	}
	if err = proto.Unmarshal(endRaw, end); err != nil {
		t.Fatal(err)
	}
	input, err := p6OriginalTables(t).FreezeBattleInput(model.FrozenBattleInput{Attacker: start.BattlePlayer, Monster: start.Monster, Seed: start.GetRandSeed(), MaxRound: start.GetMaxRoundNum(), BattleType: 4})
	if err != nil {
		t.Fatal(err)
	}
	return input, end.OptRecord
}

// The expected HP/AP/cast timeline comes exclusively from original phone logging sites.
// No server-computed values are used to author this golden or to claim full-content parity.
func p6AssertNativePlaybackGolden(t *testing.T, caseName string, outcome *model.CombatOutcome) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/native-playback-golden-" + caseName + "-20261014.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		SourceLog    string `json:"sourceLog"`
		SourceSHA256 string `json:"sourceSHA256"`
		HP           []struct {
			Target   string  `json:"target"`
			TargetHP float64 `json:"targetHp"`
		} `json:"hpMutations"`
		PrimaryHits []struct {
			Target string  `json:"target"`
			Skill  string  `json:"skill"`
			Cast   string  `json:"cast"`
			Kind   string  `json:"kind"`
			Value  float64 `json:"value"`
		} `json:"primaryHits"`
		AP []struct {
			Camp  int32   `json:"camp"`
			Value float64 `json:"value"`
			Total float64 `json:"total"`
		} `json:"ap"`
		Releases []struct {
			Round int32   `json:"round"`
			Time  float64 `json:"time"`
			Skill string  `json:"skill"`
			Cast  string  `json:"cast"`
		} `json:"releases"`
		Special []struct {
			Kind       string  `json:"kind"`
			Round      int32   `json:"round"`
			Time       float64 `json:"time"`
			Skill      string  `json:"skill"`
			Cast       string  `json:"cast"`
			TargetHero string  `json:"targetHero"`
		} `json:"special"`
	}
	if err = json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile("../../analysis/p6/" + golden.SourceLog)
	if err != nil || p6Digest(log) != golden.SourceSHA256 {
		t.Fatal("native golden source log drifted", err)
	}
	hp, ap, releases, hits := []model.CombatEvent{}, []model.CombatEvent{}, []model.CombatEvent{}, []model.CombatEvent{}
	for _, event := range outcome.Events {
		switch event.Kind {
		case "damage", "lifesteal", "buff_heal", "skill_heal", "anger_steal_heal", "anger_hp_heal", "round_heal", "revive", "teammate_revive":
			if event.Target != "" {
				hp = append(hp, event)
			}
		case "anger":
			ap = append(ap, event)
		case "release":
			hits = append(hits, event)
			// One native FRelease exports every selected area recipient in order.
			if len(releases) == 0 || releases[len(releases)-1].Round != event.Round || releases[len(releases)-1].Time != event.Time || releases[len(releases)-1].Cast != event.Cast || releases[len(releases)-1].Actor != event.Actor {
				releases = append(releases, event)
			}
		}
	}
	if len(hp) != len(golden.HP) || len(ap) != len(golden.AP) || len(releases) != len(golden.Releases) {
		t.Fatal("native timeline event counts", caseName, len(hp), len(ap), len(releases), len(golden.HP), len(golden.AP), len(golden.Releases))
	}
	for index, expected := range golden.HP {
		if hp[index].Target != expected.Target || math.Abs(hp[index].TargetHP-expected.TargetHP) > 1e-6 {
			t.Fatal("original per-event HP drift", caseName, index, hp[index], expected)
		}
	}
	if len(hits) != len(golden.PrimaryHits) {
		t.Fatal("original primary nominal hit count drift", caseName, len(hits), len(golden.PrimaryHits))
	}
	for index, expected := range golden.PrimaryHits {
		actual := hits[index]
		if actual.Target != expected.Target || actual.Skill != expected.Skill || actual.Cast != expected.Cast || actual.Critical != strings.Contains(expected.Kind, "Crit") || math.Abs(actual.Value+expected.Value) > 1e-6 {
			t.Fatal("original primary nominal damage/critical drift", caseName, index, actual, expected)
		}
	}
	totals := [2]float64{}
	for index, expected := range golden.AP {
		totals[ap[index].Camp] += ap[index].Value
		if ap[index].Camp != expected.Camp || math.Abs(ap[index].Value-expected.Value) > 1e-7 || math.Abs(totals[expected.Camp]-expected.Total) > 1e-7 {
			t.Fatal("original per-event AP drift", caseName, index, ap[index], expected)
		}
	}
	for index, expected := range golden.Releases {
		actual := releases[index]
		if actual.Round != expected.Round || math.Abs(actual.Time-expected.Time) > 1e-8 || actual.Skill != expected.Skill || actual.Cast != expected.Cast {
			t.Fatal("original per-release time/skill/cast drift", caseName, index, actual, expected)
		}
	}
	var special []model.CombatEvent
	for index, event := range outcome.Events {
		if event.Kind != "magic_stone" && event.Kind != "water_ball" {
			continue
		}
		// Native logs the hit recipient's hero ID. Magic-stone traces use the
		// rewarded teammate, and its release is the following same-time hit trace.
		if event.Kind == "magic_stone" {
			for _, following := range outcome.Events[index+1:] {
				if following.Kind == "release" {
					event.Skill, event.Cast = following.Skill, following.Cast
					break
				}
			}
		}
		special = append(special, event)
	}
	if len(special) != len(golden.Special) {
		t.Fatal("original special branch count drift", caseName, len(special), len(golden.Special))
	}
	heroes := map[string]string{}
	for _, actor := range outcome.Attacker.Formation {
		heroes["0_"+actor.GetGuid()] = actor.GetId()
	}
	if outcome.Defender != nil {
		for _, actor := range outcome.Defender.Formation {
			heroes["1_"+actor.GetGuid()] = actor.GetId()
		}
	} else {
		for _, actor := range outcome.Monster.Formation {
			heroes["1_"+actor.GetGuid()] = actor.GetId()
		}
	}
	for index, expected := range golden.Special {
		actual := special[index]
		if actual.Kind != expected.Kind || actual.Round != expected.Round || math.Abs(actual.Time-expected.Time) > 1e-8 || actual.Skill != expected.Skill || actual.Cast != expected.Cast || heroes[actual.Target] != expected.TargetHero {
			t.Fatal("original water-ball/magic-stone branch drift", caseName, index, actual, expected)
		}
	}
	if outcome.Authoritative {
		t.Fatal("two native fixtures cannot enable global authority")
	}
}

func TestP6NativeDailyBothOriginalFullHPAPReleaseGolden(t *testing.T) {
	for _, tc := range []struct{ name, fixture string }{{"solo", "daily-id4-native-20261013.json"}, {"food", "daily-fooddark-id4-native-20261013.json"}} {
		t.Run(tc.name, func(t *testing.T) {
			input, operations := p6NativeDailyInput(t, tc.fixture)
			out, err := (&GameService{Tables: input.Tables}).ComputeFrozenBattle(t.Context(), input, operations)
			if err != nil {
				t.Fatal(err)
			}
			p6AssertNativePlaybackGolden(t, tc.name, out)
		})
	}
}
