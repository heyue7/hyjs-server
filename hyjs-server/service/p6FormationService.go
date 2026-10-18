package service

import (
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// position is original FActor.MainPosition.ToFPID, separate from its source hero.
// The fallback retains synthetic mechanism actor helpers before initialization.
func (a *combatRoundActor) position() int32 {
	if a.runtimePose > 0 {
		return a.runtimePose
	}
	return a.hero.GetPose()
}

func (e *combatRoundEngine) validateOriginalFormationRecord(record *pb.BattleOperate) error {
	if record.DefenderTransOpt != nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	change := record.AttackerTransOpt
	if change == nil {
		return nil
	}
	// Original supported PVE PlaybackAI controls camp0. Generic historical
	// fullcamp diagnostic streams and original automatic PvP do not execute it.
	if !e.originalRecordMode || !OriginalP6RecordedCamp(e.input.BattleType, 0) || !originalP6CopyEnemyType(e.input.BattleType) {
		return errors.New(constUtil.CombatUnsupported)
	}
	if change.GetRoundNum() != 1 || change.GetRoundNum() > e.input.MaxRound || len(change.ActorUids) == 0 || len(change.ActorUids) != len(change.Fpids) || len(change.ActorUids) > constUtil.P6MaxActors {
		return errors.New(constUtil.CombatInvalid)
	}
	camp := (change.GetRoundNum() - 1) % 2
	if e.input.DefenderFirst {
		camp = 1 - camp
	}
	if camp != 0 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	seen := map[string]bool{}
	for i, uid := range change.ActorUids {
		if uid == "" || seen[uid] || change.Fpids[i] < 1 || change.Fpids[i] > 15 || e.find("0_"+uid, 0) == nil {
			return errors.New(constUtil.CombatInvalid)
		}
		seen[uid] = true
	}
	return nil
}

// Original DoTrans mutates each MainPosition in wire order, then records the
// complete command. It does not rewrite source heroes, stats, HP, CD or Buffs.
func (e *combatRoundEngine) applyOriginalFormationRecord() error {
	if e.supplied == nil || e.supplied.AttackerTransOpt == nil || e.originalFormationApplied {
		return nil
	}
	change := e.supplied.AttackerTransOpt
	if change.GetRoundNum() != e.round || !e.originalUsesSuppliedCamp() {
		return nil
	}
	return e.applyOriginalFormationChange(change)
}

// applyOriginalFormationChange is the shared original DoTrans operation; driver
// selection and timing are checked by the caller before changing runtime slots.
func (e *combatRoundEngine) applyOriginalFormationChange(change *pb.BattleTransOperate) error {
	for i, uid := range change.ActorUids {
		actor := e.find("0_"+uid, 0)
		if actor == nil {
			return errors.New(constUtil.P6ReplayConflict)
		}
		actor.runtimePose = change.Fpids[i]
		e.trace("formation_position", actor, nil, "", "", float64(change.Fpids[i]))
	}
	e.operate.AttackerTransOpt = proto.Clone(change).(*pb.BattleTransOperate)
	e.originalFormationApplied = true
	return nil
}
