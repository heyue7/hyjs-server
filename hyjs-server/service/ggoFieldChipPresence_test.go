package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOChipNoticesFollowCommitAndLine(t *testing.T) {
	f := ggoTradeSetup(t)
	f.s.GGOIntegrationReady = true
	t.Cleanup(f.s.ggoStopNoticeTimers)
	received := make(chan int, 8)
	for i, id := range f.ids {
		index := i
		key := p10ClientKey{f.s.p10ClientOwner(), id}
		old, _ := p10ClientSinks.Load(key)
		sink := &p10ClientSink{session: f.sessions[i], send: func(message proto.Message) error {
			if _, ok := message.(*pb.SCFieldResourceInfo); ok {
				received <- index
			}
			return nil
		}}
		p10ClientSinks.Store(key, sink)
		t.Cleanup(func() { p10ClientSinks.CompareAndSwap(key, sink, old) })
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	abort := errors.New("cancel mutation")
	if err := updateProfileWithTx(ctx, f.ids[0], func(_ *gorm.DB, _ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips[0]++
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal("rollback", err)
	}
	select {
	case index := <-received:
		t.Fatal("notification for rolled back change", index)
	case <-time.After(150 * time.Millisecond):
	}
	if err := updateProfileWithTx(ctx, f.ids[0], func(_ *gorm.DB, _ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips[0]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case index := <-received:
		if index != 0 {
			t.Fatal("notification crossed lines", index)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no committed change notification")
	}
	select {
	case index := <-received:
		t.Fatal("unexpected other-line notification", index)
	case <-time.After(150 * time.Millisecond):
	}
	if err := updateProfileWithTx(ctx, f.ids[0], func(_ *gorm.DB, _ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips[0]--
		game.GGOField.Chips[1]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case index := <-received:
		t.Fatal("unchanged total notified", index)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestGGOChipTradeNoticeCoalescesBothPlayers(t *testing.T) {
	f := ggoTradeSetup(t)
	f.s.GGOIntegrationReady = true
	t.Cleanup(f.s.ggoStopNoticeTimers)
	received := make(chan int, 8)
	for i, id := range f.ids {
		index := i
		key := p10ClientKey{f.s.p10ClientOwner(), id}
		old, _ := p10ClientSinks.Load(key)
		sink := &p10ClientSink{session: f.sessions[i], send: func(message proto.Message) error {
			if _, ok := message.(*pb.SCFieldResourceInfo); ok {
				received <- index
			}
			return nil
		}}
		p10ClientSinks.Store(key, sink)
		t.Cleanup(func() { p10ClientSinks.CompareAndSwap(key, sink, old) })
	}
	change := func(tx *gorm.DB, pair *ggoTradePair) error {
		for _, actor := range pair.Actors {
			if err := tx.First(&actor.Presence, "player_id=?", actor.Row.Id).Error; err != nil {
				return err
			}
		}
		pair.Actors[0].Game.GGOField.Chips[0]--
		pair.Actors[1].Game.GGOField.Chips[0]++
		return ggoTradeSaveProfiles(tx, pair)
	}
	if err := f.s.ggoWithTradePair(f.ctx, f.auth(0), f.sessions[0], f.ids[0], f.ids[1], change); err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for len(seen) < 2 {
		select {
		case index := <-received:
			if seen[index] {
				t.Fatal("duplicate line broadcast", index)
			}
			seen[index] = true
		case <-time.After(3 * time.Second):
			t.Fatal("missing trade notification", seen)
		}
	}
	select {
	case index := <-received:
		t.Fatal("duplicate trade notification", index)
	case <-time.After(150 * time.Millisecond):
	}
	abort := errors.New("rollback trade")
	if err := f.s.ggoWithTradePair(f.ctx, f.auth(0), f.sessions[0], f.ids[0], f.ids[1], func(tx *gorm.DB, pair *ggoTradePair) error {
		if err := change(tx, pair); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal("trade rollback", err)
	}
	select {
	case index := <-received:
		t.Fatal("rolled back trade notified", index)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestGGOChipSnapshotAndActorCommitIntegration(t *testing.T) {
	f := ggoTradeSetup(t)
	f.s.GGOIntegrationReady = true
	t.Cleanup(f.s.ggoStopNoticeTimers)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}, &db.SharedFieldBoss55{}); err != nil {
		t.Fatal(err)
	}
	received := make(chan int, 8)
	for i, id := range f.ids {
		index := i
		key := p10ClientKey{f.s.p10ClientOwner(), id}
		old, _ := p10ClientSinks.Load(key)
		sink := &p10ClientSink{session: f.sessions[i], send: func(message proto.Message) error {
			if _, ok := message.(*pb.SCFieldResourceInfo); ok {
				received <- index
			}
			return nil
		}}
		p10ClientSinks.Store(key, sink)
		t.Cleanup(func() { p10ClientSinks.CompareAndSwap(key, sink, old) })
	}
	snapshot := func() *model.GGOFieldSnapshotResponse {
		t.Helper()
		result, err := f.s.GGOFieldSnapshot(f.ctx, f.auth(0), f.sessions[0])
		if err != nil {
			t.Fatal("snapshot", err)
		}
		return result
	}
	before := []model.GGOFieldChipCountView{{PlayerID: f.ids[0], Count: 10}, {PlayerID: f.ids[1], Count: 6}}
	if actual := snapshot().PlayerChipCounts; !reflect.DeepEqual(actual, before) {
		t.Fatal("native scene snapshot did not include both projections", actual)
	}
	if err := f.s.ggoWithActor(f.ctx, f.auth(1), f.sessions[1], func(tx *gorm.DB, actor *ggoFieldActor) error {
		actor.Game.GGOField.Chips[5] += 3
		return saveProfile(tx, actor.Row, actor.Player, actor.Game)
	}); err != nil {
		t.Fatal("committed actor mutation", err)
	}
	seen := map[int]bool{}
	for len(seen) < 2 {
		select {
		case index := <-received:
			if seen[index] {
				t.Fatal("duplicate actor hint", index)
			}
			seen[index] = true
		case <-time.After(3 * time.Second):
			t.Fatal("committed actor hint not delivered", seen)
		}
	}
	after := []model.GGOFieldChipCountView{{PlayerID: f.ids[0], Count: 10}, {PlayerID: f.ids[1], Count: 9}}
	if actual := snapshot().PlayerChipCounts; !reflect.DeepEqual(actual, after) {
		t.Fatal("snapshot did not expose committed total", actual)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	if actual := snapshot().PlayerChipCounts; !reflect.DeepEqual(actual, before[:1]) {
		t.Fatal("snapshot exposed another line", actual)
	}
}

func TestGGOChipPresenceProjectionAndScope(t *testing.T) {
	f := ggoTradeSetup(t)
	var own db.P10ScenePresence
	if err := db.GetIns().First(&own, "player_id=?", f.ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	read := func() []model.GGOFieldChipCountView {
		t.Helper()
		counts, err := f.s.ggoFieldChipCounts(db.GetIns(), &ggoFieldActor{Presence: own})
		if err != nil {
			t.Fatal(err)
		}
		return counts
	}
	expected := func(peer bool, count int64) []model.GGOFieldChipCountView {
		out := []model.GGOFieldChipCountView{{PlayerID: f.ids[0], Count: 10}}
		if peer {
			out = append(out, model.GGOFieldChipCountView{PlayerID: f.ids[1], Count: count})
		}
		return out
	}
	if got := read(); !reflect.DeepEqual(got, expected(true, 6)) {
		t.Fatal("initial committed totals", got)
	}
	if err := updateProfileWithTx(f.ctx, f.ids[1], func(_ *gorm.DB, _ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips[5] = 5
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got, expected(true, 11)) {
		t.Fatal("committed change", got)
	}
	abort := errors.New("rollback")
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.First(&profile, f.ids[1]).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		game.GGOField.Chips[0] = 100
		if err = saveProfile(tx, &profile, player, game); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) || !reflect.DeepEqual(read(), expected(true, 11)) {
		t.Fatal("rolled back projection", err, read())
	}
	if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got, expected(false, 0)) {
		t.Fatal("different line disclosed", got)
	}
	if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Updates(map[string]any{"line": 1, "session_hash": "invalid"}).Error; err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got, expected(false, 0)) {
		t.Fatal("stale session disclosed", got)
	}
	if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Updates(map[string]any{"session_hash": "", "offline_until": time.Now().UnixMilli() + 10000}).Error; err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got, expected(true, 11)) {
		t.Fatal("visible offline grace", got)
	}
	if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.ids[1]).Update("offline_until", time.Now().UnixMilli()-1).Error; err != nil {
		t.Fatal(err)
	}
	if got := read(); !reflect.DeepEqual(got, expected(false, 0)) {
		t.Fatal("expired offline player disclosed", got)
	}
	own.Frame = nil
	if got := read(); len(got) != 0 {
		t.Fatal("pending line received old roster", got)
	}
}

func TestGGOChipLegacyProjectionNeverOverwritesNewValue(t *testing.T) {
	f := ggoTradeSetup(t)
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.ids[0]).UpdateColumn("ggo_chip_total", nil).Error; err != nil {
		t.Fatal(err)
	}
	var stale db.Profile
	if err := db.GetIns().First(&stale, f.ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&stale)
	if err != nil {
		t.Fatal(err)
	}
	if err = ggoEnsureChipProjection(db.GetIns(), &stale, game); err != nil {
		t.Fatal(err)
	}
	if stale.GGOChipTotal == nil || *stale.GGOChipTotal != 10 {
		t.Fatal("legacy total", stale.GGOChipTotal)
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.ids[0]).UpdateColumn("ggo_chip_total", 77).Error; err != nil {
		t.Fatal(err)
	}
	stale.GGOChipTotal = nil
	if err = ggoEnsureChipProjection(db.GetIns(), &stale, game); err != nil {
		t.Fatal(err)
	}
	var current db.Profile
	if err = db.GetIns().First(&current, f.ids[0]).Error; err != nil || current.GGOChipTotal == nil || *current.GGOChipTotal != 77 {
		t.Fatal("conditional initialization overwrote newer projection", current.GGOChipTotal, err)
	}
	// A concurrent transition can also leave the projection NULL. Comparing
	// the saved game prevents an old scene sync from restoring its stale total.
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.ids[0]).Updates(map[string]any{
		"ggo_chip_total": nil, "game": *stale.Game + " ",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = ggoEnsureChipProjection(db.GetIns(), &stale, game); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&current, f.ids[0]).Error; err != nil || current.GGOChipTotal != nil {
		t.Fatal("stale game initialized a new scene projection", current.GGOChipTotal, err)
	}
}
