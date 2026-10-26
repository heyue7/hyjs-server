package service

import (
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"reflect"
	"testing"
	"time"
)

func TestP13GuildMapEntryReadOnlyFrozenAndMissingField(t *testing.T) {
	for _, projection := range []string{"", constUtil.P13UnionFightMapProjectionReFlat} {
		t.Run("projection="+projection, func(t *testing.T) {
			f, ids, _, clock, match := p13UFFieldSetup(t)
			tx := db.GetIns()
			if err := tx.Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "map_projection": projection, "source": "isolated resource probe fixture"}).Error; err != nil {
				t.Fatal(err)
			}
			var before db.Profile
			if err := tx.First(&before, ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			out, err := f.s.P13GuildMapEntry(f.ctx, ids[0], before.Session)
			if err != nil || out.MapProjection != projection {
				t.Fatal("eligible pre-entry probe", out, err)
			}
			for _, table := range []any{&db.P13UnionFightField{}, &db.P13UnionFightFieldPlayer{}} {
				var n int64
				if err = tx.Model(table).Count(&n).Error; err != nil || n != 0 {
					t.Fatal("probe wrote field domain", n, err)
				}
			}
			var after db.Profile
			if err = tx.First(&after, ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Game, after.Game) || !reflect.DeepEqual(before.Player, after.Player) {
				t.Fatal("probe changed profile")
			}
			wrong := append([]byte(nil), before.Session...)
			wrong[0] ^= 255
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], wrong); err == nil {
				t.Fatal("bad session accepted")
			}
			if err = tx.Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("map_projection", "unknown").Error; err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], before.Session); err == nil {
				t.Fatal("unknown projection accepted")
			}
			if err = tx.Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("map_projection", projection).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.p13UFFieldEnter(f.ctx, ids[0], time.UnixMilli(clock.Load())); err != nil {
				t.Fatal(err)
			}
			changed := constUtil.P13UnionFightMapProjectionReFlat
			if projection != "" {
				changed = ""
			}
			if err = tx.Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("map_projection", changed).Error; err != nil {
				t.Fatal(err)
			}
			out, err = f.s.P13GuildMapEntry(f.ctx, ids[0], before.Session)
			if err != nil || out.MapProjection != projection {
				t.Fatal("live policy replaced frozen map", out, err)
			}
			rotated, err := LoginAccount(f.ctx, before.Uid, "isolated-local-pass")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], before.Session); err == nil {
				t.Fatal("rotated session accepted")
			}
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], rotated.Session); err != nil {
				t.Fatal(err)
			}
			if err = tx.Where("match_id=?", match.ID).Delete(&db.P13UnionFightField{}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], rotated.Session); err == nil {
				t.Fatal("missing frozen field rebuilt around actor")
			}
			var n int64
			if err = tx.Model(&db.P13UnionFightField{}).Count(&n).Error; err != nil || n != 0 {
				t.Fatal("probe rebuilt missing field", n, err)
			}
			if err = tx.Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], rotated.Session); err == nil {
				t.Fatal("disabled field accepted")
			}
			if err = tx.Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			var finalEnd int64
			if err = tx.Model(&db.P13UnionFightMatch{}).Select("MAX(end_at)").Where("day=? AND (left_guild_id=? OR right_guild_id=?)", match.Day, match.LeftGuildID, match.LeftGuildID).Scan(&finalEnd).Error; err != nil {
				t.Fatal(err)
			}
			clock.Store(finalEnd + 1)
			if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], rotated.Session); err == nil {
				t.Fatal("no eligible match accepted")
			}

		})
	}
}
