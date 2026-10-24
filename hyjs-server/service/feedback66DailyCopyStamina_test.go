package service

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// The recovered ActivityCopyPage.BattleEnd deletes End.cost after SCPlayer may
// already have set the wallet. Exercise that late callback against actual replies.
func TestFeedback66DailyCopyPrepaidStaminaProjectionAndCold(t *testing.T) {
	for _, chapter := range []string{"1", "5"} {
		for _, win := range []int32{0, 1} {
			name := chapter + "/loss"
			if win == 1 {
				name = chapter + "/win"
			}
			t.Run(name, func(t *testing.T) {
				s, ctx, path := newDailyCopyTest(t)
				id := dailyCopyTestPlayer(t, ctx, "feedback66-daily-stamina", 100)
				now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
				typ := s.table("DailyCopyChapter", "ChapterId", chapter)["Type"]
				if s.table("DailyCopy", "Type", typ)["Stamina"] != "10" {
					t.Fatal("original experience-copy cost changed")
				}
				startRequest := &pb.CSDailyCopyFightStart{ChapterId: proto.String(chapter)}
				started, err := s.dailyCopyAt(ctx, id, startRequest, 66001, now)
				if err != nil {
					t.Fatal(err)
				}
				start := started[0].(*pb.SCDailyCopyFightStart)
				snapshot := func(messages []proto.Message) int32 {
					t.Helper()
					for _, message := range messages {
						if player, ok := message.(*pb.SCPlayer); ok {
							return player.GetPlayer().GetStamina()
						}
					}
					t.Fatal("authoritative stamina snapshot missing")
					return 0
				}
				if snapshot(started) != 190 {
					t.Fatal("Start must deduct original ten stamina exactly once")
				}
				resumed, err := s.dailyCopyAt(ctx, id, startRequest, 66002, now)
				if err != nil || !proto.Equal(start, resumed[0]) || snapshot(resumed) != 190 {
					t.Fatal("Start recovery charged twice", err)
				}
				end := dailyCopyWin(start, chapter)
				if win == 0 {
					end.IsWin = proto.Int32(0)
					end.OptRecord.AttackerTotalLeftHp = proto.Float64(0)
					end.OptRecord.DefenderTotalLeftHp = proto.Float64(1)
				}
				settled, err := s.dailyCopyAt(ctx, id, end, 66003, now)
				if err != nil {
					t.Fatal(err)
				}
				projected := settled[0].(*pb.SCDailyCopyFightEnd)
				checkClient := func(messages []proto.Message) {
					t.Helper()
					raw, err := proto.Marshal(messages[0])
					if err != nil {
						t.Fatal(err)
					}
					delivered := &pb.SCDailyCopyFightEnd{}
					if err := proto.Unmarshal(raw, delivered); err != nil {
						t.Fatal(err)
					}
					balance := snapshot(messages)
					for _, cost := range delivered.GetCost() {
						if cost.GetId() == "stamina" {
							balance -= cost.GetNum()
						}
					}
					if balance != 190 {
						t.Fatal("late native Delete(cost) charged prepaid stamina again", balance)
					}
					if delivered.GetIsWin() != win {
						t.Fatal("projection changed the accepted outcome")
					}
				}
				checkClient(settled)
				var row db.DailyCopyFight
				if err := db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil {
					t.Fatal(err)
				}
				frozen := append([]byte(nil), row.Response...)
				receipt := &pb.SCDailyCopyFightEnd{}
				if err := proto.Unmarshal(frozen, receipt); err != nil {
					t.Fatal(err)
				}
				if len(receipt.GetCost()) != 1 || receipt.Cost[0].GetId() != "stamina" || receipt.Cost[0].GetNum() != 10 || row.Stamina != 10 {
					t.Fatal("actual paid cost or frozen replay evidence lost", receipt)
				}
				// This is the pre-fix native callback defect with the retained real receipt.
				if snapshot(settled)-receipt.Cost[0].GetNum() != 180 {
					t.Fatal("fixture does not expose duplicate local deduction")
				}
				if !proto.Equal(receipt.GetGet(), projected.GetGet()) || receipt.GetStarNum() != projected.GetStarNum() || receipt.GetLastTime() != projected.GetLastTime() {
					t.Fatal("projection changed reward or quota")
				}
				if win == 0 && (proto.Size(projected.GetGet()) != 0 || projected.GetStarNum() != 0) {
					t.Fatal("loss granted a reward or stars")
				}
				retry, err := s.dailyCopyAt(ctx, id, end, 66004, now)
				if err != nil || !proto.Equal(projected, retry[0]) {
					t.Fatal("End retry changed projected result", err)
				}
				checkClient(retry)
				sqlDB, err := db.GetIns().DB()
				if err != nil {
					t.Fatal(err)
				}
				if err := sqlDB.Close(); err != nil {
					t.Fatal(err)
				}
				if err := db.InitGame(ctx, path); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { conn, _ := db.GetIns().DB(); conn.Close() })
				cold, err := s.dailyCopyAt(ctx, id, end, 66005, now)
				if err != nil || !proto.Equal(projected, cold[0]) {
					t.Fatal("cold End replay changed result", err)
				}
				checkClient(cold)
				var after db.DailyCopyFight
				if err := db.GetIns().First(&after, "fight_id=?", start.GetFightId()).Error; err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(frozen, after.Response) {
					t.Fatal("network projection rewrote historical receipt")
				}
				var profile db.Profile
				if err := db.GetIns().First(&profile, id).Error; err != nil {
					t.Fatal(err)
				}
				player, _, err := decodeProfile(&profile)
				if err != nil || player.GetStamina() != 190 {
					t.Fatal("End or replay changed prepaid persisted stamina", err)
				}
				var quota db.DailyCopyState
				if err := db.GetIns().First(&quota, "player_id=? AND type=?", id, typ).Error; err != nil || quota.Used != 1 {
					t.Fatal("copy quota charged more than once", quota.Used, err)
				}
			})
		}
	}
}
