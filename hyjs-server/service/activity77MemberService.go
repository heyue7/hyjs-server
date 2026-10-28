package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyActivity77Member saves the daily coin curve in the existing policy.
// The audit and policy commit together, so retries preserve later admin edits.
func (s *GameService) ApplyActivity77Member(ctx context.Context, actor *model.AdminPrincipal) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var policy db.ClientUpdatePolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Maintenance {
			return fmt.Errorf("先关服再调整会员每日福利")
		}
		snap, err := s.adminNativeLoad(tx, "vip-benefits", true)
		if err != nil {
			return err
		}
		var applied int64
		if err = tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.Activity77MemberAudit, fmt.Sprint(s.OriginalServerID())).Count(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		form := snap.dto.Form
		for i := range form.VipTiers {
			amount, ok := constUtil.Activity77MemberDailyCoin(form.VipTiers[i].Vip)
			if !ok {
				return constUtil.ErrAdminConfiguration
			}
			found := false
			for j := range form.VipTiers[i].DailyRewards {
				r := &form.VipTiers[i].DailyRewards[j]
				if r.ResourceID != "coin" {
					continue
				}
				r.Amount = amount
				found = true
			}
			if !found {
				form.VipTiers[i].DailyRewards = append(form.VipTiers[i].DailyRewards, model.AdminResourceAmount{ResourceID: "coin", Amount: amount})
			}
		}
		if err = s.adminNativeApplyVIP(tx, snap, model.AdminNativeActivitySaveRequest{Form: form, Enabled: snap.dto.Enabled, Alias: snap.dto.Alias}); err != nil {
			return err
		}
		return adminAudit(tx, actor, constUtil.Activity77MemberAudit, fmt.Sprint(s.OriginalServerID()), "会员每日金币按VIP0至12递增至300万；其他奖励、每周福利与领取回执保留")
	})
}
