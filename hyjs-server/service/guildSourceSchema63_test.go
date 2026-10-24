package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

type guildSourceSQL63 struct {
	logger.Interface
	metadata, members, rolls int
}

func (l *guildSourceSQL63) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	sql, _ := statement()
	sql = strings.ToLower(sql)
	if strings.Contains(sql, "sqlite_master") || strings.Contains(sql, "information_schema") {
		l.metadata++
	}
	if strings.Contains(sql, "p13_guild_members") {
		l.members++
	}
	if strings.Contains(sql, "p13_guild_prayer_rolls") {
		l.rolls++
	}
}

// Count actual emitted SQL, including the metadata probes hidden behind HasTable.
// Runtime table overlays must resolve readiness through their original service owner.
func TestGuildSourceSchema63InitializedAndOptionalFallback(t *testing.T) {
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP13GuildPrayer(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13GuildTech(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !f.s.guildCoreSchemaReady.Load() || !f.s.guildPrayerSchemaReady.Load() || !f.s.guildTechSchemaReady.Load() {
		t.Fatal("successful complete migrations did not publish readiness")
	}
	runtime := f.s.withRuntimeTables(f.s.Tables)
	counts := &guildSourceSQL63{Interface: logger.Default.LogMode(logger.Silent)}
	read := db.GetIns().Session(&gorm.Session{Logger: counts})
	player := &pb.Player{PlayerId: proto.Int64(ids[0])}
	for i := 0; i < 3; i++ {
		game := &model.GameState{}
		if err := runtime.RefreshP5GuildPrayerPower(f.ctx, read, player, game, time.Now()); err != nil {
			t.Fatal(err)
		}
		if game.GuildPrayerProjection == nil || game.GuildPrayerProjection.SourceHash != "" || game.GuildPrayerProjection.TechSourceHash != "" {
			t.Fatal("inactive source projection changed")
		}
	}
	if counts.metadata != 0 {
		t.Fatalf("initialized source performed %d metadata probes", counts.metadata)
	}
	if counts.members != 6 || counts.rolls != 3 {
		t.Fatalf("live source queries were skipped: members=%d rolls=%d", counts.members, counts.rolls)
	}
	// A separately constructed optional deployment cannot inherit readiness merely
	// because another GameService initialized this database earlier.
	if err := db.GetIns().Migrator().DropTable(&db.P13GuildPrayerRoll{}, &db.P13GuildTechOffer{}); err != nil {
		t.Fatal(err)
	}
	for _, partial := range []bool{false, true} {
		optional := &GameService{Tables: f.s.Tables}
		if partial {
			optional.guildPrayerSchemaReady.Store(true)
			optional.guildTechSchemaReady.Store(true)
		}
		before := counts.metadata
		if err := optional.RefreshP5GuildPrayerPower(context.Background(), read, player, &model.GameState{}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if counts.metadata <= before {
			t.Fatal("optional or core-incomplete source bypassed compatibility metadata checks")
		}
	}
	if db.GetIns().Migrator().HasTable(&db.P13GuildPrayerRoll{}) || db.GetIns().Migrator().HasTable(&db.P13GuildTechOffer{}) {
		t.Fatal("optional source read created missing schema")
	}
}
