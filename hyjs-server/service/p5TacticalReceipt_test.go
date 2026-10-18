package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5TacticalReceiptActualSelectionMappingLevelAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		// Finite isolated unlock/growth materials; no historical win or awarded GUID is invented.
		p.Level = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "hero_011,1;item_373,6;item_372,5")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, b)
	}
	_, g := f.profile(t)
	var hero *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_011" {
			hero = w
		}
	}
	on := &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.p5FormationOnNative(ctx(on), f.id, on, 197000); err != nil {
		t.Fatal(err)
	}
	unlock := &pb.CSTaticalFormationUnLock{TacticalId: proto.String("2")}
	if _, err := f.s.tacticalFormationUnlock(ctx(unlock), f.id, unlock, 197001); err != nil {
		t.Fatal(err)
	}
	call := func(b proto.Message, stamp int64) ([]proto.Message, error) {
		out, handled, err := f.s.DispatchP5Next(ctx(b), f.id, b, stamp)
		if handled {
			return out, err
		}
		// Mirror the original dispatcher so the frozen predecessor reproduces the actual retired path.
		switch v := b.(type) {
		case *pb.CSUseTaticalFormation:
			return f.s.tacticalFormationUse(ctx(b), f.id, v)
		case *pb.CSTaticalFormationChange:
			return f.s.tacticalFormationChange(ctx(b), f.id, v)
		}
		t.Fatal("unexpected request")
		return nil, nil
	}
	reject := func(b proto.Message, stamp int64, message string) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		raw, _ := json.Marshal(beforeG)
		if _, err := call(b, stamp); err == nil {
			t.Fatal(message)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(raw) != string(after) {
			t.Fatal("old tactical packet modified locked profile")
		}
	}
	keep := func(b proto.Message, stamp int64, out proto.Message) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		raw, _ := json.Marshal(beforeG)
		again, err := call(b, stamp)
		if err != nil || !proto.Equal(out, again[0]) {
			t.Fatal("current tactical cold receipt", err)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(raw) != string(after) {
			t.Fatal("replay applied tactical action twice")
		}
	}
	use2 := &pb.CSUseTaticalFormation{Id: proto.String("2")}
	first, err := call(use2, 197002)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	keep(use2, 197002, first[0])
	use1 := &pb.CSUseTaticalFormation{Id: proto.String("1")}
	if _, err = call(use1, 197003); err != nil {
		t.Fatal(err)
	}
	reject(use2, 197002, "old tactical use selected the previous formation")
	f.restart(t)
	reject(use2, 197002, "old tactical use selected the previous formation")
	selected, err := call(use2, 197004)
	if err != nil {
		t.Fatal(err)
	}
	change := &pb.CSTaticalFormationChange{Id: proto.String("2"), Formation: proto.String("1,13;2,8")}
	mapped, err := call(change, 197005)
	if err != nil {
		t.Fatal(err)
	}
	reject(use2, 197004, "old use replayed cached positions after actual mapping")
	f.restart(t)
	keep(change, 197005, mapped[0])
	changed := &pb.CSTaticalFormationChange{Id: proto.String("2"), Formation: proto.String("1,7;2,9")}
	if _, err = call(changed, 197006); err != nil {
		t.Fatal(err)
	}
	reject(change, 197005, "old tactical change restored retired mapping")
	f.restart(t)
	reject(change, 197005, "old tactical change restored retired mapping")
	current, err := call(use2, 197007)
	if err != nil {
		t.Fatal(err)
	}
	_ = selected
	upgrade := &pb.CSTaticalFormationLevelUp{TacticalId: proto.String("2")}
	if _, err = f.s.tacticalFormationLevelUp(ctx(upgrade), f.id, upgrade, 197008); err != nil {
		t.Fatal(err)
	}
	reject(use2, 197007, "old use replayed cached skill level after actual upgrade")
	f.restart(t)
	reject(use2, 197007, "old use replayed cached skill level after actual upgrade")
	current, err = call(use2, 197009)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	keep(use2, 197009, current[0])
	p, g := f.profile(t)
	if g.TacticalSkillId != "7" {
		t.Fatal("current original formation skill", g.TacticalSkillId)
	}
	battle, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	poses := map[string]int32{}
	for _, actor := range battle.GetFormation() {
		poses[actor.GetGuid()] = actor.GetPose()
	}
	if poses[p.GetHeroWarId()] != 7 || poses[hero.GetGuid()] != 9 {
		t.Fatal("P6 actual source positions", poses)
	}
	for _, item := range g.Items {
		if item.GetId() == "item_373" || item.GetId() == "item_372" {
			if item.GetNum() != 0 {
				t.Fatal("original unlock/upgrade conservation", item)
			}
		}
	}
	if _, err = call(use1, 197010); err != nil {
		t.Fatal(err)
	}
	reject(changed, 197006, "old mapping response applied to different selected formation")
}
