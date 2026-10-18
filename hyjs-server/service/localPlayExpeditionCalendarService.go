package service

import (
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type localPlayExpeditionQuota struct {
	Day  string `json:"day"`
	Used int32  `json:"used"`
}

// 日账本仅由已持有角色锁的本玩法事务维护，等级经验不参与回拨。
func expeditionDayArchive(tx *gorm.DB, id int64, day string) (*localPlayExpeditionQuota, error) {
	if !tx.Migrator().HasTable(&db.LocalPlayActivityOperation{}) {
		return nil, errLocalPlayActivitiesInvalid
	}
	var row db.LocalPlayActivityOperation
	err := tx.First(&row, "player_id=? AND request_id=?", id, constUtil.LocalPlayExpeditionDayPrefix+day).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var q localPlayExpeditionQuota
	if row.Action != constUtil.LocalPlayExpeditionDayAction || row.ActivityID != constUtil.LocalPlayExpeditionActivity || json.Unmarshal(row.Response, &q) != nil || q.Day != day || q.Used < 0 {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &q, nil
}

func saveP7ExpeditionCalendar(tx *gorm.DB, state *db.P7ExpeditionState, real time.Time) error {
	if !GameCalendarContextBound(tx.Statement.Context) {
		return tx.Save(state).Error
	}
	if localPlayActivityOwner(tx) != state.PlayerID || !validLocalPlayAwakeDay(state.Day) || state.RefreshUsed < 0 {
		return errLocalPlayActivitiesInvalid
	}
	prior, err := expeditionDayArchive(tx, state.PlayerID, state.Day)
	if err != nil {
		return err
	}
	if prior != nil && prior.Used > state.RefreshUsed {
		return errLocalPlayActivitiesInvalid
	}
	raw, err := json.Marshal(localPlayExpeditionQuota{Day: state.Day, Used: state.RefreshUsed})
	if err != nil {
		return err
	}
	row := db.LocalPlayActivityOperation{PlayerID: state.PlayerID, RequestID: constUtil.LocalPlayExpeditionDayPrefix + state.Day, Action: constUtil.LocalPlayExpeditionDayAction, ActivityID: constUtil.LocalPlayExpeditionActivity, Response: raw, CreatedAt: real.UnixMilli()}
	if err = tx.Save(&row).Error; err != nil {
		return err
	}
	return tx.Save(state).Error
}

func refreshP7ExpeditionCalendar(tx *gorm.DB, state *db.P7ExpeditionState, real time.Time) error {
	today := p1Day(LocalPlayAt(tx.Statement.Context, real))
	if !GameCalendarContextBound(tx.Statement.Context) {
		if state.Day != today {
			state.Day, state.RefreshUsed = today, 0
		}
		return nil
	}
	if localPlayActivityOwner(tx) != state.PlayerID || state.RefreshUsed < 0 {
		return errLocalPlayActivitiesInvalid
	}
	if state.Day == "" {
		state.Day = today
	}
	if !validLocalPlayAwakeDay(state.Day) {
		return errLocalPlayActivitiesInvalid
	}
	previous, err := expeditionDayArchive(tx, state.PlayerID, state.Day)
	if err != nil {
		return err
	}
	if previous != nil {
		state.RefreshUsed = max(state.RefreshUsed, previous.Used)
	}
	if state.Day != today {
		if err = saveP7ExpeditionCalendar(tx, state, real); err != nil {
			return err
		}
		next, err := expeditionDayArchive(tx, state.PlayerID, today)
		if err != nil {
			return err
		}
		state.Day, state.RefreshUsed = today, 0
		if next != nil {
			state.RefreshUsed = next.Used
		}
	}
	return nil
}

type localPlayExpeditionClock struct {
	PlayerID       int64  `json:"playerId"`
	Serial         string `json:"serial"`
	QuoteHash      string `json:"quoteHash"`
	StartAt        int64  `json:"startAt"`
	EndAt          int64  `json:"endAt"`
	AcceptedOffset int64  `json:"acceptedOffset"`
	BusinessAt     int64  `json:"businessAt"`
	Hash           string `json:"hash"`
}

func expeditionClockHash(c localPlayExpeditionClock) string {
	c.Hash = ""
	raw, _ := json.Marshal(c)
	return p6Digest(raw)
}

// 接受原任务时创建一次私有时钟来源，报价与真实Start/End字节不变。
// 无来源的旧已接任务永远沿真实时钟，读取不会为它补账本。
func freezeP7ExpeditionCalendar(tx *gorm.DB, m db.P7ExpeditionMission, real time.Time) error {
	if !GameCalendarContextBound(tx.Statement.Context) {
		return nil
	}
	if !tx.Migrator().HasTable(&db.LocalPlayActivityOperation{}) {
		return errLocalPlayActivitiesInvalid
	}
	offset := GameCalendarOffsetDays(tx.Statement.Context)
	if localPlayActivityOwner(tx) != m.PlayerID || offset < -3650 || offset > 3650 || m.State != constUtil.P7ExpeditionRunning || m.StartAt != real.UnixMilli() {
		return errLocalPlayActivitiesInvalid
	}
	q, err := p7ReadExpeditionQuote(m)
	if err != nil {
		return err
	}
	if m.EndAt != real.Add(time.Duration(q.Hours)*time.Hour).UnixMilli() {
		return errLocalPlayActivitiesInvalid
	}
	clock := localPlayExpeditionClock{PlayerID: m.PlayerID, Serial: m.Serial, QuoteHash: m.QuoteHash, StartAt: m.StartAt, EndAt: m.EndAt, AcceptedOffset: offset, BusinessAt: LocalPlayAt(tx.Statement.Context, real).UnixMilli()}
	clock.Hash = expeditionClockHash(clock)
	raw, err := json.Marshal(clock)
	if err != nil {
		return err
	}
	return tx.Create(&db.LocalPlayActivityOperation{PlayerID: m.PlayerID, RequestID: constUtil.LocalPlayExpeditionClockPrefix + m.Serial, Action: constUtil.LocalPlayExpeditionClockAction, ActivityID: constUtil.LocalPlayExpeditionActivity, Response: raw, CreatedAt: real.UnixMilli()}).Error
}

// P10传入的是帮助者ctx；所有时钟查询都明确使用任务主人，不能借帮助者偏移推进任务。
// 未启动或未建表的旧服务保持原行为，不查询缺失表；不增加逆序Profile行锁。
func (s *GameService) expeditionOwnerOffset(tx *gorm.DB, id int64) (int64, error) {
	if !s.LocalPlayToolsEnabled || !tx.Migrator().HasTable(&db.LocalPlayPolicy{}) || !tx.Migrator().HasTable(&db.LocalPlayClockState{}) {
		return 0, nil
	}
	enabled, err := localPlayPolicyEnabled(tx)
	if err != nil || !enabled {
		return 0, err
	}
	var owner db.Profile
	if err = tx.Select("id", "uid", "player").First(&owner, id).Error; err != nil {
		return 0, err
	}
	if !localWalletOwner(&owner) {
		return 0, nil
	}
	var state db.LocalPlayClockState
	err = tx.First(&state, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if state.OffsetDays < -3650 || state.OffsetDays > 3650 || state.Revision < 0 {
		return 0, errLocalPlayActivitiesInvalid
	}
	return state.OffsetDays, nil
}

func (s *GameService) expeditionClock(tx *gorm.DB, m db.P7ExpeditionMission) (*localPlayExpeditionClock, error) {
	if !s.LocalPlayToolsEnabled || !tx.Migrator().HasTable(&db.LocalPlayActivityOperation{}) {
		return nil, nil
	}
	var row db.LocalPlayActivityOperation
	err := tx.First(&row, "player_id=? AND request_id=?", m.PlayerID, constUtil.LocalPlayExpeditionClockPrefix+m.Serial).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c localPlayExpeditionClock
	if row.Action != constUtil.LocalPlayExpeditionClockAction || row.ActivityID != constUtil.LocalPlayExpeditionActivity || json.Unmarshal(row.Response, &c) != nil || c.Hash != expeditionClockHash(c) || c.PlayerID != m.PlayerID || c.Serial != m.Serial || c.QuoteHash != m.QuoteHash || c.StartAt != m.StartAt || c.EndAt != m.EndAt || c.StartAt != row.CreatedAt || c.AcceptedOffset < -3650 || c.AcceptedOffset > 3650 || c.BusinessAt != c.StartAt+c.AcceptedOffset*24*time.Hour.Milliseconds() {
		return nil, errLocalPlayActivitiesInvalid
	}
	q, err := p7ReadExpeditionQuote(m)
	if err != nil {
		return nil, err
	}
	if c.EndAt-c.StartAt != int64(q.Hours)*time.Hour.Milliseconds() {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &c, nil
}

// 输入和持久事件保持真实轴；仅比较到期/事件可见性时使用接受偏移的差值。
func (s *GameService) p7ExpeditionMissionNow(tx *gorm.DB, m db.P7ExpeditionMission, real time.Time) (time.Time, error) {
	c, err := s.expeditionClock(tx, m)
	if err != nil || c == nil {
		return real, err
	}
	offset, err := s.expeditionOwnerOffset(tx, m.PlayerID)
	if err != nil {
		return time.Time{}, err
	}
	return real.AddDate(0, 0, int(offset-c.AcceptedOffset)), nil
}

// 原UI按ServerTimer显示：新任务展示入场业务轴，旧任务按当前主人偏移平移，保真实剩余。
func (s *GameService) expeditionDisplayOffset(tx *gorm.DB, m db.P7ExpeditionMission) (int64, error) {
	c, err := s.expeditionClock(tx, m)
	if err != nil {
		return 0, err
	}
	if c != nil {
		return c.AcceptedOffset, nil
	}
	return s.expeditionOwnerOffset(tx, m.PlayerID)
}

func (s *GameService) p7ProjectExpeditionCalendar(tx *gorm.DB, m db.P7ExpeditionMission, real time.Time) (*pb.ExpeditionMissionInfo, error) {
	now, err := s.p7ExpeditionMissionNow(tx, m, real)
	if err != nil {
		return nil, err
	}
	out, err := p7ProjectExpedition(m, now)
	if err != nil {
		return nil, err
	}
	if m.StartAt == 0 && m.EndAt == 0 {
		return out, nil
	}
	offset, err := s.expeditionDisplayOffset(tx, m)
	if err != nil {
		return nil, err
	}
	out.StartTime = proto.Int64(m.StartAt + offset*24*time.Hour.Milliseconds())
	out.EndTime = proto.Int64(m.EndAt + offset*24*time.Hour.Milliseconds())
	return out, nil
}

func (s *GameService) p7ProjectExpeditionCalendarEvent(tx *gorm.DB, m db.P7ExpeditionMission, e db.P7ExpeditionEvent) (*pb.ExpeditionStoryEvent, error) {
	offset, err := s.expeditionDisplayOffset(tx, m)
	if err != nil {
		return nil, err
	}
	out := p7ProjectExpeditionEvent(e)
	out.EventTime = proto.Int64(e.EventAt + offset*24*time.Hour.Milliseconds())
	return out, nil
}
