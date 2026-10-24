package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// InitializeAdminGameServers attaches existing records to the trusted configured runtime.
func (s *GameService) InitializeAdminGameServers(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.AdminGameServer{}); err != nil {
		return err
	}
	// Add only the ownership column; avoid rebuilding the sensitive profile table.
	fresh := !d.Migrator().HasColumn(&db.Profile{}, "server_id")
	if fresh {
		if err := d.Migrator().AddColumn(&db.Profile{}, "ServerID"); err != nil {
			return err
		}
	}
	return d.Transaction(func(tx *gorm.DB) error {
		id := s.OriginalServerID()
		// A legacy database without realm ownership belongs to its trusted
		// serving process. Existing explicit realm ownership remains unchanged.
		owners := tx.Model(&db.Profile{})
		if !fresh {
			owners = owners.Where("server_id IS NULL OR server_id=0")
		} else {
			owners = owners.Where("1=1")
		}
		if err := owners.Update("server_id", id).Error; err != nil {
			return err
		}
		// First registration of an already serving realm keeps its earliest
		// persisted character date. Empty realms begin at the real current time.
		// The conflict insert never rewrites a previously configured OpenAt.
		opened := time.Now().UnixMilli()
		var first db.Profile
		err := tx.Select("id", "created_at").Where("server_id=? AND player IS NOT NULL AND created_at>0", id).Order("created_at,id").First(&first).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && first.CreatedAt != nil {
			opened = *first.CreatedAt
		}
		row := db.AdminGameServer{ID: id, Name: fmt.Sprintf("%d区·灵魂之花", id), Enabled: true, OpenAt: opened, RuntimeBound: true, Revision: 1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.AdminGameServer{}).Where("id=?", id).Update("runtime_bound", true).Error; err != nil {
			return err
		}
		return nil
	})
}
func adminGameServerDTO(r db.AdminGameServer) model.AdminGameServerDTO {
	return model.AdminGameServerDTO{ID: r.ID, Name: r.Name, Enabled: r.Enabled, OpenAt: r.OpenAt, RuntimeBound: r.RuntimeBound, Version: adminDigest(string(adminJSON(struct {
		Row      db.AdminGameServer
		Revision int64
	}{r, r.Revision})))}
}

// AdminRequireGameServer rejects unknown realm selections.
func AdminRequireGameServer(ctx context.Context, id int64) error {
	if id < 1 {
		return constUtil.ErrAdminInput
	}
	var r db.AdminGameServer
	if err := db.GetIns().WithContext(ctx).First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constUtil.ErrAdminInput
		}
		return err
	}
	return nil
}

// AdminProfileServerID preserves legacy profile ownership in isolated compatibility tests.
func AdminProfileServerID(row db.Profile) int64 {
	if row.ServerID > 0 {
		return row.ServerID
	}
	return constUtil.GameDefaultServerID
}

// AdminSelectedServerID returns the authenticated operator's selected realm, or zero for legacy callers.
func AdminSelectedServerID(ctx context.Context) int64 {
	id, _ := ctx.Value(constUtil.AdminServerScopeContextKey).(int64)
	return id
}

// AdminCheckProfileScope prevents selected-realm deep links from reading another realm's player.
func AdminCheckProfileScope(ctx context.Context, row db.Profile) error {
	if id := AdminSelectedServerID(ctx); id > 0 && id != AdminProfileServerID(row) {
		return constUtil.ErrAdminPermission
	}
	return nil
}

// AdminGameServers lists real and planned realms for the navigation selector.
func AdminGameServers(ctx context.Context) (*model.AdminGameServersResponse, error) {
	var rows []db.AdminGameServer
	if err := db.GetIns().WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &model.AdminGameServersResponse{Items: []model.AdminGameServerDTO{}}
	for _, r := range rows {
		out.Items = append(out.Items, adminGameServerDTO(r))
	}
	return out, nil
}

// AdminCreateGameServer creates disabled metadata without claiming a game process was provisioned.
func AdminCreateGameServer(ctx context.Context, p *model.AdminPrincipal, req model.AdminGameServerCreateRequest) (*model.AdminGameServerDTO, error) {
	row := db.AdminGameServer{Name: req.Name, OpenAt: req.OpenAt, Revision: 1}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// A newly planned realm starts with an explicit empty calendar; legacy running realms retain migration compatibility.
		if gameSchemaHasTable(tx, &db.AdminActivityScheduleRevision{}) {
			if err := tx.Create(&db.AdminActivityScheduleRevision{ServerID: row.ID, Revision: 1}).Error; err != nil {
				return err
			}
		}
		return adminAudit(tx, p, "server.create", fmt.Sprint(row.ID), req.Name)
	})
	if err != nil {
		return nil, err
	}
	out := adminGameServerDTO(row)
	return &out, nil
}

// AdminUpdateGameServer preserves immutable realm identity and records every successful edit.
func AdminUpdateGameServer(ctx context.Context, p *model.AdminPrincipal, id int64, req model.AdminGameServerUpdateRequest) (*model.AdminGameServerDTO, error) {
	var row db.AdminGameServer
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if adminGameServerDTO(row).Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Enabled && !row.RuntimeBound {
			return constUtil.ErrAdminConfiguration
		}
		row.Name = req.Name
		row.OpenAt = req.OpenAt
		row.Enabled = req.Enabled
		row.Revision++
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "server.update", fmt.Sprint(id), req.Name)
	})
	if err != nil {
		return nil, err
	}
	out := adminGameServerDTO(row)
	return &out, nil
}

// runtimeGameServer uses the process identity; user-provided zone IDs never select another database.
func (s *GameService) runtimeGameServer(ctx context.Context) (*db.AdminGameServer, error) {
	d := db.GetIns().WithContext(ctx)
	if !gameSchemaHasTable(d, &db.AdminGameServer{}) {
		return &db.AdminGameServer{ID: s.OriginalServerID(), Name: "灵魂之花", Enabled: true, RuntimeBound: true}, nil
	}
	var row db.AdminGameServer
	if err := d.First(&row, s.OriginalServerID()).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// AdminCheckPlayerIDScope guards auxiliary player operations using the same immutable ownership.
func AdminCheckPlayerIDScope(ctx context.Context, id int64) error {
	if AdminSelectedServerID(ctx) == 0 {
		return nil
	}
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).Select("id,server_id").First(&row, id).Error; err != nil {
		return err
	}
	return AdminCheckProfileScope(ctx, row)
}
