package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFWorldClosedColdProfileExitAndDelayedOldScope(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "guard_hp_source": "", "map_projection": "", "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "explicit closed-world lifecycle fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	frozen, _ := json.Marshal(actor)
	checkFrozen := func() {
		t.Helper()
		var current db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&current, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(current)
		if string(raw) != string(frozen) || !bytes.Equal(current.Frame, actor.Frame) || !bytes.Equal(current.Battle, actor.Battle) {
			t.Fatal("closed field actor changed")
		}
	}
	// The world gate must fail closed even for a damaged abort proof. This row is
	// a diagnostic scalar fixture, never asserted as a valid settlement certificate.
	closed := db.P13UnionFightSettlementResult{MatchID: match.ID, ResultKind: constUtil.P13UFTechnicalAbortKind, EndAt: match.EndAt, SettledAt: match.EndAt + 5000, Awards: []byte("[]"), Source: "damaged-proof fail-closed diagnostic"}
	if err := db.GetIns().Create(&closed).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(match.EndAt + 5000)
	clocks, err := p13UFWorldClockBatch(f.ctx, []string{match.ID}, now())
	if err != nil || !clocks.closed[match.ID] || !clocks.paused(match.ID) {
		t.Fatal(clocks, err)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		closed, err := p13UFWorldClosed(tx, match.ID)
		if err != nil {
			return err
		}
		if !closed {
			t.Fatal("current-read gate reopened abort")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Original End+4s has elapsed. The lifecycle leaves only Profile/connection,
	// and does not cancel pending sources or mutate the frozen participant row.
	if err = f.s.TickP13UnionFightField(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	checkFrozen()
	var profile db.Profile
	if err = db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil || game.Position.GetScene() != "town_main" {
		t.Fatal("closed player left in old scene", game, err)
	}
	if _, left, err := f.s.p13UFFieldLeaveExpected(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_main")}, now(), match.ID); err != nil || left {
		t.Fatal("closed cleanup must be idempotent", left, err)
	}
	// Both rounds were created by the real matcher. Enter the later round through
	// the original API, then deliver delayed cleanup from the sealed old round.
	clock.Store(first + 600000)
	if _, err = f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	var activeMatch string
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		actor, current, _, _, err := f.s.p13UFFieldCurrent(tx, ids[0], now())
		if err != nil {
			return err
		}
		activeMatch = current.ID
		if actor.MatchID == match.ID {
			t.Fatal("Current selected sealed historical actor")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, left, err := f.s.p13UFFieldLeaveExpected(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_main")}, now(), match.ID); err != nil || left {
		t.Fatal("old cleanup retired new match", activeMatch, left, err)
	}
	if cleaned, err := f.s.p13UFFieldCleanupClosedProfile(f.ctx, ids[0], profile.Session); err != nil || cleaned {
		t.Fatal("reconnect cleanup retired a live new participant", cleaned, err)
	}
	checkFrozen()
	if err = db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err = decodeProfile(&profile)
	if err != nil || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		t.Fatal("new profile scene lost", game, err)
	}
}
