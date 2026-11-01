package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP10ChatAvatarCompressedRosterHistoryAndBadDigest(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, peer, outsider := p10ChannelSetup(t)
	p10ChannelEnable(t)
	p10SceneConnect(t, f, f.id, &p10SceneRecorder{})
	p10SceneConnect(t, f, peer, &p10SceneRecorder{})
	now := time.Now().Add(-3 * time.Minute)
	world := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("压缩前的世界消息")}
	private := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(peer), Content: proto.String("压缩前的私聊消息")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, world, 1, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, private, 2, now.Add(25*time.Second)); err != nil {
		t.Fatal(err)
	}
	var original db.P10ChannelChat
	if err := db.GetIns().Where("sender_id=?", f.id).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := append([]byte(nil), original.SenderInfo...)
	p10ChatSetAvatar(t, f, f.id, "hero_020")
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		hero := proto.Clone(game.Warriors[0]).(*pb.Warrior)
		hero.Guid = proto.String(fmt.Sprintf("chat-compressed-%04d", i))
		game.Warriors = append(game.Warriors, hero)
	}
	stored, err := model.MarshalProfileGame86(game)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stored, &fields); err != nil || len(fields["warriors"]) == 0 || fields["warriors"][0] != '{' || !bytes.Contains(fields["warriors"], []byte(constUtil.ProfileWarriorsCodec86)) {
		t.Fatal("fixture did not produce a compressed roster", err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("game", string(stored)).Error; err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		history, err := f.s.p10ChannelHistory(db.GetIns(), peer, constUtil.P10WorldChatType)
		if err != nil || len(history) != 1 || history[0].GetContent() != world.GetContent() || history[0].GetPlay().GetHeroWarId() != want {
			t.Fatal("world history avatar", history, err)
		}
		privateHistory := p10Call(t, f, peer, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, now).(*pb.SCChatCache)
		if len(privateHistory.GetChatList()) != 1 || privateHistory.ChatList[0].GetContent() != private.GetContent() || privateHistory.ChatList[0].GetPlay().GetHeroWarId() != want {
			t.Fatal("private history avatar", privateHistory)
		}
		if other := p10Call(t, f, outsider, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, now).(*pb.SCChatCache); len(other.GetChatList()) != 0 {
			t.Fatal("compressed private history leaked", other)
		}
	}
	check("hero_020")
	if err := db.GetIns().First(&profile, f.id).Error; err != nil || profile.Game == nil || !bytes.Equal([]byte(*profile.Game), stored) {
		t.Fatal("history changed compressed profile", err)
	}
	if err := db.GetIns().First(&original, original.ID).Error; err != nil || !bytes.Equal(original.SenderInfo, snapshot) {
		t.Fatal("history changed original message snapshot", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(fields["warriors"], &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["sha256"], _ = json.Marshal(strings.Repeat("0", 64))
	fields["warriors"], _ = json.Marshal(envelope)
	corrupt, _ := json.Marshal(fields)
	if _, err := model.ExpandProfileGame86(corrupt); err == nil {
		t.Fatal("damaged fixture digest was accepted")
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("game", string(corrupt)).Error; err != nil {
		t.Fatal(err)
	}
	check("hero_047")
}
