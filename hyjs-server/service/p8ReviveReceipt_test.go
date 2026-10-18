package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Death is an explicit finite survival contract fixture. This test never
// claims an original combat-engine result or changes battle authority.
func TestP8ReviveReceiptCurrentDeadAndGatheringCold(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeP8Gather, f.s.InitializeP8Life, f.s.InitGems, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(18)
		p.Diamond = proto.Int64(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	death := func() {
		t.Helper()
		if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
			g.Position.Current = &pb.PointNew{X: proto.Float32(1000), Y: proto.Float32(0), Z: proto.Float32(1000)}
			g.Field.Fight = nil
			g.Field.Side.CurHp = proto.Float64(0)
			g.Field.Side.BattlePosHps = nil
			g.Field.Side.State = proto.Int32(constUtil.P8FieldDead)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(request proto.Message, stamp int64) proto.Message {
		t.Helper()
		out, _, err := f.s.DispatchP8Life(f.ctx, f.id, request, stamp)
		if err != nil {
			t.Fatal(err)
		}
		return out[0]
	}
	deny := func(request proto.Message, stamp int64) {
		t.Helper()
		p, g := f.profile(t)
		before, _ := json.Marshal(g)
		if _, _, err := f.s.DispatchP8Life(f.ctx, f.id, request, stamp); err == nil {
			t.Fatal("old revival state overwrote changed actor")
		}
		afterP, afterG := f.profile(t)
		after, _ := json.Marshal(afterG)
		if !proto.Equal(p, afterP) || string(before) != string(after) {
			t.Fatal("rejected revival changed wallet/state")
		}
	}
	death()
	paid := &pb.CSResurrection{}
	receipt := call(paid, 184000).(*pb.SCResurrenttion)
	if receipt.GetCost()[0].GetNum() != 50 {
		t.Fatal("original paid cost")
	}
	f.restart(t)
	if !proto.Equal(call(paid, 184000), receipt) {
		t.Fatal("current paid cold body changed")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Field.Side.Kills = proto.Int32(g.Field.Side.GetKills() + 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	deny(paid, 184000)
	death()
	deny(paid, 184000)
	f.restart(t)
	deny(paid, 184000)
	// Use the actual free request and original safe point after only its
	// dedicated isolation clock advances past the existing ten-second limit.
	call(&pb.CSGetHp{}, 184001)
	if err := db.GetIns().Model(&db.P8LifeState{}).Where("player_id=?", f.id).Update("death_at", time.Now().UnixMilli()-10001).Error; err != nil {
		t.Fatal(err)
	}
	free := &pb.CSTransfer2Safe{}
	freeReceipt := call(free, 184002)
	f.restart(t)
	if !proto.Equal(call(free, 184002), freeReceipt) {
		t.Fatal("current free cold changed")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Field.Side.State = proto.Int32(constUtil.P8FieldMining)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	deny(free, 184002)
	death()
	deny(free, 184002)
	f.restart(t)
	deny(free, 184002)
	p, _ := f.profile(t)
	if p.GetDiamond() != 50 {
		t.Fatal("rejected old packets changed original50diamond cost")
	}
}
