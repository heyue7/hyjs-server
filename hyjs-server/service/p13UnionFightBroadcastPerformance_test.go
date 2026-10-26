package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13BroadcastRealRecipientsBatchOrderAndRevocation(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "guard_hp_source": "", "map_projection": "", "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated broadcast real field fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	var roster p13UFMatchRoster
	if err := json.Unmarshal(match.Roster, &roster); err != nil {
		t.Fatal(err)
	}
	other := roster.Right[0].PlayerID
	if other == ids[0] {
		other = roster.Left[0].PlayerID
	}
	recorders := map[int64]*p10SceneRecorder{}
	for _, id := range []int64{ids[0], other} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		r := &p10SceneRecorder{}
		recorders[id] = r
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, profile.Session, r.send))
	}
	for _, r := range recorders {
		r.drain()
	}
	trace := &p13CombatProjectionTrace{Interface: logger.Default.LogMode(logger.Silent)}
	rows, err := f.s.p13UFFieldBroadcastRecipients(db.GetIns().WithContext(f.ctx).Session(&gorm.Session{Logger: trace}), match.ID, ids[0])
	if err != nil || len(rows) != 1 || rows[0].PlayerID != other {
		t.Fatal("real opponent recipient", rows, err)
	}
	if len(trace.SQL) != 2 {
		t.Fatal("recipient reads must be snapshot plus one joined directory", len(trace.SQL))
	}
	for _, sql := range trace.SQL {
		lower := strings.ToLower(sql)
		if strings.Contains(lower, "battle_attempt") || strings.Contains(lower, "a.*") || strings.Contains(lower, "a.frame") || strings.Contains(lower, "a.battle") || strings.Count(lower, " join ") > 3 {
			t.Fatal("broadcast rebuilt combat PB", sql)
		}
	}
	// Transport-only Begin frames test ordering; no attempt/result is fabricated.
	batch := []proto.Message{&pb.SCUFPlayerFightBegin{Guid: proto.Int64(ids[0]), TargetGuid: proto.Int64(other)}, &pb.SCUFPlayerFightBegin{Guid: proto.Int64(other), TargetGuid: proto.Int64(ids[0])}}
	f.s.p13UFFieldBroadcastBatch(f.ctx, match.ID, ids[0], batch)
	got := recorders[other].drain()
	if len(got) != 2 || !proto.Equal(got[0], batch[0]) || !proto.Equal(got[1], batch[1]) || len(recorders[ids[0]].drain()) != 0 {
		t.Fatal("batch order/self exclusion", got)
	}
	var member db.P13GuildMember
	if err := db.GetIns().First(&member, "player_id=?", other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&member).Update("guild_id", "revoked-broadcast-fixture").Error; err != nil {
		t.Fatal(err)
	}
	f.s.p13UFFieldBroadcastBatch(f.ctx, match.ID, ids[0], batch)
	if len(recorders[other].drain()) != 0 {
		t.Fatal("revoked member received batch")
	}
	// Restore explicitly: Gorm Update may mutate the supplied model as well.
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&member).Update("guild_id", actor.GuildID).Error; err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, other).Error; err != nil {
		t.Fatal(err)
	}
	if len(profile.Session) == 0 {
		t.Fatal("actual account session missing")
	}
	originalSession := append([]byte(nil), profile.Session...)
	changedSession := append([]byte(nil), originalSession...)
	changedSession[0] ^= 0x80
	if err := db.GetIns().Model(&profile).Update("session", changedSession).Error; err != nil {
		t.Fatal(err)
	}
	f.s.p13UFFieldBroadcastBatch(f.ctx, match.ID, ids[0], batch)
	if len(recorders[other].drain()) != 0 {
		t.Fatal("rotated old socket received batch")
	}
	if err := db.GetIns().Model(&profile).Update("session", originalSession).Error; err != nil {
		t.Fatal(err)
	}
	// A replacement socket cannot receive a batch pinned to the prior directory.
	key := p10ClientKey{f.s.p10ClientOwner(), other}
	old, ok := p10ClientSinks.Load(key)
	if !ok {
		t.Fatal("socket disappeared")
	}
	replacement := &p10ClientSink{session: originalSession, send: recorders[other].send}
	p10ClientSinks.Store(key, replacement)
	f.s.publishP10PrivateBatchPinned(f.ctx, other, old.(*p10ClientSink), batch)
	if len(recorders[other].drain()) != 0 {
		t.Fatal("replacement inherited old pinned batch")
	}
	p10ClientSinks.Store(key, old)
	// Real next-round admission removes the old scope's active directory entry.
	clock.Store(first + 600000)
	if _, err := f.s.p13UFFieldEnter(f.ctx, other, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	f.s.p13UFFieldBroadcastBatch(f.ctx, match.ID, ids[0], batch)
	if len(recorders[other].drain()) != 0 {
		t.Fatal("new-round socket received old scope")
	}
}

func TestP13BroadcastPinnedSendFailureRetiresOnlyExpectedSink(t *testing.T) {
	f, ids := p13GuildSetup(t)
	id := ids[0]
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	sink := &p10ClientSink{session: profile.Session, send: func(proto.Message) error { calls++; return errors.New("owned send-failure fixture") }}
	key := p10ClientKey{f.s.p10ClientOwner(), id}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	f.s.publishP10PrivateBatchPinned(f.ctx, id, sink, []proto.Message{&pb.SCUFPlayerFightBegin{}, &pb.SCUFPlayerFightBegin{}})
	if calls != 1 {
		t.Fatal("failed batch continued", calls)
	}
	if current, ok := p10ClientSinks.Load(key); ok && current == sink {
		t.Fatal("failed pinned sink retained")
	}
}
