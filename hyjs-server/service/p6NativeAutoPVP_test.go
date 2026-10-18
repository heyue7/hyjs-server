package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p6NativeAutoCallback struct {
	Round int32   `json:"round"`
	Time  float64 `json:"time"`
	Skill string  `json:"skill"`
	Cast  string  `json:"cast"`
}

type p6NativeAutoTick struct {
	Round       int32   `json:"round"`
	Time        float64 `json:"time"`
	RandomDraws int64   `json:"randomDraws"`
	Players     []struct {
		Anger  float64 `json:"anger"`
		Actors []struct {
			UID string  `json:"uid"`
			HP  float64 `json:"hp"`
		} `json:"actors"`
	} `json:"players"`
}

func TestP6NativeAutomaticPVPOriginalPriorityTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-pvp-priority-tick-golden-v48.json", "643db0054ff2e421fa34c209692a85c321a85f581ee0f81868df1c14c8333cf4", 14)
}

func TestP6NativeOriginalDamageFactorAndRoundExhaustionTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-damage-factor-tick-golden-v51.json", "7f081406545f3cf72b290445a6adcc33b5f49088de0c586d5035268e4d3f7735", 4)
}

func p6NativeAutomaticPVPOriginalTickGolden(t *testing.T, file, expectedSHA string, count int) {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p6/" + file)
	if err != nil || p6Digest(raw) != expectedSHA {
		t.Fatal("original derived numerical fixture hash", err)
	}
	var golden struct {
		Manifest    string `json:"sourceManifest"`
		ManifestSHA string `json:"sourceManifestSHA256"`
		WireSource  string `json:"wireSourceManifest"`
		Cases       []struct {
			Name        string                   `json:"case"`
			Wire        string                   `json:"wire"`
			WireSHA     string                   `json:"wireSHA256"`
			Operates    []*pb.BattleSkillOperate `json:"operates"`
			Begins      []p6NativeAutoCallback   `json:"begins"`
			Casts       []p6NativeAutoCallback   `json:"casts"`
			Ticks       []p6NativeAutoTick       `json:"ticks"`
			RandomDraws int64                    `json:"randomDraws"`
			FinalRound  int32                    `json:"finalRound"`
			AttackerWon bool                     `json:"attackerWon"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &golden); err != nil || len(golden.Cases) != count {
		t.Fatal("original priority case denominator", err)
	}
	manifest, err := os.ReadFile(filepath.Join("../..", golden.Manifest))
	if err != nil || p6Digest(manifest) != golden.ManifestSHA {
		t.Fatal("original phone manifest hash changed", err)
	}
	s := p6OriginalTables(t)
	wireRoot := p6NativeOriginalWireMirrorRoot(filepath.Dir(golden.WireSource))
	for _, tc := range golden.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			wire, err := os.ReadFile(filepath.Join(wireRoot, tc.Wire))
			if err != nil || p6Digest(wire) != tc.WireSHA {
				t.Fatal("actual original paid-growth wire hash", err)
			}
			original := &pb.SCFightPlayback{}
			if err = proto.Unmarshal(wire, original); err != nil {
				t.Fatal(err)
			}
			frozen, err := p6NativeGoldenFreeze(t, s, model.FrozenBattleInput{AttackerDamageFactor: original.AttackerFactor, DefenderDamageFactor: original.DefenderFactor, Attacker: original.Attacker, Defender: original.Defender1, Monster: original.Defender2, Seed: original.GetRandSeed(), MaxRound: original.GetMaxRound(), BattleType: original.GetBattleType(), DefenderFirst: original.Attacker.GetInst().GetFirstOrder() == 0 && original.Defender1.GetInst().GetFirstOrder() == 1}, tc.WireSHA)
			if err != nil {
				t.Fatal(err)
			}
			// Original callback snapshots sample completed ticks. The observer
			// compares those exact clocks without inferring silent RNG from events.
			wanted := make(map[string]p6NativeAutoTick, len(tc.Ticks))
			for _, tick := range tc.Ticks {
				wanted[fmt.Sprintf("%d/%d", tick.Round, int64(math.Round(tick.Time*10)))] = tick
			}
			matched := 0
			var casts []p6NativeAutoCallback
			out, err := p6NativeGoldenComputeProjection(t, s, frozen, tc.WireSHA, expectedSHA, original.OptRecord, func(e *combatRoundEngine) {
				for _, cast := range e.diagnosticCasts {
					casts = append(casts, p6NativeAutoCallback{Round: cast["round"].(int32), Time: cast["time"].(float64), Skill: cast["skill"].(string), Cast: cast["cast"].(string)})
				}
				key := fmt.Sprintf("%d/%d", e.round, int64(math.Round(e.time*10)))
				tick, found := wanted[key]
				if !found {
					return
				}
				matched++
				if math.Abs(e.time-tick.Time) > 1e-8 || e.rng.Draws() != tick.RandomDraws {
					t.Fatalf("original completed tick %s: RNG native=%d Go=%d", key, tick.RandomDraws, e.rng.Draws())
				}
				for camp, player := range tick.Players {
					if math.Abs(player.Anger-e.players[camp].anger) > 1e-6 || len(player.Actors) != len(e.players[camp].actors) {
						t.Fatalf("original tick %s AP camp%d native=%g Go=%g", key, camp, player.Anger, e.players[camp].anger)
					}
					for i, actor := range player.Actors {
						actual := e.players[camp].actors[i]
						if actor.UID != actual.uid || math.Abs(actor.HP-actual.hp) > 1e-6 {
							t.Fatalf("original tick %s HP %s native=%g Go=%g", key, actor.UID, actor.HP, actual.hp)
						}
					}
				}
			})
			if err != nil || out.Authoritative {
				t.Fatal("original automatic AI calculation or global authority", err)
			}
			if matched != len(wanted) || out.RandomDraws != tc.RandomDraws || out.FinalRound != tc.FinalRound || (out.Winner == 0) != tc.AttackerWon || len(out.Operates.SkillOpts) != len(tc.Operates) {
				t.Fatal("original operations, sampled tick denominator or final result drift", matched, len(wanted), out.FinalRound, tc.FinalRound, out.RandomDraws, tc.RandomDraws)
			}
			for i, actual := range out.Operates.SkillOpts {
				want := tc.Operates[i]
				if actual.GetRoundNum() != want.GetRoundNum() || math.Abs(actual.GetRoundTime()-want.GetRoundTime()) > 1e-8 || actual.GetCamp() != want.GetCamp() || actual.GetActorUid() != want.GetActorUid() || actual.GetSkillIndex() != want.GetSkillIndex() {
					t.Fatal("original skill press drift", i, actual, want)
				}
			}
			var begins []p6NativeAutoCallback
			for _, event := range out.Events {
				if event.Kind == "skill_begin" {
					begins = append(begins, p6NativeAutoCallback{Round: event.Round, Time: event.Time, Skill: event.Skill})
				}
			}
			for _, pair := range []struct {
				name         string
				actual, want []p6NativeAutoCallback
			}{{"begin", begins, tc.Begins}, {"cast", casts, tc.Casts}} {
				if len(pair.actual) != len(pair.want) {
					t.Fatal("original callback count", pair.name, len(pair.actual), len(pair.want))
				}
				for i, actual := range pair.actual {
					want := pair.want[i]
					if actual.Round != want.Round || math.Abs(actual.Time-want.Time) > 1e-8 || actual.Skill != want.Skill || actual.Cast != want.Cast {
						t.Fatal("original callback drift", pair.name, i, actual, want)
					}
				}
			}
		})
	}
}
