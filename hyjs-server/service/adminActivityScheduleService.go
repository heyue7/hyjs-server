package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeAdminActivitySchedules imports legacy forms once as global reusable definitions.
// It never enables a draft or alters an issued gameplay instance.
func (s *GameService) InitializeAdminActivitySchedules(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminActivityPattern{}, &db.AdminActivityScheduleRevision{}, &db.AdminCharacterActivityInstance{}, &db.AdminActivityTemplateOwner{}, &db.AdminOpeningState{}, &db.AdminOpeningCounter{}, &db.AdminOpeningStock{}, &db.AdminOpeningReceipt{}); err != nil {
		return err
	}
	rows, err := s.AdminActivityInstances(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if strings.Contains(row.ID, "_schedule_") || strings.Contains(row.ID, "_character_") || adminActivityRedundantSource(row.ID) {
			continue
		}
		var mapped int64
		if err = tx.Model(&db.AdminActivityDefinition{}).Where("source_activity_id=?", row.ID).Count(&mapped).Error; err != nil {
			return err
		}
		if mapped > 0 {
			continue
		}
		id := "import_" + p6Digest([]byte(row.ID))[:24]
		days := (row.Form.EndAt - row.Form.StartAt + 86399999) / 86400000
		if days < 1 || days > 366 {
			days = 14
		}
		grace := (row.Form.ClaimEndAt - row.Form.EndAt + 86399999) / 86400000
		if grace < 0 || grace > 90 {
			grace = 0
		}
		form := row.Form
		form.StartAt = 0
		form.EndAt = 0
		form.ClaimEndAt = 0
		value := db.AdminActivityDefinition{ID: id, Name: row.Name, Mode: row.Mode, FormJSON: adminJSON(form), DefaultDays: days, ClaimDays: grace, Enabled: row.Enabled && row.Availability.Usable, Usable: row.Availability.Usable, UnavailableReason: row.Availability.Reason, SourceActivityID: row.ID, Revision: 1, UpdatedAtMillis: time.Now().UnixMilli()}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&value).Error; err != nil {
			return err
		}
	}
	opening := db.AdminActivityDefinition{ID: "original_opening_14", Name: "开服14天原版任务", Mode: "opening-14", FormJSON: adminJSON(model.AdminActivityForm{Mode: "opening-14", TemplateID: "opening-14-original", Name: "开服14天原版任务", Source: "APK原249项开服任务；每角色第1天锚点与原领取记录保持"}), DefaultDays: 14, ClaimDays: 1, Enabled: true, Usable: true, Revision: 1, UpdatedAtMillis: time.Now().UnixMilli()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&opening).Error; err != nil {
		return err
	}
	pattern := db.AdminActivityPattern{ID: "original_opening_14", Name: "开服14天 · 原版开服任务", CycleDays: 15, EntriesJSON: adminJSON([]model.AdminActivityPatternEntry{{TemplateID: opening.ID, StartDay: 0, DurationDays: 14, ClaimDays: 1, Lane: 0}}), Revision: 1, UpdatedAtMillis: time.Now().UnixMilli()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&pattern).Error; err != nil {
		return err
	}
	return s.reconcileAdminActivityCatalogue(ctx, rows)
}

// adminActivityRedundantSource identifies generated aliases, never operator
// template names or business history. Ordinary shops have their own real menu
// and shop-window adapter; basic recruitment remains in its original shop.
func adminActivityRedundantSource(id string) bool {
	if strings.Contains(id, "lp-") || id == "pool:recruit_1" || id == "pool:recruit_2" {
		return true
	}
	// GGO exchanges own their player menu. Generated commerce aliases must
	// never be imported or disabled by the legacy activity catalogue cleanup.
	if strings.HasPrefix(id, "commerce:"+constUtil.GGOShopKey+":") {
		return true
	}
	if !strings.HasPrefix(id, "commerce:p4:") {
		return false
	}
	key, _, ok := strings.Cut(strings.TrimPrefix(id, "commerce:p4:"), ":")
	return ok && adminOrdinarySchedulableShop(key)
}

// reconcileAdminActivityCatalogue archives obsolete automatic aliases and
// upgrades newly supported untouched imports. Issued forms, consumers, claims,
// repeat owners and source rows are never changed by this catalogue migration.
func (s *GameService) reconcileAdminActivityCatalogue(ctx context.Context, sources []model.AdminActivityInstance) error {
	current := map[string]model.AdminActivityInstance{}
	for _, source := range sources {
		current[source.ID] = source
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var definitions []db.AdminActivityDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("archived=?", false).Find(&definitions).Error; err != nil {
			return err
		}
		for _, row := range definitions {
			if row.SourceActivityID == "" || row.ID != "import_"+p6Digest([]byte(row.SourceActivityID))[:24] {
				continue
			}
			var edits int64
			if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", "activity.template.save", row.ID).Count(&edits).Error; err != nil {
				return err
			}
			if edits != 0 {
				continue
			}
			if adminActivityRedundantSource(row.SourceActivityID) {
				if err := tx.Model(&row).Updates(map[string]any{"archived": true, "revision": row.Revision + 1, "updated_at_millis": time.Now().UnixMilli()}).Error; err != nil {
					return err
				}
				continue
			}
			source, ok := current[row.SourceActivityID]
			if !ok || row.Mode != "unsupported" || source.Mode == "unsupported" {
				continue
			}
			form := source.Form
			form.StartAt, form.EndAt, form.ClaimEndAt = 0, 0, 0
			form.Name = row.Name
			row.Mode, row.FormJSON = source.Mode, adminJSON(form)
			row.Usable, row.UnavailableReason = source.Availability.Usable, source.Availability.Reason
			row.Enabled = source.Enabled && row.Usable
			row.Revision++
			row.UpdatedAtMillis = time.Now().UnixMilli()
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// adminActivityPresentationForm reflects the existing treasure consumer's coin projection.
// Reads preserve the stored template, issued snapshot and optimistic version.
func adminActivityPresentationForm(form model.AdminActivityForm) model.AdminActivityForm {
	form = adminActivityCanonicalForm(form)
	if form.Mode == "fortune-cat" {
		form.Milestones = append([]model.AdminActivityMilestone(nil), form.Milestones...)
		for i, milestone := range form.Milestones {
			if len(milestone.Rewards) == 1 && milestone.Rewards[0].ResourceID == "diamond" {
				form.Milestones[i].Rewards = append([]model.AdminResourceAmount(nil), milestone.Rewards...)
				form.Milestones[i].Rewards[0].ResourceID = "coin"
			}
		}
	}
	return form
}

func adminActivityDefinitionDTO(row db.AdminActivityDefinition) (model.AdminActivityDefinitionDTO, error) {
	var form model.AdminActivityForm
	if json.Unmarshal(row.FormJSON, &form) != nil {
		return model.AdminActivityDefinitionDTO{}, constUtil.ErrAdminConfiguration
	}
	if form.Name == "" {
		form.Name = row.Name
	}
	form = adminActivityPresentationForm(form)
	return model.AdminActivityDefinitionDTO{ID: row.ID, Name: form.Name, Mode: row.Mode, Form: form, DefaultDays: row.DefaultDays, ClaimDays: row.ClaimDays, Enabled: row.Enabled, Version: adminVersion(row), Availability: model.AdminActivityAvailability{Usable: row.Usable, Reason: row.UnavailableReason, Missing: []string{}}, SourceActivityID: row.SourceActivityID}, nil
}

// AdminActivityDefinitions reads globally reusable forms. No original consumer is switched by a GET.
func (s *GameService) AdminActivityDefinitions(ctx context.Context, p *model.AdminPrincipal) ([]model.AdminActivityDefinitionDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	if err := s.syncAdminShopScheduleDefinitions(tx); err != nil {
		return nil, err
	}
	var rows []db.AdminActivityDefinition
	if err := tx.Where("archived=?", false).Order("name,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []model.AdminActivityDefinitionDTO{}
	for _, row := range rows {
		dto, err := adminActivityDefinitionDTO(row)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// adminValidateDefinition reuses the real original adapters in a rolled-back savepoint.
// Resource validation stays identical to gameplay while global templates issue no live instance.
func (s *GameService) adminValidateDefinition(tx *gorm.DB, f model.AdminActivityForm, complete bool) error {
	if f.RepeatPolicy != "" && f.RepeatPolicy != "once" && f.RepeatPolicy != "repeat" {
		return constUtil.ErrAdminInput
	}
	if f.Mode == "opening-14" {
		return nil
	}
	if f.Mode == "shop-window" {
		if f.Options.ShopKey == "" || f.Options.ShopSource == "" {
			return constUtil.ErrAdminInput
		}
		if complete {
			availability, err := AdminShopAvailability(tx, f.Options.ShopKey)
			if err != nil {
				return err
			}
			if availability == nil || availability.Unlimited {
				return constUtil.ErrAdminInput
			}
		}
	}
	definition, ok := adminActivityDefinitionFor(f.Mode)
	if !ok || f.TemplateID != definition.mode+"-original" {
		return constUtil.ErrAdminInput
	}
	f.StartAt = 946684800000
	f.EndAt = f.StartAt + 14*86400000
	f.ClaimEndAt = f.EndAt + 86400000
	// 藏宝箱结束即停止领取；模板校验使用同一时间规则，避免临时宽限拒绝合法奖励。
	if f.Mode == "fortune-cat" {
		f.ClaimEndAt = f.EndAt
	}
	if err := tx.SavePoint("activity_definition_validate").Error; err != nil {
		return err
	}
	key := definition.prefix + "check_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	result := s.adminPersistTypedActivity(tx, definition, key, f, complete, false)
	if err := tx.RollbackTo("activity_definition_validate").Error; err != nil {
		return err
	}
	return result
}

// AdminSaveActivityDefinition 保存模板；藏宝箱数值同时更新尚未结束的对应排期。
func (s *GameService) AdminSaveActivityDefinition(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminActivityDefinitionSaveRequest) (*model.AdminActivityDefinitionDTO, error) {
	var out model.AdminActivityDefinitionDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		// 排期保存先锁日历再锁模板，模板同步沿用同一锁序。
		if req.Form.Mode == "fortune-cat" && id != "" {
			if err := adminLockTreasureCalendars(tx); err != nil {
				return err
			}
		}
		var row db.AdminActivityDefinition
		if id != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND archived=?", id, false).Error; err != nil {
				return err
			}
			if req.Version != adminVersion(row) {
				return constUtil.ErrAdminConfigConflict
			}
		}
		if id == "" {
			row.ID = "template_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
		}
		if req.RecruitSettings != nil {
			kind := int32(0)
			if req.Form.Mode == "king-recruit" {
				kind = constUtil.RecruitRuntimeKindKing
			}
			if err := s.adminSaveActivityRecruitSettings(tx, p, kind, req.RecruitSettings); err != nil {
				return err
			}
		}
		form := req.Form
		if form.Mode == "shop-window" {
			canonical, err := s.adminShopScheduleSourceForm(tx, form.Options.ShopKey)
			if err != nil {
				return err
			}
			canonical.RepeatPolicy = form.RepeatPolicy
			canonical.Alias = form.Alias
			form = canonical
		}
		if form.Mode == "king-recruit" {
			canonical, err := s.adminKingRecruitCanonicalForm(tx, form)
			if err != nil {
				return err
			}
			form = canonical
		}
		form.RepeatPolicy = adminActivityRepeatPolicy(form)
		form.Name = req.Name
		form = adminActivityCanonicalForm(form)
		form.StartAt = 0
		form.EndAt = 0
		form.ClaimEndAt = 0
		if err := s.adminValidateDefinition(tx, form, false); err != nil {
			return err
		}
		validation := s.adminValidateDefinition(tx, form, true)
		if req.Enabled && validation != nil {
			return validation
		}
		row.Name = form.Name
		row.Mode = form.Mode
		row.FormJSON = adminJSON(form)
		row.DefaultDays = req.DefaultDays
		row.ClaimDays = req.ClaimDays
		row.Enabled = req.Enabled
		row.Usable = validation == nil
		row.UnavailableReason = ""
		if validation != nil {
			row.UnavailableReason = "配置未齐全，请补齐奖励或当前模式规则"
		}
		row.Revision++
		row.UpdatedAtMillis = time.Now().UnixMilli()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if row.Mode == "fortune-cat" && row.Usable {
			if err := s.adminSyncTreasureTemplate(tx, row, form, time.Now()); err != nil {
				return err
			}
		}
		var err error
		out, err = adminActivityDefinitionDTO(row)
		if err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.template.save", row.ID, "global template revision="+fmt.Sprint(row.Revision))
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// adminActivityScheduleAccess enforces the trusted serving realm before any issued consumer loads.
// Legacy and personal-cycle identifiers retain their existing access rules.
func adminActivityCalendarManaged(tx *gorm.DB) (bool, error) {
	service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService)
	if !ok || !tx.Migrator().HasTable(&db.AdminActivityScheduleRevision{}) {
		return false, nil
	}
	var revision db.AdminActivityScheduleRevision
	err := tx.First(&revision, "server_id=?", service.OriginalServerID()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return revision.Revision > 0, err
}

func adminActivityScheduledLegacyMode(key string) string {
	for _, entry := range []struct{ prefix, mode string }{
		{"p4:login:", "login-rewards"}, {"p4:limit:", "limited-sign"}, {"p4:meal:", "stamina-meal"}, {"p4:rebate:", "diamond-rebate"}, {"p4:cat:", "treasure-box"},
		{"halfDiscountSale_", "half-price"}, {"discountSale_", "discount-shop"}, {"limitExchange_", "exchange"}, {"limitSale_", "limited-shop"}, {"mysticDiscount_", "mystic-shop"},
		{"rechargeTotal_", "recharge-total"}, {"costTotal_", "diamond-spent"}, {"warriorRescruitTimes_", "recruit-count"}, {"mainCopyWinTimes_", "copy-win"}, {"totalLogin_", "login-total"},
		{"pvpWinTimes_", "arena-wins"}, {"robTreasureTimes_", "plunder-count"}, {"warriorGetSpecial_", "hero-collection"}, {"aincradStars_", "trial-stars"}, {"kaiFuJingSai_", "power-race"},
	} {
		if strings.HasPrefix(key, entry.prefix) {
			return entry.mode
		}
	}
	return ""
}

func adminActivityScheduleAccess(tx *gorm.DB, key string) (bool, error) {
	if localPlayPrivateCycleID(key) != "" {
		return true, nil
	}
	if !tx.Migrator().HasTable(&db.AdminActivityOccurrence{}) {
		return !strings.Contains(key, "_schedule_"), nil
	}
	// Adopted legacy consumers retain their original IDs so existing claims and
	// personal wheel receipts continue to resolve under the realm calendar.
	if strings.Contains(key, "_character_") {
		return adminCharacterActivityAccess(tx, key)
	}
	var row db.AdminActivityOccurrence
	err := tx.Where("consumer_key=?", key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && !strings.Contains(key, "_schedule_") {
		if managed, allowed, lookupErr := adminNativePaymentAccess(tx, key); lookupErr != nil || managed {
			return allowed, lookupErr
		}
		if adminActivityScheduledLegacyMode(key) == "" {
			return true, nil
		}
		managed, lookupErr := adminActivityCalendarManaged(tx)
		return !managed, lookupErr
	}
	if err != nil {
		return false, err
	}
	service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService)
	if !ok {
		return false, nil
	}
	if row.ServerID != service.OriginalServerID() || row.Archived || !row.Enabled {
		return false, nil
	}
	owner := GameCalendarOwner(tx.Statement.Context)
	now := LocalPlayNow(tx.Statement.Context).UnixMilli()
	if owner > 0 && now >= row.StartAt && now < row.ClaimEndAt {
		return adminAcceptActivityTemplate(tx, owner, row)
	}
	return true, nil
}

// AdminScheduledTurntablePool selects only this trusted realm's active pool.
// A player's already-created personal wheel remains frozen by the caller.
func (s *GameService) AdminScheduledTurntablePool(tx *gorm.DB, now int64, fallback string) (string, error) {
	if !tx.Migrator().HasTable(&db.AdminActivityOccurrence{}) {
		return fallback, nil
	}
	if key, err := adminCharacterActivityCurrentKey(tx, "turntable", now, false); err != nil || key != "" {
		return key, err
	}
	var rows []db.AdminActivityOccurrence
	if err := tx.Where("server_id=? AND mode=? AND timing<>? AND archived=? AND enabled=? AND start_at<=? AND end_at>?", s.OriginalServerID(), "turntable", "character-day", false, true, now, now).Order("start_at DESC,id").Limit(2).Find(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		contextTx := tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameServiceContextKey, s))
		managed, err := adminActivityCalendarManaged(contextTx)
		if err != nil {
			return "", err
		}
		if managed {
			return "", nil
		}
		return fallback, nil
	}
	if len(rows) > 1 {
		return "", constUtil.ErrAdminConfiguration
	}
	return rows[0].ConsumerKey, nil
}

// AdminScheduledOpeningRule gates only the realm opening entrance; per-character day one and receipts stay intact.
func AdminScheduledOpeningRule(tx *gorm.DB, cfg *db.OpenServerActivityRule, now int64, playerIDs ...int64) error {
	// The authenticated deployment's realm date controls the fourteen unlock
	// tiers. It never replaces the already-projected role participation age.
	if service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && tx.Migrator().HasTable(&db.AdminGameServer{}) {
		var realm db.AdminGameServer
		if err := tx.First(&realm, "id=?", service.OriginalServerID()).Error; err != nil {
			return err
		}
		if realm.OpenAt <= 0 {
			return constUtil.ErrAdminConfiguration
		}
		zone := time.FixedZone("UTC+8", 8*3600)
		local := time.UnixMilli(realm.OpenAt).In(zone)
		cfg.UnlockStartAt = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone).UnixMilli()
	}
	if !tx.Migrator().HasTable(&db.AdminActivityOccurrence{}) {
		return nil
	}
	service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService)
	if !ok {
		return nil
	}
	var rows []db.AdminActivityOccurrence
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("server_id=? AND mode=? AND archived=?", service.OriginalServerID(), "opening-14", false).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		managed, err := adminActivityCalendarManaged(tx)
		if err != nil {
			return err
		}
		if managed {
			cfg.Enabled = false
		}
		return nil
	}
	playerID := int64(0)
	if len(playerIDs) > 0 {
		playerID = playerIDs[0]
	}
	return adminResolveOpeningSchedule(tx, cfg, now, playerID, rows)
}

func (s *GameService) adminActivityOccurrenceFrozen(tx *gorm.DB, row db.AdminActivityOccurrence) (bool, error) {
	if adminShopGateOccurrence(row) {
		return true, nil
	}
	if adminActivityTiming(row) == "character-day" && row.Mode != "opening-14" {
		if !tx.Migrator().HasTable(&db.AdminCharacterActivityInstance{}) {
			return false, nil
		}
		var issued []db.AdminCharacterActivityInstance
		err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("player_id", "occurrence_id").Where("occurrence_id=?", row.ID).Limit(1).Find(&issued).Error
		return len(issued) > 0, err
	}
	if row.Mode == "opening-14" {
		// Relative opening rules have no meaningful absolute start date. A
		// published programme fixes its timing; operators may still disable it.
		return true, nil
	}
	// Use a current read after consumer locking. A prior permission SELECT may already
	// have established an older InnoDB repeatable-read snapshot in this transaction.
	_, frozen, err := adminActivityRaw(tx.Clauses(clause.Locking{Strength: "SHARE"}), row.ConsumerID)
	if err == nil && row.Mode == "turntable" && !frozen && tx.Migrator().HasTable(&db.TurntableState{}) {
		var state []db.TurntableState
		if err = tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("pool_id", "player_id").Where("pool_id=?", row.ConsumerKey).Limit(1).Find(&state).Error; err == nil {
			frozen = len(state) > 0
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, constUtil.ErrAdminConfiguration
	}
	return frozen, err
}
func (s *GameService) adminActivityOccurrenceDTO(tx *gorm.DB, row db.AdminActivityOccurrence) (model.AdminActivityOccurrenceDTO, error) {
	frozen, err := s.adminActivityOccurrenceFrozen(tx, row)
	var form model.AdminActivityForm
	if json.Unmarshal(row.FormJSON, &form) != nil {
		return model.AdminActivityOccurrenceDTO{}, constUtil.ErrAdminConfiguration
	}
	timing := adminActivityTiming(row)
	startDay, durationDays, claimDays := adminCharacterScheduleDays(row)
	form = adminActivityPresentationForm(form)
	return model.AdminActivityOccurrenceDTO{Timing: timing, StartDay: startDay, DurationDays: durationDays, ClaimDays: claimDays, Form: form, ID: row.ID, TemplateID: row.TemplateID, TemplateVersion: row.TemplateVersion, Name: form.Name, Mode: row.Mode, StartAt: row.StartAt, EndAt: row.EndAt, ClaimEndAt: row.ClaimEndAt, Lane: row.Lane, Enabled: row.Enabled, Frozen: frozen, Version: adminVersion(row), ConsumerID: row.ConsumerID}, err
}
func adminRequireActivityServer(tx *gorm.DB, id int64) error {
	if !tx.Migrator().HasTable("admin_game_servers") {
		return constUtil.ErrAdminConfiguration
	}
	var count int64
	if err := tx.Table("admin_game_servers").Where("id=?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return constUtil.ErrAdminInput
	}
	return nil
}
func adminActivityScheduleVersion(row db.AdminActivityScheduleRevision) string {
	return adminVersion(row)
}
func adminActivityScheduleRevision(tx *gorm.DB, serverID int64, lock bool) (db.AdminActivityScheduleRevision, error) {
	var row db.AdminActivityScheduleRevision
	query := tx.Session(&gorm.Session{})
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&row, "server_id=?", serverID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.AdminActivityScheduleRevision{ServerID: serverID}, nil
	}
	return row, err
}

// AdminActivitySchedule returns the full calendar, including dates outside the visible viewport.
func (s *GameService) AdminActivitySchedule(ctx context.Context, p *model.AdminPrincipal, serverID int64) (*model.AdminActivityScheduleResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	if err := adminRequireActivityServer(tx, serverID); err != nil {
		return nil, err
	}
	revision, err := adminActivityScheduleRevision(tx, serverID, false)
	if err != nil {
		return nil, err
	}
	var rows []db.AdminActivityOccurrence
	if err = tx.Where("server_id=? AND archived=?", serverID, false).Order("start_at,lane,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	now, err := s.adminArenaPrizeNow(tx, false)
	if err != nil {
		return nil, err
	}
	out := &model.AdminActivityScheduleResponse{GameTimeMs: now.UnixMilli(), ServerID: serverID, Managed: revision.Revision > 0, Version: adminActivityScheduleVersion(revision), Items: []model.AdminActivityOccurrenceDTO{}, Warnings: []string{}}
	for _, row := range rows {
		dto, err := s.adminActivityOccurrenceDTO(tx, row)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, dto)
	}
	// Surface finite published windows that have no planned continuation. There
	// is no authenticated original server rotation to invent a renewal from.
	for _, row := range rows {
		if adminActivityTiming(row) == "character-day" || !row.Enabled || row.StartAt > now.UnixMilli() || row.ClaimEndAt <= now.UnixMilli() {
			continue
		}
		next := false
		for _, future := range rows {
			if future.Enabled && future.Mode == row.Mode && future.StartAt >= row.EndAt {
				next = true
				break
			}
		}
		if !next && len(out.Warnings) < 3 {
			out.Warnings = append(out.Warnings, "“"+row.Name+"”结束后尚未安排下一期。")
		}
	}
	return out, nil
}

func adminActivityModeSingleton(mode string) bool {
	return strings.HasPrefix(mode, "king-recruit:") || strings.HasPrefix(mode, "shop-window:") || mode == "opening-14" || mode == "turntable" || mode == "king-recruit" || mode == "plunder-count" || mode == "login-rewards" || mode == "limited-sign" || mode == "stamina-meal" || mode == "diamond-rebate" || mode == "treasure-box" || mode == "mystic-shop"
}

// Explicit worlds may run together; a legacy rotating pool occupies every world.
func adminActivityModesConflict(a, b string) bool {
	if a == b {
		return true
	}
	if strings.HasPrefix(a, "king-recruit:") && strings.HasPrefix(b, "king-recruit:") {
		return a == "king-recruit:0" || b == "king-recruit:0"
	}
	return false
}

func (s *GameService) adminActivityScheduleCheck(tx *gorm.DB, serverID int64, version string, items []model.AdminActivityOccurrenceDTO) (*model.AdminActivitySchedulePreviewResponse, map[string]db.AdminActivityOccurrence, error) {
	if err := adminRequireActivityServer(tx, serverID); err != nil {
		return nil, nil, err
	}
	revision, err := adminActivityScheduleRevision(tx, serverID, false)
	if err != nil {
		return nil, nil, err
	}
	if version != adminActivityScheduleVersion(revision) {
		return nil, nil, constUtil.ErrAdminConfigConflict
	}
	var rows []db.AdminActivityOccurrence
	if err = tx.Where("server_id=? AND archived=?", serverID, false).Order("id").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	existing := map[string]db.AdminActivityOccurrence{}
	for _, row := range rows {
		existing[row.ID] = row
	}
	out := &model.AdminActivitySchedulePreviewResponse{Valid: true, Conflicts: []model.AdminActivityScheduleConflict{}, Warnings: []string{}}
	add := func(i, j int, msg string) {
		out.Valid = false
		out.Conflicts = append(out.Conflicts, model.AdminActivityScheduleConflict{Index: int64(i), OtherIndex: int64(j), Message: msg})
	}
	seen := map[string]bool{}
	modes := make([]string, len(items))
	for i, item := range items {
		var definition db.AdminActivityDefinition
		query := tx.Where("id=?", item.TemplateID)
		if item.ID == "" {
			query = query.Where("archived=?", false)
		}
		if err = query.First(&definition).Error; err != nil {
			return nil, nil, err
		}
		modes[i] = definition.Mode
		if definition.Mode == constUtil.AdminKingRecruitMode {
			var f model.AdminActivityForm
			if json.Unmarshal(definition.FormJSON, &f) != nil {
				return nil, nil, constUtil.ErrAdminConfiguration
			}
			modes[i] = fmt.Sprintf("king-recruit:%d", f.Options.KingWorld)
		}
		if definition.Mode == "shop-window" {
			var f model.AdminActivityForm
			if json.Unmarshal(definition.FormJSON, &f) != nil {
				return nil, nil, constUtil.ErrAdminConfiguration
			}
			modes[i] = "shop-window:" + f.Options.ShopKey
		}
		if definition.Mode == "opening-14" {
			item.Mode = definition.Mode
			item.Timing = "character-day"
			if item.StartDay == 0 {
				item.StartDay = 1
			}
			if item.DurationDays == 0 {
				item.DurationDays = 14
				item.ClaimDays = 1
			}
			items[i] = item
		}
		if item.Timing == "character-day" || definition.Mode == "opening-14" {
			if item.StartDay < 1 || item.StartDay > 366 || item.DurationDays < 1 || item.DurationDays > 366 || item.ClaimDays < 0 || item.ClaimDays > 90 {
				add(i, -1, "请设置有效的角色起始日、持续天数和领奖宽限")
			}

		}
		if item.ID == "" && item.TemplateVersion != adminVersion(definition) {
			return nil, nil, constUtil.ErrAdminConfigConflict
		}
		if item.ID == "" && (!definition.Usable || !definition.Enabled) {
			add(i, -1, "此全局活动模板未齐全或已停用，不能发布排期")
		}
		if item.ID != "" {
			row, ok := existing[item.ID]
			if !ok || seen[item.ID] {
				return nil, nil, constUtil.ErrAdminInput
			}
			seen[item.ID] = true
			if item.Version != adminVersion(row) {
				return nil, nil, constUtil.ErrAdminConfigConflict
			}
			modes[i] = row.Mode
			if row.Mode == constUtil.AdminKingRecruitMode {
				var f model.AdminActivityForm
				if json.Unmarshal(row.FormJSON, &f) != nil {
					return nil, nil, constUtil.ErrAdminConfiguration
				}
				modes[i] = fmt.Sprintf("king-recruit:%d", f.Options.KingWorld)
			}
			if row.Mode == "shop-window" {
				var f model.AdminActivityForm
				if json.Unmarshal(row.FormJSON, &f) != nil {
					return nil, nil, constUtil.ErrAdminConfiguration
				}
				modes[i] = "shop-window:" + f.Options.ShopKey
			}
			if item.TemplateID != row.TemplateID {
				add(i, -1, "已有排期不能切换模板，请另建一期")
			}
			frozen, e := s.adminActivityOccurrenceFrozen(tx, row)
			if e != nil {
				return nil, nil, e
			}
			timing := adminActivityTiming(row)
			start, duration, grace := adminCharacterScheduleDays(row)
			if item.Timing != timing {
				add(i, -1, "已有排期不能切换时间类型，请新建排期")
			}
			if frozen && (timing == "calendar" && (item.StartAt != row.StartAt || item.EndAt != row.EndAt || item.ClaimEndAt != row.ClaimEndAt) || timing == "character-day" && (item.StartDay != start || item.DurationDays != duration || item.ClaimDays != grace)) {
				add(i, -1, "活动规则已冻结，保留时间；可复制为新一期")
			}
		}
	}
	for id, row := range existing {
		if !seen[id] {
			frozen, e := s.adminActivityOccurrenceFrozen(tx, row)
			if e != nil {
				return nil, nil, e
			}
			// 停用后允许归档排期入口，冻结规则和全部参与回执仍留在原表。
			if frozen && row.Enabled {
				add(-1, -1, "已冻结活动“"+row.Name+"”请先停用，再从排期归档")
			}
		}
	}
	for i, a := range items {
		if !a.Enabled || !adminActivityModeSingleton(modes[i]) {
			continue
		}
		for j := 0; j < i; j++ {
			b := items[j]
			if b.Enabled && adminActivityModesConflict(modes[j], modes[i]) && adminActivityWindowsOverlap(a, b, modes[i]) {
				add(i, j, "此模式原客户端没有独立实例入口，活动及领奖窗口不能重叠")
			}
		}
	}
	return out, existing, nil
}

// AdminPreviewActivitySchedule checks frozen state and original UI overlap limitations without changing rows.
func (s *GameService) AdminPreviewActivitySchedule(ctx context.Context, p *model.AdminPrincipal, serverID int64, req model.AdminActivitySchedulePreviewRequest) (*model.AdminActivitySchedulePreviewResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	req.Items = adminNormalizeCharacterItems(req.Items)
	out, _, err := s.adminActivityScheduleCheck(tx, serverID, req.Version, req.Items)
	return out, err
}

// AdminSaveActivitySchedule publishes one realm's changes and original consumers in a single transaction.
func (s *GameService) AdminSaveActivitySchedule(ctx context.Context, p *model.AdminPrincipal, serverID int64, req model.AdminActivityScheduleSaveRequest) (*model.AdminActivityScheduleResponse, error) {
	req.Items = adminNormalizeCharacterItems(req.Items)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		// Creating the guard before locking serializes two first-time publishers on MySQL too.
		guard := db.AdminActivityScheduleRevision{ServerID: serverID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; err != nil {
			return err
		}
		revision, err := adminActivityScheduleRevision(tx, serverID, true)
		if err != nil {
			return err
		}
		// Compare against the locked current row before any ordinary snapshot read.
		// InnoDB may retain a pre-lock snapshot from the permission lookup; that old
		// view cannot authorize a second publisher after the first commits.
		if req.Version != adminActivityScheduleVersion(revision) {
			return constUtil.ErrAdminConfigConflict
		}
		// Lock original consumer rows before inspecting Frozen; gameplay freezes acquire these same rows.
		// Period windows keep their pre-existing ascending-primary lock order.
		if s.P1Ready {
			if _, e := p1LockPeriods(tx); e != nil {
				return e
			}
		}
		var lockedRows []db.AdminActivityOccurrence
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND archived=?", serverID, false).Order("id").Find(&lockedRows).Error; e != nil {
			return e
		}
		for _, row := range lockedRows {
			if adminActivityTiming(row) != "character-day" && !adminShopGateOccurrence(row) {
				if _, _, e := adminActivityRaw(tx.Clauses(clause.Locking{Strength: "UPDATE"}), row.ConsumerID); e != nil {
					return e
				}
			}
		}
		check, existing, err := s.adminActivityScheduleCheck(tx, serverID, req.Version, req.Items)
		if err != nil {
			return err
		}
		if !check.Valid {
			return fmt.Errorf("%w: %s", constUtil.ErrAdminConfigConflict, check.Conflicts[0].Message)
		}
		seen := map[string]bool{}
		for _, item := range req.Items {
			row, exists := existing[item.ID]
			if exists {
				seen[row.ID] = true
				row.Lane = item.Lane
				timing := adminActivityTiming(row)
				start, duration, grace := adminCharacterScheduleDays(row)
				unchanged := timing == "character-day" && start == item.StartDay && duration == item.DurationDays && grace == item.ClaimDays || timing == "calendar" && row.StartAt == item.StartAt && row.EndAt == item.EndAt && row.ClaimEndAt == item.ClaimEndAt
				frozen, e := s.adminActivityOccurrenceFrozen(tx, row)
				if e != nil {
					return e
				}
				if frozen && !unchanged {
					return constUtil.ErrAdminConfigConflict
				}
				if timing == "character-day" {
					row.Timing = timing
					row.StartDay = item.StartDay
					row.DurationDays = item.DurationDays
					row.ClaimDays = item.ClaimDays
				}
				if !unchanged && !frozen && timing == "calendar" {
					var form model.AdminActivityForm
					if json.Unmarshal(row.FormJSON, &form) != nil {
						return constUtil.ErrAdminConfiguration
					}
					form = adminActivityCanonicalForm(form)
					form.StartAt = item.StartAt
					form.EndAt = item.EndAt
					form.ClaimEndAt = item.ClaimEndAt
					if row.Mode != "opening-14" {
						d, _ := adminActivityDefinitionFor(row.Mode)
						if err := s.adminPersistTypedActivity(tx, d, row.ConsumerKey, form, item.Enabled, true); err != nil {
							return err
						}
					}
					row.FormJSON = adminJSON(form)
				} else if row.Enabled != item.Enabled && timing == "calendar" {
					if err := adminEnableActivityConsumer(tx, row.ConsumerID, item.Enabled); err != nil {
						return err
					}
				}
				row.StartAt = item.StartAt
				row.EndAt = item.EndAt
				row.ClaimEndAt = item.ClaimEndAt
				row.Enabled = item.Enabled
				row.Revision++
			} else {
				var definition db.AdminActivityDefinition
				if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&definition, "id=? AND archived=?", item.TemplateID, false).Error; err != nil {
					return err
				}
				if item.TemplateVersion != adminVersion(definition) {
					return constUtil.ErrAdminConfigConflict
				}
				var form model.AdminActivityForm
				if json.Unmarshal(definition.FormJSON, &form) != nil {
					return constUtil.ErrAdminConfiguration
				}
				form = adminActivityCanonicalForm(form)
				form.StartAt = item.StartAt
				form.EndAt = item.EndAt
				form.ClaimEndAt = item.ClaimEndAt
				row = db.AdminActivityOccurrence{Timing: item.Timing, StartDay: item.StartDay, DurationDays: item.DurationDays, ClaimDays: item.ClaimDays, ID: "occ_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24], ServerID: serverID, TemplateID: definition.ID, TemplateVersion: adminVersion(definition), Name: form.Name, Mode: definition.Mode, StartAt: item.StartAt, EndAt: item.EndAt, ClaimEndAt: item.ClaimEndAt, Lane: item.Lane, Enabled: item.Enabled, FormJSON: adminJSON(form), Revision: 1}
				if item.Timing == "character-day" {
					row.ConsumerKey = "character_plan_" + strings.TrimPrefix(row.ID, "occ_")
					row.ConsumerID = "character-plan:" + row.ConsumerKey
				} else if definition.Mode == "opening-14" {
					row.ConsumerKey = "opening_schedule_" + strings.TrimPrefix(row.ID, "occ_")
					row.ConsumerID = "opening:" + row.ConsumerKey
				} else {
					d, ok := adminActivityDefinitionFor(definition.Mode)
					if !ok {
						return constUtil.ErrAdminInput
					}
					row.ConsumerKey = d.prefix + "schedule_" + strings.TrimPrefix(row.ID, "occ_")[:16]
					if row.Mode == "shop-window" {
						row.ConsumerKey = "shop_schedule_" + form.Options.ShopKey + "_" + strings.TrimPrefix(row.ID, "occ_")[:16]
					}
					row.ConsumerID = d.domain + ":" + row.ConsumerKey
					if row.Mode == "shop-window" && !adminOrdinarySchedulableShop(form.Options.ShopKey) {
						row.ConsumerID = "shop-gate:" + row.ConsumerKey
					}
					if err := s.adminPersistTypedActivity(tx, d, row.ConsumerKey, form, item.Enabled, false); err != nil {
						return err
					}
					presentation := db.AdminActivityPresentation{ActivityID: row.ConsumerID, Mode: row.Mode, TemplateID: form.TemplateID, Name: form.Name, Alias: form.Alias, UpdatedAtMillis: time.Now().UnixMilli()}
					if err := tx.Create(&presentation).Error; err != nil {
						return err
					}
				}
			}
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		for id, row := range existing {
			if !seen[id] {
				if adminActivityTiming(row) != "character-day" {
					if err := adminEnableActivityConsumer(tx, row.ConsumerID, false); err != nil {
						return err
					}
				}
				if err := tx.Model(&row).Updates(map[string]any{"archived": true, "enabled": false, "revision": row.Revision + 1}).Error; err != nil {
					return err
				}
			}
		}
		revision.Revision++
		if err := tx.Save(&revision).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.schedule.save", fmt.Sprint(serverID), fmt.Sprintf("realm=%d revision=%d slots=%d", serverID, revision.Revision, len(req.Items)))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminActivitySchedule(ctx, p, serverID)
}

func adminEnableActivityConsumer(tx *gorm.DB, id string, enabled bool) error {
	domain, key, ok := strings.Cut(id, ":")
	if !ok {
		return constUtil.ErrAdminConfiguration
	}
	var table any
	switch domain {
	case "shop-gate":
		return nil
	case "commerce":
		table = &db.P4CommerceRule{}
	case "welfare":
		table = &db.P4WelfareRule{}
	case "period":
		table = &db.PeriodInstance{}
	case "pool":
		table = &db.GameplayPool{}
	default:
		return constUtil.ErrAdminConfiguration
	}
	return tx.Model(table).Where("id=?", key).Update("enabled", enabled).Error
}

func adminActivityPatternDTO(row db.AdminActivityPattern) (model.AdminActivityPatternDTO, error) {
	entries := []model.AdminActivityPatternEntry{}
	if json.Unmarshal(row.EntriesJSON, &entries) != nil {
		return model.AdminActivityPatternDTO{}, constUtil.ErrAdminConfiguration
	}
	return model.AdminActivityPatternDTO{ID: row.ID, Name: row.Name, CycleDays: row.CycleDays, Entries: entries, Version: adminVersion(row)}, nil
}

// AdminActivityPatterns lists global reusable relative-day rotations.
func (s *GameService) AdminActivityPatterns(ctx context.Context, p *model.AdminPrincipal) ([]model.AdminActivityPatternDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	var rows []db.AdminActivityPattern
	if err := tx.Where("archived=?", false).Order("name,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []model.AdminActivityPatternDTO{}
	for _, row := range rows {
		dto, err := adminActivityPatternDTO(row)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// AdminSaveActivityPattern saves a reusable rotation without changing any published realm calendar.
func (s *GameService) AdminSaveActivityPattern(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminActivityPatternSaveRequest) (*model.AdminActivityPatternDTO, error) {
	var out model.AdminActivityPatternDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.AdminActivityPattern
		if id != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND archived=?", id, false).Error; err != nil {
				return err
			}
			if req.Version != adminVersion(row) {
				return constUtil.ErrAdminConfigConflict
			}
		}
		if id == "" {
			row.ID = "pattern_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
		}
		for _, entry := range req.Entries {
			var definition db.AdminActivityDefinition
			if err := tx.First(&definition, "id=? AND archived=?", entry.TemplateID, false).Error; err != nil {
				return err
			}
			if !definition.Usable {
				return constUtil.ErrAdminInput
			}
		}
		row.Name = req.Name
		row.CycleDays = req.CycleDays
		row.EntriesJSON = adminJSON(req.Entries)
		row.Revision++
		row.UpdatedAtMillis = time.Now().UnixMilli()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		var err error
		out, err = adminActivityPatternDTO(row)
		if err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.pattern.save", row.ID, fmt.Sprintf("cycleDays=%d slots=%d", row.CycleDays, len(req.Entries)))
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeleteActivityPattern archives a rotation while preserving all existing copied occurrences.
func (s *GameService) AdminDeleteActivityPattern(ctx context.Context, p *model.AdminPrincipal, id, version string) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.AdminActivityPattern
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND archived=?", id, false).Error; err != nil {
			return err
		}
		if version != adminVersion(row) {
			return constUtil.ErrAdminConfigConflict
		}
		row.Archived = true
		row.Revision++
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "activity.pattern.archive", id, "existing realm schedules preserved")
	})
}

// adminActivityScheduleFallback hides unscheduled legacy entries after this realm adopts its calendar.
func adminActivityScheduleFallback(tx *gorm.DB, key string) (string, error) {
	allowed, err := adminActivityScheduleAccess(tx, key)
	if err != nil {
		return "", err
	}
	if allowed {
		return key, nil
	}
	return "inactive_schedule_" + p6Digest([]byte(key))[:16], nil
}

// AdminActivityCostResources filters the original display catalogue through the actual commerce parser.
// It does not create currencies, prices or a new acquisition channel.
func (s *GameService) AdminActivityCostResources(ctx context.Context, p *model.AdminPrincipal, q string, filters ...model.AdminItemsRequest) (*model.AdminActivityCostResourcesResponse, error) {
	if err := adminOperationUser(db.GetIns().WithContext(ctx), p, false); err != nil {
		return nil, err
	}
	request := model.AdminItemsRequest{Q: q, Page: 1, PageSize: 100}
	if len(filters) > 0 {
		request = filters[0]
		if request.Q == "" {
			request.Q = q
		}
	}
	data := s.adminActivityCostCatalogue(request.PreferredIDs...)
	catalogue := (&GameService{AdminCatalogueData: data}).AdminCatalogueFiltered(request)
	return &model.AdminActivityCostResourcesResponse{Items: catalogue.Items, Total: catalogue.Total, Facets: catalogue.Facets}, nil
}

// AdoptAdminActivityCalendar is the one-time startup bridge from existing live
// activity configuration to the serving realm's calendar. It must run after all
// original activity initializers and InitializeAdminActivitySchedules, before
// the HTTP/TCP listeners accept requests. Existing published calendars win.
// Original IDs, exact windows, rewards and frozen receipts remain untouched.
func (s *GameService) AdoptAdminActivityCalendar(ctx context.Context) error {
	rows, err := s.AdminActivityInstances(ctx)
	if err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		serverID := s.OriginalServerID()
		if err := adminRequireActivityServer(tx, serverID); err != nil {
			return err
		}
		guard := db.AdminActivityScheduleRevision{ServerID: serverID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; err != nil {
			return err
		}
		revision, err := adminActivityScheduleRevision(tx, serverID, true)
		if err != nil {
			return err
		}
		if revision.Revision > 0 {
			return nil
		}
		// The previous selector exposed only the configured current wheel. Old
		// enabled pool archives must not become extra simultaneous live wheels.
		wheel := "turntable_20261011"
		if tx.Migrator().HasTable(&db.KingRecruitRules{}) {
			var king db.KingRecruitRules
			err := tx.First(&king, 1).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if king.WheelPool == constUtil.P2RedWheelPool || king.WheelPool == constUtil.TurntableScreenshotPool {
				wheel = king.WheelPool
			}
		}
		lane := int64(0)
		for _, source := range rows {
			if strings.Contains(source.ID, "_schedule_") || localPlayPrivateCycleID(source.ID) != "" {
				continue
			}
			if _, supported := adminActivityDefinitionFor(source.Mode); !supported {
				continue
			}
			if source.Form.StartAt <= 0 || source.Form.EndAt <= source.Form.StartAt || source.Form.ClaimEndAt < source.Form.EndAt {
				continue
			}
			var definition db.AdminActivityDefinition
			definitionID := "import_" + p6Digest([]byte(source.ID))[:24]
			if err := tx.First(&definition, "id=?", definitionID).Error; err != nil {
				return err
			}
			enabled := source.Enabled && source.Availability.Usable
			_, key, ok := strings.Cut(source.ID, ":")
			if !ok {
				return constUtil.ErrAdminConfiguration
			}
			if source.Mode == "turntable" {
				enabled = enabled && key == wheel
			}
			// Earlier template imports used availability as enabled. Correct
			// only their untouched initial revision, preserving operator edits.
			if definition.Revision == 1 && definition.SourceActivityID == source.ID && definition.Enabled != enabled {
				definition.Enabled = enabled
				if err := tx.Save(&definition).Error; err != nil {
					return err
				}
			}
			occurrence := db.AdminActivityOccurrence{
				ID:       "adopt_" + p6Digest([]byte(fmt.Sprint(serverID) + "/" + source.ID))[:24],
				ServerID: serverID, TemplateID: definition.ID, TemplateVersion: adminVersion(definition),
				Name: source.Name, Mode: source.Mode, ConsumerKey: key, ConsumerID: source.ID,
				StartAt: source.Form.StartAt, EndAt: source.Form.EndAt, ClaimEndAt: source.Form.ClaimEndAt,
				Lane: lane % 8, Enabled: enabled, FormJSON: adminJSON(source.Form), Revision: 1,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&occurrence).Error; err != nil {
				return err
			}
			lane++
		}
		// The original task rows and each character's Day 1 remain in their
		// existing domain. This calendar row controls only the realm entrance.
		if tx.Migrator().HasTable(&db.OpenServerActivityRule{}) {
			var opening db.OpenServerActivityRule
			err := tx.First(&opening, "id=?", constUtil.OpenServerActivityID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				var definition db.AdminActivityDefinition
				if err := tx.First(&definition, "id=?", "original_opening_14").Error; err != nil {
					return err
				}
				var form model.AdminActivityForm
				// Use the exact already-published opening window; do not move
				// an older character's calendar or generate a second reward.
				form.Mode = "opening-14"
				form.TemplateID = "opening-14-original"
				form.Name = definition.Name
				form.StartAt = opening.StartAt
				form.EndAt = opening.EndAt
				form.ClaimEndAt = opening.RewardEndAt
				form.Source = opening.Source
				occurrence := db.AdminActivityOccurrence{ID: "adopt_opening_" + fmt.Sprint(serverID), ServerID: serverID,
					TemplateID: definition.ID, TemplateVersion: adminVersion(definition), Name: definition.Name, Mode: definition.Mode, Timing: "character-day", StartDay: 1, DurationDays: 14, ClaimDays: 1,
					ConsumerKey: "opening_adopt_" + fmt.Sprint(serverID), ConsumerID: "opening:opening_adopt_" + fmt.Sprint(serverID),
					StartAt: form.StartAt, EndAt: form.EndAt, ClaimEndAt: form.ClaimEndAt, Lane: 0, Enabled: opening.Enabled,
					FormJSON: adminJSON(form), Revision: 1}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&occurrence).Error; err != nil {
					return err
				}
			}
		}
		revision.Revision = 1
		return tx.Save(&revision).Error
	})
}

// adminConfiguredPeriodIdentity recognizes only the separately minted typed
// activity IDs. Original monthly identities retain their stricter contracts;
// accepted battle witnesses and source-bound facts remain mandatory.
func adminConfiguredPeriodIdentity(cfg db.PeriodInstance, prefix, kind string) bool {
	return strings.HasPrefix(cfg.ID, prefix+"admin_") && len(cfg.ID) > len(prefix+"admin_") && cfg.Kind == kind && cfg.Mode == "cumulative" && cfg.Goal > 0 && cfg.RankPlaces == 0 && cfg.StartAt > 0 && cfg.EndAt > cfg.StartAt && cfg.RewardEndAt >= cfg.EndAt
}
