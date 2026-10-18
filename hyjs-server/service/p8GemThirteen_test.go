package service

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Thirty-six grade7 gems and historical craft10 are finite qualifications.
// The original 6×7→10 and 6×10→13 operations supply the actual final gem.
func TestP8OriginalRainbowThirteenSixCombineLifecycle(t *testing.T) {
	f := p1Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		_, e := f.s.GrantFixedReward(p, g, "equip_037,1;MineItem_079,36")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Create(&db.GemCraftState{PlayerID: f.id, Level: 10}).Error; e != nil {
		t.Fatal(e)
	}
	ctx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	reject := func(req proto.Message, call func() ([]proto.Message, error)) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, _ := json.Marshal(beforeG)
		if _, e := call(); e == nil {
			t.Fatal("invalid highgem allowed", req)
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("failedhighgem mutated profile")
		}
	}
	high := &pb.CSGemCombineBySix{GemId: proto.String("MineItem_106")}
	reject(high, func() ([]proto.Message, error) { return f.s.gemCombineSix(ctx(high), f.id, high, 8000) })
	middle := &pb.CSGemCombineBySix{GemId: proto.String("MineItem_080")}
	for i := int64(0); i < 6; i++ {
		out, e := f.s.gemCombineSix(ctx(middle), f.id, middle, 8100+i)
		if e != nil {
			t.Fatal("actual7→10", e)
		}
		v := out[0].(*pb.SCGemCombineBySix)
		if len(v.GetCost()) != 1 || v.GetCost()[0].GetId() != "MineItem_079" || v.GetCost()[0].GetNum() != 6 || v.GetRes().GetSimps()[0].GetId() != "MineItem_080" || v.GetRes().GetSimps()[0].GetNum() != 1 {
			t.Fatal("exactmiddle rawcost/result", v)
		}
	}
	if e := db.GetIns().Model(&db.GemCraftState{}).Where("player_id=?", f.id).Update("level", 9).Error; e != nil {
		t.Fatal(e)
	}
	reject(high, func() ([]proto.Message, error) { return f.s.gemCombineSix(ctx(high), f.id, high, 8200) })
	if e := db.GetIns().Model(&db.GemCraftState{}).Where("player_id=?", f.id).Update("level", 10).Error; e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.gemCombineSix(ctx(high), f.id, high, 8201)
			errs <- e
			if e == nil {
				replies <- out[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(replies)
	for e := range errs {
		if e != nil {
			t.Fatal("actual10→13", e)
		}
	}
	var award proto.Message
	for out := range replies {
		if award == nil {
			award = out
		}
		if !proto.Equal(out, award) {
			t.Fatal("eight highgem diverged")
		}
	}
	_, g := f.profile(t)
	counts := map[string]int32{}
	for _, v := range g.Gems {
		counts[v.GetId()] += v.GetNum()
	}
	if counts["MineItem_079"] != 0 || counts["MineItem_080"] != 0 || counts["MineItem_106"] != 1 {
		t.Fatal("36→6→1 gemconservation", counts)
	}
	f.restart(t)
	if out, e := f.s.gemCombineSix(ctx(high), f.id, high, 8201); e != nil || !proto.Equal(out[0], award) {
		t.Fatal("highgem cold once", e)
	}
	makeHigh := &pb.CSGemMake{GemId: high.GemId, Num: proto.Int32(1)}
	reject(makeHigh, func() ([]proto.Message, error) { return f.s.gemMake(ctx(makeHigh), f.id, makeHigh, 8300) })
	gear := g.Equipment[0]
	if _, e := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Guid: gear.Guid, Id: gear.EquipId, FormatPos: proto.Int32(1)}); e != nil {
		t.Fatal(e)
	}
	p, g := f.profile(t)
	before, e := f.s.battlePlayer(p, g)
	if e != nil {
		t.Fatal(e)
	}
	mount := &pb.CSGemMount{Guid: gear.Guid, GemId: high.GemId, Pos: proto.Int32(0)}
	if _, e := f.s.gemMount(ctx(mount), f.id, mount, 8400); e != nil {
		t.Fatal("actualgrade13 mount", e)
	}
	p, g = f.profile(t)
	after, e := f.s.battlePlayer(p, g)
	if e != nil {
		t.Fatal(e)
	}
	left, right := before.GetFormation()[0].GetProp(), after.GetFormation()[0].GetProp()
	actual := []float64{right.GetAtt() - left.GetAtt(), right.GetHp() - left.GetHp(), right.GetPdef() - left.GetPdef(), right.GetMdef() - left.GetMdef(), right.GetCri() - left.GetCri(), right.GetAntiCri() - left.GetAntiCri()}
	expected := []float64{3600, 13680, 1080, 1080, 5.4, 5.4}
	for i, v := range actual {
		if math.Abs(v-expected[i]) > 1e-7 {
			t.Fatal("six originalgrade13 effects", actual)
		}
	}
	unmount := &pb.CSGemUnmount{Guid: gear.Guid, Pos: proto.Int32(0)}
	if _, e := f.s.gemUnmount(ctx(unmount), f.id, unmount, 8401); e != nil {
		t.Fatal(e)
	}
	f.restart(t)
	if out, e := f.s.gemCombineSix(ctx(high), f.id, high, 8201); e != nil || !proto.Equal(out[0], award) {
		t.Fatal("simple highgem receipt afterreal use/cold", e)
	}
	reject(mount, func() ([]proto.Message, error) { return f.s.gemMount(ctx(mount), f.id, mount, 8400) })
	_, g = f.profile(t)
	if gemCount(g, "MineItem_106") != 1 {
		t.Fatal("grade13 physicalreturn")
	}
}
