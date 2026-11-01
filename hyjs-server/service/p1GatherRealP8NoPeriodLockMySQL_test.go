package service

import (
	"context"
	"fmt"
	"os"
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

// This uses actual P8 requests and save hooks, while a third transaction owns
// every period row exclusively. The finite timer adjustment is test-only;
// each accepted attempt keeps the original random frozen resource rewards.
func TestP1GatherRealTwoPlayersIgnoreOtherPeriodXMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	peer := p1GatherSecondPlayer(t, f)
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
	base := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	type actor struct {
		id           int64
		node         db.P8ResourceNode
		cfg          *p8GatherConfig
		before       *pb.Player
		gameBefore   *model.GameState
		attempt      db.P8GatherAttempt
		startElapsed time.Duration
		claimElapsed time.Duration
	}
	actors := []*actor{{id: f.id}, {id: peer}}
	for _, a := range actors {
		if err := updateProfile(f.ctx, a.id, func(p *pb.Player, _ *model.GameState) error {
			p.Level = proto.Int32(30)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for _, state := range []any{&db.GemCraftState{PlayerID: a.id, Level: 2}, &db.CookingCraftState{PlayerID: a.id, Level: 2}} {
			if err := db.GetIns().Save(state).Error; err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.s.fieldTransfer(f.ctx, a.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		if err := updateProfileWithTx(f.ctx, a.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			_, err := f.s.RefreshP8Resources(tx, p, g, time.Now(), true)
			g.PositionLoading = false
			return err
		}); err != nil {
			t.Fatal(err)
		}
		nodes, err := P8GatherNodes(db.GetIns(), a.id)
		if err != nil {
			t.Fatal(err)
		}
		for _, node := range nodes {
			cfg, err := p8DecodeConfig(node.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Kind == "mine" && node.Remaining > 0 && (a == actors[0] || node.PosID != actors[0].node.PosID) {
				a.node, a.cfg = node, cfg
				break
			}
		}
		if a.cfg == nil {
			t.Fatal("isolated map did not produce two different mine points")
		}
		if err := updateProfileWithTx(f.ctx, a.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			if err := f.s.P8CancelGather(tx, a.id, g); err != nil {
				return err
			}
			g.Position.Current = proto.Clone(a.cfg.Point).(*pb.PointNew)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		// Establish real current-month P4 and collection facts before measuring.
		if err := f.s.RecordP4Login(base, a.id); err != nil {
			t.Fatal(err)
		}
	}
	period := p1Instance(t, f, "p1:real-gather-no-period-lock", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	p1GatherFreeze(t, f, period.ID)
	ctx, cancel := context.WithTimeout(base, 30*time.Second)
	defer cancel()
	started, release, holderDone, holderFailure := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan error, 1)
	var releaseOnce sync.Once
	var operations sync.WaitGroup
	releaseHolder := func() { releaseOnce.Do(func() { close(release) }) }
	go func() {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if _, err := p1LockPeriods(tx); err != nil {
				return err
			}
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil {
			holderFailure <- err
		}
		holderDone <- err
	}()
	defer func() {
		releaseHolder()
		operations.Wait()
		if err := <-holderDone; err != nil {
			t.Errorf("third-party period holder: %v", err)
		}
	}()
	select {
	case <-started:
	case err := <-holderFailure:
		t.Fatalf("third-party period X lock failed: %v", err)
	case <-ctx.Done():
		t.Fatal("third-party period X lock timed out")
	}
	type result struct {
		index    int
		messages []proto.Message
		elapsed  time.Duration
		err      error
	}
	runBoth := func(request func(*actor) proto.Message, stamp int64) []result {
		t.Helper()
		results := make(chan result, len(actors))
		for i, a := range actors {
			i, a := i, a
			operations.Add(1)
			go func() {
				defer operations.Done()
				message := request(a)
				requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, message)
				begin := time.Now()
				messages, handled, err := f.s.DispatchP8Gather(requestCtx, a.id, message, stamp+int64(i))
				if err == nil && !handled {
					err = fmt.Errorf("P8 request was not handled")
				}
				results <- result{index: i, messages: messages, elapsed: time.Since(begin), err: err}
			}()
		}
		out := make([]result, len(actors))
		for range actors {
			select {
			case value := <-results:
				if value.err != nil {
					t.Fatalf("P8 request failed before third-party release: %v", value.err)
				}
				out[value.index] = value
			case <-time.After(4 * time.Second):
				t.Fatal("P8 request waited behind another player's period X lock")
			case <-ctx.Done():
				t.Fatal("P8 request timed out")
			}
		}
		return out
	}
	starts := runBoth(func(a *actor) proto.Message { return &pb.CSFieldResourceStartGather{PosId: proto.String(a.node.PosID)} }, 91001)
	for i, a := range actors {
		a.startElapsed = starts[i].elapsed
		if len(starts[i].messages) == 0 || starts[i].messages[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
			t.Fatal("real mine start was rejected")
		}
		if err := db.GetIns().First(&a.attempt, "player_id=? AND active=?", a.id, true).Error; err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, a.id).Error; err != nil {
			t.Fatal(err)
		}
		var err error
		a.before, a.gameBefore, err = decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		updated := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=? AND token=? AND active=?", a.id, a.attempt.Token, true).Update("ready_at", time.Now().UnixMilli()-1)
		if updated.Error != nil || updated.RowsAffected != 1 {
			t.Fatalf("could not advance the isolated attempt timer: %v", updated.Error)
		}
	}
	claims := runBoth(func(*actor) proto.Message { return &pb.CSFieldGetResource{} }, 92001)
	replays := runBoth(func(*actor) proto.Message { return &pb.CSFieldGetResource{} }, 92001)
	for i, a := range actors {
		a.claimElapsed = claims[i].elapsed
		if claims[i].messages[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable || !proto.Equal(claims[i].messages[0], replays[i].messages[0]) {
			t.Fatal("real claim or idempotent replay changed")
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, a.id).Error; err != nil {
			t.Fatal(err)
		}
		after, game, err := decodeProfile(&profile)
		if err != nil || game.Field.Side.GetTiredness() != a.gameBefore.Field.Side.GetTiredness()-a.cfg.Fatigue {
			t.Fatalf("mine fatigue did not change exactly once: %v", err)
		}
		if a.attempt.Rewards != "" {
			expected := map[string]int64{}
			for _, reward := range mustP3Resources75(t, a.attempt.Rewards) {
				expected[reward.GetId()] += int64(reward.GetNum())
			}
			for key, count := range expected {
				if fieldGameplay75Owned(after, game, key)-fieldGameplay75Owned(a.before, a.gameBefore, key) != count {
					t.Fatalf("frozen mine reward %s did not commit exactly once", key)
				}
			}
		}
		var node db.P8ResourceNode
		if err := db.GetIns().First(&node, "player_id=? AND scene=? AND pos_id=?", a.id, a.node.Scene, a.node.PosID).Error; err != nil || node.Remaining != a.node.Remaining-1 {
			t.Fatalf("mine point use mismatch: %v", err)
		}
		var progress db.PeriodProgress
		if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, a.id).Error; err != nil || progress.Value != 1 {
			t.Fatalf("committed progress missing while P1 X was held: %+v %v", progress, err)
		}
		if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", a.id, "p8:gather:"+a.attempt.Token) != 1 ||
			p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", a.id, "activity:"+period.ID) != 1 {
			t.Fatal("committed fact, progress or delivery was missing while P1 X was held")
		}
		t.Logf("player=%d point=%s start=%s claim=%s", a.id, a.node.PosID, a.startElapsed, a.claimElapsed)
	}
	releaseHolder()
	for _, a := range actors {
		info, err := f.s.P1Activity(base, a.id, &pb.CSActivityInfo{ActivityId: proto.String(period.ID)})
		if err != nil || len(info) != 1 {
			t.Fatalf("activity page after commit: %v", err)
		}
		items := info[0].(*pb.SCActivityInfo).GetItemList()
		if len(items) != 1 || items[0].GetStatus() != 1 {
			t.Fatal("activity page did not immediately show this player's earned reward")
		}
		reward := &pb.CSActivityReward{ActivityId: proto.String(period.ID), Index: proto.Int32(1), GetResources: items[0].GetRewardList()}
		first, err := f.s.P1Activity(base, a.id, reward)
		if err != nil || len(first) != 1 || len(first[0].(*pb.SCActivityReward).GetGet().GetSimps()) == 0 {
			t.Fatalf("activity claim failed: %v", err)
		}
		again, err := f.s.P1Activity(base, a.id, reward)
		if err != nil || len(again) != 1 || len(again[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 0 {
			t.Fatalf("activity claim replay duplicated reward: %v", err)
		}
	}
}
