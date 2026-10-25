package service

import (
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// 显式重置放弃本轮待结算普通试炼，保留冻结证据；旧End无法为新轮次结算。
// 调用方在同一事务内校验次数、扣费及切换RunID，失败会一起回滚。
func abandonP7TrialForReset69(tx *gorm.DB, state *db.P7RushState) error {
	if state.ActiveFight == "" {
		return nil
	}
	var fight db.P7RushFight
	if err := tx.First(&fight, "id=? AND player_id=? AND run_id=?", state.ActiveFight, state.PlayerID, state.RunID).Error; err != nil {
		return err
	}
	if fight.Diagnostic || fight.Kind != constUtil.P7RushTrial || fight.Status != constUtil.P7RushComputed || p6Digest(fight.Rule) != fight.RuleHash || p6Digest(fight.Start) != fight.StartHash || len(fight.Result) != 0 || len(fight.EndRequest) != 0 {
		return errors.New("当前试炼战斗无法放弃，请先完成结算")
	}
	if err := tx.Model(&fight).Update("status", constUtil.P7RushAbandoned69).Error; err != nil {
		return err
	}
	state.ActiveFight = ""
	return nil
}
