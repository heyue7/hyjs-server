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

// A legacy instance can receive a new, correctly tracked upgrade without the
// new ledger proving the earlier investment. Reset must keep all of it intact.
func TestP5ResetRejectsIncompleteEquipmentInvestment(t *testing.T) {
	f := legacyResetFixture(t)
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(40), proto.Int64(100000), proto.Int64(100)
		gain, err := f.s.GrantFixedReward(p, g, "equip_037,1")
		if err != nil {
			return err
		}
		guid = gain.GetEquips()[0].GetGuid()
		legacy, err := f.s.equipmentRecord(g, guid)
		if err != nil {
			return err
		}
		legacy.Level = proto.Int32(2)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	upgrade := &pb.CSEquipStrengthenOneKey{Guid: proto.String(guid)}
	if _, err := f.s.p5StrengthenOne(legacyResetContext(f, upgrade), f.id, upgrade, 991001); err != nil {
		t.Fatal("tracked growth after legacy progress", err)
	}
	_, beforeG := f.profile(t)
	current, err := f.s.equipmentRecord(beforeG, guid)
	if err != nil || current.GetLevel() <= 2 || p5KV(beforeG, "p5.invest:"+guid) == "" {
		t.Fatal("fixture did not create a later tracked upgrade", current, err)
	}
	reset := &pb.CSEquipReBuild{Guid: proto.String(guid)}
	assertLegacyResetRejected(t, f, "equipment-reset", 991002, func() error {
		_, err := f.s.p5EquipmentReset(legacyResetContext(f, reset), f.id, reset, 991002)
		return err
	})
}

func legacyResetFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitGems, f.s.InitEquipmentMutations} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	return f
}

func legacyResetContext(f *p1Fixture, request proto.Message) context.Context {
	return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
}

func assertLegacyResetRejected(t *testing.T, f *p1Fixture, operation string, stamp int64, reset func() error) {
	t.Helper()
	beforeP, beforeG := f.profile(t)
	before, err := json.Marshal(beforeG)
	if err != nil {
		t.Fatal(err)
	}
	if err := reset(); err == nil {
		t.Fatal("reset accepted unproven earlier investment")
	}
	afterP, afterG := f.profile(t)
	after, err := json.Marshal(afterG)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("rejected reset changed currency, inventory, instance or ledger")
	}
	var receipts int64
	if err := db.GetIns().Model(&db.P5MutationReceipt{}).Where("player_id=? AND operation=? AND timestamp=?", f.id, operation, stamp).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatal("rejected reset persisted a success receipt", receipts, err)
	}
	f.restart(t)
	if err := reset(); err == nil {
		t.Fatal("cold retry accepted unproven earlier investment")
	}
	coldP, coldG := f.profile(t)
	cold, err := json.Marshal(coldG)
	if err != nil || !proto.Equal(beforeP, coldP) || string(before) != string(cold) {
		t.Fatal("cold retry changed currency, inventory, instance or ledger", err)
	}
}

func TestP5ResetRejectsIncompleteWarriorInvestment(t *testing.T) {
	f := legacyResetFixture(t)
	var target, food *pb.Warrior
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(40), proto.Int64(100000), proto.Int64(100)
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,1;hero_exp_003,1")
		if err != nil {
			return err
		}
		for _, w := range gain.GetWars() {
			if w.GetId() == "hero_011" {
				target = w
			} else if w.GetId() == "hero_exp_003" {
				food = w
			}
		}
		owned, err := p5Warrior(g, target.GetGuid())
		if err != nil {
			return err
		}
		owned.Level = proto.Int32(2)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	upgrade := &pb.CSWarriorStrengthen{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: food.Id, Guid: food.Guid, Num: proto.Int32(1)}}}
	if _, err := f.s.p5WarriorStrengthen(legacyResetContext(f, upgrade), f.id, upgrade, 992001); err != nil {
		t.Fatal("tracked growth after legacy progress", err)
	}
	_, g := f.profile(t)
	if p5KV(g, "p5.invest:"+target.GetGuid()) == "" {
		t.Fatal("tracked upgrade did not create a ledger")
	}
	reset := &pb.CSWarriorReset{Guid: target.Guid}
	assertLegacyResetRejected(t, f, "warrior-reset", 992002, func() error {
		_, err := f.s.p5WarriorReset(legacyResetContext(f, reset), f.id, reset, 992002)
		return err
	})
}

func TestP5ResetRejectsIncompleteMainInvestment(t *testing.T) {
	f := legacyResetFixture(t)
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(40), proto.Int64(100000), proto.Int64(100)
		if _, err := f.s.GrantFixedReward(p, g, "item_007,10"); err != nil {
			return err
		}
		guid = p.GetHeroWarId()
		main, err := p5Warrior(g, guid)
		if err != nil {
			return err
		}
		main.Training = growthTraining([4]int32{1, 0, 0, 0}, 0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	train := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(1)}
	if _, err := f.s.warriorTraining85WithDraw(legacyResetContext(f, train), f.id, train, 993001, func(int64) (int64, error) { return 0, nil }); err != nil {
		t.Fatal("tracked main training after legacy training", err)
	}
	_, g := f.profile(t)
	if p5KV(g, "p5.invest:"+guid) == "" {
		t.Fatal("tracked training did not create a ledger")
	}
	reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
	assertLegacyResetRejected(t, f, "warrior-reset", 993002, func() error {
		_, err := f.s.p5WarriorReset(legacyResetContext(f, reset), f.id, reset, 993002)
		return err
	})
}

func TestP5ResetRejectsIncompleteTreasureInvestment(t *testing.T) {
	f := legacyResetFixture(t)
	var target, food *pb.Treasure
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin, p.Diamond = proto.Int32(40), proto.Int64(100000), proto.Int64(100)
		gain, err := f.s.GrantFixedReward(p, g, "trea_011,1;trea_013,1")
		if err != nil {
			return err
		}
		for _, treasure := range gain.GetTreas() {
			if treasure.GetTreasureId() == "trea_011" {
				target = treasure
			} else if treasure.GetTreasureId() == "trea_013" {
				food = treasure
			}
		}
		owned, err := f.s.treasureRecord(g, target.GetGuid())
		if err != nil {
			return err
		}
		owned.StrengthLevel = proto.Int32(2)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	upgrade := &pb.CSTreasureStrengthen{TargetId: target.Guid, Guids: []string{food.GetGuid()}}
	if _, err := f.s.p5TreasureStrengthen(legacyResetContext(f, upgrade), f.id, upgrade, 994001); err != nil {
		t.Fatal("tracked growth after legacy progress", err)
	}
	_, g := f.profile(t)
	if p5KV(g, "p5.invest:"+target.GetGuid()) == "" {
		t.Fatal("tracked upgrade did not create a ledger")
	}
	reset := &pb.CStreasureRebuild{TargetId: target.Guid}
	assertLegacyResetRejected(t, f, "treasure-reset", 994002, func() error {
		_, err := f.s.p5TreasureReset(legacyResetContext(f, reset), f.id, reset, 994002)
		return err
	})
}

func TestP5ResetRejectsUntrackedEquipmentStarExperience(t *testing.T) {
	f := legacyResetFixture(t)
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Diamond = proto.Int32(40), proto.Int64(100)
		gain, err := f.s.GrantFixedReward(p, g, "equip_037,1")
		if err != nil {
			return err
		}
		guid = gain.GetEquips()[0].GetGuid()
		owned, err := f.s.equipmentRecord(g, guid)
		if err != nil {
			return err
		}
		owned.StarExp = proto.Int32(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reset := &pb.CSEquipReBuild{Guid: proto.String(guid)}
	assertLegacyResetRejected(t, f, "equipment-reset", 995001, func() error {
		_, err := f.s.p5EquipmentReset(legacyResetContext(f, reset), f.id, reset, 995001)
		return err
	})
}
