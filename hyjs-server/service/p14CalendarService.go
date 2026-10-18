package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"time"
)

// p14PublishedSlots 逐行验证原阶段1..19的已发布时间，不提供虚构日期或报名资格。
func p14PublishedSlots(tx *gorm.DB, season int32) ([]db.P14TimeSlot, error) {
	var rows []db.P14TimeSlot
	if err := tx.Where("season_index=?", season).Order("start_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	var end int64
	for _, row := range rows {
		if row.Stage < 1 || row.Stage > 19 || row.StartAt <= 0 || row.EndAt <= row.StartAt || row.StartAt < end || row.Description == "" || row.Source == "" {
			return nil, errors.New("跨服已发布原阶段日历损坏或重叠")
		}
		end = row.EndAt
	}
	return rows, nil
}

// p14PublishedStage 优先读取明确发布的当前窗口，保留旧真实报名窗口的兼容路径。
func p14PublishedStage(tx *gorm.DB, policy *db.P14SeasonPolicy, now time.Time) (int32, int64, error) {
	rows, err := p14PublishedSlots(tx, policy.SeasonIndex)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) == 0 {
		return p14CurrentStage(policy, now), policy.RegisterEnd, nil
	}
	if !policy.Enabled || policy.SeasonIndex <= 0 {
		return 0, 0, nil
	}
	for _, row := range rows {
		if row.Stage == 2 && (row.StartAt != policy.RegisterStart || row.EndAt != policy.RegisterEnd) {
			return 0, 0, errors.New("跨服原报名窗口与发布日历不一致")
		}
	}
	for _, row := range rows {
		if now.UnixMilli() >= row.StartAt && now.UnixMilli() < row.EndAt {
			return row.Stage, row.EndAt, nil
		}
	}
	return 0, 0, nil
}

// p14CalendarReply 按原赛季编号返回实际时间列表，历史赛季不借用当前日历。
func p14CalendarReply(tx *gorm.DB, policy *db.P14SeasonPolicy, request *pb.CSKFTimeInfo) (*pb.SCKFTimeInfo, error) {
	season := request.GetPeakMatchIndex()
	if season == 0 {
		season = policy.SeasonIndex
	}
	if season <= 0 {
		return nil, errors.New("跨服真实赛季日历尚未发布")
	}
	rows, err := p14PublishedSlots(tx, season)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("跨服该赛季原时间列表尚未发布")
	}
	reply := &pb.SCKFTimeInfo{PeakMatchIndex: proto.Int32(season)}
	for _, row := range rows {
		reply.TimeList = append(reply.TimeList, &pb.KFTimeSlot{Stage: proto.Int32(row.Stage), Start: proto.Int64(row.StartAt), End: proto.Int64(row.EndAt), Desc: proto.String(row.Description)})
	}
	return reply, nil
}
