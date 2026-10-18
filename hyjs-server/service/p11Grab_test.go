package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p11GrabCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	out, err := f.s.p11GrabAt(f.ctx, id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return out[0]
}
func p11GrabPrepare(t *testing.T, f *p1Fixture, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(30)
			p.Diamond = proto.Int64(300)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P11GrabPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
}
func p11GrabPastDay() time.Time {
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 0, -2)
	return time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, local.Location())
}
func TestP11GrabOriginalEmptySlotAtomicOccupationBuyGiveupAndFullTimeRewards(t *testing.T) {
	f, b, _ := p11ArenaSetup(t)
	now := p11GrabPastDay()
	list := p11GrabCall(t, f, f.id, &pb.CSGetGrabData{}, 1, now).(*pb.SCGetGrabData)
	if len(list.GrabData) != 18 || list.GetGrabTimes() != 3 || list.GetLastCount() != 3 {
		t.Fatal("original18 slots/remaining counters", list)
	}
	for i, slot := range list.GrabData {
		if slot.GetPosition() != int32(i+1) || slot.GetPlayerId() != 0 {
			t.Fatal("fake empty occupant", slot)
		}
	}
	if _, err := f.s.p11GrabAt(f.ctx, f.id, &pb.CSGrab{Index: proto.Int32(1)}, 2, now); err == nil {
		t.Fatal("default new occupation open")
	}
	p11GrabPrepare(t, f, f.id, b)
	beforePlayer, beforeGame := p10LuckyProfile(t, f.id)
	itemBefore := map[string]int64{}
	for _, key := range []string{"item_459", "item_461", "item_013", "item_462", "item_460", "item_463"} {
		itemBefore[key] = int64(simpleOwned(beforePlayer, beforeGame, key))
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	results := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.p11GrabAt(f.ctx, f.id, &pb.CSGrab{Index: proto.Int32(1)}, 100, now)
			errs <- err
			if err == nil {
				results <- out[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var original proto.Message
	for r := range results {
		if original == nil {
			original = r
		}
		if !proto.Equal(original, r) {
			t.Fatal("same stamp occupation diverged")
		}
	}
	first := original.(*pb.SCGrab)
	if first.GetGrabData().GetPlayerId() != f.id || first.GetCurrEnergy() != 90 || first.GetGrabTimes() != 2 || first.AttPlayer != nil || first.GetFightId() != "" || first.GetWinPlayerId() != 0 {
		t.Fatal("original empty no-battle branch", first)
	}
	for _, args := range []struct {
		id   int64
		slot int32
	}{{f.id, 7}, {b, 1}} {
		if _, err := f.s.p11GrabAt(f.ctx, args.id, &pb.CSGrab{Index: proto.Int32(args.slot)}, 101, now); err == nil {
			t.Fatal("double own or occupied combat manufactured")
		}
	}
	bought := p11GrabCall(t, f, f.id, &pb.CSBuyGrab{Count: proto.Int32(1)}, 102, now).(*pb.SCBuyGrab)
	if bought.GetCurrCount() != 3 || bought.GetLastCount() != 2 || bought.Cost[0].GetNum() != 100 {
		t.Fatal("original buy refill counters", bought)
	}
	if _, err := f.s.p11GrabAt(f.ctx, f.id, &pb.CSBuyGrab{Count: proto.Int32(1)}, 103, now); err == nil {
		t.Fatal("maximum3 capacity exceeded")
	}
	p11GrabCall(t, f, f.id, &pb.CSGiveUpGrab{Index: proto.Int32(1)}, 104, now.Add(30*time.Minute))
	second := p11GrabCall(t, f, f.id, &pb.CSGrab{Index: proto.Int32(7)}, 105, now.Add(30*time.Minute)).(*pb.SCGrab)
	if second.GetCurrEnergy() != 80 || second.GetGrabData().GetPosition() != 7 {
		t.Fatal("real second tier empty occupancy", second)
	}
	if err := db.GetIns().Model(&db.P11GrabPolicy{}).Where("id=1").Update("time_reward_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11GrabAt(f.ctx, f.id, &pb.CSGetTimeReward{Time: proto.Int32(1)}, 106, now.Add(45*time.Minute)); err == nil {
		t.Fatal("undue timer grant accepted")
	}
	info := p11GrabCall(t, f, f.id, &pb.CSGetTimeRewardInfo{}, 107, now.Add(2*time.Hour)).(*pb.SCGetTimeRewardInfo)
	if info.GetMyGrabMinutes() != 120 || len(info.RewardTime) != 2 || info.RewardTime[0] != 1 || info.RewardTime[1] != 2 {
		t.Fatal("segmented accumulated duration", info)
	}
	var cfg db.P11GrabPolicy
	db.GetIns().First(&cfg, 1)
	var changed []p11GrabReward
	if err := json.Unmarshal([]byte(cfg.RewardsJSON), &changed); err != nil {
		t.Fatal(err)
	}
	for i := range changed {
		changed[i].Reward = "coin,1"
	}
	changedRaw, _ := json.Marshal(changed)
	if err := db.GetIns().Model(&db.P11GrabPolicy{}).Where("id=1").Updates(map[string]any{"rewards_json": string(changedRaw), "rewards_hash": p6Digest(changedRaw)}).Error; err != nil {
		t.Fatal(err)
	}
	// 日奖档在首次观察冻结，后续运营替换不减少已有真实时长的原奖励承诺。
	var claimWG sync.WaitGroup
	claimErrs := make(chan error, 8)
	claimResults := make(chan *pb.SCGetTimeReward, 8)
	for i := 0; i < 8; i++ {
		claimWG.Add(1)
		go func(stamp int64) {
			defer claimWG.Done()
			out, err := f.s.p11GrabAt(f.ctx, f.id, &pb.CSGetTimeReward{Time: proto.Int32(1)}, stamp, now.Add(2*time.Hour))
			claimErrs <- err
			if err == nil {
				claimResults <- out[0].(*pb.SCGetTimeReward)
			}
		}(int64(1000 + i))
	}
	claimWG.Wait()
	close(claimErrs)
	close(claimResults)
	for err := range claimErrs {
		if err != nil {
			t.Fatal(err)
		}
	}
	granted := 0
	for out := range claimResults {
		if len(out.GetAdd().GetSimps()) > 0 {
			granted++
		}
	}
	if granted != 1 {
		t.Fatal("parallel timer claim granted repeatedly", granted)
	}
	p11GrabCall(t, f, f.id, &pb.CSGetTimeReward{Time: proto.Int32(2)}, 109, now.Add(2*time.Hour))
	p11GrabCall(t, f, f.id, &pb.CSGetTimeReward{Time: proto.Int32(3)}, 110, now.Add(4*time.Hour))
	info = p11GrabCall(t, f, f.id, &pb.CSGetTimeRewardInfo{}, 111, now.Add(4*time.Hour)).(*pb.SCGetTimeRewardInfo)
	if info.GetMyGrabMinutes() != 210 || len(info.RewardTime) != 0 {
		t.Fatal("max3h per session exact expiry/all tiers", info)
	}
	list = p11GrabCall(t, f, f.id, &pb.CSGetGrabData{}, 112, now.Add(4*time.Hour)).(*pb.SCGetGrabData)
	if list.GrabData[6].GetPlayerId() != 0 {
		t.Fatal("expired old slot still occupied")
	}
	p, g := p10LuckyProfile(t, f.id)
	if p.GetCoin() != 600000 || p.GetDiamond() != 200 {
		t.Fatal("original three rewards or buy wallet", p)
	}
	// 真机页面可裁切第四项图标；实际发放仍必须覆盖原四项奖池中的三种物品与金币。
	for key, gain := range map[string]int64{"item_459": 15, "item_461": 3, "item_013": 12, "item_462": 3, "item_460": 1, "item_463": 3} {
		if int64(simpleOwned(p, g, key))-itemBefore[key] != gain {
			t.Fatal("full original timer reward item delta", key, gain, simpleOwned(p, g, key), itemBefore[key])
		}
	}
	before, _ := json.Marshal(g)
	restart := &GameService{}
	if err := restart.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restart.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f.s = restart
	if !proto.Equal(original, p11GrabCall(t, f, f.id, &pb.CSGrab{Index: proto.Int32(1)}, 100, now.Add(24*time.Hour))) {
		t.Fatal("old occupation transaction receipt lost")
	}
	duplicate := p11GrabCall(t, f, f.id, &pb.CSGetTimeReward{Time: proto.Int32(3)}, 113, now.Add(4*time.Hour)).(*pb.SCGetTimeReward)
	if len(duplicate.GetAdd().GetSimps()) != 0 {
		t.Fatal("time tier regranted")
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(p, afterP) || string(before) != string(after) {
		t.Fatal("restart/retry fabricated resources")
	}
}
func TestP11GrabRealPlayersCompeteOneSlotAndCrossDaySegments(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	p11GrabPrepare(t, f, f.id, b, c)
	now := p11GrabPastDay().Add(11*time.Hour + 30*time.Minute)
	var wg sync.WaitGroup
	wins := make(chan int64, 3)
	for _, id := range []int64{f.id, b, c} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, err := f.s.p11GrabAt(f.ctx, id, &pb.CSGrab{Index: proto.Int32(18)}, 100, now)
			if err == nil {
				wins <- id
			}
		}(id)
	}
	wg.Wait()
	close(wins)
	winner := int64(0)
	count := 0
	for id := range wins {
		winner = id
		count++
	}
	if count != 1 {
		t.Fatal("one original empty slot got multiple owners", count)
	}
	var ownerCount int64
	db.GetIns().Model(&db.P11GrabSlot{}).Where("owner_id IS NOT NULL").Count(&ownerCount)
	if ownerCount != 1 {
		t.Fatal("multiple live owners")
	}
	for _, id := range []int64{f.id, b, c} {
		p, _ := p10LuckyProfile(t, id)
		expected := int32(100)
		if id == winner {
			expected = 90
		}
		if p.GetEnergy() != expected {
			t.Fatal("loser charged or winner charged twice", id, p.GetEnergy())
		}
	}
	for _, id := range []int64{f.id, b, c} {
		if id == winner {
			continue
		}
		if _, err := f.s.p11GrabAt(f.ctx, id, &pb.CSGiveUpGrab{Index: proto.Int32(18)}, 101, now.Add(20*time.Minute)); err == nil {
			t.Fatal("foreign giveup accepted")
		}
	}
	yesterday := p11GrabCall(t, f, winner, &pb.CSGetTimeRewardInfo{}, 102, now.Add(29*time.Minute)).(*pb.SCGetTimeRewardInfo)
	if yesterday.GetMyGrabMinutes() != 29 {
		t.Fatal("before midnight duration", yesterday)
	}
	afterMidnight := p11GrabCall(t, f, winner, &pb.CSGetTimeRewardInfo{}, 103, now.Add(time.Hour)).(*pb.SCGetTimeRewardInfo)
	if afterMidnight.GetMyGrabMinutes() != 30 {
		t.Fatal("UTC+8 day intersection double counted", afterMidnight)
	}
	list := p11GrabCall(t, f, winner, &pb.CSGetGrabData{}, 104, now.Add(time.Hour)).(*pb.SCGetGrabData)
	if list.GetGrabTimes() != 3 || list.GrabData[17].GetPlayerId() != winner {
		t.Fatal("day reset lost owner or duplicated quota", list)
	}
	p11GrabCall(t, f, winner, &pb.CSGiveUpGrab{Index: proto.Int32(18)}, 105, now.Add(time.Hour))
	again := p11GrabCall(t, f, winner, &pb.CSGetTimeRewardInfo{}, 106, now.Add(2*time.Hour)).(*pb.SCGetTimeRewardInfo)
	if again.GetMyGrabMinutes() != 30 {
		t.Fatal("giveup timer still accrued", again)
	}
	if _, err := f.s.p11GrabAt(f.ctx, winner, &pb.CSGetRandReward{}, 107, now.Add(time.Hour)); err == nil {
		t.Fatal("unknown random reward fabricated")
	}
}
