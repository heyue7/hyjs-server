package service

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"time"
)

// These are reversible date caches under the existing profile transaction.
// Original business receipts, wallet rows and battle evidence are never changed.
func calendarQuota75Load(tx *gorm.DB, id int64, domain, key string, out any) (bool, error) {
	var row db.LocalPlayActivityOperation
	err := tx.First(&row, "player_id=? AND request_id=?", id, "calendar75:"+domain+":"+key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Action != constUtil.CalendarQuota75Action || row.ActivityID != domain || len(row.Response) > constUtil.CalendarQuota75MaxBytes || json.Unmarshal(row.Response, out) != nil {
		return false, errors.New(constUtil.CalendarQuota75Invalid)
	}
	return true, nil
}
func calendarQuota75Save(tx *gorm.DB, id int64, domain, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > constUtil.CalendarQuota75MaxBytes {
		return errors.New(constUtil.CalendarQuota75Invalid)
	}
	row := db.LocalPlayActivityOperation{PlayerID: id, RequestID: "calendar75:" + domain + ":" + key, Action: constUtil.CalendarQuota75Action, ActivityID: domain, Response: raw, CreatedAt: time.Now().UnixMilli()}
	return tx.Save(&row).Error
}

type calendarGuildQuota75 struct {
	Login   bool  `json:"login"`
	Stamina bool  `json:"stamina"`
	Coin    int64 `json:"coin"`
	Badge   int32 `json:"badge"`
}

func calendarGuildDay75(tx *gorm.DB, m *db.P13GuildMember, now time.Time) error {
	if !ServerGameClockContextBound(tx.Statement.Context) {
		return nil
	}
	day := p13GuildDay(now)
	var current struct {
		Day string `json:"day"`
	}
	found, err := calendarQuota75Load(tx, m.PlayerID, constUtil.CalendarQuota75Guild, "current", &current)
	if err != nil {
		return err
	}
	if found && current.Day == day {
		return nil
	}
	dates := map[string]bool{}
	for _, v := range []string{current.Day, m.LastLoginDay, m.StaminaDay, m.DonateDay} {
		if v != "" {
			dates[v] = true
		}
	}
	for date := range dates {
		if _, e := time.Parse("2006-01-02", date); e != nil {
			return errors.New(constUtil.CalendarQuota75Invalid)
		}
		var q calendarGuildQuota75
		if _, err = calendarQuota75Load(tx, m.PlayerID, constUtil.CalendarQuota75Guild, date, &q); err != nil {
			return err
		}
		q.Login = q.Login || m.LastLoginDay == date
		q.Stamina = q.Stamina || m.StaminaDay == date
		if m.DonateDay == date {
			q.Coin = max(q.Coin, m.TodayCoin)
			q.Badge = max(q.Badge, m.TodayBadge)
		}
		if q.Coin < 0 || q.Badge < 0 {
			return errors.New(constUtil.CalendarQuota75Invalid)
		}
		if err = calendarQuota75Save(tx, m.PlayerID, constUtil.CalendarQuota75Guild, date, q); err != nil {
			return err
		}
	}
	var q calendarGuildQuota75
	if _, err = calendarQuota75Load(tx, m.PlayerID, constUtil.CalendarQuota75Guild, day, &q); err != nil {
		return err
	}
	if q.Coin < 0 || q.Badge < 0 {
		return errors.New(constUtil.CalendarQuota75Invalid)
	}
	m.LastLoginDay = ""
	if q.Login {
		m.LastLoginDay = day
	}
	m.StaminaDay = ""
	if q.Stamina {
		m.StaminaDay = day
	}
	m.DonateDay = day
	m.TodayCoin = q.Coin
	m.TodayBadge = q.Badge
	current.Day = day
	return calendarQuota75Save(tx, m.PlayerID, constUtil.CalendarQuota75Guild, "current", current)
}
func calendarPetWeek75(tx *gorm.DB, state *db.P9PetMarketState, target string) (bool, error) {
	if !ServerGameClockContextBound(tx.Statement.Context) || state.PlayerID < 1 || state.WeekKey == target {
		return false, nil
	}
	if _, err := time.Parse("20060102", state.WeekKey); err != nil {
		return false, errors.New(constUtil.CalendarQuota75Invalid)
	}
	if err := calendarQuota75Save(tx, state.PlayerID, constUtil.CalendarQuota75Pet, state.WeekKey, state); err != nil {
		return false, err
	}
	var previous db.P9PetMarketState
	found, err := calendarQuota75Load(tx, state.PlayerID, constUtil.CalendarQuota75Pet, target, &previous)
	if err != nil || !found {
		return false, err
	}
	if previous.PlayerID != state.PlayerID || previous.WeekKey != target {
		return false, errors.New(constUtil.CalendarQuota75Invalid)
	}
	*state = previous
	return true, nil
}
func calendarGGODay75(tx *gorm.DB, id int64, rule string, state *ggoQuoteState, now time.Time) error {
	if !ServerGameClockContextBound(tx.Statement.Context) || state.Day == p1Day(now) {
		return nil
	}
	old, err := time.Parse("2006-01-02", state.Day)
	if err != nil {
		return errors.New(constUtil.CalendarQuota75Invalid)
	}
	day := p1Day(now)
	key := p6Digest([]byte(rule))[:24] + ":"
	if err = calendarQuota75Save(tx, id, constUtil.CalendarQuota75GGO, key+state.Day, state); err != nil {
		return err
	}
	var previous ggoQuoteState
	found, err := calendarQuota75Load(tx, id, constUtil.CalendarQuota75GGO, key+day, &previous)
	if err != nil {
		return err
	}
	if found {
		if previous.Day != day {
			return errors.New(constUtil.CalendarQuota75Invalid)
		}
		*state = previous
		return nil
	}
	target, err := time.Parse("2006-01-02", day)
	if err != nil {
		return err
	}
	if target.Before(old) {
		state.FreeAt += target.Sub(old).Milliseconds()
		if state.FreeAt > now.UnixMilli() {
			state.FreeAt = now.UnixMilli()
		}
	}
	state.Day = day
	state.PaidUsed = 0
	return nil
}
