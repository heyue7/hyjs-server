package service

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type localPlayAwakeQuota struct {
	Day  string `json:"day"`
	Used int32  `json:"used"`
}

func validLocalPlayAwakeDay(day string) bool {
	parsed, err := time.Parse("2006-01-02", day)
	return err == nil && parsed.Format("2006-01-02") == day
}

// p7AwakeDayArchive 只在已认证角色行锁内读取本玩法的日配额。
func p7AwakeDayArchive(tx *gorm.DB, id int64, day string) (*localPlayAwakeQuota, error) {
	var row db.LocalPlayActivityOperation
	err := tx.Where("player_id=? AND request_id=?", id, constUtil.LocalPlayAwakeDayPrefix+day).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var quota localPlayAwakeQuota
	if row.Action != constUtil.LocalPlayAwakeDayAction || row.ActivityID != constUtil.LocalPlayAwakeDayActivity || json.Unmarshal(row.Response, &quota) != nil || quota.Day != day || quota.Used < 0 {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &quota, nil
}

// saveP7AwakeCalendar 与付次数事务一起提交当前缓存和日账本。
// 只保存单调已用值；运营关闭工具后，旧个人日的付费次数也不会被下一次绑定洗掉。
func saveP7AwakeCalendar(tx *gorm.DB, state *db.P7AwakeState, real time.Time) error {
	if !GameCalendarContextBound(tx.Statement.Context) {
		return tx.Save(state).Error
	}
	if localPlayActivityOwner(tx) != state.PlayerID || !validLocalPlayAwakeDay(state.Day) || state.Used < 0 {
		return errLocalPlayActivitiesInvalid
	}
	prior, err := p7AwakeDayArchive(tx, state.PlayerID, state.Day)
	if err != nil {
		return err
	}
	if prior != nil && prior.Used > state.Used {
		return errLocalPlayActivitiesInvalid
	}
	raw, err := json.Marshal(localPlayAwakeQuota{Day: state.Day, Used: state.Used})
	if err != nil {
		return err
	}
	row := db.LocalPlayActivityOperation{PlayerID: state.PlayerID, RequestID: constUtil.LocalPlayAwakeDayPrefix + state.Day,
		Action: constUtil.LocalPlayAwakeDayAction, ActivityID: constUtil.LocalPlayAwakeDayActivity, Response: raw, CreatedAt: real.UnixMilli()}
	if err = tx.Save(&row).Error; err != nil {
		return err
	}
	return tx.Save(state).Error
}

// refreshP7AwakeCalendar 在切日时归档旧缓存，回访恢复原已用预算。
// 未绑定工具的调用不查新账本，继续原真实日切语义；旧缓存第一次绑定保留已有次数。
func refreshP7AwakeCalendar(tx *gorm.DB, state *db.P7AwakeState, real time.Time) error {
	today := p1Day(LocalPlayAt(tx.Statement.Context, real))
	if !GameCalendarContextBound(tx.Statement.Context) {
		if state.Day != today {
			state.Day, state.Used = today, 0
		}
		return nil
	}
	if localPlayActivityOwner(tx) != state.PlayerID || state.Used < 0 {
		return errLocalPlayActivitiesInvalid
	}
	if state.Day == "" {
		state.Day = today
	}
	if !validLocalPlayAwakeDay(state.Day) {
		return errLocalPlayActivitiesInvalid
	}
	// 切离的缓存可能来自工具未启用期间；不能降低已归档的历史使用量。
	previous, err := p7AwakeDayArchive(tx, state.PlayerID, state.Day)
	if err != nil {
		return err
	}
	if previous != nil {
		state.Used = max(state.Used, previous.Used)
	}
	if state.Day != today {
		if err = saveP7AwakeCalendar(tx, state, real); err != nil {
			return err
		}
		next, err := p7AwakeDayArchive(tx, state.PlayerID, today)
		if err != nil {
			return err
		}
		state.Day, state.Used = today, 0
		if next != nil {
			state.Used = next.Used
		}
	}
	return nil
}

// freezeP7AwakeCalendar 只标记新接纳的个人探索，既有Frozen字节不被补写。
func freezeP7AwakeCalendar(tx *gorm.DB, id int64, frozen *p7AwakeFrozen, real time.Time) error {
	if !GameCalendarContextBound(tx.Statement.Context) {
		return nil
	}
	if localPlayActivityOwner(tx) != id {
		return errLocalPlayActivitiesInvalid
	}
	accepted := LocalPlayAt(tx.Statement.Context, real)
	frozen.BusinessDay, frozen.BusinessAt = p1Day(accepted), accepted.UnixMilli()
	return nil
}

// p7AwakeFrozenBusinessDay 校验已付实例自身的日期来源，不读当前offset。
func p7AwakeFrozenBusinessDay(run *db.P7AwakeRun, frozen *p7AwakeFrozen) (string, error) {
	if frozen.BusinessDay == "" && frozen.BusinessAt == 0 {
		return p1Day(time.UnixMilli(run.StartAt)), nil
	}
	const oneDay = int64(24 * time.Hour / time.Millisecond)
	delta := frozen.BusinessAt - run.StartAt
	if !validLocalPlayAwakeDay(frozen.BusinessDay) || frozen.BusinessAt <= 0 || p1Day(time.UnixMilli(frozen.BusinessAt)) != frozen.BusinessDay || delta%oneDay != 0 || delta < -3650*oneDay || delta > 3650*oneDay {
		return "", errLocalPlayActivitiesInvalid
	}
	return frozen.BusinessDay, nil
}

// p7AwakeScoreClock 分开每日奖归属与完成事实时间。
// 唯一领奖键属于入场日；P1事实属于实际结算的个人日，继续原未来/旧期门禁。
func p7AwakeScoreClock(tx *gorm.DB, run *db.P7AwakeRun, frozen *p7AwakeFrozen, real time.Time) (string, time.Time, error) {
	day, err := p7AwakeFrozenBusinessDay(run, frozen)
	if err != nil {
		return "", time.Time{}, err
	}
	if GameCalendarContextBound(tx.Statement.Context) && localPlayActivityOwner(tx) != run.PlayerID {
		return "", time.Time{}, errLocalPlayActivitiesInvalid
	}
	return day, LocalPlayAt(tx.Statement.Context, real), nil
}
