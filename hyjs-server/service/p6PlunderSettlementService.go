package service

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original Plunder3 has its own callback/Max20 proof. A private validation view
// reuses only the recovered actor/field checks; actual type3 and Max20 never
// change during calculation, projection or capture.
func validateP6PlunderScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 3 || input.MaxRound != 20 || input.DefenderFirst || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 {
		return errors.New(constUtil.CombatUnsupported)
	}
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	view.MaxRound = 40
	if len(view.Attacker.GetFormation()) == 1 && len(view.Defender.GetFormation()) == 1 {
		return validateP6Starter047Scope(view)
	}
	return validateP6ActualPartyScope(view)
}

// ComputeP6PlunderSettlement requires an independently compared original
// callback, complete known execution and the original fixed Max20 terminal.
// Empty proof refuses every otherwise matching input and never borrows2/11.
func (s *GameService) ComputeP6PlunderSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6PlunderScope(frozen); err != nil {
		return s.ComputeP11GenericMechanismSettlement(ctx, frozen)
	}
	if constUtil.P6PlunderNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6PlunderTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6PlunderNativeProof
	return result, nil
}

func p6KnownPlunderSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	if input != nil && input.BattleType == 3 && p11KnownGenericMechanismProof(input, proof) {
		return true
	}
	return input != nil && input.BattleType == 3 && input.MaxRound == 20 && input.EngineVersion == constUtil.P6LegacyPartySettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6PlunderNativeProof
}

func validateP6PlunderTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	if result != nil && result.Outcome != nil && p11KnownGenericMechanismProof(input, result.Outcome.CapabilityHash) {
		return validateP11GenericMechanismTerminal(input, result)
	}
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if result == nil || result.Outcome == nil || result.OriginalOperates == nil || len(result.FinalActors) != len(input.Attacker.GetFormation())+len(input.Defender.GetFormation()) || result.OriginalRecordSHA256 != "" || len(result.OriginalOperates.SkillOpts) != 0 || result.OriginalOperates.AttackerTransOpt != nil || result.OriginalOperates.DefenderTransOpt != nil {
		return reject()
	}
	sources := map[string]*pb.BattleHero{}
	for camp, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		for _, hero := range player.Formation {
			sources[fmt.Sprintf("%d/%s", camp, hero.GetGuid())] = hero
		}
	}
	seen := map[string]bool{}
	current := [2]float64{}
	for _, actor := range result.FinalActors {
		key := fmt.Sprintf("%d/%s", actor.Camp, actor.GUID)
		source := sources[key]
		if source == nil || seen[key] || actor.Camp < 0 || actor.Camp > 1 {
			return reject()
		}
		seen[key] = true
		if actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.HeroID != source.GetId() || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.BloodNumber != 1 || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return reject()
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(raw) != actor.InitialActorSHA256 {
			return reject()
		}
		current[actor.Camp] += actor.CurrentHP
	}
	if len(seen) != len(sources) || result.Outcome.FinalRound < 1 || result.Outcome.FinalRound > input.MaxRound+1 || result.OriginalOperates.GetFinalRoundNum() != result.Outcome.FinalRound || result.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || result.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && result.Outcome.FinalRound == input.MaxRound+1) {
		return reject()
	}
	if result.Outcome.Winner != winner || result.Outcome.Operates.GetAttackerTotalLeftHp() != current[0] || result.Outcome.Operates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	for _, event := range result.Outcome.Events {
		if event.Buff != "" {
			switch event.Buff {
			case "buff_119", "buff_120", "buff_122", "buff_135", "buff_152", "buff_197", "buff_270", "buff_318", "buff_355", "buff_365", "buff_380", "buff_488", "buff_489", "buff_mstone", "buff_mstone1":
			default:
				return reject()
			}
		}
		if event.Skill != "" {

			switch event.Skill {
			case "001_1", "001_2", "001_3", "003_1", "003_2", "003_3", "025_1", "025_2", "025_3", "047_1", "047_2", "047_3":
			default:
				return reject()
			}
		}
	}
	return nil
}
