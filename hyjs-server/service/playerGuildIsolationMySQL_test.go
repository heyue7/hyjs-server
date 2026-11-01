package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestPlayerConcurrencyMySQLGuildLoginIsolation(t *testing.T) {
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p13_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p13_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated guild and accounts fixture")
	}

	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, true)
	now := time.Now()
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("登录隔离甲")}, 1, now)
	guildA := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guildA, 2, now)
	p13Join(t, f, ids[0], ids[4], guildA, 3, now)
	p13GuildCall(t, f, ids[2], &pb.CSUnionCreate{Name: proto.String("登录隔离乙")}, 4, now)
	guildB := p13GuildID(t, f, ids[2], now)
	p13Join(t, f, ids[2], ids[3], guildB, 5, now)
	if guildA == "" || guildB == "" || guildA == guildB {
		t.Fatal("guild setup did not create two distinct guilds")
	}

	database := db.GetIns()
	readGuild := func(id string) db.P13Guild {
		t.Helper()
		var guild db.P13Guild
		if err := database.First(&guild, "id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		return guild
	}
	beforeA, beforeB := readGuild(guildA), readGuild(guildB)
	tables, err := AdminGuildRuntimeTables(f.ctx, f.s.Tables)
	if err != nil {
		t.Fatal(err)
	}
	runtime := f.s.withRuntimeTables(tables)
	score, err := runtime.p10Setting("UnionLoginScore")
	if err != nil || score <= 0 {
		t.Fatalf("UnionLoginScore = %d: %v", score, err)
	}
	active, err := runtime.p10Setting("UnionLoginActiveScore")
	if err != nil || active <= 0 {
		t.Fatalf("UnionLoginActiveScore = %d: %v", active, err)
	}

	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	hold := database.WithContext(ctx).Begin()
	if hold.Error != nil {
		cancel()
		t.Fatal(hold.Error)
	}
	var otherPending, firstPending, secondPending <-chan error
	defer func() {
		_ = hold.Rollback().Error
		cancel()
		for _, pending := range []<-chan error{otherPending, firstPending, secondPending} {
			if pending != nil {
				<-pending
			}
		}
	}()
	var locked db.P13Guild
	if err := hold.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", guildA).Error; err != nil {
		t.Fatal(err)
	}

	otherDone := make(chan error, 1)
	otherPending = otherDone
	go func() { otherDone <- f.s.p13GuildLoginAt(ctx, ids[3], now) }()
	select {
	case err := <-otherPending:
		otherPending = nil
		if err != nil {
			t.Fatalf("other guild login failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("other guild login waited for the held guild row")
	case <-ctx.Done():
		t.Fatalf("other guild login timed out: %v", ctx.Err())
	}

	started := make(chan struct{}, 2)
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	firstPending, secondPending = firstDone, secondDone
	go func() {
		started <- struct{}{}
		firstDone <- f.s.p13GuildLoginAt(ctx, ids[1], now)
	}()
	go func() {
		started <- struct{}{}
		secondDone <- f.s.p13GuildLoginAt(ctx, ids[4], now)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatalf("same guild login did not start: %v", ctx.Err())
		}
	}
	select {
	case err := <-firstPending:
		firstPending = nil
		t.Fatalf("first same-guild login passed the held lock: %v", err)
	case err := <-secondPending:
		secondPending = nil
		t.Fatalf("second same-guild login passed the held lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		t.Fatalf("same guild lock check timed out: %v", ctx.Err())
	}
	if err := hold.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-firstPending:
		firstPending = nil
		if err != nil {
			t.Fatalf("first same-guild login after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("first same-guild login did not finish: %v", ctx.Err())
	}
	select {
	case err := <-secondPending:
		secondPending = nil
		if err != nil {
			t.Fatalf("second same-guild login after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("second same-guild login did not finish: %v", ctx.Err())
	}

	afterA, afterB := readGuild(guildA), readGuild(guildB)
	if afterA.Exp != beforeA.Exp+2*score || afterA.ActiveScore != beforeA.ActiveScore+2*active {
		t.Fatalf("guild A credit: exp %d -> %d, active %d -> %d", beforeA.Exp, afterA.Exp, beforeA.ActiveScore, afterA.ActiveScore)
	}
	if afterB.Exp != beforeB.Exp+score || afterB.ActiveScore != beforeB.ActiveScore+active {
		t.Fatalf("guild B credit: exp %d -> %d, active %d -> %d", beforeB.Exp, afterB.Exp, beforeB.ActiveScore, afterB.ActiveScore)
	}
	for _, id := range []int64{ids[1], ids[4]} {
		var member db.P13GuildMember
		if err := database.First(&member, "player_id=? AND guild_id=?", id, guildA).Error; err != nil {
			t.Fatal(err)
		}
		if member.LastLoginDay != p13GuildDay(now) {
			t.Fatalf("same-guild member login day = %q, want %q", member.LastLoginDay, p13GuildDay(now))
		}
	}
	for _, id := range []int64{ids[1], ids[4], ids[3]} {
		if err := f.s.p13GuildLoginAt(ctx, id, now); err != nil {
			t.Fatalf("same-day login replay: %v", err)
		}
	}
	replayA, replayB := readGuild(guildA), readGuild(guildB)
	if replayA.Exp != afterA.Exp || replayA.ActiveScore != afterA.ActiveScore || replayB.Exp != afterB.Exp || replayB.ActiveScore != afterB.ActiveScore {
		t.Fatal("same-day login replay duplicated guild credit")
	}
}
