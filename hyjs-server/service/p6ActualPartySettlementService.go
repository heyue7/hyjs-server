package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Keep the complete actual P5 party shapes and all original ultimate skills.
// This candidate covers either party on either side, including balanced mirrors;
// it remains independently proof-gated pending full original ultimate evidence.
func validateP6ActualPartyScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input.Attacker == nil || input.Defender == nil {
		return reject()
	}
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if player.Inst == nil || len(player.Inst.ExtBattleAdds) != 0 || len(player.Formation) != 2 {
			return reject()
		}
		main, partner := player.Formation[0], player.Formation[1]
		if main == nil || partner == nil || main.GetPose() != 3 || partner.GetPose() != 7 || main.GetGuid() == partner.GetGuid() {
			return reject()
		}
		if !((main.GetId() == "hero_001" && partner.GetId() == "hero_003") || (main.GetId() == "hero_047" && partner.GetId() == "hero_025")) {
			return reject()
		}
		for _, hero := range player.Formation {
			prefix := strings.TrimPrefix(hero.GetId(), "hero_")
			if hero.GetSkillOrder() != prefix+"_1,"+prefix+"_2,"+prefix+"_3" {
				return reject()
			}
			if len(hero.TalentList) != 0 && !(hero.GetId() == "hero_047" && len(hero.TalentList) == 1 && hero.TalentList[0] == "tal_1") {
				return reject()
			}
		}
	}
	// Reuse the recovered flat-panel/chance/version/table/field validator only
	// in private views. Original actors, talents, poses and skills stay intact.
	for slot := 0; slot < 2; slot++ {
		view, err := cloneP6CalculationInput(input)
		if err != nil {
			return err
		}
		for _, player := range []*pb.BattlePlayer{view.Attacker, view.Defender} {
			hero := player.Formation[slot]
			hero.Id = proto.String("hero_047")
			hero.SkillOrder = proto.String("047_1,047_2,047_3")
			hero.Pose = proto.Int32(3)
			hero.TalentList = nil
			player.Formation = []*pb.BattleHero{hero}
		}
		if err = validateP6Starter047Scope(view); err != nil {
			return err
		}
	}
	return nil
}

// ComputeP6ActualPartySettlement keeps the original complete actor calculations.
// Empty proof denies every matching input; Friend short-family evidence cannot
// substitute for actual ultimate execution/timeout evidence of this wider scope.
func (s *GameService) ComputeP6ActualPartySettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6ActualPartyScope(input); err != nil {
		return nil, err
	}
	if constUtil.P6ActualPartyNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6ActualPartyTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6ActualPartyNativeProof
	return result, nil
}

func validateP6ActualPartyTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
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
	if len(seen) != len(sources) || result.Outcome.FinalRound < 1 || result.Outcome.FinalRound > constUtil.P6ActualPartyMaxFinalRound || result.OriginalOperates.GetFinalRoundNum() != result.Outcome.FinalRound || result.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || result.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
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

func p6KnownActualPartySettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 11 && input.EngineVersion == constUtil.P6LegacyPartySettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6ActualPartyNativeProof
}
