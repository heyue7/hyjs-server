package service

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type localPlayRushQuota struct {
	Day           string `json:"day"`
	FreeResetUsed int32  `json:"freeResetUsed"`
	PaidResetUsed int32  `json:"paidResetUsed"`
	EliteUsed     int32  `json:"eliteUsed"`
	EliteBought   int32  `json:"eliteBought"`
}

func localPlayRushQuotaValid(quota localPlayRushQuota) bool {
	day, err := time.Parse("2006-01-02", quota.Day)
	return err == nil && day.Format("2006-01-02") == quota.Day && quota.FreeResetUsed >= 0 && quota.PaidResetUsed >= 0 && quota.EliteUsed >= 0 && quota.EliteBought >= 0
}

func readLocalPlayRushQuota(tx *gorm.DB, id int64, day string) (*localPlayRushQuota, error) {
	var row db.LocalPlayActivityOperation
	err := tx.First(&row, "player_id=? AND request_id=?", id, constUtil.LocalPlayRushDayPrefix+day).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var quota localPlayRushQuota
	if row.Action != constUtil.LocalPlayRushDayAction || row.ActivityID != constUtil.LocalPlayRushDayActivity || json.Unmarshal(row.Response, &quota) != nil || quota.Day != day || !localPlayRushQuotaValid(quota) {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &quota, nil
}

// saveP7RushCalendar 在原角色事务内同步四项日预算，轮次和战斗进度只保存在原状态表。
func saveP7RushCalendar(tx *gorm.DB, state *db.P7RushState, real time.Time) error {
	if !GameCalendarContextBound(tx.Statement.Context) {
		return tx.Save(state).Error
	}
	quota := localPlayRushQuota{Day: state.Day, FreeResetUsed: state.FreeResetUsed, PaidResetUsed: state.PaidResetUsed, EliteUsed: state.EliteUsed, EliteBought: state.EliteBought}
	if localPlayActivityOwner(tx) != state.PlayerID || !localPlayRushQuotaValid(quota) {
		return errLocalPlayActivitiesInvalid
	}
	prior, err := readLocalPlayRushQuota(tx, state.PlayerID, state.Day)
	if err != nil {
		return err
	}
	if prior != nil && (prior.FreeResetUsed > quota.FreeResetUsed || prior.PaidResetUsed > quota.PaidResetUsed || prior.EliteUsed > quota.EliteUsed || prior.EliteBought > quota.EliteBought) {
		return errLocalPlayActivitiesInvalid
	}
	raw, err := json.Marshal(quota)
	if err != nil {
		return err
	}
	row := db.LocalPlayActivityOperation{PlayerID: state.PlayerID, RequestID: constUtil.LocalPlayRushDayPrefix + state.Day, Action: constUtil.LocalPlayRushDayAction, ActivityID: constUtil.LocalPlayRushDayActivity, Response: raw, CreatedAt: real.UnixMilli()}
	if err = tx.Save(&row).Error; err != nil {
		return err
	}
	return tx.Save(state).Error
}

// refreshP7RushCalendar 回访个人日期时恢复原已用次数，不改轮次、星数、加成或pending。
func refreshP7RushCalendar(tx *gorm.DB, state *db.P7RushState, real time.Time) error {
	today := p1Day(LocalPlayAt(tx.Statement.Context, real))
	if !GameCalendarContextBound(tx.Statement.Context) {
		if state.Day != today {
			state.Day = today
			state.FreeResetUsed, state.PaidResetUsed, state.EliteUsed, state.EliteBought = 0, 0, 0, 0
		}
		return nil
	}
	if localPlayActivityOwner(tx) != state.PlayerID {
		return errLocalPlayActivitiesInvalid
	}
	previous, err := readLocalPlayRushQuota(tx, state.PlayerID, state.Day)
	if err != nil {
		return err
	}
	if previous != nil {
		state.FreeResetUsed = max(state.FreeResetUsed, previous.FreeResetUsed)
		state.PaidResetUsed = max(state.PaidResetUsed, previous.PaidResetUsed)
		state.EliteUsed = max(state.EliteUsed, previous.EliteUsed)
		state.EliteBought = max(state.EliteBought, previous.EliteBought)
	}
	if state.Day == today {
		return nil
	}
	if err = saveP7RushCalendar(tx, state, real); err != nil {
		return err
	}
	next, err := readLocalPlayRushQuota(tx, state.PlayerID, today)
	if err != nil {
		return err
	}
	state.Day = today
	state.FreeResetUsed, state.PaidResetUsed, state.EliteUsed, state.EliteBought = 0, 0, 0, 0
	if next != nil {
		state.FreeResetUsed, state.PaidResetUsed = next.FreeResetUsed, next.PaidResetUsed
		state.EliteUsed, state.EliteBought = next.EliteUsed, next.EliteBought
	}
	return nil
}
