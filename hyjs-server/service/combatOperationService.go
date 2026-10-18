package service

import (
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"math"
)

// p6OriginalOperateEqual compares original field values when protobuf-net omits defaults.
// HP and end round remain exact; tiny time accumulation differences cannot move a 0.1s frame.
func p6OriginalOperateEqual(a, b *pb.BattleOperate) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.GetFinalRoundNum() != b.GetFinalRoundNum() || a.GetAttackerTotalLeftHp() != b.GetAttackerTotalLeftHp() || a.GetDefenderTotalLeftHp() != b.GetDefenderTotalLeftHp() || len(a.SkillOpts) != len(b.SkillOpts) || a.AttackerTransOpt != nil || a.DefenderTransOpt != nil || b.AttackerTransOpt != nil || b.DefenderTransOpt != nil {
		return false
	}
	for i, x := range a.SkillOpts {
		y := b.SkillOpts[i]
		if x == nil || y == nil || x.GetRoundNum() != y.GetRoundNum() || x.GetCamp() != y.GetCamp() || x.GetActorUid() != y.GetActorUid() || x.GetSkillIndex() != y.GetSkillIndex() || !combatFinite(x.GetRoundTime()) || !combatFinite(y.GetRoundTime()) || math.Abs(x.GetRoundTime()-y.GetRoundTime()) > constUtil.P6OperationTimeEpsilon {
			return false
		}
	}
	return true
}
