package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

type p13ChatHistoryQueryCounter struct {
	logger.Interface
	count int
}

func (c *p13ChatHistoryQueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.count++
	c.Interface.Trace(ctx, begin, fc, err)
}

func TestP13GuildChatHistoryBatchVisibilityOrderAndFixedQueries(t *testing.T) {
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("1=1").Delete(&db.P10ChannelChat{}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour)
	p13GuildOpen(t, true)
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Update("chat_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("聊天快照")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	for i, id := range ids[1:] {
		p13Join(t, f, ids[0], id, guild, int64(10+i), now)
	}
	for _, id := range ids {
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, profile.Session, func(proto.Message) error { return nil }))
	}
	expected := []*pb.ChatInfo{}
	send := func(round int) {
		t.Helper()
		for i, id := range ids[:3] {
			at := now.Add(time.Duration(round*30+i) * time.Second)
			content := fmt.Sprintf("发送%d轮%d", i, round)
			messages, err := f.s.p10ChannelAt(f.ctx, id, &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P13GuildChatType), Content: proto.String(content)}, int64(100+round*10+i), at)
			if err != nil {
				t.Fatal(err)
			}
			frame := messages[0].(*pb.SCChatTcp)
			expected = append(expected, &pb.ChatInfo{ChatType: frame.ChatType, Content: frame.Content, Time: frame.Time, Play: frame.Play, IsAudio: proto.Int32(0)})
		}
	}
	counter := &p13ChatHistoryQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	tx := db.GetIns().Session(&gorm.Session{Logger: counter})
	read := func(want []*pb.ChatInfo) int {
		t.Helper()
		counter.count = 0
		actual, err := f.s.p10ChannelHistory(tx, ids[4], constUtil.P13GuildChatType)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(&pb.SCChatCache{ChatList: actual}, &pb.SCChatCache{ChatList: want}) {
			t.Fatalf("history differs: got=%v want=%v", actual, want)
		}
		return counter.count
	}
	send(0)
	oneRound := read(expected)
	for round := 1; round < 4; round++ {
		send(round)
	}
	if many := read(expected); many != oneRound {
		t.Fatalf("history SQL grew with rows: first=%d many=%d", oneRound, many)
	}
	for _, black := range []db.P10SocialBlack{{OwnerID: ids[4], TargetID: ids[0], CreatedAt: now.UnixMilli()}, {OwnerID: ids[1], TargetID: ids[4], CreatedAt: now.UnixMilli()}} {
		if err := db.GetIns().Create(&black).Error; err != nil {
			t.Fatal(err)
		}
	}
	visible := []*pb.ChatInfo{}
	for i, frame := range expected {
		if i%3 == 2 {
			visible = append(visible, frame)
		}
	}
	if blocked := read(visible); blocked != oneRound {
		t.Fatalf("blacklist filtering added per-row SQL: initial=%d blocked=%d", oneRound, blocked)
	}
	p13GuildCall(t, f, ids[4], &pb.CSQuitUnion{}, 900, now.Add(5*time.Minute))
	read(nil)
}
