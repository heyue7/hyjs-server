package service

import (
	"context"
	"database/sql"
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

func sceneRound2MySQL(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	name := os.Getenv("HYJS_P10_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p10_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p10_") || len(name) == len("hyjs_test_p10_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f, b, c := p10SceneSetup(t)
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("requires MySQL")
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	for _, id := range []int64{f.id, b, c} {
		p10SceneConnect(t, f, id, &p10SceneRecorder{})
	}
	return f, b, c
}

func sceneRound2SetScope(t *testing.T, ctx context.Context, id int64, scene string) {
	t.Helper()
	if err := updateProfile(ctx, id, func(_ *pb.Player, game *model.GameState) error {
		game.Position.Scene = proto.String(scene)
		game.PositionLoading = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type sceneRound2PauseKey struct{}

// Pause the marked actor at its actual presence UPDATE, inside GORM's write
// transaction, while another request uses its own database connection.
func sceneRound2PauseUpdate(t *testing.T, ctx context.Context) (context.Context, <-chan struct{}, func()) {
	t.Helper()
	marker := new(int)
	marked := context.WithValue(ctx, sceneRound2PauseKey{}, marker)
	entered, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	callback := db.GetIns().Callback().Update()
	const name = "scene_round2_pause_presence_update"
	if err := callback.Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "p10_scene_presence" || tx.Statement.Context.Value(sceneRound2PauseKey{}) != marker {
			return
		}
		once.Do(func() {
			close(entered)
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
				tx.AddError(tx.Statement.Context.Err())
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	return marked, entered, func() {
		releaseOnce.Do(func() { close(release) })
	}
}

func sceneRound2Wait(t *testing.T, done <-chan error, label string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(8 * time.Second):
		t.Fatalf("%s did not complete", label)
		return nil
	}
}

func sceneRound2Drain(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("scene requests remained blocked during cleanup")
	}
}

func sceneRound2Entered(t *testing.T, entered <-chan struct{}, done <-chan error) {
	t.Helper()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("first membership write returned before the pause: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("first membership write did not reach the pause")
	}
}

func TestSceneRound2MySQLIndependentRoomsAndMovement(t *testing.T) {
	f, b, c := sceneRound2MySQL(t)
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, b, &pb.CSTransferLine{Line: proto.Int32(2)}, 0); !handled || err != nil {
		t.Fatalf("park player on line 2: %v", err)
	}
	sceneRound2SetScope(t, f.ctx, f.id, "field_001")
	sceneRound2SetScope(t, f.ctx, b, "field_001")
	if err := playerConcurrencyScenePosition(f.ctx, c, 37); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 25*time.Second)
	marked, entered, release := sceneRound2PauseUpdate(t, ctx)
	var wg sync.WaitGroup
	defer func() {
		release()
		cancel()
		sceneRound2Drain(t, &wg)
		if err := db.GetIns().Callback().Update().Remove("scene_round2_pause_presence_update"); err != nil {
			t.Error(err)
		}
	}()
	first := make(chan error, 1)
	wg.Add(1)
	go func() { defer wg.Done(); first <- f.s.SyncP10Scene(marked, f.id, false) }()
	sceneRound2Entered(t, entered, first)
	other := make(chan error, 1)
	move := make(chan error, 1)
	wg.Add(2)
	go func() { defer wg.Done(); other <- f.s.SyncP10Scene(ctx, b, false) }()
	go func() { defer wg.Done(); move <- f.s.SyncP10Scene(ctx, c, true) }()
	if err := sceneRound2Wait(t, other, "independent room entry"); err != nil {
		t.Fatalf("independent room entry failed: %v", err)
	}
	if err := sceneRound2Wait(t, move, "stable movement"); err != nil {
		t.Fatalf("stable movement failed: %v", err)
	}
	select {
	case err := <-first:
		t.Fatalf("held transaction completed before release: %v", err)
	default:
	}
	release()
	if err := sceneRound2Wait(t, first, "first room entry"); err != nil {
		t.Fatal(err)
	}
}

func TestSceneRound2MySQLLastRoomSlot(t *testing.T) {
	f, b, c := sceneRound2MySQL(t)
	sceneRound2SetScope(t, f.ctx, c, "field_001")
	if err := f.s.SyncP10Scene(f.ctx, c, false); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=1").Updates(map[string]any{"line_count": 1, "max_players": 2}).Error; err != nil {
		t.Fatal(err)
	}
	sceneRound2SetScope(t, f.ctx, f.id, "field_001")
	sceneRound2SetScope(t, f.ctx, b, "field_001")
	ctx, cancel := context.WithTimeout(f.ctx, 25*time.Second)
	marked, entered, release := sceneRound2PauseUpdate(t, ctx)
	var wg sync.WaitGroup
	defer func() {
		release()
		cancel()
		sceneRound2Drain(t, &wg)
		if err := db.GetIns().Callback().Update().Remove("scene_round2_pause_presence_update"); err != nil {
			t.Error(err)
		}
	}()
	first, second := make(chan error, 1), make(chan error, 1)
	wg.Add(1)
	go func() { defer wg.Done(); first <- f.s.SyncP10Scene(marked, f.id, false) }()
	sceneRound2Entered(t, entered, first)
	wg.Add(1)
	go func() { defer wg.Done(); second <- f.s.SyncP10Scene(ctx, b, false) }()
	database, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	key := p10SceneLockKey{database: database, kind: 2, room: p10SceneRoom{scene: "field_001", line: 1}}
	deadline := time.After(8 * time.Second)
	for {
		p10SceneLockRegistry.Lock()
		entry := p10SceneLockRegistry.entries[key]
		waiting := entry != nil && entry.refs >= 2
		p10SceneLockRegistry.Unlock()
		if waiting {
			break
		}
		select {
		case err := <-second:
			t.Fatalf("second entry returned before waiting for capacity: %v", err)
		case <-deadline:
			t.Fatal("second entry did not wait for the selected room")
		case <-time.After(5 * time.Millisecond):
		}
	}
	release()
	if err := sceneRound2Wait(t, first, "first room entry"); err != nil {
		t.Fatal(err)
	}
	if err := sceneRound2Wait(t, second, "second room entry"); err == nil {
		t.Fatal("second entry occupied an already full last slot")
	}
	peers, err := f.s.p10ScenePeers(db.GetIns().WithContext(ctx), "field_001", 1, 0)
	if err != nil || len(peers) != 2 {
		t.Fatalf("expected exactly two live occupants after competition: count=%d err=%v", len(peers), err)
	}
}

func TestSceneRound2MySQLReverseTransfers(t *testing.T) {
	f, b, _ := sceneRound2MySQL(t)
	if _, _, err := f.s.DispatchP10Scene(f.ctx, b, &pb.CSTransferLine{Line: proto.Int32(2)}, 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, transfer := range []struct {
		id   int64
		line int32
	}{{f.id, 2}, {b, 1}} {
		wg.Add(1)
		go func(id int64, line int32) {
			defer wg.Done()
			<-start
			_, handled, err := f.s.DispatchP10Scene(ctx, id, &pb.CSTransferLine{Line: proto.Int32(line)}, 0)
			if !handled && err == nil {
				err = context.Canceled
			}
			results <- err
		}(transfer.id, transfer.line)
	}
	defer func() {
		cancel()
		sceneRound2Drain(t, &wg)
	}()
	close(start)
	for i := 0; i < 2; i++ {
		if err := sceneRound2Wait(t, results, "reverse transfer"); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct {
		id   int64
		line int32
	}{{f.id, 2}, {b, 1}} {
		var row db.P10ScenePresence
		if err := db.GetIns().First(&row, "player_id=?", want.id).Error; err != nil || row.Line != want.line {
			t.Fatalf("reverse transfer player=%d line=%d err=%v", want.id, row.Line, err)
		}
	}
}

func TestSceneRound2MySQLOldDisconnectKeepsReplacement(t *testing.T) {
	f, _, _ := sceneRound2MySQL(t)
	var old db.Profile
	if err := db.GetIns().First(&old, f.id).Error; err != nil {
		t.Fatal(err)
	}
	newSession, err := LoginAccount(f.ctx, old.Uid, "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	closeNew := f.s.RegisterP10Client(f.ctx, f.id, newSession.Session, (&p10SceneRecorder{}).send)
	t.Cleanup(closeNew)
	if err := f.s.SyncP10Scene(f.ctx, f.id, false); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DisconnectP10Scene(f.ctx, f.id, old.Session); err != nil {
		t.Fatal(err)
	}
	var presence db.P10ScenePresence
	if err := db.GetIns().First(&presence, "player_id=?", f.id).Error; err != nil || presence.SessionHash != p6Digest(newSession.Session) {
		t.Fatalf("old disconnect changed replacement presence: %v", err)
	}
}

func TestSceneRound2RegistryReclaimsWaiters(t *testing.T) {
	key := p10SceneLockKey{database: &sql.DB{}, kind: 1, playerID: 29}
	other := p10SceneLockKey{database: &sql.DB{}, kind: 1, playerID: 29}
	unlock := p10SceneAcquire([]p10SceneLockKey{key}, false)
	otherUnlock := p10SceneAcquire([]p10SceneLockKey{other}, false)
	otherUnlock()
	entered, done := make(chan struct{}), make(chan struct{})
	go func() {
		close(entered)
		release := p10SceneAcquire([]p10SceneLockKey{key}, true)
		release()
		close(done)
	}()
	<-entered
	deadline := time.After(3 * time.Second)
	for {
		p10SceneLockRegistry.Lock()
		entry := p10SceneLockRegistry.entries[key]
		waiting := entry != nil && entry.refs == 2
		p10SceneLockRegistry.Unlock()
		if waiting {
			break
		}
		select {
		case <-deadline:
			unlock()
			t.Fatal("waiting reader was not retained in the registry")
		case <-time.After(time.Millisecond):
		}
	}
	unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("waiting reader did not acquire and release")
	}
	p10SceneLockRegistry.Lock()
	_, exists := p10SceneLockRegistry.entries[key]
	_, otherExists := p10SceneLockRegistry.entries[other]
	p10SceneLockRegistry.Unlock()
	if exists || otherExists {
		t.Fatal("unused scene lock entries were retained")
	}
	rooms := p10SceneRooms(p10SceneRoom{"b", 1}, p10SceneRoom{"a", 2}, p10SceneRoom{"a", 1}, p10SceneRoom{"a", 2})
	if len(rooms) != 3 || rooms[0] != (p10SceneRoom{"a", 1}) || rooms[1] != (p10SceneRoom{"a", 2}) || rooms[2] != (p10SceneRoom{"b", 1}) {
		t.Fatalf("room order or duplicate elimination failed: %v", rooms)
	}
}
