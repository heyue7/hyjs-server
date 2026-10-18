package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10Setup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p10.db")}
	if name := os.Getenv("HYJS_P10_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p10_") {
			t.Fatal("P10 isolation guard")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P10_MYSQL_HOST"), User: os.Getenv("HYJS_P10_MYSQL_USER"), Password: os.Getenv("HYJS_P10_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10SocialPolicy{}, &db.P10SocialRelation{}, &db.P10SocialApply{}, &db.P10SocialBlack{}, &db.P10SocialChat{}, &db.P10SocialChatState{}, &db.P10SocialReceipt{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10SocialPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for i, name := range []string{"社交甲", "社交乙", "社交丙"} {
		a, err := LoginAccount(f.ctx, fmt.Sprintf("p10-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "local-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, a.Id, name, "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, a.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(20); return nil }); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.Id)
	}
	f.id = ids[0]
	t.Cleanup(func() { d, _ := db.GetIns().DB(); d.Close() })
	return f, ids[1], ids[2]
}
func p10Call(t *testing.T, f *p1Fixture, id int64, req proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	r, _, err := f.s.p10Social(f.ctx, id, req, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return r[0]
}
func TestP10RealFriendApplyApproveBlacklistAndRestart(t *testing.T) {
	f, b, c := p10Setup(t)
	now := time.Now()
	a := f.id
	search := p10Call(t, f, a, &pb.CSSearchPlayer{IdOrName: proto.String(fmt.Sprint(b))}, 0, now).(*pb.SCSearchPlayer)
	var matched *pb.SimplePlayerInfo
	for _, info := range search.GetPlayer() {
		if info.GetPlayerId() == b {
			matched = info
			break
		}
	}
	if matched == nil || matched.GetHeroWarId() != "hero_047" || matched.GetFightPower() <= 0 {
		t.Fatal("actual visible profile search", search)
	}
	apply := &pb.CSApplyFriend{FriendId: proto.Int64(b), Word: proto.String("一起冒险")}
	p10Call(t, f, a, apply, 1, now)
	p10Call(t, f, a, apply, 2, now.Add(time.Hour))
	incoming := p10Call(t, f, b, &pb.CSFriendApplyList{}, 0, now).(*pb.SCFriendApplyList)
	if len(incoming.GetApplyList()) != 1 || incoming.GetApplyList()[0].GetWord() != "一起冒险" || incoming.GetApplyList()[0].GetApplyTime() != now.UnixMilli() {
		t.Fatal("duplicate application changed original", incoming)
	}
	if _, _, err := f.s.p10Social(f.ctx, c, &pb.CSAgreeApply{FriendId: proto.Int64(a)}, 1, now); err == nil {
		t.Fatal("foreign approve")
	}
	p10Call(t, f, b, &pb.CSAgreeApply{FriendId: proto.Int64(a)}, 1, now)
	for _, pair := range [][2]int64{{a, b}, {b, a}} {
		list := p10Call(t, f, pair[0], &pb.CSFriendList{}, 0, now).(*pb.SCFriendList)
		if len(list.GetFriendList()) != 1 || list.GetFriendList()[0].GetSimplePlayerInfo().GetPlayerId() != pair[1] {
			t.Fatal("asymmetric friends")
		}
	}
	f.restart(t)
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	p10Call(t, f, b, &pb.CSSendToBlackList{FriendId: proto.Int64(a)}, 1, now)
	for _, id := range []int64{a, b} {
		if len(p10Call(t, f, id, &pb.CSFriendList{}, 0, now).(*pb.SCFriendList).GetFriendList()) != 0 {
			t.Fatal("black did not unlink")
		}
	}
	for _, pair := range [][2]int64{{a, b}, {b, a}} {
		if _, _, err := f.s.p10Social(f.ctx, pair[0], &pb.CSApplyFriend{FriendId: proto.Int64(pair[1])}, 3, now); err == nil {
			t.Fatal("black relationship bypass")
		}
	}
	if len(p10Call(t, f, b, &pb.CSFriendBlackList{}, 0, now).(*pb.SCFriendBlackList).GetBlackList()) != 1 {
		t.Fatal("owner blacklist")
	}
	if len(p10Call(t, f, a, &pb.CSFriendBlackList{}, 0, now).(*pb.SCFriendBlackList).GetBlackList()) != 0 {
		t.Fatal("foreign blacklist visibility")
	}
	p10Call(t, f, b, &pb.CSDeleteFromBlackList{FriendId: proto.Int64(a)}, 1, now)
	p10Call(t, f, a, &pb.CSApplyFriend{FriendId: proto.Int64(b)}, 4, now)
	p10Call(t, f, b, &pb.CSDenyApply{FriendId: proto.Int64(a)}, 1, now)
	if p1Count(t, &db.P10SocialRelation{}, "low_id=? OR high_id=?", a, a) != 0 || p1Count(t, &db.P10SocialApply{}, "sender_id=?", a) != 0 {
		t.Fatal("unblack restored friend or deny failed")
	}
}
func TestP10ConcurrentOppositeApprovalAtomicSingleRelationship(t *testing.T) {
	f, b, _ := p10Setup(t)
	a := f.id
	now := time.Now()
	p10Call(t, f, a, &pb.CSApplyFriend{FriendId: proto.Int64(b)}, 1, now)
	p10Call(t, f, b, &pb.CSApplyFriend{FriendId: proto.Int64(a)}, 1, now)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, target := a, b
			if i%2 == 1 {
				id, target = b, a
			}
			_, _, err := f.s.p10Social(f.ctx, id, &pb.CSAgreeApply{FriendId: proto.Int64(target)}, 2, now)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.P10SocialRelation{}, "low_id=? OR high_id=?", a, a) != 1 || p1Count(t, &db.P10SocialApply{}, "sender_id=? OR receiver_id=?", a, a) != 0 {
		t.Fatal("concurrent relation duplicates")
	}
	p10Call(t, f, a, &pb.CSDeleteFriend{FriendId: proto.Int64(b)}, 1, now)
	p10Call(t, f, a, &pb.CSDeleteFriend{FriendId: proto.Int64(b)}, 1, now)
	p10Call(t, f, b, &pb.CSDeleteFriend{FriendId: proto.Int64(a)}, 1, now)
	if p1Count(t, &db.P10SocialRelation{}, "low_id=? OR high_id=?", a, a) != 0 {
		t.Fatal("duplicate delete restored relation")
	}
}
func TestP10PrivateChatOfflineHistoryPermissionsAndExactReceipt(t *testing.T) {
	f, b, c := p10Setup(t)
	a := f.id
	now := time.Now()
	req := &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(b), Content: proto.String("原私聊内容")}
	first := p10Call(t, f, a, req, 1, now).(*pb.SCChatTcp)
	if first.GetPlay().GetPlayerId() != a || first.GetTime() != now.UnixMilli() {
		t.Fatal("sender/time spoof")
	}
	if !proto.Equal(first, p10Call(t, f, a, req, 1, now.Add(time.Hour))) {
		t.Fatal("retry changed frozen chat")
	}
	changed := proto.Clone(req).(*pb.CSChatTcp)
	changed.Content = proto.String("篡改")
	if _, _, err := f.s.p10Social(f.ctx, a, changed, 1, now); err == nil {
		t.Fatal("timestamp content collision")
	}
	if _, _, err := f.s.p10Social(f.ctx, a, req, 2, now.Add(time.Second)); err == nil {
		t.Fatal("original interval bypass")
	}
	for _, id := range []int64{a, b} {
		list := p10Call(t, f, id, &pb.CSChatCache{ChatType: proto.Int32(3)}, 0, now).(*pb.SCChatCache)
		if len(list.GetChatList()) != 1 || list.GetChatList()[0].GetContent() != req.GetContent() {
			t.Fatal("offline participant history")
		}
	}
	if len(p10Call(t, f, c, &pb.CSChatCache{ChatType: proto.Int32(3)}, 0, now).(*pb.SCChatCache).GetChatList()) != 0 {
		t.Fatal("private message leaked")
	}
	f.restart(t)
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(p10Call(t, f, b, &pb.CSChatCache{ChatType: proto.Int32(3)}, 0, now).(*pb.SCChatCache).GetChatList()) != 1 {
		t.Fatal("history lost after restart")
	}
	p10Call(t, f, b, &pb.CSSendToBlackList{FriendId: proto.Int64(a)}, 1, now)
	if _, _, err := f.s.p10Social(f.ctx, a, req, 3, now.Add(time.Minute)); err == nil {
		t.Fatal("black chat bypass")
	}
	if len(p10Call(t, f, b, &pb.CSChatCache{ChatType: proto.Int32(3)}, 0, now).(*pb.SCChatCache).GetChatList()) != 0 {
		t.Fatal("blocked history visible")
	}
	if p1Count(t, &db.P10SocialChat{}, "sender_id=?", a) != 1 {
		t.Fatal("retry/failure inserted message")
	}
}
func TestP10PrivateRealtimeSessionRotationAndRetryOnePush(t *testing.T) {
	f, b, _ := p10Setup(t)
	var receiver db.Profile
	if err := db.GetIns().First(&receiver, b).Error; err != nil {
		t.Fatal(err)
	}
	pushed := []proto.Message{}
	closeOld := f.s.RegisterP10Client(f.ctx, b, receiver.Session, func(m proto.Message) error { pushed = append(pushed, m); return nil })
	defer closeOld()
	req := &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(b), Content: proto.String("实时消息")}
	for i := 0; i < 2; i++ {
		if _, handled, err := f.s.DispatchP10Social(f.ctx, f.id, req, 1); !handled || err != nil {
			t.Fatal(err)
		}
	}
	if len(pushed) != 1 {
		t.Fatal("retry duplicate push", len(pushed))
	}
	if _, err := LoginAccount(f.ctx, receiver.Uid, "local-test-password"); err != nil {
		t.Fatal(err)
	}
	f.s.publishP10Private(f.ctx, b, pushed[0])
	if len(pushed) != 1 {
		t.Fatal("old socket received after login")
	}
	var fresh db.Profile
	if err := db.GetIns().First(&fresh, b).Error; err != nil {
		t.Fatal(err)
	}
	closeNew := f.s.RegisterP10Client(f.ctx, b, fresh.Session, func(m proto.Message) error { pushed = append(pushed, m); return nil })
	defer closeNew()
	closeOld()
	f.s.publishP10Private(f.ctx, b, pushed[0])
	if len(pushed) != 2 {
		t.Fatal("old unregister removed new session")
	}
}
func TestP10PermissionLimitsAndFailureNoPartialState(t *testing.T) {
	f, b, _ := p10Setup(t)
	now := time.Now()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(1); return nil }); err != nil {
		t.Fatal(err)
	}
	req := &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(b), Content: proto.String("hello")}
	if _, _, err := f.s.p10Social(f.ctx, f.id, req, 1, now); err == nil {
		t.Fatal("low level chat")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Vip = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	req.Content = proto.String(strings.Repeat("😀", 21))
	if _, _, err := f.s.p10Social(f.ctx, f.id, req, 1, now); err == nil {
		t.Fatal("native UTF16 length bypass")
	}
	req.Content = proto.String(strings.Repeat("😀", 20))
	p10Call(t, f, f.id, req, 1, now)
	if _, _, err := f.s.p10Social(f.ctx, f.id, &pb.CSApplyFriend{FriendId: proto.Int64(f.id)}, 1, now); err == nil {
		t.Fatal("self relationship")
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, &pb.CSApplyFriend{FriendId: proto.Int64(99999999)}, 1, now); err == nil {
		t.Fatal("missing target")
	}
	if p1Count(t, &db.P10SocialApply{}, "sender_id=?", f.id) != 0 || p1Count(t, &db.P10SocialChat{}, "sender_id=?", f.id) != 1 {
		t.Fatal("failed operation left partial state")
	}
}

func TestP10OriginalTimestampFreeTCPRetryOnlyWithinThrottleWindow(t *testing.T) {
	f, b, _ := p10Setup(t)
	now := time.Now()
	req := &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(b), Content: proto.String("原TCP没有重试时间戳")}
	first := p10Call(t, f, f.id, req, 0, now)
	if !proto.Equal(first, p10Call(t, f, f.id, req, 0, now.Add(time.Second))) {
		t.Fatal("TCP retry changed frame")
	}
	changed := proto.Clone(req).(*pb.CSChatTcp)
	changed.Content = proto.String("新内容")
	if _, _, err := f.s.p10Social(f.ctx, f.id, changed, 0, now.Add(time.Second)); err == nil {
		t.Fatal("TCP different payload bypassed interval")
	}
	second := p10Call(t, f, f.id, req, 0, now.Add(20*time.Second))
	if proto.Equal(first, second) || p1Count(t, &db.P10SocialChat{}, "sender_id=?", f.id) != 2 {
		t.Fatal("intentional TCP resend beyond interval was suppressed")
	}
}

func TestP10ClosedPolicyRealLoginQueriesHistoryAndOldReceipt(t *testing.T) {
	f, b, c := p10Setup(t)
	now := time.Now()
	apply := &pb.CSApplyFriend{FriendId: proto.Int64(b), Word: proto.String("旧实际申请")}
	p10Call(t, f, f.id, apply, 1, now)
	p10Call(t, f, b, &pb.CSAgreeApply{FriendId: proto.Int64(f.id)}, 1, now)
	chat := &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(b), Content: proto.String("关闭后仍可查询的旧实际私聊")}
	old := p10Call(t, f, f.id, chat, 1, now)
	if err := db.GetIns().Model(&db.P10SocialPolicy{}).Where("id=?", 1).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if len(p10Call(t, f, f.id, &pb.CSFriendList{}, 0, now).(*pb.SCFriendList).GetFriendList()) != 1 {
		t.Fatal("closed policy hid actual friends")
	}
	for _, request := range []proto.Message{&pb.CSFriendBlackList{}, &pb.CSFriendApplyList{}, &pb.CSRecommendFriendList{}, &pb.CSSearchPlayer{IdOrName: proto.String("社交乙")}} {
		p10Call(t, f, f.id, request, 0, now)
	}
	for _, channel := range []int32{0, 1, 2, 3, 4, 5, 6} {
		history := p10Call(t, f, b, &pb.CSChatCache{ChatType: proto.Int32(channel)}, 0, now).(*pb.SCChatCache)
		if (channel == 0 || channel == 3) && len(history.GetChatList()) != 1 {
			t.Fatal("closed real private history missing", channel)
		}
		if channel != 0 && channel != 3 && len(history.GetChatList()) != 0 {
			t.Fatal("invented unrelated channel history", channel)
		}
	}
	if !proto.Equal(old, p10Call(t, f, f.id, chat, 1, now)) {
		t.Fatal("disabled policy lost exact old receipt")
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, &pb.CSApplyFriend{FriendId: proto.Int64(c)}, 2, now); err == nil {
		t.Fatal("disabled new relationship opened")
	}
	if _, _, err := f.s.p10Social(f.ctx, f.id, chat, 2, now.Add(time.Minute)); err == nil {
		t.Fatal("disabled new private message opened")
	}
	p10ProfileCall(t, f, f.id, b)
}
