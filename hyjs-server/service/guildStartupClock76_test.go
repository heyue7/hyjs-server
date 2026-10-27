package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Initial guild/coin/roster prerequisites use the existing finite fixture;
// contribution and the future prayer are produced by original paid consumers.
// This exercises the actual startup migration with its role-less service ctx.
func TestGuildStartupClock76RepairLegacyAfterFuturePrayerAndRewind(t *testing.T) {
	f, ids, _ := p13PrayerSetup(t)
	id := ids[0]
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	ctx := guildClock75Move(t, f, actor, id, 6)
	forward := LocalPlayNow(ctx)
	p13PrayerPublish(t, forward.Add(-time.Second))
	if _, err := f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, time.Now().UnixMilli(), forward); err != nil {
		t.Fatal(err)
	}
	var paid db.P13GuildPrayerRoll
	if err := db.GetIns().First(&paid, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	frozen, _ := json.Marshal(paid)
	ctx = guildClock75Move(t, f, actor, id, -7)
	back := LocalPlayNow(ctx)
	var member db.P13GuildMember
	if err := db.GetIns().First(&member, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := p13GuildPrayerPropertiesRead(&paid, &member, back); err == nil {
		t.Fatal("strict unprojected future snapshot became valid")
	}
	before, _ := p10LuckyProfile(t, id)
	f.restart(t)
	// Matches main: InitializeServerGameClock precedes service context, with no
	// role/session/bound worker calendar on the startup context itself.
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	startup := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if ServerGameClockContextBound(startup) {
		t.Fatal("fixture accidentally bound away the startup gap")
	}
	if err := f.s.RepairLegacyFirstMission(startup); err != nil {
		t.Fatal("actual startup migration rejected a paid prayer on a future visited day", err)
	}
	after, _ := p10LuckyProfile(t, id)
	if before.GetCoin() != after.GetCoin() || before.GetDiamond() != after.GetDiamond() || before.GetContribution() != after.GetContribution() || before.GetLevel() != after.GetLevel() || before.GetExp() != after.GetExp() {
		t.Fatal("startup clock repair changed assets")
	}
	var preserved db.P13GuildPrayerRoll
	if err := db.GetIns().First(&preserved, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	stable, _ := json.Marshal(preserved)
	if !bytes.Equal(frozen, stable) {
		t.Fatal("startup rewrote immutable paid prayer")
	}
	bound := serverClockBindTest(t, f, id)
	prop, hash, _, err := f.s.P13GuildPrayerProperties(bound, db.GetIns(), id, LocalPlayNow(bound))
	if err != nil || hash != "" || !proto.Equal(prop, &pb.BaseProp{}) {
		t.Fatal("rewound role borrowed future prayer", prop, hash, err)
	}
	ctx = guildClock75Move(t, f, actor, id, 7)
	prop, hash, _, err = f.s.P13GuildPrayerProperties(ctx, db.GetIns(), id, LocalPlayNow(ctx))
	if err != nil || hash != paid.SourceHash || proto.Equal(prop, &pb.BaseProp{}) {
		t.Fatal("return to original paid day lost frozen prayer", hash, err)
	}
}
