package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestInstanceLock51PersistenceProtectionAndExplicitUnlock(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	heroID, equipID := "", ""
	for _, row := range f.s.Tables["Hero"] {
		if row["quality"] == "13" && row["heroClass"] == "0" {
			heroID = row["id"]
			break
		}
	}
	for _, row := range f.s.Tables["Equip"] {
		if row["quality"] == "13" {
			equipID = row["id"]
			break
		}
	}
	if heroID == "" || equipID == "" {
		t.Fatal("original blue fixtures absent")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(34)
		_, err := f.s.GrantFixedReward(p, g, heroID+",2;"+equipID+",1;coin,10000")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	var heroes []*pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == heroID {
			heroes = append(heroes, w)
		}
	}
	if len(heroes) != 2 || len(g.Equipment) < 1 {
		t.Fatal("finite owned fixtures")
	}
	hero, equip := heroes[0], g.Equipment[0]
	var row db.Profile
	db.GetIns().First(&row, f.id)
	baseP, baseG := proto.Clone(p), *g
	before, _ := json.Marshal(baseG)
	state, err := SetInstanceLock(f.ctx, f.id, row.Session, hero.GetGuid(), true, 0)
	if err != nil || state.Revision != 1 {
		t.Fatal(state, err)
	}
	retry, err := SetInstanceLock(f.ctx, f.id, row.Session, hero.GetGuid(), true, 0)
	if err != nil || retry.Revision != 1 {
		t.Fatal("retry changed protection", retry, err)
	}
	if _, err = SetInstanceLock(f.ctx, f.id, row.Session, equip.GetGuid(), true, 0); err == nil {
		t.Fatal("stale version accepted")
	}
	state, err = SetInstanceLock(f.ctx, f.id, row.Session, equip.GetGuid(), true, 1)
	if err != nil || state.Revision != 2 {
		t.Fatal(state, err)
	}
	p, g = f.profile(t)
	baseG.KV = g.KV
	after, _ := json.Marshal(g)
	before, _ = json.Marshal(baseG)
	if !proto.Equal(baseP, p) || string(before) != string(after) {
		t.Fatal("lock changed assets or progress")
	}
	for _, guid := range []string{hero.GetGuid(), equip.GetGuid()} {
		if P5GuardInstance(p, g, guid) == nil {
			t.Fatal("locked instance unprotected")
		}
	}
	for _, decompose := range []bool{false, true} {
		request := &pb.CSWarriorSell{Guid: []string{hero.GetGuid()}}
		if _, err = f.s.p5WarriorRecycle(f.ctx, f.id, request.Guid, decompose, 51001, request); err == nil {
			t.Fatal("locked hero consumed")
		}
	}
	if _, err = f.s.p5EquipmentDecompose(f.ctx, f.id, &pb.CSEquipDecompose{Guid: []string{equip.GetGuid()}}, 51002); err == nil {
		t.Fatal("locked equipment decomposed")
	}
	if _, err = f.s.p5EquipmentSell(f.ctx, f.id, &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: equip.EquipId, Guid: equip.Guid, Num: proto.Int32(1)}}}, 51003); err == nil {
		t.Fatal("locked equipment sold")
	}
	feed := &pb.CSWarriorStrengthen{Guid: heroes[1].Guid, Cost: []*pb.BaseResource{{Id: hero.Id, Guid: hero.Guid}}}
	ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, feed)
	if _, err = f.s.p5WarriorStrengthen(ctx, f.id, feed, 51004); err == nil {
		t.Fatal("locked hero used as material")
	}
	if _, err = SetInstanceLock(f.ctx, f.id, make([]byte, 16), hero.GetGuid(), false, 2); err == nil {
		t.Fatal("stale session unlocked")
	}
	if _, err = SetInstanceLock(f.ctx, f.id, row.Session, "not-owned", true, 2); err == nil {
		t.Fatal("unowned instance locked")
	}
	f.restart(t)
	state, err = InstanceLockState(f.ctx, f.id, row.Session)
	if err != nil || state.Revision != 2 || len(state.LockedGUIDs) != 2 {
		t.Fatal("cold restore", state, err)
	}
	state, err = SetInstanceLock(f.ctx, f.id, row.Session, hero.GetGuid(), false, 2)
	if err != nil || state.Revision != 3 {
		t.Fatal("explicit unlock", state, err)
	}
	p, g = f.profile(t)
	if P5GuardInstance(p, g, hero.GetGuid()) != nil || P5GuardInstance(p, g, equip.GetGuid()) == nil {
		t.Fatal("unlock affected wrong instance")
	}
	if err = f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p5WarriorStrengthen(ctx, f.id, feed, 51005); err != nil {
		t.Fatal("unlocked blue material was unusable", err)
	}
	_, g = f.profile(t)
	if _, err = p5Warrior(g, hero.GetGuid()); err == nil {
		t.Fatal("explicitly unlocked material was not consumed")
	}
}
