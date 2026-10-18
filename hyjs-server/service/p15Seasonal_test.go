package service

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP15SeasonalNativeDirectoryActualSourcesAndFrozenRewards(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	suffix := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	list := func(kind string) *pb.SCActivityList {
		t.Helper()
		out, err := f.s.p15SeasonalList(f.ctx, f.id, &pb.CSActivityList{ActivityId: proto.String(kind + "_p4_" + suffix)}, now)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCActivityList)
	}
	festival := list("festival")
	update := list("update")
	if len(festival.GetActivityList()) != 2 || len(update.GetActivityList()) != 1 || update.GetActivityList()[0].GetActivityId() != "totalLoginUpdate_p4_update_"+suffix {
		t.Fatal("original nested prefix mismatch")
	}
	// A direct query must not manufacture a login or a completion fact.
	child := festival.GetActivityList()[0].GetActivityId()
	info := func(key string) *pb.SCActivityInfo {
		t.Helper()
		out, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCActivityInfo)
	}
	if info(child).GetItemList()[0].GetStatus() != 0 {
		t.Fatal("query fabricated login")
	}
	f.event(t, "real-login:"+p1Day(now), constUtil.P1EventLoginDay, 1, now)
	earned := info(child).GetItemList()[0]
	if earned.GetStatus() != 1 || earned.GetRewardList()[0].GetId() != "item_124" {
		t.Fatal("actual login not linked")
	}
	reward := &pb.CSActivityReward{ActivityId: proto.String(child), Index: proto.Int32(1), GetResources: earned.GetRewardList()}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.P1Activity(f.ctx, f.id, reward)
			errs <- err
			if err == nil {
				replies <- out[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	winners := 0
	for r := range replies {
		if len(r.(*pb.SCActivityReward).GetGet().GetSimps()) > 0 {
			winners++
		}
	}
	if winners != 1 {
		t.Fatal("claim awarded more than once", winners)
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_124") != 15 {
		t.Fatal("login reward duplicated")
	}
	// Frozen parent and child rules preserve their original catalogue and reward.
	if err := db.GetIns().Model(&db.P15SeasonalBundle{}).Where("id=?", "festival_p4_"+suffix).Updates(map[string]any{"children": []byte(`[]`), "title": "changed"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", child).Update("rewards", "coin,999999").Error; err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(list("festival"), festival) || info(child).GetItemList()[0].GetRewardList()[0].GetId() != "item_124" {
		t.Fatal("hot edit changed frozen rules")
	}
	// Finite login-day eligibility facts exercise the three-day instance without changing rewards.
	f.event(t, "historic-day-1", constUtil.P1EventLoginDay, 1, now.Add(-24*time.Hour))
	f.event(t, "historic-day-2", constUtil.P1EventLoginDay, 1, now.Add(-48*time.Hour))
	if info(update.GetActivityList()[0].GetActivityId()).GetItemList()[0].GetStatus() != 1 {
		t.Fatal("three login facts did not unlock update")
	}
	celebration := "celebrationActivity_p4_" + suffix
	celebrationItem := info(celebration).GetItemList()[0]
	if celebrationItem.GetStatus() != 1 {
		t.Fatal("original celebration login fact missing")
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(celebration), Index: proto.Int32(1), GetResources: celebrationItem.GetRewardList()}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(90)
		p.Coin = proto.Int64(1000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	exchange := festival.GetActivityList()[1].GetActivityId()
	query, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(exchange)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	slot := query[0].(*pb.SCActivityShopInfo).GetShopItemList()[0]
	request := &pb.CSBuyActivityShop{ActivityId: proto.String(exchange), Index: slot.Index, Num: proto.Int32(1), CostResources: slot.GetCostResource(), GetResources: slot.GetGetResource()}
	out, err := f.s.p4Commerce(f.ctx, f.id, request, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := f.s.p4Commerce(f.ctx, f.id, request, 1, now)
	if err != nil || !proto.Equal(out[0], repeated[0]) {
		t.Fatal("actual exchange receipt changed", err)
	}
	p, g = f.profile(t)
	if p.GetCoin() != 800000 || simpleOwned(p, g, "item_470") != 10 {
		t.Fatal("natural coin supply did not trade")
	}
	// Parent shutdown applies to direct child commerce and directory lookup.
	if err := db.GetIns().Model(&db.P15SeasonalBundle{}).Where("id=?", "festival_p4_"+suffix).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, request, 2, now); err == nil {
		t.Fatal("closed parent traded child")
	}
	if _, err := f.s.p15SeasonalList(f.ctx, f.id, &pb.CSActivityList{ActivityId: proto.String("festival_p4_" + suffix)}, now); err == nil {
		t.Fatal("closed parent listed children")
	}
	var live db.P15SeasonalBundle
	db.GetIns().First(&live, "id=?", "update_p4_"+suffix)
	// A forged child prefix cannot select a native unsupported subpage.
	live.Frozen = nil
	live.Children, _ = json.Marshal([]p15SeasonalChild{{ID: "fake_p4_update_" + suffix, Title: "forged", Domain: "period"}})
	if _, _, err := p15SeasonalSnapshot(live); err == nil {
		t.Fatal("unsupported Update child accepted")
	}
	t.Log("three original types, native nested prefixes, real event facts, actual transaction, freezing, eight claim replay and parent shutdown passed")
}
