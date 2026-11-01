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

func TestP8SharedFactRequiresTrustedGatherEntry(t *testing.T) {
	f := p1Setup(t)
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	var modes []bool
	err := updateProfileWithSharedPeriods(ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		modes = append(modes, p1SharedRead(tx))
		return f.s.RecordP1Event(tx, f.id, "other-flow-gather-event", constUtil.P8EventMineGather, 1, time.Now())
	})
	if err != nil || len(modes) != 2 || !modes[0] || modes[1] || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "other-flow-gather-event") != 1 {
		t.Fatalf("unrelated shared flow did not use the exclusive retry: %v %v", modes, err)
	}
}

// Exercise the original resource query, start, claim and retry with InnoDB locks.
func TestP8GatherSharedAndActivityRetryRealMySQL(t *testing.T) {
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
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(30)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, state := range []any{&db.GemCraftState{PlayerID: f.id, Level: 2}, &db.CookingCraftState{PlayerID: f.id, Level: 2}} {
		if err := db.GetIns().Save(state).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, err := f.s.RefreshP8Resources(tx, p, g, time.Now(), true)
		g.PositionLoading = false
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// 现网同量级的其他活动不应让资源查询或开始采集取独占锁。
	now := time.Now()
	periods := make([]db.PeriodInstance, 700)
	for i := range periods {
		periods[i] = db.PeriodInstance{ID: fmt.Sprintf("p8_shared_other_%04d", i), Enabled: true, Title: "隔离活动",
			Kind: "other_isolated_event", Mode: "cumulative", Goal: 1, Rewards: "coin,1", Source: "隔离并发测试",
			StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli()}
	}
	if err := db.GetIns().CreateInBatches(periods, 100).Error; err != nil {
		t.Fatal(err)
	}
	base := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.RecordP4Login(base, f.id); err != nil {
		t.Fatal(err)
	}
	nodes, err := P8GatherNodes(db.GetIns(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	type selectedNode struct {
		node db.P8ResourceNode
		cfg  *p8GatherConfig
	}
	selected := map[string]selectedNode{}
	for _, node := range nodes {
		cfg, err := p8DecodeConfig(node.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if node.Remaining > 1 && selected[cfg.Kind].cfg == nil {
			selected[cfg.Kind] = selectedNode{node, cfg}
		}
	}
	if selected["mine"].cfg == nil || selected["fish"].cfg == nil {
		t.Fatal("isolated first map did not generate both kinds")
	}
	move := func(node selectedNode) {
		t.Helper()
		if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			if err := f.s.P8CancelGather(tx, f.id, g); err != nil {
				return err
			}
			g.Position.Current = proto.Clone(node.cfg.Point).(*pb.PointNew)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(base, 30*time.Second)
	defer cancel()
	database := db.GetIns()
	oldLogger := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: oldLogger}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = oldLogger }()
	hold := func() func() {
		t.Helper()
		acquired, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
			done <- database.WithContext(shared).Transaction(func(tx *gorm.DB) error {
				if _, err := p1LockPeriods(tx); err != nil {
					return err
				}
				close(acquired)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}()
		select {
		case <-acquired:
		case err := <-done:
			t.Fatalf("holding another activity reader: %v", err)
		case <-ctx.Done():
			t.Fatal("activity reader timed out")
		}
		var once sync.Once
		finish := func() {
			once.Do(func() {
				close(release)
				if err := <-done; err != nil {
					t.Errorf("activity reader: %v", err)
				}
			})
		}
		t.Cleanup(finish)
		return finish
	}
	type callResult struct {
		messages []proto.Message
		err      error
	}
	call := func(finish func(), action func() ([]proto.Message, error)) []proto.Message {
		t.Helper()
		result := make(chan callResult, 1)
		go func() { m, err := action(); result <- callResult{m, err} }()
		select {
		case r := <-result:
			if r.err != nil {
				t.Fatal(r.err)
			}
			return r.messages
		case <-time.After(4 * time.Second):
			finish()
			<-result
			t.Fatal("ordinary gather blocked behind another shared activity reader")
			return nil
		}
	}
	requestContext := func(m proto.Message) context.Context {
		return context.WithValue(ctx, constUtil.GameRequestContextKey, m)
	}
	for index, kind := range []string{"mine", "fish"} {
		chosen := selected[kind]
		move(chosen)
		finish := hold()
		trace.shared.Store(0)
		trace.exclusive.Store(0)
		check := &pb.CSSideResourceCheck{PosId: proto.String(chosen.node.PosID)}
		call(finish, func() ([]proto.Message, error) { return f.s.p8ResourceQuery(requestContext(check), f.id, check) })
		start := &pb.CSFieldResourceStartGather{PosId: proto.String(chosen.node.PosID)}
		stamp := int64(20000 + index*10)
		if kind == "mine" {
			stamp = 0 // 原 TCP 采集没有请求时间戳。
		}
		started := call(finish, func() ([]proto.Message, error) { return f.s.p8StartGather(requestContext(start), f.id, start, stamp) })
		if started[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
			t.Fatalf("%s did not begin", kind)
		}
		var attempt db.P8GatherAttempt
		if err := database.First(&attempt, "player_id=?", f.id).Error; err != nil || !attempt.Active {
			t.Fatalf("%s missing frozen attempt: %v", kind, err)
		}
		call(finish, func() ([]proto.Message, error) { return f.s.p8StartGather(requestContext(start), f.id, start, stamp) })
		var resumed db.P8GatherAttempt
		if err := database.First(&resumed, "player_id=?", f.id).Error; err != nil || resumed.Token != attempt.Token || resumed.ReadyAt != attempt.ReadyAt || resumed.Rewards != attempt.Rewards {
			t.Fatalf("%s repeat start changed the frozen attempt: %v", kind, err)
		}
		if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
			t.Fatalf("%s resource query or start used an exclusive period lock: shared=%d exclusive=%d", kind, trace.shared.Load(), trace.exclusive.Load())
		}
		// An unrelated held reader must not block a claim with no matching
		// mutable activity. Keep that reader until the claim and replay return.
		trace.shared.Store(0)
		trace.exclusive.Store(0)
		claim := &pb.CSFieldGetResource{}
		claimStamp := stamp + 1
		if stamp == 0 {
			claimStamp = 0
		}
		before, gameBefore := f.profile(t)
		if time.Now().UnixMilli() < attempt.ReadyAt {
			if _, err := f.s.p8ClaimGather(requestContext(claim), f.id, claim, claimStamp); err == nil {
				t.Fatalf("%s claimed before its original timer", kind)
			}
		}
		if wait := time.Until(time.UnixMilli(attempt.ReadyAt)); wait > 0 {
			time.Sleep(wait + 20*time.Millisecond)
		}
		claimed := call(finish, func() ([]proto.Message, error) {
			return f.s.p8ClaimGather(requestContext(claim), f.id, claim, claimStamp)
		})
		replay := call(finish, func() ([]proto.Message, error) {
			return f.s.p8ClaimGather(requestContext(claim), f.id, claim, claimStamp)
		})
		if claimed[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable || (claimStamp == 0 && replay[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceMissing) || (claimStamp != 0 && !proto.Equal(claimed[0], replay[0])) {
			t.Fatalf("%s claim or replay mismatch", kind)
		}
		after, gameAfter := f.profile(t)
		if before.GetPlayerId() != after.GetPlayerId() || gameAfter.Field.Side.GetTiredness() != gameBefore.Field.Side.GetTiredness()-chosen.cfg.Fatigue {
			t.Fatalf("%s fatigue not deducted exactly once", kind)
		}
		var node db.P8ResourceNode
		if err := database.First(&node, "player_id=? AND scene=? AND pos_id=?", f.id, chosen.node.Scene, chosen.node.PosID).Error; err != nil || node.Remaining != chosen.node.Remaining-1 {
			t.Fatalf("%s node use mismatch: %v", kind, err)
		}
		if n := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:"+attempt.Token); n != 1 {
			t.Fatalf("%s event fact count=%d", kind, n)
		}
		if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
			t.Fatalf("%s claim or replay did not retain shared period locks: shared=%d exclusive=%d", kind, trace.shared.Load(), trace.exclusive.Load())
		}
		finish()
	}

	// A currently open matching activity freezes its rule and credits once.
	period := p1Instance(t, f, "p8_shared_active_mine", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	chosen := selected["mine"]
	move(chosen)
	start := &pb.CSFieldResourceStartGather{PosId: proto.String(chosen.node.PosID)}
	if _, err := f.s.p8StartGather(requestContext(start), f.id, start, 20100); err != nil {
		t.Fatal(err)
	}
	var attempt db.P8GatherAttempt
	if err := database.First(&attempt, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&attempt).Update("ready_at", time.Now().UnixMilli()-1).Error; err != nil {
		t.Fatal(err)
	}
	var beforeNode db.P8ResourceNode
	if err := database.First(&beforeNode, "player_id=? AND scene=? AND pos_id=?", f.id, chosen.node.Scene, chosen.node.PosID).Error; err != nil {
		t.Fatal(err)
	}
	_, before := f.profile(t)
	finish := hold()
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	claim := &pb.CSFieldGetResource{}
	result := make(chan callResult, 1)
	go func() {
		inheritedShared := context.WithValue(requestContext(claim), constUtil.P1SharedReadContextKey{}, true)
		messages, err := f.s.p8ClaimGather(inheritedShared, f.id, claim, 20101)
		result <- callResult{messages, err}
	}()
	select {
	case r := <-result:
		finish()
		t.Fatalf("active activity did not wait for exclusive lock: %v", r.err)
	case <-time.After(250 * time.Millisecond):
	}
	var pending db.P8GatherAttempt
	if err := database.First(&pending, "player_id=?", f.id).Error; err != nil || !pending.Active {
		finish()
		t.Fatalf("failed shared attempt changed reservation: %v", err)
	}
	var pendingNode db.P8ResourceNode
	if err := database.First(&pendingNode, "player_id=? AND scene=? AND pos_id=?", f.id, chosen.node.Scene, chosen.node.PosID).Error; err != nil || pendingNode.Remaining != beforeNode.Remaining {
		finish()
		t.Fatalf("rolled-back shared claim consumed the node: %v", err)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:"+attempt.Token) != 0 || p1Count(t, &db.P8GatherReceipt{}, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P8GatherClaimOperation, 20101) != 0 {
		finish()
		t.Fatal("rolled-back shared claim leaked a fact or receipt")
	}
	_, pendingGame := f.profile(t)
	if pendingGame.Field.Side.GetTiredness() != before.Field.Side.GetTiredness() {
		finish()
		t.Fatal("rolled-back shared claim consumed fatigue")
	}
	finish()
	r := <-result
	if r.err != nil || r.messages[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceAvailable {
		t.Fatalf("exclusive retry failed: %v", r.err)
	}
	if trace.exclusive.Load() == 0 {
		t.Fatalf("missing exclusive activity lock: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	if trace.shared.Load() == 0 {
		t.Fatal("missing shared claim before the exclusive freeze retry")
	}
	_, after := f.profile(t)
	progress := p1ProgressValue(t, f, period.ID)
	if after.Field.Side.GetTiredness() != before.Field.Side.GetTiredness()-chosen.cfg.Fatigue || progress.Value != 1 || progress.RewardID == 0 || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:"+attempt.Token) != 1 {
		t.Fatal(fmt.Sprintf("active activity or claim credited incorrectly: progress=%+v", progress))
	}
	replayed, err := f.s.p8ClaimGather(requestContext(claim), f.id, claim, 20101)
	if err != nil || !proto.Equal(r.messages[0], replayed[0]) || p1ProgressValue(t, f, period.ID).Value != 1 {
		t.Fatalf("active activity claim replay changed: %v", err)
	}
}
