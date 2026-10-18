package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
)

type localPlayCopyDailyQuota struct {
	Counts map[string]int32 `json:"counts"`
}

// refreshCopyCalendar 保存离开的个人日期额度，回拨时恢复该日已用次数。
// 正常部署继续使用原日切；原入场、结算、扫荡和付费重置共享同一当前日缓存。
func (s *GameService) refreshCopyCalendar(ctx context.Context, tx *gorm.DB, id int64, game *model.GameState, real time.Time) error {
	if !GameCalendarContextBound(ctx) {
		refreshCopySweepDay(game, real)
		return nil
	}
	if localPlayActivityOwner(tx) != id {
		return errLocalPlayActivitiesInvalid
	}
	today := p1Day(LocalPlayAt(ctx, real))
	if game.CopyChallengeDay == today {
		return nil
	}
	// 缺日期的旧档先绑定已有次数，不能在首次使用钱包日期时丢掉旧额度。
	if game.CopyChallengeDay == "" {
		game.CopyChallengeDay = today
		return nil
	}
	old := localPlayCopyDailyQuota{Counts: map[string]int32{}}
	for _, info := range game.Copy.GetPassedCopyInfo() {
		key := info.GetChapterId() + "/" + info.GetMissionId()
		if info == nil || info.GetChapterId() == "" || info.GetMissionId() == "" || info.GetChallengeTimes() < 0 {
			return errLocalPlayActivitiesInvalid
		}
		if _, exists := old.Counts[key]; exists {
			return errLocalPlayActivitiesInvalid
		}
		old.Counts[key] = info.GetChallengeTimes()
	}
	raw, err := json.Marshal(old)
	if err != nil {
		return err
	}
	archive := db.LocalPlayActivityOperation{PlayerID: id, RequestID: "copy-day:" + game.CopyChallengeDay,
		Action: "copy-day", ActivityID: "mainline", Response: raw, CreatedAt: real.UnixMilli()}
	if err = tx.Save(&archive).Error; err != nil {
		return err
	}
	var prior db.LocalPlayActivityOperation
	err = tx.First(&prior, "player_id=? AND request_id=?", id, "copy-day:"+today).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	next := localPlayCopyDailyQuota{Counts: map[string]int32{}}
	if err == nil {
		if prior.Action != "copy-day" || prior.ActivityID != "mainline" || json.Unmarshal(prior.Response, &next) != nil || next.Counts == nil {
			return errLocalPlayActivitiesInvalid
		}
		for key, count := range next.Counts {
			if key == "" || count < 0 {
				return errLocalPlayActivitiesInvalid
			}
		}
	}
	for _, info := range game.Copy.GetPassedCopyInfo() {
		info.ChallengeTimes = proto.Int32(next.Counts[info.GetChapterId()+"/"+info.GetMissionId()])
	}
	game.CopyChallengeDay = today
	return nil
}
