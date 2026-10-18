package service

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13FieldQueuedOldRoundKickCannotLeaveActualNextRound(t *testing.T) {
	f, ids, first, clock, oldMatch := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated actual two-round stale lifecycle kick"}).Error; err != nil {
		t.Fatal(err)
	}
	id := ids[0]
	if _, err := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(first-60000)); err != nil {
		t.Fatal(err)
	}
	// Capture the actual old actor exactly as the background Find does. The
	// original second-round enter then closes that actor under the normal locks.
	var queued db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&queued, "match_id=? AND player_id=? AND left_at=0", oldMatch.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	var next db.P13UnionFightMatch
	if err := db.GetIns().First(&next, "round=2 AND (left_guild_id=? OR right_guild_id=?)", queued.GuildID, queued.GuildID).Error; err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(next.StartAt - 60000)
	clock.Store(now.UnixMilli())
	if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
		t.Fatal("real second-round enter", err)
	}
	var before db.Profile
	var actor db.P13UnionFightFieldPlayer
	db.GetIns().First(&before, id)
	db.GetIns().First(&actor, "match_id=? AND player_id=?", next.ID, id)
	workers := 1
	if f.mysql.Database != "" {
		workers = 8
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frames, left, err := f.s.p13UFFieldLeaveExpected(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now, queued.MatchID)
			if err == nil && (left || len(frames) != 0) {
				t.Error("queued old kick produced current-round transfer/kick")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var after db.Profile
	var live db.P13UnionFightFieldPlayer
	db.GetIns().First(&after, id)
	db.GetIns().First(&live, "match_id=? AND player_id=?", next.ID, id)
	if before.Player == nil || after.Player == nil || before.Game == nil || after.Game == nil || *before.Player != *after.Player || *before.Game != *after.Game || live.LeftAt != 0 || live.FrameHash != actor.FrameHash || live.UpdatedAt != actor.UpdatedAt {
		t.Fatal("stale lifecycle request touched actual new-round state")
	}
	frames, left, err := f.s.p13UFFieldLeaveExpected(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now, next.ID)
	if err != nil || !left || len(frames) == 0 {
		t.Fatal("real current-round scheduled leave stopped working", err)
	}
	frames, left, err = f.s.p13UFFieldLeaveExpected(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now, next.ID)
	if err != nil || left || len(frames) != 0 {
		t.Fatal("same expired worker emitted a second leave", err)
	}
}
