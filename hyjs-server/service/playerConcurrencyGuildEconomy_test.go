package service

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestPlayerConcurrencyMySQLGuildEconomyIsolation(t *testing.T) {
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p13_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p13_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated guild and accounts fixture")
	}

	f, ids := p13GuildSetup(t)
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("requires MySQL for guild row locking")
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)

	var giftPolicy db.P13GuildGiftPolicy
	if err := database.First(&giftPolicy, 1).Error; err != nil || giftPolicy.StartsAt <= 0 {
		t.Fatalf("read monthly gift start: %v", err)
	}
	// The standard gift policy is present, but this timestamp precedes its start.
	now := time.UnixMilli(giftPolicy.StartsAt).Add(-12 * time.Hour)
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("经济隔离甲")}, 101, now)
	guildA := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guildA, 102, now)
	p13GuildCall(t, f, ids[2], &pb.CSUnionCreate{Name: proto.String("经济隔离乙")}, 103, now)
	guildB := p13GuildID(t, f, ids[2], now)
	if guildA == "" || guildB == "" || guildA == guildB {
		t.Fatal("fixture must create two different guilds")
	}
	amount, err := f.s.p10Setting("UnionMinDailyDonateCoin")
	if err != nil || amount <= 0 {
		t.Fatalf("original minimum coin donation: %d, %v", amount, err)
	}
	contribution, err := f.s.p10Setting("UnionDonateCoinContributVal")
	if err != nil || contribution <= 0 {
		t.Fatalf("original contribution: %d, %v", contribution, err)
	}
	donateScore, err := f.s.p10Setting("UnionDonateCoinDonateVal")
	if err != nil || donateScore <= 0 {
		t.Fatalf("original donation score: %d, %v", donateScore, err)
	}
	exp, err := f.s.p10Setting("UnionFirstContributeCoinScore")
	if err != nil || exp <= 0 {
		t.Fatalf("original first donation experience: %d, %v", exp, err)
	}

	readGuild := func(id string) db.P13Guild {
		t.Helper()
		var guild db.P13Guild
		if err := database.First(&guild, "id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		return guild
	}
	readPlayer := func(id int64) (*pb.Player, db.P13GuildMember) {
		t.Helper()
		var row db.Profile
		if err := database.First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		player, _, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		var member db.P13GuildMember
		if err := database.First(&member, "player_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		return player, member
	}
	beforeA, beforeB := readGuild(guildA), readGuild(guildB)
	players := [3]int64{ids[0], ids[1], ids[2]}
	stamps := [3]int64{201, 202, 203}
	var beforePlayers [3]*pb.Player
	var beforeMembers [3]db.P13GuildMember
	for i, id := range players {
		beforePlayers[i], beforeMembers[i] = readPlayer(id)
		if beforePlayers[i].GetCoin() < int64(amount) || beforeMembers[i].TodayCoin != 0 {
			t.Fatalf("player %d is not ready for its first coin donation", i)
		}
	}
	if beforeMembers[0].GuildID != guildA || beforeMembers[1].GuildID != guildA || beforeMembers[2].GuildID != guildB {
		t.Fatal("fixture membership does not match the two guilds")
	}

	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	type markerKey struct{}
	firstMarker, sameMarker := new(int), new(int)
	firstCtx := context.WithValue(ctx, markerKey{}, firstMarker)
	sameCtx := context.WithValue(ctx, markerKey{}, sameMarker)
	type donationResult struct {
		messages []proto.Message
		err      error
	}
	var firstPending, otherPending, samePending <-chan donationResult
	var queryRegistered, createRegistered bool
	const queryCallback = "test:player-concurrency-guild-economy-locks"
	const createCallback = "test:player-concurrency-guild-economy-hold"
	firstEntered, sameAtLock, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var blockOnce, sameOnce, releaseOnce sync.Once
	var traceMu sync.Mutex
	var firstPolicyShared, firstGuildAfterPolicy bool
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	defer func() {
		releaseFirst()
		cancel()
		for _, pending := range []<-chan donationResult{firstPending, otherPending, samePending} {
			if pending != nil {
				<-pending
			}
		}
		if createRegistered {
			if err := database.Callback().Create().Remove(createCallback); err != nil {
				t.Errorf("remove guild receipt callback: %v", err)
			}
		}
		if queryRegistered {
			if err := database.Callback().Query().Remove(queryCallback); err != nil {
				t.Errorf("remove guild query callback: %v", err)
			}
		}
	}()
	if err := database.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if !ok {
			return
		}
		marker := tx.Statement.Context.Value(markerKey{})
		if marker == firstMarker {
			traceMu.Lock()
			if tx.Statement.Table == "p13_guild_policies" && locking.Strength == "SHARE" {
				firstPolicyShared = true
			}
			if tx.Statement.Table == "p13_guilds" && locking.Strength == "UPDATE" && len(tx.Statement.Selects) == 1 && tx.Statement.Selects[0] == "id" {
				firstGuildAfterPolicy = firstPolicyShared
			}
			traceMu.Unlock()
		}
		if marker == sameMarker && tx.Statement.Table == "p13_guilds" && locking.Strength == "UPDATE" && len(tx.Statement.Selects) == 1 && tx.Statement.Selects[0] == "id" {
			sameOnce.Do(func() { close(sameAtLock) })
		}
	}); err != nil {
		t.Fatalf("register guild query callback: %v", err)
	}
	queryRegistered = true
	if err := database.Callback().Create().Before("gorm:create").Register(createCallback, func(tx *gorm.DB) {
		if tx.Statement.Table != "p13_guild_receipts" || tx.Statement.Context.Value(markerKey{}) != firstMarker {
			return
		}
		blockOnce.Do(func() {
			close(firstEntered)
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
				tx.AddError(tx.Statement.Context.Err())
			}
		})
	}); err != nil {
		t.Fatalf("register guild receipt callback: %v", err)
	}
	createRegistered = true

	request := &pb.CSUnionDonate{DonateType: proto.String("coin")}
	startDonation := func(callCtx context.Context, i int) <-chan donationResult {
		out := make(chan donationResult, 1)
		go func() {
			messages, err := f.s.p13GuildEconomy(callCtx, players[i], request, stamps[i], now)
			out <- donationResult{messages: messages, err: err}
		}()
		return out
	}
	checkReply := func(result donationResult, label string) proto.Message {
		t.Helper()
		if result.err != nil || len(result.messages) == 0 {
			t.Fatalf("%s: %v", label, result.err)
		}
		reply, ok := result.messages[0].(*pb.SCUnionDonate)
		if !ok || reply == nil || reply.GetContributionAdd() != contribution || reply.GetDonateScoreAdd() != donateScore || reply.GetScore() != exp {
			t.Fatalf("%s returned unexpected donation deltas", label)
		}
		return reply
	}
	firstPending = startDonation(firstCtx, 0)
	select {
	case <-firstEntered:
	case result := <-firstPending:
		firstPending = nil
		t.Fatalf("first donation ended before holding its transaction: %v", result.err)
	case <-ctx.Done():
		t.Fatalf("first donation did not reach its receipt: %v", ctx.Err())
	}
	traceMu.Lock()
	seenPolicy, seenGuild := firstPolicyShared, firstGuildAfterPolicy
	traceMu.Unlock()
	if !seenPolicy || !seenGuild {
		t.Fatal("first donation did not acquire policy SHARE before its guild UPDATE lock")
	}

	otherPending = startDonation(ctx, 2)
	otherTimer := time.NewTimer(2 * time.Second)
	defer otherTimer.Stop()
	var other donationResult
	select {
	case other = <-otherPending:
		otherPending = nil
	case <-otherTimer.C:
		t.Fatal("other guild donation waited for the first guild")
	case <-ctx.Done():
		t.Fatalf("other guild donation timed out: %v", ctx.Err())
	}
	otherReply := checkReply(other, "other guild donation")
	select {
	case result := <-firstPending:
		firstPending = nil
		t.Fatalf("first donation committed before release: %v", result.err)
	default:
	}

	samePending = startDonation(sameCtx, 1)
	select {
	case <-sameAtLock:
	case result := <-samePending:
		samePending = nil
		t.Fatalf("same-guild donation ended before its guild lock: %v", result.err)
	case <-ctx.Done():
		t.Fatalf("same-guild donation did not reach its guild lock: %v", ctx.Err())
	}
	sameTimer := time.NewTimer(100 * time.Millisecond)
	defer sameTimer.Stop()
	select {
	case result := <-samePending:
		samePending = nil
		t.Fatalf("same-guild donation passed the held lock: %v", result.err)
	case <-sameTimer.C:
	case <-ctx.Done():
		t.Fatalf("same-guild lock wait timed out: %v", ctx.Err())
	}
	releaseFirst()
	var first, same donationResult
	select {
	case first = <-firstPending:
		firstPending = nil
	case <-ctx.Done():
		t.Fatalf("first donation did not finish after release: %v", ctx.Err())
	}
	firstReply := checkReply(first, "first donation")
	select {
	case same = <-samePending:
		samePending = nil
	case <-ctx.Done():
		t.Fatalf("same-guild donation did not finish after release: %v", ctx.Err())
	}
	sameReply := checkReply(same, "same-guild donation")

	verify := func() {
		t.Helper()
		for _, expected := range []struct {
			id     string
			before db.P13Guild
			count  int32
		}{{guildA, beforeA, 2}, {guildB, beforeB, 1}} {
			guild := readGuild(expected.id)
			if guild.Coin != expected.before.Coin+int64(expected.count)*int64(amount) || guild.DonateScore != expected.before.DonateScore+expected.count*donateScore || guild.Exp != expected.before.Exp+expected.count*exp {
				t.Fatalf("guild %s donation totals: coin=%d donate=%d exp=%d", expected.id, guild.Coin, guild.DonateScore, guild.Exp)
			}
		}
		for i, id := range players {
			player, member := readPlayer(id)
			if player.GetCoin() != beforePlayers[i].GetCoin()-int64(amount) || player.GetContribution() != beforePlayers[i].GetContribution()+contribution || member.TodayCoin != int64(amount) || member.Contribution != beforeMembers[i].Contribution+contribution {
				t.Fatalf("player %d donation wallet or contribution mismatch", i)
			}
			var count int64
			if err := database.WithContext(ctx).Model(&db.P13GuildReceipt{}).Where("player_id=? AND operation=? AND stamp=?", id, "CSUnionDonate", stamps[i]).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("player %d donation receipts: %d, %v", i, count, err)
			}
		}
	}
	verify()
	for i, expected := range []proto.Message{firstReply, sameReply, otherReply} {
		messages, err := f.s.p13GuildEconomy(ctx, players[i], request, stamps[i], now)
		if err != nil || len(messages) == 0 || !proto.Equal(messages[0], expected) {
			t.Fatalf("player %d donation replay changed its receipt: %v", i, err)
		}
	}
	verify()
}
