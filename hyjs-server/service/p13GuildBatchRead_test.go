package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

type p13BatchQueryCounter struct {
	logger.Interface
	count int
}

func (c *p13BatchQueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.count++
	c.Interface.Trace(ctx, begin, fc, err)
}

func p13BatchAssertSingle(t *testing.T, f *p1Fixture, ids []int64, now time.Time) {
	t.Helper()
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		got, err := f.s.p13GuildReadPlayers(tx, ids, now)
		if err != nil {
			return err
		}
		for _, id := range ids {
			var profile db.Profile
			if err := tx.First(&profile, id).Error; err != nil {
				return err
			}
			player, game, err := decodeProfile(&profile)
			if err != nil {
				return err
			}
			if err := f.s.PrepareP5GuildPrayerProperties(f.ctx, tx, player, game, now); err != nil {
				return err
			}
			power, err := f.s.formationFightPower(player, game)
			if err != nil {
				return err
			}
			actual, expected := got[id].Game.GuildPrayerProjection, game.GuildPrayerProjection
			if power != got[id].Power || !proto.Equal(actual.Properties, expected.Properties) || !proto.Equal(actual.TechProperties, expected.TechProperties) || actual.SourceHash != expected.SourceHash || actual.TechSourceHash != expected.TechSourceHash || actual.ExpiresAt != expected.ExpiresAt {
				t.Fatalf("batch differs from single source for %d", id)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestP13GuildBatchReadPrayerExpiryTermAndCorruption(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	p13PrayerPublish(t, now.Add(-time.Second))
	if _, err := f.s.p13GuildPrayer(f.ctx, ids[0], &pb.CSUnionPray{}, 100, now); err != nil {
		t.Fatal(err)
	}
	p13BatchAssertSingle(t, f, ids, now)
	if _, err := f.s.p13GuildReadPlayers(db.GetIns(), ids, now.Add(-time.Millisecond)); err == nil {
		t.Fatal("future prayer accepted")
	}
	p13BatchAssertSingle(t, f, ids, now.Add(24*time.Hour))
	if err := db.GetIns().Model(&db.P13GuildMember{}).Where("player_id=?", ids[0]).Update("joined_at", now.Add(time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	p13BatchAssertSingle(t, f, ids, now.Add(time.Hour))
	if err := db.GetIns().Model(&db.P13GuildPrayerRoll{}).Where("player_id=?", ids[0]).Update("effect_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildReadPlayers(db.GetIns(), ids, now.Add(time.Hour)); err == nil {
		t.Fatal("corrupt latest prayer accepted")
	}
	if _, _, _, err := f.s.P13GuildPrayerProperties(f.ctx, db.GetIns(), ids[0], now.Add(time.Hour)); err == nil {
		t.Fatal("single source accepted corruption")
	}
	if err := db.GetIns().Where("player_id=?", ids[0]).Delete(&db.P13GuildMember{}).Error; err != nil {
		t.Fatal(err)
	}
	p13BatchAssertSingle(t, f, ids, now.Add(time.Hour))
}

func TestP13GuildBatchReadTechFixedQueriesAndIndependentProjections(t *testing.T) {
	f, ids, now, _ := p13TechSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	p13TechPublish(t, now.Add(-time.Second))
	p13TechUpgrade(t, f, ids[0], "peratt", constUtil.P13GuildTechHeroStatue, 7000, now)
	p13BatchAssertSingle(t, f, ids, now)
	counter := &p13BatchQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	tx := db.GetIns().Session(&gorm.Session{Logger: counter})
	if _, err := f.s.p13GuildReadPlayers(tx, ids[:1], now); err != nil {
		t.Fatal(err)
	}
	one := counter.count
	counter.count = 0
	got, err := f.s.p13GuildReadPlayers(tx, append(ids, ids[0]), now)
	if err != nil {
		t.Fatal(err)
	}
	if one != counter.count {
		t.Fatalf("query count grew: one=%d many=%d", one, counter.count)
	}
	if len(got) != len(ids) {
		t.Fatal("duplicate IDs were not deduplicated")
	}
	got[ids[0]].Game.GuildPrayerProjection.TechProperties.Peratt = proto.Float64(999)
	if got[ids[1]].Game.GuildPrayerProjection.TechProperties.GetPeratt() == 999 {
		t.Fatal("members share mutable tech properties")
	}
	if _, err := f.s.p13GuildReadPlayers(tx, []int64{ids[0], -1}, now); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing profile: %v", err)
	}
	var step db.P13GuildTechUpgrade
	if err := db.GetIns().First(&step).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&step).Update("prop", "peratt,999").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildReadPlayers(tx, ids, now); err == nil {
		t.Fatal("corrupt tech accepted")
	}
}

func TestP13GuildBatchReadMissingOptionalSchema(t *testing.T) {
	f, ids := p13GuildSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Migrator().DropTable(&db.P13GuildPrayerRoll{}, &db.P13GuildTechOffer{}, &db.P13GuildTechUpgrade{}, &db.P13GuildMember{}, &db.P13Guild{}); err != nil {
		t.Fatal(err)
	}
	p13BatchAssertSingle(t, f, ids, time.Now())
	for _, table := range []any{&db.P13GuildPrayerRoll{}, &db.P13GuildMember{}, &db.P13Guild{}} {
		if db.GetIns().Migrator().HasTable(table) {
			t.Fatal("read created optional schema")
		}
	}
}
