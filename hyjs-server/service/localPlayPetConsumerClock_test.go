package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The shared fixture declares finite level/fragments/diamonds. This bounded
// consumer regression additionally declares10000 coin,20 original EXP flowers
// and32 original fire foods; it makes no natural-source or payment assertion.
func TestLocalPlayPetConsumerPreviousDayLevelEatResetAndCold(t *testing.T) {
	f, id, owner, ctx := localPetGrowthClockFixture(t)
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		_, err := f.s.GrantFixedReward(p, g, "coin,10000;item_124,20;item_129,32")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	initialPlayer, initialGame := f.profile(t)
	inventory := func(item string) int64 {
		t.Helper()
		p, g := f.profile(t)
		return simpleOwned(p, g, item)
	}
	owned := func(guid string) *pb.Pet {
		t.Helper()
		pet, err := p9OwnedPet(db.GetIns(), id, guid)
		if err != nil {
			t.Fatal("actual combined owned pet", err)
		}
		return pet
	}
	if inventory("item_124") != 20 || inventory("item_129") != 32 || initialPlayer.GetCoin() != 10000 {
		t.Fatal("finite consumer prerequisites differ")
	}
	begin := LocalPlayNow(ctx)
	combine := &pb.CSCombine{TargetId: proto.String("pet_002")}
	combined := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetCombine(ctx, id, combine, 92001)
	}).(*pb.SCCombine)
	if len(combined.GetGet().GetPets()) != 1 || combined.GetGet().GetPets()[0].GetId() != "pet_002" || inventory("frag_pet_002") != simpleOwned(initialPlayer, initialGame, "frag_pet_002")-50 {
		t.Fatal("original50 fragment combine or same-stamp conservation", combined)
	}
	pet := combined.GetGet().GetPets()[0]
	guid := pet.GetGuid()
	if guid == "" || pet.GetLevel() != 1 || pet.GetExp() != 0 || len(pet.GetRandPropList()) != 0 {
		t.Fatal("original pet002 initial state", pet)
	}
	localPetGrowthClockFact(t, id, "p9:fragment-combine:92001", "pet_fragment_consumed", 50, begin, LocalPlayNow(ctx))

	level := &pb.CSPetLevelup{Guid: proto.String(guid), Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(20)}}}
	levelAt := LocalPlayNow(ctx)
	leveled := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetLevelup(ctx, id, level, 92002)
	}).(*pb.SCPetLevelup)
	current := owned(guid)
	need, err := f.s.p9LevelExp(pet, pet.GetLevel())
	if err != nil || need != 1500 || current.GetLevel() != 2 || current.GetExp() != 2000-need || !proto.Equal(leveled.GetPet(), current) {
		t.Fatal("original EXP threshold and accepted current level", current, need, err)
	}
	if inventory("item_124") != 0 || inventory("item_129") != 32 {
		t.Fatal("eight EXP retries consumed beyond20 flowers")
	}
	afterLevel, _ := f.profile(t)
	if initialPlayer.GetCoin()-afterLevel.GetCoin() != 2000 {
		t.Fatal("original EXP100 per flower/coin1 per EXP cost", afterLevel.GetCoin())
	}
	localPetGrowthClockFact(t, id, "p9:level:92002", constUtil.P1EventCoinSpent, 2000, levelAt, LocalPlayNow(ctx))
	localPetGrowthClockFact(t, id, "p9:level:92002", "spend:item_124", 20, levelAt, LocalPlayNow(ctx))

	// Same-nature32*10*2=640 crosses the original626 first intimacy threshold.
	// Inspect the allocated policy outcome; never prescribe a random talent ID.
	eat := &pb.CSPetEat{Guid: proto.String(guid), Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(32)}}}
	eatAt := LocalPlayNow(ctx)
	fed := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetEat(ctx, id, eat, 92003)
	}).(*pb.SCPetEat)
	grown := owned(guid)
	if !proto.Equal(fed.GetPet(), grown) || grown.GetLevel() != 2 || grown.GetExp() != 500 || grown.GetEat().GetLevel() != 1 || grown.GetEat().GetExp() != 14 || len(grown.GetRandPropList()) != 1 || grown.GetRandPropList()[0] == grown.GetInitProp() || inventory("item_129") != 0 {
		t.Fatal("actual original first intimacy state/allocation", grown)
	}
	if inventory("item_124") != 0 {
		t.Fatal("feeding modified unrelated EXP material")
	}
	afterEat, _ := f.profile(t)
	if afterEat.GetCoin() != afterLevel.GetCoin() || afterEat.GetDiamond() != initialPlayer.GetDiamond() {
		t.Fatal("food transaction invented a currency cost or reward")
	}
	localPetGrowthClockFact(t, id, "p9:eat:92003", "spend:item_129", 32, eatAt, LocalPlayNow(ctx))
	var allocation db.P9PetTalentAllocation
	if err = db.GetIns().First(&allocation, "guid=? AND player_id=?", guid, id).Error; err != nil || allocation.PolicyHash == "" || allocation.Source == "" {
		t.Fatal("actual random allocation provenance", err)
	}
	var draws []string
	if err = json.Unmarshal(allocation.Draws, &draws); err != nil || !equalP9Strings(draws, grown.GetRandPropList()) {
		t.Fatal("random allocation ledger differs", err)
	}
	beforeProp, beforeTalents, err := f.s.p9PetProperties(current)
	if err != nil {
		t.Fatal(err)
	}
	afterProp, afterTalents, err := f.s.p9PetProperties(grown)
	if err != nil || reflect.DeepEqual(beforeProp, afterProp) && reflect.DeepEqual(beforeTalents, afterTalents) {
		t.Fatal("allocated talent has no derived consumer", err)
	}

	// A real cold DB reopen must preserve the random choice and exact current
	// Eat receipt. A stale pre-Eat Levelup receipt cannot overwrite that choice.
	savedPlayer, _ := f.profile(t)
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	replay, err := f.s.p9PetEat(ctx, id, eat, 92003)
	if err != nil || !proto.Equal(replay[0], fed) || !proto.Equal(owned(guid), grown) {
		t.Fatal("cold current food receipt/random state", err)
	}
	coldPlayer, _ := f.profile(t)
	if !proto.Equal(savedPlayer, coldPlayer) {
		t.Fatal("cold replay changed currency")
	}
	if _, err = f.s.p9PetLevelup(ctx, id, level, 92002); err == nil {
		t.Fatal("old pre-intimacy growth receipt overwrote later random state")
	}

	// Reuse the existing reset: refund only this GUID's actual recorded costs,
	// charge the original50 diamonds, retire it and grant a new original GUID.
	reset := &pb.CSPetReset{Guid: proto.String(guid)}
	resetAt := LocalPlayNow(ctx)
	rebuilt := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetReset(ctx, id, reset, 92004)
	}).(*pb.SCPetReset)
	if len(rebuilt.GetGet().GetPets()) != 1 {
		t.Fatal("reset did not return one original initial pet")
	}
	fresh := rebuilt.GetGet().GetPets()[0]
	if fresh.GetId() != "pet_002" || fresh.GetGuid() == "" || fresh.GetGuid() == guid || fresh.GetLevel() != 1 || fresh.GetExp() != 0 || fresh.GetEat().GetLevel() != 0 || fresh.GetEat().GetExp() != 0 || len(fresh.GetRandPropList()) != 0 {
		t.Fatal("reset retained grown state or reused retired GUID", fresh)
	}
	finalPlayer, _ := f.profile(t)
	if finalPlayer.GetCoin() != initialPlayer.GetCoin() || finalPlayer.GetDiamond() != initialPlayer.GetDiamond()-50 || inventory("item_124") != 20 || inventory("item_129") != 32 || inventory("frag_pet_002") != 50 {
		t.Fatal("actual investment refund or original reset fee conservation")
	}
	localPetGrowthClockFact(t, id, constUtil.P9PetResetOperation+":92004", constUtil.P1EventDiamondSpent, 50, resetAt, LocalPlayNow(ctx))
	var retired db.P9PetRetired
	if err = db.GetIns().First(&retired, "player_id=? AND guid=?", id, guid).Error; err != nil || retired.Operation != constUtil.P9PetResetOperation {
		t.Fatal("old actual GUID retirement", err)
	}
	preserved := &pb.Pet{}
	if err = proto.Unmarshal(retired.Data, preserved); err != nil || !proto.Equal(preserved, grown) {
		t.Fatal("retirement lost original grown/random state", err)
	}
	if _, err = f.s.p9PetEat(ctx, id, eat, 92003); err == nil {
		t.Fatal("old food receipt resurrected retired pet")
	}

	f.restart(t)
	ctx = localClockBind(t, f, owner)
	replay, err = f.s.p9PetReset(ctx, id, reset, 92004)
	if err != nil || !proto.Equal(replay[0], rebuilt) || !proto.Equal(owned(fresh.GetGuid()), fresh) {
		t.Fatal("cold reset receipt/replacement GUID", err)
	}
	finalCold, _ := f.profile(t)
	if !proto.Equal(finalPlayer, finalCold) || inventory("item_124") != 20 || inventory("item_129") != 32 {
		t.Fatal("cold repeated reset duplicated actual refund")
	}
}
