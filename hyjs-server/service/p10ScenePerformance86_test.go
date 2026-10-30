package service

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10PerformancePeer(id int64, sink *p10ClientSink) p10ScenePeer {
	return p10ScenePeer{frame: &pb.MovePlayer{Info: &pb.MovePlayerInfo{PlayerId: proto.Int64(id)}}, sink: sink}
}

func TestP10ScenePerformance86ScopeProjectionMatchesFullProfile(t *testing.T) {
	player := `{"warriorBagSize":10,"equipBagSize":10,"petBagSize":10}`
	for _, tc := range []struct {
		name, game, expected string
	}{
		{"town alias", `{"position":{"scene":"town"}}`, "town_main"},
		{"town loading", `{"position":{"scene":"town_main"},"position_loading":true}`, "town_main"},
		{"field one", `{"position":{"scene":"field_001"}}`, "field_001"},
		{"field two", `{"position":{"scene":"field_002"}}`, "field_002"},
		{"field loading", `{"position":{"scene":"field_001"},"position_loading":true}`, ""},
		{"desert marker", `{"position":{"scene":"field_003"},"ggo_field":{}}`, constUtil.GGOFieldScene},
		{"desert missing marker", `{"position":{"scene":"field_003"},"ggo_field":null}`, ""},
		{"desert loading", `{"position":{"scene":"field_003"},"position_loading":true,"ggo_field":{}}`, ""},
		{"run pending", `{"position":{"scene":"town_main"},"run":{"complete":false}}`, ""},
		{"run complete", `{"position":{"scene":"town_main"},"run":{"complete":true}}`, "town_main"},
		{"field fight pending", `{"position":{"scene":"field_001"},"field":{"fight":{"settled":false}}}`, ""},
		{"field fight settled", `{"position":{"scene":"field_001"},"field":{"fight":{"settled":true}}}`, "field_001"},
		{"unsupported scene", `{"position":{"scene":"unknown"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := &db.Profile{Player: &player, Game: &tc.game}
			_, original, err := decodeProfile(row)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := p10SceneScopeProfile(row)
			if err != nil {
				t.Fatal(err)
			}
			if got, full := p10SceneScope(projected), p10SceneScope(original); got != tc.expected || got != full {
				t.Fatalf("projected scope %q, full scope %q, expected %q", got, full, tc.expected)
			}
		})
	}
}

func TestP10ScenePerformance86ScopeProjectionValidation(t *testing.T) {
	validPlayer := `{"warriorBagSize":10,"equipBagSize":10,"petBagSize":10}`
	badPlayer := `{"level":"invalid"}`
	validGame := `{"position":{"scene":"town_main"}}`
	badGame := `{"position":{"scene":"town_main"},"warriors":[}`
	badPosition := `{"position":{"scene":1}}`
	badCapacity := `{"position":{"scene":"town_main"},"bag_capacity_scale":42}`
	badCredit := `{"position":{"scene":"town_main"},"items":[{"id":"item_9710","num":-1}]}`
	for _, tc := range []struct {
		name string
		row  *db.Profile
	}{
		{"missing player", &db.Profile{Game: &validGame}},
		{"missing game", &db.Profile{Player: &validPlayer}},
		{"invalid player", &db.Profile{Player: &badPlayer, Game: &validGame}},
		{"invalid complete JSON", &db.Profile{Player: &validPlayer, Game: &badGame}},
		{"invalid scope", &db.Profile{Player: &validPlayer, Game: &badPosition}},
		{"invalid capacity", &db.Profile{Player: &validPlayer, Game: &badCapacity}},
		{"invalid credit item", &db.Profile{Player: &validPlayer, Game: &badCredit}},
		{"invalid credit column", &db.Profile{Player: &validPlayer, Game: &validGame, GGOCredit: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := p10SceneScopeProfile(tc.row); err == nil {
				t.Fatal("invalid projected profile was accepted")
			}
			if _, _, err := decodeProfile(tc.row); err == nil {
				t.Fatal("full profile unexpectedly accepted the same invalid input")
			}
		})
	}
	// Directory visibility intentionally does not decode a valid-JSON roster;
	// the owner's complete profile path must still reject a corrupt envelope.
	damaged := `{"position":{"scene":"town_main"},"warriors":{"codec":"hyjs-warriors-zlib-v1","bytes":1,"sha256":"0000000000000000000000000000000000000000000000000000000000000000","data":"eA=="}}`
	row := &db.Profile{Player: &validPlayer, Game: &damaged}
	projected, err := p10SceneScopeProfile(row)
	if err != nil || p10SceneScope(projected) != "town_main" {
		t.Fatalf("directory did not use the projected scope: %v", err)
	}
	if _, _, err := decodeProfile(row); err == nil {
		t.Fatal("complete profile accepted the corrupted roster")
	}
}

func TestP10ScenePerformance86BoundedSynchronousFanout(t *testing.T) {
	const recipients = 9
	var peers []p10ScenePeer
	for id := int64(1); id <= recipients; id++ {
		peers = append(peers, p10PerformancePeer(id, &p10ClientSink{}))
	}
	peers = append(peers, p10PerformancePeer(100, nil))
	entered := make(chan int64, len(peers))
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var mu sync.Mutex
	counts := make(map[int64]int)
	go func() {
		send := func(peer p10ScenePeer) {
			id := peer.frame.GetInfo().GetPlayerId()
			entered <- id
			<-release
			mu.Lock()
			counts[id]++
			mu.Unlock()
		}
		var batches sync.WaitGroup
		for _, batch := range [][]p10ScenePeer{peers[:5], peers[5:]} {
			batches.Add(1)
			go func(batch []p10ScenePeer) {
				defer batches.Done()
				p10SceneForEachLivePeer(batch, send)
			}(batch)
		}
		batches.Wait()
		close(finished)
	}()
	for i := 0; i < constUtil.P10SceneBroadcastWorkers; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not start")
		}
	}
	select {
	case id := <-entered:
		t.Fatalf("more than %d sends started while workers were blocked: %d", constUtil.P10SceneBroadcastWorkers, id)
	case <-finished:
		t.Fatal("broadcast returned before its sends finished")
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast did not finish after releasing sends")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(counts) != recipients {
		t.Fatalf("expected %d live recipients, got %v", recipients, counts)
	}
	for id := int64(1); id <= recipients; id++ {
		if counts[id] != 1 {
			t.Fatalf("recipient %d delivered %d times", id, counts[id])
		}
	}
	if counts[100] != 0 {
		t.Fatal("offline presence was treated as a socket")
	}
}

func TestP10ScenePerformance86PinnedSelectionAndOfflineReconnect(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, b, c := p10SceneSetup(t)
	self, oldRecorder, newRecorder, offlineRecorder := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, self)
	p10SceneConnect(t, f, b, oldRecorder)
	self.drain()
	oldRecorder.drain()
	peers, err := f.s.p10ScenePeerEntries(db.GetIns().WithContext(f.ctx), "town_main", 1, f.id)
	if err != nil || len(peers) != 1 || peers[0].sink == nil {
		t.Fatalf("online directory selection: %v, %v", peers, err)
	}
	var offlineProfile db.Profile
	if err := db.GetIns().First(&offlineProfile, c).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&offlineProfile)
	if err != nil {
		t.Fatal(err)
	}
	game.Position.Scene = proto.String(constUtil.GGOFieldScene)
	game.PositionLoading = false
	game.GGOField = &model.GGOFieldState{}
	gameJSON, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", c).Update("game", string(gameJSON)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P10ScenePresence{PlayerID: c, Scene: constUtil.GGOFieldScene, Line: 1, Frame: p10BatchFrame(t, c), OfflineUntil: time.Now().Add(time.Minute).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	offline, err := f.s.p10ScenePeerEntries(db.GetIns().WithContext(f.ctx), constUtil.GGOFieldScene, 1, 0)
	if err != nil || len(offline) != 1 || offline[0].sink != nil || offline[0].frame.GetInfo().GetPlayerId() != c {
		t.Fatalf("offline directory selection: %v, %v", offline, err)
	}
	old := peers[0].sink
	key := p10ClientKey{f.s.p10ClientOwner(), b}
	replacement := &p10ClientSink{session: append([]byte(nil), old.session...), send: newRecorder.send}
	p10ClientSinks.Store(key, replacement)
	defer p10ClientSinks.CompareAndSwap(key, replacement, old)
	offlineKey := p10ClientKey{f.s.p10ClientOwner(), c}
	offlineReplacement := &p10ClientSink{send: offlineRecorder.send}
	p10ClientSinks.Store(offlineKey, offlineReplacement)
	defer p10ClientSinks.CompareAndDelete(offlineKey, offlineReplacement)
	peers = append(peers, offline...)
	f.s.p10SceneSendPeers(f.ctx, peers, &pb.SCMovePush{})
	if len(oldRecorder.drain()) != 0 || len(newRecorder.drain()) != 0 || len(offlineRecorder.drain()) != 0 {
		t.Fatal("a replacement or offline-reconnected socket inherited an earlier selection")
	}
}

func TestP10ScenePerformance86MoveCompletesBeforeLineChange(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, b, c := p10SceneSetup(t)
	self, oldLine, newLine := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, self)
	p10SceneConnect(t, f, b, oldLine)
	if err := db.GetIns().Create(&db.P10ScenePresence{PlayerID: c, Scene: "town_main", Line: 2, Frame: p10BatchFrame(t, c)}).Error; err != nil {
		t.Fatal(err)
	}
	p10SceneConnect(t, f, c, newLine)
	self.drain()
	oldLine.drain()
	newLine.drain()
	value, ok := p10LiveClient(p10ClientKey{f.s.p10ClientOwner(), b})
	if !ok {
		t.Fatal("old-line recipient is not connected")
	}
	originalSink := value.(*p10ClientSink)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	sink := &p10ClientSink{session: append([]byte(nil), originalSink.session...), send: func(message proto.Message) error {
		if _, ok := message.(*pb.SCMovePush); ok {
			entered <- struct{}{}
			<-release
		}
		return originalSink.send(message)
	}}
	key := p10ClientKey{f.s.p10ClientOwner(), b}
	p10ClientSinks.Store(key, sink)
	var calls sync.WaitGroup
	defer func() {
		unblock()
		finished := make(chan struct{})
		go func() { calls.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("timed out waiting for test calls during cleanup")
		}
		p10ClientSinks.CompareAndSwap(key, sink, originalSink)
	}()
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.Position.Current = &pb.PointNew{X: proto.Float32(8), Y: proto.Float32(0), Z: proto.Float32(9)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	moveDone := make(chan error, 1)
	calls.Add(1)
	go func() {
		defer calls.Done()
		moveDone <- f.s.SyncP10Scene(f.ctx, f.id, true)
		close(moveDone)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old-line movement did not reach the socket")
	}
	lineDone := make(chan error, 1)
	calls.Add(1)
	go func() {
		defer calls.Done()
		_, handled, err := f.s.DispatchP10Scene(f.ctx, f.id, &pb.CSTransferLine{Line: proto.Int32(2)}, 1)
		if !handled && err == nil {
			err = errors.New("line transfer was not handled")
		}
		lineDone <- err
		close(lineDone)
	}()
	select {
	case err := <-lineDone:
		t.Fatalf("line changed before the pending movement finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	for _, result := range []<-chan error{moveDone, lineDone} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("movement or line change did not complete")
		}
	}
	messages := oldLine.drain()
	if len(messages) != 2 {
		t.Fatalf("old line received unexpected messages: %v", messages)
	}
	if _, ok := messages[0].(*pb.SCMovePush); !ok || !p10SceneHasDisappear(messages[1:], f.id) {
		t.Fatalf("old line did not receive movement then disappearance: %v", messages)
	}
	if !p10SceneHasPlayer(newLine.drain(), f.id) {
		t.Fatal("new line did not receive the appearance")
	}
}
