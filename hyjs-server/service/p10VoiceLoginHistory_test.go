package service

import (
	"bytes"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

// 原登录先读取HTTP聊天缓存，再连接TCP；历史中的世界语音元数据必须能在这一步恢复。
func TestP10VoiceOriginalLoginWorldMetadataBeforeTCP(t *testing.T) {
	f, recipient, _ := p10VoiceSetup(t)
	p10VoiceEnable(t)
	p10ChannelEnable(t)
	senderSession := p10VoiceSession(t, f.id)
	recipientSession := p10VoiceSession(t, recipient)
	t.Cleanup(f.s.RegisterP10Client(f.ctx, f.id, senderSession, func(proto.Message) error { return nil }))
	raw := p10VoiceTone(2100)
	upload, err := f.s.UploadP10Voice(f.ctx, f.id, senderSession, 1, "original-login-world", "world/1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p10VoicePublish(f.ctx, f.id, senderSession, p10VoiceBody(upload, 1, 0)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int32{0, 1} {
		messages, handled, err := f.s.DispatchP10Voice(f.ctx, recipient, recipientSession, &pb.CSChatCache{ChatType: proto.Int32(kind)}, 0)
		if err != nil || !handled || len(messages) != 1 {
			t.Fatal("original HTTP login cache failed", err)
		}
		list := messages[0].(*pb.SCChatCache).GetChatList()
		if len(list) != 1 || list[0].GetIsAudio() != 2 || list[0].GetAudioDownloadUrl() != upload.FileName {
			t.Fatal("world voice disappeared before original TCP login", list)
		}
	}
	// 历史中的元数据不授予字节访问；实际播放仍等待当前认证TCP。
	if _, err = f.s.DownloadP10Voice(f.ctx, recipient, recipientSession, upload.FileName); err == nil {
		t.Fatal("HTTP history bypassed live TCP playback requirement")
	}
	disconnect := f.s.RegisterP10Client(f.ctx, recipient, recipientSession, func(proto.Message) error { return nil })
	t.Cleanup(disconnect)
	got, err := f.s.DownloadP10Voice(f.ctx, recipient, recipientSession, upload.FileName)
	if err != nil || !bytes.Equal(raw, got) {
		t.Fatal("original TCP login did not enable authenticated playback", err)
	}
	disconnect()
	if _, err = f.s.DownloadP10Voice(f.ctx, recipient, recipientSession, upload.FileName); err == nil {
		t.Fatal("disconnected recipient retained playback access")
	}
}
