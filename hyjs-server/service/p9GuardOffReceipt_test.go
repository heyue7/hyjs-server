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

// Move an actual same-ID guardian between two heroes. Its unchanged aggregate
// power reproduces the old off acknowledgement's captured-position ambiguity.
func TestP9GuardOffReceiptActualEqualPowerMoveAndCold(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitializeP9Pets, f.s.InitializeP9PetNext} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	f.s.P9PetsReady = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(34)
		p.NoviceGuideSchedule = proto.Int32(8)
		_, err := f.s.GrantFixedReward(p, g, "hero_011,1;frag_pet_002,50")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	firstReq := &pb.CSPetFirstRecruit{}
	first, err := f.s.p9PetFirst(ctx(firstReq), f.id, 194000, firstReq)
	if err != nil {
		t.Fatal(err)
	}
	a := first[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	combine := &pb.CSCombine{TargetId: proto.String("pet_002")}
	combined, err := f.s.p9PetCombine(ctx(combine), f.id, combine, 194001)
	if err != nil {
		t.Fatal(err)
	}
	b := combined[0].(*pb.SCCombine).GetGet().GetPets()[0]
	if a.GetId() != b.GetId() || a.GetGuid() == b.GetGuid() {
		t.Fatal("real distinct same-ID guardian sources missing")
	}
	_, g := f.profile(t)
	var hero *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_011" {
			hero = w
		}
	}
	heroOn := &pb.CSFormationOn{Id: hero.Id, Guid: hero.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.p5FormationOnNative(ctx(heroOn), f.id, heroOn, 194002); err != nil {
		t.Fatal(err)
	}
	on := func(pet *pb.Pet, pos int32, stamp int64) {
		t.Helper()
		request := &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(pos)}
		if _, err := f.s.p9GuardOn(ctx(request), f.id, request, stamp); err != nil {
			t.Fatal(err)
		}
	}
	off := func(pet *pb.Pet, pos int32, stamp int64) *pb.SCFormationOff {
		t.Helper()
		request := &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(pos)}
		out, err := f.s.p9GuardOff(ctx(request), f.id, request, stamp)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCFormationOff)
	}
	on(a, 1, 194003)
	on(b, 2, 194004)
	old := off(a, 1, 194005)
	f.restart(t)
	if !proto.Equal(old, off(a, 1, 194005)) {
		t.Fatal("current empty target cold acknowledgement changed")
	}
	off(b, 2, 194006)
	on(b, 1, 194007)
	p, g := f.profile(t)
	if p.GetFightPower() != old.GetFightpower() {
		t.Fatalf("equal-power original condition not reproduced: old=%d actual=%d", old.GetFightpower(), p.GetFightPower())
	}
	request := &pb.CSFormationOff{Id: a.Id, Guid: a.Guid, FormatPos: proto.Int32(1)}
	reject := func() {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, err := f.s.p9GuardOff(ctx(request), f.id, request, 194005); err == nil {
			t.Fatal("stale off cleared different current guardian at equal power")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("rejected guardian off changed profile")
		}
	}
	reject()
	f.restart(t)
	reject()
	_, g = f.profile(t)
	if !p9Guarded(g, b.GetGuid()) || p9Guarded(g, a.GetGuid()) {
		t.Fatal("actual guardian changed")
	}
	current := off(b, 1, 194008)
	f.restart(t)
	if !proto.Equal(current, off(b, 1, 194008)) {
		t.Fatal("new actual empty-target receipt rejected")
	}
}
