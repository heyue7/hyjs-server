package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The reported first-task win is an isolated settlement contract fixture, not
// restored original battle-engine authority. Its earned instance is then sold
// through the unchanged original recycling operation.
func TestP5CopyAwardFirstDropSoldColdReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.WarriorBagSize = proto.Int32(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.copyEnter(f.ctx, f.id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
		t.Fatal(err)
	}
	starts, err := f.s.copyFightStart(f.ctx, f.id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")})
	if err != nil {
		t.Fatal(err)
	}
	start := starts[0].(*pb.SCCopyFightStart)
	end := &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: start.BattlePlayer}
	out, err := f.s.copyFightEnd(f.ctx, f.id, end)
	if err != nil {
		t.Fatal(err)
	}
	reply := out[0].(*pb.SCCopyFightEnd)
	if len(reply.GetGain().GetWars()) != 1 || reply.GetGain().GetWars()[0].GetId() != "hero_exp_006" {
		t.Fatal("original guaranteed first drop missing", reply)
	}
	f.restart(t)
	if again, err := f.s.copyFightEnd(f.ctx, f.id, end); err != nil || !proto.Equal(reply, again[0]) {
		t.Fatal("held first source cold replay", err)
	}
	w := reply.GetGain().GetWars()[0]
	sell := &pb.CSWarriorSell{Guid: []string{w.GetGuid()}}
	ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, sell)
	if _, err := f.s.p5WarriorRecycle(ctx, f.id, sell.Guid, false, 181001, sell); err != nil {
		t.Fatal("actual source material sell", err)
	}
	beforeP, beforeG := f.profile(t)
	beforeRaw, _ := json.Marshal(beforeG)
	if _, err := f.s.copyFightEnd(f.ctx, f.id, end); err == nil {
		t.Fatal("sold first-drop instance resurrected")
	}
	f.restart(t)
	if _, err := f.s.copyFightEnd(f.ctx, f.id, end); err == nil {
		t.Fatal("sold first-drop instance resurrected cold")
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("old rejected end modified save")
	}
}

func TestP5CopySweepSourceSoldColdReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Original prior five-star and level eligibility are finite prerequisites;
	// the sweep stamina cost and randomized original material are real actions.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(14)
		p.WarriorBagSize = proto.Int32(1000)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5), ChallengeTimes: proto.Int32(0), IsOpen: proto.Int32(1)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	out, err := f.s.copySweep(f.ctx, f.id, req, 182000)
	if err != nil {
		t.Fatal(err)
	}
	reply := out[0].(*pb.SCCopySweep)
	if len(reply.GetGain().GetWars()) < 1 {
		t.Fatal("actual ordinary sweep source missing")
	}
	f.restart(t)
	if again, err := f.s.copySweep(f.ctx, f.id, req, 182000); err != nil || !proto.Equal(reply, again[0]) {
		t.Fatal("held sweep current cold", err)
	}
	w := reply.GetGain().GetWars()[0]
	sell := &pb.CSWarriorSell{Guid: []string{w.GetGuid()}}
	ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, sell)
	if _, err := f.s.p5WarriorRecycle(ctx, f.id, sell.Guid, false, 182001, sell); err != nil {
		t.Fatal("actual sweep source sell", err)
	}
	beforeP, beforeG := f.profile(t)
	beforeRaw, _ := json.Marshal(beforeG)
	if _, err := f.s.copySweep(f.ctx, f.id, req, 182000); err == nil {
		t.Fatal("sold sweep instance resurrected")
	}
	f.restart(t)
	if _, err := f.s.copySweep(f.ctx, f.id, req, 182000); err == nil {
		t.Fatal("sold sweep instance resurrected cold")
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("old sweep changed save/cost/quota")
	}
}

func TestP5CopyAwardPetOriginalWholeAndLegacyProjection(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetIntimacy, f.s.InitializeP9CopyIntimacy} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(34)
		_, err := f.s.GrantFixedReward(p, g, "item_129,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out, _, err := f.s.DispatchP9Pets(f.ctx, f.id, &pb.CSPetFirstRecruit{}, 183000)
	if err != nil {
		t.Fatal(err)
	}
	pet := out[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	if _, _, err := f.s.DispatchP9PetNext(f.ctx, f.id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 183001); err != nil {
		t.Fatal(err)
	}
	var adds []*pb.PetAddIntimacy
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var err error
		adds, err = f.s.ApplyP9CopyIntimacy(tx, f.id, p, g, "copy:isolated-contract-copy-award")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(adds) != 1 || adds[0].GetCurExp() != 2 {
		t.Fatal("original two-earned projection missing")
	}
	full := &pb.SCCopySweep{Gain: &pb.ResultObject{}, PetAdds: adds}
	legacy := proto.Clone(full).(*pb.SCCopySweep)
	legacy.PetAdds[0].Pet = nil
	check := func(reply proto.Message, wantOK bool) {
		t.Helper()
		err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			return f.s.ValidateCopyAwardReceiptCurrent(tx, f.id, g, reply)
		})
		if (err == nil) != wantOK {
			t.Fatal("projection guard", wantOK, err)
		}
	}
	f.restart(t)
	check(full, true)
	check(legacy, true)
	if _, _, err := f.s.DispatchP9PetIntimacy(f.ctx, f.id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(1)}}}, 183002); err != nil {
		t.Fatal("actual original food growth", err)
	}
	check(full, false)
	check(legacy, false)
	f.restart(t)
	check(full, false)
	check(legacy, false)
	// Pure currency receipts have no incremental entity projection to overwrite.
	check(&pb.SCCopySweep{Gain: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(7)}}}}, true)
}
