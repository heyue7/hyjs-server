package service

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p7ActualEventMission(t *testing.T, risk int32) (*p1Fixture, db.P7ExpeditionMission, p7ExpeditionQuote, int64, time.Time) {
	t.Helper()
	f := p7ExpeditionSetup(t)
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "risk_percent": risk}).Error; err != nil {
		t.Fatal(err)
	}
	helper, err := LoginAccount(f.ctx, "p7-real-event-helper", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, helper.Id, "真实远征帮助者", "hero_047"); err != nil {
		t.Fatal(err)
	}
	// Isolated historical clock keeps all produced facts before real now;
	// P1 deliberately rejects synthetic future facts.
	at := time.Now().Add(-13 * time.Hour).Truncate(time.Millisecond)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	_, game := f.profile(t)
	request := &pb.CSAcceptMission{MissionSerial: list.RefreshedMissions[0].MissionSerial, HeroGuids: []string{game.Warriors[1].GetGuid()}}
	p7ExpeditionCall(t, f, request, 31, at)
	var mission db.P7ExpeditionMission
	if err := db.GetIns().Where("serial=?", request.GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	quote, err := p7ReadExpeditionQuote(mission)
	if err != nil || quote.Events == nil {
		t.Fatal("actual acceptance did not freeze event policy", err)
	}
	return f, mission, quote, helper.Id, at
}

func TestP7ActualExpeditionEventProducerPendingHelpAtomicResolutionAndFrozenColdReward(t *testing.T) {
	f, mission, quote, helper, at := p7ActualEventMission(t, 100)
	before, _ := f.profile(t)
	var early []P7ExpeditionPendingHelp
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		early, err = f.s.ListP7ExpeditionHelp(tx, []int64{f.id}, at.Add(time.Hour-time.Millisecond))
		return err
	}); err != nil || len(early) != 0 {
		t.Fatal("future event leaked", err, early)
	}
	detail := p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: proto.String(mission.Serial)}, 0, at.Add(time.Hour-time.Millisecond)).(*pb.SCExpeditionMissionDetail)
	if len(detail.Events) != 0 {
		t.Fatal("original detail exposed future events")
	}
	var pending []P7ExpeditionPendingHelp
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		pending, err = f.s.ListP7ExpeditionHelp(tx, []int64{f.id}, at.Add(time.Hour))
		return err
	}); err != nil || len(pending) != 1 || pending[0].EventHash == "" {
		t.Fatal("real first hour risk unavailable", err, pending)
	}
	first := pending[0]
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				_, err := f.s.ApplyP7ExpeditionHelp(tx, helper, f.id, first.MissionSerial, first.EventSerial, at.Add(time.Hour))
				return err
			})
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("one actual event resolved more than once", success)
	}
	if err := db.GetIns().Model(&db.P7ExpeditionEventPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "risk_percent": 0, "help_reward": "energy,99"}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	last := time.UnixMilli(mission.EndAt - 1)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		pending, err := f.s.ListP7ExpeditionHelp(tx, []int64{f.id}, last)
		if err != nil {
			return err
		}
		if len(pending) != int(quote.Hours)-2 {
			t.Fatalf("actual later risk count %d", len(pending))
		}
		for _, event := range pending {
			if _, err := f.s.ApplyP7ExpeditionHelp(tx, helper, f.id, event.MissionSerial, event.EventSerial, last); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	claim := &pb.CSGetFinalReward{MissionSerial: proto.String(mission.Serial)}
	response := p7ExpeditionCall(t, f, claim, 32, time.UnixMilli(mission.EndAt)).(*pb.SCGetFinalReward)
	after, game := f.profile(t)
	if after.GetEnergy()-before.GetEnergy() != 2*(quote.Hours-1) || simpleOwned(after, game, "item_001") != 1 || response.GetExp() != quote.Exp {
		t.Fatal("actual cleared-risk reward or frozen local energy changed", before.GetEnergy(), after.GetEnergy(), response)
	}
	if !proto.Equal(response, p7ExpeditionCall(t, f, claim, 32, last.Add(time.Hour))) || p1Count(t, &db.P7ExpeditionHero{}, "player_id=?", f.id) != 0 {
		t.Fatal("cold/replay duplicated payment or retained actor")
	}
	detail = p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: proto.String(mission.Serial)}, 0, last).(*pb.SCExpeditionMissionDetail)
	if len(detail.Events) != 2*(int(quote.Hours)-1) {
		t.Fatal("real risk/help records lost", len(detail.Events))
	}
}

func TestP7ActualExpeditionUnresolvedRiskFailsWithOriginalExperienceAndNoFalseSuccessFact(t *testing.T) {
	f, mission, quote, helper, _ := p7ActualEventMission(t, 100)
	end := time.UnixMilli(mission.EndAt)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, err := f.s.ApplyP7ExpeditionHelp(tx, helper, f.id, mission.Serial, "nonexistent", end)
		return err
	}); err == nil {
		t.Fatal("expired task allowed help")
	}
	result := p7ExpeditionCall(t, f, &pb.CSGetFinalReward{MissionSerial: proto.String(mission.Serial)}, 33, end).(*pb.SCGetFinalReward)
	player, game := f.profile(t)
	if len(result.GetGet().GetSimps()) != 0 || result.GetExp() != quote.FailExp || simpleOwned(player, game, "item_001") != 0 {
		t.Fatal("unresolved risk paid successful rewards", result)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "expedition_complete") != 0 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "expedition_failed") != 1 {
		t.Fatal("failed mission manufactured success facts")
	}
	detail := p7ExpeditionCall(t, f, &pb.CSExpeditionMissionDetail{MissionSerial: proto.String(mission.Serial)}, 0, end).(*pb.SCExpeditionMissionDetail)
	if detail.Events[len(detail.Events)-1].GetEventType() != 5 || detail.Events[len(detail.Events)-1].GetStoryId() != "stroy_105" {
		t.Fatal("original failure story/type absent")
	}
}
