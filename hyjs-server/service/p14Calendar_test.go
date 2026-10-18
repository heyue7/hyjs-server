package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestP14CrossCalendar 核对全部原1..19阶段、明确日历、历史编号、重启与未授予晋级资格。
func TestP14CrossCalendarAllOriginalStagesExplicitTimesHistoryAndRecovery(t *testing.T) {
	f, ids, _, now := p14Setup(t)
	if _, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFTimeInfo{}, now); err == nil {
		t.Fatal("invented missing calendar")
	}
	base := now.Add(-90 * time.Minute)
	for stage := int32(1); stage <= 19; stage++ {
		start := base.Add(time.Duration(stage-1) * time.Hour)
		if err := db.GetIns().Create(&db.P14TimeSlot{SeasonIndex: 7, Stage: stage, StartAt: start.UnixMilli(), EndAt: start.Add(time.Hour).UnixMilli(), Description: fmt.Sprintf("隔离已发布原阶段%d", stage), Source: "明确fixture，原运营日期未知"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"register_start": base.Add(time.Hour).UnixMilli(), "register_end": base.Add(2 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterGame{}, 11, now); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFTimeInfo{PeakMatchIndex: proto.Int32(7)}, now)
	if err != nil {
		t.Fatal(err)
	}
	reply := messages[0].(*pb.SCKFTimeInfo)
	if reply.GetPeakMatchIndex() != 7 || len(reply.TimeList) != 19 {
		t.Fatal("all original phases", reply)
	}
	for i, row := range reply.TimeList {
		if row.GetStage() != int32(i+1) || row.GetStart() != base.Add(time.Duration(i)*time.Hour).UnixMilli() || row.GetEnd()-row.GetStart() != 3600000 {
			t.Fatal("published original time slot", row)
		}
	}
	encoded, err := proto.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip := &pb.SCKFTimeInfo{}
	if proto.Unmarshal(encoded, roundtrip) != nil || !proto.Equal(roundtrip, reply) {
		t.Fatal("original binary calendar")
	}
	// Commit the actual cutoff through its producer; a pending query must roll back.
	if err = f.s.TickP14Season(f.ctx, base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	final16 := base.Add(7*time.Hour + time.Minute)
	messages, err = f.s.p14Query(f.ctx, ids[0], &pb.CSKFOpenFrightServerInfo{}, final16)
	if err == nil {
		t.Fatal("calendar assigned unverified qualification or fake failure stage")
	}
	if _, err = f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 12, final16); err == nil {
		t.Fatal("upload after published registration")
	}
	if _, err = f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterGame{}, 11, final16); err != nil {
		t.Fatal("old actual receipt not restored", err)
	}
	history := db.P14TimeSlot{SeasonIndex: 6, Stage: 2, StartAt: base.Add(-48 * time.Hour).UnixMilli(), EndAt: base.Add(-47 * time.Hour).UnixMilli(), Description: "已发布上赛季报名", Source: "隔离历史日历"}
	if err = db.GetIns().Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	messages, err = f.s.p14Query(f.ctx, ids[0], &pb.CSKFTimeInfo{PeakMatchIndex: proto.Int32(6)}, now)
	if err != nil || len(messages[0].(*pb.SCKFTimeInfo).TimeList) != 1 {
		t.Fatal("history uses actual season", err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	messages, err = f.s.p14Query(f.ctx, ids[0], &pb.CSKFTimeInfo{}, now)
	if err != nil || !proto.Equal(messages[0], reply) {
		t.Fatal("restart changed published calendar", err)
	}
	if err = db.GetIns().Model(&db.P14TimeSlot{}).Where("season_index=7 AND stage=1").Update("end_at", base.Add(2*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p14PublishedSlots(db.GetIns(), 7); err == nil {
		t.Fatal("overlapping newly published calendar accepted")
	}
	messages, err = f.s.p14Query(f.ctx, ids[0], &pb.CSKFTimeInfo{}, now)
	if err != nil || !proto.Equal(messages[0], reply) {
		t.Fatal("changed publication overwrote sealed actual season calendar", err)
	}
}
