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
)

// This named compatibility scope removes demo actor IDs, retaining the original
// single-wave typed9/10 driver and strict independently recomputed End. Global
// native coverage and the original math kernel stay unchanged.
func p7GenericMechanismRushProof(series *model.FrozenP6RushSeries) (string, error) {
	if err := p7GenericMechanismRushShape(series); err != nil {
		return "", err
	}
	input := series.Template
	// Exact canonical table digest is checked again by generic admission; actor
	// instances remain the original owner/NPC snapshots bound by the P7 caller.
	validator := &GameService{Tables: input.Tables}
	if _, err := validator.ValidateP6GenericMechanisms(context.Background(), input); err != nil {
		return "", fmt.Errorf("Trial/Elite original mechanism: %w", err)
	}
	return constUtil.P7GenericMechanismCompatibilityProof, nil
}

func p7GenericMechanismRushShape(series *model.FrozenP6RushSeries) error {
	if err := validateP6RushSeries(series); err != nil {
		return err
	}
	input := series.Template
	kind := series.Start.GetRushType()
	if series.PlayerAutomatic || len(series.Start.BattleMonsterList) != 1 ||
		(kind != constUtil.P7RushTrial && kind != constUtil.P7RushElite) ||
		(series.Trial != (kind == constUtil.P7RushTrial)) ||
		input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables ||
		input.Seed < 0 || input.Seed > (1<<48)-1 || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.PVECompatibilityRule != "" || input.KFReplayViewer != nil ||
		p6ScopedUnknown(series.Start) || p6ScopedUnknown(input.Attacker) || p6ScopedUnknown(input.Attacker.Inst) || len(input.Attacker.Inst.ExtBattleAdds) != 0 || len(constUtil.P7GenericMechanismCompatibilityProof) != 64 {
		return errors.New(constUtil.CombatUnsupported)
	}
	return nil
}

// p7GenericMechanismRushAdmission 仅绑定当前调用栈内已完整证明的冻结series。
// seal覆盖全部表/演员/Start；使用前重验，禁止复用到修改后的series或其他指针。
type p7GenericMechanismRushAdmission struct {
	series *model.FrozenP6RushSeries
	seal   string
}

func newP7GenericMechanismRushAdmission(series *model.FrozenP6RushSeries) (*p7GenericMechanismRushAdmission, error) {
	if _, err := p7GenericMechanismRushProof(series); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(series)
	if err != nil {
		return nil, err
	}
	return &p7GenericMechanismRushAdmission{series: series, seal: p6Digest(raw)}, nil
}

func (admission *p7GenericMechanismRushAdmission) validate(series *model.FrozenP6RushSeries) error {
	if admission == nil || series == nil || admission.series != series || admission.seal == "" {
		return errors.New(constUtil.P6ReplayConflict)
	}
	raw, err := json.Marshal(series)
	if err != nil || p6Digest(raw) != admission.seal {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

func p7GenericMechanismRushSourceAvailable(series *model.FrozenP6RushSeries) bool {
	if series == nil {
		return false
	}
	view := *series
	view.PlayerAutomatic = false
	_, err := p7GenericMechanismRushProof(&view)
	return err == nil
}

func p7GenericMechanismRushInputAt(series *model.FrozenP6RushSeries, at float64) (*model.FrozenBattleInput, error) {
	if _, err := p7GenericMechanismRushProof(series); err != nil {
		return nil, err
	}
	return p7GenericMechanismRushInputAtAdmitted(series, at)
}

func p7GenericMechanismRushInputAtAdmitted(series *model.FrozenP6RushSeries, at float64) (*model.FrozenBattleInput, error) {
	if !combatFinite(at) || at < 0 || at > constUtil.P7NaturalRushAutoRoundTimeLimit {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		return nil, err
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(at)
	if err = ValidateP6AIDriverPolicy(input); err != nil {
		return nil, err
	}
	return input, nil
}

// Actual successful player commands drive only camp0. NPC AI and all damage,
// timing, cooldown, anger, result and terminal HP are recomputed by the original driver.
func p7GenericMechanismRushCalculationInput(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if _, err := p7GenericMechanismRushProof(series); err != nil {
		return nil, err
	}
	return p7GenericMechanismRushCalculationInputAdmitted(series, supplied)
}

func p7GenericMechanismRushCalculationInputAdmitted(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if err := p7GenericMechanismRushValidateControls(series, supplied); err != nil {
		return nil, err
	}
	return cloneP6CalculationInput(series.Template)
}

// p7GenericMechanismRushValidateControls 仅读取已admitted的实例和原完整控制记录。
// 保留旧clone在控制检查前的metadata有限数值门禁；只有实际选中的driver才深拷贝输入。
func p7GenericMechanismRushValidateControls(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) error {
	if series == nil || series.Template == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	input := series.Template
	metadata := *input
	metadata.Tables = nil
	if _, err := json.Marshal(metadata); err != nil {
		return err
	}
	if supplied == nil || supplied.DefenderTransOpt != nil || len(supplied.SkillOpts) == 0 || len(supplied.SkillOpts) > 16000 || p6ScopedUnknown(supplied) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if change := supplied.AttackerTransOpt; change != nil {
		if p6ScopedUnknown(change) || change.GetRoundNum() != 1 || len(change.ActorUids) != len(input.Attacker.Formation) || len(change.Fpids) != len(change.ActorUids) {
			return errors.New(constUtil.P6ReplayConflict)
		}
		seenActors, seenPositions := map[string]bool{}, map[int32]bool{}
		for index, uid := range change.ActorUids {
			found := false
			for _, actor := range input.Attacker.Formation {
				found = found || actor.GetGuid() == uid
			}
			if !found || seenActors[uid] || change.Fpids[index] < 1 || change.Fpids[index] > 15 || seenPositions[change.Fpids[index]] {
				return errors.New(constUtil.P6ReplayConflict)
			}
			seenActors[uid], seenPositions[change.Fpids[index]] = true, true
		}
	}
	for _, op := range supplied.SkillOpts {
		if op == nil || p6ScopedUnknown(op) || op.GetCamp() < 0 || op.GetCamp() > 1 || op.GetRoundNum() < 1 || op.GetRoundNum() > input.MaxRound || !combatFinite(op.GetRoundTime()) || op.GetRoundTime() < 0 || op.GetRoundTime() > 120 || op.GetSkillIndex() < 0 || op.GetSkillIndex() > 3 {
			return errors.New(constUtil.P6ReplayConflict)
		}
		formation := input.Attacker.Formation
		if op.GetCamp() == 1 {
			formation = input.Monster.Formation
		}
		found := false
		for _, actor := range formation {
			found = found || actor.GetGuid() == op.GetActorUid()
		}
		if !found {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	return nil
}

func p7GenericMechanismRushPreview(ctx context.Context, s *GameService, series *model.FrozenP6RushSeries) (*model.CombatFinalHPOutcome, error) {
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		return nil, err
	}
	out, _, err := s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
	if err = p7GenericMechanismRushValidatePreview(series, out); err != nil {
		return nil, err
	}
	return out, nil
}

func p7GenericMechanismRushValidateTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if input == nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P7GenericMechanismCompatibilityProof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalRecordSHA256 != "" {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err := validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

func p7GenericMechanismRushValidatePreview(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome) error {
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		return err
	}
	return p7GenericMechanismRushValidateTerminal(input, out)
}

func p7GenericMechanismRushComputeEnd(ctx context.Context, _ *GameService, series *model.FrozenP6RushSeries, supplied *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	input, err := p7GenericMechanismRushCalculationInput(series, supplied)
	if err != nil {
		return nil, err
	}
	engine := &GameService{Tables: input.Tables}
	// Actual successful player commands are always tried first. NPC decisions,
	// legal timing/cooldown/anger and all damage remain independently recomputed.
	out, _, manualErr := engine.ComputeP6GenericMechanismBattle(ctx, input, supplied)
	if manualErr == nil {
		out.Outcome.Authoritative = true
		out.Outcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
		if err = p7GenericMechanismRushValidateEnd(series, out, supplied, reported); err == nil {
			return out, nil
		}
		manualErr = err
	}
	// 原轮首布阵与LiveAuto技能驱动独立执行；完整记录只用于最终精确核对。
	if supplied.AttackerTransOpt != nil {
		out, err = engine.ComputeP6RoundOneLiveAutoFormation(ctx, input, supplied, supplied.AttackerTransOpt)
		if err != nil {
			return nil, err
		}
		out.Outcome.Authoritative = true
		out.Outcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
		if err = p7GenericMechanismRushValidateEnd(series, out, supplied, reported); err != nil {
			return nil, err
		}
		return out, nil
	}
	// Original live Auto can emit simultaneous presses which original PlaybackAI
	// intentionally consumes one per Update. Try only its single recorded control
	// candidate; no clock search, rounding or partial-field acceptance is allowed.
	autoInput, err := p7GenericMechanismRushAutoCalculationInput(series, supplied)
	if err != nil {
		if result, fallbackErr := p7RoundLiveAutoRushComputeEnd(ctx, engine, series, supplied, reported); fallbackErr == nil {
			return result, nil
		}
		if result, fallbackErr := p7LiveRecordedRushComputeEnd(ctx, engine, series, supplied, reported); fallbackErr == nil {
			return result, nil
		}
		return nil, manualErr
	}
	out, _, err = engine.ComputeP6GenericMechanismBattle(ctx, autoInput, nil)
	if err != nil || !proto.Equal(supplied, out.OriginalOperates) {
		if result, fallbackErr := p7RoundLiveAutoRushComputeEnd(ctx, engine, series, supplied, reported); fallbackErr == nil {
			return result, nil
		}
		if result, fallbackErr := p7LiveRecordedRushComputeEnd(ctx, engine, series, supplied, reported); fallbackErr == nil {
			return result, nil
		}
		return nil, manualErr
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p7GenericMechanismRushManualSourceDigest(autoInput, supplied)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
	if err = p7GenericMechanismRushValidateEnd(series, out, supplied, reported); err != nil {
		return nil, err
	}
	return out, nil
}

func p7GenericMechanismRushAutoCalculationInput(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if _, err := p7GenericMechanismRushProof(series); err != nil {
		return nil, err
	}
	return p7GenericMechanismRushAutoCalculationInputAdmitted(series, supplied)
}

func p7GenericMechanismRushAutoCalculationInputAdmitted(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if err := p7GenericMechanismRushValidateControls(series, supplied); err != nil {
		return nil, err
	}
	if supplied.AttackerTransOpt != nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	first := supplied.SkillOpts[0]
	if first.GetCamp() != 0 || first.GetRoundNum() != 1 {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return p7GenericMechanismRushInputAtAdmitted(series, first.GetRoundTime())
}

func p7GenericMechanismRushManualSourceDigest(input *model.FrozenBattleInput, record *pb.BattleOperate) (string, string, error) {
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil {
		return "", "", err
	}
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(record)
	if err != nil {
		return "", "", err
	}
	recordHash := p6Digest(body)
	bound, err := json.Marshal(struct {
		FrozenInput    string `json:"frozenInput"`
		OriginalRecord string `json:"originalRecord"`
	}{p6Digest(raw), recordHash})
	if err != nil {
		return "", "", err
	}
	return recordHash, p6Digest(bound), nil
}

func p7GenericMechanismRushValidateEnd(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome, supplied *pb.BattleOperate, reported int32) error {
	admission, err := newP7GenericMechanismRushAdmission(series)
	if err != nil {
		return err
	}
	return admission.validateEnd(series, out, supplied, reported)
}

func (admission *p7GenericMechanismRushAdmission) validateEnd(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome, supplied *pb.BattleOperate, reported int32) error {
	if err := admission.validate(series); err != nil {
		return err
	}
	if err := p7GenericMechanismRushValidateControls(series, supplied); err != nil {
		return err
	}
	if out == nil || out.Outcome == nil {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if out.Outcome.DiagnosticAIDriverPolicy == constUtil.P6LiveRecordedDriver {
		return p7LiveRecordedRushValidateEnd(series, out, supplied, reported)
	}
	if out.Outcome.DiagnosticAIDriverPolicy == constUtil.P6RoundLiveAutoDriver {
		return p7RoundLiveAutoRushValidateEnd(series, out, supplied, reported)
	}
	var input *model.FrozenBattleInput
	var err error
	switch out.Outcome.DiagnosticAIDriverPolicy {
	case "":
		input, err = cloneP6CalculationInput(series.Template)
	case constUtil.P6DiagnosticManualAutoStart:
		input, err = p7GenericMechanismRushAutoCalculationInputAdmitted(series, supplied)
		if err != nil {
			return err
		}
	case constUtil.P6RoundOneLiveAutoFormationDriver:
		if supplied.AttackerTransOpt == nil {
			return errors.New(constUtil.P6ReplayConflict)
		}
		input, err = cloneP6CalculationInput(series.Template)
	default:
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P7GenericMechanismCompatibilityProof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || !proto.Equal(supplied, out.OriginalOperates) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	recordHash, sourceHash, err := p7GenericMechanismRushManualSourceDigest(input, supplied)
	if out.Outcome.DiagnosticAIDriverPolicy == constUtil.P6RoundOneLiveAutoFormationDriver {
		recordHash, sourceHash, err = P6RoundOneLiveAutoFormationSourceDigests(input, supplied, supplied.AttackerTransOpt)
	}
	if err != nil || out.OriginalRecordSHA256 != recordHash || out.CalculationSourceSHA256 != sourceHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	positions := map[string]int32{}
	for _, actor := range input.Attacker.Formation {
		positions[actor.GetGuid()] = actor.GetPose()
	}
	if change := supplied.AttackerTransOpt; change != nil {
		for index, uid := range change.ActorUids {
			positions[uid] = change.Fpids[index]
		}
	}
	for _, actor := range out.FinalActors {
		expected := int32(0)
		if actor.Camp == 0 {
			position, ok := positions[actor.GUID]
			if !ok {
				return errors.New(constUtil.P6ReplayConflict)
			}
			if position != actor.Pose {
				expected = position
			}
		}
		if actor.RuntimePose != expected {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	expected := int32(0)
	if out.Outcome.Winner == 0 {
		expected = 1
	}
	if reported != expected {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

func (admission *p7GenericMechanismRushAdmission) validatePreview(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome) error {
	if err := admission.validate(series); err != nil {
		return err
	}
	input, err := p7GenericMechanismRushInputAtAdmitted(series, 0)
	if err != nil {
		return err
	}
	return p7GenericMechanismRushValidateTerminal(input, out)
}
