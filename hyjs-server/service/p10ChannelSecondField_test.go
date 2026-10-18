package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原雪原转场和在线身份必须真实建立；森林、异线与未完成加载者都不能读到该频道。
func TestP10ChannelSecondFieldOriginalEntryHistoryAndIsolation(t *testing.T) {
	f, peer, forest := p10ChannelSetup(t)
	p10ChannelEnable(t)
	for _, id := range []int64{f.id, peer, forest} {
		// 只准备原地图25级资格，不补货币、道具、关系或聊天历史。
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(25)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		scene := "field_002"
		if id == forest {
			scene = "field_001"
		}
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldScene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	var (
		selfRecorder   = &p10SceneRecorder{}
		peerRecorder   = &p10SceneRecorder{}
		forestRecorder = &p10SceneRecorder{}
	)
	p10SceneConnect(t, f, f.id, selfRecorder)
	p10SceneConnect(t, f, peer, peerRecorder)
	p10SceneConnect(t, f, forest, forestRecorder)
	selfRecorder.drain()
	peerRecorder.drain()
	forestRecorder.drain()
	now := time.Now()
	request := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10FieldChatType), Content: proto.String("雪原当前频道")}
	first, err := f.s.p10ChannelAt(f.ctx, f.id, request, 0, now)
	if err != nil || len(first) != 1 {
		t.Fatal("original snow-field send", err)
	}
	if len(p10ChannelChats(peerRecorder.drain())) != 1 || len(p10ChannelChats(forestRecorder.drain())) != 0 {
		t.Fatal("snow chat crossed map or did not reach current peer")
	}
	result, handled, err := f.s.DispatchP10Channel(f.ctx, peer, &pb.CSSceneChatCache{}, 0)
	if err != nil || !handled || len(result[0].(*pb.SCSceneChatCache).GetChatList()) != 1 {
		t.Fatal("original snow history", err)
	}
	history, err := f.s.p10ChannelHistory(db.GetIns(), forest, constUtil.P10FieldChatType)
	if err != nil || len(history) != 0 {
		t.Fatal("snow history crossed into forest", err)
	}
	// 原TCP无时间戳的同内容重传仍使用一份回执，不再广播第二次。
	retry, err := f.s.p10ChannelAt(f.ctx, f.id, request, 0, now.Add(time.Second))
	if err != nil || !proto.Equal(first[0], retry[0]) || len(p10ChannelChats(peerRecorder.drain())) != 0 {
		t.Fatal("snow original retry duplicated", err)
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, peer, &pb.CSTransferLine{Line: proto.Int32(2)}, 0); err != nil {
		t.Fatal(err)
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), peer, constUtil.P10FieldChatType)
	if err != nil || len(history) != 0 {
		t.Fatal("snow history crossed lines", err)
	}
	// 旧presence仍在雪原时，角色进入加载阶段也不能继续发送或读取旧场景消息。
	if _, err = f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p10ChannelAt(f.ctx, f.id, request, 5002, now.Add(21*time.Second)); err == nil {
		t.Fatal("loading sender used stale snow presence")
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), f.id, constUtil.P10FieldChatType)
	if err != nil || len(history) != 0 {
		t.Fatal("loading sender read stale snow history", err)
	}
	if count := p1Count(t, &db.P10ChannelChat{}, "chat_type=?", constUtil.P10FieldChatType); count != 1 {
		t.Fatal("retries or rejected sends added chat", count)
	}
}
