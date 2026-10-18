package service

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9PetEquipment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-equip.db")
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
	open := func() {
		t.Helper()
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("P9 isolation")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := int64(510000)
	state := func(id int64) (*pb.Player, *model.GameState, db.Profile) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g, row
	}
	fresh := func(name string) (int64, *pb.Pet) {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-equip-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "精灵装备隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(34)
			_, err := s.GrantFixedReward(p, g, "coin,100000000;item_153,1;item_124,20000;item_374,1000;item_375,1000;item_376,1000;item_377,1000;item_378,1000;item_379,1000;item_380,1000;item_381,1000;item_382,1000;item_383,1000;item_384,1000;item_385,1000;item_386,1000;item_387,1000;item_388,1000;item_389,1000;item_390,1000;item_391,1000;item_392,1000;item_393,1000;item_394,1000;item_395,1000;item_396,1000;item_397,1000;item_398,1000")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, handled, err := s.DispatchP9Pets(ctx, row.Id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_153"), Num: proto.Int32(1)}}, stamp)
		if err != nil || !handled {
			t.Fatal(err)
		}
		pet := out[0].(*pb.SCItemUse).GetGet().GetPets()[0]
		var gain int64
		for level := int32(1); level < 20; level++ {
			need, err := s.p9LevelExp(pet, level)
			if err != nil {
				t.Fatal(err)
			}
			gain += need
		}
		stamp++
		grown, _, err := s.DispatchP9PetNext(ctx, row.Id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(int32((gain + 99) / 100))}}}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		pet = grown[0].(*pb.SCPetLevelup).GetPet()
		stamp++
		if _, _, err = s.DispatchP9PetNext(ctx, row.Id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
			t.Fatal(err)
		}
		return row.Id, pet
	}
	call := func(id int64, b proto.Message) proto.Message {
		t.Helper()
		raw, err := proto.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		request := b.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, request); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, handled, err := s.DispatchP9PetEquip(ctx, id, request, stamp)
		if !handled || err != nil {
			t.Fatalf("%T handled=%v err=%v", b, handled, err)
		}
		return out[0]
	}
	deny := func(id int64, b proto.Message) {
		t.Helper()
		_, _, before := state(id)
		stamp++
		if _, handled, err := s.DispatchP9PetEquip(ctx, id, b, stamp); err == nil || !handled {
			t.Fatal("denial missing", b, err)
		}
		_, _, after := state(id)
		if *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("failure changed owned state")
		}
	}
	battle := func(id int64) *pb.BaseProp {
		t.Helper()
		p, g, _ := state(id)
		party, err := s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		return party.GetFormation()[0].GetProp()
	}
	t.Run("native-single-five-receipts-ledger-and-battle-property", func(t *testing.T) {
		id, pet := fresh("strength")
		equip := pet.GetPetEquipList()[0]
		base := battle(id).GetHp()
		out := call(id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(1)}).(*pb.SCPetEquipStrengthOne)
		row := s.p9TablePair("PetEquipStrength", "equipId", equip.GetEquipId(), "level", "1")
		expected := map[string]float64{}
		if err := p9AddProperties(expected, row["prop"], 1); err != nil {
			t.Fatal(err)
		}
		if out.GetPetEquip().GetStrengthLevel() != 1 || math.Abs(battle(id).GetHp()-base-expected["hp"]) > 1e-7 {
			t.Fatal("real strength property", out)
		}
		request := &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)}
		stamp++
		frozen := stamp
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		replies := make(chan string, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, _, err := s.DispatchP9PetEquip(ctx, id, request, frozen)
				if err != nil {
					errs <- err
					return
				}
				data, err := proto.Marshal(out[0])
				if err != nil {
					errs <- err
				} else {
					replies <- string(data)
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(replies)
		for err := range errs {
			t.Fatal(err)
		}
		first := ""
		for reply := range replies {
			if first == "" {
				first = reply
			}
			if first != reply {
				t.Fatal("batch receipt divergence")
			}
		}
		owned, err := p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil {
			t.Fatal(err)
		}
		current, err := s.p9OwnedEquip(owned, equip.GetEquipId())
		if err != nil || current.GetStrengthLevel() != 6 {
			t.Fatal(current, err)
		}
		row = s.p9TablePair("PetEquipStrength", "equipId", equip.GetEquipId(), "level", "6")
		expected = map[string]float64{}
		if err = p9AddProperties(expected, row["prop"], 1); err != nil {
			t.Fatal(err)
		}
		if math.Abs(battle(id).GetHp()-base-expected["hp"]) > 1e-7 {
			t.Fatal("cumulative native strength property")
		}
		var paid int64
		for level := 1; level <= 6; level++ {
			costs, err := p3Resources(s.p9TablePair("PetEquipStrength", "equipId", equip.GetEquipId(), "level", strconv.Itoa(level))["cost"])
			if err != nil {
				t.Fatal(err)
			}
			paid += int64(costs[0].GetNum())
		}
		var ledger db.P9PetInvestment
		if err = db.GetIns().First(&ledger, "guid=?", pet.GetGuid()).Error; err != nil {
			t.Fatal(err)
		}
		costs := map[string]int64{}
		if err = json.Unmarshal(ledger.Costs, &costs); err != nil || costs["coin"] < paid {
			t.Fatal("actual equipment ledger", costs, err)
		}
		_, _, saved := state(id)
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		retry, _, err := s.DispatchP9PetEquip(ctx, id, request, frozen)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := proto.Marshal(retry[0])
		if string(data) != first {
			t.Fatal("equipment receipt after reopen")
		}
		_, _, after := state(id)
		if *saved.Player != *after.Player || *saved.Game != *after.Game {
			t.Fatal("equipment retry repeated fee")
		}
		bounded := call(id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)}).(*pb.SCPetEquipStrengthOne)
		if bounded.GetPetEquip().GetStrengthLevel() != 10 {
			t.Fatal("five-step request must stop at original advance boundary")
		}
		deny(id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)})
	})
	t.Run("original-advance-awake-costs-effects-and-owned-query", func(t *testing.T) {
		id, pet := fresh("advance")
		equip := pet.GetPetEquipList()[0]
		deny(id, &pb.CSPetEquipAdvance{PetGuid: pet.Guid, EquipId: equip.EquipId})
		call(id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)})
		call(id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)})
		before := battle(id)
		advanced := call(id, &pb.CSPetEquipAdvance{PetGuid: pet.Guid, EquipId: equip.EquipId}).(*pb.SCPetEquipAdvance)
		if advanced.GetPetEquip().GetAdvanceLevel() != 1 {
			t.Fatal(advanced)
		}
		row := s.p9TablePair("PetEquipAdvance", "equipId", equip.GetEquipId(), "level", "1")
		props := map[string]float64{}
		if err := p9AddProperties(props, row["prop"], 1); err != nil {
			t.Fatal(err)
		}
		after := battle(id)
		if math.Abs(after.GetHp()-before.GetHp()-props["hp"]) > 1e-7 || math.Abs(after.GetAtt()-before.GetAtt()-props["att"]) > 1e-7 {
			t.Fatal("advance effect")
		}
		before = after
		awake := call(id, &pb.CSPetEquipAwake{PetGuid: pet.Guid, EquipId: equip.EquipId}).(*pb.SCPetEquipAwake)
		if awake.GetPetEquip().GetAwakeLevel() != 1 {
			t.Fatal(awake)
		}
		row = s.p9TablePair("PetEquipAwake", "equipId", equip.GetEquipId(), "level", "1")
		props = map[string]float64{}
		if err := p9AddProperties(props, row["prop"], 1); err != nil {
			t.Fatal(err)
		}
		after = battle(id)
		if math.Abs(after.GetHp()-before.GetHp()-props["hp"]) > 1e-7 || math.Abs(after.GetAtt()-before.GetAtt()-props["att"]) > 1e-7 {
			t.Fatal("awake effect")
		}
		list := call(id, &pb.CSGetPetEquipInfo{PetGuid: []string{pet.GetGuid()}}).(*pb.SCGetPetEquipInfo)
		if len(list.GetPetEquip()) != 4 || list.GetPetEquip()[0].GetAwakeLevel() != 1 {
			t.Fatal(list)
		}
		deny(id, &pb.CSGetPetEquipInfo{PetGuid: []string{"foreign"}})
		deny(id, &pb.CSPetEquipStrengthAll{PetGuid: proto.String("foreign")})
	})
	t.Run("insufficient-batch-rollback-and-foreign-equipment", func(t *testing.T) {
		id, pet := fresh("rollback")
		equip := pet.GetPetEquipList()[0]
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Coin = proto.Int64(1); return nil }); err != nil {
			t.Fatal(err)
		}
		deny(id, &pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: equip.EquipId, Num: proto.Int32(5)})
		owned, err := p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil || owned.GetPetEquipList()[0].GetStrengthLevel() != 0 {
			t.Fatal("failed batch partially strengthened", err)
		}
		foreign := ""
		for _, row := range s.Tables["PetEquip"] {
			if row["petId"] != pet.GetId() {
				foreign = row["id"]
				break
			}
		}
		deny(id, &pb.CSPetEquipAwake{PetGuid: pet.Guid, EquipId: proto.String(foreign)})
	})
}
