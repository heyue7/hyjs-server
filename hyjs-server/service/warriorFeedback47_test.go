package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestWarriorFeedback47BatchMaterialsAndColdReceipt exercises a real six/20-GUID
// upgrade, material ownership and the committed retry through the original entry.
func TestWarriorFeedback47BatchMaterialsAndColdReceipt(t *testing.T) {
	for _, count := range []int{6, 20} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := p1Setup(t)
			if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			if err := f.s.InitializeP5(f.ctx); err != nil {
				t.Fatal(err)
			}
			var target string
			request := &pb.CSWarriorStrengthen{}
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
				p.Level, p.Coin = proto.Int32(100), proto.Int64(1000000)
				gain, err := f.s.GrantFixedReward(p, g, fmt.Sprintf("hero_011,1;hero_exp_001,%d", count))
				if err != nil {
					return err
				}
				for _, w := range gain.GetWars() {
					if w.GetId() == "hero_011" {
						target, request.Guid = w.GetGuid(), w.Guid
					} else {
						request.Cost = append(request.Cost, &pb.BaseResource{Id: w.Id, Guid: w.Guid})
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
			beforeP, beforeG := f.profile(t)
			reject := proto.Clone(request).(*pb.CSWarriorStrengthen)
			reject.Cost[len(reject.Cost)-1] = proto.Clone(reject.Cost[0]).(*pb.BaseResource)
			if _, err := f.s.p5WarriorStrengthen(ctx, f.id, reject, 47001); err == nil {
				t.Fatal("duplicate material accepted")
			}
			afterP, afterG := f.profile(t)
			beforeRaw, _ := json.Marshal(beforeG)
			afterRaw, _ := json.Marshal(afterG)
			if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
				t.Fatal("invalid batch partially consumed inventory")
			}
			out, err := f.s.p5WarriorStrengthen(ctx, f.id, request, 47002)
			if err != nil {
				t.Fatal("valid batch over five rejected", err)
			}
			response := out[0].(*pb.SCWarriorStrengthen)
			if response.GetGuid().GetGuid() != target || response.GetGuid().GetLevel() <= 1 || len(response.Cost) != count+1 {
				t.Fatal("batch not fully consumed or target not upgraded")
			}
			paidP, paidG := f.profile(t)
			if paidP.GetCoin() >= beforeP.GetCoin() || len(paidG.Warriors) != len(beforeG.Warriors)-count {
				t.Fatal("upgrade material/coin conservation")
			}
			f.restart(t)
			retry, err := f.s.p5WarriorStrengthen(ctx, f.id, request, 47002)
			if err != nil || !proto.Equal(retry[0], response) {
				t.Fatal("cold committed retry", err)
			}
			retryP, retryG := f.profile(t)
			paidRaw, _ := json.Marshal(paidG)
			retryRaw, _ := json.Marshal(retryG)
			if !proto.Equal(paidP, retryP) || string(paidRaw) != string(retryRaw) {
				t.Fatal("retry consumed a second batch")
			}
		})
	}
}

// TestWarriorFeedback47BreachUsesPlayerUnlock checks the original player-level
// feature gate while retaining the independent awakening level contract.
func TestWarriorFeedback47BreachUsesPlayerUnlock(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{"GameConfigure": {
		{"StaticName": "HeroBreachedLevel", "NameValue": "35"},
		{"StaticName": "HeroTrainLevel", "NameValue": "10"},
		{"StaticName": "heroAwakeLevel", "NameValue": "60"},
	}}}
	p := &pb.Player{HeroWarId: proto.String("main"), Level: proto.Int32(35)}
	w := &pb.Warrior{Guid: proto.String("partner"), Level: proto.Int32(1)}
	if err := s.growthLevel(p, w, "HeroBreachedLevel"); err != nil {
		t.Fatal("unlocked player cannot breach a level-one partner", err)
	}
	if err := s.growthLevel(p, w, "HeroTrainLevel"); err != nil {
		t.Fatal("training player gate diverged", err)
	}
	if err := s.growthLevel(p, w, "heroAwakeLevel"); err == nil {
		t.Fatal("awakening partner level requirement changed")
	}
	p.Level, w.Level = proto.Int32(34), proto.Int32(100)
	if err := s.growthLevel(p, w, "HeroBreachedLevel"); err == nil {
		t.Fatal("under-level player bypassed unlock with a grown partner")
	}
}

// TestWarriorFeedback47BreachLowPartnerActualCommit verifies an unlocked player
// can spend the original costs on a level-one partner and safely retry after reopen.
func TestWarriorFeedback47BreachLowPartnerActualCommit(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(35)
		gain, err := f.s.GrantFixedReward(p, g, "coin,1000000;hero_011,1;item_006,100")
		if err == nil {
			guid = gain.Wars[0].GetGuid()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	request := &pb.CSWarriorBreached{Guid: proto.String(guid), Type: proto.Int32(1)}
	before, old := f.profile(t)
	w, err := p5Warrior(old, guid)
	if err != nil || w.GetLevel() != 1 {
		t.Fatal("finite level-one partner fixture", err)
	}
	out, err := f.s.warriorBreached(ctx, f.id, request, 47010)
	if err != nil {
		t.Fatal("player35/partner1 original breach rejected", err)
	}
	response := out[0].(*pb.SCWarriorBreached)
	if len(response.Cost) == 0 || response.GetGuid().GetBreached().GetLevel() < 1 || (response.GetGuid().GetBreached().GetLevel() == 1 && response.GetGuid().GetBreached().GetExp() <= 0) {
		t.Fatal("no actual original breach progression")
	}
	paid, state := f.profile(t)
	for _, cost := range response.Cost {
		if cost.GetNum() <= 0 || simpleOwned(before, old, cost.GetId())-simpleOwned(paid, state, cost.GetId()) != int64(cost.GetNum()) {
			t.Fatal("original breach resource conservation", cost.GetId())
		}
	}
	f.restart(t)
	retry, err := f.s.warriorBreached(ctx, f.id, request, 47010)
	if err != nil || !proto.Equal(retry[0], response) {
		t.Fatal("cold breach receipt", err)
	}
	after, current := f.profile(t)
	stateRaw, _ := json.Marshal(state)
	currentRaw, _ := json.Marshal(current)
	if !proto.Equal(paid, after) || string(stateRaw) != string(currentRaw) {
		t.Fatal("cold breach replay consumed resources twice")
	}
}
