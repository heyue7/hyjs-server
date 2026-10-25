package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sort"
	"testing"
	"time"
)

func TestExpedition69FormationBusyProjectionAndRelease(t *testing.T) {
	f := p7ExpeditionSetup(t)
	at := time.Now().Add(-13 * time.Hour)
	list := p7ExpeditionCall(t, f, &pb.CSGetRefreshedMissions{}, 0, at).(*pb.SCGetRefreshedMissions)
	first, second := list.GetRefreshedMissions()[0], list.GetRefreshedMissions()[1]
	p, g := f.profile(t)
	main := p.GetHeroWarId()
	guid := g.Warriors[1].GetGuid()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Formation = append(g.Formation, &pb.FormationItem{FormatPos: proto.Int32(5), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: proto.String(guid)}}})
		_, err := f.s.GrantFixedReward(p, g, "hero_exp_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// An unknown client-table hero cannot be projected into the native icon loop.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Warriors = append(g.Warriors, &pb.Warrior{Guid: proto.String("unknown69"), Id: proto.String("hero_unknown69")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	query := func(m *pb.ExpeditionMissionInfo) *pb.SCGetExpeditionIdleHeros {
		return p7ExpeditionCall(t, f, &pb.CSGetExpeditionIdleHeros{MissionSerial: m.MissionSerial}, 0, at).(*pb.SCGetExpeditionIdleHeros)
	}
	contains := func(values []string, target string) bool {
		for _, v := range values {
			if v == target {
				return true
			}
		}
		return false
	}
	idle := query(first)
	if !contains(idle.GetCanSendGuids(), guid) || contains(idle.GetCanSendGuids(), main) || contains(idle.GetCanSendGuids(), "unknown69") || !sort.StringsAreSorted(idle.GetCanSendGuids()) {
		t.Fatal("native selectable projection", idle)
	}
	_, g = f.profile(t)
	for _, h := range g.Warriors {
		if h.GetId() == "hero_exp_001" && contains(idle.GetCanSendGuids(), h.GetGuid()) {
			t.Fatal("material shown")
		}
	}
	request := &pb.CSAcceptMission{MissionSerial: first.MissionSerial, HeroGuids: []string{guid}}
	accepted := p7ExpeditionCall(t, f, request, 6901, at)
	if !proto.Equal(accepted, p7ExpeditionCall(t, f, request, 6901, at)) {
		t.Fatal("accept replay changed")
	}
	projected := query(second)
	if contains(projected.GetCanSendGuids(), guid) || !contains(projected.GetSendGuids(), guid) || !sort.StringsAreSorted(projected.GetSendGuids()) {
		t.Fatal("busy companion missing or selectable", projected)
	}
	if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSAcceptMission{MissionSerial: second.MissionSerial, HeroGuids: []string{guid}}, 6902, at); err == nil {
		t.Fatal("double dispatch")
	}
	if _, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSAcceptMission{MissionSerial: second.MissionSerial, HeroGuids: []string{main}}, 6903, at); err == nil {
		t.Fatal("main dispatched")
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		for _, h := range g.Warriors {
			if h.GetGuid() == guid {
				h.Level = proto.Int32(h.GetLevel() + 1)
			}
		}
		return f.s.GuardP7ExpeditionSave(tx, f.id, g)
	}); err == nil {
		t.Fatal("held snapshot mutated")
	}
	p, g = f.profile(t)
	if !p7ExpeditionInFormation(g, guid) {
		t.Fatal("dispatch removed formation")
	}
	var mission db.P7ExpeditionMission
	if err := db.GetIns().First(&mission, "serial=?", first.GetMissionSerial()).Error; err != nil {
		t.Fatal(err)
	}
	q, err := p7ReadExpeditionQuote(mission)
	if err != nil {
		t.Fatal(err)
	}
	if p.GetEnergy() != 100-q.Energy || q.Success == nil {
		t.Fatal("frozen success or energy changed")
	}
	claim := &pb.CSGetFinalReward{MissionSerial: first.MissionSerial}
	now := time.Now()
	reward := p7ExpeditionCall(t, f, claim, 6904, now)
	if !proto.Equal(reward, p7ExpeditionCall(t, f, claim, 6904, now)) {
		t.Fatal("claim replay changed")
	}
	idle = query(second)
	if !contains(idle.GetCanSendGuids(), guid) || contains(idle.GetSendGuids(), guid) {
		t.Fatal("completed hold not released", idle)
	}
}
