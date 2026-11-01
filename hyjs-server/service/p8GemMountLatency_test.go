package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type gemMountSQLTrace struct {
	logger.Interface
	queries int
	members int
}

func TestGemMountUsesCurrentGuildPrayerAndExpiry(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitGems(f.ctx); err != nil {
		t.Fatal(err)
	}
	at := now
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return at })
	p13PrayerPublish(t, now.Add(-time.Second))
	if _, err := f.s.p13GuildPrayer(ctx, ids[0], &pb.CSUnionPray{}, 33301, now); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, ids[0], func(p *pb.Player, g *model.GameState) error {
		g.Equipment = append(g.Equipment, &pb.Equipment{Guid: proto.String("guild-gem-equip"), EquipId: proto.String("equip_037"), Level: proto.Int32(1)})
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(1), Id: proto.String("equip_037"), Guid: proto.String("guild-gem-equip")})
		g.Gems = append(g.Gems, &pb.SimpleItem{Id: proto.String("MineItem_019"), Num: proto.Int32(1)}, &pb.SimpleItem{Id: proto.String("MineItem_020"), Num: proto.Int32(1)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for index, key := range []string{"MineItem_019", "MineItem_020"} {
		if index == 1 {
			at = now.Add(24*time.Hour + time.Second)
		}
		request := &pb.CSGemMountOrChange{Guid: proto.String("guild-gem-equip"), GemId: proto.String(key)}
		requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, request)
		if _, err := f.s.gemMountOrChange(requestCtx, ids[0], request, int64(33302+index)); err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row db.Profile
			if err := tx.First(&row, ids[0]).Error; err != nil {
				return err
			}
			player, game, err := decodeProfile(&row)
			if err != nil {
				return err
			}
			if err = f.s.PrepareP5GuildPrayerProperties(ctx, tx, player, game, at); err != nil {
				return err
			}
			if active := game.GuildPrayerProjection.SourceHash != ""; active != (index == 0) {
				t.Fatal("prayer source did not follow the actual expiration", active)
			}
			power, err := f.s.formationFightPower(player, game)
			if err != nil || player.GetFightPower() != power {
				t.Fatalf("saved power after mount=%d current=%d err=%v", player.GetFightPower(), power, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func (trace *gemMountSQLTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	trace.queries++
	if strings.Contains(strings.ToLower(query), "p13_guild_members") {
		trace.members++
	}
}

// 原客户端拖放使用自动镶嵌，验证连换与最终战力并统计真实公会源读取。
func TestGemMountLatencyAndPower(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitGems(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P13GuildMember{}, &db.P13Guild{}, &db.P13GuildPrayerRoll{}, &db.P13GuildTechOffer{}, &db.P13GuildTechUpgrade{}); err != nil {
		t.Fatal(err)
	}
	f.s.guildCoreSchemaReady.Store(true)
	f.s.guildPrayerSchemaReady.Store(true)
	f.s.guildTechSchemaReady.Store(true)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(60)
		g.Equipment = []*pb.Equipment{{Guid: proto.String("gem-latency-equip"), EquipId: proto.String("equip_037"), Level: proto.Int32(1)}}
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(1), Id: proto.String("equip_037"), Guid: proto.String("gem-latency-equip")})
		g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_019"), Num: proto.Int32(1)}, {Id: proto.String("MineItem_020"), Num: proto.Int32(1)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	previous := db.GetIns().Config.Logger
	trace := &gemMountSQLTrace{Interface: previous}
	db.GetIns().Config.Logger = trace
	started := time.Now()
	for i := 0; i < 12; i++ {
		key := []string{"MineItem_019", "MineItem_020"}[i%2]
		request := &pb.CSGemMountOrChange{Guid: proto.String("gem-latency-equip"), GemId: proto.String(key)}
		ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
		if _, err := f.s.gemMountOrChange(ctx, f.id, request, int64(2000+i)); err != nil {
			db.GetIns().Config.Logger = previous
			t.Fatal(err)
		}
	}
	elapsed := time.Since(started)
	db.GetIns().Config.Logger = previous
	player, game := f.profile(t)
	actualPower, err := f.s.formationFightPower(player, game)
	if err != nil || player.GetFightPower() != actualPower || game.Equipment[0].GetGems()[0] != "MineItem_020" || gemCount(game, "MineItem_019") != 1 || gemCount(game, "MineItem_020") != 0 {
		t.Fatalf("gem or final power changed: power=%d/%d error=%v", player.GetFightPower(), actualPower, err)
	}
	if trace.members != 24 {
		t.Fatalf("12 mounts performed %d guild member reads, want one prayer and one tech read each", trace.members)
	}
	t.Logf("12 mounts: queries=%d guild-member queries=%d elapsed=%s", trace.queries, trace.members, elapsed)
}
