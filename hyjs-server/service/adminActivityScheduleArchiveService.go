package service

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminArchivedActivitySchedules reads soft-archived rows without reopening consumers.
func (s *GameService) AdminArchivedActivitySchedules(ctx context.Context, p *model.AdminPrincipal, serverID int64) ([]model.AdminActivityOccurrenceDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	if err := adminRequireActivityServer(tx, serverID); err != nil {
		return nil, err
	}
	var rows []db.AdminActivityOccurrence
	if err := tx.Where("server_id=? AND archived=?", serverID, true).Order("start_at DESC,id").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []model.AdminActivityOccurrenceDTO{}
	for _, row := range rows {
		dto, err := s.adminActivityOccurrenceDTO(tx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// AdminRestoreActivitySchedule restores visibility in a disabled state under the calendar lock.
func (s *GameService) AdminRestoreActivitySchedule(ctx context.Context, p *model.AdminPrincipal, serverID int64, id string, req model.AdminActivityScheduleRestoreRequest) (*model.AdminActivityScheduleResponse, error) {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := adminRequireActivityServer(tx, serverID); err != nil {
			return err
		}
		revision, err := adminActivityScheduleRevision(tx, serverID, true)
		if err != nil {
			return err
		}
		if req.Version != adminActivityScheduleVersion(revision) {
			return constUtil.ErrAdminConfigConflict
		}
		var row db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND server_id=? AND archived=?", id, serverID, true).Error; err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{"archived": false, "enabled": false, "revision": row.Revision + 1}).Error; err != nil {
			return err
		}
		revision.Revision++
		if err := tx.Save(&revision).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.schedule.restore", id, fmt.Sprintf("realm=%d restored disabled; history preserved", serverID))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminActivitySchedule(ctx, p, serverID)
}
