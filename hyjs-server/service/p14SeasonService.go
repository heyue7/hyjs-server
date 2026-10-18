package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"time"
)

func p14AuditionRowHash(row *db.P14AuditionRoster) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14AuditionRosterHash(rows []db.P14AuditionRoster) string {
	var hashes []string
	for _, row := range rows {
		hashes = append(hashes, row.SourceHash)
	}
	raw, _ := json.Marshal(hashes)
	return p6Digest(raw)
}
func p14AuditionBoundaryHash(boundary *db.P14AuditionBoundary) string {
	copy := *boundary
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14AuditionValidate(row *db.P14AuditionRoster) error {
	if row.SeasonIndex <= 0 || row.ServerID <= 0 || row.PlayerID <= 0 || row.Revision <= 0 || row.CapturedAt <= 0 || row.ClientVersion == "" || row.EngineVersion == "" || len(row.ConfigurationHash) != 64 || p14AuditionRowHash(row) != row.SourceHash || p14SnapshotDigest(row.Snapshot, row.Battle) != row.SnapshotHash {
		return errors.New("海选冻结联合身份/阵容来源SHA损坏")
	}
	battle := &pb.BattlePlayer{}
	formation := &pb.FormationSimpleInfo{}
	if proto.Unmarshal(row.Battle, battle) != nil || proto.Unmarshal(row.Snapshot, formation) != nil || battle.GetPlayerId() != row.PlayerID || int64(battle.GetServerId()) != row.ServerID || formation.GetFightPower() != row.FightPower || len(battle.Formation) == 0 {
		return errors.New("海选冻结原身份/完整阵容损坏")
	}
	return nil
}

// p14AuditionRead 验证整份名册和真实截止/日历，查询不会借当前阵容改写历史。
func p14AuditionRead(tx *gorm.DB, season int32, server int64) (*db.P14AuditionBoundary, []db.P14AuditionRoster, error) {
	var boundary db.P14AuditionBoundary
	if err := tx.First(&boundary, "season_index=? AND server_id=?", season, server).Error; err != nil {
		return nil, nil, err
	}
	if boundary.RegisterStart <= 0 || boundary.RegisterEnd <= boundary.RegisterStart || boundary.FrozenAt < boundary.RegisterEnd || p6Digest(boundary.Calendar) != boundary.CalendarHash || p14AuditionBoundaryHash(&boundary) != boundary.SourceHash {
		return nil, nil, errors.New("海选封存截止/日历SHA损坏")
	}
	var rows []db.P14AuditionRoster
	if err := tx.Where("season_index=? AND server_id=?", season, server).Order("player_id").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	if int64(len(rows)) != boundary.Count || p14AuditionRosterHash(rows) != boundary.RosterHash {
		return nil, nil, errors.New("海选完整截止名册SHA/数量损坏")
	}
	for _, row := range rows {
		if err := p14AuditionValidate(&row); err != nil {
			return nil, nil, err
		}
		if row.CapturedAt < boundary.RegisterStart || row.CapturedAt >= boundary.RegisterEnd {
			return nil, nil, errors.New("海选冻结阵容越过真实报名截止")
		}
	}
	return &boundary, rows, nil
}

// p14FreezeAuditionTX 只冻结本部署服的权威报名存档；别服迟到签名快照不会改本服海选名册。
func (s *GameService) p14FreezeAuditionTX(tx *gorm.DB, policy *db.P14SeasonPolicy, now time.Time) error {
	if !policy.Enabled || policy.SeasonIndex <= 0 || now.UnixMilli() < policy.RegisterEnd {
		return nil
	}
	_, _, err := p14AuditionRead(tx, policy.SeasonIndex, s.OriginalServerID())
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	slots, err := p14PublishedSlots(tx, policy.SeasonIndex)
	if err != nil {
		return err
	}
	registered, audition := false, false
	for _, slot := range slots {
		if slot.Stage == constUtil.P14RegisterStage {
			registered = slot.StartAt == policy.RegisterStart && slot.EndAt == policy.RegisterEnd
		}
		if slot.Stage == constUtil.P14AuditionStage {
			audition = true
		}
	}
	// 旧只发布报名窗口的兼容记录不虚构海选，明确海选窗口发布后才封存并授予此阶段资格。
	if !registered || !audition {
		return nil
	}
	var members []db.P14Member
	if err = tx.Where("season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Order("player_id").Find(&members).Error; err != nil {
		return err
	}
	rows := make([]db.P14AuditionRoster, 0, len(members))
	for _, member := range members {
		row := db.P14AuditionRoster{SeasonIndex: member.SeasonIndex, ServerID: member.ServerID, PlayerID: member.PlayerID, Revision: member.Revision, CapturedAt: member.CapturedAt, FightPower: member.FightPower, Snapshot: append([]byte(nil), member.Snapshot...), Battle: append([]byte(nil), member.Battle...), SnapshotHash: member.SnapshotHash, ClientVersion: member.ClientVersion, EngineVersion: member.EngineVersion, ConfigurationHash: member.ConfigurationHash}
		row.SourceHash = p14AuditionRowHash(&row)
		if err = p14AuditionValidate(&row); err != nil {
			return err
		}
		if row.CapturedAt < policy.RegisterStart || row.CapturedAt >= policy.RegisterEnd {
			return errors.New("原本服海选名册存在报名截止外的阵容")
		}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		rows = append(rows, row)
	}
	raw, err := json.Marshal(slots)
	if err != nil {
		return err
	}
	boundary := &db.P14AuditionBoundary{SeasonIndex: policy.SeasonIndex, ServerID: s.OriginalServerID(), RegisterStart: policy.RegisterStart, RegisterEnd: policy.RegisterEnd, FrozenAt: now.UnixMilli(), Count: int64(len(rows)), Calendar: raw, CalendarHash: p6Digest(raw), RosterHash: p14AuditionRosterHash(rows), Source: constUtil.P14AuditionSource}
	boundary.SourceHash = p14AuditionBoundaryHash(boundary)
	return tx.Create(boundary).Error
}

func (s *GameService) TickP14Season(ctx context.Context, now time.Time) error {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		return s.p14FreezeAuditionTX(tx, &policy, now)
	})
	if err != nil {
		return err
	}
	return s.TickP14Audition(ctx, now)
}

// p14SeasonCalendar 已封存赛季继续采用本服真实冻结的原日历，后续发布改动不追溯改写海选窗口。
func (s *GameService) p14SeasonCalendar(tx *gorm.DB, season int32) ([]db.P14TimeSlot, error) {
	boundary, _, err := p14AuditionRead(tx, season, s.OriginalServerID())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p14PublishedSlots(tx, season)
	}
	if err != nil {
		return nil, err
	}
	var slots []db.P14TimeSlot
	if err = json.Unmarshal(boundary.Calendar, &slots); err != nil {
		return nil, err
	}
	return slots, nil
}

func (s *GameService) p14SeasonStage(tx *gorm.DB, policy *db.P14SeasonPolicy, now time.Time) (int32, int64, error) {
	var boundary db.P14AuditionBoundary
	err := tx.First(&boundary, "season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p14PublishedStage(tx, policy, now)
	}
	if err != nil {
		return 0, 0, err
	}
	if !policy.Enabled {
		return 0, 0, nil
	}
	slots, err := s.p14SeasonCalendar(tx, policy.SeasonIndex)
	if err != nil {
		return 0, 0, err
	}
	for _, slot := range slots {
		if now.UnixMilli() >= slot.StartAt && now.UnixMilli() < slot.EndAt {
			return slot.Stage, slot.EndAt, nil
		}
	}
	return 0, 0, nil
}

func (s *GameService) p14SeasonCalendarReply(tx *gorm.DB, policy *db.P14SeasonPolicy, request *pb.CSKFTimeInfo) (*pb.SCKFTimeInfo, error) {
	season := request.GetPeakMatchIndex()
	if season == 0 {
		season = policy.SeasonIndex
	}
	if season <= 0 {
		return nil, errors.New("跨服真实赛季日历尚未发布")
	}
	slots, err := s.p14SeasonCalendar(tx, season)
	if err != nil {
		return nil, err
	}
	if len(slots) == 0 {
		return nil, errors.New("跨服该赛季原时间列表尚未发布")
	}
	reply := &pb.SCKFTimeInfo{PeakMatchIndex: proto.Int32(season)}
	for _, slot := range slots {
		reply.TimeList = append(reply.TimeList, &pb.KFTimeSlot{Stage: proto.Int32(slot.Stage), Start: proto.Int64(slot.StartAt), End: proto.Int64(slot.EndAt), Desc: proto.String(slot.Description)})
	}
	return reply, nil
}

func (s *GameService) p14AuditionEntered(tx *gorm.DB, policy *db.P14SeasonPolicy, id int64, stage int32) (int32, error) {
	if stage == 5 || stage == 6 {
		qualified, err := s.p14AuditionQualificationRead(tx.Statement.Context, tx, policy.SeasonIndex, s.OriginalServerID())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		var players []int64
		if err = json.Unmarshal(qualified.Players, &players); err != nil {
			return 0, err
		}
		for _, player := range players {
			if player == id {
				return 1, nil
			}
		}
		return 0, nil
	}
	if stage != constUtil.P14AuditionBeforeStage && stage != constUtil.P14AuditionStage {
		return 0, nil
	}
	_, rows, err := p14AuditionRead(tx, policy.SeasonIndex, s.OriginalServerID())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if row.PlayerID == id {
			return 1, nil
		}
	}
	return 0, nil
}
