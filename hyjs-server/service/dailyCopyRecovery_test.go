package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestDailyCopyExhaustedNativeRecovery 保留已扣费次数，用原状态投影让客户端可恢复同一场，结算后资格清空。
func TestDailyCopyExhaustedNativeRecovery(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "daily-native-recovery", 75)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	started, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 101, now)
	if err != nil {
		t.Fatal(err)
	}
	start := started[0].(*pb.SCDailyCopyFightStart)
	check := func(wantRecover bool) {
		t.Helper()
		messages, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyState{}, 102, now)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, item := range messages[0].(*pb.SCDailyCopyState).GetItems() {
			if item.GetType() != "TreasureExp" {
				if wantRecover && item.GetIsOpen() != 0 {
					t.Fatal("unrelated type opened during unfinished battle")
				}
				continue
			}
			found = true
			if (item.GetLastTime() == 1) != wantRecover {
				t.Fatal("native recovery projection", item)
			}
			for _, chapter := range item.GetDailyCopyState() {
				if wantRecover && (chapter.GetIsOpen() == 1) != (chapter.GetChapterId() == "5") {
					t.Fatal("wrong chapter eligible for frozen recovery")
				}
			}
		}
		if !found {
			t.Fatal("frozen resource type missing")
		}
		var state db.DailyCopyState
		if err := db.GetIns().Where("player_id = ? AND type = ?", id, "TreasureExp").First(&state).Error; err != nil {
			t.Fatal(err)
		}
		if state.Used != 1 || state.Limit != 1 {
			t.Fatal("native recovery changed consumed quota", state)
		}
	}
	check(true)
	retried, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 103, now)
	if err != nil || !proto.Equal(start, retried[0]) {
		t.Fatal("frozen native start changed", err)
	}
	check(true)
	if _, err = s.dailyCopyAt(ctx, id, dailyCopyWin(start, "5"), 104, now); err != nil {
		t.Fatal(err)
	}
	check(false)
}
