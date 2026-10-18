package service

import (
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// SetP6FirstOrder writes the original player-instance flags for a newly created
// two-player battle. Historical frozen snapshots are never normalized here.
func SetP6FirstOrder(attacker, defender *pb.BattlePlayer, defenderFirst bool) error {
	if attacker == nil || defender == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if attacker.Inst == nil {
		attacker.Inst = &pb.BattlePlayerInst{}
	}
	if defender.Inst == nil {
		defender.Inst = &pb.BattlePlayerInst{}
	}
	attacker.Inst.FirstOrder, defender.Inst.FirstOrder = proto.Int32(1), proto.Int32(0)
	if defenderFirst {
		attacker.Inst.FirstOrder, defender.Inst.FirstOrder = proto.Int32(0), proto.Int32(1)
	}
	return nil
}

// ValidateP6FirstOrder follows original BattleData.SetPlaybackData: only 0/1
// starts the defending player; every other pair starts the attacking player.
// Keeping the fallback permits old 0/0 snapshots without changing their wire.
func ValidateP6FirstOrder(input *model.FrozenBattleInput) error {
	if input == nil || input.Attacker == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	defenderFirst := input.Defender != nil && input.Attacker.GetInst().GetFirstOrder() == 0 && input.Defender.GetInst().GetFirstOrder() == 1
	if input.KFReplay != nil {
		if err := validateP6KFProjection(input); err != nil {
			return err
		}
		// 原KF先读全局0/1，再按当地角色交换双方，不反转attackFirst。
		players := input.KFReplay.GetPlayerList()
		defenderFirst = players[0].GetInst().GetFirstOrder() == 0 && players[1].GetInst().GetFirstOrder() == 1
	}
	if input.DefenderFirst != defenderFirst {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}
