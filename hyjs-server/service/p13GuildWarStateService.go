package service

import (
	"context"
	"crypto/subtle"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// p13UFNeedWarEnabled is called only after the global guild policy lock at new
// admission. It is deliberately independent of frozen field/worker eligibility.
func p13UFNeedWarEnabled(tx *gorm.DB) error {
	var policy db.P13UnionFightPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,war_enabled").First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.WarEnabled {
		return errors.New(constUtil.P13UnionFightMasterClosed)
	}
	return nil
}

// P13GuildWarState authenticates a live same-realm role without entering a map,
// preparing activities, producing a declaration or changing a frozen source.
func (s *GameService) P13GuildWarState(ctx context.Context, id int64, session []byte) (*model.P13GuildWarStateResponse, error) {
	account, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	if AdminProfileServerID(*account) != s.OriginalServerID() {
		return nil, errors.New("请求区服与当前部署不一致")
	}
	var out model.P13GuildWarStateResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id,session,server_id").First(&profile, id).Error; err != nil {
			return err
		}
		if AdminProfileServerID(profile) != s.OriginalServerID() {
			return errors.New("请求区服与当前部署不一致")
		}
		if subtle.ConstantTimeCompare(profile.Session, session) != 1 {
			return errors.New("本地会话无效，请重新登录")
		}
		// This is a display snapshot. Do not hold a policy lock while waiting
		// for a profile: signup locks profile before its current policy gate read.
		var policy db.P13UnionFightPolicy
		if err := tx.Select("id,war_enabled").First(&policy, 1).Error; err != nil {
			return err
		}
		out.WarEnabled = policy.WarEnabled
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
