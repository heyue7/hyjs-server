package service

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func dailyCopyLoss(start *pb.SCDailyCopyFightStart, chapter string) *pb.CSDailyCopyFightEnd {
	return &pb.CSDailyCopyFightEnd{ChapterId: proto.String(chapter), FightId: start.FightId, OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(0), DefenderTotalLeftHp: proto.Float64(1)}}
}

func dailyCopyRefundBalance(t *testing.T, id int64) int32 {
	t.Helper()
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	return player.GetStamina()
}

func dailyCopyRefundQuota(t *testing.T, id int64, day, kind string) int32 {
	t.Helper()
	var quota db.DailyCopyState
	if err := db.GetIns().First(&quota, "player_id = ? AND day = ? AND type = ?", id, day, kind).Error; err != nil {
		t.Fatal(err)
	}
	return quota.Used
}

// A legacy pending battle has exactly the same stored prepayment and receipt as
// a new one. Settlement returns its own day's reservation even across midnight.
func TestDailyCopyRefundAcrossDaysAndAtomicRollback(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "daily-copy-refund-days", 100)
	day1 := time.Date(2026, 10, 17, 23, 59, 0, 0, time.FixedZone("UTC+8", 8*3600))
	day2 := day1.Add(2 * time.Minute)
	startRequest := &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}
	started, err := s.dailyCopyAt(ctx, id, startRequest, 101, day1)
	if err != nil {
		t.Fatal(err)
	}
	first := started[0].(*pb.SCDailyCopyFightStart)
	resumed, err := s.dailyCopyAt(ctx, id, startRequest, 102, day1)
	if err != nil || !proto.Equal(first, resumed[0]) || dailyCopyRefundBalance(t, id) != 190 {
		t.Fatal("pending fight was changed or charged again", err)
	}
	loss := dailyCopyLoss(first, "5")
	invalid := proto.Clone(loss).(*pb.CSDailyCopyFightEnd)
	invalid.OptRecord.AttackerTotalLeftHp = proto.Float64(1)
	if _, err := s.dailyCopyAt(ctx, id, invalid, 103, day1); err == nil {
		t.Fatal("inconsistent loss bypassed result validation")
	}
	rollback := errors.New("forced test rollback")
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if _, _, err = s.dailyCopyEnd(tx, player, game, loss, day1); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) || dailyCopyRefundQuota(t, id, dailyCopyDay(day1), "TreasureExp") != 1 || dailyCopyRefundBalance(t, id) != 190 {
		t.Fatal("rollback changed prepayment", err)
	}
	var firstRow db.DailyCopyFight
	if err := db.GetIns().First(&firstRow, "fight_id = ?", first.GetFightId()).Error; err != nil || firstRow.Settled {
		t.Fatal("rollback saved receipt", err)
	}
	secondMessages, err := s.dailyCopyAt(ctx, id, startRequest, 104, day2)
	if err != nil {
		t.Fatal(err)
	}
	second := secondMessages[0].(*pb.SCDailyCopyFightStart)
	if second.GetFightId() == first.GetFightId() || dailyCopyRefundBalance(t, id) != 180 {
		t.Fatal("new day did not keep the old prepayment")
	}
	ended, err := s.dailyCopyAt(ctx, id, loss, 105, day2)
	if err != nil {
		t.Fatal(err)
	}
	result := ended[0].(*pb.SCDailyCopyFightEnd)
	if result.GetLastTime() != 1 || len(result.GetCost()) != 0 || result.GetStarNum() != 0 || proto.Size(result.GetGet()) != 0 || ended[1].(*pb.SCPlayer).GetPlayer().GetStamina() != 190 {
		t.Fatal("loss did not project refunded balance and quota", result)
	}
	if dailyCopyRefundQuota(t, id, dailyCopyDay(day1), "TreasureExp") != 0 || dailyCopyRefundQuota(t, id, dailyCopyDay(day2), "TreasureExp") != 1 {
		t.Fatal("loss changed the wrong business day")
	}
	if _, err := s.dailyCopyAt(ctx, id, dailyCopyWin(second, "5"), 106, day2); err != nil {
		t.Fatal(err)
	}
	retry, err := s.dailyCopyAt(ctx, id, loss, 107, day2)
	if err != nil || !proto.Equal(retry[0], result) || dailyCopyRefundBalance(t, id) != 190 || dailyCopyRefundQuota(t, id, dailyCopyDay(day2), "TreasureExp") != 1 {
		t.Fatal("old loss replay changed later victory", err)
	}
	if _, err := s.dailyCopyAt(ctx, id, startRequest, 108, day2); err == nil {
		t.Fatal("victory did not consume current-day quota")
	}
}

func TestDailyCopyRefundStaminaLimit(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		t.Run(map[bool]string{false: "above natural cap", true: "protocol overflow rollback"}[overflow], func(t *testing.T) {
			s, ctx, _ := newDailyCopyTest(t)
			id := dailyCopyTestPlayer(t, ctx, "daily-copy-refund-limit", 100)
			now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
			started, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 101, now)
			if err != nil {
				t.Fatal(err)
			}
			start := started[0].(*pb.SCDailyCopyFightStart)
			loss := dailyCopyLoss(start, "5")
			balance := int32(100)
			if overflow {
				balance = math.MaxInt32 - 5
			}
			if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Stamina = proto.Int32(balance); return nil }); err != nil {
				t.Fatal(err)
			}
			if overflow {
				if _, err := s.dailyCopyAt(ctx, id, loss, 102, now); err == nil {
					t.Fatal("overflow was silently clipped")
				}
				var fight db.DailyCopyFight
				if err := db.GetIns().First(&fight, "fight_id = ?", start.GetFightId()).Error; err != nil || fight.Settled || dailyCopyRefundQuota(t, id, dailyCopyDay(now), "TreasureExp") != 1 || dailyCopyRefundBalance(t, id) != balance {
					t.Fatal("overflow changed pending battle", err)
				}
				balance = math.MaxInt32 - 10
				if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Stamina = proto.Int32(balance); return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.dailyCopyAt(ctx, id, loss, 103, now); err != nil {
				t.Fatal(err)
			}
			if got := dailyCopyRefundBalance(t, id); got != balance+10 || dailyCopyRefundQuota(t, id, dailyCopyDay(now), "TreasureExp") != 0 {
				t.Fatal("full refund not preserved", got)
			}
		})
	}
}

func dailyCopyRefundConcurrent(t *testing.T, s *GameService, ctx context.Context) {
	t.Helper()
	id := dailyCopyTestPlayer(t, ctx, "daily-copy-refund-concurrent", 100)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	startRequest := &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}
	started, err := s.dailyCopyAt(ctx, id, startRequest, 101, now)
	if err != nil {
		t.Fatal(err)
	}
	loss := dailyCopyLoss(started[0].(*pb.SCDailyCopyFightStart), "5")
	const workers = 8
	var wait sync.WaitGroup
	replies := make(chan []proto.Message, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			messages, err := s.dailyCopyAt(ctx, id, loss, 102, now)
			if err != nil {
				errs <- err
				return
			}
			replies <- messages
		}()
	}
	wait.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		t.Error(err)
	}
	var response proto.Message
	count := 0
	for messages := range replies {
		count++
		if response == nil {
			response = messages[0]
		} else if !proto.Equal(response, messages[0]) {
			t.Fatal("concurrent retries received different settlement")
		}
	}
	if count != workers || dailyCopyRefundBalance(t, id) != 200 || dailyCopyRefundQuota(t, id, dailyCopyDay(now), "TreasureExp") != 0 {
		t.Fatal("concurrent retries did not refund exactly once", count)
	}
	next, err := s.dailyCopyAt(ctx, id, startRequest, 103, now)
	if err != nil {
		t.Fatal("loss did not permit another attempt", err)
	}
	if _, err = s.dailyCopyAt(ctx, id, dailyCopyWin(next[0].(*pb.SCDailyCopyFightStart), "5"), 104, now); err != nil {
		t.Fatal(err)
	}
	if _, err = s.dailyCopyAt(ctx, id, loss, 105, now); err != nil || dailyCopyRefundBalance(t, id) != 190 || dailyCopyRefundQuota(t, id, dailyCopyDay(now), "TreasureExp") != 1 {
		t.Fatal("old replay refunded the later victory", err)
	}
}

func TestDailyCopyRefundConcurrentSQLite(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	dailyCopyRefundConcurrent(t, s, ctx)
}

func TestDailyCopyRefundConcurrentMySQL(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("requires a managed isolated MySQL database")
	}
	ctx := context.Background()
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); err != nil {
		t.Fatal(err)
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() { pool.Close() })
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeDailyCopy(ctx); err != nil {
		t.Fatal(err)
	}
	dailyCopyRefundConcurrent(t, s, ctx)
}
