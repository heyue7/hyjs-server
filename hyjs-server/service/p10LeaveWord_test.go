package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP10LeaveWordOriginalAckOnlineOnceAndOfflineColdHistory(t *testing.T) {
	f, b, c := p10Setup(t) // 有限三个真实原角色；沿既有聊天20级前提，不跑新号成长。
	before, game := f.profile(t)
	beforeGame, _ := json.Marshal(game)
	var receiver db.Profile
	if err := db.GetIns().First(&receiver, b).Error; err != nil {
		t.Fatal(err)
	}
	pushes := []proto.Message{}
	closeReceiver := f.s.RegisterP10Client(f.ctx, b, receiver.Session, func(m proto.Message) error { pushes = append(pushes, m); return nil })
	defer closeReceiver()
	req := &pb.CSSendMassageToFriend{PlayerId: proto.Int64(b), Content: proto.String("原界面留言，中文输入")}
	for i := 0; i < 2; i++ {
		messages, handled, err := f.s.DispatchP10Social(f.ctx, f.id, req, 41001)
		if !handled || err != nil || len(messages) != 1 {
			t.Fatal("原24105接线", handled, err)
		}
		if _, ok := messages[0].(*pb.SCSendMessageToFriend); !ok {
			t.Fatal("HTTP未返回原空Ack", messages)
		}
	}
	if len(pushes) != 1 || pushes[0].(*pb.SCChatTcp).GetContent() != req.GetContent() {
		t.Fatal("在线目标投递未一次生效", pushes)
	}
	closeReceiver()
	offline := &pb.CSSendMassageToFriend{PlayerId: proto.Int64(b), Content: proto.String("离线仍保留的留言")}
	if messages, handled, err := f.s.DispatchP10Social(f.ctx, c, offline, 41002); !handled || err != nil || len(messages) != 1 {
		t.Fatal("离线投递", handled, err)
	}
	if len(pushes) != 1 {
		t.Fatal("离线会话仍收到推送")
	}
	f.restart(t)
	messages, handled, err := f.s.DispatchP10Social(f.ctx, f.id, req, 41001)
	if !handled || err != nil || len(messages) != 1 || p1Count(t, &db.P10SocialChat{}, "receiver_id=?", b) != 2 {
		t.Fatal("冷重复留言", err)
	}
	cache, _, err := f.s.p10Social(f.ctx, b, &pb.CSChatCache{ChatType: proto.Int32(constUtil.P10PrivateChatType)}, 0, time.Now())
	if err != nil || len(cache[0].(*pb.SCChatCache).GetChatList()) != 2 {
		t.Fatal("离线留言冷历史缺失", err)
	}
	after, current := f.profile(t)
	afterGame, _ := json.Marshal(current)
	if !proto.Equal(before, after) || string(beforeGame) != string(afterGame) {
		t.Fatal("留言改变了钱包或成长存档")
	}
}

func TestP10LeaveWordSharesBlacklistLevelThrottleAndKeepsSeparateReceiptDomain(t *testing.T) {
	f, b, _ := p10Setup(t)
	now := time.Now()
	req := &pb.CSSendMassageToFriend{PlayerId: proto.Int64(b), Content: proto.String("原HTTP留言")}
	for _, invalid := range []string{"", "  ", strings.Repeat("字", 41), strings.Repeat("😀", 21), "控制\x00字符"} {
		if _, _, err := f.s.p10Social(f.ctx, f.id, &pb.CSSendMassageToFriend{PlayerId: proto.Int64(b), Content: proto.String(invalid)}, 42001, now); err == nil {
			t.Fatal("绕过原40UTF16/文本资格")
		}
	}
	p10Call(t, f, f.id, &pb.CSSendToBlackList{FriendId: proto.Int64(b)}, 1, now)
	if _, _, err := f.s.p10Social(f.ctx, f.id, req, 42001, now); err == nil {
		t.Fatal("留言绕过黑名单")
	}
	p10Call(t, f, f.id, &pb.CSDeleteFromBlackList{FriendId: proto.Int64(b)}, 2, now)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(1)
		p.Vip = proto.Int32(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, req, 42001, now); err == nil {
		t.Fatal("留言绕过聊天等级")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(20); return nil }); err != nil {
		t.Fatal(err)
	}
	first, fresh, err := f.s.p10Social(f.ctx, f.id, req, 42001, now)
	if err != nil || !fresh || len(first) != 2 {
		t.Fatal("真实留言未提交", err)
	}
	chat := &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: proto.Int64(b), Content: proto.String("原TCP私聊")}
	if _, _, err = f.s.p10Social(f.ctx, f.id, chat, 42001, now.Add(time.Second)); err == nil {
		t.Fatal("HTTP留言未共用原聊天节流")
	}
	if _, _, err = f.s.p10Social(f.ctx, f.id, chat, 42001, now.Add(time.Minute)); err != nil {
		t.Fatal("HTTP与TCP回执域冲突", err)
	}
	changed := proto.Clone(req).(*pb.CSSendMassageToFriend)
	changed.Content = proto.String("改写同帧")
	if _, _, err = f.s.p10Social(f.ctx, f.id, changed, 42001, now.Add(time.Minute)); err == nil {
		t.Fatal("同原帧可变更留言")
	}
	if err = db.GetIns().Model(&db.P10SocialPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	replay, fresh, err := f.s.p10Social(f.ctx, f.id, req, 42001, now.Add(2*time.Minute))
	if err != nil || fresh || !proto.Equal(replay[0], first[0]) || len(replay) != 1 {
		t.Fatal("关闭后旧原空Ack不可恢复", err)
	}
	if _, _, err = f.s.p10Social(f.ctx, f.id, req, 42002, now.Add(2*time.Minute)); err == nil {
		t.Fatal("关闭后新留言仍生效")
	}
	if p1Count(t, &db.P10SocialChat{}, "sender_id=?", f.id) != 2 || p1Count(t, &db.P10SocialReceipt{}, "player_id=? AND domain=?", f.id, constUtil.P10LeaveWordReceiptDomain) != 1 {
		t.Fatal("失败/重复留言制造新事实")
	}
}
