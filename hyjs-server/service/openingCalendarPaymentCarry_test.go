package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func openingPaymentFixture(t *testing.T) (*gorm.DB, db.OpenServerActivityRule, db.OpenServerActivityRule, int64) {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "opening-payment.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	if err = conn.AutoMigrate(&db.OpeningCalendarPaymentCarry{}, &db.PaymentOrder{}, &db.LocalPlayPaymentTime{},
		&db.OpenServerActivityRule{}, &db.OpenServerActivityCounter{}, &db.OpenServerActivityState{}, &db.OpenServerActivityTask{}, &db.AdminOpeningCounter{}, &db.Profile{}); err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).UnixMilli()
	after := db.OpenServerActivityRule{ID: constUtil.OpenServerActivityID, Enabled: true, StartAt: start, EndAt: start + 14*86400000, RewardEndAt: start + 15*86400000}
	before := after
	before.StartAt -= 14 * 86400000
	before.EndAt -= 14 * 86400000
	before.RewardEndAt -= 14 * 86400000
	return conn, before, after, start + 1000
}

func openingPaymentOrder(t *testing.T, conn *gorm.DB, id string, key, player, cents, realAt, gameAt int64) db.PaymentOrder {
	t.Helper()
	order := db.PaymentOrder{ID: id, RequestKey: key, PlayerID: player, Cents: cents, ConfirmedAt: realAt, Status: constUtil.P3PaymentConfirmed, Snapshot: `{"id":"opening-test","charge":1}`}
	if err := conn.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if gameAt != realAt {
		if err := conn.Create(&db.LocalPlayPaymentTime{OrderID: id, PlayerID: player, RealAt: realAt, GameAt: gameAt}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return order
}

func TestOpeningCalendarPaymentCarryBaselineCutoffAndReadProjection(t *testing.T) {
	conn, before, after, cutoff := openingPaymentFixture(t)
	old := []db.PaymentOrder{
		openingPaymentOrder(t, conn, "old-a", 1, 7, 50, cutoff, before.StartAt+1000),
		openingPaymentOrder(t, conn, "old-b", 2, 7, 75, cutoff, before.StartAt+2000),
		openingPaymentOrder(t, conn, "old-day14", 3, 7, 999, cutoff, before.StartAt+13*86400000+1000),
		// Already confirmed but outside the old window: never becomes new money.
		openingPaymentOrder(t, conn, "old-outside", 4, 7, 5000, cutoff, after.StartAt+500),
	}
	history := []db.OpenServerActivityCounter{{PlayerID: 7, CounterKey: constUtil.P3OpenServerTotalPaid, Value: 777},
		{PlayerID: 7, CounterKey: constUtil.P3OpenServerSinglePaid + p3Day(time.UnixMilli(before.StartAt)), Value: 888},
		{PlayerID: 7, CounterKey: constUtil.OpenServerActivityCounterLogin + p3Day(time.UnixMilli(after.StartAt)), Value: 1}}
	if err := conn.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	var carry *db.OpeningCalendarPaymentCarry
	if err := conn.Transaction(func(tx *gorm.DB) error {
		var err error
		carry, err = PrepareOpeningCalendarPaymentCarry(tx, 7, before, after, "restore", before.EndAt, cutoff)
		if err != nil {
			return err
		}
		return SaveOpeningCalendarPaymentCarry(tx, carry)
	}); err != nil {
		t.Fatal(err)
	}
	var baseline, facts []OpeningCalendarPaymentFact
	json.Unmarshal(carry.Baseline, &baseline)
	json.Unmarshal(carry.Facts, &facts)
	if len(baseline) != 4 || len(facts) != 3 {
		t.Fatalf("baseline=%+v facts=%+v", baseline, facts)
	}
	initial, _, err := openingCalendarPaymentCounters(conn, 7, &after, time.UnixMilli(after.StartAt+2000))
	if err != nil || initial[constUtil.P3OpenServerTotalPaid] != 11 || initial[constUtil.P3OpenServerSinglePaid+p3Day(time.UnixMilli(after.StartAt+13*86400000))] != 0 {
		t.Fatalf("day14 old total must survive on day1 while future single stays locked: %v %v", initial, err)
	}
	openingPaymentOrder(t, conn, "new-a", 5, 7, 250, cutoff+1, after.StartAt+100)
	openingPaymentOrder(t, conn, "new-b", 6, 7, 150, cutoff+2, after.StartAt+200)
	openingPaymentOrder(t, conn, "new-same-millisecond", 7, 7, 101, cutoff, after.StartAt+300)
	openingPaymentOrder(t, conn, "new-before-cutoff", 8, 7, 90000, cutoff-1, after.StartAt+400)
	openingPaymentOrder(t, conn, "new-after-window", 9, 7, 90000, cutoff+3, after.EndAt)
	openingPaymentOrder(t, conn, "other-player", 1, 8, 90000, cutoff+4, after.StartAt+400)
	openingPaymentOrder(t, conn, "future-game", 10, 7, 90000, cutoff+5, after.StartAt+86400000)
	now := time.UnixMilli(after.StartAt + 2000)
	values, carried, err := openingCalendarPaymentCounters(conn, 7, &after, now)
	if err != nil || !carried || values[constUtil.P3OpenServerTotalPaid] != 16 || values[constUtil.P3OpenServerSinglePaid+p3Day(now)] != 2 {
		t.Fatalf("projection=%v carried=%v err=%v", values, carried, err)
	}
	day14 := time.UnixMilli(after.StartAt + 13*86400000 + 2000)
	if values[constUtil.P3OpenServerSinglePaid+p3Day(day14)] != 0 {
		t.Fatal("future old single-payment date qualified early")
	}
	values, _, err = openingCalendarPaymentCounters(conn, 7, &after, day14)
	if err != nil || values[constUtil.P3OpenServerSinglePaid+p3Day(day14)] != 9 {
		t.Fatalf("mapped day14=%v %v", values, err)
	}
	if err := conn.Transaction(func(tx *gorm.DB) error {
		s := &GameService{}
		if err := s.refreshP3OpenServerCounters(tx, 7, &after, now); err != nil {
			return err
		}
		rows, err := openServerReadCounters(tx, 7)
		if err != nil {
			return err
		}
		read := map[string]int32{}
		for _, row := range rows {
			read[row.CounterKey] = row.Value
		}
		if read[constUtil.P3OpenServerTotalPaid] != 16 || read[history[2].CounterKey] != 1 || read[history[1].CounterKey] != 0 {
			t.Fatalf("read counters=%v", read)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var saved []db.OpenServerActivityCounter
	if err := conn.Where("player_id=?", 7).Order("counter_key").Find(&saved).Error; err != nil {
		t.Fatal(err)
	}
	expected := map[string]int32{}
	for _, row := range history {
		expected[row.CounterKey] = row.Value
	}
	for _, row := range saved {
		if expected[row.CounterKey] != row.Value {
			t.Fatalf("history overwritten=%+v", saved)
		}
	}
	if len(saved) != len(history) {
		t.Fatal("projection wrote new historical counters")
	}
	for _, order := range old {
		var got db.PaymentOrder
		if err := conn.First(&got, "id=?", order.ID).Error; err != nil || !reflect.DeepEqual(order, got) {
			t.Fatalf("original money changed: %+v %v", got, err)
		}
	}
	if err := conn.Model(&db.PaymentOrder{}).Where("id=?", "old-outside").Update("cents", 5001).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := openingCalendarPaymentCounters(conn, 7, &after, now); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatalf("baseline amount conflict accepted: %v", err)
	}
}

func TestOpeningCalendarPaymentCarryOverlapRollbackRetryAndScopes(t *testing.T) {
	conn, before, after, cutoff := openingPaymentFixture(t)
	// Keep windows overlapping while changing their accepted relative dates.
	before.StartAt = after.StartAt - 86400000
	before.EndAt = after.EndAt - 86400000
	before.RewardEndAt = after.RewardEndAt - 86400000
	openingPaymentOrder(t, conn, "overlap-old", 1, 7, 275, cutoff, after.StartAt+500)
	openingPaymentOrder(t, conn, "old-out-of-window", 2, 7, 99999, cutoff, before.StartAt-1)
	interrupt := errors.New("interrupt opening restore")
	var row *db.OpeningCalendarPaymentCarry
	prepareSave := func(tx *gorm.DB) error {
		var err error
		row, err = PrepareOpeningCalendarPaymentCarry(tx, 7, before, after, "restore", before.EndAt, cutoff)
		if err != nil {
			return err
		}
		return SaveOpeningCalendarPaymentCarry(tx, row)
	}
	if err := conn.Transaction(func(tx *gorm.DB) error {
		if err := prepareSave(tx); err != nil {
			return err
		}
		return interrupt
	}); !errors.Is(err, interrupt) {
		t.Fatal(err)
	}
	var count int64
	if err := conn.Model(&db.OpeningCalendarPaymentCarry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rollback count=%d %v", count, err)
	}
	if err := conn.Transaction(prepareSave); err != nil {
		t.Fatal(err)
	}
	if err := conn.Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarPaymentCarry(tx, row) }); err != nil {
		t.Fatal("retry", err)
	}
	for _, change := range []string{"request", "window", "cutoff"} {
		t.Run(change, func(t *testing.T) {
			changed := *row
			switch change {
			case "request":
				changed.RequestID = "other"
			case "window":
				changed.AfterStartAt++
				changed.AfterEndAt++
				changed.AfterRewardAt++
			case "cutoff":
				changed.RealCutoff++
			}
			changed.Digest = openingCalendarPaymentDigest(changed)
			if err := conn.Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarPaymentCarry(tx, &changed) }); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
				t.Fatalf("changed input accepted: %v", err)
			}
		})
	}
	values, _, err := openingCalendarPaymentCounters(conn, 7, &after, time.UnixMilli(after.StartAt+1000))
	if err != nil || values[constUtil.P3OpenServerTotalPaid] != 2 {
		t.Fatalf("overlap counted twice: %v %v", values, err)
	}
	for _, scope := range []string{"first", "second"} {
		ctx := context.WithValue(context.Background(), adminOpeningScopeKey{}, &adminOpeningScope{OccurrenceID: scope, PlayerID: 7})
		scoped := conn.WithContext(ctx)
		if _, carried, err := openingCalendarPaymentCounters(scoped, 7, &after, time.Now()); err != nil || carried {
			t.Fatalf("lifetime leaked into %s: %v %v", scope, carried, err)
		}
		if err := scoped.Transaction(prepareSave); err != nil {
			t.Fatal(err)
		}
		original := db.AdminOpeningCounter{OccurrenceID: scope, PlayerID: 7, CounterKey: constUtil.P3OpenServerTotalPaid, Value: 987}
		if err := scoped.Create(&original).Error; err != nil {
			t.Fatal(err)
		}
		rows, err := openServerReadCounters(scoped, 7)
		if err != nil || len(rows) != 1 || rows[0].Value != 2 {
			t.Fatalf("occurrence repository projection %s: %+v %v", scope, rows, err)
		}
		var stored db.AdminOpeningCounter
		if err := scoped.First(&stored, "occurrence_id=? AND player_id=?", scope, 7).Error; err != nil || !reflect.DeepEqual(stored, original) {
			t.Fatalf("occurrence history changed: %+v %v", stored, err)
		}
	}
	if err := conn.Model(&db.OpeningCalendarPaymentCarry{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("scope rows=%d %v", count, err)
	}
	if _, carried, err := openingCalendarPaymentCounters(conn, 8, &after, time.Now()); err != nil || carried {
		t.Fatal("cross-player carry", carried, err)
	}
	bad := conn.WithContext(context.WithValue(context.Background(), adminOpeningScopeKey{}, &adminOpeningScope{OccurrenceID: "first", PlayerID: 8}))
	if _, _, err := openingCalendarPaymentCounters(bad, 7, &after, time.Now()); err == nil {
		t.Fatal("scope owner mismatch accepted")
	}
	if _, err := PrepareOpeningCalendarPaymentCarry(conn, 7, before, after, "bare", before.EndAt, cutoff); err == nil {
		t.Fatal("bare prepare accepted")
	}
	if err := SaveOpeningCalendarPaymentCarry(conn, row); err == nil {
		t.Fatal("bare save accepted")
	}
}

func TestOpeningCalendarPaymentCarryRefreshEvaluatesOriginalTasks(t *testing.T) {
	conn, before, after, cutoff := openingPaymentFixture(t)
	openingPaymentOrder(t, conn, "prior-paid", 1, 7, 650, cutoff, before.StartAt+100)
	if err := conn.Transaction(func(tx *gorm.DB) error {
		row, err := PrepareOpeningCalendarPaymentCarry(tx, 7, before, after, "restore", before.EndAt, cutoff)
		if err != nil {
			return err
		}
		return SaveOpeningCalendarPaymentCarry(tx, row)
	}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Create(&after).Error; err != nil {
		t.Fatal(err)
	}
	for _, task := range []db.OpenServerActivityTask{{ID: 1, TitleID: 1, Tap: 1, Reward: "coin,1", TaskType: 2, OpenDay: 1, Target: 6, Visible: true}, {ID: 2, TitleID: 2, Tap: 1, Reward: "coin,1", TaskType: 101, OpenDay: 1, Target: 6, Visible: true}} {
		if err := conn.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &GameService{}
	if err := conn.Transaction(func(tx *gorm.DB) error {
		return s.refreshOpenServerProgressAt(tx, &pb.Player{PlayerId: proto.Int64(7)}, &model.GameState{}, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	var states []db.OpenServerActivityState
	if err := conn.Where("player_id=?", 7).Order("task_id").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || !states[0].Achieved || !states[1].Achieved || states[0].Progress != 6 || states[1].Progress != 6 || states[0].Claims != 0 || states[1].Claims != 0 {
		t.Fatalf("actual task evaluation=%+v", states)
	}
	var count int64
	if err := conn.Model(&db.OpenServerActivityCounter{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("refresh wrote historical counters: %d %v", count, err)
	}
}

func TestOpeningCalendarPaymentCarryBoundedBaseline(t *testing.T) {
	conn, before, after, cutoff := openingPaymentFixture(t)
	orders := make([]db.PaymentOrder, 10001)
	for i := range orders {
		orders[i] = db.PaymentOrder{ID: fmt.Sprintf("bounded-%05d", i), PlayerID: 7, RequestKey: int64(i + 1), Status: constUtil.P3PaymentConfirmed, Cents: 1, ConfirmedAt: cutoff}
	}
	if err := conn.CreateInBatches(orders, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.Transaction(func(tx *gorm.DB) error {
		_, err := PrepareOpeningCalendarPaymentCarry(tx, 7, before, after, "bounded", before.EndAt, cutoff)
		return err
	}); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatalf("oversized baseline accepted: %v", err)
	}
	var count int64
	if err := conn.Model(&db.OpeningCalendarPaymentCarry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("oversized preparation wrote carry: %d %v", count, err)
	}
}

func TestOpeningCalendarPaymentCarryNoCarryRetainsOriginalCounters(t *testing.T) {
	conn, _, cfg, cutoff := openingPaymentFixture(t)
	openingPaymentOrder(t, conn, "original-a", 1, 7, 125, cutoff, cfg.StartAt+100)
	openingPaymentOrder(t, conn, "original-b", 2, 7, 175, cutoff+1, cfg.StartAt+200)
	if err := (&GameService{}).refreshP3OpenServerCounters(conn, 7, &cfg, time.UnixMilli(cfg.StartAt+2000)); err != nil {
		t.Fatal(err)
	}
	var stored []db.OpenServerActivityCounter
	if err := conn.Where("player_id=?", 7).Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	values := map[string]int32{}
	for _, row := range stored {
		values[row.CounterKey] = row.Value
	}
	if values[constUtil.P3OpenServerTotalPaid] != 3 || values[constUtil.P3OpenServerSinglePaid+p3Day(time.UnixMilli(cfg.StartAt))] != 1 {
		t.Fatalf("original sum/single semantics changed: %v", values)
	}
	read, err := openServerReadCounters(conn, 7)
	if err != nil || !reflect.DeepEqual(read, stored) {
		t.Fatalf("no-carry repository changed: %+v %v", read, err)
	}
}

func TestOpeningCalendarPaymentCarryRejectsFactConflictAndTamper(t *testing.T) {
	conn, before, after, cutoff := openingPaymentFixture(t)
	openingPaymentOrder(t, conn, "accepted", 1, 7, 600, cutoff, before.StartAt+100)
	var row *db.OpeningCalendarPaymentCarry
	if err := conn.Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = PrepareOpeningCalendarPaymentCarry(tx, 7, before, after, "restore", before.EndAt, cutoff)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"cents", "real", "duplicate", "outside"} {
		t.Run(scenario, func(t *testing.T) {
			changed := *row
			var facts []OpeningCalendarPaymentFact
			if err := json.Unmarshal(changed.Facts, &facts); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "cents":
				facts[0].Cents++
			case "real":
				facts[0].RealAt++
			case "duplicate":
				facts = append(facts, facts[0])
			case "outside":
				facts[0].GameAt = before.EndAt
			}
			changed.Facts, _ = json.Marshal(facts)
			changed.Digest = openingCalendarPaymentDigest(changed)
			if err := conn.Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarPaymentCarry(tx, &changed) }); !errors.Is(err, constUtil.ErrAdminConfiguration) {
				t.Fatalf("invalid fact accepted: %v", err)
			}
		})
	}
	if err := conn.Transaction(func(tx *gorm.DB) error { return SaveOpeningCalendarPaymentCarry(tx, row) }); err != nil {
		t.Fatal(err)
	}
	changedWindow := after
	changedWindow.EndAt++
	if _, _, err := openingCalendarPaymentCounters(conn, 7, &changedWindow, time.Now()); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatalf("changed live window accepted: %v", err)
	}
	if err := conn.Model(&db.OpeningCalendarPaymentCarry{}).Where("player_id=?", 7).Update("real_cutoff", cutoff+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := openServerReadCounters(conn, 7); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatalf("repository swallowed digest error: %v", err)
	}
}
