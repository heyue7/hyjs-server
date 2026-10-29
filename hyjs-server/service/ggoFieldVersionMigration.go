package service

import (
	"errors"
	"strings"

	"hyjs-server/db"
	"hyjs-server/model"
)

// A current node must still match the exact reservation. After a publish, an
// old frozen attempt may settle on its own durable attempt/profile identity;
// it must never consume or release another generation's reservation.
func ggoResolveGatherReservation(state *ggoLineState, attempt *db.P8GatherAttempt, frozen *ggoGatherFrozen) (*ggoResourceNode, bool, bool, error) {
	var current *ggoResourceNode
	for i := range state.Resources {
		if state.Resources[i].PointID == attempt.PosID {
			current = &state.Resources[i]
			break
		}
	}
	if current != nil && current.Generation == attempt.Generation && current.ReservedBy == attempt.PlayerID && current.AttemptToken == attempt.Token {
		return current, false, false, nil
	}
	released := ggoReleaseLegacyBlock(state, attempt.PosID, "gather", attempt.PlayerID, attempt.Token, attempt.Generation)
	revision, ok := strings.CutPrefix(frozen.Source, "ggo-v4:")
	if !released && (!ok || revision == "" || revision == state.DeploymentRevision) {
		return nil, false, false, errors.New("沙漠采集预约状态不一致")
	}
	if !released || current == nil || current.Kind != frozen.Kind || current.ReservedBy != 0 {
		current = nil
	}
	return current, true, released, nil
}

func ggoResolveMonsterReservation(state *ggoLineState, fight *model.GGOFieldMonsterFight, playerID int64, now int64) (*ggoMonsterNode, bool, error) {
	var current *ggoMonsterNode
	for i := range state.Monsters {
		if state.Monsters[i].PointID == fight.PointID {
			current = &state.Monsters[i]
			break
		}
	}
	if current != nil && current.UUID == fight.MonsterUUID && current.Generation == fight.Generation && current.ReservedBy == playerID && current.FightID == fight.FightID && (current.ReservedUntilMS == 0 || current.ReservedUntilMS >= now) {
		return current, false, nil
	}
	released := ggoReleaseLegacyBlock(state, fight.PointID, "monster", playerID, fight.FightID, fight.Generation)
	if !released && (fight.DeploymentRevision == "" || fight.DeploymentRevision == state.DeploymentRevision) {
		return nil, false, errors.New("沙漠小怪战斗占用已失效")
	}
	if !released || current == nil || current.ReservedBy != 0 {
		current = &ggoMonsterNode{}
	}
	return current, true, nil
}
