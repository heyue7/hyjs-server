package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9EquipFiveOriginalAll76BoundaryMatrix(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{}}
	if err := s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if len(s.Tables["PetEquip"]) != 76 {
		t.Fatal("original 19 red pets / four equipment slots changed")
	}
	for _, row := range s.Tables["PetEquip"] {
		pet := &pb.Pet{Id: proto.String(row["petId"]), Level: proto.Int32(20)}
		equip := &pb.PetEquip{EquipId: proto.String(row["id"])}
		for _, v := range []struct{ level, current, want int32 }{{20, 0, 5}, {20, 8, 2}, {9, 6, 3}, {5, 4, 1}} {
			pet.Level, equip.StrengthLevel = proto.Int32(v.level), proto.Int32(v.current)
			steps, err := s.p9EquipFiveSteps(pet, equip)
			if err != nil || steps != v.want {
				t.Fatalf("%s pet%d equip%d: steps=%d err=%v", row["id"], v.level, v.current, steps, err)
			}
		}
		// 最终进阶后，总最高级前1～4阶按实际剩余阶数执行。
		for remaining := int32(1); remaining <= 4; remaining++ {
			pet.Level, equip.StrengthLevel, equip.AdvanceLevel = proto.Int32(100), proto.Int32(100-remaining), proto.Int32(12)
			steps, err := s.p9EquipFiveSteps(pet, equip)
			if err != nil || steps != remaining {
				t.Fatalf("%s final remaining%d: steps=%d err=%v", row["id"], remaining, steps, err)
			}
		}
		equip.AdvanceLevel = proto.Int32(0)
		for _, v := range []struct{ level, current int32 }{{20, 10}, {9, 9}, {100, 100}} {
			pet.Level, equip.StrengthLevel = proto.Int32(v.level), proto.Int32(v.current)
			if _, err := s.p9EquipFiveSteps(pet, equip); err == nil {
				t.Fatalf("%s admitted no-progress request", row["id"])
			}
		}
	}
}

func TestP9EquipFiveActualCostsBoundaryReceiptAndAtomicFailure(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP9PetNext(f.ctx); err != nil {
		t.Fatal(err)
	}
	var pet *pb.Pet
	// 有限基础实例、材料和等级资格；随后均走原成长/强化事务，不计自然获取。
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin = proto.Int32(100), proto.Int64(10000000)
		if _, err := f.s.GrantFixedReward(p, g, "item_124,10000"); err != nil {
			return err
		}
		pets, err := f.s.P9GrantPet(tx, p, "pet_047", 1)
		if err == nil {
			pet = pets[0]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stamp := int64(925000)
	grow := func(level int32) {
		t.Helper()
		current, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
		if err != nil {
			t.Fatal(err)
		}
		need := -current.GetExp()
		for n := current.GetLevel(); n < level; n++ {
			value, err := f.s.p9LevelExp(current, n)
			if err != nil {
				t.Fatal(err)
			}
			need += value
		}
		stamp++
		messages, _, err := f.s.DispatchP9PetNext(f.ctx, f.id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(int32((need + 99) / 100))}}}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		pet = messages[0].(*pb.SCPetLevelup).GetPet()
		if pet.GetLevel() != level {
			t.Fatal("finite preparation used actual growth but reached wrong level", pet.GetLevel(), level)
		}
	}
	costFor := func(equipID string, first, last int32) int64 {
		t.Helper()
		var total int64
		for n := first; n <= last; n++ {
			fees, err := p3Resources(f.s.p9TablePair("PetEquipStrength", "equipId", equipID, "level", strconv.Itoa(int(n)))["cost"])
			if err != nil || len(fees) != 1 || fees[0].GetId() != "coin" {
				t.Fatal("original equipment coin row", err)
			}
			total += int64(fees[0].GetNum())
		}
		return total
	}
	call := func(equipID string, count int32) *pb.SCPetEquipStrengthOne {
		t.Helper()
		stamp++
		messages, handled, err := f.s.DispatchP9PetEquip(f.ctx, f.id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: proto.String(equipID), Num: proto.Int32(count)}, stamp)
		if !handled || err != nil {
			t.Fatal("actual equipment dispatch", handled, err)
		}
		return messages[0].(*pb.SCPetEquipStrengthOne)
	}
	grow(9)
	equipID := pet.GetPetEquipList()[0].GetEquipId()
	initial, _ := f.profile(t)
	if out := call(equipID, 5); out.GetPetEquip().GetStrengthLevel() != 5 {
		t.Fatal("normal five-step behavior changed")
	}
	if out := call(equipID, 1); out.GetPetEquip().GetStrengthLevel() != 6 {
		t.Fatal("original single-step behavior changed")
	}
	before, _ := f.profile(t)
	petBefore, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	propsBefore, _, err := f.s.p9PetProperties(petBefore)
	if err != nil {
		t.Fatal(err)
	}
	stamp++
	frozen := stamp
	request := &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: proto.String(equipID), Num: proto.Int32(5)}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	results := make(chan *pb.SCPetEquipStrengthOne, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, _, err := f.s.DispatchP9PetEquip(f.ctx, f.id, request, frozen)
			errs <- err
			if err == nil {
				results <- messages[0].(*pb.SCPetEquipStrengthOne)
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *pb.SCPetEquipStrengthOne
	for result := range results {
		if first == nil {
			first = result
		} else if !proto.Equal(first, result) {
			t.Fatal("concurrent receipt differs")
		}
	}
	if first.GetPetEquip().GetStrengthLevel() != 9 || len(first.GetCost()) != 3 {
		t.Fatal("pet-level boundary did not charge exactly three original rows", first)
	}
	after, _ := f.profile(t)
	if before.GetCoin()-after.GetCoin() != costFor(equipID, 7, 9) || initial.GetCoin()-after.GetCoin() != costFor(equipID, 1, 9) {
		t.Fatal("actual partial fee differs or repeated")
	}
	petAfter, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	propsAfter, _, err := f.s.p9PetProperties(petAfter)
	if err != nil || propsAfter["hp"] <= propsBefore["hp"] {
		t.Fatal("partial equipment levels did not reach actual pet properties", err)
	}
	var fact db.EventFact
	if err := db.GetIns().First(&fact, "player_id=? AND source_key=? AND kind=?", f.id, constUtil.P9PetEquipStrengthEventPrefix+strconv.FormatInt(frozen, 10), constUtil.P1EventCoinSpent).Error; err != nil || fact.Amount != costFor(equipID, 7, 9) {
		t.Fatal("partial original cost event differs", err)
	}
	var investment db.P9PetInvestment
	if err := db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil || !strings.Contains(investment.Source, constUtil.P9PetEquipFiveSource) {
		t.Fatal("bounded policy provenance missing", err)
	}
	f.restart(t)
	messages, _, err := f.s.DispatchP9PetEquip(f.ctx, f.id, request, frozen)
	if err != nil || !proto.Equal(first, messages[0]) {
		t.Fatal("cold original five-step receipt", err)
	}
	// 精灵升到20后，第二槽8→10只做两级；当前进阶上限保持10。
	grow(20)
	second := pet.GetPetEquipList()[1].GetEquipId()
	call(second, 5)
	for i := 0; i < 3; i++ {
		call(second, 1)
	}
	before, _ = f.profile(t)
	if out := call(second, 5); out.GetPetEquip().GetStrengthLevel() != 10 || len(out.GetCost()) != 2 {
		t.Fatal("advance boundary must stop at10")
	}
	after, _ = f.profile(t)
	if before.GetCoin()-after.GetCoin() != costFor(second, 9, 10) {
		t.Fatal("advance boundary charged unperformed steps")
	}
	stamp++
	if _, _, err = f.s.DispatchP9PetEquip(f.ctx, f.id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: proto.String(second), Num: proto.Int32(5)}, stamp); err == nil {
		t.Fatal("current cap accepted no-progress request")
	}
	call(equipID, 1)
	if _, _, err = f.s.DispatchP9PetEquip(f.ctx, f.id, request, frozen); err == nil {
		t.Fatal("old level9 receipt would overwrite current level10")
	}
	// 第三槽预算只少最后1币，前四阶临时扣费也必须随整个事务回滚。
	third := pet.GetPetEquipList()[2].GetEquipId()
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Coin = proto.Int64(costFor(third, 1, 5) - 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := f.profile(t)
	beforeJSON, _ := json.Marshal(beforeG)
	ownedBefore, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	stamp++
	if _, _, err = f.s.DispatchP9PetEquip(f.ctx, f.id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: proto.String(third), Num: proto.Int32(5)}, stamp); err == nil {
		t.Fatal("insufficient batch accepted")
	}
	afterP, afterG := f.profile(t)
	afterJSON, _ := json.Marshal(afterG)
	ownedAfter, err := p9OwnedPet(db.GetIns(), f.id, pet.GetGuid())
	if err != nil || !proto.Equal(beforeP, afterP) || string(beforeJSON) != string(afterJSON) || !proto.Equal(ownedBefore, ownedAfter) {
		t.Fatal("failed batch left partial state", err)
	}
	if p1Count(t, &db.P9PetReceipt{}, "player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P9PetEquipStrengthOperation, stamp) != 0 {
		t.Fatal("failed batch saved receipt")
	}
	// 有限顶级前置夹具，只验最终1～4阶的真实扣费、回执及满级拒绝。
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin = proto.Int32(100), proto.Int64(30000000)
		current, err := p9OwnedPet(tx, f.id, pet.GetGuid())
		if err != nil {
			return err
		}
		current.Level = proto.Int32(100)
		for index, equip := range current.GetPetEquipList() {
			equip.AdvanceLevel, equip.StrengthLevel = proto.Int32(12), proto.Int32(96+int32(index))
		}
		pet = current
		return p9SavePet(tx, f.id, g, current)
	}); err != nil {
		t.Fatal(err)
	}
	for index, equip := range pet.GetPetEquipList() {
		before, _ := f.profile(t)
		out := call(equip.GetEquipId(), 5)
		after, _ := f.profile(t)
		if out.GetPetEquip().GetStrengthLevel() != 100 || len(out.GetCost()) != 4-index || before.GetCoin()-after.GetCoin() != costFor(equip.GetEquipId(), 97+int32(index), 100) {
			t.Fatal("final boundary did not charge actual remaining steps", index, out)
		}
		request := &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)}
		messages, _, err := f.s.DispatchP9PetEquip(f.ctx, f.id, request, stamp)
		if err != nil || !proto.Equal(out, messages[0]) {
			t.Fatal("final boundary receipt differs", err)
		}
		replayed, _ := f.profile(t)
		if replayed.GetCoin() != after.GetCoin() {
			t.Fatal("final boundary replay charged twice")
		}
		stamp++
		if _, _, err = f.s.DispatchP9PetEquip(f.ctx, f.id, request, stamp); err == nil {
			t.Fatal("final maximum accepted no-progress request")
		}
		full, _ := f.profile(t)
		if full.GetCoin() != after.GetCoin() {
			t.Fatal("final maximum rejection charged coins")
		}
	}

}
