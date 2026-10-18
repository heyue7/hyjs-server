package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"sync"
	"testing"
	"time"
)

func p11OpenLimit(t *testing.T, now time.Time, id string) {
	t.Helper()
	if err := db.GetIns().Model(&db.P11LimitPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "period_id": id, "starts_at": now.Add(-time.Hour).UnixMilli(), "ends_at": now.Add(48 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
}
func p11LimitCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	replies, err := f.s.p11LimitAt(f.ctx, id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return replies[0]
}
func TestP11LimitRealScoreWindowAtomicPurchaseReceiptAndRecovery(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Date(2026, 10, 13, 23, 59, 0, 0, time.FixedZone("UTC+8", 8*3600))
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	if _, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSLimitPvpFindMatch{}, 101, now); err == nil {
		t.Fatal("default period unexpectedly active")
	}
	p11OpenLimit(t, now, "isolated-period-a")
	for _, id := range []int64{b, c} {
		p11LimitCall(t, f, id, &pb.CSLimitPvpFindMatch{}, 101, now)
	}
	query := p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 101, now).(*pb.SCLimitPvpFindMatch)
	if query.GetFirstRewardScore() != 10 || query.GetMyrank().GetPvpIntegral() != 10 || query.GetFightTimes() != 0 || query.GetAllTimes() != 10 || len(query.RankList) != 2 {
		t.Fatal("original free/login counters or real list", query)
	}
	if !proto.Equal(query, p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 101, now)) {
		t.Fatal("query retry changed")
	}
	again := p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 102, now).(*pb.SCLimitPvpFindMatch)
	if again.GetFirstRewardScore() != 0 || again.GetMyrank().GetPvpIntegral() != 10 {
		t.Fatal("daily entry duplicated")
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("query changed wallet or squad")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSBuyLimitPvpTimes{Num: proto.Int32(2)}, 200, now)
			errs <- err
			if err == nil {
				replies <- r[0]
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
	var original proto.Message
	for reply := range replies {
		if original == nil {
			original = reply
		}
		if !proto.Equal(original, reply) {
			t.Fatal("concurrent receipt changed")
		}
	}
	bought := original.(*pb.SCBuyLimitPvpTimes)
	if bought.GetFightTimes() != 0 || bought.GetAllTimes() != 12 || bought.Cost[0].GetNum() != 60 {
		t.Fatal("original used/total semantics", bought)
	}
	wallet, _ := p10LuckyProfile(t, f.id)
	if wallet.GetDiamond() != 940 {
		t.Fatal("concurrent repeat charged repeatedly", wallet.GetDiamond())
	}
	restarted := &GameService{}
	if err := restarted.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f.s = restarted
	if !proto.Equal(original, p11LimitCall(t, f, f.id, &pb.CSBuyLimitPvpTimes{Num: proto.Int32(2)}, 200, now.Add(49*time.Hour))) {
		t.Fatal("expired-period paid receipt lost")
	}
	for _, request := range []proto.Message{&pb.CSBuyLimitPvpTimes{Num: proto.Int32(3)}, &pb.CSLimitPvpFightResult{PlayerId: proto.Int64(b)}} {
		if _, err := f.s.p11LimitAt(f.ctx, f.id, request, 200, now); err == nil {
			t.Fatal("conflict/authority/reward accepted")
		}
	}
	next := p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 201, now.Add(2*time.Minute)).(*pb.SCLimitPvpFindMatch)
	if next.GetMyrank().GetPvpIntegral() != 20 || next.GetFirstRewardScore() != 10 || next.GetAllTimes() != 10 {
		t.Fatal("cross-day actual score/counter", next)
	}
	// 持久日积分记录确保重登及跨日不会重复赠分。
	var days []db.P11LimitDaily
	db.GetIns().Where("period_id=? AND player_id=?", "isolated-period-a", f.id).Find(&days)
	if len(days) != 2 || days[0].LoginScore+days[1].LoginScore != 20 {
		t.Fatal("persistent daily score", days)
	}
}
func TestP11LimitPurchaseFailuresPeriodRolloverAndOriginalFilters(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Now()
	p11OpenLimit(t, now, "isolated-period-a")
	p11LimitCall(t, f, b, &pb.CSLimitPvpFindMatch{}, 100, now)
	p11LimitCall(t, f, c, &pb.CSLimitPvpFindMatch{}, 100, now)
	if err := updateProfile(f.ctx, c, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(60); return nil }); err != nil {
		t.Fatal(err)
	}
	reply := p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 100, now).(*pb.SCLimitPvpFindMatch)
	if len(reply.RankList) != 1 || reply.RankList[0].Info.GetPlayerId() != b {
		t.Fatal("original level window not respected", reply)
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: f.id, TargetID: b, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	reply = p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 101, now).(*pb.SCLimitPvpFindMatch)
	if len(reply.RankList) != 0 {
		t.Fatal("black peer matched")
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	for _, stamp := range []int64{0, 200} {
		if _, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSBuyLimitPvpTimes{Num: proto.Int32(1)}, stamp, now); err == nil {
			t.Fatal("missing receipt/insufficient diamonds accepted")
		}
	}
	var day db.P11LimitDaily
	db.GetIns().First(&day, "period_id=? AND player_id=?", "isolated-period-a", f.id)
	if day.Bought != 0 {
		t.Fatal("failed purchase consumed quota")
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("failed purchase mutated wallet")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	p11LimitCall(t, f, f.id, &pb.CSBuyLimitPvpTimes{Num: proto.Int32(10)}, 202, now)
	if _, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSBuyLimitPvpTimes{Num: proto.Int32(1)}, 203, now); err == nil {
		t.Fatal("original buy cap exceeded")
	}
	if err := db.GetIns().Model(&db.P11LimitPolicy{}).Where("id=1").Update("ends_at", now.Add(time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSLimitPvpFindMatch{}, 204, now); err == nil || !strings.Contains(err.Error(), "不可覆盖") {
		t.Fatal("period window overwritten", err)
	}
	p11OpenLimit(t, now, "isolated-period-b")
	next := p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 205, now).(*pb.SCLimitPvpFindMatch)
	if next.GetMyrank().GetPvpIntegral() != 10 || next.GetAllTimes() != 10 || len(next.RankList) != 0 {
		t.Fatal("new period inherited old score or peers", next)
	}
	var old db.P11LimitMember
	db.GetIns().First(&old, "period_id=? AND player_id=?", "isolated-period-a", f.id)
	if old.Score != 10 {
		t.Fatal("old history mutated")
	}
}

// TestP11LimitOriginalElevenScoreTiers实际逐日进入累计800分，验证原整档与重启唯一领取，不写任何伪胜利。
func TestP11LimitOriginalElevenScoreTiersAndConcurrentClaims(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	now := time.Now().Add(-80 * 24 * time.Hour)
	p11OpenLimit(t, now, "isolated-score-tiers")
	if err := db.GetIns().Model(&db.P11LimitPolicy{}).Where("id=1").Update("ends_at", now.Add(90*24*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 100, now)
	info := p11LimitCall(t, f, f.id, &pb.CSLimitPvpRewardInfo{}, 101, now).(*pb.SCLimitPvpRewardInfo)
	if len(info.Items) != 11 || info.Items[0].GetState() != 0 {
		t.Fatal("original 11 tiers or default reward gate", info)
	}
	if _, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("1")}, 102, now); err == nil {
		t.Fatal("closed reward operator bypassed")
	}
	if err := db.GetIns().Model(&db.P11LimitPolicy{}).Where("id=1").Update("reward_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	info = p11LimitCall(t, f, f.id, &pb.CSLimitPvpRewardInfo{}, 103, now).(*pb.SCLimitPvpRewardInfo)
	if info.Items[0].GetState() != 1 || info.Items[1].GetState() != 0 {
		t.Fatal("original eligibility state", info)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	results := make(chan *pb.SCGetLimitPvpRewardInfo, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(stamp int64) {
			defer wg.Done()
			r, err := f.s.p11LimitAt(f.ctx, f.id, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("1")}, stamp, now)
			errs <- err
			if err == nil {
				results <- r[0].(*pb.SCGetLimitPvpRewardInfo)
			}
		}(int64(200 + i))
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	granted := 0
	for r := range results {
		if len(r.GetGet().GetSimps()) > 0 {
			granted++
		}
	}
	if granted != 1 {
		t.Fatal("concurrent unique tier claims granted repeatedly", granted)
	}
	for _, request := range []proto.Message{&pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("2")}, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("hero_082")}} {
		if _, err := f.s.p11LimitAt(f.ctx, f.id, request, 300, now); err == nil {
			t.Fatal("undue or unknown original tier accepted")
		}
	}
	for day := 1; day < 80; day++ {
		p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, int64(1000+day), now.Add(time.Duration(day)*24*time.Hour))
	}
	end := now.Add(79 * 24 * time.Hour)
	info = p11LimitCall(t, f, f.id, &pb.CSLimitPvpRewardInfo{}, 3000, end).(*pb.SCLimitPvpRewardInfo)
	if info.Items[0].GetState() != 2 {
		t.Fatal("claimed state lost")
	}
	for _, item := range info.Items[1:] {
		if item.GetState() != 1 {
			t.Fatal("actual 800 score did not unlock full original tiers", item)
		}
	}
	for key := 2; key <= 11; key++ {
		p11LimitCall(t, f, f.id, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String(fmt.Sprintf("%d", key))}, int64(4000+key), end)
	}
	_, g := p10LuckyProfile(t, f.id)
	found := false
	for _, w := range g.Warriors {
		if w.GetId() == "hero_082" {
			found = true
		}
	}
	if !found {
		t.Fatal("original 800 score tier did not create real hero instance")
	}
	var claims int64
	db.GetIns().Model(&db.P11LimitRewardClaim{}).Where("period_id=? AND player_id=?", "isolated-score-tiers", f.id).Count(&claims)
	if claims != 11 {
		t.Fatal("full original tiers not unique", claims)
	}
	var member db.P11LimitMember
	db.GetIns().First(&member, "period_id=? AND player_id=?", "isolated-score-tiers", f.id)
	if member.Score != 800 {
		t.Fatal("true entry score ledger", member.Score)
	}
	restarted := &GameService{}
	if err := restarted.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f.s = restarted
	final := p11LimitCall(t, f, f.id, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("11")}, 4011, end)
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	if !proto.Equal(final, p11LimitCall(t, f, f.id, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("11")}, 4011, end.Add(20*24*time.Hour))) {
		t.Fatal("expired paid award receipt lost")
	}
	duplicate := p11LimitCall(t, f, f.id, &pb.CSGetLimitPvpRewardInfo{ItemId: proto.String("11")}, 5000, end).(*pb.SCGetLimitPvpRewardInfo)
	if len(duplicate.GetGet().GetWars()) != 0 || len(duplicate.GetGet().GetSimps()) != 0 {
		t.Fatal("new request granted old award again")
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("award replay mutated resources")
	}
}
