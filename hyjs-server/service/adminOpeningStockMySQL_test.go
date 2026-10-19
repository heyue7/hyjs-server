package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"sync"
	"testing"
	"time"
)

// Both native requests establish their RR snapshots before waiting on the same
// original task lock. This deterministically exposes a stale stock snapshot.
func TestAdminOpeningRepeatMySQLLastStockAndCold(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("isolated managed MySQL required")
	}
	f, actor := reuseFixture(t, true)
	if err := db.GetIns().Model(&db.OpenServerActivityTask{}).Where("id=?", 20).Updates(map[string]any{"target": 1, "used_stock": 0, "reward": "coin,7", "new_price": 400}).Error; err != nil {
		t.Fatal(err)
	}
	f.s.table("ActivityNewOpen7Day", "id", "20")["reward"] = "coin,7"
	var ids []int64
	var diamondBefore, coinBefore int64
	for i := 0; i < 2; i++ {
		role, err := LoginAccount(f.ctx, fmt.Sprintf("last-stock-role-%d", i), "fixture-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, role.Id, "最后库存并发", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, role.Id, func(p *pb.Player, _ *model.GameState) error {
			p.Diamond = proto.Int64(2000)
			diamondBefore += p.GetDiamond()
			coinBefore += p.GetCoin()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, role.Id)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var definition model.AdminActivityDefinitionDTO
	for _, d := range definitions {
		if d.ID == "original_opening_14" {
			definition = d
		}
	}
	definition.Form.RepeatPolicy = "repeat"
	saved, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: definition.Name, Form: definition.Form, DefaultDays: 1, Enabled: true, Version: definition.Version})
	if err != nil {
		t.Fatal(err)
	}
	sources := []model.AdminActivityOccurrenceDTO{reusePublish(t, f, actor, *saved, 1, 1, "character-day"), reusePublish(t, f, actor, *saved, 2, 1, "character-day")}
	type winnerReceipt struct {
		id, stamp int64
		reply     proto.Message
	}
	receipts := []winnerReceipt{}
	var task db.OpenServerActivityTask
	for day, source := range sources {
		if day > 0 {
			reuseClockDay(t, f, actor)
		}
		// Phase 1 tests an absent stock row; phase 2 tests an already-existing zero.
		if day == 1 {
			if err = db.GetIns().Create(&db.AdminOpeningStock{OccurrenceID: source.ID, TaskID: 20}).Error; err != nil {
				t.Fatal(err)
			}
		}
		blocker := db.GetIns().Begin()
		if blocker.Error != nil {
			t.Fatal(blocker.Error)
		}
		if err = blocker.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, "id=?", 20).Error; err != nil {
			blocker.Rollback()
			t.Fatal(err)
		}
		arrived := make(chan int64, 2)
		callback := fmt.Sprintf("repeat-stock-barrier-%d", day)
		if err = db.GetIns().Callback().Query().Before("gorm:query").Register(callback, func(q *gorm.DB) {
			locked, ok := q.Statement.Clauses["FOR"].Expression.(clause.Locking)
			if q.Statement.Table == "open_server_activity_tasks" && ok && locked.Strength == "UPDATE" {
				arrived <- GameCalendarOwner(q.Statement.Context)
			}
		}); err != nil {
			blocker.Rollback()
			t.Fatal(err)
		}
		type outcome struct {
			id       int64
			messages []proto.Message
			err      error
		}
		outcomes := make(chan outcome, 2)
		var group sync.WaitGroup
		for _, id := range ids {
			bound := serverClockBindTest(t, f, id)
			group.Add(1)
			go func(id int64) {
				defer group.Done()
				messages, e := f.s.openServerActivityAt(bound, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}, int64(86001+day), LocalPlayNow(bound))
				outcomes <- outcome{id, messages, e}
			}(id)
		}
		seen := map[int64]bool{}
		for len(seen) < 2 {
			select {
			case id := <-arrived:
				seen[id] = true
			case <-time.After(15 * time.Second):
				blocker.Rollback()
				db.GetIns().Callback().Query().Remove(callback)
				group.Wait()
				close(outcomes)
				for out := range outcomes {
					t.Logf("blocked request role=%d err=%v", out.id, out.err)
				}
				t.Fatal("native requests did not reach the serialized task read", seen)
			}
		}
		if err = blocker.Commit().Error; err != nil {
			t.Fatal(err)
		}
		group.Wait()
		db.GetIns().Callback().Query().Remove(callback)
		close(outcomes)
		won := 0
		for out := range outcomes {
			if out.err == nil {
				won++
				receipts = append(receipts, winnerReceipt{out.id, int64(86001 + day), out.messages[0]})
			}
		}
		if won != 1 {
			t.Fatal("last unit was overissued or lost", day, won)
		}
		var stock db.AdminOpeningStock
		if err = db.GetIns().First(&stock, "occurrence_id=? AND task_id=?", source.ID, 20).Error; err != nil || stock.UsedStock != 1 {
			t.Fatal("period stock not conserved", stock, err)
		}
	}
	for _, receipt := range receipts {
		bound := serverClockBindTest(t, f, receipt.id)
		for i := 0; i < 3; i++ {
			messages, e := f.s.openServerActivityAt(bound, receipt.id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}, receipt.stamp, LocalPlayNow(bound))
			if e != nil || !proto.Equal(messages[0], receipt.reply) {
				t.Fatal("native retry changed result", e)
			}
		}
	}
	f.restart(t)
	if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range receipts {
		bound := serverClockBindTest(t, f, receipt.id)
		messages, e := f.s.openServerActivityAt(bound, receipt.id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}, receipt.stamp, LocalPlayNow(bound))
		if e != nil || !proto.Equal(messages[0], receipt.reply) {
			t.Fatal("cold retry changed result", e)
		}
	}
	var diamondAfter, coinAfter int64
	for _, id := range ids {
		var row db.Profile
		if err = db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		player, _, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		diamondAfter += player.GetDiamond()
		coinAfter += player.GetCoin()
	}
	if diamondBefore-diamondAfter != 800 || coinAfter-coinBefore != 14 {
		t.Fatal("retry/cold did not conserve native costs/rewards", diamondBefore, diamondAfter, coinBefore, coinAfter)
	}
	if got := p1Count(t, &db.AdminOpeningReceipt{}, "timestamp IN ?", []int64{86001, 86002}); got != 2 {
		t.Fatal("wrong number of native committed receipts", got)
	}
	if err = db.GetIns().First(&task, "id=?", 20).Error; err != nil || task.UsedStock != 0 {
		t.Fatal("repeat period modified legacy stock", task.UsedStock, err)
	}
}
