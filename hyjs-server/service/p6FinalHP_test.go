package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6FinalActorHPTerminalRuntimeMaxAndUnchangedBaseOutcome(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_071", "final-hp-player", "071_1,071_2,071_3", 3, 20000)
	d := p6Actor("hero_047", "final-hp-enemy", "047_1", 7, 20000)
	d.BloodNumber = proto.Int32(3)
	f := p6Input(t, s, a, d, 2)
	f.Attacker.Inst.Anger = proto.Float64(300)
	f.AppliedProperties = map[string][]string{"frame": {"explicit mechanism probe; exact original skill071_3 buff755 runtime maxHp source"}}
	before, _ := json.Marshal(f)
	audit, err := s.ComputeFrozenBattleFinalHP(t.Context(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := s.ComputeFrozenBattle(t.Context(), f, nil)
	if err != nil || !p6CopySeriesJSONEqual(t, base, audit.Outcome) || audit.Outcome.Authoritative || audit.OriginalOperates != nil || len(audit.FinalActors) != 2 {
		t.Fatal("opt-in terminal audit changed base calculation", err)
	}
	for _, actor := range audit.FinalActors {
		if actor.InitialActorSHA256 == "" || actor.CurrentHP != audit.Outcome.Attacker.GetFormation()[0].GetInst().GetCurHp() && actor.Camp == 0 || actor.CurrentHP != audit.Outcome.Defender.GetFormation()[0].GetInst().GetCurHp() && actor.Camp == 1 {
			t.Fatal("final actor identity/HP not bound to outcome", actor)
		}
		if actor.Camp == 1 && (actor.MaxHP != d.Prop.GetHp() || actor.MaxHP != 20000 || actor.BloodNumber != 3 || actor.Pose != 7) {
			t.Fatal("perhp HP damage or visual bars changed original runtime maxHP", actor)
		}
	}
	verified, err := s.ComputeFrozenBattleFinalHP(t.Context(), f, audit.Outcome.Operates)
	if err != nil || !p6CopySeriesJSONEqual(t, audit, verified) {
		t.Fatal("opt-in terminal full replay drift", err)
	}
	after, _ := json.Marshal(f)
	if !bytes.Equal(before, after) {
		t.Fatal("terminal audit mutated frozen frame")
	}
	f.AppliedProperties["frame"][0] += "/changed"
	changed, err := s.ComputeFrozenBattleFinalHP(t.Context(), f, nil)
	if err != nil || changed.CalculationSourceSHA256 == audit.CalculationSourceSHA256 || !p6CopySeriesJSONEqual(t, audit.Outcome, changed.Outcome) {
		t.Fatal("source digest omitted caller's actual frame provenance", err)
	}
}

func TestP6FinalActorHPAlreadyDeadAndCancellation(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "final-already-dead", "047_1", 1, 1000), p6Actor("hero_047", "final-live", "047_1", 2, 1000), 2)
	f.Attacker.Formation[0].Inst.CurHp = proto.Float64(0)
	audit, err := s.ComputeFrozenBattleFinalHP(t.Context(), f, nil)
	if err != nil || len(audit.FinalActors) != 2 || audit.FinalActors[0].State != 4 || audit.FinalActors[0].CurrentHP != 0 || audit.FinalActors[0].MaxHP != 1000 || audit.Outcome.Winner != 1 || audit.Outcome.FinalRound != 1 {
		t.Fatal("initial zeroHP original runtime DEAD actor omitted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if partial, computeErr := s.ComputeFrozenBattleFinalHP(ctx, f, nil); !errors.Is(computeErr, context.Canceled) || partial != nil {
		t.Fatal("cancelled calculation returned partial terminal audit", computeErr)
	}
}

func TestP6FinalActorHPActualOriginalUF14TerminalSnapshot(t *testing.T) {
	raw, wireRoot := p6NativeDiagnosticFixture(t, "native-local-uf14-golden-v82.json", "45af2777020516dcdc354447eb5a1a38a21115b22643762f043399aaffa55473", "", true)
	var original struct {
		Cases []struct {
			Wire  string `json:"wire"`
			SHA   string `json:"wireSHA256"`
			Ticks []struct {
				Players []struct {
					Actors []struct {
						UID       string  `json:"uid"`
						HP        float64 `json:"hp"`
						State     int32   `json:"actorState"`
						Attribute struct {
							MaxHP float64 `json:"maxHp"`
						} `json:"attribute"`
					} `json:"actors"`
				} `json:"players"`
			} `json:"ticks"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &original); err != nil || len(original.Cases) != 1 {
		t.Fatal("actual local UF14 terminal native source", err)
	}
	c := original.Cases[0]
	data, err := os.ReadFile(filepath.Join(wireRoot, c.Wire))
	if err != nil || p6Digest(data) != c.SHA {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(data, wire); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	frozen, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	audit, err := s.ComputeFrozenBattleFinalHP(t.Context(), frozen, nil)
	if err != nil {
		t.Fatal(err)
	}
	last := c.Ticks[len(c.Ticks)-1]
	index := 0
	for camp, player := range last.Players {
		for _, native := range player.Actors {
			if index >= len(audit.FinalActors) {
				t.Fatal("terminal actor denominator")
			}
			actor := audit.FinalActors[index]
			if actor.UID != native.UID || actor.Camp != int32(camp) || actor.State != native.State {
				t.Fatal("actual original terminal actor identity/state", actor)
			}
			closeCombat(t, actor.CurrentHP, native.HP)
			closeCombat(t, actor.MaxHP, native.Attribute.MaxHP)
			index++
		}
	}
	if index != len(audit.FinalActors) || audit.Outcome.Authoritative {
		t.Fatal("terminal actual native count/authority")
	}
}
