package service

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// ensureNoActiveDailyCopy 在角色行锁内拒绝与本日资源副本并行创建战斗。
// 已有战斗回执应在调用之前返回；旧日快照保留结算能力，不占用新日挑战。
func (s *GameService) ensureNoActiveDailyCopy(tx *gorm.DB, id int64, now time.Time) error {
	if s.P12WantedReady {
		if err := P12WantedRequireIdle(tx, id, now); err != nil {
			return err
		}
	}
	if s.P12FieldPVPReady {
		if err := P12FieldPVPRequireIdle(tx, id, now); err != nil {
			return err
		}
	}
	// Original exploration owns the same player until its empty Settle request commits.
	if err := p7AwakeRequireIdle(tx, id); err != nil {
		return err
	}
	var count int64
	// 所有开战入口在角色行锁内检查未结束首领；开战重放在此之前恢复原回执。
	if s.P12WorldBossBattleReady {
		if err := tx.Model(&db.P12WorldBossBattleReceipt{}).Where("player_id = ? AND committed = ? AND abandoned_at = 0", id, false).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("当前世界首领战斗尚未结束")
		}
	}
	if err := tx.Model(&db.DailyCopyFight{}).Where("player_id = ? AND day = ? AND settled = ?", id, dailyCopyDay(LocalPlayAt(tx.Statement.Context, now)), false).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New(constUtil.DailyCopyActiveError)
	}
	return nil
}
