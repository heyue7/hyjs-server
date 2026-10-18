package service

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10LuckySetup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f, b, c := p10SceneSetup(t)
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Lucky(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10LuckyObject{}, &db.P10LuckyDaily{}, &db.P10LuckyPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10Lucky(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ChannelPolicy{}).Where("id=1").Update("world_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(70)
		_, err := f.s.GrantFixedReward(p, g, "item_350,6")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f, b, c
}
func p10LuckyEnable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P10LuckyPolicy{}).Where("id=1").Updates(map[string]any{"fireworks_enabled": true, "drop_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
}
func p10LuckyProfile(t *testing.T, id int64) (*pb.Player, *model.GameState) {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	return p, g
}
func p10LuckyItems(t *testing.T, f *p1Fixture, id int64, spec string) {
	t.Helper()
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { _, err := f.s.GrantFixedReward(p, g, spec); return err }); err != nil {
		t.Fatal(err)
	}
}
func p10LuckyDropRequest(x float32) *pb.CSThrowLuckyBags {
	return &pb.CSThrowLuckyBags{Num: proto.Int32(1), PosArray: []*pb.PointNew{{X: proto.Float32(x), Y: proto.Float32(0), Z: proto.Float32(0)}}}
}
func p10LuckyObjectRow(t *testing.T, id int64) db.P10LuckyObject {
	t.Helper()
	var row db.P10LuckyObject
	if err := db.GetIns().Where("owner_id=?", id).Order("created_at DESC").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}
func TestP10LuckyOriginal57RewardCatalogueActualAllResourcesAndClosed(t *testing.T) {
	f, _, _ := p10LuckySetup(t)
	cfg, options, err := f.s.p10LuckyConfig(db.GetIns())
	if err != nil || len(options) != 57 || cfg.FireworksEnabled || cfg.DropEnabled {
		t.Fatal("original complete closed pool", len(options), err)
	}
	for _, option := range options {
		p, g := p10LuckyProfile(t, f.id)
		pair := strings.Split(option.Result, ",")
		n, _ := strconv.Atoi(pair[1])
		gain, err := f.s.p10LuckyGrant(p, g, option.Result)
		if err != nil {
			t.Fatalf("original %s %s: %v", option.ID, option.Result, err)
		}
		if len(gain.GetSimps()) != 1 || gain.GetSimps()[0].GetId() != pair[0] || gain.GetSimps()[0].GetNum() != int32(n) {
			t.Fatal("wrong original resource", option, gain)
		}
	}
	result, err := f.s.p10LuckyAt(f.ctx, f.id, &pb.CSCurrentLuckyBagsInfo{}, 0, time.Now())
	if err != nil || len(result[0].(*pb.SCCurrentLuckyBagsInfo).GetLuckyBags()) != 0 {
		t.Fatal("closed initialization must read empty actual set", err)
	}
	rec := &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, rec)
	before, _ := p10LuckyProfile(t, f.id)
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_350"), Num: proto.Int32(1)}}, 1, time.Now()); err == nil {
		t.Fatal("closed fireworks consumed")
	}
	after, _ := p10LuckyProfile(t, f.id)
	if !proto.Equal(before, after) {
		t.Fatal("failed closed request changed role")
	}
}
func TestP10LuckyFireworksActual20BagsOriginalDailyCDAndReceipt(t *testing.T) {
	f, b, _ := p10LuckySetup(t)
	p10LuckyEnable(t)
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	now := time.Now().Add(-time.Hour)
	request := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_350"), Num: proto.Int32(1)}}
	first, err := f.s.p10LuckyAt(f.ctx, f.id, request, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	reply := first[0].(*pb.SCItemUse)
	if reply.GetCost().GetNum() != 1 || reply.GetGet().GetSimps()[0].GetNum() != 20 {
		t.Fatal("wrong original conversion", reply)
	}
	if len(ra.drain()) != 1 || len(rb.drain()) != 1 {
		t.Fatal("original same-line fireworks visual missing")
	}
	repeated, err := f.s.p10LuckyAt(f.ctx, f.id, request, 100, now.Add(time.Second))
	if err != nil || !proto.Equal(first[0], repeated[0]) || len(ra.drain())+len(rb.drain()) != 0 {
		t.Fatal("fireworks retry duplicated visual or gain", err)
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, request, 101, now.Add(time.Second)); err == nil {
		t.Fatal("fireworks CD bypass")
	}
	for i := 1; i < 5; i++ {
		if _, err := f.s.p10LuckyAt(f.ctx, f.id, request, int64(100+i), now.Add(time.Duration(i)*21*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, request, 999, now.Add(6*21*time.Second)); err == nil {
		t.Fatal("original daily5 bypass")
	}
	p, g := p10LuckyProfile(t, f.id)
	if simpleOwned(p, g, "item_350") != 1 || simpleOwned(p, g, "item_349") != 100 {
		t.Fatal("conversion inventory", g.Items)
	}
	if err := db.GetIns().Model(&db.P10LuckyPolicy{}).Where("id=1").Update("fireworks_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, request, 100, now.Add(2*time.Hour)); err != nil {
		t.Fatal("closed original receipt", err)
	}
	changed := proto.Clone(request).(*pb.CSItemUse)
	changed.GetItemIndex = proto.Int32(1)
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, changed, 100, now); err == nil {
		t.Fatal("same receipt changed body")
	}
}
func TestP10LuckyRealObjectsSameLineDistanceConcurrencyFreezeAndRestart(t *testing.T) {
	f, b, c := p10LuckySetup(t)
	p10LuckyEnable(t)
	p10LuckyItems(t, f, f.id, "item_349,3")
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	p10SceneConnect(t, f, c, rc)
	ra.drain()
	rb.drain()
	rc.drain()
	now := time.Now().Add(-time.Minute)
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(8), 0, now); err == nil {
		t.Fatal("distant throw allowed")
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(1), 0, now); err != nil {
		t.Fatal(err)
	}
	object := p10LuckyObjectRow(t, f.id)
	if object.Reward == "" || object.RewardHash == "" || object.ExpiresAt-object.CreatedAt != 3600000 {
		t.Fatal("original source freeze incomplete")
	}
	if len(rb.drain()) != 1 || len(rc.drain()) != 1 {
		t.Fatal("same-line real object not broadcast")
	}
	if _, _, err := f.s.DispatchP10Scene(f.ctx, c, &pb.CSTransferLine{Line: proto.Int32(2)}, 500); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSSnatchOneLuckyBag{Guid: proto.String(object.GUID)}
	if _, err := f.s.p10LuckyAt(f.ctx, c, request, 0, now.Add(time.Second)); err == nil {
		t.Fatal("cross line snatch")
	}
	if _, _, err := f.s.DispatchP10Scene(f.ctx, c, &pb.CSTransferLine{Line: proto.Int32(1)}, 501); err != nil {
		t.Fatal(err)
	}
	// 删除当前奖池不改变已扣资源冻结对象的兑现事实。
	if err := db.GetIns().Model(&db.P10LuckyPolicy{}).Where("id=1").Updates(map[string]any{"drop_enabled": false, "reward_json": "[]", "reward_hash": "altered"}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	type outcome struct {
		id  int64
		err error
	}
	results := make(chan outcome, 8)
	for i := 0; i < 8; i++ {
		target := b
		if i%2 == 1 {
			target = c
		}
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, err := f.s.p10LuckyAt(f.ctx, id, request, 0, now.Add(2*time.Second))
			results <- outcome{id, err}
		}(target)
	}
	wg.Wait()
	close(results)
	refreshed := p10LuckyObjectRow(t, f.id)
	if refreshed.ClaimedBy != b && refreshed.ClaimedBy != c {
		t.Fatal("no real winner")
	}
	for result := range results {
		if (result.err == nil) != (result.id == refreshed.ClaimedBy) {
			t.Fatal("multiple actual winners", result)
		}
	}
	var daily db.P10LuckyDaily
	if err := db.GetIns().First(&daily, "player_id=? AND day=?", refreshed.ClaimedBy, dailyCopyDay(now)).Error; err != nil || daily.Snatched != 1 {
		t.Fatal("duplicate snatch count", daily, err)
	}
	original := &pb.SCSnatchOneLuckyBag{}
	if err := proto.Unmarshal(refreshed.ClaimResponse, original); err != nil {
		t.Fatal(err)
	}
	restarted := &GameService{}
	if err := restarted.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.p10LuckyAt(f.ctx, refreshed.ClaimedBy, request, 0, now.Add(2*time.Hour))
	if err != nil || !proto.Equal(original, replay[0]) {
		t.Fatal("object original receipt after restart/expiry", err)
	}
}
func TestP10LuckyCrossDayNativeDropRetryExpiryAndFailureRollback(t *testing.T) {
	f, b, _ := p10LuckySetup(t)
	p10LuckyEnable(t)
	p10LuckyItems(t, f, f.id, "item_349,4")
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	local := time.FixedZone("UTC+8", 8*3600)
	base := time.Now().In(local).AddDate(0, 0, -1)
	now := time.Date(base.Year(), base.Month(), base.Day(), 23, 59, 59, 0, local)
	first, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(1), 0, now)
	if err != nil {
		t.Fatal(err)
	}
	object := p10LuckyObjectRow(t, f.id)
	rb.drain()
	replay, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(1), 0, now.Add(2*time.Second))
	if err != nil || !proto.Equal(first[0], replay[0]) || len(rb.drain()) != 0 {
		t.Fatal("midnight anonymous retry duplicated drop", err)
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(2), 0, now.Add(2*time.Second)); err == nil {
		t.Fatal("midnight cooldown reset")
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(2), 0, now.Add(21*time.Second)); err != nil {
		t.Fatal(err)
	}
	p, g := p10LuckyProfile(t, f.id)
	if simpleOwned(p, g, "item_349") != 2 {
		t.Fatal("midnight duplicated debit")
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(10)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, b, true); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSSnatchOneLuckyBag{Guid: proto.String(object.GUID)}
	if _, err := f.s.p10LuckyAt(f.ctx, b, request, 0, now.Add(time.Minute)); err == nil {
		t.Fatal("original5m guard bypass")
	}
	if _, err := f.s.p10LuckyAt(f.ctx, b, request, 0, now.Add(time.Hour)); err == nil {
		t.Fatal("expired object claimed")
	}
	var count int64
	db.GetIns().Model(&db.P10LuckyObject{}).Where("claimed_at>0").Count(&count)
	if count != 0 {
		t.Fatal("failed claims committed")
	}
	invalid := &pb.CSThrowLuckyBags{Num: proto.Int32(1), PosArray: []*pb.PointNew{{X: proto.Float32(float32(math.NaN()))}}}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, invalid, 0, now.Add(time.Minute)); err == nil {
		t.Fatal("nonfinite authored object")
	}
}
func TestP10LuckyFailedAwardRollsBackObjectAndResources(t *testing.T) {
	f, b, _ := p10LuckySetup(t)
	p10LuckyEnable(t)
	p10LuckyItems(t, f, f.id, "item_349,1")
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	now := time.Now().Add(-time.Minute)
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(1), 0, now); err != nil {
		t.Fatal(err)
	}
	object := p10LuckyObjectRow(t, f.id)
	if err := db.GetIns().Model(&db.P10LuckyObject{}).Where("guid=?", object.GUID).Update("reward", "coin,66").Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { p.Coin = proto.Int64(math.MaxInt64); return nil }); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := p10LuckyProfile(t, b)
	before, _ := json.Marshal(beforeG)
	if _, err := f.s.p10LuckyAt(f.ctx, b, &pb.CSSnatchOneLuckyBag{Guid: proto.String(object.GUID)}, 1, now.Add(time.Second)); err == nil {
		t.Fatal("overflow reward accepted")
	}
	afterP, afterG := p10LuckyProfile(t, b)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) || p10LuckyObjectRow(t, f.id).ClaimedAt != 0 {
		t.Fatal("failed award persisted partial state")
	}
	var count int64
	db.GetIns().Model(&db.P10LuckyDaily{}).Where("player_id=?", b).Count(&count)
	if count != 0 {
		t.Fatal("failed claim retained daily counter")
	}
}

func TestP10LuckyOwnerBlackFiltersPhysicalPushQueryAndClaim(t *testing.T) {
	f, b, c := p10LuckySetup(t)
	p10LuckyEnable(t)
	p10LuckyItems(t, f, f.id, "item_349,1")
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	p10SceneConnect(t, f, c, rc)
	ra.drain()
	rb.drain()
	rc.drain()
	now := time.Now().Add(-time.Minute)
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: c, TargetID: f.id, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p10LuckyAt(f.ctx, f.id, p10LuckyDropRequest(1), 0, now); err != nil {
		t.Fatal(err)
	}
	if len(rb.drain()) != 1 || len(rc.drain()) != 0 {
		t.Fatal("black owner physical payload leaked")
	}
	objects := p10LuckyObjectRow(t, f.id)
	list, err := f.s.p10LuckyList(db.GetIns(), c, now)
	if err != nil || len(list.GetLuckyBags()) != 0 {
		t.Fatal("black object query leaked", err)
	}
	if _, err := f.s.p10LuckyAt(f.ctx, c, &pb.CSSnatchOneLuckyBag{Guid: proto.String(objects.GUID)}, 0, now.Add(time.Second)); err == nil {
		t.Fatal("black owner object claimed")
	}
	if _, err := f.s.p10LuckyAt(f.ctx, b, &pb.CSSnatchOneLuckyBag{Guid: proto.String(objects.GUID)}, 0, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(rc.drain()) != 0 {
		t.Fatal("hidden object removal leaked")
	}
	var sent, received int64
	db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND kind=?", f.id, "lucky_bags_dropped").Count(&sent)
	db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND kind=?", b, "lucky_bag_claimed").Count(&received)
	if sent != 1 || received != 1 {
		t.Fatal("actual P1 object facts missing", sent, received)
	}
}
