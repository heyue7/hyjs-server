package service

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestWorldSeedNativeGuideCurrentID uses the original level-13 request shape.
// One finite seed isolates the native boundary; it makes no supply/progression claim.
func TestWorldSeedNativeGuideCurrentID(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(13)
		_, err := f.s.GrantFixedReward(p, g, "item_017,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before, game := f.profile(t)
	main, err := p5Warrior(game, before.GetHeroWarId())
	if err != nil || main.GetId() != "hero_043" || before.GetCurrentWorldSeedId() != 1 {
		t.Fatal("original starter identity/current seed", err, main, before)
	}
	request := &pb.CSLightenWorldSeed{Id: proto.Int32(before.GetCurrentWorldSeedId())}
	invoke := func(r *pb.CSLightenWorldSeed, stamp int64) ([]proto.Message, error) {
		t.Helper()
		ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, r)
		messages, handled, err := f.s.DispatchP5(ctx, f.id, r, stamp)
		if !handled {
			t.Fatal("original world-seed dispatch was not handled")
		}
		return messages, err
	}
	first, err := invoke(request, 131101)
	if err != nil || len(first) != 2 {
		t.Fatal("native current-ID request rejected", err)
	}
	response, ok := first[0].(*pb.SCLightenWorldSeed)
	if !ok || !p1SameResources(response.Cost, []*pb.BaseResource{{Id: proto.String("item_017"), Num: proto.Int32(1)}}) {
		t.Fatal("original node-2 seed cost changed", first)
	}
	after, game := f.profile(t)
	if after.GetCurrentWorldSeedId() != 2 || simpleOwned(after, game, "item_017") != 0 || after.GetHeroWarId() != main.GetGuid() {
		t.Fatal("native callback level/material/identity mismatch", after)
	}
	assertUnchanged := func() {
		t.Helper()
		p, g := f.profile(t)
		if !proto.Equal(p, after) || simpleOwned(p, g, "item_017") != 0 {
			t.Fatal("retry or rejection changed current level/materials")
		}
	}
	for _, cold := range []bool{false, true} {
		if cold {
			f.restart(t)
		}
		replay, err := invoke(request, 131101)
		if err != nil || !proto.Equal(replay[0], first[0]) {
			t.Fatal("native same-stamp replay changed", cold, err)
		}
		assertUnchanged()
		// The next native request is legitimate but has no remaining seed.
		if _, err = invoke(&pb.CSLightenWorldSeed{Id: proto.Int32(2)}, 131102); err == nil {
			t.Fatal("next level accepted without the original seed cost", cold)
		}
		assertUnchanged()
		if _, err = invoke(request, 131103); err == nil {
			t.Fatal("new-stamp old current-ID request advanced twice", cold)
		}
		assertUnchanged()
	}
}
