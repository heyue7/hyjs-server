package service

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"sort"
	"strings"
	"time"
)

func adminMonthSignView(tx *gorm.DB, months ...string) (*model.AdminMonthSignRulesResponse, error) {
	month := ""
	if len(months) > 0 {
		month = months[0]
	}
	month, err := monthSignMonth(month)
	if err != nil {
		return nil, err
	}
	rows, configured, err := monthSignRulesForAdmin(tx, month)
	if err != nil {
		return nil, err
	}
	parsed, _ := time.Parse("2006-01", month)
	version := adminVersion(struct {
		Month      string
		Configured bool
		Rows       []db.MonthSignRule
	}{month, configured, rows})
	out := &model.AdminMonthSignRulesResponse{Month: month, Configured: configured, DaysInMonth: int64(parsed.AddDate(0, 1, 0).AddDate(0, 0, -1).Day()), Version: version, Enabled: true, Rows: []model.AdminMonthSignDay{}}
	sources := []string{}
	for i, row := range rows {
		if row.Day != int32(i+1) {
			return nil, constUtil.ErrAdminConfiguration
		}
		out.Enabled = out.Enabled && row.Enabled
		if len(sources) == 0 || sources[len(sources)-1] != row.Source {
			sources = append(sources, row.Source)
		}
		out.Rows = append(out.Rows, model.AdminMonthSignDay{Day: int64(row.Day), Reward: adminTypedResources(row.Reward), DoubleVIP: int64(row.DoubleVIP)})
	}
	out.Source = strings.Join(sources, "；")
	return out, nil
}

// AdminMonthSignRules 只读所选自然月奖历，未配置时继承全局默认，不创建领取状态。
func (s *GameService) AdminMonthSignRules(ctx context.Context, actor *model.AdminPrincipal, months ...string) (*model.AdminMonthSignRulesResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	month := ""
	if len(months) > 1 {
		return nil, constUtil.ErrAdminInput
	}
	if len(months) == 1 {
		month = months[0]
	}
	month, err := monthSignMonth(month)
	if err != nil {
		return nil, err
	}
	return adminMonthSignView(tx, month)
}

// AdminSaveMonthSignRules 原子保存31档；已签到日期继续使用原LastReward快照。
func (s *GameService) AdminSaveMonthSignRules(ctx context.Context, actor *model.AdminPrincipal, req model.AdminMonthSignRulesSaveRequest) (*model.AdminMonthSignRulesResponse, error) {
	month, err := monthSignMonth(req.Month)
	if err != nil {
		return nil, err
	}
	currentMonth, _ := monthSignMonth("")
	if month < currentMonth || len(req.Rows) != 31 {
		return nil, constUtil.ErrAdminInput
	}
	req.Rows = append([]model.AdminMonthSignDay(nil), req.Rows...)
	sort.Slice(req.Rows, func(i, j int) bool { return req.Rows[i].Day < req.Rows[j].Day })
	for i, row := range req.Rows {
		if row.Day != int64(i+1) || row.DoubleVIP < 0 || row.DoubleVIP > 12 || len(row.Reward) != 1 {
			return nil, constUtil.ErrAdminInput
		}
	}
	var out *model.AdminMonthSignRulesResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		// 固定行先串行化未创建月份的保存；随后当前读避免复用鉴权形成的旧事务快照。
		var lock db.MonthSignRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, "day=?", 1).Error; err != nil {
			return err
		}
		currentMonth, _ := monthSignMonth("")
		if month < currentMonth {
			return constUtil.ErrAdminInput
		}
		current, err := adminMonthSignView(tx.Clauses(clause.Locking{Strength: "UPDATE"}), month)
		if err != nil {
			return err
		}
		if current.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		rows := make([]db.MonthSignRule, 0, 31)
		for _, day := range req.Rows {
			content, err := s.adminTypedResourceSpec(day.Reward, false)
			if err != nil {
				return err
			}
			supported, err := s.openServerRewardSupported(content)
			if err != nil {
				return err
			}
			if !supported {
				return constUtil.ErrAdminInput
			}
			rows = append(rows, db.MonthSignRule{Day: int32(day.Day), Reward: content, DoubleVIP: int32(day.DoubleVIP), Enabled: req.Enabled, Source: "运营配置；保留已领取日期快照"})
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if err = tx.Save(&db.MonthSignCalendar{MonthKey: month, Rules: raw}).Error; err != nil {
			return err
		}

		if err = adminAudit(tx, actor, "month-sign.save", "month-sign:"+month, "更新31档月奖历；保留原领取历史"); err != nil {
			return err
		}
		out, err = adminMonthSignView(tx, month)
		return err
	})
	return out, err
}
