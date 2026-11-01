package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p8ClaimSharedBarrierKey struct{}

// Use real P8 start/claim transactions and frozen activity progress for two
// different players and points. The fixture is an allocated MySQL database.
func TestP8GatherClaimFrozenPeriodParallelRealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	if err := f.s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeP8Gather, f.s.InitGems, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	other, err := LoginAccount(f.ctx, fmt.Sprintf("p8-claim-shared-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "独立采集角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	ids := []int64{f.id, other.Id}
	base := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	type participant struct {
		node    db.P8ResourceNode
		cfg     *p8GatherConfig
		attempt db.P8GatherAttempt
		fatigue int32
	}
	players := make([]participant, len(ids))
	for i, id := range ids {
		if err = updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
			p.Level = proto.Int32(30)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Save(&db.GemCraftState{PlayerID: id, Level: 6}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 6}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		if err = updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			_, e := f.s.RefreshP8Resources(tx, p, g, time.Now(), true)
			g.PositionLoading = false
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if err = f.s.RecordP4Login(base, id); err != nil {
			t.Fatal(err)
		}
		nodes, e := P8GatherNodes(db.GetIns(), id)
		if e != nil {
			t.Fatal(e)
		}
		for _, node := range nodes {
			cfg, e := p8DecodeConfig(node.Snapshot)
			if e != nil {
				t.Fatal(e)
			}
			if cfg.Kind == "mine" && node.Remaining > 0 && (i == 0 || node.PosID != players[0].node.PosID) {
				players[i].node, players[i].cfg = node, cfg
				break
			}
		}
		if players[i].cfg == nil {
			t.Fatal("two distinct real mine points are required")
		}
		if err = updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			if e := f.s.P8CancelGather(tx, id, g); e != nil {
				return e
			}
			g.Position.Current = proto.Clone(players[i].cfg.Point).(*pb.PointNew)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		start := &pb.CSFieldResourceStartGather{PosId: proto.String(players[i].node.PosID)}
		startCtx := context.WithValue(base, constUtil.GameRequestContextKey, start)
		started, e := f.s.p8StartGather(startCtx, id, start, int64(93000+i*10))
		if e != nil || started[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
			t.Fatalf("start %d: %v", id, e)
		}
		if err = db.GetIns().First(&players[i].attempt, "player_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Model(&players[i].attempt).Update("ready_at", time.Now().UnixMilli()-1).Error; err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, e := decodeProfile(&profile)
		if e != nil {
			t.Fatal(e)
		}
		players[i].fatigue = game.Field.Side.GetTiredness()
	}

	period := p1Instance(t, f, "p8_claim_parallel_frozen", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	if err = db.GetIns().WithContext(base).Transaction(func(tx *gorm.DB) error {
		if _, e := p1LockPeriods(tx); e != nil {
			return e
		}
		_, e := f.s.freezeP1Period(tx, period, true)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	oldLogger := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: oldLogger}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = oldLogger }()

	ctx, cancel := context.WithTimeout(base, 20*time.Second)
	defer cancel()
	entered := make(chan int64, len(ids))
	release := make(chan struct{})
	var releaseOnce sync.Once
	const callback = "test:p8-claim-shared-barrier"
	if err = database.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		fact, ok := tx.Statement.Dest.(*db.EventFact)
		if !ok || tx.Statement.Context.Value(p8ClaimSharedBarrierKey{}) != true || fact.Kind != constUtil.P8EventMineGather || !strings.HasPrefix(fact.SourceKey, "p8:gather:") {
			return
		}
		entered <- fact.PlayerID
		select {
		case <-release:
		case <-ctx.Done():
			tx.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Callback().Create().Remove(callback) }()
	type result struct {
		index    int
		messages []proto.Message
		err      error
	}
	done := make(chan result, len(ids))
	var workers sync.WaitGroup
	defer func() {
		releaseOnce.Do(func() { close(release) })
		workers.Wait()
	}()
	for i, id := range ids {
		i, id := i, id
		workers.Add(1)
		go func() {
			defer workers.Done()
			claim := &pb.CSFieldGetResource{}
			claimCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, claim)
			claimCtx = context.WithValue(claimCtx, p8ClaimSharedBarrierKey{}, true)
			messages, e := f.s.p8ClaimGather(claimCtx, id, claim, int64(93001+i*10))
			done <- result{index: i, messages: messages, err: e}
		}()
	}
	seen := map[int64]bool{}
	for len(seen) < len(ids) {
		select {
		case id := <-entered:
			seen[id] = true
		case r := <-done:
			t.Fatalf("claim %d ended before both transactions reached the shared event: %v", ids[r.index], r.err)
		case <-ctx.Done():
			t.Fatal("different players did not reach their shared activity events concurrently")
		}
	}
	releaseOnce.Do(func() { close(release) })
	var replies = make([]proto.Message, len(ids))
	for range ids {
		select {
		case r := <-done:
			if r.err != nil || len(r.messages) == 0 || r.messages[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
				t.Fatalf("claim %d failed: %v", ids[r.index], r.err)
			}
			replies[r.index] = r.messages[0]
		case <-ctx.Done():
			t.Fatal("shared claims did not complete")
		}
	}
	if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
		t.Fatalf("frozen claims used exclusive period locks: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	for i, id := range ids {
		claim := &pb.CSFieldGetResource{}
		claimCtx := context.WithValue(base, constUtil.GameRequestContextKey, claim)
		replayed, e := f.s.p8ClaimGather(claimCtx, id, claim, int64(93001+i*10))
		if e != nil || !proto.Equal(replies[i], replayed[0]) {
			t.Fatalf("claim %d receipt replay changed: %v", id, e)
		}
		var attempt db.P8GatherAttempt
		if e = database.First(&attempt, "player_id=?", id).Error; e != nil || attempt.Active || attempt.Token != players[i].attempt.Token {
			t.Fatalf("claim %d attempt was not consumed exactly once: %+v %v", id, attempt, e)
		}
		if n := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", id, "p8:gather:"+attempt.Token); n != 1 {
			t.Fatalf("claim %d event count=%d", id, n)
		}
		var progress db.PeriodProgress
		if e = database.First(&progress, "instance_id=? AND player_id=?", period.ID, id).Error; e != nil || progress.Value != 1 || progress.RewardID == 0 {
			t.Fatalf("claim %d progress mismatch: %+v %v", id, progress, e)
		}
		var profile db.Profile
		if e = database.First(&profile, id).Error; e != nil {
			t.Fatal(e)
		}
		_, game, e := decodeProfile(&profile)
		if e != nil || game.Field.Side.GetTiredness() != players[i].fatigue-players[i].cfg.Fatigue {
			t.Fatalf("claim %d fatigue mismatch: %v", id, e)
		}
	}
}
