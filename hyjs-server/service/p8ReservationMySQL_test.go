package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p8ReservationHoldKey struct{}
type p8AutoRelineProbeKey struct{}

func TestP8ReservationMySQLSamePointSerializesOnlyItsLine(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p8ReservationSetup(t, "field_002", "mine")
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	if err = p8CreateReservationGate(f.ctx, &p8ReservationGateMissing{serverID: f.s.OriginalServerID(), scene: "field_002", posID: f.pos[0]}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.base, 20*time.Second)
	defer cancel()
	ready := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var held atomic.Bool
	database := db.GetIns()
	const callback = "test:p8-reservation-hold-line"
	if err = database.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "p8_resource_reservations" || tx.Statement.Context.Value(p8ReservationHoldKey{}) != true || !held.CompareAndSwap(false, true) {
			return
		}
		close(ready)
		select {
		case <-release:
		case <-ctx.Done():
			tx.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Callback().Update().Remove(callback) }()
	type result struct {
		messages []proto.Message
		err      error
	}
	var workers sync.WaitGroup
	defer func() {
		once.Do(func() { close(release) })
		workers.Wait()
	}()
	launch := func(index int, hold bool) <-chan result {
		out := make(chan result, 1)
		workers.Add(1)
		go func() {
			defer workers.Done()
			request := &pb.CSFieldResourceStartGather{PosId: proto.String(f.pos[index])}
			requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, request)
			if hold {
				requestCtx = context.WithValue(requestCtx, p8ReservationHoldKey{}, true)
			}
			messages, e := f.s.p8StartGather(requestCtx, f.ids[index], request, 0)
			out <- result{messages: messages, err: e}
		}()
		return out
	}
	first := launch(0, true)
	select {
	case <-ready:
	case r := <-first:
		t.Fatalf("first start finished before its point lock: %v", r.err)
	case <-ctx.Done():
		t.Fatal("first start did not lock its point")
	}
	otherLine := launch(3, false)
	otherPoint := launch(2, false)
	for label, result := range map[string]<-chan result{"same point other line": otherLine, "same line other point": otherPoint} {
		select {
		case r := <-result:
			if r.err != nil || len(r.messages) == 0 || r.messages[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
				t.Fatalf("%s failed while unrelated point was locked: %v", label, r.err)
			}
		case <-time.After(4 * time.Second):
			t.Fatalf("%s waited on another resource's held lock", label)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	sameLine := launch(1, false)
	select {
	case r := <-sameLine:
		t.Fatalf("same point and line passed an uncommitted owner: %v", r.err)
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	once.Do(func() { close(release) })
	select {
	case r := <-first:
		if r.err != nil || len(r.messages) == 0 || r.messages[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
			t.Fatalf("held start failed: %v", r.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case r := <-sameLine:
		if r.err == nil || !strings.Contains(r.err.Error(), constUtil.P8ResourceOccupied) || len(r.messages) != 0 {
			t.Fatalf("same point and line did not reject the later start: %+v", r)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestP8MySQLCancelDoesNotWaitForOtherLine(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p8ReservationSetup(t, "field_002", "mine")
	p8ExpectStart(t, f, 0, 0)
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	ctx, cancel := context.WithTimeout(f.base, 20*time.Second)
	defer cancel()
	held, release := make(chan struct{}), make(chan struct{})
	var heldOnce atomic.Bool
	var releaseOnce sync.Once
	var workers sync.WaitGroup
	database := db.GetIns()
	const callback = "test:p8-reservation-other-line-cancel"
	if err = database.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "p8_resource_reservations" || tx.Statement.Context.Value(p8ReservationHoldKey{}) != true || !heldOnce.CompareAndSwap(false, true) {
			return
		}
		close(held)
		select {
		case <-release:
		case <-ctx.Done():
			tx.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Callback().Update().Remove(callback) }()
	defer func() {
		releaseOnce.Do(func() { close(release) })
		workers.Wait()
	}()
	startDone, cancelDone := make(chan error, 1), make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		request := &pb.CSFieldResourceStartGather{PosId: proto.String(f.pos[3])}
		requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, request)
		requestCtx = context.WithValue(requestCtx, p8ReservationHoldKey{}, true)
		_, e := f.s.p8StartGather(requestCtx, f.ids[3], request, 0)
		startDone <- e
	}()
	select {
	case <-held:
	case e := <-startDone:
		t.Fatalf("other-line start exited before holding its row: %v", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		cancelDone <- updateProfileWithTx(ctx, f.ids[0], func(tx *gorm.DB, _ *pb.Player, game *model.GameState) error {
			return f.s.P8CancelGather(tx, f.ids[0], game)
		})
	}()
	select {
	case e := <-cancelDone:
		if e != nil {
			t.Fatalf("cancel failed during unrelated line update: %v", e)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("cancel waited for a different line of the same point")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case e := <-startDone:
		if e != nil {
			t.Fatalf("other-line start failed: %v", e)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if f.attempt(t, 0).Active || !f.attempt(t, 3).Active {
		t.Fatal("cancel changed the other line's accepted attempt")
	}
}

func TestP8MySQLStartAndAutomaticRelineSerializeOnProfile(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	for _, first := range []string{"start", "reline"} {
		t.Run(first+"-first", func(t *testing.T) {
			f := p8ReservationSetup(t, "field_002", "mine")
			pool, err := db.GetIns().DB()
			if err != nil {
				t.Fatal(err)
			}
			pool.SetMaxOpenConns(8)
			pool.SetMaxIdleConns(8)
			if err = db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).
				Updates(map[string]any{"enabled": true, "line_count": 2, "max_players": 1}).Error; err != nil {
				t.Fatal(err)
			}
			// One authenticated visible peer fills line 1; other fixture frames
			// remain empty, so the real sync selects line 2 for this player.
			peerFrame, err := proto.Marshal(&pb.MovePlayer{Info: &pb.MovePlayerInfo{PlayerId: proto.Int64(f.ids[1])}})
			if err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Update("frame", peerFrame).Error; err != nil {
				t.Fatal(err)
			}
			if err = p8CreateReservationGate(f.base, &p8ReservationGateMissing{serverID: f.s.OriginalServerID(), scene: "field_002", posID: f.pos[0]}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(f.base, 25*time.Second)
			defer cancel()
			held, otherAtProfile, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			var heldOnce, probeOnce atomic.Bool
			var workers sync.WaitGroup
			database := db.GetIns()
			const updateCallback = "test:p8-auto-reline-hold"
			const queryCallback = "test:p8-auto-reline-probe"
			if err = database.Callback().Update().After("gorm:update").Register(updateCallback, func(tx *gorm.DB) {
				operation, _ := tx.Statement.Context.Value(p8AutoRelineProbeKey{}).(string)
				table := "p8_resource_reservations"
				if first == "reline" {
					table = "p10_scene_presences"
				}
				if operation != first || tx.Statement.Table != table || !heldOnce.CompareAndSwap(false, true) {
					return
				}
				close(held)
				select {
				case <-release:
				case <-ctx.Done():
					tx.AddError(ctx.Err())
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Callback().Update().Remove(updateCallback) }()
			if err = database.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
				operation, _ := tx.Statement.Context.Value(p8AutoRelineProbeKey{}).(string)
				locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
				if operation == "" || operation == first || tx.Statement.Table != "profiles" || !ok || locking.Strength != "UPDATE" || !probeOnce.CompareAndSwap(false, true) {
					return
				}
				close(otherAtProfile)
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Callback().Query().Remove(queryCallback) }()
			defer func() {
				releaseOnce.Do(func() { close(release) })
				workers.Wait()
			}()
			startDone, relineDone := make(chan error, 1), make(chan error, 1)
			start := func() {
				workers.Add(1)
				go func() {
					defer workers.Done()
					request := &pb.CSFieldResourceStartGather{PosId: proto.String(f.pos[0])}
					requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, request)
					requestCtx = context.WithValue(requestCtx, p8AutoRelineProbeKey{}, "start")
					messages, e := f.s.p8StartGather(requestCtx, f.ids[0], request, 0)
					if e == nil && (len(messages) == 0 || messages[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable) {
						e = fmt.Errorf("start did not accept the resource")
					}
					startDone <- e
				}()
			}
			reline := func() {
				workers.Add(1)
				go func() {
					defer workers.Done()
					relineCtx := context.WithValue(ctx, p8AutoRelineProbeKey{}, "reline")
					relineDone <- f.s.SyncP10Scene(relineCtx, f.ids[0], false)
				}()
			}
			if first == "start" {
				start()
			} else {
				reline()
			}
			select {
			case <-held:
			case e := <-startDone:
				t.Fatalf("start exited before its reservation was held: %v", e)
			case e := <-relineDone:
				t.Fatalf("automatic reline exited before its update was held: %v", e)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if first == "start" {
				reline()
			} else {
				start()
			}
			select {
			case <-otherAtProfile:
			case e := <-startDone:
				t.Fatalf("start exited before waiting for the profile: %v", e)
			case e := <-relineDone:
				t.Fatalf("automatic reline exited before waiting for the profile: %v", e)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case e := <-startDone:
				t.Fatalf("start passed the held profile: %v", e)
			case e := <-relineDone:
				t.Fatalf("automatic reline passed the held profile: %v", e)
			case <-time.After(200 * time.Millisecond):
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			releaseOnce.Do(func() { close(release) })
			var startErr, relineErr error
			select {
			case startErr = <-startDone:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case relineErr = <-relineDone:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if startErr != nil || first == "start" && relineErr == nil || first == "reline" && relineErr != nil {
				t.Fatalf("unexpected interleaving: start=%v reline=%v", startErr, relineErr)
			}
			line := int32(1)
			if first == "reline" {
				line = 2
			}
			var presence db.P10ScenePresence
			if err = db.GetIns().First(&presence, "player_id=?", f.ids[0]).Error; err != nil || presence.Line != line {
				t.Fatalf("wrong committed line: %+v %v", presence, err)
			}
			attempt := f.attempt(t, 0)
			var reservation db.P8ResourceReservation
			err = db.GetIns().Where("server_id=? AND scene=? AND pos_id=? AND line=?", f.s.OriginalServerID(), "field_002", f.pos[0], line).First(&reservation).Error
			if err != nil || !attempt.Active || reservation.PlayerID != f.ids[0] || reservation.Token != attempt.Token {
				t.Fatalf("attempt and line reservation diverged: attempt=%+v reservation=%+v err=%v", attempt, reservation, err)
			}
		})
	}
}
