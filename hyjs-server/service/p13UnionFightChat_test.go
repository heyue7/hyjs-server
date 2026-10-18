package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightChatActual 使用原实际名册/阵营/会话，验证交流闭环和撤权，不产生胜负。
func TestP13UnionFightChatActualText18OrdersEightRetriesPrivateColdMembership(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightChat{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离原聊天初始化；原固定复活点/明确非权威HP"}).Error; err != nil {
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
	received := map[int64][]proto.Message{}
	var mu sync.Mutex
	for _, id := range []int64{ids[0], ids[10], ids[11], enemy.PlayerID} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		playerID := id
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, profile.Session, func(m proto.Message) error {
			mu.Lock()
			defer mu.Unlock()
			received[playerID] = append(received[playerID], proto.Clone(m))
			return nil
		}))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	received = map[int64][]proto.Message{}
	text := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P13UnionFightChatType), Content: proto.String("真实资源上交后守护工厂")}
	if _, err := f.s.p13UFChat(f.ctx, ids[0], text, 8100, now); err == nil {
		t.Fatal("closed text awarded")
	}
	if err := db.GetIns().Model(&db.P13UnionFightChatPolicy{}).Where("id=1").Updates(map[string]any{"text_enabled": true, "talk_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := f.s.p13UFChat(f.ctx, ids[0], text, 8100, now)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].(*pb.SCChatTcp).GetPlay().GetPlayerId() != ids[0] || len(received[ids[10]]) != 1 || len(received[enemy.PlayerID]) != 0 {
		t.Fatal("actual identity or guild privacy", result, received)
	}
	if f.mysql.Database != "" {
		var workers sync.WaitGroup
		failures := make(chan error, 8)
		for i := 0; i < 8; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				reply, err := f.s.p13UFChat(f.ctx, ids[0], text, 8100, now)
				if err == nil && !proto.Equal(reply[0], result[0]) {
					t.Error("receipt changed")
				}
				failures <- err
			}()
		}
		workers.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(received[ids[10]]) != 1 {
		t.Fatal("repeated pushes")
	}
	if _, err = f.s.p13UFChat(f.ctx, ids[0], text, 0, now); err != nil {
		t.Fatal("tcp retry", err)
	}
	conflict := proto.Clone(text).(*pb.CSChatTcp)
	conflict.Content = proto.String("另一个消息")
	if _, err = f.s.p13UFChat(f.ctx, ids[0], conflict, 8100, now); err == nil {
		t.Fatal("same stamp changed text")
	}
	if _, err = f.s.p13UFChat(f.ctx, ids[0], conflict, 8101, now); err == nil {
		t.Fatal("20s rate bypass")
	}
	for _, content := range []string{" ", strings.Repeat("汉", 41), "bad\x00text", string([]byte{255})} {
		body := proto.Clone(text).(*pb.CSChatTcp)
		body.Content = proto.String(content)
		if _, err = f.s.p13UFChat(f.ctx, ids[10], body, 8200, now); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
	history, err := f.s.p13UFChatHistory(f.ctx, ids[10], now)
	if err != nil || len(history[0].(*pb.SCUFChatCache).ChatList) != 1 {
		t.Fatal("actual history", err, history)
	}
	history, err = f.s.p13UFChatHistory(f.ctx, enemy.PlayerID, now)
	if err != nil || len(history[0].(*pb.SCUFChatCache).ChatList) != 0 {
		t.Fatal("enemy history leaked", err)
	}
	if len(f.s.Tables["UnionFightTalk"]) != 18 {
		t.Fatal("missing original18")
	}
	for i, row := range f.s.Tables["UnionFightTalk"] {
		content := row["localizedContent"]
		if content == "" {
			t.Fatal("missing original localized command", row)
		}
		order := &pb.CSUFTalk{Content: proto.String(content)}
		at := now.Add(time.Duration(i) * 20 * time.Second)
		// 准备窗口允许交流，原场结束后拒绝；测试最多18条持续340秒。
		if i == 15 {
			match.EndAt += 600000
			if err = db.GetIns().Model(match).Update("end_at", match.EndAt).Error; err != nil {
				t.Fatal(err)
			}
		}
		if i == 0 {
			if _, err = f.s.p13UFChat(f.ctx, ids[10], order, 8300, at); err == nil {
				t.Fatal("ordinary member order")
			}
			if _, err = f.s.p13UFFieldHook(f.ctx, ids[0], &pb.CSUFOnHook{OnHook: proto.Int32(1)}, at); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.p13UFChat(f.ctx, ids[0], order, 8300, at); err == nil {
				t.Fatal("hook order")
			}
			if _, err = f.s.p13UFFieldHook(f.ctx, ids[0], &pb.CSUFOnHook{OnHook: proto.Int32(0)}, at); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = f.s.p13UFChat(f.ctx, ids[11], order, 8400+int64(i), at); err != nil {
			t.Fatal("actual vice original order", i, err)
		}
	}
	if _, err = f.s.p13UFChat(f.ctx, ids[0], &pb.CSUFTalk{Content: proto.String("伪造城门战果")}, 8500, now); err == nil {
		t.Fatal("arbitrary order accepted")
	}
	if err = db.GetIns().Model(&db.P13UnionFightChatPolicy{}).Where("id=1").Update("text_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFChat(f.ctx, ids[0], text, 8100, now); err != nil {
		t.Fatal("cold gate-off receipt", err)
	}
	history, err = f.s.p13UFChatHistory(f.ctx, ids[10], now)
	if err != nil || len(history[0].(*pb.SCUFChatCache).ChatList) != 1 {
		t.Fatal("cold text history", err)
	}
	if _, err = f.s.p13UFChatHistory(f.ctx, ids[0], time.UnixMilli(match.EndAt)); err == nil {
		t.Fatal("ended match history permission retained")
	}
	if err = db.GetIns().Model(&db.P13UnionFightChat{}).Where("sender_id=? AND kind=0", ids[0]).Update("frame_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFChatHistory(f.ctx, ids[10], now); err == nil {
		t.Fatal("corrupt history accepted")
	}
	if err = db.GetIns().Where("player_id=?", ids[10]).Delete(&db.P13GuildMember{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFChatHistory(f.ctx, ids[10], now); err == nil {
		t.Fatal("ex-member history accepted")
	}
}
