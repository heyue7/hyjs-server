package service

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

//go:embed data/growth74_activities.json
var growth74ActivitiesJSON []byte

type growth74Activity struct {
	Timing       string                  `json:"timing"`
	StartDay     int64                   `json:"startDay"`
	DurationDays int64                   `json:"durationDays"`
	ClaimDays    int64                   `json:"claimDays"`
	Lane         int64                   `json:"lane"`
	MonthOffset  *int                    `json:"monthOffset"`
	Form         model.AdminActivityForm `json:"form"`
}

func (s *GameService) growth74Activities(ctx context.Context, a *model.AdminPrincipal) error {
	d := db.GetIns().WithContext(ctx)
	var applied int64
	if e := d.Model(&db.AdminAudit{}).Where("action=? AND target=?", "growth74.activities", fmt.Sprint(s.OriginalServerID())).Count(&applied).Error; e != nil {
		return e
	}
	if applied > 0 {
		return nil
	}
	var realm db.AdminGameServer
	if e := d.First(&realm, s.OriginalServerID()).Error; e != nil {
		return e
	}
	start := time.UnixMilli(realm.OpenAt).In(time.FixedZone("Asia/Shanghai", 8*3600))
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	schedule, e := s.AdminActivitySchedule(ctx, a, s.OriginalServerID())
	if e != nil {
		return e
	}
	var kingPool db.GameplayPool
	if e = d.First(&kingPool, "id=?", "recruit_3").Error; e != nil {
		return e
	}
	var kingPrizes []db.GameplayPoolReward
	if e = d.Where("pool_id=?", "recruit_3").Order("id").Find(&kingPrizes).Error; e != nil {
		return e
	}
	kingForm, e := s.adminKingRecruitActivityForm(d, kingPool, kingPrizes)
	if e != nil {
		return fmt.Errorf("原轮转池: %w", e)
	}
	rotating := &kingForm
	// First disable old frozen entrances, then archive them with the normal schedule save.
	for i := range schedule.Items {
		schedule.Items[i].Enabled = false
	}
	schedule, e = s.AdminSaveActivitySchedule(ctx, a, s.OriginalServerID(), model.AdminActivityScheduleSaveRequest{Version: schedule.Version, Items: schedule.Items})
	if e != nil {
		return fmt.Errorf("停用旧排期: %w", e)
	}
	schedule, e = s.AdminSaveActivitySchedule(ctx, a, s.OriginalServerID(), model.AdminActivityScheduleSaveRequest{Version: schedule.Version, Items: []model.AdminActivityOccurrenceDTO{}})
	if e != nil {
		return fmt.Errorf("归档旧排期: %w", e)
	}
	var input []growth74Activity
	if e = json.Unmarshal(growth74ActivitiesJSON, &input); e != nil {
		return e
	}
	if rotating != nil {
		rotating.Name = "王者招募"
		rotating.Source = "成长投放V12 · 原SAO/ALO/GGO轮转"
		rotating.Alias = "成长投放V12"
		input = append(input, growth74Activity{Timing: "calendar", StartDay: 1, DurationDays: 183, Lane: 16, Form: *rotating})
	}
	input = append(input, growth74Activity{Timing: "character-day", StartDay: 1, DurationDays: 14, ClaimDays: 1, Form: model.AdminActivityForm{Mode: "opening-14", TemplateID: "opening-14-original", Name: "开服十四日挑战", Source: "成长投放V12 · 原249项任务和消费规则"}})
	templates := map[string]*model.AdminActivityDefinitionDTO{}
	existing, e := s.AdminActivityDefinitions(ctx, a)
	if e != nil {
		return e
	}
	out := []model.AdminActivityOccurrenceDTO{}
	for _, v := range input {
		f := v.Form
		f.StartAt, f.EndAt, f.ClaimEndAt = 0, 0, 0
		key := adminVersion(f)
		t := templates[key]
		if t == nil {
			req := model.AdminActivityDefinitionSaveRequest{Name: f.Name, Form: f, DefaultDays: v.DurationDays, ClaimDays: v.ClaimDays, Enabled: true}
			id := ""
			for _, old := range existing {
				if growth74Source(old.Form.Source) && old.Name == f.Name && adminVersion(old.Form) == adminVersion(f) {
					id = old.ID
					req.Version = old.Version
					break
				}
			}
			t, e = s.AdminSaveActivityDefinition(ctx, a, id, req)
			if e != nil {
				return fmt.Errorf("模板 %s (%s): %w", f.Name, f.Mode, e)
			}
			templates[key] = t
		}
		at := start.AddDate(0, 0, int(v.StartDay-1))
		if v.MonthOffset != nil {
			at = time.Date(start.Year(), start.Month()+time.Month(*v.MonthOffset), 15, 0, 0, 0, 0, start.Location())
			if f.Mode == "shop-window" && f.Options.ShopKey == "guild" {
				at = time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, at.Location())
				v.DurationDays = int64(at.AddDate(0, 1, 0).Sub(at) / (24 * time.Hour))
			}
		}
		end := at.AddDate(0, 0, int(v.DurationDays))
		claim := end.AddDate(0, 0, int(v.ClaimDays))
		out = append(out, model.AdminActivityOccurrenceDTO{Timing: v.Timing, StartDay: v.StartDay, DurationDays: v.DurationDays, ClaimDays: v.ClaimDays, TemplateID: t.ID, TemplateVersion: t.Version, Name: t.Name, Mode: t.Mode, StartAt: at.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: claim.UnixMilli(), Lane: v.Lane, Enabled: true})
	}
	preview, e := s.AdminPreviewActivitySchedule(ctx, a, s.OriginalServerID(), model.AdminActivitySchedulePreviewRequest{Version: schedule.Version, Items: out})
	if e != nil {
		return e
	}
	if !preview.Valid {
		return fmt.Errorf("新排期校验: %+v", preview.Conflicts)
	}
	_, e = s.AdminSaveActivitySchedule(ctx, a, s.OriginalServerID(), model.AdminActivityScheduleSaveRequest{Version: schedule.Version, Items: out})
	if e != nil {
		return e
	}
	return adminAudit(d, a, "growth74.activities", fmt.Sprint(s.OriginalServerID()), fmt.Sprintf("归档旧排期，发布%d项原消费者排期；个人开服窗口至少14天", len(out)))
}
