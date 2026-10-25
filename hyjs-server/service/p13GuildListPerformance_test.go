package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// This explicit read-load fixture represents a full 20-guild search page. Its
// copied starting formations are only for measuring reads, never battle proof.
func TestP13GuildListFullPageReadBudget(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	now := time.Now()
	var template db.Profile
	if err := db.GetIns().First(&template, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&template)
	if err != nil {
		t.Fatal(err)
	}
	var profiles []db.Profile
	var members []db.P13GuildMember
	var guilds []db.P13Guild
	for group := 0; group < 20; group++ {
		guildID := fmt.Sprintf("00000000-0000-4000-8000-%012d", group+1)
		guilds = append(guilds, db.P13Guild{ID: guildID, Name: fmt.Sprintf("读载荷%02d", group), Level: 1, CreatedAt: now.UnixMilli()})
		for i := 0; i < 35; i++ {
			id := int64(10000 + group*35 + i)
			copy := proto.Clone(player).(*pb.Player)
			copy.PlayerId = proto.Int64(id)
			copy.Name = proto.String(fmt.Sprintf("成员%03d", group*35+i))
			raw, err := json.Marshal(copy)
			if err != nil {
				t.Fatal(err)
			}
			body := string(raw)
			profiles = append(profiles, db.Profile{Id: id, Uid: fmt.Sprintf("p13-list-read-%d", id), Player: &body, Game: template.Game, Session: []byte("read-fixture")})
			role := constUtil.P13GuildMember
			if i == 0 {
				role = constUtil.P13GuildChairman
			}
			members = append(members, db.P13GuildMember{PlayerID: id, GuildID: guildID, Role: role, JoinedAt: now.Add(-time.Hour).UnixMilli()})
		}
	}
	for _, value := range []any{&guilds, &profiles, &members} {
		if err := db.GetIns().CreateInBatches(value, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
	var recorder = &adminActivityQueryRecorder{Interface: db.GetIns().Logger}
	previous := db.GetIns().Config.Logger
	db.GetIns().Config.Logger = recorder
	t.Cleanup(func() { db.GetIns().Config.Logger = previous })
	started := time.Now()
	out, err := f.s.p13GuildQuery(f.ctx, ids[0], &pb.CSUnionSearch{Page: proto.Int32(1)}, now)
	if err != nil {
		t.Fatal(err)
	}
	list := out[0].(*pb.SCUnionSearch).Unions
	if len(list) != 20 {
		t.Fatal("full search page", len(list))
	}
	for _, guild := range list {
		if guild.GetCurrentMemberNum() != 35 || guild.GetChairmanName() == "" || guild.GetTotalFightPower() <= 0 {
			t.Fatal("actual member/chairman/current power lost", guild)
		}
	}
	queries := len(recorder.sqls)
	t.Logf("20guilds/700members: SQL=%d elapsed=%s", queries, time.Since(started))
	if queries > 40 {
		t.Errorf("search must batch current profile/guild sources; got %d SQL", queries)
	}
}
