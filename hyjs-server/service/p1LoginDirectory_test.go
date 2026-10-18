package service

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p1DirectoryInfo(t *testing.T, f *p1Fixture) *pb.SCActivityInfo {
	t.Helper()
	replies, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.P1LoginDirectoryID)})
	if err != nil {
		t.Fatal(err)
	}
	return replies[0].(*pb.SCActivityInfo)
}

func TestP1LoginDirectoryActualGoalsCrossEntryOnceAndCold(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	f.event(t, "login:"+p1Day(now.AddDate(0, 0, -1)), constUtil.P1EventLoginDay, 1, now.AddDate(0, 0, -1))
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	facts := p1Count(t, &db.EventFact{}, "player_id = ?", f.id)
	tabs, err := f.s.P1ActivityTabs(f.ctx, f.id)
	if err != nil || len(tabs) != 1 || tabs[0].GetActivityId() != constUtil.P1LoginDirectoryID || tabs[0].GetStatus() != 1 {
		t.Fatalf("directory tabs: %v / %v", tabs, err)
	}
	info := p1DirectoryInfo(t, f)
	if len(info.GetItemList()) != 2 {
		t.Fatal("original Type4 lost a child reward")
	}
	one, three := info.ItemList[0], info.ItemList[1]
	if one.GetValue() != "1" || one.GetTotal() != 1 || one.GetStatus() != 1 || three.GetValue() != "3" || three.GetTotal() != 3 || three.GetLeft() != 2 || three.GetStatus() != 0 || one.GetIndex() == three.GetIndex() {
		t.Fatalf("actual goals: %v", info.ItemList)
	}
	if !strings.Contains(one.GetMessage(), "·1天·领至") || !strings.Contains(three.GetMessage(), "·3天·领至") {
		t.Fatal("shared tab lost each child's claim deadline", info.ItemList)
	}
	if p1Count(t, &db.EventFact{}, "player_id = ?", f.id) != facts {
		t.Fatal("directory fabricated facts")
	}
	before, _ := f.profile(t)
	for _, bad := range []*pb.CSActivityReward{
		{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Index: three.Index, GetResources: three.RewardList},
		{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Index: one.Index, GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(1000)}}},
	} {
		if _, err := f.s.P1Activity(f.ctx, f.id, bad); err == nil {
			t.Fatal("unqualified or forged option granted")
		}
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("rejected request changed wallet")
	}
	var member db.P1LoginDirectoryMember
	if err := db.GetIns().Where("index_id = ?", one.GetIndex()).First(&member).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Index: proto.Int32(one.GetIndex()), GetResources: one.RewardList})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	player, _ := f.profile(t)
	if player.GetCoin() != 1000 {
		t.Fatal("duplicate directory reward")
	}
	old, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(member.InstanceID), Index: proto.Int32(1), GetResources: one.RewardList})
	if err != nil || len(old[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 0 {
		t.Fatal("direct route duplicated delivery")
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", member.InstanceID).Updates(map[string]any{"goal": 99, "rewards": "diamond,99"}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	cold := p1DirectoryInfo(t, f)
	if cold.ItemList[0].GetIndex() != one.GetIndex() || cold.ItemList[1].GetIndex() != three.GetIndex() || cold.ItemList[0].GetValue() != "1" || cold.ItemList[0].GetStatus() != 2 {
		t.Fatal("cold or hot config shifted frozen child")
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id = ? AND source_key = ?", f.id, "activity:"+member.InstanceID) != 1 {
		t.Fatal("new directory entitlement")
	}
}

func TestP1LoginDirectoryRemovedChildCannotShiftIndex(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	first := p1DirectoryInfo(t, f)
	old := first.ItemList[0]
	var member db.P1LoginDirectoryMember
	if err := db.GetIns().Where("index_id = ?", old.GetIndex()).First(&member).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", member.InstanceID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	p1Instance(t, f, "totalLogin_p1_new_month", constUtil.P1EventLoginDay, "cumulative", 2, time.Now().Add(time.Hour), 0)
	second := p1DirectoryInfo(t, f)
	for _, item := range second.ItemList {
		if item.GetIndex() == old.GetIndex() {
			t.Fatal("disabled index reassigned")
		}
	}
	before, _ := f.profile(t)
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Index: proto.Int32(old.GetIndex()), GetResources: old.RewardList}); err == nil {
		t.Fatal("disabled child granted")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("rejected index changed wallet")
	}
	f.restart(t)
	cold := p1DirectoryInfo(t, f)
	for i, item := range cold.ItemList {
		if item.GetIndex() != second.ItemList[i].GetIndex() {
			t.Fatal("cold mapping shifted")
		}
	}
	if p1Count(t, &db.P1LoginDirectoryMember{}, "instance_id = ?", member.InstanceID) != 1 {
		t.Fatal("disabled mapping deleted")
	}
}

func TestP1ActivityGoalValueMatchesRealCopyRequirement(t *testing.T) {
	f := p1Setup(t)
	key := fmt.Sprint("p1:goal_value_", time.Now().UnixNano())
	p1Instance(t, f, key, constUtil.P1EventCopyWin, "cumulative", 10, time.Now().Add(time.Hour), 0)
	replies, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
	if err != nil {
		t.Fatal(err)
	}
	item := replies[0].(*pb.SCActivityInfo).ItemList[0]
	if item.GetValue() != "10" || item.GetTotal() != 10 || item.GetLeft() != 0 {
		t.Fatal("client goal text differs from eligibility")
	}
	if !strings.Contains(replies[0].(*pb.SCActivityInfo).GetInformation(), "主线胜利累计10次") {
		t.Fatal("local copy objective still uses recharge caption")
	}
}
