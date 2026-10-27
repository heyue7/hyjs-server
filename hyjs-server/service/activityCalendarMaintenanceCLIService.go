package service

import (
	"context"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// SetActivityCalendarMaintenanceCLI uses the same audited, idempotent server action
// as the console. It opens no listener and changes no release or notice policy.
func (s *GameService) SetActivityCalendarMaintenanceCLI(ctx context.Context, stop bool) error {
	var (
		user   db.AdminUser
		policy db.ClientUpdatePolicy
	)
	d := db.GetIns().WithContext(ctx)
	if err := d.Where("enabled=? AND role=?", true, constUtil.AdminRoleAdmin).Order("id").First(&user).Error; err != nil {
		return err
	}
	if err := d.First(&policy, 1).Error; err != nil {
		return err
	}
	action := "server_start"
	if stop {
		action = "server_stop"
	}
	_, err := s.AdminClientUpdateAction(ctx, &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}, model.AdminClientUpdateActionRequest{Version: clientUpdateVersion(policy), RequestID: uuid.NewString(), Action: action, Reason: "活动排期修复维护"})
	return err
}
