package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPlayerRenameOriginalUTF16WidthAndInputIdentity(t *testing.T) {
	for _, name := range []string{"Alice0123456", "六个汉字正好", "ĀĀĀĀĀĀĀĀĀĀĀĀ", "ABC汉字123"} {
		if !playerRenameNameValid(name) {
			t.Fatal("original width rejected", name)
		}
	}
	for _, name := range []string{"", " ", " Alice", "Alice ", "1234567890123", "七个汉字超限了", "āāāāāāā", "甲\n乙", "甲\u200b乙", string([]byte{0xff})} {
		if playerRenameNameValid(name) {
			t.Fatal("invalid original input accepted", name)
		}
	}
}

func renameCall(t *testing.T, f *p1Fixture, name string, stamp int64) *pb.SCChangePlayerName {
	t.Helper()
	out, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String(name)}, stamp)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatal("only original cost callback is legal; extra SCPlayer double-charges", out)
	}
	return out[0].(*pb.SCChangePlayerName)
}
func renameCount(t *testing.T, s *GameService, ctx context.Context, id int64) int32 {
	t.Helper()
	out, err := s.changeNameTimes(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return out[0].(*pb.SCChangeNameTimes).GetChangeNameTimes()
}

func TestPlayerRenameFreePaidConcurrentReceiptCountAndCold(t *testing.T) {
	f := p4Setup(t)
	before, bg := f.profile(t)
	if renameCount(t, f.s, f.ctx, f.id) != 0 {
		t.Fatal("legacy profile has no successful rename")
	}
	free := renameCall(t, f, "初改甲", 1001)
	if len(free.Cost) != 0 || renameCount(t, f.s, f.ctx, f.id) != 1 {
		t.Fatal("original first change must be free", free)
	}
	p, g := f.profile(t)
	if p.GetName() != "初改甲" || p.GetDiamond() != before.GetDiamond() || len(g.Warriors) != len(bg.Warriors) {
		t.Fatal("free change mutated unrelated resources")
	}
	if !proto.Equal(renameCall(t, f, "初改甲", 1001), free) || renameCount(t, f.s, f.ctx, f.id) != 1 {
		t.Fatal("free retry counted twice")
	}
	if _, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String("二改甲")}, 1002); err == nil {
		t.Fatal("paid rename ignored actual 300-diamond cost")
	}
	p, _ = f.profile(t)
	if p.GetName() != "初改甲" || renameCount(t, f.s, f.ctx, f.id) != 1 {
		t.Fatal("failed charge altered name/count")
	}
	// Finite 300-diamond balance isolates the unchanged original paid consumer.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(300); return nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	replies := make(chan proto.Message, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String("二改甲")}, 1002)
			if err != nil {
				failures <- err
				return
			}
			if len(out) != 1 {
				failures <- fmt.Errorf("extra native wallet frame")
				return
			}
			replies <- out[0]
		}()
	}
	wg.Wait()
	close(replies)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var paid proto.Message
	for out := range replies {
		if paid == nil {
			paid = out
		}
		if !proto.Equal(out, paid) {
			t.Fatal("concurrent paid receipt differs")
		}
	}
	if paid == nil || len(paid.(*pb.SCChangePlayerName).Cost) != 1 || paid.(*pb.SCChangePlayerName).Cost[0].GetNum() != 300 {
		t.Fatal("original EditNameCost changed", paid)
	}
	p, _ = f.profile(t)
	if p.GetName() != "二改甲" || p.GetDiamond() != 0 || renameCount(t, f.s, f.ctx, f.id) != 2 {
		t.Fatal("paid rename count/resource conservation")
	}
	var spent int64
	if err := db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent).Select("COALESCE(SUM(amount),0)").Scan(&spent).Error; err != nil || spent != 300 {
		t.Fatal("actual P1 cost must be recorded once", spent, err)
	}
	for _, v := range []struct {
		name  string
		stamp int64
	}{{"初改甲", 1001}, {"篡改甲", 1002}, {"二改甲", 1003}, {" 三改甲", 1004}} {
		if _, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String(v.name)}, v.stamp); err == nil {
			t.Fatal("old/conflicting/noop/invalid rename accepted", v)
		}
	}
	f.restart(t)
	if renameCount(t, f.s, f.ctx, f.id) != 2 || !proto.Equal(renameCall(t, f, "二改甲", 1002), paid) {
		t.Fatal("cold lost count/once receipt")
	}
	p, _ = f.profile(t)
	if p.GetDiamond() != 0 || p.GetName() != "二改甲" {
		t.Fatal("cold replay reverted name or charged")
	}
}

// Existing Create allows duplicate display names. The recovered UI does not
// establish a different rename rule; real IDs continue to identify the owners.
func TestPlayerRenameDuplicateDisplayNamesKeepOwnerIsolation(t *testing.T) {
	f := p4Setup(t)
	other, err := LoginAccount(f.ctx, fmt.Sprintf("rename-other-%d", time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "已有昵称", "hero_047"); err != nil {
		t.Fatal(err)
	}
	renameCall(t, f, "已有昵称", 3001)
	if renameCount(t, f.s, f.ctx, f.id) != 1 || renameCount(t, f.s, f.ctx, other.Id) != 0 {
		t.Fatal("display name merged different owners' counters")
	}
	out, err := f.s.changePlayerName(f.ctx, other.Id, &pb.CSChangePlayerName{PlayerName: proto.String("另一个昵称")}, 3002)
	if err != nil || len(out) != 1 || len(out[0].(*pb.SCChangePlayerName).Cost) != 0 {
		t.Fatal("other owner's actual first rename was not free", err)
	}
	if renameCount(t, f.s, f.ctx, f.id) != 1 || renameCount(t, f.s, f.ctx, other.Id) != 1 {
		t.Fatal("per-owner counter isolation")
	}
	p, _ := f.profile(t)
	if p.GetName() != "已有昵称" {
		t.Fatal("other owner rename changed this player")
	}
}

func TestPlayerRenameCorruptCountAndMissingInitializationFailClosed(t *testing.T) {
	f := p4Setup(t)
	renameCall(t, f, "回执甲", 4001)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, constUtil.PlayerRenameStateKey, `{"count":0}`)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.changeNameTimes(f.ctx, f.id); err == nil {
		t.Fatal("damaged state invented another first-free change")
	}
	if _, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String("回执乙")}, 4002); err == nil {
		t.Fatal("damaged count accepted")
	}
	f.s.P1Ready = false
	if _, err := f.s.changePlayerName(f.ctx, f.id, &pb.CSChangePlayerName{PlayerName: proto.String("回执丙")}, 4003); err == nil {
		t.Fatal("uninitialized reward accounting accepted a paid-name operation")
	}
}
