package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP15KaiGongRealCopyFactsFrozenClaimAndCold(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	key := constUtil.P15KaiGongPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	// Only this finite copy-win material period is opened for the isolated test.
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", key).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	query := func() *pb.SCActivityInfo {
		t.Helper()
		out, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCActivityInfo)
	}
	if query().GetItemList()[0].GetStatus() != 0 || query().GetItemList()[0].GetTotal() != 10 {
		t.Fatal("query fabricated copy wins")
	}
	var row db.PeriodInstance
	if err := db.GetIns().First(&row, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if row.Source != constUtil.P15KaiGongSource {
		t.Fatal("missing original/local boundary")
	}
	for i := 0; i < 9; i++ {
		f.event(t, string(rune('a'+i)), constUtil.P1EventCopyWin, 1, now)
	}
	item := query().GetItemList()[0]
	if item.GetLeft() != 9 || item.GetStatus() != 0 {
		t.Fatal("nine wins claimed ten-win supply")
	}
	request := &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1), GetResources: item.GetRewardList()}
	if _, err := f.s.P1Activity(f.ctx, f.id, request); err == nil {
		t.Fatal("unearned welfare")
	}
	// The same actual business fact is replayed without advancing the activity.
	f.event(t, "a", constUtil.P1EventCopyWin, 1, now)
	if query().GetItemList()[0].GetLeft() != 9 {
		t.Fatal("copy fact replay counted")
	}
	f.event(t, "tenth", constUtil.P1EventCopyWin, 1, now)
	first, err := f.s.P1Activity(f.ctx, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 2 {
		t.Fatal("missing actual original materials")
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_139") != 5 || simpleOwned(p, g, "item_150") != 1 {
		t.Fatal("wrong original supply inventory")
	}
	if err := db.GetIns().Model(&row).Update("rewards", "coin,999999").Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	again, err := f.s.P1Activity(f.ctx, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(again[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 0 {
		t.Fatal("cold claimed reward duplicated")
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_139") != 5 || simpleOwned(p, g, "item_150") != 1 || p.GetCoin() != 0 {
		t.Fatal("cold inventory changed")
	}
	t.Log("native32 generic ActivityInfo path, ten-copy facts, ninth/replay guards, original materials and cold exactly-once passed")
}
