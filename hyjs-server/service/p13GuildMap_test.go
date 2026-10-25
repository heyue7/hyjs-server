package service

import (
	"bytes"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestP13GuildMapAuthenticatedLoadingFrozenProjectionAndSessionRotation(t *testing.T) {
	for _, projection := range []string{"", constUtil.P13UnionFightMapProjectionReFlat} {
		name := projection
		if name == "" {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			f, ids, _, clock, match := p13UFFieldSetup(t)
			now := func() time.Time { return time.UnixMilli(clock.Load()) }
			if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "map_projection": projection, "source": "isolated authenticated map loading test"}).Error; err != nil {
				t.Fatal(err)
			}
			id := ids[0]
			var profile db.Profile
			if err := db.GetIns().First(&profile, id).Error; err != nil {
				t.Fatal(err)
			}
			session := append([]byte(nil), profile.Session...)
			if _, err := AuthorizeAccount(f.ctx, id, session); err != nil {
				t.Fatal("existing account fixture lacks real authorization; do not bypass it", err)
			}
			// A valid account/participant has not entered a field yet; this HTTP
			// helper must not create an actor or bypass the original entry flow.
			if _, err := f.s.P13GuildMap(f.ctx, id, session); err == nil {
				t.Fatal("account without field entry received a map")
			}
			var count int64
			if err := db.GetIns().Model(&db.P13UnionFightFieldPlayer{}).Where("player_id=?", id).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("map query created field entry", count, err)
			}
			if _, err := f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().First(&profile, id).Error; err != nil {
				t.Fatal(err)
			}
			_, game, err := decodeProfile(&profile)
			if err != nil || !game.PositionLoading {
				t.Fatal("fixture completed loading before map request", err)
			}
			var field db.P13UnionFightField
			if err := db.GetIns().First(&field, "match_id=?", match.ID).Error; err != nil {
				t.Fatal(err)
			}
			frozenSHA := field.SnapshotHash
			out, err := f.s.P13GuildMap(f.ctx, id, session)
			if err != nil || out == nil || out.MapProjection != projection {
				t.Fatal("authenticated loading map", projection, out, err)
			}
			wrong := append([]byte(nil), session...)
			wrong[0] ^= 0xff
			if _, err := f.s.P13GuildMap(f.ctx, id, wrong); err == nil {
				t.Fatal("wrong original session accepted")
			}
			// The response belongs to the accepted field, so an operator edit
			// during client loading cannot switch this map's resource adapter.
			changed := constUtil.P13UnionFightMapProjectionReFlat
			if projection != "" {
				changed = ""
			}
			if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("map_projection", changed).Error; err != nil {
				t.Fatal(err)
			}
			out, err = f.s.P13GuildMap(f.ctx, id, session)
			if err != nil || out.MapProjection != projection {
				t.Fatal("live policy replaced frozen projection", err)
			}
			if err := db.GetIns().First(&field, "match_id=?", match.ID).Error; err != nil || field.SnapshotHash != frozenSHA {
				t.Fatal("map query rewrote frozen field", err)
			}
			// Login uses the fixture's actual stored account/credential and rotates
			// the original session; no direct session column rewrite simulates it.
			rotated, err := LoginAccount(f.ctx, profile.Uid, "isolated-local-pass")
			if err != nil || bytes.Equal(rotated.Session, session) {
				t.Fatal("actual account session rotation", err)
			}
			if _, err := f.s.P13GuildMap(f.ctx, id, session); err == nil {
				t.Fatal("rotated old session retained map access")
			}
			out, err = f.s.P13GuildMap(f.ctx, id, rotated.Session)
			if err != nil || out.MapProjection != projection {
				t.Fatal("new original session lost accepted field", err)
			}
			if err := db.GetIns().First(&profile, id).Error; err != nil {
				t.Fatal(err)
			}
			_, game, err = decodeProfile(&profile)
			if err != nil || !game.PositionLoading {
				t.Fatal("map endpoint completed original loading handshake", err)
			}
			// Corrupt only the actor's claimed projection, leaving field bytes
			// and hashes intact. The endpoint must reject this inconsistency.
			if err := db.GetIns().Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND player_id=?", match.ID, id).Update("map_projection", changed).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.P13GuildMap(f.ctx, id, rotated.Session); err == nil {
				t.Fatal("actor/snapshot projection mismatch returned map")
			}
			// Restore the valid actor first so absence of its accepted frozen
			// field is the sole rejection cause. A read must never rebuild it
			// from the changed live policy, even while the client is loading.
			if err := db.GetIns().Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND player_id=?", match.ID, id).Update("map_projection", projection).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Where("match_id=?", match.ID).Delete(&db.P13UnionFightField{}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.P13GuildMap(f.ctx, id, rotated.Session); err == nil {
				t.Fatal("missing accepted frozen field returned a map")
			}
			var rebuilt int64
			if err := db.GetIns().Model(&db.P13UnionFightField{}).Where("match_id=?", match.ID).Count(&rebuilt).Error; err != nil || rebuilt != 0 {
				t.Fatal("map query rebuilt a missing frozen field", rebuilt, err)
			}
			var retained int64
			if err := db.GetIns().Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND player_id=? AND left_at=0", match.ID, id).Count(&retained).Error; err != nil || retained != 1 {
				t.Fatal("rejected map query changed accepted actor", retained, err)
			}
		})
	}
}
