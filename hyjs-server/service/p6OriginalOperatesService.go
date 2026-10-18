package service

import (
	"context"
	"errors"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// OriginalP6RecordedCamp follows original InFightData.BuildInFightData00e3..00ff.
// These original modes enable the attacker record container. ComputeCore.DoSkill
// puts successful actions from both camps in that container; this flag names
// the container owner and does not filter the action's camp.
func OriginalP6RecordedCamp(battleType, camp int32) bool {
	if camp != 0 {
		return false
	}
	switch battleType {
	case 1, 8, 4, 9, 10, 6, 12, 18, 16:
		return true
	}
	return false
}

func (e *combatRoundEngine) originalUsesSuppliedCamp() bool {
	return e.supplied != nil && (!e.originalRecordMode || OriginalP6RecordedCamp(e.input.BattleType, e.camp))
}

func p6OriginalProtocolRecordEqual(want, actual *pb.BattleOperate) bool {
	// Actual input skill order is executed and validated by the original driver.
	// PlaybackAI uses one message per Update, so rerecorded press clocks can be
	// one frame later than simultaneous source presses. Verify every action's
	// identity and original automatic NPC clocks, then exact terminal round/HP.
	// Source records remain unchanged; computed player clocks are audited apart.
	if want == nil || actual == nil || len(want.SkillOpts) != len(actual.SkillOpts) {
		return false
	}
	for index, source := range want.SkillOpts {
		computed := actual.SkillOpts[index]
		if source == nil || computed == nil || source.GetRoundNum() != computed.GetRoundNum() || source.GetCamp() != computed.GetCamp() || source.GetActorUid() != computed.GetActorUid() || source.GetSkillIndex() != computed.GetSkillIndex() || source.GetCamp() == 1 && math.Abs(source.GetRoundTime()-computed.GetRoundTime()) > 1e-8 {
			return false
		}
	}
	left := proto.Clone(want).(*pb.BattleOperate)
	right := proto.Clone(actual).(*pb.BattleOperate)
	left.SkillOpts, right.SkillOpts = nil, nil
	if !proto.Equal(left.AttackerTransOpt, right.AttackerTransOpt) || !proto.Equal(left.DefenderTransOpt, right.DefenderTransOpt) {
		return false
	}
	left.AttackerTransOpt, right.AttackerTransOpt = nil, nil
	left.DefenderTransOpt, right.DefenderTransOpt = nil, nil
	return p6OriginalOperateEqual(left, right)
}

// OriginalP6OperateProjection preserves final-round/HP fields and the original
// recorded skill/transform lists. Original ParseOptRecord takes UID.Split('_')[1],
// including its truncation of a source GUID containing additional underscores.
func OriginalP6OperateProjection(battleType int32, all *pb.BattleOperate) (*pb.BattleOperate, error) {
	if all == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	out := &pb.BattleOperate{}
	// Original protobuf-net messages omit scalar defaults, even after setters.
	if all.GetFinalRoundNum() != 0 {
		out.FinalRoundNum = proto.Int32(all.GetFinalRoundNum())
	}
	if all.GetAttackerTotalLeftHp() != 0 {
		out.AttackerTotalLeftHp = proto.Float64(all.GetAttackerTotalLeftHp())
	}
	if all.GetDefenderTotalLeftHp() != 0 {
		out.DefenderTotalLeftHp = proto.Float64(all.GetDefenderTotalLeftHp())
	}
	for _, op := range all.SkillOpts {
		if op == nil {
			return nil, errors.New(constUtil.CombatInvalid)
		}
		if !OriginalP6RecordedCamp(battleType, 0) {
			continue
		}
		copy := &pb.BattleSkillOperate{}
		if op.GetRoundNum() != 0 {
			copy.RoundNum = proto.Int32(op.GetRoundNum())
		}
		if op.GetRoundTime() != 0 {
			copy.RoundTime = proto.Float64(op.GetRoundTime())
		}
		if op.GetCamp() != 0 {
			copy.Camp = proto.Int32(op.GetCamp())
		}
		uid := strings.Split(op.GetActorUid(), "_")[0]
		if uid != "" {
			copy.ActorUid = proto.String(uid)
		}
		if op.GetSkillIndex() != 0 {
			copy.SkillIndex = proto.Int32(op.GetSkillIndex())
		}
		out.SkillOpts = append(out.SkillOpts, copy)
	}
	firstTransform := all.AttackerTransOpt
	if all.DefenderTransOpt != nil && (firstTransform == nil || all.DefenderTransOpt.GetRoundNum() < firstTransform.GetRoundNum()) {
		firstTransform = all.DefenderTransOpt
	} else if all.DefenderTransOpt != nil && firstTransform.GetRoundNum() == all.DefenderTransOpt.GetRoundNum() {
		// The diagnostic protobuf has no within-round transform ordering. Keep
		// this ambiguous projection closed until the actual event order exists.
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if OriginalP6RecordedCamp(battleType, 0) && firstTransform != nil {
		out.AttackerTransOpt = proto.Clone(firstTransform).(*pb.BattleTransOperate)
		for i, uid := range out.AttackerTransOpt.ActorUids {
			out.AttackerTransOpt.ActorUids[i] = strings.Split(uid, "_")[0]
		}
	}
	return out, nil
}

// ComputeOriginalP6ProtocolBattle uses the original PVE playback driver for camp0
// while its enemy remains automatic. The original recorded container includes
// successful actions from both camps. Existing ComputeFrozenBattle semantics
// remain available for historical complete diagnostic replay and are unchanged.
func (s *GameService) ComputeOriginalP6ProtocolBattle(ctx context.Context, input *model.FrozenBattleInput, originalRecord *pb.BattleOperate) (*model.CombatOriginalOperateOutcome, error) {
	if err := validateOriginalP6ProtocolInput(input, originalRecord); err != nil {
		return nil, err
	}
	out, err := s.computeFrozenBattleRecordObserved(ctx, input, originalRecord, nil, nil, true)
	if err != nil {
		return nil, err
	}
	record, err := OriginalP6OperateProjection(input.BattleType, out.Operates)
	if err != nil {
		return nil, err
	}
	result := &model.CombatOriginalOperateOutcome{Schema: constUtil.P6OriginalOperatesSchema, RecordAttacker: OriginalP6RecordedCamp(input.BattleType, 0), RecordDefender: false, OriginalOperates: record, Outcome: out}
	if originalRecord != nil {
		result.SuppliedOriginalOperates = proto.Clone(originalRecord).(*pb.BattleOperate)
	}
	return result, nil
}

// validateOriginalP6ProtocolInput preserves the actually audited wrapper shapes
// for both original record computation and its opt-in final runtime HP audit.
func validateOriginalP6ProtocolInput(input *model.FrozenBattleInput, originalRecord *pb.BattleOperate) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	// Record flags and original driver selection are separate contracts. These
	// actually audited callbacks have fixed player/monster initialization and
	// playback/automatic driver behavior; other mode wrappers need their own
	// original SetData/SetFightEngineData proof before accepting a source record.
	switch input.BattleType {
	case 1, 4, 9, 10:
		if input.Monster == nil || input.Defender != nil {
			return errors.New(constUtil.CombatUnsupported)
		}
	case 2, 3, 11:
		if input.Defender == nil || input.Monster != nil {
			return errors.New(constUtil.CombatUnsupported)
		}
	case 14:
	default:
		return errors.New(constUtil.CombatUnsupported)
	}
	if originalRecord != nil {
		for _, op := range originalRecord.SkillOpts {
			if op == nil || !OriginalP6RecordedCamp(input.BattleType, 0) || op.GetCamp() < 0 || op.GetCamp() > 1 {
				return errors.New(constUtil.P6ReplayConflict)
			}
		}
	}
	return nil
}
