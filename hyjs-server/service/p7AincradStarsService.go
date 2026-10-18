package service

import (
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// recordP7AcceptedStarActivity runs inside p7End's already verified trial-win
// branch after real stars are credited and before the fight is committed.
// Keep the original global peak fact and record a separate earned delta for
// personal cycles, so a new one-star action cannot inherit an old ten-star run.
func (s *GameService) recordP7AcceptedStarActivity(tx *gorm.DB, fight *db.P7RushFight, cfg db.P7RushStageRule, state *db.P7RushState) error {
	if fight == nil || state == nil || fight.PlayerID <= 0 || fight.ID == "" || fight.Diagnostic || fight.Kind != constUtil.P7RushTrial || cfg.Kind != constUtil.P7RushTrial || cfg.TaskID != fight.TaskID || cfg.Star < 1 || cfg.Star > 3 || state.PlayerID != fight.PlayerID || state.ActiveFight != fight.ID || state.RunID != fight.RunID || state.TotalStars < cfg.Star {
		return errors.New(constUtil.P7AincradAcceptedInvalid)
	}
	now := LocalPlayNow(tx.Statement.Context)
	// 真实得星继续入账，活动奖励由后台显式开放，通关不再自动开启投放。
	if err := s.seedP7StarActivity(tx, now, false); err != nil {
		return err
	}
	source := "rush-stars:" + fight.ID
	if err := s.RecordP1Event(tx, fight.PlayerID, source, constUtil.P7RushStarEvent, int64(state.TotalStars), now); err != nil {
		return err
	}
	return s.RecordP1Event(tx, fight.PlayerID, source, constUtil.P7RushEarnedStarEvent, int64(cfg.Star), now)
}
