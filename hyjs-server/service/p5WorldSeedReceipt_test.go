package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The material/level preparation is finite qualification, not natural seed supply.
// Actual native nodes and paid growth demonstrate the stale original UI projection.
func TestP5WorldSeedReceiptCurrentNodeIdentityGrowthAndCold(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(1000000)
		p.WarriorBagSize = proto.Int32(1000)
		_, e := f.s.GrantFixedReward(p, g, "item_017,25;item_007,4")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	ctx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	call := func(node int32) (proto.Message, *pb.CSLightenWorldSeed) {
		t.Helper()
		r := &pb.CSLightenWorldSeed{Id: proto.Int32(node)}
		out, e := f.s.lightenWorldSeed(ctx(r), f.id, r, int64(node)+810000)
		if e != nil {
			t.Fatal("actual original seed node", node, e)
		}
		return out[0], r
	}
	var purple proto.Message
	var purpleRequest *pb.CSLightenWorldSeed
	var oldSimple *pb.CSLightenWorldSeed
	for node := int32(2); node <= 10; node++ {
		out, r := call(node)
		if node == 9 {
			oldSimple = r
		}
		if node == 10 {
			purple, purpleRequest = out, r
		}
	}
	p, g := f.profile(t)
	main, e := p5Warrior(g, p.GetHeroWarId())
	if e != nil || main.GetId() != "hero_044" {
		t.Fatal("original purple transition", main, e)
	}
	guid := main.GetGuid()
	// Concurrent retry returns the unchanged original body and never another cost.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.lightenWorldSeed(ctx(purpleRequest), f.id, purpleRequest, 810010)
			if e == nil && !proto.Equal(out[0], purple) {
				t.Error("same-node body changed")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	f.restart(t)
	if out, e := f.s.lightenWorldSeed(ctx(purpleRequest), f.id, purpleRequest, 810010); e != nil || !proto.Equal(out[0], purple) {
		t.Fatal("unchanged current-node cold replay", e)
	}
	grow := &pb.CSWarriorTraining{Guid: main.Guid, CostType: proto.Int32(2), NumType: proto.Int32(1)}
	if _, e = f.s.warriorTraining(ctx(grow), f.id, grow, 820001); e != nil {
		t.Fatal("actual paid main training", e)
	}
	unchanged := func(req *pb.CSLightenWorldSeed) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, e := f.s.lightenWorldSeed(ctx(req), f.id, req, int64(req.GetId())+810000); e == nil {
			t.Fatal("stale original seed callback accepted", req)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("rejected old seed changed save")
		}
	}
	unchanged(purpleRequest)
	unchanged(oldSimple)
	f.restart(t)
	unchanged(purpleRequest)
	var orange proto.Message
	var orangeRequest *pb.CSLightenWorldSeed
	for node := int32(11); node <= 25; node++ {
		orange, orangeRequest = call(node)
	}
	p, g = f.profile(t)
	main, e = p5Warrior(g, guid)
	if e != nil || main.GetId() != "hero_045" || p.GetCurrentWorldSeedId() != 25 || simpleOwned(p, g, "item_017") != 1 {
		t.Fatal("original orange/current seed/material conservation", main, p.GetCurrentWorldSeedId(), simpleOwned(p, g, "item_017"), e)
	}
	unchanged(purpleRequest)
	f.restart(t)
	if out, e := f.s.lightenWorldSeed(ctx(orangeRequest), f.id, orangeRequest, 810025); e != nil || !proto.Equal(out[0], orange) {
		t.Fatal("current original orange cold replay", e)
	}
	unchanged(oldSimple)
	t.Log("24 actual nodes/24 finite seeds, 3000coin/four actual training materials, purple/orange sameGUID,8once,current cold replay,grown/prior node cold reject passed; no natural84-source claim")
}
