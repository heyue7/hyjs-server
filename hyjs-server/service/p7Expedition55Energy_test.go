package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestP7Expedition55EnergyAbsoluteCommitReplayAndCold(t *testing.T) {
	f := p7ExpeditionSetup(t)
	now := time.Now()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Energy = proto.Int32(30)
		p.EnergyLastRefreshTime = proto.Int64(now.UnixMilli())
		_, err := f.s.GrantFixedReward(p, g, "hero_003,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	call := func(req proto.Message, stamp int64, want int32) {
		t.Helper()
		out, err := f.s.p7Expedition(f.ctx, f.id, req, stamp, now)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 {
			t.Fatal("missing absolute balance", len(out))
		}
		snapshot, ok := out[1].(*pb.SCPlayer)
		if !ok || snapshot.GetPlayer().GetEnergy() != want || snapshot.GetPlayer().GetEnergyLastRefreshTime() <= 0 {
			t.Fatal("wrong current absolute balance", out[1])
		}
	}
	call(&pb.CSInterExpedition{}, 0, 30)
	// Finite C capacity isolates the energy check from the unrelated D two-task limit.
	if err := db.GetIns().Model(&db.P7ExpeditionState{}).Where("player_id=?", f.id).Update("level", "C").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("player_id=? AND state=0", f.id).Delete(&db.P7ExpeditionMission{}).Error; err != nil {
		t.Fatal(err)
	}
	var cfg db.P7ExpeditionPolicy
	if err := db.GetIns().First(&cfg, 1).Error; err != nil {
		t.Fatal(err)
	}
	for i, hours := range []int32{4, 8, 4} {
		var row map[string]string
		for _, candidate := range f.s.Tables["ExpeditionMission"] {
			if candidate["level"] == "D" && candidate["time"] == fmt.Sprint(hours) {
				row = candidate
				break
			}
		}
		q, err := f.s.p7ExpeditionQuote(row, cfg)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(q)
		mission := db.P7ExpeditionMission{Serial: fmt.Sprintf("energy55-%d", i), PlayerID: f.id, MissionID: row["missionId"], Quote: raw, QuoteHash: p6Digest(raw)}
		if err := db.GetIns().Create(&mission).Error; err != nil {
			t.Fatal(err)
		}
	}
	player, g := f.profile(t)
	var heroes []string
	for _, hero := range g.Warriors {
		if hero.GetGuid() != player.GetHeroWarId() && !p7ExpeditionInFormation(g, hero.GetGuid()) {
			heroes = append(heroes, hero.GetGuid())
		}
	}
	if len(heroes) < 3 {
		t.Fatal("need three independent fixture heroes")
	}
	first := &pb.CSAcceptMission{MissionSerial: proto.String("energy55-0"), HeroGuids: []string{heroes[0]}}
	second := &pb.CSAcceptMission{MissionSerial: proto.String("energy55-1"), HeroGuids: []string{heroes[1]}}
	call(first, 550601, 22)
	call(second, 550602, 6)
	call(&pb.CSInterExpedition{}, 0, 6)
	call(first, 550601, 6)
	before, beforeGame := f.profile(t)
	stable, _ := json.Marshal([]any{before, beforeGame})
	var busyBefore int64
	db.GetIns().Model(&db.P7ExpeditionHero{}).Where("player_id=?", f.id).Count(&busyBefore)
	_, err := f.s.p7Expedition(f.ctx, f.id, &pb.CSAcceptMission{MissionSerial: proto.String("energy55-2"), HeroGuids: []string{heroes[2]}}, 550603, now)
	if err == nil || !strings.Contains(err.Error(), "远征耐力不足") {
		t.Fatal("insufficient energy was accepted or wrong gate", err)
	}
	after, afterGame := f.profile(t)
	unchanged, _ := json.Marshal([]any{after, afterGame})
	if string(stable) != string(unchanged) {
		t.Fatal("rejected accept changed assets")
	}
	var busyAfter int64
	db.GetIns().Model(&db.P7ExpeditionHero{}).Where("player_id=?", f.id).Count(&busyAfter)
	if busyBefore != busyAfter {
		t.Fatal("rejected accept occupied hero")
	}
	var offered db.P7ExpeditionMission
	if err := db.GetIns().First(&offered, "serial=?", "energy55-2").Error; err != nil || offered.State != 0 {
		t.Fatal("rejected offer advanced", err)
	}
	f.restart(t)
	call(&pb.CSInterExpedition{}, 0, 6)
	call(first, 550601, 6)
}
