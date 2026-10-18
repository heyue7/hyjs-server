package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// p14AuditionAutoRecoveryVerify verifies the separately prepublished local
// recovery rule; the recovery observation is never backdated into a fake match.
func p14AuditionAutoRecoveryVerify(boundary *db.P14AuditionBoundary, qualified *db.P14AuditionQualification) error {
	if qualified.RecoverySource == "" && qualified.RecoveryPublishedAt == 0 {
		return nil
	}
	if qualified.RecoverySource != constUtil.P14AuditionAutoRecoverySource || qualified.RecoveryPublishedAt <= 0 || qualified.RoundCount != 0 {
		return errors.New("海选停服恢复资格缺具名预发布规则")
	}
	var slots []db.P14TimeSlot
	if json.Unmarshal(boundary.Calendar, &slots) != nil {
		return errors.New("海选停服恢复缺原冻结日历")
	}
	var start, end int64
	observed := false
	for _, slot := range slots {
		if slot.Stage == constUtil.P14AuditionStage {
			start, end = slot.StartAt, slot.EndAt
		}
		if slot.Stage >= 5 && slot.Stage <= constUtil.P14LegacyPromotionStage && qualified.QualifiedAt >= slot.StartAt && qualified.QualifiedAt < slot.EndAt {
			observed = true
		}
	}
	if start <= 0 || end <= start || qualified.RecoveryPublishedAt >= start || qualified.QualifiedAt < end || !observed {
		return errors.New("海选停服恢复的预发布或实际观察时刻不符")
	}
	return nil
}

// recoverP14AuditionAuto catches up only a truly empty small-roster audition.
// Any round or match keeps the battle authority boundary intact.
func (s *GameService) recoverP14AuditionAuto(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var gate db.P14AuditionMatchPolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || !gate.Enabled || !gate.AutoRecoveryEnabled {
			return nil
		}
		stage, _, err := s.p14SeasonStage(tx, &policy, now)
		if err != nil {
			return err
		}
		if stage < 5 || stage > constUtil.P14PromotionStage {
			return nil
		}
		boundary, roster, err := p14AuditionRead(tx, policy.SeasonIndex, s.OriginalServerID())
		if err != nil {
			return err
		}
		if boundary.FrozenAt > now.UnixMilli() || len(roster) == 0 || len(roster) > constUtil.P14AuditionPromotionLimit {
			return nil
		}
		var count int64
		for _, table := range []any{&db.P14AuditionRound{}, &db.P14AuditionMatch{}} {
			if err = tx.Model(table).Where("season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
		}
		var existing db.P14AuditionQualification
		err = tx.First(&existing, "season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Error
		if err == nil {
			_, err = s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, s.OriginalServerID())
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		qualified, err := s.p14AuditionQualificationBuild(ctx, tx, boundary, roster)
		if err != nil {
			return err
		}
		qualified.QualifiedAt = now.UnixMilli()
		qualified.RecoverySource = gate.AutoRecoverySource
		qualified.RecoveryPublishedAt = gate.AutoRecoveryPublishedAt
		if err = p14AuditionAutoRecoveryVerify(boundary, qualified); err != nil {
			return err
		}
		qualified.SourceHash = p14AuditionQualificationHash(qualified)
		return tx.Create(qualified).Error
	})
}
