package service

import (
	"context"
	"encoding/json"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
)

// The phone pin is a negative drift registry, not an authority certificate.
func TestP6UFGuardPhoneInputScopeRejectsActorAndDriverDrift(t *testing.T) {
	root := "../../analysis/p6/guild-guard-inputs-20261025-native"
	raw, err := readTestAnalysis75(filepath.Join(root, "manifest.json"))
	if os.IsNotExist(err) {
		t.Skip("retained independent phone inputs required")
	}
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			ID   string `json:"case"`
			File string `json:"frozenInput"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &manifest) != nil || len(manifest.Cases) != 10 {
		t.Fatal("phone registry missing ten inputs")
	}
	for _, row := range manifest.Cases {
		t.Run(row.ID, func(t *testing.T) {
			raw, err := readTestAnalysis75(filepath.Join(root, row.File))
			if err != nil {
				t.Fatal(err)
			}
			var input model.FrozenBattleInput
			if json.Unmarshal(raw, &input) != nil {
				t.Fatal("input decode")
			}
			if err := validateP6UFGuardCandidate(&input); err != nil {
				t.Fatal("unchanged phone input", err)
			}

			// Build an explicitly local offer from the preserved producer, solely
			// to exercise source binding; it is never an authority certificate.
			rawRoot := "../../analysis/p6/guild-guard-inputs-20261025-raw"
			var player pb.InstUFPlayer
			var guard pb.InstUFGuard
			prefix := strings.TrimSuffix(row.File, ".frozen-input.json")
			playerRaw, e1 := readTestAnalysis75(filepath.Join(rawRoot, prefix+".player-frame.pb"))
			guardRaw, e2 := readTestAnalysis75(filepath.Join(rawRoot, prefix+".guard-frame.pb"))
			if e1 != nil || e2 != nil || proto.Unmarshal(playerRaw, &player) != nil || proto.Unmarshal(guardRaw, &guard) != nil {
				t.Fatal("retained source frames", e1, e2)
			}
			monsterRaw, _ := proto.Marshal(input.Monster)
			offer := &db.P13UnionFightGuardAuditOffer{GuardGUID: guard.GetGuid(), GuardID: guard.GetUFGuardId(), HeroID: input.Monster.Formation[0].GetId(), Camp: int64(guard.GetCamp()), BindGUID: guard.GetBindGuid(), InitialHP: input.Monster.Formation[0].Prop.GetHp(), ArriveRange: .8, Monster: monsterRaw, MonsterHash: p6Digest(monsterRaw), Source: "explicit original exported template API fixture"}
			offer.SourceHash = p13UFGuardOfferHash(offer)
			engine := &GameService{Tables: input.Tables}
			q, err := engine.ValidateP6UFGuardMechanisms(context.Background(), &input, input.Attacker.GetPlayerId(), &player, &guard, offer)
			if err != nil || q == nil || q.NativeCertified {
				t.Fatal("generic qualification must remain non-authoritative", err)
			}
			input.Attacker.Formation[0].Guid = proto.String("new-owner-hero")
			input.Attacker.Formation[0].Inst.CurHp = proto.Float64(1000)
			input.Attacker.Inst.Anger = proto.Float64(10)
			player.FightInfo.HpPos[0] = 1000
			player.FightInfo.Anger = proto.Float64(10)
			q, err = engine.ValidateP6UFGuardMechanisms(context.Background(), &input, input.Attacker.GetPlayerId(), &player, &guard, offer)
			if err != nil || q == nil || q.NativeCertified {
				t.Fatal("mechanism inventory must not require per-GUID/HP certificates", err)
			}
			if validateP6UFGuardCandidate(&input) == nil {
				t.Fatal("mechanism qualification silently broadened phone certificate")
			}
			if json.Unmarshal(raw, &input) != nil {
				t.Fatal("restore phone input")
			}
			mutations := map[string]func(*model.FrozenBattleInput){
				"skill-order": func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].SkillOrder = proto.String("injected-skill") },
				"attack":      func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Prop.Att = proto.Float64(99999) },
				"player-stat": func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Prop.Hp = proto.Float64(2055) },
				"guard-stat":  func(v *model.FrozenBattleInput) { v.Monster.Formation[0].Prop.Hp = proto.Float64(1001) },
				"player-guid": func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Guid = proto.String("new-guid") },
				"guard-guid":  func(v *model.FrozenBattleInput) { v.Monster.Formation[0].Guid = proto.String("guard/new-guid") },
				"initial-hp":  func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Inst.CurHp = proto.Float64(2000) },
				"initial-ap":  func(v *model.FrozenBattleInput) { v.Monster.Inst.Anger = proto.Float64(1) },
				"seed":        func(v *model.FrozenBattleInput) { v.Seed++ },
				"factor":      func(v *model.FrozenBattleInput) { v.AttackerDamageFactor = proto.Float64(2) },
				"initiative":  func(v *model.FrozenBattleInput) { v.DefenderFirst = true },
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					var changed model.FrozenBattleInput
					if json.Unmarshal(raw, &changed) != nil {
						t.Fatal("clone")
					}
					mutate(&changed)
					if validateP6UFGuardCandidate(&changed) == nil {
						t.Fatal("unproved input drift accepted")
					}
				})
			}
		})
	}
}
