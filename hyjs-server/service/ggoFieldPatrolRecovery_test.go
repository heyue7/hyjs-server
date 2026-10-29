package service

import (
	"encoding/json"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOPatrolSharedClockFreezeAndResume(t *testing.T) {
	point := func(x, z float64) *model.AdminFieldGGOPosition {
		y := float64(0)
		return &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}
	}
	path := &model.AdminFieldGGOPoint{Position: point(0, 0), Patrol: &model.AdminFieldGGOPatrol{Waypoints: []*model.AdminFieldGGOPosition{point(0, 0), point(3, 0), point(3, 4)}, Speed: 1}}
	node := &ggoMonsterNode{PatrolEpochMS: 1000}
	check := func(now int64, wantX, wantZ float64) {
		t.Helper()
		p, err := ggoPatrolPosition(node, path, now)
		if err != nil || math.Abs(*p.X-wantX) > 1e-9 || math.Abs(*p.Z-wantZ) > 1e-9 {
			t.Fatalf("at %d: got %v, err %v", now, p, err)
		}
	}
	check(1000, 0, 0)
	check(4000, 3, 0)
	check(6000, 3, 2)
	check(8000, 3, 4)
	check(9000, 3, 3)
	check(15000, 0, 0)
	node.PatrolPausedAtMS = 6000
	check(9000, 3, 2)
	if err := ggoPatrolResume(node, 11000); err != nil {
		t.Fatal(err)
	}
	check(11000, 3, 2)
	check(12000, 3, 3)
	wire, err := ggoMonsterSide(node, path, 12000)
	if err != nil || wire.GetCurrentPoint().GetX() != 3 || wire.GetCurrentPoint().GetZ() != 3 {
		t.Fatalf("native battle check disagrees with snapshot: %v %v", wire, err)
	}
}

func TestGGOLineDueRecoveryAndNotify(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	f.s.GGOIntegrationReady = true
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	var before int64
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, f.config, "isolated-trade-config", now)
		if err != nil {
			return err
		}
		n := &state.Resources[0]
		n.Remaining, n.RefreshAtMS = 0, now+100
		before = n.Generation
		if err = ggoLineSave(tx, row, state, now); err != nil {
			return err
		}
		if row.NextDueAt != now+100 {
			t.Fatalf("wrong indexed deadline %d", row.NextDueAt)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.ggoRecoverLines(f.ctx, now+200); err != nil {
		t.Fatal(err)
	}
	var line db.GGOFieldLine
	if err = db.GetIns().First(&line, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error; err != nil {
		t.Fatal(err)
	}
	var got ggoLineState
	if err = json.Unmarshal(line.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.Resources[0].Generation == before || got.Resources[0].Remaining != f.config.Runtime.Resource.Capacity || line.NextDueAt <= now+200 {
		t.Fatal("depleted node not refreshed", got.Resources[0], line.NextDueAt)
	}
	received := make(chan int64, 4)
	for i, id := range f.ids {
		key := p10ClientKey{f.s.p10ClientOwner(), id}
		sink := &p10ClientSink{session: f.sessions[i], send: func(m proto.Message) error {
			if _, ok := m.(*pb.SCFieldResourceInfo); ok {
				received <- id
			}
			return nil
		}}
		p10ClientSinks.Store(key, sink)
		t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	}
	if err = f.s.ggoBroadcastFieldChange(f.ctx, 2); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-received:
		t.Fatalf("wrong line notified %d", id)
	default:
	}
	if err = f.s.ggoBroadcastFieldChange(f.ctx, 1); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for range f.ids {
		select {
		case id := <-received:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatal("notification missing")
		}
	}
	if !seen[f.ids[0]] || !seen[f.ids[1]] {
		t.Fatal("notification recipient mismatch", seen)
	}
}

func TestGGONoticeCoalescesChangesDuringSlowSocket(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	f.s.GGOIntegrationReady = true
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	var sends atomic.Int64
	key := p10ClientKey{f.s.p10ClientOwner(), f.ids[0]}
	sink := &p10ClientSink{session: f.sessions[0], send: func(proto.Message) error {
		sends.Add(1)
		started <- struct{}{}
		<-release
		return nil
	}}
	p10ClientSinks.Store(key, sink)
	defer p10ClientSinks.CompareAndDelete(key, sink)
	defer f.s.ggoStopNoticeTimers()
	f.s.ggoNotifyFieldChange(f.ctx, 1)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial hint missing")
	}
	for i := 0; i < 100; i++ {
		f.s.ggoNotifyFieldChange(f.ctx, 1)
	}
	time.Sleep(150 * time.Millisecond)
	if sends.Load() != 1 {
		t.Fatalf("slow socket spawned parallel broadcasts: %d", sends.Load())
	}
	unblock.Do(func() { close(release) })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("coalesced follow-up missing")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := f.s.ggoNoticeTimers.Load(int32(1)); !ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := f.s.ggoNoticeTimers.Load(int32(1)); ok || sends.Load() != 2 {
		t.Fatalf("hint task did not settle: %d", sends.Load())
	}
}
