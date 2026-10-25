package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestTalentMaterialStateMatchesConsumedDonorAndRelease(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.P7ExpeditionHero{}); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(50000)
		gain, err := f.s.GrantFixedReward(p, g, "hero_004,2;item_005,800")
		if err != nil {
			return err
		}
		for i, item := range gain.GetWars() {
			w, e := p5Warrior(g, item.GetGuid())
			if e != nil {
				return e
			}
			if i == 0 {
				w.Guid = proto.String("target-yuuki")
				w.Level = proto.Int32(45)
				w.Talent = proto.Int32(4)
			} else {
				w.Guid = proto.String("donor-yuuki")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	db.GetIns().First(&profile, f.id)
	_, game := f.profile(t)
	donor, err := p5Warrior(game, "donor-yuuki")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := proto.Marshal(donor)
	if err := db.GetIns().Create(&db.P7ExpeditionHero{PlayerID: f.id, GUID: donor.GetGuid(), MissionSerial: "isolated-hold-release-fixture", Snapshot: snapshot}).Error; err != nil {
		t.Fatal(err)
	}
	before := func() string { p, g := f.profile(t); raw, _ := json.Marshal([]any{p, g}); return string(raw) }
	original := before()
	state, err := InstanceLockState(f.ctx, f.id, profile.Session)
	if err != nil || !strings.Contains(state.MaterialUnavailable["donor-yuuki"], "远征") {
		t.Fatal("authoritative material count reason", state, err)
	}
	if original != before() {
		t.Fatal("state read changed profile")
	}
	req := &pb.CSWarriorTalent{Guid: proto.String("target-yuuki"), Talent: proto.String("5"), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(50000)}, {Id: proto.String("item_005"), Num: proto.Int32(800)}, {Id: proto.String("hero_004"), Guid: proto.String("donor-yuuki"), Num: proto.Int32(1)}}}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, req)
	if _, err := f.s.p5WarriorTalent(ctx, f.id, req, 202610250050); err == nil || !strings.Contains(err.Error(), "远征") {
		t.Fatal("held donor must retain actionable refusal", err)
	}
	if original != before() {
		t.Fatal("rejected talent spent or changed cards")
	}
	// The release is a finite local fixture; full expedition reward behavior is unchanged.
	if err := db.GetIns().Where("player_id=? AND guid=?", f.id, "donor-yuuki").Delete(&db.P7ExpeditionHero{}).Error; err != nil {
		t.Fatal(err)
	}
	state, err = InstanceLockState(f.ctx, f.id, profile.Session)
	if err != nil || state.MaterialUnavailable["donor-yuuki"] != "" {
		t.Fatal("released donor still excluded", state, err)
	}
	out, err := f.s.p5WarriorTalent(ctx, f.id, req, 202610250050)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].(*pb.SCWarriorTalent).GetGuid().GetTalent() != 5 {
		t.Fatal("target talent not activated")
	}
	p, g := f.profile(t)
	if p.GetCoin() != 0 || simpleOwned(p, g, "item_005") != 0 {
		t.Fatal("original cost differs")
	}
	if _, err := p5Warrior(g, "donor-yuuki"); err == nil {
		t.Fatal("donor not consumed")
	}
	f.restart(t)
	saved := before()
	replay, err := f.s.p5WarriorTalent(ctx, f.id, req, 202610250050)
	if err != nil || !proto.Equal(replay[0], out[0]) || saved != before() {
		t.Fatal("cold retry changed costs", err)
	}
}
