package service

import (
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7ExpeditionSetup(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7Setup(t)
	if err := f.s.LoadP7ExpeditionTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P7ExpeditionPolicy{}, &db.P7ExpeditionState{}, &db.P7ExpeditionMission{}, &db.P7ExpeditionHero{}, &db.P7ExpeditionCataloguePolicy{}, &db.P7ExpeditionStoryPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7ExpeditionPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(32)
		p.Energy = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "hero_001,1;hero_025,1")
		// 本组隔离时长/占用/奖励合同；有限成熟伙伴使原D/C成功率达到100%。
		// 成功率边界另由专门测试覆盖，保留本组无关合同的稳定前提。
		for _, hero := range g.Warriors {
			if hero.GetGuid() != p.GetHeroWarId() {
				hero.Level = proto.Int32(100)
				hero.Talent = proto.Int32(2)
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

func p7ExpeditionCall(t *testing.T, f *p1Fixture, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	r, err := f.s.p7Expedition(f.ctx, f.id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return r[0]
}

// 原帧集成测试使用真实伙伴实例；仅在显式专属MySQL库保留夹具。
func TestP7ExpeditionProtocolSeed(t *testing.T) {
	f := p7ExpeditionSetup(t)
	a, err := LoginAccount(f.ctx, "p7-native-expedition", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, a.Id, "远征原帧回归", "hero_047"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(32)
		p.Energy = proto.Int32(100)
		p.Diamond = proto.Int64(1000)
		_, err := f.s.GrantFixedReward(p, g, "hero_001,1;hero_025,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP7ExpeditionRealDurationEnergyOccupancyFrozenRewardAndRestart(t *testing.T) {
	f := p7ExpeditionSetup(t)
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	if len(list.GetRefreshedMissions()) != 2 {
		t.Fatal("original D max two offers")
	}
	offer := list.GetRefreshedMissions()[0]
	_, game := f.profile(t)
	guid := game.Warriors[1].GetGuid()
	request := &pb.CSAcceptMission{MissionSerial: offer.MissionSerial, HeroGuids: []string{guid}}
	p7ExpeditionCall(t, f, request, 1, at)
	accepted := p7ExpeditionCall(t, f, &pb.CSGetAcceptedMissions{}, 0, at).(*pb.SCGetAcceptedMissions)
	mission := accepted.GetAcceptedMissions()[0]
	var row db.P7ExpeditionMission
	if err := db.GetIns().Where("serial=?", offer.GetMissionSerial()).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	q, err := p7ReadExpeditionQuote(row)
	if err != nil {
		t.Fatal(err)
	}
	if mission.GetEndTime()-mission.GetStartTime() != int64(q.Hours)*3600000 || mission.GetMainHeroId() != game.Warriors[1].GetId() || mission.GetState() != 1 {
		t.Fatal("actual frozen original duration/hero")
	}
	p, _ := f.profile(t)
	if p.GetEnergy() != 100-q.Energy {
		t.Fatal("actual hourly energy cost")
	}
	if _, err = f.s.p7Expedition(f.ctx, f.id, &pb.CSGetFinalReward{MissionSerial: offer.MissionSerial}, 2, at.Add(time.Duration(q.Hours)*time.Hour-time.Millisecond)); err == nil {
		t.Fatal("early reward")
	}
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		g.Warriors[1].Level = proto.Int32(2)
		return f.s.GuardP7ExpeditionSave(tx, f.id, g)
	}); err == nil {
		t.Fatal("reserved actor modified")
	}
	if err = db.GetIns().Model(&db.P7ExpeditionPolicy{}).Where("id=?", 1).Update("reward", "diamond,9999").Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ready := p7ExpeditionCall(t, f, &pb.CSGetAcceptedMissions{}, 0, now).(*pb.SCGetAcceptedMissions)
	if ready.GetAcceptedMissions()[0].GetState() != 2 {
		t.Fatal("offline timer did not restore")
	}
	claim := &pb.CSGetFinalReward{MissionSerial: offer.MissionSerial}
	first := p7ExpeditionCall(t, f, claim, 2, now).(*pb.SCGetFinalReward)
	if len(first.GetGet().GetSimps()) != 1 || first.GetGet().GetSimps()[0].GetId() != "item_001" || first.GetExp() != q.Exp {
		t.Fatal("live config changed frozen reward")
	}
	if err = db.GetIns().Model(&db.P7ExpeditionPolicy{}).Where("id=?", 1).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(first, p7ExpeditionCall(t, f, claim, 2, now.Add(time.Hour))) || !proto.Equal(first, p7ExpeditionCall(t, f, claim, 3, now.Add(time.Hour))) {
		t.Fatal("closed policy lost old reward receipt")
	}
	p, game = f.profile(t)
	if simpleOwned := simpleOwned(p, game, "item_001"); simpleOwned != 1 {
		t.Fatal("reward multiplicity", simpleOwned)
	}
	if p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 0 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "expedition_complete") != 1 {
		t.Fatal("holds not released or fact repeated")
	}
}

func TestP7ExpeditionRefreshOriginalFreeDiamondAndQuoteReplay(t *testing.T) {
	f := p7ExpeditionSetup(t)
	now := time.Now()
	request := &pb.CSRefreshMissionList{}
	for stamp := int64(1); stamp <= 3; stamp++ {
		first := p7ExpeditionCall(t, f, request, stamp, now).(*pb.SCRefreshMissionList)
		if stamp < 3 && first.GetCost().GetNum() != 0 {
			t.Fatal("free refresh charged")
		}
		if stamp == 3 && (first.GetCost().GetId() != "diamond" || first.GetCost().GetNum() != 20) {
			t.Fatal("original refresh price")
		}
		if !proto.Equal(first, p7ExpeditionCall(t, f, request, stamp, now)) {
			t.Fatal("refresh retry rerolled list")
		}
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 980 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent) != 1 {
		t.Fatal("refresh actual/duplicate spend")
	}
	inter := p7ExpeditionCall(t, f, &pb.CSInterExpedition{}, 0, now.AddDate(0, 0, 1)).(*pb.SCInterExpedition)
	if inter.GetFreeRefreshTime() != 2 {
		t.Fatal("refresh next day did not reset")
	}
}

func TestP7ExpeditionConcurrentAcceptClaimAndNoMainOrDoubleAssignment(t *testing.T) {
	f := p7ExpeditionSetup(t)
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	_, g := f.profile(t)
	guid := g.Warriors[1].GetGuid()
	request := &pb.CSAcceptMission{MissionSerial: list.GetRefreshedMissions()[0].MissionSerial, HeroGuids: []string{guid}}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.p7Expedition(f.ctx, f.id, request, 1, at); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "expedition_accept") != 1 {
		t.Fatal("duplicate reserve/accept fact")
	}
	for _, hero := range []string{guid, g.Warriors[0].GetGuid()} {
		if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSAcceptMission{MissionSerial: list.GetRefreshedMissions()[1].MissionSerial, HeroGuids: []string{hero}}, 2, at); err == nil {
			t.Fatal("double hold/main assignment")
		}
	}
	claim := &pb.CSGetFinalReward{MissionSerial: request.MissionSerial}
	responses := make(chan proto.Message, 4)
	errs = make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.s.p7Expedition(f.ctx, f.id, claim, 1, time.Now())
			if e == nil {
				responses <- r[0]
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	close(responses)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first proto.Message
	for r := range responses {
		if first == nil {
			first = r
		} else if !proto.Equal(first, r) {
			t.Fatal("claim reply differs")
		}
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_001") != 1 || p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 0 {
		t.Fatal("parallel completion duplicated")
	}
}

func TestP7ExpeditionFailuresRollBackReservationAndRewards(t *testing.T) {
	f := p7ExpeditionSetup(t)
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	_, g := f.profile(t)
	request := &pb.CSAcceptMission{MissionSerial: list.GetRefreshedMissions()[0].MissionSerial, HeroGuids: []string{g.Warriors[1].GetGuid()}}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(0); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p7Expedition(f.ctx, f.id, request, 1, at); err == nil {
		t.Fatal("insufficient energy accepted")
	}
	if p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7RushReceipt{}, "player_id=? AND domain=?", f.id, "exp-accept") != 0 {
		t.Fatal("failed assignment left hold/receipt")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	p7ExpeditionCall(t, f, request, 1, at)
	if err := db.GetIns().Model(&db.P7ExpeditionState{}).Where("player_id=?", f.id).Update("exp", math.MaxInt32).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSGetFinalReward{MissionSerial: request.MissionSerial}, 1, time.Now()); err == nil {
		t.Fatal("overflow exp reward")
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_001") != 0 || p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 1 {
		t.Fatal("overflow partially paid/released")
	}
	var mission db.P7ExpeditionMission
	if err := db.GetIns().Where("serial=?", request.GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	var q p7ExpeditionQuote
	if json.Unmarshal(mission.Quote, &q) != nil {
		t.Fatal("quote")
	}
	q.Reward = "diamond,9999"
	raw, _ := json.Marshal(q)
	if err := db.GetIns().Model(&mission).Update("quote", raw).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSGetFinalReward{MissionSerial: request.MissionSerial}, 1, time.Now()); err == nil {
		t.Fatal("tampered quote")
	}
}
