package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strings"
)

// This independent original type16 family retains complete actual source actors.
func validateP7Awake16Scope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.BattleType != 16 || input.MaxRound != 40 || input.Attacker == nil || input.Attacker.Inst == nil || input.Monster == nil || input.Monster.Inst == nil || input.Defender != nil || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.DefenderFirst || input.IsAttToDef || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	if input.EngineVersion != constUtil.P6LinearPhysicalEngine || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables || input.Seed < 0 || input.Seed > math.MaxInt32 {
		return reject()
	}
	for _, factor := range []*float64{input.AttackerDamageFactor, input.DefenderDamageFactor} {
		if factor == nil || *factor != 1 {
			return reject()
		}
	}
	party := input.Attacker
	if party.GetPlayerId() <= 0 || party.GetServerId() <= 0 || len(party.Formation) != 2 || party.Inst.GetAnger() != 0 || party.Inst.GetFirstOrder() != 0 || len(party.Inst.ExtBattleAdds) != 0 || p6ScopedUnknown(party) || p6ScopedUnknown(party.Inst) {
		return reject()
	}
	seen := map[string]bool{}
	talents := []string{}
	for index, actor := range party.Formation {
		id := []string{"001", "003"}[index]
		pose := []int32{3, 7}[index]
		if actor == nil || actor.Prop == nil || actor.Inst == nil || actor.GetId() != "hero_"+id || actor.GetSkillOrder() != id+"_1,"+id+"_2,"+id+"_3" || actor.GetPose() != pose || actor.GetBreachedLevel() != 1 || actor.GetBloodNumber() != 1 || actor.GetSkillCastChance() != 100 || actor.GetLevel() < 1 || actor.GetLevel() > 100 || actor.GetAwakeStage() != 0 || actor.GetAwakenSkill() != "" || actor.GetWeaponId() != "" || len(actor.TalentList) != len(talents) || p6ScopedUnknown(actor) || p6ScopedUnknown(actor.Prop) || p6ScopedUnknown(actor.Inst) {
			return reject()
		}
		for index, talent := range talents {
			if actor.TalentList[index] != talent {
				return reject()
			}
		}
		if actor.GetGuid() == "" || len(actor.GetGuid()) > 128 || strings.Contains(actor.GetGuid(), "_") || seen[actor.GetGuid()] {
			return reject()
		}
		seen[actor.GetGuid()] = true
		p := actor.Prop
		for _, value := range []float64{p.GetAtt(), p.GetHp(), p.GetPdef(), p.GetMdef()} {
			if !combatFinite(value) || value < 0 || value > constUtil.P6LinearPhysicalPanelLimit || math.Trunc(value) != value {
				return reject()
			}
		}
		hit, critical := 95.0, 10.0
		if p.GetAtt() < 1 || p.GetHp() < 1 || actor.Inst.GetCurHp() != p.GetHp() || p.GetPeratt() != 0 || p.GetPerhp() != 0 || p.GetPerpdef() != 0 || p.GetPermdef() != 0 || p.GetDamageAdd() != 0 || p.GetDamageReduce() != 0 || p.GetHit() != hit || p.GetDodge() != 5 || p.GetCri() != critical || p.GetAntiCri() != 0 || p.GetCriFactor() != 50 {
			return reject()
		}
	}
	// Both original NPC actors retain exact original GUID, pose, skill, talent and HP bytes.
	if len(input.Monster.Formation) != 2 || input.Monster.Inst.GetAnger() != 0 || p6ScopedUnknown(input.Monster) || p6ScopedUnknown(input.Monster.Inst) {
		return reject()
	}
	expected := []string{
		"32c52d97853abd98c6870595290697a18a34a3045801f3ff1da2f4745b07c5d4",
		"4006c1503719e5c45a503ee66154802b375d580f83655b869a705c940a6d7dae",
	}
	for index, actor := range input.Monster.Formation {
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
		if err != nil || p6Digest(raw) != expected[index] {
			return reject()
		}
	}
	raw, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(raw) != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	return nil
}

func validateP7Awake16Terminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if input == nil || out == nil || out.Outcome == nil || out.OriginalOperates == nil || out.Outcome.Operates == nil || out.OriginalRecordSHA256 != "" || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalOperates.AttackerTransOpt != nil || out.OriginalOperates.DefenderTransOpt != nil {
		return reject()
	}
	sources := map[string]*pb.BattleHero{}
	for camp, formation := range [][]*pb.BattleHero{input.Attacker.GetFormation(), input.Monster.GetFormation()} {
		for _, actor := range formation {
			if actor == nil || actor.Prop == nil || actor.Inst == nil {
				return reject()
			}
			sources[fmt.Sprintf("%d/%s", camp, actor.GetGuid())] = actor
		}
	}
	if len(sources) != 4 || len(out.FinalActors) != len(sources) {
		return reject()
	}
	seen := map[string]bool{}
	current := [2]float64{}
	for _, actor := range out.FinalActors {
		key := fmt.Sprintf("%d/%s", actor.Camp, actor.GUID)
		source := sources[key]
		if source == nil || seen[key] || actor.Camp < 0 || actor.Camp > 1 || actor.UID != fmt.Sprintf("%d_%s", actor.Camp, source.GetGuid()) || actor.HeroID != source.GetId() || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.BloodNumber != source.GetBloodNumber() || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return reject()
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(raw) != actor.InitialActorSHA256 {
			return reject()
		}
		seen[key] = true
		current[actor.Camp] += actor.CurrentHP
	}
	if out.Outcome.FinalRound < 1 || out.Outcome.FinalRound > input.MaxRound+1 || out.OriginalOperates.GetFinalRoundNum() != out.Outcome.FinalRound || out.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || out.OriginalOperates.GetDefenderTotalLeftHp() != current[1] || out.Outcome.Operates.GetAttackerTotalLeftHp() != current[0] || out.Outcome.Operates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && out.Outcome.FinalRound == input.MaxRound+1) {
		return reject()
	}
	if out.Outcome.Winner != winner {
		return reject()
	}
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return reject()
	}
	return nil
}

// P7Awake16ProofForInput admits the validated original automatic family only.
func P7Awake16ProofForInput(input *model.FrozenBattleInput) (string, error) {
	if err := validateP7Awake16Scope(input); err != nil {
		return "", err
	}
	if constUtil.P7Awake16NativeProof == "" {
		return "", errors.New(constUtil.CombatUnsupported)
	}
	return constUtil.P7Awake16NativeProof, nil
}

// ComputeP7Awake16ProtocolFinalHP does not mutate the caller's frozen input.
func (s *GameService) ComputeP7Awake16ProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	proof, err := P7Awake16ProofForInput(input)
	if err != nil {
		return nil, err
	}
	out, err := s.computeFrozenBattleFinalHP(ctx, input, nil, true)
	if err != nil {
		return nil, err
	}
	if err = validateP7Awake16Terminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	return out, nil
}

// ValidateP7Awake16CertifiedReceipt binds cold receipts to the same source and proof.
func ValidateP7Awake16CertifiedReceipt(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	proof, err := P7Awake16ProofForInput(input)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return validateP7Awake16Terminal(input, out)
}
