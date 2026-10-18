package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestP13GuildChannel 复用P10真实TCP会话与原ChatGroup=2，同会历史和推送在成员变更后立即失权。
func TestP13GuildChannelRealRolesScopePushHistoryAndWithdrawal(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, false)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10ChannelChat{}, &db.P10SocialChatState{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	recorders := make([]*p10SceneRecorder, len(ids))
	for i, id := range ids {
		recorders[i] = &p10SceneRecorder{}
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		close := f.s.RegisterP10Client(f.ctx, id, row.Session, recorders[i].send)
		t.Cleanup(close)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("同会频道")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	p13Join(t, f, ids[0], ids[2], guild, 3, now)
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 4, now)
	p13GuildCall(t, f, ids[4], &pb.CSUnionCreate{Name: proto.String("其他公会")}, 1, now)
	for _, rec := range recorders {
		rec.drain()
	}
	req := &pb.CSChatTcp{ChatType: proto.Int32(2), Content: proto.String("真实公会成员消息")}
	if _, err := f.s.p10ChannelAt(f.ctx, ids[0], req, 10, now); err == nil {
		t.Fatal("closed guild chat sent")
	}
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Update("chat_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.p10ChannelAt(f.ctx, ids[0], req, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	frame := messages[0].(*pb.SCChatTcp)
	if frame.GetChatType() != 2 || frame.GetPlay().GetPlayerId() != ids[0] {
		t.Fatal("original guild frame", frame)
	}
	for i, rec := range recorders {
		chats := p10ChannelChats(rec.drain())
		expected := 0
		if i == 1 || i == 2 {
			expected = 1
		}
		if len(chats) != expected {
			t.Fatal("foreign/absent guild push", i, len(chats), expected)
		}
	}
	if _, err = f.s.p10ChannelAt(f.ctx, ids[0], req, 10, now); err != nil {
		t.Fatal(err)
	}
	for _, rec := range recorders {
		if len(p10ChannelChats(rec.drain())) != 0 {
			t.Fatal("retry duplicate guild push")
		}
	}
	for i, id := range ids {
		history, err := f.s.p10ChannelHistory(db.GetIns(), id, 2)
		if err != nil {
			t.Fatal(err)
		}
		expected := 0
		if i < 3 {
			expected = 1
		}
		if len(history) != expected {
			t.Fatal("foreign guild history", i, len(history), expected)
		}
	}
	p13GuildCall(t, f, ids[1], &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(ids[2])}, 11, now)
	history, err := f.s.p10ChannelHistory(db.GetIns(), ids[2], 2)
	if err != nil || len(history) != 0 {
		t.Fatal("kicked member retained history", err)
	}
	if _, err = f.s.p10ChannelAt(f.ctx, ids[2], req, 12, now.Add(21*time.Second)); err == nil {
		t.Fatal("kicked member sent guild chat")
	}
	if _, err = f.s.p10ChannelAt(f.ctx, ids[0], &pb.CSChatTcp{ChatType: proto.Int32(2), Content: proto.String("退出后不会收到")}, 13, now.Add(21*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(p10ChannelChats(recorders[2].drain())) != 0 {
		t.Fatal("kicked member retained realtime push")
	}
	aggregate, handled, err := f.s.DispatchP10Channel(f.ctx, ids[1], &pb.CSChatCache{}, 14)
	if err != nil || !handled {
		t.Fatal("guild aggregate", err)
	}
	found := 0
	for _, chat := range aggregate[0].(*pb.SCChatCache).GetChatList() {
		if chat.GetChatType() == 2 {
			found++
		}
	}
	if found != 2 {
		t.Fatal("aggregate lost guild history", found)
	}
	p13GuildCall(t, f, ids[0], &pb.CSDissolveUnion{}, 15, now)
	history, err = f.s.p10ChannelHistory(db.GetIns(), ids[1], 2)
	if err != nil || len(history) != 0 {
		t.Fatal("dissolved guild exposed old history", err)
	}
}
