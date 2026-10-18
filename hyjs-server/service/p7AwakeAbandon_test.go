package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// 原End失败后显式退出只结算已开箱；不能把未提交战斗补成胜利或回收付过的次数。
func TestP7AwakeExplicitExitAbandonsPendingAndReplaysEarnedReceipt(t *testing.T) {
	f := p7AwakeBusinessFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	before, bg := f.profile(t)
	enter := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: enter.Boxes[0].Guid}, now)
	start, end := p7AwakeBusinessStartEnd(t, f, enter.Monsters[0].GetGuid(), now)
	replies, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 991001)
	if err != nil {
		t.Fatal(err)
	}
	receipt := replies[0].(*pb.SCAwakeCopySettle)
	if receipt.GetScore() != 5 || len(receipt.ScoreRewards) != 0 || receipt.GetCoinRewards() != 0 {
		t.Fatal("abandon manufactured monster/completion rewards", receipt)
	}
	var fight db.P7AwakeFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil || fight.Status != constUtil.P7AwakeFightAbandoned || len(fight.EndRequest) != 0 || len(fight.Terminal) != 0 {
		t.Fatal("pending was not abandoned without a result", err)
	}
	var ledger db.P7AwakeExploration
	if err = db.GetIns().First(&ledger, "run_id=?", fight.RunID).Error; err != nil || ledger.Status != constUtil.P7AwakeExploreAbandoned || ledger.SettleStamp != 991001 {
		t.Fatal("explicit exit did not retain abandoned receipt", err)
	}
	after, ag := f.profile(t)
	if simpleOwned(after, ag, "item_320")-simpleOwned(before, bg, "item_320") != 1 {
		t.Fatal("earned box material missing or duplicated")
	}
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, end, now); err == nil {
		t.Fatal("abandoned battle later became a victory")
	}
	f.restart(t)
	retry, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 991001)
	if err != nil || !proto.Equal(retry[0], receipt) {
		t.Fatal("cold abandoned receipt failed", err)
	}
	cold, cg := f.profile(t)
	if simpleOwned(cold, cg, "item_320") != simpleOwned(after, ag, "item_320") {
		t.Fatal("cold retry repeated material")
	}
	next := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	if next.Monsters[0].GetGuid() == enter.Monsters[0].GetGuid() {
		t.Fatal("new exploration reused abandoned node identity")
	}
	old, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 991001)
	if err != nil || !proto.Equal(old[0], receipt) {
		t.Fatal("old abandon settled new exploration", err)
	}
	if p1Count(t, &db.P7AwakeExploration{}, "player_id=? AND status=?", f.id, constUtil.P7AwakeExploreActive) != 1 {
		t.Fatal("old abandon affected active successor")
	}
}
