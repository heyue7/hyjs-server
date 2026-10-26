package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// TestP4CopyBenefitsIntegratedSweep 在已应用stage3补丁的隔离副本里验证真实扫荡保存与回执。
func TestP4CopyBenefitsIntegratedSweep(t *testing.T) {
	f, _, now := p4CopySetup(t)
	if err := f.s.InitCooking(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(14)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5), ChallengeTimes: proto.Int32(0)}}
		refreshCopySweepDay(g, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	first, err := f.s.copySweepAt(f.ctx, f.id, request, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	result := first[0].(*pb.SCCopySweep)
	if len(result.GetGain().GetSimps()) != 2 || result.GetGain().GetSimps()[0].GetNum() != 4380 || result.GetGain().GetSimps()[1].GetNum() != 310 {
		t.Fatal("actual sweep not doubled", result)
	}
	p, g := f.profile(t)
	if p.GetCoin() != 4380 || p.GetStamina() != 110 || p.GetLevel() != 15 || p.GetExp() != 30 || g.Copy.GetPassedCopyInfo()[0].GetChallengeTimes() != 1 {
		t.Fatal("sweep reward did not persist", p, g.Copy)
	}
	for _, at := range []int{0, 2} {
		again, err := f.s.copySweepAt(f.ctx, f.id, request, 100, now.AddDate(0, at, 0))
		if err != nil || !proto.Equal(first[0], again[0]) {
			t.Fatal("original sweep receipt changed", err)
		}
	}
	after, _ := f.profile(t)
	if after.GetCoin() != 4380 || after.GetExp() != 30 {
		t.Fatal("sweep retry amplified rewards")
	}
}

// TestP4CopyBenefitsIntegratedFirstDropRetainsFightPointers 验证额外奖后仍保存同一战斗结算对象。
func TestP4CopyBenefitsIntegratedFirstDropRetainsFightPointers(t *testing.T) {
	f, _, _ := p4CopySetup(t)
	if err := f.s.InitCooking(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.copyEnter(f.ctx, f.id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
		t.Fatal(err)
	}
	start, err := f.s.copyFightStart(f.ctx, f.id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")})
	if err != nil {
		t.Fatal(err)
	}
	fight := start[0].(*pb.SCCopyFightStart)
	request := &pb.CSCopyFightEnd{FightId: fight.FightId, Result: proto.Int32(1), BattlePlayer: fight.BattlePlayer}
	first, err := f.s.copyFightEnd(f.ctx, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	result := first[0].(*pb.SCCopyFightEnd)
	if len(result.GetGain().GetWars()) != 2 || result.GetGain().GetWars()[0].GetGuid() == result.GetGain().GetWars()[1].GetGuid() {
		t.Fatal("confirmed first drop missing independent bonus instance")
	}
	_, game := f.profile(t)
	if game.Run == nil || game.Run.Fight == nil || !game.Run.Fight.Settled || game.Run.Fight.Result == nil {
		t.Fatal("public reward replaced caller fight pointer")
	}
	again, err := f.s.copyFightEnd(f.ctx, f.id, request)
	if err != nil || !proto.Equal(first[0], again[0]) {
		t.Fatal("first drop settlement replay changed", err)
	}
	_, game = f.profile(t)
	if len(game.Warriors) != 3 {
		t.Fatal("first drop replay granted again")
	}
}
