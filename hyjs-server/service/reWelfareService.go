package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strconv"
	"time"
)

// InitializeReRestaurant 初始化一次本服Re餐厅恢复期。
// 必须在后台日历接管后调用；原关闭期、历史回执及运营停用全部保留。
// Re仅证明每日窗口，活动总期采用本月；下个月由后台显式续排。
func (s *GameService) InitializeReRestaurant(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		serverID := s.OriginalServerID()
		guard, err := adminActivityScheduleRevision(tx, serverID, true)
		if err != nil {
			return err
		}
		occurrenceID := "restore_re1051_meal_" + strconv.FormatInt(serverID, 10)
		var previous db.AdminActivityOccurrence
		err = tx.First(&previous, "id=?", occurrenceID).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now()
		if gameSchemaHasTable(tx, &db.ServerGameClock{}) {
			var clock db.ServerGameClock
			if err := tx.First(&clock, "server_id=?", s.serverGameClockID()).Error; err != nil {
				return err
			}
			now = now.AddDate(0, 0, int(clock.OffsetDays))
		}
		zone := time.FixedZone("UTC+8", 8*3600)
		local := now.In(zone)
		start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone)
		end := start.AddDate(0, 1, 0)
		key := "p4:meal:admin_re1051_schedule_" + strconv.FormatInt(serverID, 10)
		sourceID := "welfare:" + key
		form := model.AdminActivityForm{Mode: "stamina-meal", TemplateID: "stamina-meal-original", Name: constUtil.ReRestaurantTemplateName, Source: constUtil.ReRestaurantSource, RepeatPolicy: "repeat", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Products: []model.AdminActivityProduct{}, Milestones: []model.AdminActivityMilestone{}, Prizes: []model.AdminActivityPrize{}}
		form.Options.Meals = []model.AdminMealWindow{{StartMinute: 660, EndMinute: 810, Amount: 180}, {StartMinute: 1020, EndMinute: 1170, Amount: 180}}
		// 已配置的当前餐厅优先；恢复期保留成关闭草稿，避免重叠活动抢原无ID请求。
		var active int64
		if err = tx.Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND mode=? AND enabled=? AND archived=? AND start_at<=? AND claim_end_at>?", serverID, form.Mode, true, false, now.UnixMilli(), now.UnixMilli()).Count(&active).Error; err != nil {
			return err
		}
		enabled := active == 0
		definitionForm := form
		definitionForm.StartAt = 0
		definitionForm.EndAt = 0
		definitionForm.ClaimEndAt = 0
		definition := db.AdminActivityDefinition{ID: "import_" + p6Digest([]byte(sourceID))[:24], Name: form.Name, Mode: form.Mode, FormJSON: adminJSON(definitionForm), DefaultDays: int64(end.Sub(start) / (24 * time.Hour)), Usable: true, Enabled: true, SourceActivityID: sourceID, Revision: 1, UpdatedAtMillis: time.Now().UnixMilli()}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&definition).Error; err != nil {
			return err
		}
		if err = tx.First(&definition, "id=?", definition.ID).Error; err != nil {
			return err
		}
		d, _ := adminActivityDefinitionFor(form.Mode)
		if err = s.adminPersistTypedActivity(tx, d, key, form, enabled, false); err != nil {
			return err
		}
		presentation := db.AdminActivityPresentation{ActivityID: sourceID, Mode: form.Mode, TemplateID: form.TemplateID, Name: form.Name, UpdatedAtMillis: time.Now().UnixMilli()}
		if err = tx.Create(&presentation).Error; err != nil {
			return err
		}
		occurrence := db.AdminActivityOccurrence{ID: occurrenceID, ServerID: serverID, TemplateID: definition.ID, TemplateVersion: adminVersion(definition), Name: form.Name, Mode: form.Mode, Timing: "calendar", ConsumerKey: key, ConsumerID: sourceID, StartAt: form.StartAt, EndAt: form.EndAt, ClaimEndAt: form.ClaimEndAt, Lane: 1, Enabled: enabled, FormJSON: adminJSON(form), Revision: 1}
		if err = tx.Create(&occurrence).Error; err != nil {
			return err
		}
		guard.Revision++
		if err = tx.Save(&guard).Error; err != nil {
			return err
		}
		return adminAudit(tx, &model.AdminPrincipal{Username: "system"}, "activity.re1051.restore", sourceID, "恢复Re两窗180体力；本月总期为本地策略，后续后台续排；旧配置与领取历史保持")
	})
}
