package service

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP7ExpeditionCatalogueAll90OriginalRegularRowsAndSSGate(t *testing.T) {
	f := p7ExpeditionSetup(t)
	cfg := db.P7ExpeditionPolicy{Reward: "item_001,1", Source: constUtil.P7ExpeditionCatalogueSource}
	counts := map[string]int{}
	for _, row := range f.s.Tables["ExpeditionMission"] {
		q, err := f.s.p7ExpeditionQuoteForCatalogue(row, cfg, true)
		if row["level"] == "SS" {
			if err == nil {
				t.Fatal("unknown SS published")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		counts[q.Grade]++
		rate := map[string]int32{"D": 10, "C": 20, "B": 30, "A": 40, "S": 50}[q.Grade]
		heroes := map[string]int32{"D": 1, "C": 1, "B": 2, "A": 2, "S": 3}[q.Grade]
		if q.Exp != q.Hours*rate || q.Energy != q.Hours*2 || q.MaxHeroes != heroes {
			t.Fatalf("original row projection %v", q)
		}
		if row["level"] != "D" {
			if _, err := f.s.p7ExpeditionQuote(row, cfg); err == nil {
				t.Fatal("disabled catalogue widened legacy quote")
			}
		}
	}
	for _, grade := range []string{"D", "C", "B", "A", "S"} {
		if counts[grade] != 18 {
			t.Fatal("missing original grade", counts)
		}
	}
}

func TestP7ExpeditionCatalogueEarnedUpgradeFrozenAcceptanceClosedCold(t *testing.T) {
	f := p7ExpeditionSetup(t)
	if err := db.GetIns().Model(&db.P7ExpeditionCataloguePolicy{}).Where("id = ?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// A finite just-before-upgrade prerequisite tests the actual completion
	// transition; it does not represent natural maximum-rank acquisition.
	at := time.Now().Add(-26 * time.Hour)
	state := db.P7ExpeditionState{PlayerID: f.id, Level: "D", Exp: 1199, Day: p1Day(at)}
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	d := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	_, g := f.profile(t)
	guid := g.Warriors[1].GetGuid()
	p7ExpeditionCall(t, f, &pb.CSAcceptMission{MissionSerial: d.RefreshedMissions[0].MissionSerial, HeroGuids: []string{guid}}, 1, at)
	var mission db.P7ExpeditionMission
	if err := db.GetIns().Where("serial = ?", d.RefreshedMissions[0].GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	q, err := p7ReadExpeditionQuote(mission)
	if err != nil {
		t.Fatal(err)
	}
	finish := p7ExpeditionCall(t, f, &pb.CSGetFinalReward{MissionSerial: d.RefreshedMissions[0].MissionSerial}, 2, at.Add(time.Duration(q.Hours)*time.Hour)).(*pb.SCGetFinalReward)
	if finish.GetLevel() != "C" || finish.GetExp() != q.Exp-1 {
		t.Fatal("committed mission did not earn C")
	}
	now := time.Now().Add(-13 * time.Hour)
	c := p7ExpeditionCall(t, f, &pb.CSRefreshMissionList{}, 3, now).(*pb.SCRefreshMissionList)
	if len(c.RefreshedMissions) != 3 {
		t.Fatal("C publication slots missing")
	}
	seen := map[string]bool{}
	for _, offer := range c.RefreshedMissions {
		if f.s.table("ExpeditionMission", "missionId", offer.GetMissionId())["level"] != "C" || seen[offer.GetMissionId()] {
			t.Fatal("wrong or repeated earned-grade task")
		}
		seen[offer.GetMissionId()] = true
	}
	chosen := c.RefreshedMissions[0]
	p7ExpeditionCall(t, f, &pb.CSAcceptMission{MissionSerial: chosen.MissionSerial, HeroGuids: []string{guid}}, 4, now)
	mission = db.P7ExpeditionMission{}
	if err := db.GetIns().Where("serial = ?", chosen.GetMissionSerial()).First(&mission).Error; err != nil {
		t.Fatal(err)
	}
	old, err := p7ReadExpeditionQuote(mission)
	if err != nil {
		t.Fatal(err)
	}
	if old.Grade != "C" || old.Exp != old.Hours*20 || !strings.Contains(old.Source, "local-earned-grade-expedition-v1") {
		t.Fatal("C quote missing actual source")
	}
	if err := db.GetIns().Model(&db.P7ExpeditionCataloguePolicy{}).Where("id = ?", 1).Updates(map[string]any{"enabled": false, "source": "closed future"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7ExpeditionPolicy{}).Where("id = ?", 1).Updates(map[string]any{"enabled": false, "reward": "diamond,999"}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := f.s.InitializeP7Expedition(f.ctx); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSGetFinalReward{MissionSerial: chosen.MissionSerial}
	result := p7ExpeditionCall(t, f, request, 5, now.Add(time.Duration(old.Hours)*time.Hour)).(*pb.SCGetFinalReward)
	if len(result.GetGet().GetSimps()) != 1 || result.GetGet().GetSimps()[0].GetId() != "item_001" || result.GetExp() != finish.GetExp()+old.Exp {
		t.Fatal("closed publication changed frozen reward or experience")
	}
	if !proto.Equal(result, p7ExpeditionCall(t, f, request, 5, now.Add(13*time.Hour))) {
		t.Fatal("cold retry changed receipt")
	}
}
