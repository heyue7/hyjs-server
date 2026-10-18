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

func TestP5TreasureMaterialOriginalInheritedEXPTrackedResetRetryAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(16)
		_, err := f.s.GrantFixedReward(p, g, "coin,100000;diamond,100;trea_011,2;trea_013,2")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	_, initialG := f.profile(t)
	var rings, necklaces []*pb.Treasure
	for _, item := range initialG.Treasures {
		if item.GetTreasureId() == "trea_011" {
			rings = append(rings, item)
		}
		if item.GetTreasureId() == "trea_013" {
			necklaces = append(necklaces, item)
		}
	}
	if len(rings) != 2 || len(necklaces) != 2 {
		t.Fatal("finite original treasure fixture")
	}
	targetGUID, donorGUID := rings[0].GetGuid(), rings[1].GetGuid()
	feed := &pb.CSTreasureStrengthen{TargetId: proto.String(donorGUID), Guids: []string{necklaces[0].GetGuid()}}
	first, err := f.s.p5TreasureStrengthen(ctx(feed), f.id, feed, 771001)
	if err != nil {
		t.Fatal("first real donor growth", err)
	}
	donor := first[0].(*pb.SCTreasureStrengthen).GetTreasure()
	if donor.GetStrengthLevel() <= 1 {
		t.Fatal("donor did not actually grow")
	}
	p, beforeG := f.profile(t)
	if p.GetCoin() != 80000 {
		t.Fatal("original first 20000-EXP/coin fee", p.GetCoin())
	}
	if _, err = p5InvestmentState(beforeG, donorGUID, donor); err != nil {
		t.Fatal("actual donor investment missing", err)
	}
	// A later protected item must roll back the earlier donor audit/transfer,
	// original EXP calculation, money and removals as one existing profile transaction.
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+necklaces[1].GetGuid(), "1")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	failedP, failedG := f.profile(t)
	failedRaw, _ := json.Marshal(failedG)
	mixed := &pb.CSTreasureStrengthen{TargetId: proto.String(targetGUID), Guids: []string{donorGUID, necklaces[1].GetGuid()}}
	if _, err = f.s.p5TreasureStrengthen(ctx(mixed), f.id, mixed, 771002); err == nil {
		t.Fatal("locked late material accepted")
	}
	unchangedP, unchangedG := f.profile(t)
	unchangedRaw, _ := json.Marshal(unchangedG)
	if !proto.Equal(failedP, unchangedP) || string(failedRaw) != string(unchangedRaw) {
		t.Fatal("mixed batch partially transferred or charged")
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		p5SetKV(g, "p5.lock:"+necklaces[1].GetGuid(), "")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	strengthen := &pb.CSTreasureStrengthen{TargetId: proto.String(targetGUID), Guids: []string{donorGUID}}
	var wg sync.WaitGroup
	replies := make(chan proto.Message, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, e := f.s.p5TreasureStrengthen(ctx(strengthen), f.id, strengthen, 771002)
			if e != nil {
				failures <- e
				return
			}
			replies <- result[0]
		}()
	}
	wg.Wait()
	close(replies)
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	var original proto.Message
	for result := range replies {
		if original == nil {
			original = result
		} else if !proto.Equal(original, result) {
			t.Error("same-stamp result changed")
		}
	}
	if original == nil {
		t.Fatal("grown donor never accepted")
	}
	p, grownG := f.profile(t)
	target, err := f.s.treasureRecord(grownG, targetGUID)
	if err != nil {
		t.Fatal(err)
	}
	var storedEXP int64 = int64(target.GetCurrentExp())
	for level := int32(1); level < target.GetStrengthLevel(); level++ {
		n, e := f.s.treasureLevelExp(target, level)
		if e != nil {
			t.Fatal(e)
		}
		storedEXP += n
	}
	if storedEXP != 40000 || p.GetCoin() != 40000 {
		t.Fatal("original base20000 plus stored20000 EXP and exact fee", storedEXP, p.GetCoin())
	}
	ledger, err := p5InvestmentState(grownG, targetGUID, target)
	if err != nil {
		t.Fatal(err)
	}
	amounts := map[string]int32{}
	for _, cost := range ledger.Costs {
		amounts[cost.GetId()] += cost.GetNum()
	}
	if amounts["coin"] != 60000 || amounts["trea_011"] != 1 || amounts["trea_013"] != 1 || len(amounts) != 3 {
		t.Fatal("nested exact investment not flattened", amounts)
	}
	if p5KV(grownG, "p5.invest:"+donorGUID) != "" {
		t.Fatal("consumed donor still has refundable active ledger")
	}
	var audit p5TreasureMaterialAudit
	if json.Unmarshal([]byte(p5KV(grownG, constUtil.P5TreasureMaterialAuditPrefix+donorGUID)), &audit) != nil || audit.TargetGUID != targetGUID || audit.Policy != constUtil.P5TreasureMaterialPolicy {
		t.Fatal("consumed donor audit missing")
	}
	if _, err = f.s.p5TreasureStrengthen(ctx(feed), f.id, feed, 771001); err == nil {
		t.Fatal("consumed donor old response resurrected its GUID")
	}
	f.restart(t)
	if err = f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	cold, err := f.s.p5TreasureStrengthen(ctx(strengthen), f.id, strengthen, 771002)
	if err != nil || !proto.Equal(cold[0], original) {
		t.Fatal("cold growth receipt", err)
	}
	reset := &pb.CStreasureRebuild{TargetId: proto.String(targetGUID)}
	result, err := f.s.p5TreasureReset(ctx(reset), f.id, reset, 771003)
	if err != nil {
		t.Fatal("real target reset", err)
	}
	out := result[0].(*pb.SCtreasureRebuild)
	if out.GetCost()[0].GetId() != "diamond" || out.GetCost()[0].GetNum() != 50 {
		t.Fatal("original reset price")
	}
	p, resetG := f.profile(t)
	if p.GetCoin() != 100000 || p.GetDiamond() != 50 {
		t.Fatal("all real 60000 costs refunded exactly once", p.GetCoin(), p.GetDiamond())
	}
	var ringCount, necklaceCount int
	for _, item := range resetG.Treasures {
		if item.GetGuid() == donorGUID || item.GetGuid() == necklaces[0].GetGuid() {
			t.Fatal("refund reused consumed GUID")
		}
		if item.GetTreasureId() == "trea_011" {
			ringCount++
		}
		if item.GetTreasureId() == "trea_013" {
			necklaceCount++
		}
		if item.GetStrengthLevel() != 1 || item.GetRefineLevel() != 0 || item.GetCurrentExp() != 0 {
			t.Fatal("refund did not restore pristine units")
		}
	}
	if ringCount != 2 || necklaceCount != 2 {
		t.Fatal("nested instance costs lost or duplicated", ringCount, necklaceCount)
	}
	_, beforeReplay := f.profile(t)
	beforeRaw, _ := json.Marshal(beforeReplay)
	if _, err = f.s.p5TreasureReset(ctx(reset), f.id, reset, 771003); err != nil {
		t.Fatal("same reset receipt", err)
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(afterP, p) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("reset retry refunded twice")
	}
	if _, err = f.s.p5TreasureStrengthen(ctx(strengthen), f.id, strengthen, 771002); err == nil {
		t.Fatal("pre-reset growth projection accepted")
	}
}

func TestP5TreasureMaterialMissingTamperedRefinedLockedAndForeignProtected(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	p := &pb.Player{}
	baseline := &pb.Treasure{Guid: proto.String("donor"), TreasureId: proto.String("trea_011"), StrengthLevel: proto.Int32(1), RefineLevel: proto.Int32(0), CurrentExp: proto.Int32(0)}
	current := proto.Clone(baseline).(*pb.Treasure)
	current.StrengthLevel, current.CurrentExp = proto.Int32(2), proto.Int32(10)
	request := &pb.CSTreasureStrengthen{TargetId: proto.String("target"), Guids: []string{"donor"}}
	for _, mode := range []string{"missing", "fingerprint", "partial-origin", "refined", "locked", "foreign-request", "wrong-consumer", "already-transferred", "unsupported-cost"} {
		t.Run(mode, func(t *testing.T) {
			material := proto.Clone(current).(*pb.Treasure)
			before := &model.GameState{Treasures: []*pb.Treasure{material}}
			if err := RecordP5Investment(before, "donor", baseline, material, []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1200)}}); err != nil {
				t.Fatal(err)
			}
			var req proto.Message = request
			switch mode {
			case "missing":
				p5SetKV(before, "p5.invest:donor", "")
			case "fingerprint":
				material.CurrentExp = proto.Int32(11)
			case "refined":
				material.RefineLevel = proto.Int32(1)
			case "locked":
				p5SetKV(before, "p5.lock:donor", "1")
			case "foreign-request":
				req = &pb.CSTreasureStrengthen{TargetId: proto.String("target"), Guids: []string{"other"}}
			case "wrong-consumer":
				req = &pb.CSTreasureRefine{TargetId: proto.String("target")}
			case "partial-origin", "unsupported-cost":
				state, err := p5InvestmentState(before, "donor", material)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "partial-origin" {
					grownBase := proto.Clone(baseline).(*pb.Treasure)
					grownBase.StrengthLevel = proto.Int32(2)
					state.Baseline, _ = json.Marshal(grownBase)
				} else {
					state.Costs[0].Id = proto.String("unknown-resource")
				}
				raw, _ := json.Marshal(state)
				p5SetKV(before, "p5.invest:donor", string(raw))
			case "already-transferred":
				p5SetKV(before, constUtil.P5TreasureMaterialAuditPrefix+"donor", "old-audit")
			}
			encoded, _ := json.Marshal(before)
			after := &model.GameState{}
			if err := json.Unmarshal(encoded, after); err != nil {
				t.Fatal(err)
			}
			after.Treasures = nil
			unchanged, _ := json.Marshal(after)
			if _, err := s.p5TreasureMaterialCosts(p, before, after, material, "target", req); err == nil {
				t.Fatal("untracked/invalid donor accepted")
			}
			currentRaw, _ := json.Marshal(after)
			if string(currentRaw) != string(unchanged) {
				t.Fatal("failed validation changed audit or ledger")
			}
		})
	}
}
