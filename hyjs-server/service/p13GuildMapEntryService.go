package service

import (
	"context"
	"crypto/subtle"
	"errors"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// P13GuildMapEntry probes eligible map resources without accepting field entry.
// The original transfer and map handshake still perform their own admission.
func (s *GameService) P13GuildMapEntry(ctx context.Context, id int64, session []byte) (*model.P13GuildMapEntryResponse, error) {
	if _, err := AuthorizeAccount(ctx, id, session); err != nil {
		return nil, err
	}
	var result model.P13GuildMapEntryResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		policy, err := s.p13UFFieldNeedPolicy(tx)
		if err != nil {
			return err
		}
		if err := p13UFNeedWarEnabled(tx); err != nil {
			return err
		}
		var profile db.Profile
		if err = tx.Select("id", "session").First(&profile, id).Error; err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(profile.Session, session) != 1 {
			return errors.New("本地会话无效，请重新登录")
		}
		now := p13UFNow(ctx)
		calendar, err := p13UFCalendar(tx, now)
		if err != nil {
			return err
		}
		if calendar.Stage == constUtil.P13UnionFightStageMatch {
			return errors.New("原公会战匹配窗口尚未结束")
		}
		preparation := policy.PreparationMillis
		if calendar.Custom && calendar.Schedule != nil {
			preparation = calendar.Schedule.PreparationMillis
		}
		_, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		var matches []db.P13UnionFightMatch
		if err = tx.Where("(left_guild_id=? OR right_guild_id=?) AND end_at>?", guild.ID, guild.ID, now.UnixMilli()).Order("start_at").Find(&matches).Error; err != nil {
			return err
		}
		var selected *db.P13UnionFightMatch
		for i := range matches {
			if preparation > 0 && now.UnixMilli() >= matches[i].StartAt-preparation {
				selected = &matches[i]
				break
			}
		}
		if selected == nil {
			return errors.New("本会没有处于已发布准备或开赛窗口的真实对局")
		}
		if selected.Day != calendar.MatchDay {
			return errors.New("本场已发布对局不属于当前原比赛日")
		}
		if closed, e := p13UFWorldClosed(tx, selected.ID); e != nil {
			return e
		} else if closed {
			return errors.New("本场已经封账，不能重新入场")
		}
		if _, _, err = p13UFFieldFighter(tx, selected, id, guild.ID); err != nil {
			return err
		}
		snapshot, err := p13UFFieldStoredProjection(tx, selected.ID)
		if err == nil {
			if now.UnixMilli() < selected.StartAt-snapshot.PreparationMillis {
				return errors.New("本场冻结的准备窗口尚未开始")
			}
			result.MapProjection = snapshot.MapProjection
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var actors int64
		if err = tx.Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=?", selected.ID).Count(&actors).Error; err != nil {
			return err
		}
		if actors != 0 {
			return errors.New("公会战原地图快照损坏")
		}
		if !p13UFMapProjectionKnown(policy.MapProjection) {
			return errors.New("公会战冻结地图投影损坏")
		}
		result.MapProjection = policy.MapProjection
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
