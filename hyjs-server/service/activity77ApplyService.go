package service

import (
	"context"
	"fmt"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func (s *GameService) CheckActivity77Maintenance(ctx context.Context) error {
	var policy db.ClientUpdatePolicy
	if err := db.GetIns().WithContext(ctx).First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Maintenance {
		return fmt.Errorf("先关服再应用活动修复")
	}
	if s.OriginalServerID() != 1 {
		return fmt.Errorf("本次活动修复只覆盖已审核一区")
	}
	return nil
}

// ApplyActivity77 runs bounded, audited domain repairs. An incomplete step
// leaves maintenance enabled; repeating the command resumes its own receipts.
func (s *GameService) ApplyActivity77(ctx context.Context) error {
	if err := s.CheckActivity77Maintenance(ctx); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	var user db.AdminUser
	if err := db.GetIns().WithContext(ctx).Where("enabled=? AND role=?", true, constUtil.AdminRoleAdmin).Order("id").First(&user).Error; err != nil {
		return err
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username + "/77活动修复", Role: user.Role}
	for _, step := range []struct {
		name string
		run  func(context.Context, *model.AdminPrincipal) error
	}{
		{"活动奖励与旧领取映射", s.ApplyActivity77Periods},
		{"会员每日福利", s.ApplyActivity77Member},
		{"试炼与积分商店", s.ApplyActivity77Shops},
		{"月光餐厅与王者池", s.ApplyActivity77WelfareKing},
	} {
		if err := step.run(ctx, actor); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return s.CheckActivity77Maintenance(ctx)
}
