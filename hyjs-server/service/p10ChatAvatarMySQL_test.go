package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10MySQLSetChatAvatar(t *testing.T, f *p1Fixture, id int64, heroID string, largeRoster bool) {
	t.Helper()
	if err := updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
		hero := proto.Clone(g.Warriors[0]).(*pb.Warrior)
		hero.Id = proto.String(heroID)
		hero.Guid = proto.String(fmt.Sprintf("mysql-chat-%d-%s", id, heroID))
		g.Warriors = append(g.Warriors, hero)
		if largeRoster {
			for i := 0; i < 400; i++ {
				copy := proto.Clone(g.Warriors[0]).(*pb.Warrior)
				copy.Guid = proto.String(fmt.Sprintf("mysql-chat-%d-roster-%04d", id, i))
				g.Warriors = append(g.Warriors, copy)
			}
		}
		g.Formation = []*pb.FormationItem{{FormatPos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: hero.Id, Guid: hero.Guid}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestP10ChatAvatarInnoDB requires an explicitly provisioned, task-owned P10 database.
func TestP10ChatAvatarInnoDBCompressedHistoryAndRetry(t *testing.T) {
	name := os.Getenv("HYJS_P10_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p10_chatavatar_") {
		t.Skip("requires a dedicated hyjs_test_p10_chatavatar_ database")
	}
	if os.Getenv("HYJS_P10_MYSQL_HOST") != "127.0.0.1:3306" || !strings.HasPrefix(os.Getenv("HYJS_P10_MYSQL_USER"), "hyjs_t_") {
		t.Fatal("requires the isolated local chat-avatar MySQL allocation and temporary user")
	}
	f, peer, outsider := p10ChannelSetup(t)
	if f.mysql.Database != name || db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("test did not use the designated MySQL database")
	}
	var actual string
	if err := db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != name {
		t.Fatal("connected database does not match the owned allocation", err)
	}
	var engines []struct{ TableName, Engine string }
	if err := db.GetIns().Raw("SELECT TABLE_NAME AS table_name, ENGINE AS engine FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", []string{"profiles", "p10_channel_chats", "p10_social_chats", "p10_social_receipts"}).Scan(&engines).Error; err != nil || len(engines) != 4 {
		t.Fatal("could not confirm the four actual tables", engines, err)
	}
	for _, table := range engines {
		if table.Engine != "InnoDB" {
			t.Fatal("non-InnoDB table in the exercised path", table)
		}
	}
	p10ChannelEnable(t)
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, peer, rb)
	p10SceneConnect(t, f, outsider, rc)
	ra.drain()
	rb.drain()
	rc.drain()
	now := time.Now().Add(-4 * time.Minute)
	oldWorld := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("历史世界甲")}
	oldPrivate := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(peer), Content: proto.String("历史私聊甲")}
	if _, err := f.s.p10ChannelAt(f.ctx, f.id, oldWorld, 101, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, oldPrivate, 102, now.Add(25*time.Second)); err != nil {
		t.Fatal(err)
	}
	counter := &p13ChatHistoryQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	counted := db.GetIns().Session(&gorm.Session{Logger: counter})
	if _, err := f.s.p10ChannelHistory(counted, outsider, constUtil.P10WorldChatType); err != nil {
		t.Fatal(err)
	}
	oneSenderQueries := counter.count
	peerWorld := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("历史世界乙")}
	if _, err := f.s.p10ChannelAt(f.ctx, peer, peerWorld, 103, now.Add(50*time.Second)); err != nil {
		t.Fatal(err)
	}
	var oldPublicRow db.P10ChannelChat
	var oldPrivateRow db.P10SocialChat
	if err := db.GetIns().Where("sender_id=? AND content=?", f.id, oldWorld.GetContent()).First(&oldPublicRow).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("sender_id=? AND content=?", f.id, oldPrivate.GetContent()).First(&oldPrivateRow).Error; err != nil {
		t.Fatal(err)
	}
	oldPublicSnapshot := append([]byte(nil), oldPublicRow.SenderInfo...)
	oldPrivateSnapshot := append([]byte(nil), oldPrivateRow.SenderInfo...)
	p10MySQLSetChatAvatar(t, f, f.id, "hero_020", true)
	p10MySQLSetChatAvatar(t, f, peer, "hero_001", false)
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil || profile.Game == nil {
		t.Fatal("compressed sender profile unavailable", err)
	}
	var game struct {
		Warriors json.RawMessage `json:"warriors"`
	}
	if err := json.Unmarshal([]byte(*profile.Game), &game); err != nil || len(game.Warriors) == 0 || game.Warriors[0] != '{' || !bytes.Contains(game.Warriors, []byte(constUtil.ProfileWarriorsCodec86)) {
		t.Fatal("actual persisted roster did not use the bounded compression codec", err)
	}
	storedGame := *profile.Game
	counter.count = 0
	history, err := f.s.p10ChannelHistory(counted, outsider, constUtil.P10WorldChatType)
	if err != nil || len(history) != 2 || counter.count != oneSenderQueries {
		t.Fatal("world history or SQL count changed with another sender", history, oneSenderQueries, counter.count, err)
	}
	if history[0].GetPlay().GetPlayerId() != f.id || history[0].GetPlay().GetHeroWarId() != "hero_020" || history[1].GetPlay().GetPlayerId() != peer || history[1].GetPlay().GetHeroWarId() != "hero_001" {
		t.Fatal("world history mixed the two senders' first-position avatars", history)
	}
	for _, id := range []int64{f.id, peer} {
		privateHistory := p10Call(t, f, id, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, now).(*pb.SCChatCache)
		if len(privateHistory.GetChatList()) != 1 || privateHistory.ChatList[0].GetPlay().GetPlayerId() != f.id || privateHistory.ChatList[0].GetPlay().GetHeroWarId() != "hero_020" {
			t.Fatal("private history lost the sender avatar", id, privateHistory)
		}
	}
	if privateHistory := p10Call(t, f, outsider, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, now).(*pb.SCChatCache); len(privateHistory.GetChatList()) != 0 {
		t.Fatal("private history leaked to a third player", privateHistory)
	}
	if err := db.GetIns().First(&profile, f.id).Error; err != nil || profile.Game == nil || *profile.Game != storedGame {
		t.Fatal("history read rewrote the compressed profile", err)
	}
	rb.drain()
	rc.drain()
	newWorld := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10WorldChatType), Content: proto.String("新世界甲")}
	newPublicReply, err := f.s.p10ChannelAt(f.ctx, f.id, newWorld, 104, now.Add(100*time.Second))
	if err != nil || newPublicReply[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new world sender avatar", newPublicReply, err)
	}
	for _, rec := range []*p10SceneRecorder{rb, rc} {
		if got := p10ChannelChats(rec.drain()); len(got) != 1 || got[0].GetPlay().GetPlayerId() != f.id || got[0].GetPlay().GetHeroWarId() != "hero_020" {
			t.Fatal("new world push avatar", got)
		}
	}
	newPrivate := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(peer), Content: proto.String("新私聊甲")}
	newPrivateReply, handled, err := f.s.DispatchP10Social(f.ctx, f.id, newPrivate, 105)
	if err != nil || !handled || newPrivateReply[0].(*pb.SCChatTcp).GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new private sender avatar", newPrivateReply, handled, err)
	}
	if got := p10ChannelChats(rb.drain()); len(got) != 1 || got[0].GetPlay().GetHeroWarId() != "hero_020" {
		t.Fatal("new private recipient avatar", got)
	}
	if got := p10ChannelChats(rc.drain()); len(got) != 0 {
		t.Fatal("private push leaked to a third player", got)
	}
	var newPublicRow db.P10ChannelChat
	var newPrivateRow db.P10SocialChat
	if err := db.GetIns().Where("sender_id=? AND content=?", f.id, newWorld.GetContent()).First(&newPublicRow).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("sender_id=? AND content=?", f.id, newPrivate.GetContent()).First(&newPrivateRow).Error; err != nil {
		t.Fatal(err)
	}
	newPublicSnapshot := append([]byte(nil), newPublicRow.SenderInfo...)
	newPrivateSnapshot := append([]byte(nil), newPrivateRow.SenderInfo...)
	var publicReceipt, privateReceipt db.P10SocialReceipt
	if err := db.GetIns().First(&publicReceipt, "player_id=? AND domain=? AND stamp=?", f.id, fmt.Sprintf("channel-chat-%d", constUtil.P10WorldChatType), 104).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&privateReceipt, "player_id=? AND domain=? AND stamp=?", f.id, "private-chat", 105).Error; err != nil {
		t.Fatal(err)
	}
	publicReceiptBytes := append([]byte(nil), publicReceipt.Response...)
	privateReceiptBytes := append([]byte(nil), privateReceipt.Response...)
	p10MySQLSetChatAvatar(t, f, f.id, "hero_043", false)
	retry, err := f.s.p10ChannelAt(f.ctx, f.id, newWorld, 104, time.Now())
	if err != nil || !proto.Equal(retry[0], newPublicReply[0]) || len(p10ChannelChats(rb.drain())) != 0 || len(p10ChannelChats(rc.drain())) != 0 {
		t.Fatal("world retry changed or duplicated its original frame", retry, err)
	}
	privateRetry, handled, err := f.s.DispatchP10Social(f.ctx, f.id, newPrivate, 105)
	if err != nil || !handled || !proto.Equal(privateRetry[0], newPrivateReply[0]) || len(p10ChannelChats(rb.drain())) != 0 {
		t.Fatal("private retry changed or duplicated its original frame", privateRetry, err)
	}
	if err := db.GetIns().First(&publicReceipt, "player_id=? AND domain=? AND stamp=?", f.id, fmt.Sprintf("channel-chat-%d", constUtil.P10WorldChatType), 104).Error; err != nil || !bytes.Equal(publicReceipt.Response, publicReceiptBytes) {
		t.Fatal("world retry changed the persisted receipt", err)
	}
	if err := db.GetIns().First(&privateReceipt, "player_id=? AND domain=? AND stamp=?", f.id, "private-chat", 105).Error; err != nil || !bytes.Equal(privateReceipt.Response, privateReceiptBytes) {
		t.Fatal("private retry changed the persisted receipt", err)
	}
	for _, entry := range []struct {
		row  any
		id   int64
		want []byte
	}{
		{&oldPublicRow, oldPublicRow.ID, oldPublicSnapshot}, {&newPublicRow, newPublicRow.ID, newPublicSnapshot},
		{&oldPrivateRow, oldPrivateRow.ID, oldPrivateSnapshot}, {&newPrivateRow, newPrivateRow.ID, newPrivateSnapshot},
	} {
		if err := db.GetIns().First(entry.row, entry.id).Error; err != nil {
			t.Fatal(err)
		}
		var got []byte
		switch row := entry.row.(type) {
		case *db.P10ChannelChat:
			got = row.SenderInfo
		case *db.P10SocialChat:
			got = row.SenderInfo
		}
		if !bytes.Equal(got, entry.want) {
			t.Fatal("history or retry changed the persisted sender snapshot")
		}
	}
	if p1Count(t, &db.P10ChannelChat{}, "sender_id=?", f.id) != 2 || p1Count(t, &db.P10SocialChat{}, "sender_id=?", f.id) != 2 {
		t.Fatal("retry created an extra message")
	}
	history, err = f.s.p10ChannelHistory(db.GetIns(), outsider, constUtil.P10WorldChatType)
	if err != nil || len(history) != 3 || history[0].GetPlay().GetHeroWarId() != "hero_043" || history[1].GetPlay().GetHeroWarId() != "hero_001" || history[2].GetPlay().GetHeroWarId() != "hero_043" {
		t.Fatal("current avatar was not isolated per sender after formation change", history, err)
	}
}
