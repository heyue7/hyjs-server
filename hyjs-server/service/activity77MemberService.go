package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ApplyActivity77Member moves the approved weekly coin budget into the saved
// daily tiers. The audit and policy commit together, so retries cannot add twice.
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
			found := false
			for j := range form.VipTiers[i].DailyRewards {
				r := &form.VipTiers[i].DailyRewards[j]
				if r.ResourceID != "coin" {
					continue
				}
				if r.Amount < 0 || r.Amount > math.MaxInt32-constUtil.Activity77MemberDayCoin-constUtil.Activity77MemberSundayCoin {
					return constUtil.ErrAdminConfiguration
				}
				r.Amount += constUtil.Activity77MemberDayCoin
				found = true
			}
			if !found {
				form.VipTiers[i].DailyRewards = append(form.VipTiers[i].DailyRewards, model.AdminResourceAmount{ResourceID: "coin", Amount: constUtil.Activity77MemberDayCoin})
			}
		}
		if err = s.adminNativeApplyVIP(tx, snap, model.AdminNativeActivitySaveRequest{Form: form, Enabled: snap.dto.Enabled, Alias: snap.dto.Alias}); err != nil {
			return err
		}
		return adminAudit(tx, actor, constUtil.Activity77MemberAudit, fmt.Sprint(s.OriginalServerID()), "原档位金币每天增加6428571；北京时间周日额外3，七天合计45000000；原付费资格与已领日回执保留")
	})
}

// activity77MemberSunday projects the indivisible three-coin weekly remainder
// from the same saved activation used by the maintenance operator.
func (s *GameService) activity77MemberSunday(tx *gorm.DB, form model.AdminNativeActivityForm, now time.Time) (model.AdminNativeActivityForm, error) {
	if now.In(time.FixedZone("UTC+8", 8*3600)).Weekday() != time.Sunday || !gameSchemaHasTable(tx, &db.AdminAudit{}) {
		return form, nil
	}
	var applied int64
	if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.Activity77MemberAudit, fmt.Sprint(s.OriginalServerID())).Count(&applied).Error; err != nil {
		return form, err
	}
	if applied == 0 {
		return form, nil
	}
	for i := range form.VipTiers {
		for j := range form.VipTiers[i].DailyRewards {
			r := &form.VipTiers[i].DailyRewards[j]
			if r.ResourceID == "coin" {
				if r.Amount > math.MaxInt32-constUtil.Activity77MemberSundayCoin {
					return form, constUtil.ErrAdminConfiguration
				}
				r.Amount += constUtil.Activity77MemberSundayCoin
			}
		}
	}
	return form, nil
}
