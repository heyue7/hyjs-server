package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
)

// Two real fixture profiles intentionally share PID1 across realms; the actual
// certified winner updates exactly its own original UI entry, preserving flags.
func TestP14FinalScoreActualCertifiedHistoryJointIdentity(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("two-file fixture")
	}
	a, b, series, manifest, _ := p14ContinuationFixture(t)
	p14PerGameSwitch(t, a)
	if a.id != b.id {
		t.Fatal("joint collision premise")
	}
	var current db.P14FinalSeries
	if err := db.GetIns().First(&current, "id=?", series.ID).Error; err != nil {
		t.Fatal(err)
	}
	winners, _, err := p14ContinuationHistoryTX(db.GetIns(), &current, manifest, 1)
	if err != nil {
		t.Fatal(err)
	}
	left, right, _, _, err := p14FinalSeriesProgress(winners)
	if err != nil {
		t.Fatal(err)
	}
	list := &pb.KFMatchAllRandList{}
	for _, server := range []int32{1, 2, 3} {
		list.KFMatchFirstList = append(list.KFMatchFirstList, &pb.KFMatchFightRandList{ServerId: proto.Int32(server), PlayerId: proto.Int64(a.id), PromotionInfo: proto.Int32(0), WinTime: proto.Int32(0)})
	}
	if err = p14FinalScoreApplyPair(list, manifest.Pair, left, right, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if list.KFMatchFirstList[0].GetWinTime() != left || list.KFMatchFirstList[1].GetWinTime() != right || list.KFMatchFirstList[2].GetWinTime() != 0 {
		t.Fatal("pid-only projection lost realm")
	}
	for _, entry := range list.KFMatchFirstList {
		if entry.GetPromotionInfo() != 0 {
			t.Fatal("invented bracket promotion")
		}
	}
}
