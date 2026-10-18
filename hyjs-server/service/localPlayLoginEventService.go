package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type localPlayLoginCycleKey struct{}

// Called only from the original successful character-login transaction. Each
// date counts once in each current private period, including a same-day fresh
// period. Opening a tool never calls this producer.
func (s *GameService) RecordLocalPlayLogin(tx *gorm.DB, id int64, now time.Time) error {
	if localPlayActivityOwner(tx) != id || !tx.Migrator().HasTable(&db.LocalPlayActivityBinding{}) {
		return nil
	}
	var bindings []db.LocalPlayActivityBinding
	if err := tx.Where("player_id=?", id).Order("activity_id").Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		switch binding.ActivityID {
		case "LoginAward", "Congratulation", "Festival", "Update":
		default:
			continue
		}
		var cycle db.LocalPlayActivityCycle
		if err := tx.First(&cycle, "id=?", binding.CycleID).Error; err != nil {
			return err
		}
		if cycle.PlayerID != id || cycle.Source != constUtil.LocalPlayActivitiesVersion {
			return errLocalPlayActivitiesInvalid
		}
		if now.UnixMilli() < cycle.StartAt || now.UnixMilli() >= cycle.EndAt {
			continue
		}
		var snapshot localPlayScopedSnapshot
		if json.Unmarshal(cycle.Snapshot, &snapshot) != nil {
			return errLocalPlayActivitiesInvalid
		}
		found := false
		for _, period := range snapshot.Periods {
			found = found || period.Kind == constUtil.P1EventLoginDay
		}
		if !found {
			continue
		}
		scope := context.WithValue(tx.Statement.Context, localPlayLoginCycleKey{}, cycle.ID)
		if err := s.RecordP1Event(tx.WithContext(scope), id, "login:"+p1Day(now)+":"+cycle.ID, constUtil.P1EventLoginDay, 1, now); err != nil {
			return err
		}
	}
	return nil
}

func localPlayActivityEventVisible(tx *gorm.DB, period db.PeriodInstance, source string) (bool, error) {
	scope, _ := tx.Statement.Context.Value(localPlayLoginCycleKey{}).(string)
	cycleID := localPlayPrivateCycleID(period.ID)
	if scope != "" {
		if cycleID != scope {
			return false, nil
		}
	} else if cycleID != "" && period.Kind == constUtil.P1EventLoginDay && strings.HasPrefix(source, "login:") {
		// The ordinary per-date fact remains original/global. A later real login
		// emits the scoped fact above; one action cannot count twice in a cycle.
		return false, nil
	}
	return localPlayActivityVisible(tx, period.ID)
}
