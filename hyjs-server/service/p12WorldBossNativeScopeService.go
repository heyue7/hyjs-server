package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// validateP12WorldBossPartyScope 只接受具名自动队伍族；高成长047和其他模式的证书均不扩大此范围。
func validateP12WorldBossPartyScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.BattleType != 18 || input.MaxRound != 40 || input.Attacker == nil || input.Attacker.Inst == nil || input.Monster == nil || input.Monster.Inst == nil || input.Defender != nil || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.DefenderFirst || input.IsAttToDef || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
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
	boosts, err := p12WorldBossPartyBoostMask(input)
	if err != nil {
		return err
	}
	talents := []string{}
	if boosts&4 != 0 {
		talents = append(talents, "tal_3165")
	}
	if boosts&8 != 0 {
		talents = append(talents, "tal_3163")
	}
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
		if boosts&1 != 0 {
			hit += 80
		}
		if boosts&2 != 0 {
			critical += 80
		}
		if p.GetAtt() < 1 || p.GetHp() < 1 || actor.Inst.GetCurHp() != p.GetHp() || p.GetPeratt() != 0 || p.GetPerhp() != 0 || p.GetPerpdef() != 0 || p.GetPermdef() != 0 || p.GetDamageAdd() != 0 || p.GetDamageReduce() != 0 || p.GetHit() != hit || p.GetDodge() != 5 || p.GetCri() != critical || p.GetAntiCri() != 0 || p.GetCriFactor() != 50 {
			return reject()
		}
	}
	// 原四部位Lv1完整字节包含天赋、技能空槽、bloodNumber和浮点面板，避免只核ID遗漏语义。
	if len(input.Monster.Formation) != 4 || input.Monster.Inst.GetAnger() != 0 || p6ScopedUnknown(input.Monster) || p6ScopedUnknown(input.Monster.Inst) {
		return reject()
	}
	expected := []string{
		"ccc6ad2bb5a8c9439d68d4fe26a6db66fcd2335ef4544f2f61473395dac4b4e8",
		"17bb70506264ce1786f1d40746fee2f81119ee6600e26d071b060808cf2e8c6f",
		"29925ea90c78dba9a0bf5045746be6fff5b059cd0fca1f584d0a6d94e6950cc0",
		"374477d816b1cb712b8006566d843a9be13cf9979e8bf2905336f8b31eee834d",
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

// p12WorldBossPartyBoostMask 校验生产购买账本的有序ID集合，不把未购买的数字面板视作增益资格。
func p12WorldBossPartyBoostMask(input *model.FrozenBattleInput) (int, error) {
	mask, previous := 0, ""
	for _, id := range input.AppliedProperties["worldBossPurchasedBoosts"] {
		if id <= previous {
			return 0, errors.New(constUtil.CombatUnsupported)
		}
		previous = id
		switch id {
		case "wordboss_1":
			mask |= 1
		case "wordboss_2":
			mask |= 2
		case "wordboss_3":
			mask |= 4
		case "wordboss_4":
			mask |= 8
		default:
			return 0, errors.New(constUtil.CombatUnsupported)
		}
	}
	return mask, nil
}

// computeP12WorldBossCertifiedFinalHP 将独立证明及原驱动固定在同一输入；调用者仍须保存事务回执。
func (s *GameService) computeP12WorldBossCertifiedFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	proof, err := p12WorldBossNativeProofForInput(input)
	if err != nil {
		return nil, err
	}
	if input.AppliedProperties == nil {
		input.AppliedProperties = map[string][]string{}
	}
	input.AppliedProperties[constUtil.P12WorldBossPartyNativeScope] = []string{proof}
	if proof == constUtil.P12WorldBossMechanismProof {
		return s.computeP12MechanismFinalHP(ctx, input, 18)
	}
	out, err := s.computeFrozenBattleFinalHP(ctx, input, nil, true)
	if err != nil {
		return nil, err
	}
	if err = validateP12WorldBossCertifiedTerminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	return out, nil
}

func p12WorldBossLegacyNativeProofForInput(input *model.FrozenBattleInput) (string, error) {
	if err := validateP12WorldBossPartyScope(input); err != nil {
		return "", err
	}
	proof := constUtil.P12WorldBossPartyNativeProof
	boosts, err := p12WorldBossPartyBoostMask(input)
	if err != nil {
		return "", err
	}
	if boosts != 0 {
		proof = constUtil.P12WorldBossPartyBoostNativeProof
	}
	if proof == "" {
		return "", errors.New(constUtil.CombatUnsupported)
	}
	return proof, nil
}

func validateP12WorldBossCertifiedTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P12WorldBossBattleInvalid) }
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
	if len(sources) != 6 || len(out.FinalActors) != len(sources) {
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

// validateP12WorldBossCertifiedReceipt 冷恢复继续使用冻结证明，拒绝无证明、跨族或来源被改写的结果。
func validateP12WorldBossCertifiedReceipt(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if out != nil && out.Outcome != nil && out.Outcome.CapabilityHash == constUtil.P12WorldBossMechanismProof {
		if input == nil {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		frozen := input.AppliedProperties[constUtil.P12WorldBossPartyNativeScope]
		if len(frozen) != 1 || frozen[0] != constUtil.P12WorldBossMechanismProof {
			return errors.New(constUtil.P12WorldBossBattleInvalid)
		}
		return p12MechanismTerminal(input, out, 18)
	}

	proof, err := p12WorldBossLegacyNativeProofForInput(input)
	if err != nil {
		return err
	}
	frozen := input.AppliedProperties[constUtil.P12WorldBossPartyNativeScope]
	if proof == "" || len(frozen) != 1 || frozen[0] != proof || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof {
		return errors.New(constUtil.P12WorldBossBattleInvalid)
	}
	return validateP12WorldBossCertifiedTerminal(input, out)
}

// New inputs retain the old native family when eligible; otherwise use the
// separately named local mechanism compatibility family for this exact mode.
func p12WorldBossNativeProofForInput(input *model.FrozenBattleInput) (string, error) {
	if proof, err := p12WorldBossLegacyNativeProofForInput(input); err == nil {
		return proof, nil
	}
	if err := p12MechanismScope(input, 18); err != nil {
		return "", err
	}
	return constUtil.P12WorldBossMechanismProof, nil
}
