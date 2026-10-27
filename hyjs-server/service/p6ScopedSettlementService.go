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

// validateP6LinearPhysicalScope checks a closed reachable mechanism family.
// The private check grants no authority by itself. The opt-in calculation also
// checks its pinned native certificate and completed terminal invariants.
// GUID, seed and panel values are not exact native-case whitelist identities.
func validateP6LinearPhysicalScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if (input.BattleType != 2 && input.BattleType != 11) || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || len(input.WinConditions) != 0 || input.IsAttToDef || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.MaxRound != 40 {
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
		if h == nil || h.Prop == nil || h.Inst == nil || len(h.TalentList) != 0 || h.GetAwakeStage() != 0 || h.GetAwakenSkill() != "" || h.GetWeaponId() != "" || h.GetPose() != 1 || h.GetBloodNumber() != 1 || h.GetSkillCastChance() != 100 || h.GetLevel() < 1 || h.GetLevel() > 100 || h.GetBreachedLevel() < 0 || h.GetBreachedLevel() > 25 {
			return reject()
		}
		if h.GetGuid() == "" || len(h.GetGuid()) > 128 || strings.Contains(h.GetGuid(), "_") || p6ScopedUnknown(h) || p6ScopedUnknown(h.Prop) || p6ScopedUnknown(h.Inst) {
			return reject()
		}
		prefix := ""
		switch h.GetId() {
		case "hero_069":
			prefix = "069"
		case "hero_070":
			prefix = "070"
		default:
			return reject()
		}
		if h.GetSkillOrder() != prefix+"_1,"+prefix+"_2,"+prefix+"_3" {
			return reject()
		}
		p := h.Prop
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
	// The pinned original tables contain no normal anger, Buff, state or projectile
	// source for this family. AP remains0 and the existing ultimate is unreachable.
	tables, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(tables) != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	return nil
}

func p6ScopedUnknown(message proto.Message) bool {
	return len(message.ProtoReflect().GetUnknown()) != 0
}

// ComputeP6ScopedSettlement provides an opt-in settlement result for a pinned,
// completely closed mechanism family. Unsupported sources return no result.
// The generic compute/audit APIs and the global capability flags stay unchanged.
// Only the pinned compile-time original composition proof enables this entry.
// No environment or caller-provided flag can grant authority.
func (s *GameService) ComputeP6ScopedSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6LinearPhysicalScope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6LinearPhysicalNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6LinearPhysicalTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6LinearPhysicalNativeProof
	return result, nil
}

// Recheck the closed-family invariants on the completed calculation before
// giving consumers a settlement result. Future implementation drift fails closed.
func validateP6LinearPhysicalTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
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
		if strings.HasPrefix(event.Kind, "buff_") || event.Kind == "anger" || event.Kind == "skill_press" && event.Value != 0 && event.Value != 1 {
			return reject()
		}
	}
	return nil
}

// Completed receipts retain the exact accepted old proof without recomputing
// under new tables/engine. Extend this explicit historical list when rotating a
// certificate; an arbitrary proof string cannot promote persisted outcomes.
func p6KnownScopedSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	if input == nil || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables {
		return false
	}
	if input.EngineVersion == constUtil.P6LinearPhysicalEngine && proof != "" && (proof == constUtil.P6LinearPhysicalNativeProof || proof == constUtil.P6LinearPhysicalParentProof) {
		return input.BattleType == 2 || input.BattleType == 11
	}
	if input.EngineVersion != constUtil.P6LinearPhysicalOriginalEngine {
		return false
	}
	if proof == constUtil.P6LinearPhysicalPreviousProof {
		return input.BattleType == 2 || input.BattleType == 11
	}
	return proof == constUtil.P6LinearPhysicalOriginalProof && input.BattleType == 2
}
