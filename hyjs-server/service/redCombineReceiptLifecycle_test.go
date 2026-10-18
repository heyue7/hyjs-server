package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestRedCombineReceiptCurrentWarriorLifecycle(t *testing.T) {
	f := p4Setup(t)
	if e := f.s.InitializeP2(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeP5(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.WarriorBagSize = proto.Int32(1000)
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("frag_hero_077"), Num: proto.Int32(150)})
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	request := &pb.CSCombine{TargetId: proto.String("hero_077")}
	combined, e := f.s.redCombine(f.ctx, f.id, request, 1)
	if e != nil {
		t.Fatal(e)
	}
	war := combined[0].(*pb.SCCombine).GetGet().GetWars()[0]
	replay, e := f.s.redCombine(f.ctx, f.id, request, 1)
	if e != nil || !proto.Equal(combined[0], replay[0]) {
		t.Fatal("unchanged original receipt", e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		w, e := p5Warrior(g, war.GetGuid())
		if e != nil {
			return e
		}
		w.Level = proto.Int32(2)
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	p, g := f.profile(t)
	before, _ := json.Marshal(g)
	if _, e = f.s.redCombine(f.ctx, f.id, request, 1); e == nil {
		t.Fatal("old wholeWar would overwrite growth")
	}
	afterP, afterG := f.profile(t)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(p, afterP) || string(before) != string(after) {
		t.Fatal("rejected old receipt changed source/profile")
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		w, e := p5Warrior(g, war.GetGuid())
		if e != nil {
			return e
		}
		w.Level = proto.Int32(1)
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	// Same held GUID plus unchanged whole-warrior remains a safe inventory projection after formation changes.
	if _, e = f.s.redCombine(f.ctx, f.id, request, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.p5WarriorRecycle(f.ctx, f.id, []string{war.GetGuid()}, true, 2, &pb.CSWarriorDecompose{Guid: []string{war.GetGuid()}}); e != nil {
		t.Fatal(e)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	p, g = f.profile(t)
	before, _ = json.Marshal(g)
	if _, e = f.s.redCombine(f.ctx, f.id, request, 1); e == nil {
		t.Fatal("retired GUID would resurrect clientWar")
	}
	afterP, afterG = f.profile(t)
	after, _ = json.Marshal(afterG)
	if !proto.Equal(p, afterP) || string(before) != string(after) {
		t.Fatal("cold rejected old receipt mutated")
	}
	var count int64
	db.GetIns().Model(&db.RedP2Receipt{}).Where("player_id=? AND operation=?", f.id, constUtil.P2CombineOperation).Count(&count)
	if count != 1 {
		t.Fatal("original audit preserved", count)
	}
	t.Log("unchanged sameGUID originalreplay;grownWar refused no mutation;actualdecompose retiredGUID/cold refuses no source rerun/audit preserved")
}
