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

// The narrow legacy party is determined by real P5 source shape and closed
// executed mechanisms. It never whitelists player IDs, GUIDs, seeds or exact panels.
func validateP6LegacyPartyScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input.Attacker == nil || input.Defender == nil {
		return reject()
	}
	var strong, weak *pb.BattleHero
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if player.Inst == nil || len(player.Inst.ExtBattleAdds) != 0 || len(player.Formation) != 2 {
			return reject()
		}
		main, partner := player.Formation[0], player.Formation[1]
		if main == nil || partner == nil || main.GetPose() != 3 || partner.GetPose() != 7 || main.GetGuid() == partner.GetGuid() {
			return reject()
		}
		switch main.GetId() {
		case "hero_001":
			if strong != nil || partner.GetId() != "hero_003" {
				return reject()
			}
			strong = main
		case "hero_047":
			if weak != nil || partner.GetId() != "hero_025" {
				return reject()
			}
			weak = main
		default:
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
	if strong == nil || weak == nil {
		return reject()
	}
	// Reuse only the proven flat-panel field validator in private identity views.
	// Original identities, talent, skills and poses remain untouched in calculation.
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
	// Current legacy mains differ substantially in real P5 growth. Keep that
	// bounded dominant structure; balanced/long-running parties need new evidence.
	if strong.Prop.GetAtt() < 3*weak.Prop.GetAtt() || strong.Prop.GetHp() < 4*weak.Prop.GetHp() {
		return reject()
	}
	return nil
}

// ComputeP6LegacyPartySettlement retains every actual original skill including
// ultimates. An unobserved execution is rejected after full calculation, before
// any consumer commits a result, resource change or immutable accepted receipt.
func (s *GameService) ComputeP6LegacyPartySettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6LegacyPartyScope(input); err != nil {
		return nil, err
	}
	if constUtil.P6LegacyPartyNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6LegacyPartyScope(frozen); err != nil {
		return nil, err
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6LegacyPartyTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6LegacyPartyNativeProof
	return result, nil
}

func validateP6LegacyPartyTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
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
	if len(seen) != len(sources) || result.Outcome.FinalRound < 1 || result.Outcome.FinalRound > constUtil.P6LegacyPartyMaxFinalRound || result.OriginalOperates.GetFinalRoundNum() != result.Outcome.FinalRound || result.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || result.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
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
			case "buff_119", "buff_270", "buff_488", "buff_mstone", "buff_mstone1":
			default:
				return reject()
			}
		}
		if event.Skill != "" {
			// Original PlayerAI can queue an ultimate that never begins before
			// terminal death. The original8 input/core/manager/RNG proof includes
			// that exact queue behavior; no ultimate effect is admitted here.
			if event.Kind == "skill_press" && event.Skill == "001_3" {
				continue
			}
			switch event.Skill {
			case "001_1", "001_2", "003_1", "003_2", "025_1", "025_2", "047_1", "047_2":
			default:
				return reject()
			}
		}
	}
	return nil
}

func p6KnownLegacyPartySettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 11 && input.EngineVersion == constUtil.P6LegacyPartySettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6LegacyPartyNativeProof
}
