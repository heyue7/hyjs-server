package service

import (
	"context"
	"crypto/subtle"
	"errors"

	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
)

// P13GuildMap reads only an authenticated role's accepted frozen field. It also
// works during position loading, before the original map handshake completes.
func (s *GameService) P13GuildMap(ctx context.Context, id int64, session []byte) (*model.P13GuildMapResponse, error) {
	if _, err := AuthorizeAccount(ctx, id, session); err != nil {
		return nil, err
	}
	var result model.P13GuildMapResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Select("id", "session").First(&profile, id).Error; err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(profile.Session, session) != 1 {
			return errors.New("本地会话无效，请重新登录")
		}
		actor, _, snapshot, _, err := s.p13UFFieldCurrent(tx, id, p13UFNow(ctx))
		if err != nil {
			return err
		}
		if actor.MapProjection != snapshot.MapProjection {
			return errors.New("公会战地图投影与本场来源不一致")
		}
		result.MapProjection = snapshot.MapProjection
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
