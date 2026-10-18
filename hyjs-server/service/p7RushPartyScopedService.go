package service

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p7ManualRushFamily只分派计算与校验，现有pending/accepted事务和回执绑定由原消费者保存。
type p7ManualRushFamily struct {
	DriverSource string
	ScopeProof   string
	Party        bool
	Natural      bool
	Generic      bool
}

func p7PartyRushSourceAvailable(series *model.FrozenP6RushSeries) bool {
	if constUtil.P7RushPartyManualNativeProof == "" || series == nil {
		return false
	}
	view := *series
	view.PlayerAutomatic = false
	return validateP7PartyRushScope(&view) == nil
}

func validateP7PartyRushScope(series *model.FrozenP6RushSeries) error {
	if series == nil || series.Start == nil || series.Start.GetRushType() != constUtil.P7RushTrial {
		return errors.New(constUtil.CombatUnsupported)
	}
	return validateP7PartyRushDiagnosticScope(series)
}

// 新开局只能从实际typed源选择族；双演员绝不进入旧047证明。
func p7ManualRushFamilyForSeries(series *model.FrozenP6RushSeries) (p7ManualRushFamily, error) {
	proof, err := p7GenericMechanismRushProof(series)
	if err == nil {
		return p7ManualRushFamily{DriverSource: constUtil.P7GenericMechanismDriverSource, ScopeProof: proof, Generic: true}, nil
	}
	legacy, legacyErr := p7ManualRushLegacyFamilyForSeries(series)
	if legacyErr == nil {
		return legacy, nil
	}
	// Keep the exact unsupported original source visible when no legacy family applies.
	return p7ManualRushFamily{}, err
}

func p7ManualRushLegacyFamilyForSeries(series *model.FrozenP6RushSeries) (p7ManualRushFamily, error) {
	if p7NaturalRushIdentity(series) {
		proof, err := p7NaturalRushProof(series)
		if err != nil {
			return p7ManualRushFamily{}, err
		}
		return p7ManualRushFamily{DriverSource: constUtil.P7NaturalRushDriverSource, ScopeProof: proof, Party: true, Natural: true}, nil
	}
	if series != nil && series.Template != nil && series.Template.Attacker != nil && len(series.Template.Attacker.Formation) == 2 {
		if constUtil.P7RushPartyManualNativeProof == "" {
			return p7ManualRushFamily{}, errors.New(constUtil.CombatUnsupported)
		}
		family := p7ManualRushFamily{DriverSource: constUtil.P7RushPartyManualDriverSource, ScopeProof: constUtil.P7RushPartyManualNativeProof, Party: true}
		return family, family.ValidateScope(series)
	}
	if err := validateP6RushManualScope(series); err != nil {
		return p7ManualRushFamily{}, err
	}
	_, source, proof, _ := p7RushManualIdentity(series.Start.GetRushType())
	if proof == "" {
		return p7ManualRushFamily{}, errors.New(constUtil.CombatUnsupported)
	}
	return p7ManualRushFamily{DriverSource: source, ScopeProof: proof}, nil
}

// 冷读从冻结的driver/proof分派，不以当前阵容或开关升级历史pending。
func p7ManualRushFamilyForReceipt(kind int32, driverSource, proof string) (p7ManualRushFamily, error) {
	if (kind == constUtil.P7RushTrial || kind == constUtil.P7RushElite) && driverSource == constUtil.P7GenericMechanismDriverSource && proof != "" && proof == constUtil.P7GenericMechanismCompatibilityProof {
		return p7ManualRushFamily{DriverSource: driverSource, ScopeProof: proof, Generic: true}, nil
	}
	if kind == constUtil.P7RushTrial && driverSource == constUtil.P7NaturalRushDriverSource && proof != "" && proof == constUtil.P7NaturalRushNativeProof {
		return p7ManualRushFamily{DriverSource: driverSource, ScopeProof: proof, Party: true, Natural: true}, nil
	}
	if kind == constUtil.P7RushTrial && driverSource == constUtil.P7RushPartyManualDriverSource && proof != "" && proof == constUtil.P7RushPartyManualNativeProof {
		return p7ManualRushFamily{DriverSource: driverSource, ScopeProof: proof, Party: true}, nil
	}
	_, expectedSource, expectedProof, _ := p7RushManualIdentity(kind)
	if expectedProof != "" && driverSource == expectedSource && proof == expectedProof {
		return p7ManualRushFamily{DriverSource: driverSource, ScopeProof: proof}, nil
	}
	return p7ManualRushFamily{}, errors.New(constUtil.P7ScopedRushConflict)
}

func (family p7ManualRushFamily) ValidateScope(series *model.FrozenP6RushSeries) error {
	if family.Generic {
		if family.DriverSource != constUtil.P7GenericMechanismDriverSource || family.ScopeProof != constUtil.P7GenericMechanismCompatibilityProof {
			return errors.New(constUtil.CombatUnsupported)
		}
		_, err := p7GenericMechanismRushProof(series)
		return err
	}
	if family.Natural {
		if !family.Party || family.DriverSource != constUtil.P7NaturalRushDriverSource || family.ScopeProof == "" || family.ScopeProof != constUtil.P7NaturalRushNativeProof {
			return errors.New(constUtil.CombatUnsupported)
		}
		_, err := p7NaturalRushProof(series)
		return err
	}
	if family.Party {
		if family.DriverSource != constUtil.P7RushPartyManualDriverSource || family.ScopeProof == "" || family.ScopeProof != constUtil.P7RushPartyManualNativeProof {
			return errors.New(constUtil.CombatUnsupported)
		}
		return validateP7PartyRushScope(series)
	}
	if err := validateP6RushManualScope(series); err != nil {
		return err
	}
	_, source, proof, _ := p7RushManualIdentity(series.Start.GetRushType())
	if proof == "" || family.ScopeProof != proof || family.DriverSource != source {
		return errors.New(constUtil.P7ScopedRushConflict)
	}
	return nil
}

func (family p7ManualRushFamily) Preview(ctx context.Context, s *GameService, series *model.FrozenP6RushSeries) (*model.CombatFinalHPOutcome, error) {
	if err := family.ValidateScope(series); err != nil {
		return nil, err
	}
	if family.Generic {
		return p7GenericMechanismRushPreview(ctx, s, series)
	}
	if family.Natural {
		return p7NaturalRushPreview(ctx, s, series)
	}
	if !family.Party {
		automatic := *series
		automatic.PlayerAutomatic = true
		return s.ComputeP6RushStarterSettlement(ctx, &automatic)
	}
	// 预览只采用已取样的Auto0驱动；实际End仍独立依原控制时点重新计算。
	out, err := s.ComputeP7PartyRushDiagnostic(ctx, series, 0)
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = family.ScopeProof
	if err = family.ValidatePreview(series, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (family p7ManualRushFamily) ValidatePreview(series *model.FrozenP6RushSeries, preview *model.CombatFinalHPOutcome) error {
	if err := family.ValidateScope(series); err != nil {
		return err
	}
	if family.Generic {
		return p7GenericMechanismRushValidatePreview(series, preview)
	}
	if family.Natural {
		return p7NaturalRushValidatePreview(series, preview)
	}
	if preview == nil || preview.Outcome == nil {
		return errors.New(constUtil.P7ScopedRushConflict)
	}
	reported := int32(0)
	if preview.Outcome.Winner == 0 {
		reported = 1
	}
	if !family.Party {
		automatic := *series
		automatic.PlayerAutomatic = true
		return ValidateP6RushStarterEnd(&automatic, preview, preview.OriginalOperates, reported)
	}
	input, err := p7PartyRushInputAt(series, 0)
	if err != nil {
		return err
	}
	return validateP7PartyRushCertifiedTerminal(input, preview, family.ScopeProof)
}

func (family p7ManualRushFamily) ComputeEnd(ctx context.Context, s *GameService, series *model.FrozenP6RushSeries, supplied *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	if err := family.ValidateScope(series); err != nil {
		return nil, err
	}
	if family.Generic {
		return p7GenericMechanismRushComputeEnd(ctx, s, series, supplied, reported)
	}
	if family.Natural {
		return p7NaturalRushComputeEnd(ctx, s, series, supplied, reported)
	}
	if !family.Party {
		return s.ComputeP6RushManualAutoSettlement(ctx, series, supplied, reported)
	}
	input, err := p7PartyRushCalculationInput(series, supplied)
	if err != nil {
		return nil, err
	}
	// 原End从不作为回放输入，所有AI、操作、终末HP由冻结来源独立重算。
	out, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP7PartyRushDiagnosticTerminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = family.ScopeProof
	if err = family.ValidateEnd(series, out, supplied, reported); err != nil {
		return nil, err
	}
	return out, nil
}

func (family p7ManualRushFamily) ValidateEnd(series *model.FrozenP6RushSeries, terminal *model.CombatFinalHPOutcome, supplied *pb.BattleOperate, reported int32) error {
	if err := family.ValidateScope(series); err != nil {
		return err
	}
	if family.Generic {
		return p7GenericMechanismRushValidateEnd(series, terminal, supplied, reported)
	}
	if family.Natural {
		return p7NaturalRushValidateEnd(series, terminal, supplied, reported)
	}
	if !family.Party {
		return ValidateP6RushManualAutoEnd(series, terminal, supplied, reported)
	}
	input, err := p7PartyRushCalculationInput(series, supplied)
	if err != nil {
		return err
	}
	if err = validateP7PartyRushCertifiedTerminal(input, terminal, family.ScopeProof); err != nil {
		return err
	}
	expected := int32(0)
	if terminal.Outcome.Winner == 0 {
		expected = 1
	}
	if reported != expected || !p6RushStarterStrictRecordEqual(supplied, terminal.OriginalOperates) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

func p7PartyRushCalculationInput(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if err := validateP7PartyRushScope(series); err != nil {
		return nil, err
	}
	if supplied == nil || supplied.AttackerTransOpt != nil || supplied.DefenderTransOpt != nil || len(supplied.SkillOpts) == 0 || p6ScopedUnknown(supplied) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	first := supplied.SkillOpts[0]
	if first == nil || first.GetCamp() != 0 || first.GetRoundNum() != 1 || first.GetSkillIndex() < 0 || first.GetSkillIndex() > 2 || !combatFinite(first.GetRoundTime()) || first.GetRoundTime() < 0 || first.GetRoundTime() > 110 || p6ScopedUnknown(first) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	known := false
	for _, hero := range series.Template.Attacker.Formation {
		if first.GetActorUid() == hero.GetGuid() {
			known = true
		}
	}
	if !known {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return p7PartyRushInputAt(series, first.GetRoundTime())
}

func p7PartyRushInputAt(series *model.FrozenP6RushSeries, autoAt float64) (*model.FrozenBattleInput, error) {
	if err := validateP7PartyRushScope(series); err != nil {
		return nil, err
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		return nil, err
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(autoAt)
	if err = ValidateP6AIDriverPolicy(input); err != nil {
		return nil, err
	}
	return input, nil
}

func validateP7PartyRushCertifiedTerminal(input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, proof string) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if input == nil || proof == "" || proof != constUtil.P7RushPartyManualNativeProof || terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != proof || terminal.Schema != constUtil.P6FinalActorHPSchema || terminal.EngineVersion != input.EngineVersion || terminal.ClientVersion != input.ClientVersion || terminal.ConfigurationHash != input.ConfigurationHash || terminal.Outcome.Operates == nil {
		return reject()
	}
	if err := validateP7PartyRushDiagnosticTerminal(input, terminal); err != nil {
		return err
	}
	if terminal.Outcome.Operates.GetFinalRoundNum() != terminal.Outcome.FinalRound || terminal.Outcome.Operates.GetAttackerTotalLeftHp() != terminal.OriginalOperates.GetAttackerTotalLeftHp() || terminal.Outcome.Operates.GetDefenderTotalLeftHp() != terminal.OriginalOperates.GetDefenderTotalLeftHp() {
		return reject()
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != terminal.CalculationSourceSHA256 {
		return reject()
	}
	return nil
}
