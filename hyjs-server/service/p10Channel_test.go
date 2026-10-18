package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p10ChannelSetup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f, b, c := p10SceneSetup(t)
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10ChannelChat{}, &db.P10ChannelPolicy{}, &db.P10SocialChatState{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, b, c} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(20)
			g.Position.Scene = proto.String("field_001")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return f, b, c
}
func p10ChannelEnable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P10ChannelPolicy{}).Where("id=1").Updates(map[string]any{"world_enabled": true, "field_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
}
func p10ChannelChats(messages []proto.Message) []*pb.SCChatTcp {
	var out []*pb.SCChatTcp
	for _, m := range messages {
		if c, ok := m.(*pb.SCChatTcp); ok {
			out = append(out, c)
		}
	}
	return out
}
func TestP10ChannelClosedReadsAndOfflineMutationGate(t *testing.T) {
	f, _, _ := p10ChannelSetup(t)
	for _, request := range []proto.Message{&pb.CSChatCache{}, &pb.CSChatCache{ChatType: proto.Int32(1)}, &pb.CSSceneChatCache{}} {
		result, handled, err := f.s.DispatchP10Channel(f.ctx, f.id, request, 1)
		if !handled || err != nil || len(result) != 1 {
			t.Fatal("closed original read", result, handled, err)
		}
	}
	request := &pb.CSChatTcp{ChatType: proto.Int32(1), Content: proto.String("原世界")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, request, 1, time.Now()); err == nil {
		t.Fatal("closed channel sent")
	}
	p10ChannelEnable(t)
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, request, 2, time.Now()); err == nil {
		t.Fatal("offline sender injected public text")
	}
	var count int64
	db.GetIns().Model(&db.P10ChannelChat{}).Count(&count)
	if count != 0 {
		t.Fatal("failed send retained message")
	}
}
func TestP10ChannelActualWorldFieldLineBlackAndFrozenHistory(t *testing.T) {
	f, b, c := p10ChannelSetup(t)
	p10ChannelEnable(t)
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	p10SceneConnect(t, f, c, rc)
	if _, _, err := f.s.DispatchP10Scene(f.ctx, c, &pb.CSTransferLine{Line: proto.Int32(2)}, 99); err != nil {
		t.Fatal(err)
	}
	ra.drain()
	rb.drain()
	rc.drain()
	now := time.Now()
	field := &pb.CSChatTcp{ChatType: proto.Int32(5), Content: proto.String("真实同线文字")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, field, 1, now); err != nil {
		t.Fatal(err)
	}
	if len(p10ChannelChats(rb.drain())) != 1 || len(p10ChannelChats(rc.drain())) != 0 {
		t.Fatal("field line visibility")
	}
	var count int64
	history, err := f.s.p10ChannelHistory(db.GetIns(), b, 5)
	if err != nil || len(history) != 1 || history[0].GetContent() != field.GetContent() {
		t.Fatal("real field history", history, err)
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), c, 5)
	if err != nil || len(history) != 0 {
		t.Fatal("foreign line history leak")
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: b, TargetID: f.id, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	world := &pb.CSChatTcp{ChatType: proto.Int32(1), Content: proto.String("真实世界文字")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, world, 2, now.Add(21*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(p10ChannelChats(rb.drain())) != 0 || len(p10ChannelChats(rc.drain())) != 1 {
		t.Fatal("world permission or line restriction")
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), b, 1)
	if err != nil || len(history) != 0 {
		t.Fatal("black public history leaked")
	}
	if err := db.GetIns().Model(&db.P10ChannelPolicy{}).Where("id=1").Update("world_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	result, err := f.s.p10ChannelAt(f.ctx, f.id, world, 2, now.Add(time.Minute))
	if err != nil || result[0].(*pb.SCChatTcp).GetTime() != now.Add(21*time.Second).UnixMilli() {
		t.Fatal("closed old receipt", err)
	}
	db.GetIns().Model(&db.P10ChannelChat{}).Count(&count)
	if count != 2 {
		t.Fatal("receipt duplicated public message")
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), c, 1)
	if err != nil || len(history) != 1 {
		t.Fatal("closed actual world history")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Name = proto.String("新名字"); return nil }); err != nil {
		t.Fatal(err)
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), c, 1)
	if err != nil || history[0].GetPlay().GetPlayerName() == "新名字" {
		t.Fatal("sender snapshot was not frozen")
	}
}
func TestP10ChannelNativeStampZeroConcurrentRetryAndCrossDomainThrottle(t *testing.T) {
	f, b, _ := p10ChannelSetup(t)
	p10ChannelEnable(t)
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	now := time.Now()
	request := &pb.CSChatTcp{ChatType: proto.Int32(1), Content: proto.String("并发真实消息")}
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, err := f.s.p10ChannelAt(f.ctx, f.id, request, 0, now); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	db.GetIns().Model(&db.P10ChannelChat{}).Count(&count)
	if count != 1 || len(p10ChannelChats(rb.drain())) != 1 {
		t.Fatal("native retry duplicate")
	}
	changed := proto.Clone(request).(*pb.CSChatTcp)
	changed.Content = proto.String("不同消息")
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, changed, 0, now.Add(time.Second)); err == nil {
		t.Fatal("interval bypass")
	}
	field := proto.Clone(request).(*pb.CSChatTcp)
	field.ChatType = proto.Int32(5)
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, field, 0, now.Add(time.Second)); err == nil {
		t.Fatal("cross channel bypass")
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(b), Content: proto.String("私聊同节流")}, 5, now.Add(time.Second)); err == nil {
		t.Fatal("private bypassed public throttle")
	}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, request, 0, now.Add(21*time.Second)); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Model(&db.P10ChannelChat{}).Count(&count)
	if count != 2 {
		t.Fatal("same content after interval was dropped")
	}
}
func TestP10ChannelOriginalHistoryLimitAndCurrentSceneAuthorization(t *testing.T) {
	f, b, _ := p10ChannelSetup(t)
	p10ChannelEnable(t)
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	now := time.Now()
	for i := 0; i < 52; i++ {
		request := &pb.CSChatTcp{ChatType: proto.Int32(5), Content: proto.String("有限真实历史")}
		if _, err := f.s.p10ChannelAt(f.ctx, f.id, request, int64(i+1), now.Add(time.Duration(i)*21*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	history, err := f.s.p10ChannelHistory(db.GetIns(), b, 5)
	if err != nil || len(history) != 50 || history[0].GetTime() != now.Add(42*time.Second).UnixMilli() {
		t.Fatal("original 50 chronological history", len(history), err)
	}
	result, handled, err := f.s.DispatchP10Channel(f.ctx, b, &pb.CSChatCache{}, 1)
	if !handled || err != nil || len(result[0].(*pb.SCChatCache).GetChatList()) != 50 {
		t.Fatal("bootstrap all real visible channels", err)
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { g.Position.Scene = proto.String("town_main"); return nil }); err != nil {
		t.Fatal(err)
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), b, 5)
	if err != nil || len(history) != 0 {
		t.Fatal("stale scene history authorization")
	}
	audio := &pb.CSChatTcp{ChatType: proto.Int32(1), Content: proto.String("音频"), IsAudio: proto.Int32(1)}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, audio, 999, now.Add(time.Hour)); err == nil {
		t.Fatal("unknown audio opened")
	}
}
