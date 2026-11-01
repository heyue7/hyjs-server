package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Capture the actual GORM query without connecting to an external database.
type p1SharedQueryConnector struct{ queries []string }

func (c *p1SharedQueryConnector) Connect(context.Context) (driver.Conn, error) {
	return &p1SharedQueryConnection{connector: c}, nil
}
func (c *p1SharedQueryConnector) Driver() driver.Driver { return p1SharedQueryDriver{connector: c} }

type p1SharedQueryDriver struct{ connector *p1SharedQueryConnector }

func (d p1SharedQueryDriver) Open(string) (driver.Conn, error) {
	return &p1SharedQueryConnection{connector: d.connector}, nil
}

type p1SharedQueryConnection struct{ connector *p1SharedQueryConnector }

func (c *p1SharedQueryConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepared statement")
}
func (c *p1SharedQueryConnection) Close() error { return nil }
func (c *p1SharedQueryConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *p1SharedQueryConnection) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.connector.queries = append(c.connector.queries, query)
	return p1SharedQueryRows{}, nil
}

type p1SharedQueryRows struct{}

func (p1SharedQueryRows) Columns() []string           { return []string{"id"} }
func (p1SharedQueryRows) Close() error                { return nil }
func (p1SharedQueryRows) Next(_ []driver.Value) error { return io.EOF }

func TestP1SharedReadMySQLLockOrderAndMode(t *testing.T) {
	connector := &p1SharedQueryConnector{}
	connection := sql.OpenDB(connector)
	t.Cleanup(func() { _ = connection.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: connection, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		marked   bool
		expected string
	}{
		{name: "ordinary transaction", expected: "FOR UPDATE"},
		{name: "shared attempt", marked: true, expected: "FOR SHARE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), constUtil.P1SharedReadContextKey{}, tc.marked)
			before := len(connector.queries)
			if _, err := p1LockPeriods(database.WithContext(ctx)); err != nil {
				t.Fatal(err)
			}
			if len(connector.queries) != before+1 {
				t.Fatalf("expected one locking query, got %v", connector.queries[before:])
			}
			query := connector.queries[before]
			if !strings.Contains(query, "FORCE INDEX (PRIMARY) ORDER BY id ASC "+tc.expected) {
				t.Fatalf("incorrect activity lock or order: %s", query)
			}
			if err := p1LockPeriodIDs(database.WithContext(ctx)); err != nil {
				t.Fatal(err)
			}
			narrow := connector.queries[before+1]
			if narrow != "SELECT id FROM p1_period_instances FORCE INDEX (PRIMARY) ORDER BY id ASC "+tc.expected {
				t.Fatalf("activity prelock lost its primary order, mode or narrow projection: %s", narrow)
			}
		})
	}
}

func TestP1SharedReadRollsBackBeforeActivityWrites(t *testing.T) {
	// This test owns only a temporary SQLite fixture, even when another test
	// window uses the project's allocated MySQL environment.
	t.Setenv("HYJS_P4_MYSQL_DATABASE", "")
	f := p4Setup(t)
	now := time.Now()
	cfg := p1Instance(t, f, "p1_shared_read", constUtil.P1EventLoginDay, "cumulative", 2, now.Add(time.Hour), 0)
	sharedCtx := context.WithValue(f.ctx, constUtil.P1SharedReadContextKey{}, true)
	shared := db.GetIns().WithContext(sharedCtx)
	if err := shared.Transaction(func(tx *gorm.DB) error {
		rows, err := p1LockPeriods(tx)
		if err != nil {
			return err
		}
		for i := 1; i < len(rows); i++ {
			if rows[i-1].ID >= rows[i].ID {
				t.Fatal("shared SQLite traversal did not preserve primary-key order")
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("shared SQLite activity read failed: %v", err)
	}
	if err := f.s.seedP4Periods(shared, now); err != nil {
		t.Fatalf("existing seeds need no exclusive retry: %v", err)
	}
	if _, err := f.s.freezeP1Period(shared, cfg, false); err != nil {
		t.Fatalf("read-only frozen validation failed: %v", err)
	}

	// A failed attempt must discard work already performed by its caller.
	err := shared.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&db.EventFact{PlayerID: f.id, SourceKey: "p1-shared-rollback", Kind: "probe", Amount: 1, OccurredAt: now.UnixMilli(), Day: p1Day(now)}).Error; err != nil {
			return err
		}
		neutral := context.WithValue(tx.Statement.Context, constUtil.GameRequestContextKey, struct{}{})
		return f.s.RecordP1Event(tx.WithContext(neutral), f.id, "p1-shared-event", constUtil.P1EventLoginDay, 1, now)
	})
	if !errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		t.Fatalf("expected event to request full rollback: %v", err)
	}
	if got := p1Count(t, &db.EventFact{}, "player_id=? AND source_key IN ?", f.id, []string{"p1-shared-rollback", "p1-shared-event"}); got != 0 {
		t.Fatalf("shared attempt leaked %d event facts", got)
	}
	if _, err = f.s.freezeP1Period(shared, cfg, true); !errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		t.Fatalf("freezing under shared locks must request rollback: %v", err)
	}
	var persisted db.PeriodInstance
	if err = db.GetIns().First(&persisted, "id=?", cfg.ID).Error; err != nil || len(persisted.Frozen) != 0 {
		t.Fatalf("shared freeze changed persisted rule: %v", err)
	}

	// A missing ordinary seed and a missing seasonal bundle each force fallback
	// before any initializer starts inserting or repairing activity records.
	plunderID := p4PlunderActivityPeriod(now).ID
	if err = db.GetIns().Where("id=?", plunderID).Delete(&db.PeriodInstance{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = shared.Transaction(func(tx *gorm.DB) error { return f.s.seedP4Periods(tx, now) }); !errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		t.Fatalf("missing P4 seed must request exclusive retry: %v", err)
	}
	if p1Count(t, &db.PeriodInstance{}, "id=?", plunderID) != 0 {
		t.Fatal("shared attempt created missing P4 period")
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.seedP4Periods(tx, now) }); err != nil {
		t.Fatalf("exclusive P4 retry failed: %v", err)
	}
	if p1Count(t, &db.PeriodInstance{}, "id=?", plunderID) != 1 {
		t.Fatal("exclusive retry did not restore P4 period")
	}

	month := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	bundleID := "festival_p4_" + month
	if err = db.GetIns().Where("id=?", bundleID).Delete(&db.P15SeasonalBundle{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = shared.Transaction(func(tx *gorm.DB) error { return f.s.seedP4Periods(tx, now) }); !errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		t.Fatalf("missing seasonal bundle must request exclusive retry: %v", err)
	}
	if p1Count(t, &db.P15SeasonalBundle{}, "id=?", bundleID) != 0 {
		t.Fatal("shared attempt created missing seasonal bundle")
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.seedP4Periods(tx, now) }); err != nil {
		t.Fatalf("exclusive seasonal retry failed: %v", err)
	}
	if p1Count(t, &db.P15SeasonalBundle{}, "id=?", bundleID) != 1 {
		t.Fatal("exclusive retry did not restore seasonal bundle")
	}

	// The unmarked transaction preserves actual activity facts and idempotence.
	for attempt := 0; attempt < 2; attempt++ {
		if err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
			return f.s.RecordP1Event(tx, f.id, "p1-shared-event", constUtil.P1EventLoginDay, 1, now)
		}); err != nil {
			t.Fatalf("exclusive event attempt %d: %v", attempt, err)
		}
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p1-shared-event") != 1 || p1ProgressValue(t, f, cfg.ID).Value != 1 {
		t.Fatal("exclusive retry did not commit one fact and one progress increment")
	}
}

func TestP1SharedReadFieldTransactionRetry(t *testing.T) {
	t.Setenv("HYJS_P4_MYSQL_DATABASE", "")
	testP1SharedReadFieldTransactionRetry(t)
}

func TestP1SharedReadFieldTransactionRetryMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	testP1SharedReadFieldTransactionRetry(t)
}

func testP1SharedReadFieldTransactionRetry(t *testing.T) {
	t.Helper()
	f := p4Setup(t)
	base := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	// Establish the ordinary P4 baseline before checking a no-event request.
	if err := updateProfileWithTx(base, f.id, func(*gorm.DB, *pb.Player, *model.GameState) error { return nil }); err != nil {
		t.Fatalf("prepare ordinary P4 baseline: %v", err)
	}
	for _, tc := range []struct {
		name string
		body proto.Message
	}{
		{name: "move", body: &pb.CSMove{}},
		{name: "heartbeat", body: &pb.CSPingTcp{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := f.profile(t)
			var modes []bool
			ctx := context.WithValue(base, constUtil.GameRequestContextKey, tc.body)
			err := updateFieldProfileWithTx(ctx, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
				modes = append(modes, p1SharedRead(tx))
				player.Coin = proto.Int64(player.GetCoin() + 1)
				return nil
			})
			if err != nil || len(modes) != 1 || !modes[0] {
				t.Fatalf("no-event request should commit one shared attempt: modes=%v err=%v", modes, err)
			}
			after, _ := f.profile(t)
			if after.GetCoin() != before.GetCoin()+1 {
				t.Fatal("no-event request did not commit exactly one profile change")
			}
		})
	}

	now := time.Now()
	callbackPeriod := p1Instance(t, f, "p1_shared_callback", "p1_shared_callback_kind", "cumulative", 99, now.Add(time.Hour), 0)
	before, _ := f.profile(t)
	var callbackModes []bool
	move := context.WithValue(base, constUtil.GameRequestContextKey, &pb.CSMove{})
	err := updateFieldProfileWithTx(move, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		callbackModes = append(callbackModes, p1SharedRead(tx))
		player.Coin = proto.Int64(player.GetCoin() + 1)
		// The same unique marker in both attempts proves the first insert rolled back.
		marker := db.EventFact{PlayerID: f.id, SourceKey: "p1-callback-attempt", Kind: "probe", Amount: 1, OccurredAt: now.UnixMilli(), Day: p1Day(now)}
		if err := tx.Create(&marker).Error; err != nil {
			return err
		}
		return f.s.RecordP1Event(tx, f.id, "p1-callback-source", callbackPeriod.Kind, 1, now)
	})
	if err != nil || len(callbackModes) != 2 || !callbackModes[0] || callbackModes[1] {
		t.Fatalf("event callback should retry once under exclusive locks: modes=%v err=%v", callbackModes, err)
	}
	after, _ := f.profile(t)
	if after.GetCoin() != before.GetCoin()+1 || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p1-callback-attempt") != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p1-callback-source") != 1 || p1ProgressValue(t, f, callbackPeriod.ID).Value != 1 {
		t.Fatal("callback retry did not commit exactly one profile change, fact and progress")
	}

	// A first P4 save can emit a collection fact even for a heartbeat. Remove
	// only this isolated fixture's baseline to reproduce that first registration.
	collectPeriod := p1Instance(t, f, "p1_shared_collection", constUtil.P4KindCollect, "cumulative", 99, now.Add(time.Hour), 0)
	if err := db.GetIns().Where("player_id=? AND kind=?", f.id, constUtil.P4KindCollect).Delete(&db.EventFact{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("player_id=?", f.id).Delete(&db.P4PlayerStats{}).Error; err != nil {
		t.Fatal(err)
	}
	before, _ = f.profile(t)
	var hookModes []bool
	heartbeat := context.WithValue(base, constUtil.GameRequestContextKey, &pb.CSPingTcp{})
	err = updateFieldProfileWithTx(heartbeat, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		hookModes = append(hookModes, p1SharedRead(tx))
		player.Coin = proto.Int64(player.GetCoin() + 1)
		marker := db.EventFact{PlayerID: f.id, SourceKey: "p1-hook-attempt", Kind: "probe", Amount: 1, OccurredAt: now.UnixMilli(), Day: p1Day(now)}
		return tx.Create(&marker).Error
	})
	if err != nil || len(hookModes) != 2 || !hookModes[0] || hookModes[1] {
		t.Fatalf("first P4 registration should retry under exclusive locks: modes=%v err=%v", hookModes, err)
	}
	after, _ = f.profile(t)
	if after.GetCoin() != before.GetCoin()+1 || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p1-hook-attempt") != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect) != 1 || p1ProgressValue(t, f, collectPeriod.ID).Value != 1 {
		t.Fatal("P4 retry did not commit exactly one profile change, collection fact and progress")
	}

	var ordinaryModes []bool
	ordinary := context.WithValue(base, constUtil.GameRequestContextKey, &pb.CSPlayer{})
	if err = updateFieldProfileWithTx(ordinary, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		ordinaryModes = append(ordinaryModes, p1SharedRead(tx))
		return nil
	}); err != nil || len(ordinaryModes) != 1 || ordinaryModes[0] {
		t.Fatalf("other protocol must keep its ordinary lock: modes=%v err=%v", ordinaryModes, err)
	}

	before, _ = f.profile(t)
	var invalidModes []bool
	err = updateFieldProfileWithTx(move, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		invalidModes = append(invalidModes, p1SharedRead(tx))
		player.Coin = proto.Int64(player.GetCoin() + 1)
		return f.s.RecordP1Event(tx, f.id, "", callbackPeriod.Kind, 1, now)
	})
	after, _ = f.profile(t)
	if err == nil || err.Error() != "事件事实参数无效" || len(invalidModes) != 1 || !invalidModes[0] || after.GetCoin() != before.GetCoin() {
		t.Fatalf("invalid event must retain its original error and roll back: modes=%v err=%v", invalidModes, err)
	}
}

func TestP1SharedReadPrivateCollectionReplay(t *testing.T) {
	t.Setenv("HYJS_P4_MYSQL_DATABASE", "")
	testP1SharedReadPrivateCollectionReplay(t)
}

func TestP1SharedReadPrivateCollectionReplayMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	testP1SharedReadPrivateCollectionReplay(t)
}

func testP1SharedReadPrivateCollectionReplay(t *testing.T) {
	t.Helper()
	f := p4Setup(t)
	// A private activity belongs to an actual non-system character.
	if f.id <= 1 {
		account, err := LoginAccount(f.ctx, "private-collection-shared", "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, account.Id, "私人收藏回归", "hero_043"); err != nil {
			t.Fatal(err)
		}
		f.id = account.Id
	}
	if err := db.GetIns().AutoMigrate(&db.LocalPlayActivityBinding{}, &db.LocalPlayActivityCycle{}); err != nil {
		t.Fatal(err)
	}
	base := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := updateProfileWithTx(base, f.id, func(*gorm.DB, *pb.Player, *model.GameState) error { return nil }); err != nil {
		t.Fatalf("prepare P4 baseline: %v", err)
	}
	now := time.Now()
	const cycleID = "lp-00000000000000000000000001"
	period := p1Instance(t, f, "warriorGetSpecial_p4_"+cycleID, constUtil.P4KindCollect, "cumulative", 99, now.Add(time.Hour), 0)
	cycle := db.LocalPlayActivityCycle{
		ID: cycleID, PlayerID: f.id, ActivityID: "CollectionWarrior",
		CreatedAt: now.UnixMilli(), RealCreatedAt: now.UnixMilli(),
		StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(),
		RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Snapshot: []byte(`{}`), Source: constUtil.LocalPlayActivitiesVersion,
	}
	if err := db.GetIns().Create(&cycle).Error; err != nil {
		t.Fatal(err)
	}
	binding := db.LocalPlayActivityBinding{PlayerID: f.id, ActivityID: "CollectionWarrior", CycleID: cycleID, Revision: 1}
	if err := db.GetIns().Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	private := context.WithValue(base, constUtil.LocalPlayBoundKey, true)
	private = context.WithValue(private, constUtil.LocalPlayOwnerKey, f.id)
	private = context.WithValue(private, constUtil.GameRequestContextKey, &pb.CSPingTcp{})
	before, _ := f.profile(t)
	var newModes []bool
	err := updateFieldProfileWithTx(private, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		newModes = append(newModes, p1SharedRead(tx))
		player.Coin = proto.Int64(player.GetCoin() + 1)
		return nil
	})
	if err != nil || len(newModes) != 2 || !newModes[0] || newModes[1] {
		t.Fatalf("new private collection fact should retry exclusively: modes=%v err=%v", newModes, err)
	}
	after, _ := f.profile(t)
	source := "owned:" + cycleID + ":hero_043"
	if after.GetCoin() != before.GetCoin()+1 || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=? AND kind=?", f.id, source, constUtil.P4KindCollect) != 1 || p1ProgressValue(t, f, period.ID).Value != 1 {
		t.Fatal("new private collection fact did not commit exactly once")
	}

	for _, body := range []proto.Message{&pb.CSMove{}, &pb.CSPingTcp{}} {
		ctx := context.WithValue(private, constUtil.GameRequestContextKey, body)
		before, _ = f.profile(t)
		var modes []bool
		err = updateFieldProfileWithTx(ctx, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
			modes = append(modes, p1SharedRead(tx))
			player.Coin = proto.Int64(player.GetCoin() + 1)
			return nil
		})
		after, _ = f.profile(t)
		if err != nil || len(modes) != 1 || !modes[0] || after.GetCoin() != before.GetCoin()+1 || p1ProgressValue(t, f, period.ID).Value != 1 {
			t.Fatalf("private collection replay must commit once under shared locks: body=%T modes=%v err=%v", body, modes, err)
		}
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=? AND kind=?", f.id, source, constUtil.P4KindCollect) != 1 {
		t.Fatal("private collection replay duplicated the fact")
	}

	before, _ = f.profile(t)
	var conflictModes []bool
	err = updateFieldProfileWithTx(private, f.id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
		conflictModes = append(conflictModes, p1SharedRead(tx))
		player.Coin = proto.Int64(player.GetCoin() + 1)
		return f.s.RecordP1Event(tx, f.id, source, constUtil.P4KindCollect, 2, now)
	})
	after, _ = f.profile(t)
	if err == nil || err.Error() != "事件来源金额与原事实不一致" || len(conflictModes) != 1 || !conflictModes[0] || after.GetCoin() != before.GetCoin() || p1ProgressValue(t, f, period.ID).Value != 1 {
		t.Fatalf("conflicting replay must retain its error and roll back: modes=%v err=%v", conflictModes, err)
	}
}
