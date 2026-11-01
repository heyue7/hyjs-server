package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The table fixture can be shared read-only with an isolated worktree.
func p1GatherFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p1-gather.db")}
	if name := os.Getenv("HYJS_P1_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p1_") {
			t.Fatal("P1 gather tests require an isolated database")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	f.s = &GameService{}
	tables := "../../analysis/tables"
	if root := os.Getenv("HYJS_TEST_FIXTURE_ROOT"); root != "" {
		if !filepath.IsAbs(root) {
			t.Fatal("HYJS_TEST_FIXTURE_ROOT must be an absolute analysis directory")
		}
		tables = filepath.Join(root, "tables")
	}
	if err := f.s.LoadTables(tables); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, fmt.Sprintf("p1-gather-%s-%d", t.Name(), time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "采集活动回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	t.Cleanup(func() {
		pool, err := db.GetIns().DB()
		if err == nil {
			_ = pool.Close()
		}
	})
	return f
}

func p1GatherContext(f *p1Fixture) context.Context {
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	return context.WithValue(ctx, constUtil.GameRequestContextKey, struct{}{})
}

// Exercise the actual p3 transaction boundary used by ordinary gathers. Each
// retry acquires a lease in its own SQL transaction before profile recovery.
func p1GatherTestExclusiveProfile(s *GameService, ctx context.Context, id int64, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	ctx = p1GatherCatalogContext(ctx)
	return s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
		if err := fn(tx, player, game); err != nil {
			return err
		}
		return saveProfile(tx, row, player, game)
	})
}

func p1GatherTestSharedProfile(s *GameService, ctx context.Context, id int64, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	err := p1GatherTestExclusiveProfile(s, shared, id, fn)
	if errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		return p1GatherTestExclusiveProfile(s, ctx, id, fn)
	}
	return err
}

func p1GatherFreeze(t *testing.T, f *p1Fixture, ids ...string) {
	t.Helper()
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := p1LockPeriods(tx)
		if err != nil {
			return err
		}
		remaining := make(map[string]bool, len(ids))
		for _, id := range ids {
			remaining[id] = true
		}
		for _, live := range rows {
			if !remaining[live.ID] {
				continue
			}
			if _, err = f.s.freezeP1Period(tx, live, true); err != nil {
				return err
			}
			delete(remaining, live.ID)
		}
		if len(remaining) != 0 {
			return errors.New("missing gather period")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP1GatherFrozenProgressAndReplay(t *testing.T) {
	f := p1GatherFixture(t)
	end := time.Now().Add(time.Hour)
	mine := p1Instance(t, f, "p1:gather-frozen-mine", constUtil.P8EventMineGather, "cumulative", 1, end, 0)
	daily := p1Instance(t, f, "gather-frozen-daily", constUtil.P8EventMineGather, "daily", 2, end, 0)
	fish := p1Instance(t, f, "gather-frozen-fish", constUtil.P8EventFishGather, "single", 1, end, 0)
	tiers, err := json.Marshal([]p1PeriodMilestone{{Goal: 1, Rewards: mine.Rewards}, {Goal: 2, Rewards: "coin,19"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&mine).Update("milestones", tiers).Error; err != nil {
		t.Fatal(err)
	}
	p1GatherFreeze(t, f, mine.ID, daily.ID, fish.ID)
	ctx := p1GatherContext(f)
	write := func(token, kind string, desert bool) {
		t.Helper()
		var modes []bool
		attempt := &db.P8GatherAttempt{PlayerID: f.id, Token: token}
		err := p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			modes = append(modes, p1SharedRead(tx))
			return f.s.recordP1GatherEvent(tx, f.id, attempt, kind, desert, time.Now())
		})
		if err != nil || len(modes) != 1 || !modes[0] {
			t.Fatalf("frozen gather did not commit in shared mode: %v %v", modes, err)
		}
	}
	write("mine-one", constUtil.P8EventMineGather, false)
	write("mine-one", constUtil.P8EventMineGather, false)
	write("mine-two", constUtil.P8EventMineGather, false)
	write("fish-one", constUtil.P8EventFishGather, true)
	if p1ProgressValue(t, f, mine.ID).Value != 2 || p1ProgressValue(t, f, daily.ID).Value != 2 || p1ProgressValue(t, f, fish.ID).Value != 1 {
		t.Fatal("cumulative, daily or single progress differs")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind IN ?", f.id, []string{constUtil.P8EventMineGather, constUtil.P8EventFishGather}) != 3 {
		t.Fatal("gather facts were lost or replayed")
	}
	for _, source := range []string{"p8:gather:mine-one", "p8:gather:mine-two", "ggo:gather:fish-one"} {
		if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, source) != 1 {
			t.Fatalf("original gather source missing or duplicated: %s", source)
		}
	}
	sources := []string{"activity:" + mine.ID, "activity:" + mine.ID + ":tier:2", "activity:" + daily.ID, "activity:" + fish.ID}
	if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key IN ?", f.id, sources) != int64(len(sources)) {
		t.Fatal("milestones were not delivered once")
	}
	messages, err := f.s.P1Activity(ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(mine.ID)})
	if err != nil || len(messages) != 1 {
		t.Fatalf("activity page after committed gather: %v", err)
	}
	info, ok := messages[0].(*pb.SCActivityInfo)
	if !ok || len(info.GetItemList()) != 2 || info.GetItemList()[0].GetStatus() != 1 || info.GetItemList()[1].GetStatus() != 1 {
		t.Fatal("activity page did not immediately show both earned tiers")
	}
	shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	err = db.GetIns().WithContext(shared).Transaction(func(tx *gorm.DB) error {
		var current db.PeriodInstance
		if err := tx.First(&current, "id=?", mine.ID).Error; err != nil {
			return err
		}
		_, err := f.s.freezeP1Period(tx, current, true)
		return err
	})
	if !errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		t.Fatalf("general freeze was widened for shared transactions: %v", err)
	}

	// A later non-gather event rolls back the already-projected shared gather.
	before, _ := f.profile(t)
	var modes []bool
	err = p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		modes = append(modes, p1SharedRead(tx))
		player.Coin = proto.Int64(player.GetCoin() + 1)
		attempt := &db.P8GatherAttempt{PlayerID: f.id, Token: "mine-three"}
		if err := f.s.recordP1GatherEvent(tx, f.id, attempt, constUtil.P8EventMineGather, false, time.Now()); err != nil {
			return err
		}
		return f.s.RecordP1Event(tx, f.id, "other-save-event", "other_save_kind", 1, time.Now())
	})
	if err != nil || len(modes) != 2 || !modes[0] || modes[1] {
		t.Fatalf("other event did not cause a full exclusive retry: %v %v", modes, err)
	}
	after, _ := f.profile(t)
	if after.GetCoin() != before.GetCoin()+1 || p1ProgressValue(t, f, mine.ID).Value != 3 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:mine-three") != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "other-save-event") != 1 {
		t.Fatal("exclusive retry duplicated or lost the gather transaction")
	}
}

func TestP1GatherFirstFreezeRollsBackBeforeRetry(t *testing.T) {
	f := p1GatherFixture(t)
	if err := db.GetIns().AutoMigrate(&db.P8GatherAttempt{}); err != nil {
		t.Fatal(err)
	}
	period := p1Instance(t, f, "gather-first-freeze", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	started := db.P8GatherAttempt{PlayerID: f.id, Token: "first-freeze-token", Active: true}
	if err := db.GetIns().Create(&started).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	var modes []bool
	err := p1GatherTestSharedProfile(f.s, p1GatherContext(f), f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		modes = append(modes, p1SharedRead(tx))
		var attempt db.P8GatherAttempt
		if err := tx.First(&attempt, "player_id=? AND active=?", f.id, true).Error; err != nil {
			return err
		}
		attempt.Active = false
		if err := tx.Save(&attempt).Error; err != nil {
			return err
		}
		player.Coin = proto.Int64(player.GetCoin() + 1)
		return f.s.recordP1GatherEvent(tx, f.id, &attempt, constUtil.P8EventMineGather, false, time.Now())
	})
	if err != nil || len(modes) != 2 || !modes[0] || modes[1] {
		t.Fatalf("first freeze did not retry exclusively: %v %v", modes, err)
	}
	var saved db.P8GatherAttempt
	var frozen db.PeriodInstance
	if err = db.GetIns().First(&saved, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&frozen, "id=?", period.ID).Error; err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if saved.Active || len(frozen.Frozen) == 0 || after.GetCoin() != before.GetCoin()+1 || p1ProgressValue(t, f, period.ID).Value != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:first-freeze-token") != 1 {
		t.Fatal("rollback or first freeze changed the committed outcome")
	}
}

func TestP1GatherHonorsEnabledFrozenAndExpiredWindows(t *testing.T) {
	f := p1GatherFixture(t)
	period := p1Instance(t, f, "gather-window-enabled", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	if err := db.GetIns().Model(&period).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	ctx := p1GatherContext(f)
	write := func(token string, at time.Time) []bool {
		t.Helper()
		var modes []bool
		err := p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			modes = append(modes, p1SharedRead(tx))
			return f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: token}, constUtil.P8EventMineGather, false, at)
		})
		if err != nil {
			t.Fatal(err)
		}
		return modes
	}
	if modes := write("window-disabled", time.Now()); len(modes) != 1 || !modes[0] {
		t.Fatalf("disabled period unexpectedly required a freeze: %v", modes)
	}
	if p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", period.ID, f.id) != 0 {
		t.Fatal("disabled period counted a gather")
	}
	if err := db.GetIns().Model(&period).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if modes := write("window-first", time.Now()); len(modes) != 2 || !modes[0] || modes[1] {
		t.Fatalf("first enabled gather did not freeze exclusively: %v", modes)
	}
	if err := db.GetIns().Model(&period).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	write("window-disabled-again", time.Now())
	if p1ProgressValue(t, f, period.ID).Value != 1 {
		t.Fatal("disabled frozen period counted a later gather")
	}
	// Even if legacy mutable columns change, a new player must observe the
	// existing frozen goal and rewards when the instance is enabled again.
	if err := db.GetIns().Model(&period).Updates(map[string]any{"enabled": true, "goal": 99, "rewards": "coin,99"}).Error; err != nil {
		t.Fatal(err)
	}
	peer := p1GatherSecondPlayer(t, f)
	err := p1GatherTestSharedProfile(f.s, ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.recordP1GatherEvent(tx, peer, &db.P8GatherAttempt{PlayerID: peer, Token: "window-peer"}, constUtil.P8EventMineGather, true, time.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	var peerProgress db.PeriodProgress
	if err = db.GetIns().First(&peerProgress, "instance_id=? AND player_id=?", period.ID, peer).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot db.PeriodInstance
	if err = json.Unmarshal(peerProgress.Snapshot, &snapshot); err != nil || peerProgress.Value != 1 || snapshot.Goal != 1 || snapshot.Rewards != "coin,17;item_001,1" ||
		p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", peer, "activity:"+period.ID) != 1 {
		t.Fatalf("frozen player rules changed after live columns changed: %+v %v", snapshot, err)
	}
	closed := p1Instance(t, f, "gather-window-closed", constUtil.P8EventFishGather, "cumulative", 1, time.Now().Add(-time.Hour), 0)
	p1GatherFreeze(t, f, closed.ID)
	oldTime := time.Now().Add(-2 * time.Hour)
	if err = p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: "window-late"}, constUtil.P8EventFishGather, true, oldTime)
	}); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "ggo:gather:window-late") != 1 ||
		p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", closed.ID, f.id) != 0 {
		t.Fatal("late accepted fact changed closed activity progress")
	}
}

func TestP1GatherRejectsInvalidAttemptAndOtherSharedFacts(t *testing.T) {
	f := p1GatherFixture(t)
	ctx := context.WithValue(p1GatherContext(f), constUtil.P1SharedReadContextKey{}, true)
	for _, tc := range []struct {
		name    string
		attempt *db.P8GatherAttempt
		kind    string
	}{
		{"missing", nil, constUtil.P8EventMineGather},
		{"foreign", &db.P8GatherAttempt{PlayerID: f.id + 1, Token: "foreign"}, constUtil.P8EventMineGather},
		{"active", &db.P8GatherAttempt{PlayerID: f.id, Token: "active", Active: true}, constUtil.P8EventMineGather},
		{"empty", &db.P8GatherAttempt{PlayerID: f.id}, constUtil.P8EventMineGather},
		{"oversized", &db.P8GatherAttempt{PlayerID: f.id, Token: strings.Repeat("x", 65)}, constUtil.P8EventMineGather},
		{"wrong-kind", &db.P8GatherAttempt{PlayerID: f.id, Token: "wrong-kind"}, constUtil.P1EventLoginDay},
	} {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return f.s.recordP1GatherEvent(tx, f.id, tc.attempt, tc.kind, false, time.Now())
		})
		if err == nil || errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
			t.Fatalf("%s attempt accepted or retried: %v", tc.name, err)
		}
	}
	if p1Count(t, &db.EventFact{}, "player_id=?", f.id) != 0 {
		t.Fatal("invalid attempts wrote event facts")
	}
	var modes []bool
	err := p1GatherTestSharedProfile(f.s, p1GatherContext(f), f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		modes = append(modes, p1SharedRead(tx))
		return f.s.RecordP1Event(tx, f.id, "untrusted-gather-source", constUtil.P8EventMineGather, 1, time.Now())
	})
	if err != nil || len(modes) != 2 || !modes[0] || modes[1] {
		t.Fatalf("generic gather fact did not retain exclusive retry: %v %v", modes, err)
	}
}
