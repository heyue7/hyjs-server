package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Intrinsic source contract only: no HP normalization, wrapper conversion,
// sample GUID/seed/panel whitelist, or borrowed native certificate.
func validateP6UFPartyPlayerScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 14 || input.Seed <= 0 || input.MaxRound != 40 || input.DefenderFirst || input.IsAttToDef || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables || input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 {
		return reject()
	}
	raw, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(raw) != input.ConfigurationHash {
		return reject()
	}
	if input.Attacker.GetPlayerId() == input.Defender.GetPlayerId() {
		return reject()
	}
	for camp, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if player.GetPlayerId() <= 0 || player.Inst == nil || player.Inst.GetFirstOrder() != int32(1-camp) || len(player.Inst.ExtBattleAdds) != 0 || len(player.Formation) < 1 || len(player.Formation) > 4 || p6ScopedUnknown(player) || p6ScopedUnknown(player.Inst) || !combatFinite(player.Inst.GetAnger()) || player.Inst.GetAnger() < 0 || player.Inst.GetAnger() > 300 {
			return reject()
		}
		guids, poses := map[string]bool{}, map[int32]bool{}
		alive := false
		for _, hero := range player.Formation {
			if hero == nil || hero.Prop == nil || hero.Inst == nil || hero.GetBloodNumber() != 1 || hero.GetGuid() == "" || len(hero.GetGuid()) > 128 || guids[hero.GetGuid()] || poses[hero.GetPose()] || p6ScopedUnknown(hero) || p6ScopedUnknown(hero.Prop) || p6ScopedUnknown(hero.Inst) || !combatFinite(hero.Prop.GetHp()) || hero.Prop.GetHp() <= 0 || !combatFinite(hero.Inst.GetCurHp()) || hero.Inst.GetCurHp() < 0 || hero.Inst.GetCurHp() > hero.Prop.GetHp() {
				return reject()
			}
			guids[hero.GetGuid()] = true
			poses[hero.GetPose()] = true
			alive = alive || hero.Inst.GetCurHp() > 0
			var source map[string]string
			for _, row := range input.Tables["Hero"] {
				if row["id"] == hero.GetId() {
					source = row
					break
				}
			}
			if source == nil || !strings.HasPrefix(hero.GetId(), "hero_") || source["heroClass"] == "99" {
				return reject()
			}
			normal := strings.Join([]string{source["SkillId1"], source["SkillId2"], source["UltSkillId"]}, ",")
			swapped := strings.Join([]string{source["SkillId2"], source["SkillId1"], source["UltSkillId"]}, ",")
			if hero.GetSkillOrder() != normal && hero.GetSkillOrder() != swapped {
				return reject()
			}
		}
		if !alive {
			return reject()
		}
	}
	if err := validateFrozenP6(input); err != nil {
		return err
	}
	return ValidateP6FirstOrder(input)
}

// Qualification compiles the complete original actor/skill/talent/factory
// closure without computing a fight. It is explicitly non-native evidence.
func (s *GameService) ValidateP6UFPartyPlayerMechanisms(ctx context.Context, input *model.FrozenBattleInput) (*model.P6MechanismQualification, error) {
	if err := validateP6UFPartyPlayerScope(input); err != nil {
		return nil, err
	}
	qualification, err := s.ValidateP6GenericMechanisms(ctx, input)
	if err != nil {
		return nil, err
	}
	qualification.NativeCertified = false
	return qualification, nil
}

func (s *GameService) ComputeP6UFPartyPlayerSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if _, err = s.ValidateP6UFPartyPlayerMechanisms(ctx, frozen); err != nil {
		return nil, err
	}
	if constUtil.P6UFPartyPlayerNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.ComputeOriginalP6UFPlayerProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFPartyPlayerTerminal(frozen, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6UFPartyPlayerNativeProof
	return out, nil
}

func p6KnownUFPartyPlayerSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return proof != "" && constUtil.P6UFPartyPlayerNativeProof != "" && proof == constUtil.P6UFPartyPlayerNativeProof && validateP6UFPartyPlayerScope(input) == nil
}

func validateP6UFPartyPlayerTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if err := validateP6UFPartyPlayerScope(input); err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalRecordSHA256 != "" || out.Outcome.Attacker == nil || out.Outcome.Defender == nil || out.Outcome.Monster != nil {
		return reject()
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return reject()
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	parties := []*pb.BattlePlayer{proto.Clone(input.Attacker).(*pb.BattlePlayer), proto.Clone(input.Defender).(*pb.BattlePlayer)}
	for camp, final := range []*pb.BattlePlayer{out.Outcome.Attacker, out.Outcome.Defender} {
		if final.Inst == nil || !combatFinite(final.Inst.GetAnger()) || final.Inst.GetAnger() < 0 || final.Inst.GetAnger() > 300 {
			return reject()
		}
		parties[camp].Inst.Anger = proto.Float64(final.Inst.GetAnger())
	}
	for _, actor := range out.FinalActors {
		if actor.Camp < 0 || actor.Camp > 1 {
			return reject()
		}
		found := false
		for _, hero := range parties[actor.Camp].Formation {
			if hero.GetGuid() == actor.GUID {
				if hero.GetPose() != actor.Pose || actor.BloodNumber != hero.GetBloodNumber() || actor.MaxHP != hero.Prop.GetHp() {
					return reject()
				}
				hero.Inst.CurHp = proto.Float64(actor.CurrentHP)
				found = true
				break
			}
		}
		if !found {
			return reject()
		}
	}
	if !proto.Equal(parties[0], out.Outcome.Attacker) || !proto.Equal(parties[1], out.Outcome.Defender) {
		return reject()
	}
	return nil
}
