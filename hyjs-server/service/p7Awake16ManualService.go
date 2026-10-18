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

// validateP7Awake16ManualRecord 只允许冻结队伍实际发出的原操作；不导入胜者或客户端HP。
func validateP7Awake16ManualRecord(input *model.FrozenBattleInput, record *pb.BattleOperate) error {
	if err := validateP7Awake16Scope(input); err != nil {
		return err
	}
	if record == nil || record.DefenderTransOpt != nil || len(record.SkillOpts) == 0 || len(record.SkillOpts) > 16000 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if change := record.AttackerTransOpt; change != nil {
		if change.GetRoundNum() != 1 || len(change.ActorUids) != len(input.Attacker.Formation) || len(change.Fpids) != len(change.ActorUids) {
			return errors.New(constUtil.P6ReplayConflict)
		}
		seenActors, seenPositions := map[string]bool{}, map[int32]bool{}
		for index, uid := range change.ActorUids {
			found := false
			for _, actor := range input.Attacker.Formation {
				if actor.GetGuid() == uid {
					found = true
				}
			}
			if !found || seenActors[uid] || change.Fpids[index] < 1 || change.Fpids[index] > 15 || seenPositions[change.Fpids[index]] {
				return errors.New(constUtil.P6ReplayConflict)
			}
			seenActors[uid] = true
			seenPositions[change.Fpids[index]] = true
		}
	}
	for _, op := range record.SkillOpts {
		if op == nil || op.GetCamp() < 0 || op.GetCamp() > 1 || op.GetRoundNum() < 1 || op.GetRoundNum() > input.MaxRound || !combatFinite(op.GetRoundTime()) || op.GetRoundTime() < 0 || op.GetRoundTime() > 120 || op.GetSkillIndex() < 0 || op.GetSkillIndex() > 3 {
			return errors.New(constUtil.P6ReplayConflict)
		}
		// ParseOptRecord 保存成功操作；敌方仍用冻结NPC的原AI重新产生，逐项严格核对。
		formation := input.Attacker.Formation
		if op.GetCamp() == 1 {
			formation = input.Monster.Formation
		}
		found := false
		for _, actor := range formation {
			if actor.GetGuid() == op.GetActorUid() {
				found = true
				break
			}
		}
		if !found {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	return nil
}

// p7Awake16ManualSourceDigest 同时绑定原冻结参数和实际原记录，保留旧自动证书的独立范围。
func p7Awake16ManualSourceDigest(input *model.FrozenBattleInput, record *pb.BattleOperate) (string, string, error) {
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

// computeP7Awake16ManualTerminal 执行原回放driver；只有我方真实操作作为输入，伤害、敌方和终局独立重算。
func (s *GameService) computeP7Awake16ManualTerminal(ctx context.Context, input *model.FrozenBattleInput, record *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	if err := validateP7Awake16ManualRecord(input, record); err != nil {
		return nil, err
	}
	return s.computeFrozenBattleFinalHP(ctx, input, record, true)
}

func validateP7Awake16ManualTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, record *pb.BattleOperate, reported int32) error {
	if err := validateP7Awake16ManualRecord(input, record); err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !proto.Equal(record, out.OriginalOperates) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	recordHash, sourceHash, err := p7Awake16ManualSourceDigest(input, record)
	if err != nil || out.OriginalRecordSHA256 != recordHash || out.CalculationSourceSHA256 != sourceHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	// 复用原四演员/终局HP/胜者结构验证。该局部视图只适配验证器的自动来源字段，真实双摘要已独立核对。
	view := *out
	view.OriginalRecordSHA256 = ""
	// 原布阵是输入，先核真实运行位置与完整重演记录，再适配只认识自动原位的结构验证器。
	view.OriginalOperates = proto.Clone(out.OriginalOperates).(*pb.BattleOperate)
	view.OriginalOperates.AttackerTransOpt = nil
	view.FinalActors = append([]model.CombatFinalActorHP(nil), out.FinalActors...)
	positions := map[string]int32{}
	for _, actor := range input.Attacker.Formation {
		positions[actor.GetGuid()] = actor.GetPose()
	}
	if change := record.AttackerTransOpt; change != nil {
		for index, uid := range change.ActorUids {
			positions[uid] = change.Fpids[index]
		}
	}
	for index, actor := range view.FinalActors {
		if actor.Camp != 0 {
			continue
		}
		position, ok := positions[actor.GUID]
		if !ok {
			return errors.New(constUtil.P6ReplayConflict)
		}
		expected := position
		if expected == actor.Pose {
			expected = 0
		}
		if actor.RuntimePose != expected {
			return errors.New(constUtil.P6ReplayConflict)
		}
		view.FinalActors[index].RuntimePose = 0
	}
	source := *input
	source.Tables = nil
	raw, err := json.Marshal(&source)
	if err != nil {
		return err
	}
	view.CalculationSourceSHA256 = p6Digest(raw)
	if err = validateP7Awake16Terminal(input, &view); err != nil {
		return err
	}
	expected := int32(0)
	if out.Outcome.Winner == 0 {
		expected = 1
	}
	if expected != reported {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

// ComputeP7Awake16ManualSettlement 的独立proof不能复用强制自动的原证书。
func (s *GameService) ComputeP7Awake16ManualSettlement(ctx context.Context, input *model.FrozenBattleInput, record *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	if constUtil.P7Awake16ManualProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.computeP7Awake16ManualTerminal(ctx, input, record)
	if err != nil {
		return nil, err
	}
	if err = validateP7Awake16ManualTerminal(input, out, record, reported); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P7Awake16ManualProof
	return out, nil
}

// validateP7Awake16EndReceipt 冷重试分别校验旧自动和新原记录family的来源与能力标识。
func validateP7Awake16EndReceipt(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, record *pb.BattleOperate, reported int32) error {
	if out != nil && out.Outcome != nil && constUtil.P7Awake16ManualProof != "" && out.Outcome.CapabilityHash == constUtil.P7Awake16ManualProof {
		if !out.Outcome.Authoritative {
			return errors.New(constUtil.P6ReplayConflict)
		}
		return validateP7Awake16ManualTerminal(input, out, record, reported)
	}
	return ValidateP7Awake16CertifiedReceipt(input, out)
}
