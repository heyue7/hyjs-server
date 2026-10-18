package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Boundaries come from server slots only. The five-minute closed interval
// cannot be bypassed by falling through to the following game.
func TestP14PerGameServerInfersOwnSlotAndFiveMinuteLock(t *testing.T) {
	manifest := &p14FinalSeriesManifest{Slots: []int64{1000000, 4600000, 8200000, 11800000, 15400000}}
	for _, test := range []struct {
		at     int64
		want   int32
		closed bool
	}{{999999, 0, false}, {1000000, 2, false}, {4299999, 2, false}, {4300000, 0, true}, {4599999, 0, true}, {4600000, 3, false}, {15399999, 0, true}, {15400000, 0, true}} {
		got, err := p14PerGameCurrentSlot(manifest, test.at)
		if test.closed {
			if err == nil {
				t.Fatal("locked interval selected future slot", test.at)
			}
		} else if err != nil || got != test.want {
			t.Fatal(test.at, got, err)
		}
	}
}

func p14PerGameFixture(t *testing.T, server int64) (*p1Fixture, *db.P14FinalSeries, *p14FinalSeriesManifest, time.Time) {
	t.Helper()
	f, _, _, _ := p14Setup(t)
	f.s.ServerID = server
	if err := f.s.InitializeP14PerGame(f.ctx); err != nil {
		t.Fatal(err)
	}
	game, offer, policy := p14SeriesActualGame(t)
	series, slots, err := p14FinalSeriesPlan(game, offer, policy, time.UnixMilli(game.CreatedAt))
	if err != nil {
		t.Fatal(err)
	}
	tx := db.GetIns()
	if err = tx.Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"season_index": game.SeasonIndex, "enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{game, offer, series, &slots} {
		if err = tx.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if server == 2 {
		if err = tx.Model(&db.P14TrustedPeer{}).Where("server_id=2").Update("server_id", 1).Error; err != nil {
			t.Fatal(err)
		}
	}
	var manifest p14FinalSeriesManifest
	if json.Unmarshal(series.Manifest, &manifest) != nil {
		t.Fatal("manifest decode")
	}
	return f, series, &manifest, time.UnixMilli(manifest.Slots[0] + 1)
}
func p14PerGameSwitch(t *testing.T, f *p1Fixture) {
	t.Helper()
	if sql, err := db.GetIns().DB(); err == nil {
		sql.Close()
	}
	f.open(t)
}
func p14PerGameCapture(t *testing.T, f *p1Fixture, at time.Time) error {
	t.Helper()
	return db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var season db.P14SeasonPolicy
		if err := tx.First(&season, 1).Error; err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.First(&profile, f.id).Error; err != nil {
			return err
		}
		player, state, err := decodeProfileForUpdate(&profile)
		if err != nil {
			return err
		}
		if err = f.s.RefreshP5GuildPrayerPower(f.ctx, tx, player, state, p13UFNow(f.ctx)); err != nil {
			return err
		}
		_, err = f.s.P14FinalFormationUploadCurrent(f.ctx, tx, &season, player, state, at)
		return err
	})
}

// Two actual fixture profiles independently capture new game2 inputs. The
// saved157 source is only the explicit historical series prerequisite, never
// reused as either new upload. No new qualification or played series is claimed.
func TestP14PerGameTwoActualOriginsLateSealColdNoGameOneReuse(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("two-file domain fixture; actual two-schema HTTP has a separate managed runner")
	}
	a, series, manifest, at := p14PerGameFixture(t, 1)
	if err := p14PerGameCapture(t, a, at); err == nil {
		t.Fatal("default-off accepted new game upload")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	b, other, _, _ := p14PerGameFixture(t, 2)
	if other.ID != series.ID {
		t.Fatal("independent realms changed series")
	}
	type origin struct {
		f        *p1Fixture
		payloads [][]byte
	}
	origins := []origin{{f: a}, {f: b}}
	for index := range origins {
		item := &origins[index]
		p14PerGameSwitch(t, item.f)
		if err := db.GetIns().Model(&db.P14PerGamePolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "prepublished-per-game-domain-v1", "published_at": manifest.FormationOffer.StageStart - 1}).Error; err != nil {
			t.Fatal(err)
		}
		if err := p14PerGameCapture(t, item.f, at); err != nil {
			t.Fatal(err)
		}
		if err := p14PerGameCapture(t, item.f, at.Add(time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if err := p14PerGameCapture(t, item.f, time.UnixMilli(manifest.Slots[1]-300000)); err == nil {
			t.Fatal("own lock accepted upload")
		}
		if err := item.f.s.QueueP14PerGameSeals(item.f.ctx, time.UnixMilli(manifest.Slots[1]-300000)); err != nil {
			t.Fatal(err)
		}
		if err := p14PerGameCapture(t, item.f, at.Add(2*time.Millisecond)); err == nil {
			t.Fatal("source already sealed accepted clock rollback capture")
		}
		var queue []db.P14PerGameDelivery
		if err := db.GetIns().Order("created_at,id").Find(&queue).Error; err != nil {
			t.Fatal(err)
		}
		if len(queue) != 3 {
			t.Fatal("expected two fresh revisions and one real seal", len(queue))
		}
		for _, row := range queue {
			item.payloads = append(item.payloads, append([]byte{}, row.Payload...))
		}
		var row db.P14PerGameFormation
		if err := db.GetIns().First(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.Revision != 2 || row.GameIndex != 2 {
			t.Fatal("wrong actual source revision")
		}
		player := &pb.BattlePlayer{}
		if proto.Unmarshal(row.Battle, player) != nil || player.GetPlayerId() != item.f.id || int64(player.GetServerId()) != item.f.s.OriginalServerID() {
			t.Fatal("fresh profile joint source lost")
		}
		var first db.P14FinalGame
		if err := db.GetIns().First(&first).Error; err != nil {
			t.Fatal(err)
		}
		if first.ID != series.GameOneID {
			t.Fatal("first game changed")
		}
		if _, _, err := item.f.s.p14PerGameReadySourceTX(item.f.ctx, db.GetIns(), series.ID, 2, time.UnixMilli(manifest.Slots[1])); err == nil {
			t.Fatal("missing peer seal reused Game1")
		}
	}
	for source := range origins {
		destination := 1 - source
		target := origins[destination].f
		p14PerGameSwitch(t, target)
		ctx := context.WithValue(target.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return time.UnixMilli(manifest.Slots[1] + 1) })
		// Deliver the origin's genuinely captured revisions after its lock and in
		// original outbox order, followed by the immutable complete origin seal.
		for _, raw := range origins[source].payloads {
			var envelope struct{ Request model.P14PerGameImportRequest }
			if json.Unmarshal(raw, &envelope.Request) != nil {
				t.Fatal("envelope decode")
			}
			key := []byte("isolated-peer-key-isolated-peer-key-")
			if _, err := target.s.ImportP14PerGame(ctx, envelope.Request, raw, P14SignPayload(key, raw)); err != nil {
				t.Fatal(err)
			}
			if _, err := target.s.ImportP14PerGame(ctx, envelope.Request, raw, P14SignPayload(key, raw)); err != nil {
				t.Fatal("cold exact retry", err)
			}
		}
		players, hashes, err := target.s.p14PerGameReadySourceTX(ctx, db.GetIns(), series.ID, 2, time.UnixMilli(manifest.Slots[1]+1))
		if err != nil || len(players) != 2 || len(hashes) != 2 || players[0].GetServerId() != int32(manifest.Pair.LeftServerID) || players[1].GetServerId() != int32(manifest.Pair.RightServerID) {
			t.Fatal("complete new ordered joint inputs not ready", err)
		}
		var season db.P14SeasonPolicy
		if err := db.GetIns().First(&season, 1).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := target.s.P14FinalFormationQueryCurrent(ctx, db.GetIns(), &season, target.s.OriginalServerID(), target.id, time.UnixMilli(manifest.Slots[1]+1)); err != nil {
			t.Fatal("next upload window lost last actual game2 display", err)
		}
		target.restart(t)
		var old db.P14FinalSeries
		if err := db.GetIns().First(&old).Error; err != nil || old.LeftWins != 0 || old.RightWins != 0 || old.CurrentGame != 1 {
			t.Fatal("source readiness counted as played next game", err)
		}
		var next db.P14FinalSeriesSlot
		if err := db.GetIns().First(&next, "series_id=? AND game_index=2", series.ID).Error; err != nil || len(next.Input) != 0 || len(next.Certificate) != 0 {
			t.Fatal("uncertified source created game/result", err)
		}
	}
}
