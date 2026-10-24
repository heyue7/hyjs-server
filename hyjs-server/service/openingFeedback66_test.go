package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"testing"
	"time"
)

func TestOpeningFeedback66AcceptedTrialStarsAndCold(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	loadOpenServerTaskFixture(t, f.s)
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	begin := today.AddDate(0, 0, -5)
	if err := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", constUtil.OpenServerActivityID).Updates(map[string]any{"enabled": true, "start_at": begin.UnixMilli(), "end_at": begin.AddDate(0, 0, 14).UnixMilli(), "reward_end_at": begin.AddDate(0, 0, 15).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePlayerOpeningCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Finite calendar preparation only; stars still come from actual accepted original battles.
	if err := db.GetIns().Save(&db.PlayerOpeningCalendar{PlayerID: f.id, StartAt: begin.UnixMilli(), CreatedAt: now.UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}).Error; err != nil {
		t.Fatal(err)
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	f.ctx = context.WithValue(f.ctx, constUtil.GameRequestContextKey, &pb.CSRushFightEnd{})
	query := func(want int32) {
		t.Helper()
		messages, err := f.s.openServerActivityAt(f.ctx, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(6), Tap: proto.Int32(2)}, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range messages[0].(*pb.SCNewOpenSevenDayByTitle).GetItemList() {
			if item.GetItemId() == 102 {
				found = true
				if item.GetCurrentNum() != want {
					t.Fatalf("real Trial stars got %d want %d", item.GetCurrentNum(), want)
				}
			}
		}
		if !found {
			t.Fatal("original day6 task102 missing")
		}
	}
	query(0)
	end := p7SectionPlayFloor(t, f, 1, 3, 96601)
	// The End save hook must record progress before any activity page query.
	var saved db.OpenServerActivityState
	if err := db.GetIns().Where("player_id=? AND task_id=?", f.id, 102).First(&saved).Error; err != nil || saved.Progress != 3 {
		t.Fatal("same transaction accepted stars", saved, err)
	}
	query(3)
	if _, err := f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	query(3)
	p7SectionPlayFloor(t, f, 2, 3, 96602)
	p7SectionPlayFloor(t, f, 3, 3, 96603)
	query(9)
	if _, err := f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 96604); err != nil {
		t.Fatal(err)
	}
	query(9)
	f.restart(t)
	query(9)
	// Another owner's scalar progress cannot satisfy this player's task.
	if err := db.GetIns().Create(&db.P7RushState{PlayerID: f.id + 100000, RecordStars: 90, TotalStars: 90}).Error; err != nil {
		t.Fatal(err)
	}
	query(9)
}

func TestOpeningFeedback66PurchaseSpendAndReplay(t *testing.T) {
	f := p4Setup(t)
	loadOpenServerTaskFixture(t, f.s)
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(10000); return nil }); err != nil {
		t.Fatal(err)
	}
	var task db.OpenServerActivityTask
	if err := db.GetIns().First(&task, 20).Error; err != nil {
		t.Fatal(err)
	}
	req := &pb.CSGetOpenServerReward{ItemId: proto.Int32(task.ID)}
	ctx := f.ctx
	buy := func(stamp int64) ([]proto.Message, error) {
		return f.s.openServerActivityAt(ctx, f.id, req, stamp, now)
	}
	feedback66RejectProfileSave(t, func() error { _, err := buy(96610); return err })
	source := "opening:purchase:" + strconv.FormatInt(f.id, 10) + ":96610:20"
	feedback66SpendFact(t, f, source, 0, "")
	first, err := buy(96610)
	if err != nil {
		t.Fatal(err)
	}
	feedback66SpendFact(t, f, source, task.NewPrice, p1Day(now))
	repeat, err := buy(96610)
	if err != nil || !proto.Equal(first[0], repeat[0]) {
		t.Fatal("opening receipt", err)
	}
	feedback66SpendFact(t, f, source, task.NewPrice, p1Day(now))
	f.restart(t)
	repeat, err = buy(96610)
	if err != nil || !proto.Equal(first[0], repeat[0]) {
		t.Fatal("cold opening receipt", err)
	}
	feedback66SpendFact(t, f, source, task.NewPrice, p1Day(now))
}
