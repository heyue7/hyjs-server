package service

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"time"
)

func (s *GameService) growth74PrepareTestCalendar(ctx context.Context, a *model.AdminPrincipal, opening int64) error {
	var profiles []db.Profile
	d := db.GetIns().WithContext(ctx)
	if e := d.Where("player IS NOT NULL AND game IS NOT NULL AND (server_id=? OR server_id=0)", s.OriginalServerID()).Order("id").Find(&profiles).Error; e != nil {
		return e
	}
	for _, row := range profiles {
		var character struct {
			Name string `json:"name"`
		}
		if row.Player == nil || json.Unmarshal([]byte(*row.Player), &character) != nil || character.Name == "" {
			continue
		}
		if e := updateProfileWithTx(ctx, row.Id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			if g.Run != nil {
				before := string(adminJSON(g.Run))
				if g.Run.Fight != nil && !g.Run.Fight.Settled {
					if e := s.CancelOriginalCopyReplay(ctx, tx, row.Id, g.Run.Fight.Start); e != nil {
						return e
					}
				}
				g.Run = nil
				if e := adminAudit(tx, a, "growth74.test.leave", row.Uid, before); e != nil {
					return e
				}
			}
			// Temporary test anchor only; real profile/account creation and reward receipts are retained.
			var old db.PlayerOpeningCalendar
			err := tx.First(&old, "player_id=?", row.Id).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			if old.StartAt == opening {
				return nil
			}
			if e := adminAudit(tx, a, "growth74.test.anchor", row.Uid, string(adminJSON(old))); e != nil {
				return e
			}
			next := db.PlayerOpeningCalendar{PlayerID: row.Id, StartAt: opening, CreatedAt: time.Now().UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}
			return tx.Save(&next).Error
		}); e != nil {
			return e
		}
	}
	return nil
}
