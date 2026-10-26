package service

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strings"
	"time"
)

// CheckGrowth75Maintenance rejects the explicit deployment before startup migrations.
func (s *GameService) CheckGrowth75Maintenance(ctx context.Context) error {
	if s.OriginalServerID() != constUtil.Growth75ServerID {
		return fmt.Errorf("75版投放仅允许已审核的一区")
	}
	var p db.ClientUpdatePolicy
	if err := db.GetIns().WithContext(ctx).Select("id", "maintenance", "latest_build", "min_login_build", "min_run_build").First(&p, 1).Error; err != nil {
		return err
	}
	if !p.Maintenance {
		return fmt.Errorf("先关服再应用75版投放")
	}
	if p.LatestBuild > constUtil.Growth75Build || p.MinLoginBuild > constUtil.Growth75Build || p.MinRunBuild > constUtil.Growth75Build {
		return fmt.Errorf("现有客户端版本高于75版，停止降级")
	}
	return nil
}

// ApplyGrowth75 is an explicit, resumable maintenance command; it opens no listener.
// Completed domain steps retain their own audits. A later failure keeps maintenance
// and the deployment's private database backup for inspection and a bounded retry.
func (s *GameService) ApplyGrowth75(ctx context.Context) error {
	if err := s.CheckGrowth75Maintenance(ctx); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	var u db.AdminUser
	if err := db.GetIns().WithContext(ctx).Where("enabled=? AND role=?", true, constUtil.AdminRoleAdmin).Order("id").First(&u).Error; err != nil {
		return err
	}
	actor := &model.AdminPrincipal{UserID: u.ID, Username: u.Username + "/75版维护部署", Role: u.Role}
	if err := s.growth75VerifyPublication(ctx); err != nil {
		return fmt.Errorf("75版发布证据校验：%w", err)
	}
	for _, step := range []struct {
		name  string
		apply func(context.Context, *model.AdminPrincipal) error
	}{
		{"恢复活动真实发布时间", s.BackfillGrowth75PublishedAt},
		{"修复测试日历锚点", s.RepairGameCalendar75},
		{"应用商店经济", s.ApplyGrowth75Economy},
		{"发布75客户端兼容策略", s.growth75Release},
	} {
		if err := step.apply(ctx, actor); err != nil {
			return fmt.Errorf("%s：%w", step.name, err)
		}
	}
	if err := s.CheckGrowth75Maintenance(ctx); err != nil {
		return err
	}
	return adminAudit(db.GetIns().WithContext(ctx), actor, constUtil.Growth75ApplyAction, fmt.Sprint(s.OriginalServerID()), "audit321/820排期发布时间已核对；日历锚点与经济配置已应用；1.0.75兼容策略已发布；保持维护。活动CAS补丁由部署脚本随后应用并独立记录")
}

// The whole source set is checked, including already backfilled rows on a retry.
func (s *GameService) growth75VerifyPublication(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var proof db.AdminAudit
		if err := tx.First(&proof, "id=?", constUtil.Growth75SourceAuditID).Error; err != nil {
			return err
		}
		if proof.Time != constUtil.Growth75SourcePublishedAt || proof.Action != "activity.schedule.save" || proof.Target != "1" || proof.Detail != constUtil.Growth75SourceDetail {
			return constUtil.ErrAdminConfiguration
		}
		var rows []db.AdminActivityOccurrence
		if err := tx.Select("id", "form_json", "first_published_at").Where("server_id=?", constUtil.Growth75ServerID).Order("id").Limit(5001).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 5000 {
			return constUtil.ErrAdminConfiguration
		}
		count := 0
		for _, row := range rows {
			var form model.AdminActivityForm
			if json.Unmarshal(row.FormJSON, &form) != nil {
				return constUtil.ErrAdminConfiguration
			}
			if !strings.Contains(form.Source, constUtil.Growth75SourceLabel) {
				continue
			}
			count++
			if row.FirstPublishedAt != 0 && row.FirstPublishedAt != proof.Time {
				return fmt.Errorf("排期真实发布时间与审核证据冲突：%s", row.ID)
			}
		}
		if count != constUtil.Growth75SourceSlots {
			return fmt.Errorf("审核排期应为%d条，实际%d条", constUtil.Growth75SourceSlots, count)
		}
		return nil
	})
}

func (s *GameService) growth75Release(ctx context.Context, a *model.AdminPrincipal) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, a, true); err != nil {
			return err
		}
		var p db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, 1).Error; err != nil {
			return err
		}
		if !p.Maintenance || p.LatestBuild > constUtil.Growth75Build || p.MinLoginBuild > constUtil.Growth75Build || p.MinRunBuild > constUtil.Growth75Build {
			return constUtil.ErrAdminConfigConflict
		}
		if p.LatestBuild == constUtil.Growth75Build && p.MinLoginBuild == constUtil.Growth75Build && p.MinRunBuild == constUtil.Growth75Build && p.VersionName == constUtil.Growth75Version {
			return nil
		}
		now := time.Now().UnixMilli()
		r := db.ClientUpdateRelease{BuildCode: constUtil.Growth75Build, VersionName: constUtil.Growth75Version, Title: "成长与商店修复", Content: "请安装1.0.75测试包。商店价格、活动奖励与时间校验已更新。", Mode: "maintenance", MinLoginBuild: constUtil.Growth75Build, MinRunBuild: constUtil.Growth75Build, MaintenanceStartAt: p.MaintenanceStartAt, State: "published", PublishedAt: now, UpdatedAt: now, PublishedBy: a.Username}
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		p.ReleaseID = r.ID
		p.LatestBuild = r.BuildCode
		p.MinLoginBuild = r.MinLoginBuild
		p.MinRunBuild = r.MinRunBuild
		p.VersionName = r.VersionName
		p.Title = r.Title
		p.Content = r.Content
		p.Mode = r.Mode
		p.LoginGateAt = now
		p.NoticeEnabled = false
		p.Revision++
		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		return adminAudit(tx, a, "growth75.client.compatibility", constUtil.Growth75Version, "仅更新兼容门槛；维护状态、维护epoch与人工开服保持")
	})
}
