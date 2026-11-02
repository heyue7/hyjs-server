package service

import (
	"context"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type ggoOfflineTimer struct{ timer *time.Timer }

func (s *GameService) ggoCancelOfflineTimer(id int64) {
	if item, ok := s.p10ClientOwner().ggoOfflineTimers.LoadAndDelete(id); ok {
		item.(*ggoOfflineTimer).timer.Stop()
	}
}

func (s *GameService) ggoScheduleOfflineExpiry(id, until int64) {
	base := s.p10ClientOwner()
	task := &ggoOfflineTimer{}
	delay := time.Until(time.UnixMilli(until))
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	task.timer = time.AfterFunc(delay, func() {
		if !base.ggoOfflineTimers.CompareAndDelete(id, task) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		unlockScene := p10ScenePlayersLock(id)
		var row db.P10ScenePresence
		err := db.GetIns().WithContext(ctx).First(&row, "player_id=?", id).Error
		eligible := err == nil && row.Scene == constUtil.GGOFieldScene && row.SessionHash == "" && row.OfflineUntil == until && until <= time.Now().UnixMilli()
		unlockScene()
		if eligible {
			base.p10SceneBroadcast(ctx, row.Scene, row.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
			base.ggoNotifyFieldChange(ctx, row.Line)
		}
	})
	if old, ok := base.ggoOfflineTimers.Swap(id, task); ok {
		old.(*ggoOfflineTimer).timer.Stop()
	}
}
