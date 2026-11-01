package service

import (
	"context"
	"os"
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

func playerConcurrencyScenePosition(ctx context.Context, id int64, x float32) error {
	return updateProfile(ctx, id, func(_ *pb.Player, game *model.GameState) error {
		game.Position.Current = &pb.PointNew{X: proto.Float32(x), Y: proto.Float32(0), Z: proto.Float32(9)}
		return nil
	})
}

func playerConcurrencySceneSameLine(t *testing.T, ids ...int64) {
	t.Helper()
	var scene string
	var line int32
	for i, id := range ids {
		var presence db.P10ScenePresence
		if err := db.GetIns().First(&presence, "player_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		if presence.SessionHash == "" || len(presence.Frame) == 0 || presence.Line < 1 {
			t.Fatal("role is not connected to a visible scene")
		}
		if i == 0 {
			scene, line = presence.Scene, presence.Line
		} else if presence.Scene != scene || presence.Line != line {
			t.Fatal("roles are not on the same scene line")
		}
	}
}

func TestPlayerConcurrencyMySQLSceneMovement(t *testing.T) {
	if os.Getenv("HYJS_P10_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p10_ MySQL database")
	}
	f, b, c := p10SceneSetup(t)
	a := f.id
	for _, id := range []int64{a, b, c} {
		p10SceneConnect(t, f, id, &p10SceneRecorder{})
	}
	playerConcurrencySceneSameLine(t, a, b, c)
	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	defer cancel()
	if err := playerConcurrencyScenePosition(ctx, a, 8); err != nil {
		t.Fatal(err)
	}
	if err := playerConcurrencyScenePosition(ctx, c, 18); err != nil {
		t.Fatal(err)
	}

	entered, release := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	var releaseOnce sync.Once
	releaseA := func() { releaseOnce.Do(func() { close(release) }) }
	callback := db.GetIns().Callback().Update()
	const callbackName = "player_concurrency_scene_presence"
	if err := callback.Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		fields, ok := tx.Statement.Dest.(map[string]any)
		if tx.Statement.Table != "p10_scene_presence" || !ok {
			return
		}
		if _, writesFrame := fields["frame"]; !writesFrame || !blocked.CompareAndSwap(false, true) {
			return
		}
		// A is the only sync started before this callback is entered.
		close(entered)
		select {
		case <-release:
		case <-tx.Statement.Context.Done():
			tx.AddError(tx.Statement.Context.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	var aDone, cDone <-chan error
	defer func() {
		releaseA()
		cancel()
		if aDone != nil {
			<-aDone
		}
		if cDone != nil {
			<-cDone
		}
		if err := callback.Remove(callbackName); err != nil {
			t.Errorf("remove presence callback: %v", err)
		}
	}()
	resultA := make(chan error, 1)
	aDone = resultA
	go func() { resultA <- f.s.SyncP10Scene(ctx, a, true) }()
	entryTimer := time.NewTimer(2 * time.Second)
	defer entryTimer.Stop()
	select {
	case <-entered:
	case err := <-aDone:
		aDone = nil
		t.Fatalf("first movement completed before the frame write was blocked: %v", err)
	case <-entryTimer.C:
		t.Fatal("first movement did not reach the presence update")
	case <-ctx.Done():
		t.Fatalf("first movement timed out: %v", ctx.Err())
	}
	resultC := make(chan error, 1)
	cDone = resultC
	go func() { resultC <- f.s.SyncP10Scene(ctx, c, true) }()
	peerTimer := time.NewTimer(2 * time.Second)
	defer peerTimer.Stop()
	select {
	case err := <-cDone:
		cDone = nil
		if err != nil {
			t.Fatalf("independent movement failed: %v", err)
		}
	case <-peerTimer.C:
		t.Fatal("independent movement waited for the blocked presence update")
	case <-ctx.Done():
		t.Fatalf("independent movement timed out: %v", ctx.Err())
	}
	select {
	case err := <-aDone:
		aDone = nil
		t.Fatalf("first movement was no longer blocked: %v", err)
	default:
	}
	releaseA()
	select {
	case err := <-aDone:
		aDone = nil
		if err != nil {
			t.Fatalf("first movement failed after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("first movement did not finish after release: %v", ctx.Err())
	}
}

func TestPlayerConcurrencyMySQLSlowSocket(t *testing.T) {
	if os.Getenv("HYJS_P10_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p10_ MySQL database")
	}
	f, b, c := p10SceneSetup(t)
	a := f.id
	p10SceneConnect(t, f, a, &p10SceneRecorder{})
	p10SceneConnect(t, f, c, &p10SceneRecorder{})
	var profile db.Profile
	if err := db.GetIns().First(&profile, b).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	entered, release, sendExited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var blockOnce, releaseOnce sync.Once
	releaseB := func() { releaseOnce.Do(func() { close(release) }) }
	send := func(message proto.Message) error {
		if _, ok := message.(*pb.SCMovePush); !ok {
			return nil
		}
		first := false
		blockOnce.Do(func() {
			first = true
			close(entered)
		})
		if !first {
			return nil
		}
		defer close(sendExited)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	var aDone, cDone <-chan error
	var unregister func()
	defer func() {
		releaseB()
		cancel()
		if aDone != nil {
			<-aDone
		}
		if cDone != nil {
			<-cDone
		}
		if unregister != nil {
			unregister()
		}
		select {
		case <-entered:
			<-sendExited
		default:
		}
	}()
	unregister = f.s.RegisterP10Client(ctx, b, profile.Session, send, releaseB)
	playerConcurrencySceneSameLine(t, a, b, c)
	if err := playerConcurrencyScenePosition(ctx, a, 8); err != nil {
		t.Fatal(err)
	}
	resultA := make(chan error, 1)
	aDone = resultA
	go func() { resultA <- f.s.SyncP10Scene(ctx, a, true) }()
	var enteredWait <-chan struct{} = entered
	timerA := time.NewTimer(2 * time.Second)
	defer timerA.Stop()
	for aDone != nil || enteredWait != nil {
		select {
		case <-enteredWait:
			enteredWait = nil
		case err := <-aDone:
			aDone = nil
			if err != nil {
				t.Fatalf("first movement failed: %v", err)
			}
		case <-timerA.C:
			t.Fatal("first movement or slow socket did not reach the required state")
		case <-ctx.Done():
			t.Fatalf("first movement timed out: %v", ctx.Err())
		}
	}
	select {
	case <-sendExited:
		t.Fatal("slow socket stopped blocking before release")
	default:
	}
	resultC := make(chan error, 1)
	cDone = resultC
	go func() {
		err := playerConcurrencyScenePosition(ctx, c, 18)
		if err == nil {
			err = f.s.SyncP10Scene(ctx, c, true)
		}
		resultC <- err
	}()
	timerC := time.NewTimer(2 * time.Second)
	defer timerC.Stop()
	select {
	case err := <-cDone:
		cDone = nil
		if err != nil {
			t.Fatalf("independent movement failed: %v", err)
		}
	case <-timerC.C:
		t.Fatal("independent movement waited for the slow socket")
	case <-ctx.Done():
		t.Fatalf("independent movement timed out: %v", ctx.Err())
	}
	select {
	case <-sendExited:
		t.Fatal("slow socket stopped blocking before the second movement finished")
	default:
	}
	releaseB()
}
