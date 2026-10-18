package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This separate family preserves the historical linear certificate source.
// Every starter actor carries all three original skills; anger, ultimate,
// float and the original self damageAdd380 remain enabled in the calculation.
func validateP6Starter047Scope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 11 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || len(input.WinConditions) != 0 || input.IsAttToDef || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.MaxRound != 40 {
		return reject()
	}
	if input.EngineVersion != constUtil.P6LinearPhysicalEngine || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	for _, factor := range []*float64{input.AttackerDamageFactor, input.DefenderDamageFactor} {
		if factor != nil && *factor != 1 {
			return reject()
		}
	}
	a, b := input.Attacker.GetInst(), input.Defender.GetInst()
	if a == nil || b == nil || a.GetAnger() != 0 || b.GetAnger() != 0 {
		return reject()
	}
	if !(a.GetFirstOrder() == 1 && b.GetFirstOrder() == 0 || a.GetFirstOrder() == 0 && b.GetFirstOrder() == 1) || input.DefenderFirst != (a.GetFirstOrder() == 0) {
		return reject()
	}
	if input.Attacker.GetPlayerId() == input.Defender.GetPlayerId() && input.Attacker.GetServerId() == input.Defender.GetServerId() {
		return reject()
	}
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if player.GetPlayerId() <= 0 || player.GetServerId() <= 0 || len(player.Formation) != 1 || p6ScopedUnknown(player) || p6ScopedUnknown(player.Inst) {
			return reject()
		}
		h := player.Formation[0]
		if h == nil || h.Prop == nil || h.Inst == nil || len(h.TalentList) != 0 || h.GetAwakeStage() != 0 || h.GetAwakenSkill() != "" || h.GetWeaponId() != "" || h.GetPose() != 3 || h.GetBloodNumber() != 1 || h.GetSkillCastChance() != 100 || h.GetLevel() < 1 || h.GetLevel() > 100 || h.GetBreachedLevel() != 1 {
			return reject()
		}
		if h.GetGuid() == "" || len(h.GetGuid()) > 128 || strings.Contains(h.GetGuid(), "_") || p6ScopedUnknown(h) || p6ScopedUnknown(h.Prop) || p6ScopedUnknown(h.Inst) {
			return reject()
		}
		if h.GetId() != "hero_047" || h.GetSkillOrder() != "047_1,047_2,047_3" {
			return reject()
		}
		p := h.Prop
		// The first playable family uses the original unbreached full skill set.
		// Integer base panels eliminate initial maxHP percentage/stack projections.
		// Fixed chance fields select the recovered normal hit/critical/RNG branches;
		// positive/negative attack-defense differences both retain the original floor1.
		for _, value := range []float64{p.GetAtt(), p.GetHp(), p.GetPdef(), p.GetMdef()} {
			if !combatFinite(value) || value < 0 || value > constUtil.P6LinearPhysicalPanelLimit || math.Trunc(value) != value {
				return reject()
			}
		}
		if p.GetAtt() < 1 || p.GetHp() < 1 || h.Inst.GetCurHp() != p.GetHp() {
			return reject()
		}
		if p.GetPeratt() != 0 || p.GetPerhp() != 0 || p.GetPerpdef() != 0 || p.GetPermdef() != 0 || p.GetDamageAdd() != 0 || p.GetDamageReduce() != 0 || p.GetHit() != 95 || p.GetDodge() != 5 || p.GetCri() != 10 || p.GetAntiCri() != 0 || p.GetCriFactor() != 50 {
			return reject()
		}
	}
	// Verify actual frozen table bytes instead of accepting a caller's hash label.
	// The pinned original starter has its complete normal/ultimate group, anger,
	// float and self damageAdd380. No additional skill/Talent/Buff source is accepted.
	tables, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(tables) != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	return nil
}

// ComputeP6StarterSettlement is independently certificate-gated. An empty
// proof retains the pending family as unsupported, even for a matching source.
func (s *GameService) ComputeP6StarterSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6Starter047Scope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6StarterNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6Starter047Terminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6StarterNativeProof
	return result, nil
}

func validateP6Starter047Terminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if result == nil || result.Outcome == nil || result.OriginalOperates == nil || len(result.FinalActors) != 2 || result.OriginalRecordSHA256 != "" || len(result.OriginalOperates.SkillOpts) != 0 || result.OriginalOperates.AttackerTransOpt != nil || result.OriginalOperates.DefenderTransOpt != nil {
		return reject()
	}
	current := [2]float64{}
	seen := [2]bool{}
	for _, actor := range result.FinalActors {
		if actor.Camp < 0 || actor.Camp > 1 || seen[actor.Camp] {
			return reject()
		}
		seen[actor.Camp] = true
		source := input.Attacker.Formation[0]
		if actor.Camp == 1 {
			source = input.Defender.Formation[0]
		}
		if actor.GUID != source.GetGuid() || actor.HeroID != source.GetId() || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return reject()
		}
		rawActor, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(rawActor) != actor.InitialActorSHA256 {
			return reject()
		}
		current[actor.Camp] = actor.CurrentHP
	}
	if !seen[0] || !seen[1] || result.Outcome.FinalRound < 1 || result.Outcome.FinalRound > 41 || result.OriginalOperates.GetFinalRoundNum() != result.Outcome.FinalRound || result.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || result.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	terminalWinner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		terminalWinner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && result.Outcome.FinalRound == 41) {
		return reject()
	}
	if terminalWinner != result.Outcome.Winner {
		return reject()
	}
	for _, event := range result.Outcome.Events {
		if event.Buff != "" && event.Buff != "buff_380" {
			return reject()
		}
		if event.Skill != "" && event.Skill != "047_1" && event.Skill != "047_2" && event.Skill != "047_3" {
			return reject()
		}
		if event.Kind == "skill_press" && event.Value != 0 && event.Value != 1 && event.Value != 2 {
			return reject()
		}
	}
	return nil
}

func p6KnownStarterSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 11 && input.EngineVersion == constUtil.P6LinearPhysicalEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6StarterNativeProof
}
