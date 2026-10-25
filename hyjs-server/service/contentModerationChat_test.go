package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// All sending paths must reject before looking up a receipt, charging a resource,
// changing a cooldown, or invoking a push. No database is needed for this gate.
func TestContentModerationChatEntrypoints(t *testing.T) {
	s := &GameService{}
	ctx := context.Background()
	for _, text := range []string{"色\u200b情", "習 近 平"} {
		for _, kind := range []int32{constUtil.P10WorldChatType, constUtil.P13GuildChatType, constUtil.P10FieldChatType} {
			if _, err := s.p10ChannelAt(ctx, 1, &pb.CSChatTcp{ChatType: &kind, Content: &text}, 1, time.Now()); !errors.Is(err, constUtil.ErrChatSensitive) {
				t.Fatalf("channel %d did not reject: %v", kind, err)
			}
		}
		for _, request := range []proto.Message{
			&pb.CSApplyFriend{FriendId: proto.Int64(2), Word: &text},
			&pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(2), Content: &text},
			&pb.CSSendMassageToFriend{PlayerId: proto.Int64(2), Content: &text},
		} {
			if _, handled, err := s.p10Social(ctx, 1, request, 1, time.Now()); !handled || !errors.Is(err, constUtil.ErrChatSensitive) {
				t.Fatalf("private/leave word did not reject: %v", err)
			}
		}
		for _, request := range []proto.Message{
			&pb.CSChatTcp{ChatType: proto.Int32(6), Content: &text},
			&pb.CSUFTalk{Content: &text},
		} {
			if _, err := s.p13UFChat(ctx, 1, request, 1, time.Now()); !errors.Is(err, constUtil.ErrChatSensitive) {
				t.Fatalf("guild fight did not reject: %v", err)
			}
		}
		if _, err := s.p10VoicePublish(ctx, 1, nil, &pb.CSChatTcp{IsAudio: proto.Int32(2), Content: &text}); !errors.Is(err, constUtil.ErrChatSensitive) {
			t.Fatalf("voice text field escaped: %v", err)
		}
	}
}

func TestContentModerationChannelNoWritePushOrCooldown(t *testing.T) {
	f, recipient, _ := p10ChannelSetup(t)
	p10ChannelEnable(t)
	sender, peer := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, sender)
	p10SceneConnect(t, f, recipient, peer)
	sender.drain()
	peer.drain()
	now := time.Now()
	request := &pb.CSChatTcp{ChatType: proto.Int32(1), Content: proto.String("色 情")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, request, 901, now); !errors.Is(err, constUtil.ErrChatSensitive) {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10ChannelChat{}, &db.P10SocialReceipt{}, &db.P10SocialChatState{}} {
		var count int64
		if err := db.GetIns().Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rejection changed %T: count=%d err=%v", table, count, err)
		}
	}
	if len(p10ChannelChats(sender.drain())) != 0 || len(p10ChannelChats(peer.drain())) != 0 {
		t.Fatal("blocked chat was pushed")
	}
	// The same stamp can be corrected immediately; the rejection consumed nothing.
	request.Content = proto.String("大家晚上八点集合")
	result, err := f.s.p10ChannelAt(f.ctx, f.id, request, 901, now)
	if err != nil || len(result) != 1 || result[0].(*pb.SCChatTcp).GetContent() != request.GetContent() {
		t.Fatalf("normal corrected message failed: %v", err)
	}
	if len(p10ChannelChats(peer.drain())) != 1 {
		t.Fatal("normal corrected message was not delivered once")
	}
}
