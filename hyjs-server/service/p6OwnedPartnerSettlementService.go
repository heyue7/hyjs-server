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

// validateP6OwnedPartnerScope admits the actual owned main/partner layout. The
// two validation views only reuse already closed actor-field conditions; source
// poses, team composition and calculation input are never rewritten.
func validateP6OwnedPartnerScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input.Attacker == nil || input.Defender == nil {
		return reject()
	}
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if len(player.Formation) != 2 {
			return reject()
		}
		main, partner := player.Formation[0], player.Formation[1]
		if main == nil || partner == nil || main.GetId() != "hero_047" || main.GetPose() != 3 || partner.GetPose() != 7 || partner.GetBreachedLevel() != 1 || main.GetGuid() == partner.GetGuid() {
			return reject()
		}
		if partner.GetId() != "hero_069" && partner.GetId() != "hero_070" {
			return reject()
		}
	}
	primary := *input
	primary.Attacker = proto.Clone(input.Attacker).(*pb.BattlePlayer)
	primary.Defender = proto.Clone(input.Defender).(*pb.BattlePlayer)
	primary.Attacker.Formation = primary.Attacker.Formation[:1]
	primary.Defender.Formation = primary.Defender.Formation[:1]
	if err := validateP6Starter047Scope(&primary); err != nil {
		return err
	}
	secondary := *input
	secondary.Attacker = proto.Clone(input.Attacker).(*pb.BattlePlayer)
	secondary.Defender = proto.Clone(input.Defender).(*pb.BattlePlayer)
	secondary.Attacker.Formation = secondary.Attacker.Formation[1:]
	secondary.Defender.Formation = secondary.Defender.Formation[1:]
	secondary.Attacker.Formation[0].Pose = proto.Int32(1)
	secondary.Defender.Formation[0].Pose = proto.Int32(1)
	return validateP6LinearPhysicalScope(&secondary)
}

// ComputeP6OwnedPartnerSettlement retains all real skills and computes the full
// original team. Only original-observed executed mechanisms can close a result;
// an unobserved069/070 ultimate is rejected before a consumer pays or settles.
func (s *GameService) ComputeP6OwnedPartnerSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6OwnedPartnerScope(input); err != nil {
		return nil, err
	}
	if constUtil.P6OwnedPartnerNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6OwnedPartnerScope(frozen); err != nil {
		return nil, err
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6OwnedPartnerTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6OwnedPartnerNativeProof
	return result, nil
}

func validateP6OwnedPartnerTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if result == nil || result.Outcome == nil || result.OriginalOperates == nil || len(result.FinalActors) != 4 || result.OriginalRecordSHA256 != "" || len(result.OriginalOperates.SkillOpts) != 0 || result.OriginalOperates.AttackerTransOpt != nil || result.OriginalOperates.DefenderTransOpt != nil {
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
	if len(seen) != len(sources) || result.Outcome.FinalRound < 1 || result.Outcome.FinalRound > 41 || result.OriginalOperates.GetFinalRoundNum() != result.Outcome.FinalRound || result.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || result.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && result.Outcome.FinalRound == 41) {
		return reject()
	}
	if result.Outcome.Winner != winner || result.Outcome.Operates.GetAttackerTotalLeftHp() != current[0] || result.Outcome.Operates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	for _, event := range result.Outcome.Events {
		if event.Buff != "" && event.Buff != "buff_380" {
			return reject()
		}
		if event.Skill != "" {
			switch event.Skill {
			case "047_1", "047_2", "047_3", "069_1", "069_2", "070_1", "070_2":
			default:
				return reject()
			}
		}
	}
	return nil
}

func p6KnownOwnedPartnerSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 11 && input.EngineVersion == constUtil.P6OwnedPartnerSettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6OwnedPartnerNativeProof
}
