package service

import (
	"context"
	"errors"
	"math"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原type16保留自己的PVE驱动；资格来自实际原技能/天赋机制，不再由示范角色ID决定。
func (s *GameService) p7Awake16GenericProof(ctx context.Context, input *model.FrozenBattleInput) (string, error) {
	if ctx == nil || input == nil || input.BattleType != 16 || input.MaxRound != 40 || input.Attacker == nil || input.Attacker.Inst == nil || input.Monster == nil || input.Monster.Inst == nil || input.Defender != nil || input.KFReplay != nil || input.KFReplayViewer != nil || input.IsAttToDef || input.DefenderFirst || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.Seed < 0 || input.Seed > math.MaxInt32 || input.Attacker.GetServerId() <= 0 || len(constUtil.P7Awake16GenericCompatibilityProof) != 64 {
		return "", errors.New(constUtil.CombatUnsupported)
	}
	for _, factor := range []*float64{input.AttackerDamageFactor, input.DefenderDamageFactor} {
		if factor == nil || *factor != 1 {
			return "", errors.New(constUtil.CombatUnsupported)
		}
	}
	if _, err := s.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return "", err
	}
	return constUtil.P7Awake16GenericCompatibilityProof, nil
}

// 新开战优先采用具名通用族。旧证书只服务其原范围及已保存回执。
func (s *GameService) p7AwakeBattleProof(ctx context.Context, input *model.FrozenBattleInput) (string, error) {
	proof, err := s.p7Awake16GenericProof(ctx, input)
	if err == nil {
		return proof, nil
	}
	if old, oldErr := P7Awake16ProofForInput(input); oldErr == nil {
		return old, nil
	}
	return "", err
}

func p7Awake16GenericRecord(input *model.FrozenBattleInput, record *pb.BattleOperate) error {
	if input == nil || input.Attacker == nil || input.Monster == nil || record == nil || p6ScopedUnknown(record) || record.DefenderTransOpt != nil || len(record.SkillOpts) == 0 || len(record.SkillOpts) > 16000 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if change := record.AttackerTransOpt; change != nil {
		if p6ScopedUnknown(change) || change.GetRoundNum() != 1 || len(change.ActorUids) != len(input.Attacker.Formation) || len(change.Fpids) != len(change.ActorUids) {
			return errors.New(constUtil.P6ReplayConflict)
		}
		seen, positions := map[string]bool{}, map[int32]bool{}
		for index, uid := range change.ActorUids {
			found := false
			for _, actor := range input.Attacker.Formation {
				found = found || actor.GetGuid() == uid
			}
			if !found || seen[uid] || change.Fpids[index] < 1 || change.Fpids[index] > 15 || positions[change.Fpids[index]] {
				return errors.New(constUtil.P6ReplayConflict)
			}
			seen[uid], positions[change.Fpids[index]] = true, true
		}
	}
	for _, op := range record.SkillOpts {
		if op == nil || p6ScopedUnknown(op) || op.GetCamp() < 0 || op.GetCamp() > 1 || op.GetRoundNum() < 1 || op.GetRoundNum() > input.MaxRound || !combatFinite(op.GetRoundTime()) || op.GetRoundTime() < 0 || op.GetRoundTime() > constUtil.P6RoundTimeLimit || op.GetSkillIndex() < 0 || op.GetSkillIndex() > 3 {
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

// 原完整操作优先重演我方成功命令，NPC仍独立原AI；不能用客户端胜利/HP替代计算。
func (s *GameService) ComputeP7Awake16GenericSettlement(ctx context.Context, input *model.FrozenBattleInput, record *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	proof, err := s.p7Awake16GenericProof(ctx, input)
	if err != nil {
		return nil, err
	}
	if err = p7Awake16GenericRecord(input, record); err != nil {
		return nil, err
	}
	out, manualErr := s.computeFrozenBattleFinalHP(ctx, input, record, true)
	if manualErr == nil && out != nil && out.Outcome != nil && proto.Equal(record, out.OriginalOperates) {
		out.Outcome.Authoritative = true
		out.Outcome.CapabilityHash = proof
		if err = validateP7Awake16GenericEnd(ctx, input, out, record, reported); err == nil {
			return out, nil
		}
		manualErr = err
	}
	// Round1轮首DoTrans后开启原LiveAuto；不将技能回放输入用作自动决策。
	if record.AttackerTransOpt != nil {
		out, err = s.ComputeP6RoundOneLiveAutoFormation(ctx, input, record, record.AttackerTransOpt)
		if err != nil {
			return nil, err
		}
		out.Outcome.Authoritative = true
		out.Outcome.CapabilityHash = proof
		if err = validateP7Awake16GenericEnd(ctx, input, out, record, reported); err != nil {
			return nil, err
		}
		return out, nil
	}
	// 原LiveAuto与PlaybackAI每帧取命令规则不同。仅重算无布阵记录的原自动起点，
	// 全部命令时间与终末字段仍逐字相等；手动/布阵不能进入该分支冒充自动。
	autoInput, err := p7Awake16GenericAutoInput(input, record)
	if err != nil {
		if manualErr != nil {
			return nil, manualErr
		}
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	out, err = s.computeFrozenBattleFinalHP(ctx, autoInput, nil, true)
	if err != nil || !proto.Equal(record, out.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p7Awake16ManualSourceDigest(autoInput, record)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	if err = validateP7Awake16GenericEnd(ctx, input, out, record, reported); err != nil {
		return nil, err
	}
	return out, nil
}

func p7Awake16GenericAutoInput(input *model.FrozenBattleInput, record *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if err := p7Awake16GenericRecord(input, record); err != nil {
		return nil, err
	}
	first := record.SkillOpts[0]
	if record.AttackerTransOpt != nil || first.GetCamp() != 0 || first.GetRoundNum() != 1 || first.GetRoundTime() > constUtil.P7NaturalRushAutoRoundTimeLimit {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	copy, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	copy.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	copy.DiagnosticManualAutoStartRoundTime = proto.Float64(first.GetRoundTime())
	if err = ValidateP6AIDriverPolicy(copy); err != nil {
		return nil, err
	}
	return copy, nil
}

func validateP7Awake16GenericEnd(ctx context.Context, input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, record *pb.BattleOperate, reported int32) error {
	if input == nil {
		return errors.New(constUtil.P6ReplayConflict)
	}
	validator := &GameService{Tables: input.Tables}
	proof, err := validator.p7Awake16GenericProof(ctx, input)
	if err != nil {
		return err
	}
	if err = p7Awake16GenericRecord(input, record); err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || !proto.Equal(record, out.OriginalOperates) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	calculation := input
	switch out.Outcome.DiagnosticAIDriverPolicy {
	case "":
	case constUtil.P6DiagnosticManualAutoStart:
		calculation, err = p7Awake16GenericAutoInput(input, record)
		if err != nil {
			return err
		}
	case constUtil.P6RoundOneLiveAutoFormationDriver:
		if record.AttackerTransOpt == nil {
			return errors.New(constUtil.P6ReplayConflict)
		}
	default:
		return errors.New(constUtil.P6ReplayConflict)
	}
	recordHash, sourceHash, err := p7Awake16ManualSourceDigest(calculation, record)
	if out.Outcome.DiagnosticAIDriverPolicy == constUtil.P6RoundOneLiveAutoFormationDriver {
		recordHash, sourceHash, err = P6RoundOneLiveAutoFormationSourceDigests(calculation, record, record.AttackerTransOpt)
	}
	if err != nil || out.OriginalRecordSHA256 != recordHash || out.CalculationSourceSHA256 != sourceHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(calculation, out); err != nil {
		return err
	}
	positions := map[string]int32{}
	for _, actor := range input.Attacker.Formation {
		positions[actor.GetGuid()] = actor.GetPose()
	}
	if change := record.AttackerTransOpt; change != nil {
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
