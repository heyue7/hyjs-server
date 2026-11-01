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
	effective, err := ggoPatrolPath(path)
	if err != nil || len(effective.Waypoints) != 2 || *effective.Waypoints[1].X != 1 || effective.Speed != .8 {
		t.Fatal("effective route is not confined to the configured first segment", effective, err)
	}
	check := func(now int64, wantX, wantZ float64) {
		t.Helper()
		p, err := ggoPatrolPosition(node, path, now)
		if err != nil || math.Abs(*p.X-wantX) > 1e-9 || math.Abs(*p.Z-wantZ) > 1e-9 {
			t.Fatalf("at %d: got %v, err %v", now, p, err)
		}
	}
	check(1000, 0, 0)
	check(13000, 0, 0)
	check(13625, .5, 0)
	check(14250, 1, 0)
	check(26250, 1, 0)
	check(26875, .5, 0)
	check(27500, 0, 0)
	// A second observer reading the same line receives the same position.
	other := *node
	for _, now := range []int64{13000, 13625, 14250, 26875} {
		first, e1 := ggoPatrolPosition(node, path, now)
		second, e2 := ggoPatrolPosition(&other, path, now)
		if e1 != nil || e2 != nil || *first.X != *second.X || *first.Y != *second.Y || *first.Z != *second.Z {
			t.Fatalf("line observers disagree at %d: %v %v", now, e1, e2)
		}
	}
	node.PatrolPausedAtMS = 13625
	check(20000, .5, 0)
	if err := ggoPatrolResume(node, 25000); err != nil {
		t.Fatal(err)
	}
	check(25000, .5, 0)
	check(25625, 1, 0)
	wire, err := ggoMonsterSide(node, path, 25625)
	if err != nil || wire.GetCurrentPoint().GetX() != 1 || wire.GetCurrentPoint().GetZ() != 0 {
		t.Fatalf("native battle check disagrees with snapshot: %v %v", wire, err)
	}
	stationary := &model.AdminFieldGGOPoint{Position: point(4, 5)}
	for _, now := range []int64{1000, 13625, 1000000} {
		p, err := ggoPatrolPosition(node, stationary, now)
		if err != nil || *p.X != 4 || *p.Z != 5 {
			t.Fatalf("unverified point moved at %d: %v %v", now, p, err)
		}
	}
}

func TestGGOPatrolRecentContactKeepsBoundedReach(t *testing.T) {
	position := func(x, z float64) *model.AdminFieldGGOPosition {
		y := float64(0)
		return &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}
	}
	point := &model.AdminFieldGGOPoint{Position: position(0, 0), Radius: .2, Patrol: &model.AdminFieldGGOPatrol{Waypoints: []*model.AdminFieldGGOPosition{position(0, 0), position(1, 0)}}}
	node := &ggoMonsterNode{PatrolEpochMS: 1000}
	contact := &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}
	check := func(now int64, actor *pb.PointNew, want bool) {
		t.Helper()
		side, err := ggoMonsterSide(node, point, now)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ggoMonsterRecentContact(actor, side, node, point, now)
		if err != nil || got != want {
			t.Fatalf("now=%d got=%v want=%v err=%v", now, got, want, err)
		}
	}
	check(14000, contact, true)                            // recently walked from the contact point
	check(18000, contact, false)                           // last contact is outside the three-second window
	check(14000, &pb.PointNew{X: proto.Float32(4)}, false) // distant actor remains rejected
	point.Patrol = nil
	check(14000, &pb.PointNew{X: proto.Float32(.3)}, false) // stationary monster retains exact radius
}

func TestGGOPatrolBakedRoutesSharedClockAndPositionGuard(t *testing.T) {
	point := &model.AdminFieldGGOPoint{ID: "M05", Position: &model.AdminFieldGGOPosition{}}
	if _, err := ggoMonsterNavRoute(point); err != nil {
		t.Fatal(err)
	}
	if len(ggoMonsterNavRoutes) != 20 {
		t.Fatalf("expected 20 released NavMesh routes, got %d", len(ggoMonsterNavRoutes))
	}
	for id, waypoints := range ggoMonsterNavRoutes {
		t.Run(id, func(t *testing.T) {
			point := &model.AdminFieldGGOPoint{ID: id, Position: waypoints[0]}
			path, err := ggoPatrolPath(point)
			if err != nil || path == nil || len(path.Waypoints) < 2 {
				t.Fatalf("verified route unavailable: %v", err)
			}
			node := &ggoMonsterNode{PatrolEpochMS: 1000}
			start, err := ggoPatrolPosition(node, point, 13000)
			if err != nil {
				t.Fatal(err)
			}
			moving, err := ggoPatrolPosition(node, point, 13700)
			if err != nil || distanceGGOPositions(start, moving) < .4 {
				t.Fatalf("no short walk: %v", err)
			}
			for _, sample := range []struct {
				now      int64
				distance float64
			}{{13000, 0}, {13312, .2496}, {13625, .5}, {14260, 1}, {20000, 1}, {26875, .5}, {27510, 0}} {
				got, e := ggoPatrolPosition(node, point, sample.now)
				want := interpolateGGORoute(path.Waypoints, sample.distance)
				if e != nil || distanceGGOPositions(got, want) > 1e-7 {
					t.Fatalf("incorrect route projection at %d: %v", sample.now, e)
				}
			}
			other := *node
			also, err := ggoPatrolPosition(&other, point, 13700)
			if err != nil || distanceGGOPositions(moving, also) > 1e-9 {
				t.Fatalf("same-line observers differ: %v", err)
			}
			node.PatrolPausedAtMS = 13700
			frozen, err := ggoPatrolPosition(node, point, 19700)
			if err != nil || distanceGGOPositions(moving, frozen) > 1e-9 {
				t.Fatalf("fight did not freeze position: %v", err)
			}
			if err = ggoPatrolResume(node, 19700); err != nil {
				t.Fatal(err)
			}
			resumed, err := ggoPatrolPosition(node, point, 19700)
			if err != nil || distanceGGOPositions(moving, resumed) > 1e-9 {
				t.Fatalf("release changed the frozen position: %v", err)
			}
			x := *point.Position.X + .01
			changed := *point.Position
			changed.X = &x
			point.Position = &changed
			if candidate, err := ggoPatrolPath(point); err != nil || candidate != nil {
				t.Fatalf("route incorrectly applied to changed point: %v", err)
			}
		})
	}
}

func TestGGOPatrolMigratesPreviouslyStaticFightAtAnchor(t *testing.T) {
	if _, err := ggoMonsterNavRoute(&model.AdminFieldGGOPoint{ID: "M05", Position: &model.AdminFieldGGOPosition{}}); err != nil {
		t.Fatal(err)
	}
	point := &model.AdminFieldGGOPoint{ID: "M05", Position: ggoMonsterNavRoutes["M05"][0]}
	node := &ggoMonsterNode{ReservedBy: 7, PatrolEpochMS: 1000}
	changed, err := ggoMigrateReservedMonsterPatrol(node, point, 13700)
	if err != nil || !changed || node.PatrolEpochMS != 13700 || node.PatrolPausedAtMS != 13700 {
		t.Fatalf("previously static fight was not frozen: %v", err)
	}
	frozen, err := ggoPatrolPosition(node, point, 20000)
	if err != nil || distanceGGOPositions(frozen, point.Position) > 1e-9 {
		t.Fatalf("old fight moved from its accepted anchor: %v", err)
	}
	if changed, err = ggoMigrateReservedMonsterPatrol(node, point, 21000); err != nil || changed {
		t.Fatalf("paused clock migrated twice: %v", err)
	}
	if err = ggoPatrolResume(node, 22000); err != nil {
		t.Fatal(err)
	}
	resumed, err := ggoPatrolPosition(node, point, 22000)
	if err != nil || distanceGGOPositions(resumed, point.Position) > 1e-9 {
		t.Fatalf("old fight did not resume at the anchor: %v", err)
	}
}

func interpolateGGORoute(waypoints []*model.AdminFieldGGOPosition, distance float64) *model.AdminFieldGGOPosition {
	for i := 1; i < len(waypoints); i++ {
		a, b := waypoints[i-1], waypoints[i]
		segment := distanceGGOPositions(a, b)
		if distance <= segment || i == len(waypoints)-1 {
			fraction := math.Max(0, math.Min(1, distance/segment))
			x := *a.X + (*b.X-*a.X)*fraction
			y := *a.Y + (*b.Y-*a.Y)*fraction
			z := *a.Z + (*b.Z-*a.Z)*fraction
			return &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}
		}
		distance -= segment
	}
	return waypoints[len(waypoints)-1]
}

func distanceGGOPositions(a, b *model.AdminFieldGGOPosition) float64 {
	dx, dy, dz := *a.X-*b.X, *a.Y-*b.Y, *a.Z-*b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
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
