package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func connectionDrainRound2Marker(message proto.Message, marker int64) bool {
	disappear, ok := message.(*pb.SCPlayerDisappear)
	return ok && len(disappear.GetPlayerId()) == 1 && disappear.GetPlayerId()[0] == marker
}

func TestConnectionDrainRound2MySQLReplacementBarrier(t *testing.T) {
	f, otherID, _ := sceneRound2MySQL(t)
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 25*time.Second)
	closed, entered, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	closeBeforeDrain := make(chan bool, 1)
	oldPush, newPush := make(chan struct{}, 4), make(chan struct{}, 4)
	var closeOnce, releaseOnce sync.Once
	releaseDrain := func() { releaseOnce.Do(func() { close(release) }) }
	const marker = int64(-752001)
	oldClose := f.s.RegisterP10Client(ctx, f.id, profile.Session, func(message proto.Message) error {
		if connectionDrainRound2Marker(message, marker) {
			select {
			case oldPush <- struct{}{}:
			default:
			}
		}
		return nil
	}, func() {
		closeOnce.Do(func() { close(closed) })
	}, func() {
		select {
		case <-closed:
			closeBeforeDrain <- true
		default:
			closeBeforeDrain <- false
		}
		close(entered)
		<-release
	})
	key := p10ClientKey{f.s.p10ClientOwner(), f.id}
	value, ok := p10LiveClient(key)
	if !ok {
		oldClose()
		cancel()
		t.Fatal("old connection did not register")
	}
	oldSink := value.(*p10ClientSink)
	if oldSink.drain == nil {
		oldClose()
		cancel()
		t.Fatal("old connection did not retain its drain callback")
	}

	registration := make(chan func(), 1)
	movement := make(chan error, 1)
	var registrationStarted, registrationCollected, movementStarted, movementCollected bool
	var newClose func()
	t.Cleanup(func() {
		releaseDrain()
		if registrationStarted && !registrationCollected {
			select {
			case newClose = <-registration:
				registrationCollected = true
			case <-time.After(8 * time.Second):
				t.Error("replacement registration did not exit during cleanup")
			}
		}
		if newClose != nil {
			newClose()
		}
		if !registrationStarted || registrationCollected {
			oldClose()
		}
		cancel()
		if movementStarted && !movementCollected {
			select {
			case <-movement:
			case <-time.After(8 * time.Second):
				t.Error("other player's movement did not exit during cleanup")
			}
		}
	})

	registrationStarted = true
	go func() {
		registration <- f.s.RegisterP10Client(ctx, f.id, profile.Session, func(message proto.Message) error {
			if connectionDrainRound2Marker(message, marker) {
				select {
				case newPush <- struct{}{}:
				default:
				}
			}
			return nil
		}, func() {})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("replacement did not reach the old connection's drain")
	}
	if !<-closeBeforeDrain {
		t.Fatal("old connection was not closed before drain started")
	}
	stored, ok := p10ClientSinks.Load(key)
	if !ok || stored != oldSink {
		t.Fatal("replacement was installed before the old drain finished")
	}
	if _, live := p10LiveClient(key); live {
		t.Fatal("closed old connection remained eligible for delivery")
	}
	select {
	case newClose = <-registration:
		registrationCollected = true
		t.Fatal("replacement registration returned before the old drain finished")
	default:
	}

	movementStarted = true
	go func() {
		if err := playerConcurrencyScenePosition(ctx, otherID, 53); err != nil {
			movement <- err
			return
		}
		movement <- f.s.SyncP10Scene(ctx, otherID, true)
	}()
	select {
	case err := <-movement:
		movementCollected = true
		if err != nil {
			t.Fatalf("other player's movement failed during the drain: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("other player's movement waited for the old connection's drain")
	}
	var otherPresence db.P10ScenePresence
	if err := db.GetIns().WithContext(ctx).First(&otherPresence, "player_id=?", otherID).Error; err != nil {
		t.Fatal(err)
	}
	var otherFrame pb.MovePlayer
	if err := proto.Unmarshal(otherPresence.Frame, &otherFrame); err != nil || otherFrame.GetPosition().GetCurrent().GetX() != 53 {
		t.Fatalf("other player's movement was not persisted while drain waited: %v", err)
	}
	select {
	case newClose = <-registration:
		registrationCollected = true
		t.Fatal("replacement returned while the old drain was still held")
	default:
	}
	probe := &pb.SCPlayerDisappear{PlayerId: []int64{marker}}
	f.s.publishP10PrivateBatchPinned(ctx, f.id, oldSink, []proto.Message{probe})
	select {
	case <-oldPush:
		t.Fatal("old connection received a push after it was closed")
	default:
	}

	releaseDrain()
	select {
	case newClose = <-registration:
		registrationCollected = true
	case <-ctx.Done():
		t.Fatal("replacement did not finish after releasing the old drain")
	}
	value, ok = p10LiveClient(key)
	if !ok || value == oldSink {
		t.Fatal("replacement did not become the live connection")
	}
	var presence db.P10ScenePresence
	if err := db.GetIns().First(&presence, "player_id=?", f.id).Error; err != nil || presence.SessionHash != p6Digest(profile.Session) {
		t.Fatalf("replacement did not restore authenticated presence: %v", err)
	}
	if frame, live := f.s.p10SceneLive(db.GetIns().WithContext(ctx), presence); !live || frame.GetInfo().GetPlayerId() != f.id {
		t.Fatal("replacement did not restore a live visible actor")
	}
	f.s.publishP10PrivateBatchPinned(ctx, f.id, oldSink, []proto.Message{probe})
	f.s.publishP10Private(ctx, f.id, probe)
	select {
	case <-newPush:
	case <-ctx.Done():
		t.Fatal("replacement did not receive its authorized push")
	}
	select {
	case <-oldPush:
		t.Fatal("old connection received a push after replacement")
	default:
	}
}

func TestConnectionDrainRound2SelfUnregisterDoesNotDrain(t *testing.T) {
	f, _, _ := p10SceneSetup(t)
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	drainCalled, releaseDrain := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDrain) }) }
	defer release()
	unregister := f.s.RegisterP10Client(ctx, f.id, profile.Session, func(proto.Message) error { return nil }, func() {}, func() {
		drainCalled <- struct{}{}
		<-releaseDrain
	})
	key := p10ClientKey{f.s.p10ClientOwner(), f.id}
	if _, ok := p10LiveClient(key); !ok {
		t.Fatal("connection did not register")
	}
	done := make(chan struct{})
	go func() {
		// Drain represents this dispatch exiting, which occurs after unregister.
		unregister()
		release()
		close(done)
	}()
	defer func() {
		release()
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("self-unregister remained blocked during cleanup")
		}
	}()
	select {
	case <-done:
	case <-drainCalled:
		release()
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("self-unregister stayed blocked after releasing drain")
		}
		t.Fatal("self-unregister waited for its own drain")
	case <-ctx.Done():
		release()
		t.Fatal("self-unregister did not finish")
	}
	select {
	case <-drainCalled:
		t.Fatal("self-unregister invoked its own drain")
	default:
	}
	if _, ok := p10ClientSinks.Load(key); ok {
		t.Fatal("self-unregister left its sink installed")
	}
}
