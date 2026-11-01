package service

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP10ChatAvatarFirstCampAndInvalidSources(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{"Hero": {
		{"id": "hero_047", "iconName": "hero_icon_047"},
		{"id": "hero_020", "iconName": "hero_icon_020"},
		{"id": "hero_no_icon"},
	}}}
	player := &pb.Player{HeroWarId: proto.String("main")}
	main := &pb.Warrior{Id: proto.String("hero_047"), Guid: proto.String("main")}
	other := &pb.Warrior{Id: proto.String("hero_020"), Guid: proto.String("other")}
	slot := func(position int32, id, guid string) *pb.FormationItem {
		return &pb.FormationItem{FormatPos: proto.Int32(position), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String(id), Guid: proto.String(guid)}}}
	}
	tests := []struct {
		name      string
		game      *model.GameState
		want      string
		wantValid bool
	}{
		{"first position, not list order", &model.GameState{Warriors: []*pb.Warrior{main, other}, Formation: []*pb.FormationItem{slot(2, "hero_047", "main"), slot(1, "hero_020", "other")}}, "hero_020", true},
		{"legacy no first position", &model.GameState{Warriors: []*pb.Warrior{main, other}, Formation: []*pb.FormationItem{slot(2, "hero_020", "other")}}, "hero_047", true},
		{"unowned first position", &model.GameState{Warriors: []*pb.Warrior{main}, Formation: []*pb.FormationItem{slot(1, "hero_020", "foreign")}}, "", false},
		{"mismatched hero and guid", &model.GameState{Warriors: []*pb.Warrior{main, other}, Formation: []*pb.FormationItem{slot(1, "hero_047", "other")}}, "", false},
		{"missing hero", &model.GameState{Formation: []*pb.FormationItem{slot(1, "hero_020", "other")}}, "", false},
		{"unknown table id", &model.GameState{Warriors: []*pb.Warrior{{Id: proto.String("hero_unknown"), Guid: proto.String("other")}}, Formation: []*pb.FormationItem{slot(1, "hero_unknown", "other")}}, "", false},
		{"missing icon", &model.GameState{Warriors: []*pb.Warrior{{Id: proto.String("hero_no_icon"), Guid: proto.String("other")}}, Formation: []*pb.FormationItem{slot(1, "hero_no_icon", "other")}}, "", false},
		{"duplicate ownership guid", &model.GameState{Warriors: []*pb.Warrior{other, proto.Clone(other).(*pb.Warrior)}, Formation: []*pb.FormationItem{slot(1, "hero_020", "other")}}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := s.p10ChatAvatar(player, tt.game)
			if id != tt.want || ok != tt.wantValid {
				t.Fatalf("avatar=(%q,%v), want (%q,%v)", id, ok, tt.want, tt.wantValid)
			}
		})
	}
}

func p10ChatSetAvatar(t *testing.T, f *p1Fixture, id int64, first string) {
	t.Helper()
	if err := updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
		main := g.Warriors[0]
		var other *pb.Warrior
		for _, warrior := range g.Warriors {
			if warrior.GetGuid() == "chat-avatar-other" {
				other = warrior
			}
		}
		if other == nil {
			other = proto.Clone(main).(*pb.Warrior)
			other.Id, other.Guid = proto.String("hero_020"), proto.String("chat-avatar-other")
			g.Warriors = append(g.Warriors, other)
		}
		chosen := other
		if first == main.GetId() {
			chosen = main
		}
		g.Formation = []*pb.FormationItem{
			{FormatPos: proto.Int32(2), Things: []*pb.FormationThing{{Pos: proto.Int32(0)}}},
			{FormatPos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: chosen.Id, Guid: chosen.Guid}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP10ChatAvatarPublicPrivateHistoryAndReceipts(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, peer, outsider := p10ChannelSetup(t)
	p10ChannelEnable(t)
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, peer, rb)
	p10SceneConnect(t, f, outsider, rc)
	if _, _, err := f.s.DispatchP10Scene(f.ctx, outsider, &pb.CSTransferLine{Line: proto.Int32(2)}, 99); err != nil {
		t.Fatal(err)
	}
	ra.drain()
	rb.drain()
	rc.drain()
	start := time.Now().Add(-3 * time.Minute)
	world := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("旧世界")}
	field := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10FieldChatType), Content: proto.String("旧野外")}
	private := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(peer), Content: proto.String("旧私聊")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, world, 100, start); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, field, 101, start.Add(25*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, private, 102, start.Add(50*time.Second)); err != nil {
		t.Fatal(err)
	}
	var saved db.P10ChannelChat
	if err := db.GetIns().Where("sender_id=? AND chat_type=?", f.id, constUtil.P10WorldChatType).First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), saved.SenderInfo...)
	p10ChatSetAvatar(t, f, f.id, "hero_020")
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Name = proto.String("改名后"); return nil }); err != nil {
		t.Fatal(err)
	}
	plain, err := f.s.p10Player(db.GetIns(), f.id)
	if err != nil || plain.GetHeroWarId() != "hero_047" {
		t.Fatal("non-chat player projection changed", plain, err)
	}
	for _, viewer := range []int64{f.id, peer} {
		for _, kind := range []int32{constUtil.P10WorldChatType, constUtil.P10FieldChatType} {
			history, err := f.s.p10ChannelHistory(db.GetIns(), viewer, kind)
			if err != nil || len(history) != 1 || history[0].GetPlay().GetHeroWarId() != "hero_020" || history[0].GetPlay().GetPlayerName() == "改名后" {
				t.Fatal("visible old public history", viewer, kind, history, err)
			}
		}
		got := p10Call(t, f, viewer, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, start).(*pb.SCChatCache)
		if len(got.GetChatList()) != 1 || got.ChatList[0].GetPlay().GetHeroWarId() != "hero_020" || got.ChatList[0].GetPlay().GetPlayerName() == "改名后" {
			t.Fatal("visible old private history", viewer, got)
		}
	}
	if got, err := f.s.p10ChannelHistory(db.GetIns(), outsider, constUtil.P10FieldChatType); err != nil || len(got) != 0 {
		t.Fatal("foreign field history", got, err)
	}
	if got := p10Call(t, f, outsider, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, start).(*pb.SCChatCache); len(got.GetChatList()) != 0 {
		t.Fatal("private history leaked", got)
	}
	if err := db.GetIns().First(&saved, saved.ID).Error; err != nil || !bytes.Equal(before, saved.SenderInfo) {
		t.Fatal("history read modified frozen sender snapshot", err)
	}
	rb.drain()
	rc.drain()
	newWorld := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("新世界")}
	newField := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10FieldChatType), Content: proto.String("新野外")}
	sent, err := f.s.p10ChannelAt(f.ctx, f.id, newWorld, 200, start.Add(75*time.Second))
	if err != nil || sent[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new world sender", sent, err)
	}
	for _, rec := range []*p10SceneRecorder{rb, rc} {
		pushed := p10ChannelChats(rec.drain())
		if len(pushed) != 1 || pushed[0].GetPlay().GetHeroWarId() != "hero_020" {
			t.Fatal("new world recipient", pushed)
		}
	}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, newField, 201, start.Add(100*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := p10ChannelChats(rb.drain()); len(got) != 1 || got[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new field recipient", got)
	}
	if got := p10ChannelChats(rc.drain()); len(got) != 0 {
		t.Fatal("new field leaked across lines", got)
	}
	newPrivate := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(peer), Content: proto.String("新私聊")}
	privateResult, handled, err := f.s.DispatchP10Social(f.ctx, f.id, newPrivate, 202)
	if err != nil || !handled || privateResult[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new private sender", privateResult, handled, err)
	}
	if got := p10ChannelChats(rb.drain()); len(got) != 1 || got[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new private recipient", got)
	}
	if got := p10ChannelChats(rc.drain()); len(got) != 0 {
		t.Fatal("new private leaked", got)
	}
	p10ChatSetAvatar(t, f, f.id, "hero_047")
	retry, err := f.s.p10ChannelAt(f.ctx, f.id, newWorld, 200, time.Now())
	if err != nil || !proto.Equal(sent[0], retry[0]) || len(p10ChannelChats(rb.drain())) != 0 || len(p10ChannelChats(rc.drain())) != 0 {
		t.Fatal("public retry changed or repushed", retry, err)
	}
	privateRetry, handled, err := f.s.DispatchP10Social(f.ctx, f.id, newPrivate, 202)
	if err != nil || !handled || !proto.Equal(privateResult[0], privateRetry[0]) || len(p10ChannelChats(rb.drain())) != 0 {
		t.Fatal("private retry changed or repushed", privateRetry, err)
	}
	if got, err := f.s.p10ChannelHistory(db.GetIns(), peer, constUtil.P10WorldChatType); err != nil || len(got) != 2 || got[1].GetPlay().GetHeroWarId() != "hero_047" {
		t.Fatal("history did not follow current first camp", got, err)
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: peer, TargetID: f.id, CreatedAt: time.Now().UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := f.s.p10ChannelHistory(db.GetIns(), peer, constUtil.P10WorldChatType); err != nil || len(got) != 0 {
		t.Fatal("blocked public history leaked", got, err)
	}
	if got := p10Call(t, f, peer, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, start).(*pb.SCChatCache); len(got.GetChatList()) != 0 {
		t.Fatal("blocked private history leaked", got)
	}
}

func TestP10ChatAvatarGuildHistoryAndMembership(t *testing.T) {
	t.Setenv("HYJS_P13_MYSQL_DATABASE", "")
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	p13GuildOpen(t, true)
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Update("chat_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Minute)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("头像测试")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	receivers := map[int64]*p10SceneRecorder{}
	for _, id := range ids[:3] {
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		rec := &p10SceneRecorder{}
		receivers[id] = rec
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, profile.Session, rec.send))
	}
	old := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P13GuildChatType), Content: proto.String("旧公会消息")}
	if _, err := f.s.p10ChannelAt(f.ctx, ids[0], old, 100, now); err != nil {
		t.Fatal(err)
	}
	p10ChatSetAvatar(t, f, ids[0], "hero_020")
	history, err := f.s.p10ChannelHistory(db.GetIns(), ids[1], constUtil.P13GuildChatType)
	if err != nil || len(history) != 1 || history[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("old guild avatar", history, err)
	}
	if history, err := f.s.p10ChannelHistory(db.GetIns(), ids[2], constUtil.P13GuildChatType); err != nil || len(history) != 0 {
		t.Fatal("nonmember guild history leaked", history, err)
	}
	receivers[ids[1]].drain()
	receivers[ids[2]].drain()
	newMessage := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P13GuildChatType), Content: proto.String("新公会消息")}
	sent, err := f.s.p10ChannelAt(f.ctx, ids[0], newMessage, 101, now.Add(25*time.Second))
	if err != nil || sent[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new guild sender", sent, err)
	}
	if got := p10ChannelChats(receivers[ids[1]].drain()); len(got) != 1 || got[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new guild member push", got)
	}
	if got := p10ChannelChats(receivers[ids[2]].drain()); len(got) != 0 {
		t.Fatal("new guild push leaked", got)
	}
	p13GuildCall(t, f, ids[1], &pb.CSQuitUnion{}, 200, now.Add(time.Minute))
	if history, err := f.s.p10ChannelHistory(db.GetIns(), ids[1], constUtil.P13GuildChatType); err != nil || len(history) != 0 {
		t.Fatal("departed member retained history", history, err)
	}
}

func TestP10ChatAvatarVoiceHistoryAndImmutableFrames(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, peer, outsider := p10VoiceSetup(t)
	p10VoiceEnable(t)
	p10ChannelEnable(t)
	ownerSession, peerSession := p10VoiceSession(t, f.id), p10VoiceSession(t, peer)
	received, outside := &p10SceneRecorder{}, &p10SceneRecorder{}
	t.Cleanup(f.s.RegisterP10Client(f.ctx, f.id, ownerSession, func(proto.Message) error { return nil }))
	t.Cleanup(f.s.RegisterP10Client(f.ctx, peer, peerSession, received.send))
	t.Cleanup(f.s.RegisterP10Client(f.ctx, outsider, p10VoiceSession(t, outsider), outside.send))
	upload, err := f.s.UploadP10Voice(f.ctx, f.id, ownerSession, constUtil.P10WorldChatType, "old-avatar-voice", "world/1", p10VoiceTone(2100))
	if err != nil {
		t.Fatal(err)
	}
	body := p10VoiceBody(upload, constUtil.P10WorldChatType, 0)
	old, err := f.s.p10VoicePublish(f.ctx, f.id, ownerSession, body)
	if err != nil || old[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_047" {
		t.Fatal("old voice sender", old, err)
	}
	var stored db.P10VoiceMessage
	if err := db.GetIns().Where("owner_id=?", f.id).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	wire := append([]byte(nil), stored.Frame...)
	received.drain()
	outside.drain()
	p10ChatSetAvatar(t, f, f.id, "hero_020")
	history, err := f.s.p10VoiceHistory(f.ctx, peer, peerSession, constUtil.P10WorldChatType)
	if err != nil || len(history) != 1 || history[0].GetPlay().GetHeroWarId() != "hero_020" || history[0].GetAudioDownloadUrl() != upload.FileName {
		t.Fatal("old voice history", history, err)
	}
	if err := db.GetIns().First(&stored, stored.ID).Error; err != nil || !bytes.Equal(wire, stored.Frame) || p6Digest(stored.Frame) != stored.FrameHash {
		t.Fatal("voice history modified signed frame", err)
	}
	retry, err := f.s.p10VoicePublish(f.ctx, f.id, ownerSession, body)
	if err != nil || !proto.Equal(old[0], retry[0]) || len(p10ChannelChats(received.drain())) != 0 {
		t.Fatal("voice retry changed or repushed", retry, err)
	}
	// The isolated clock fixture advances the common chat throttle before a second upload.
	if err := db.GetIns().Model(&db.P10SocialChatState{}).Where("player_id=?", f.id).Update("last_send_at", time.Now().Add(-time.Minute).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	private, err := f.s.UploadP10Voice(f.ctx, f.id, ownerSession, constUtil.P10PrivateChatType, "new-avatar-voice", "person", p10VoiceTone(2100))
	if err != nil {
		t.Fatal(err)
	}
	newFrame, err := f.s.p10VoicePublish(f.ctx, f.id, ownerSession, p10VoiceBody(private, constUtil.P10PrivateChatType, peer))
	if err != nil || newFrame[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new private voice sender", newFrame, err)
	}
	if got := p10ChannelChats(received.drain()); len(got) != 1 || got[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new private voice recipient", got)
	}
	if got := p10ChannelChats(outside.drain()); len(got) != 0 {
		t.Fatal("private voice leaked", got)
	}
	if history, err := f.s.p10VoiceHistory(f.ctx, outsider, p10VoiceSession(t, outsider), constUtil.P10PrivateChatType); err != nil || len(history) != 0 {
		t.Fatal("private voice history leaked", history, err)
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: peer, TargetID: f.id, CreatedAt: time.Now().UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if history, err := f.s.p10VoiceHistory(f.ctx, peer, peerSession, constUtil.P10WorldChatType); err != nil || len(history) != 0 {
		t.Fatal("blocked voice history leaked", history, err)
	}
}

func TestP10ChatAvatarHistoryInvalidOrMissingProfileKeepsSnapshot(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, peer, _ := p10Setup(t)
	p10ChatSetAvatar(t, f, f.id, "hero_020")
	original := &pb.SimplePlayerInfo{PlayerId: proto.Int64(f.id), PlayerName: proto.String("旧名"), HeroWarId: proto.String("hero_047")}
	mismatch := &pb.SimplePlayerInfo{PlayerId: proto.Int64(peer), HeroWarId: proto.String("hero_047")}
	missing := &pb.SimplePlayerInfo{PlayerId: proto.Int64(99999999), HeroWarId: proto.String("hero_047")}
	refs := []p10ChatAvatarRef{{senderID: f.id, info: original}, {senderID: f.id, info: mismatch}, {senderID: 99999999, info: missing}}
	f.s.p10ApplyChatAvatars(db.GetIns(), refs)
	if original.GetHeroWarId() != "hero_020" || original.GetPlayerName() != "旧名" || mismatch.GetHeroWarId() != "hero_047" || missing.GetHeroWarId() != "hero_047" {
		t.Fatal("avatar projection changed unrelated or mismatched identities", original, mismatch, missing)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("game", "{invalid").Error; err != nil {
		t.Fatal(err)
	}
	original.HeroWarId = proto.String("hero_047")
	f.s.p10ApplyChatAvatars(db.GetIns(), refs)
	if original.GetHeroWarId() != "hero_047" {
		t.Fatal("invalid profile fabricated an avatar or lost the snapshot", original)
	}
	// A failed optional profile query must also leave the authorized snapshot usable.
	f.s.p10ApplyChatAvatars(db.GetIns().Where("nonexistent_avatar_column = ?", 1), refs)
	if original.GetHeroWarId() != "hero_047" {
		t.Fatal("failed optional lookup changed snapshot", original)
	}
}

func TestP10ChatAvatarUnavailableDoesNotBlockMessages(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, peer, _ := p10ChannelSetup(t)
	p10ChannelEnable(t)
	sender, recipient := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, sender)
	p10SceneConnect(t, f, peer, recipient)
	sender.drain()
	recipient.drain()
	// Keep the combat formation valid while making both display sources unavailable.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.HeroWarId = proto.String("missing-main-guid")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hero := f.s.table("Hero", "id", "hero_047")
	if hero == nil {
		t.Fatal("fixture hero missing")
	}
	hero["iconName"] = ""
	now := time.Now().Add(-time.Minute)
	world := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("头像缺失仍可发送")}
	message, err := f.s.p10ChannelAt(f.ctx, f.id, world, 1, now)
	if err != nil || message[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "" {
		t.Fatal("missing avatar blocked or fabricated public sender", message, err)
	}
	if got := p10ChannelChats(recipient.drain()); len(got) != 1 || got[0].GetContent() != world.GetContent() {
		t.Fatal("missing avatar blocked public recipient", got)
	}
	if history, err := f.s.p10ChannelHistory(db.GetIns(), peer, constUtil.P10WorldChatType); err != nil || len(history) != 1 || history[0].GetPlay().GetHeroWarId() != "" {
		t.Fatal("missing avatar blocked public history", history, err)
	}
	private := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(peer), Content: proto.String("头像缺失仍可私聊")}
	if _, _, err := f.s.p10Social(f.ctx, f.id, private, 2, now.Add(25*time.Second)); err != nil {
		t.Fatal("missing avatar blocked private sender", err)
	}
	if history := p10Call(t, f, peer, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, now).(*pb.SCChatCache); len(history.GetChatList()) != 1 || history.ChatList[0].GetPlay().GetHeroWarId() != "" {
		t.Fatal("missing avatar blocked private history", history)
	}
}

func TestP10ChatAvatarUnionFightHistoryAndScope(t *testing.T) {
	t.Setenv("HYJS_P13_MYSQL_DATABASE", "")
	f, ids, _, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离聊天头像测试"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightChatPolicy{}).Where("id=1").Update("text_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(clock.Load())
	var enemy db.P13GuildMember
	otherGuild := match.RightGuildID
	if otherGuild == p13GuildID(t, f, ids[0], now) {
		otherGuild = match.LeftGuildID
	}
	if err := db.GetIns().First(&enemy, "guild_id=?", otherGuild).Error; err != nil {
		t.Fatal(err)
	}
	receivers := map[int64]*p10SceneRecorder{}
	for _, id := range []int64{ids[0], ids[10], enemy.PlayerID} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		rec := &p10SceneRecorder{}
		receivers[id] = rec
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, profile.Session, rec.send))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, rec := range receivers {
		rec.drain()
	}
	old := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P13UnionFightChatType), Content: proto.String("战场旧消息")}
	sent, err := f.s.p13UFChat(f.ctx, ids[0], old, 100, now)
	if err != nil || sent[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_047" {
		t.Fatal("old war sender", sent, err)
	}
	receivers[ids[10]].drain()
	receivers[enemy.PlayerID].drain()
	p10ChatSetAvatar(t, f, ids[0], "hero_020")
	history, err := f.s.p13UFChatHistory(f.ctx, ids[10], now)
	if err != nil || len(history[0].(*pb.SCUFChatCache).ChatList) != 1 || history[0].(*pb.SCUFChatCache).ChatList[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("old war avatar", history, err)
	}
	if history, err := f.s.p13UFChatHistory(f.ctx, enemy.PlayerID, now); err != nil || len(history[0].(*pb.SCUFChatCache).ChatList) != 0 {
		t.Fatal("enemy war history leaked", history, err)
	}
	retry, err := f.s.p13UFChat(f.ctx, ids[0], old, 100, now)
	if err != nil || !proto.Equal(sent[0], retry[0]) || len(p10ChannelChats(receivers[ids[10]].drain())) != 0 {
		t.Fatal("war retry changed or repushed", retry, err)
	}
	newMessage := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P13UnionFightChatType), Content: proto.String("战场新消息")}
	newFrame, err := f.s.p13UFChat(f.ctx, ids[0], newMessage, 101, now.Add(25*time.Second))
	if err != nil || newFrame[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new war sender", newFrame, err)
	}
	if got := p10ChannelChats(receivers[ids[10]].drain()); len(got) != 1 || got[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new war member push", got)
	}
	if got := p10ChannelChats(receivers[enemy.PlayerID].drain()); len(got) != 0 {
		t.Fatal("enemy war push leaked", got)
	}
}
