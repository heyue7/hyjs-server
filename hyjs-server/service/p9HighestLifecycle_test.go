package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP9HighestLifecycleAll78 uses explicitly prepared resources and real growth
// transactions. Natural obtaining, intimacy randomness and device comparison are
// separate gates; these fixtures never grant anything to the running database.
func TestP9HighestLifecycleAll78(t *testing.T) {
	p9HighestLifecycle(t, false)
}

func TestP9HighestEquipmentLifecycleAll19(t *testing.T) {
	p9HighestLifecycle(t, true)
}

func TestP9HighestEquipmentOneKeyLifecycleAll19(t *testing.T) {
	p9HighestLifecycle(t, true, true)
}

func p9HighestLifecycle(t *testing.T, withEquipment bool, oneKey ...bool) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-highest.db")
	open := func() {
		t.Helper()
		var err error
		if schema := os.Getenv("HYJS_P9_MYSQL_DATABASE"); schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("P9 highest isolated schema guard")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		} else {
			err = db.InitGame(ctx, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); _ = pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetRecycle} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	state := func(id int64) (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	items := func(g *model.GameState) map[string]int32 {
		out := map[string]int32{}
		for _, item := range g.Items {
			out[item.GetId()] += item.GetNum()
		}
		return out
	}
	stamp := int64(910000)
	call := func(t *testing.T, id int64, b proto.Message) proto.Message {
		t.Helper()
		stamp++
		out, handled, err := s.DispatchP9PetNext(ctx, id, b, stamp)
		if !handled || err != nil {
			t.Fatalf("%T id=%d: %v", b, id, err)
		}
		return out[0]
	}
	deny := func(t *testing.T, id int64, pet *pb.Pet, b proto.Message) {
		t.Helper()
		p, g := state(id)
		before, err := json.Marshal([]any{p, g})
		if err != nil {
			t.Fatal(err)
		}
		stamp++
		if _, handled, err := s.DispatchP9PetNext(ctx, id, b, stamp); !handled || err == nil {
			t.Fatal("highest boundary accepted", b)
		}
		p, g = state(id)
		after, err := json.Marshal([]any{p, g})
		if err != nil || string(before) != string(after) {
			t.Fatal("highest denial changed profile", id, err)
		}
		current, err := p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil || !proto.Equal(current, pet) {
			t.Fatal("highest denial changed pet", id, err)
		}
	}
	type savedCase struct {
		ID      int64
		Pet     *pb.Pet
		Battle  *pb.BattlePlayer
		Coin    int64
		Diamond int64
		Items   map[string]int32
		Donors  int32
	}
	cases := []savedCase{}
	fixture := map[string]json.RawMessage{}
	levelCalls, breachCalls, strengthCalls, advanceCalls, awakeCalls := 0, 0, 0, 0, 0
	for _, content := range s.Tables["Pet"] {
		key := content["id"]
		if key == "" {
			continue
		}
		if withEquipment && content["quality"] != "23" {
			continue
		}
		t.Run(key, func(t *testing.T) {
			account, err := LoginAccount(ctx, "p9-highest-"+key, "local-test-password")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "最高精灵隔离", "hero_043"); err != nil {
				t.Fatal(err)
			}
			costs := map[string]int64{"item_124": 100000}
			var donorCount int32
			for _, row := range s.Tables["PetTalentMaterial"] {
				if row["petId"] != key || row["cost"] == "" {
					continue
				}
				resources, err := p3Resources(row["cost"])
				if err != nil {
					t.Fatal(err)
				}
				for _, resource := range resources {
					if resource.GetId() == key {
						donorCount += resource.GetNum()
					} else {
						costs[resource.GetId()] += int64(resource.GetNum())
					}
				}
			}
			if withEquipment {
				for _, equip := range s.Tables["PetEquip"] {
					if equip["petId"] != key {
						continue
					}
					for _, table := range []string{"PetEquipStrength", "PetEquipAdvance", "PetEquipAwake"} {
						for _, row := range s.Tables[table] {
							if row["equipId"] != equip["id"] {
								continue
							}
							resources, err := p3Resources(row["cost"])
							if err != nil {
								t.Fatal(err)
							}
							for _, resource := range resources {
								if resource.GetId() != "coin" {
									costs[resource.GetId()] += int64(resource.GetNum())
								}
							}
							donorCount += int32(tableNumber(row, "selfCostNum"))
						}
					}
				}
			}
			var pet *pb.Pet
			if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				p.Diamond = proto.Int64(1000)
				keys := []string{}
				for resource := range costs {
					keys = append(keys, resource)
				}
				sort.Strings(keys)
				parts := []string{}
				for _, resource := range keys {
					parts = append(parts, resource+","+strconv.FormatInt(costs[resource], 10))
				}
				if _, err := s.GrantFixedReward(p, g, strings.Join(parts, ";")); err != nil {
					return err
				}
				owned, err := s.P9GrantPet(tx, p, key, 1+donorCount)
				if err != nil {
					return err
				}
				pet = owned[0]
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before, beforeGame := state(account.Id)
			baseline := savedCase{ID: account.Id, Coin: before.GetCoin(), Diamond: before.GetDiamond(), Items: items(beforeGame), Donors: donorCount}
			for stage := 0; stage <= 4; stage++ {
				material := s.p9Material(pet)
				if material == nil {
					t.Fatal("missing original material", key, stage)
				}
				cap := int32(tableNumber(material, "maxPetLevel"))
				gain := -pet.GetExp()
				for level := pet.GetLevel(); level < cap; level++ {
					need, err := s.p9LevelExp(pet, level)
					if err != nil {
						t.Fatal(err)
					}
					gain += need
				}
				if gain <= 0 || gain > math.MaxInt32-99 {
					t.Fatal("invalid original level gain", gain)
				}
				pet = call(t, account.Id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(int32((gain + 99) / 100))}}}).(*pb.SCPetLevelup).GetPet()
				levelCalls++
				if pet.GetLevel() != cap {
					t.Fatal("original cap not reached", stage, cap, pet)
				}
				if stage < 4 {
					pet = call(t, account.Id, &pb.CSPetBreached{Guid: pet.Guid}).(*pb.SCPetBreached).GetPet()
					breachCalls++
				}
			}
			if pet.GetLevel() != 100 || pet.GetBreached().GetLevel() != 4 {
				t.Fatal("highest state", pet)
			}
			if withEquipment {
				grown, strength, advance, awake := p9HighestEquipment(t, s, ctx, account.Id, pet, &stamp, oneKey...)
				strengthCalls += strength
				advanceCalls += advance
				awakeCalls += awake
				// Keep the actual equipment projection for guardian, cold and full-refund checks.
				pet = grown
			}
			deny(t, account.Id, pet, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}})
			deny(t, account.Id, pet, &pb.CSPetBreached{Guid: pet.Guid})
			call(t, account.Id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)})
			p, g := state(account.Id)
			battle, err := s.battlePlayer(p, g)
			if err != nil {
				t.Fatal(err)
			}
			_, talents, err := s.p9PetProperties(pet)
			if err != nil || len(talents) != 2 {
				t.Fatal("highest original talent unlocks", err, talents)
			}
			base := proto.Clone(battle).(*pb.BattlePlayer)
			baseline.Pet, baseline.Battle = proto.Clone(pet).(*pb.Pet), base
			cases = append(cases, baseline)
			raw, err := protojson.Marshal(battle)
			if err != nil {
				t.Fatal(err)
			}
			fixture[key] = raw
		})
	}
	if t.Failed() {
		return
	}
	expectedCases := 78
	if withEquipment {
		expectedCases = 19
	}
	if len(cases) != expectedCases {
		t.Fatal("original Pet coverage", len(cases))
	}
	// Reopen the actual persisted database before comparing guarded projections.
	pool, _ := db.GetIns().DB()
	_ = pool.Close()
	open()
	keepHighest := os.Getenv("HYJS_P9_HIGHEST_KEEP") == "1"
	for _, original := range cases {
		pet, err := p9OwnedPet(db.GetIns(), original.ID, original.Pet.GetGuid())
		if err != nil || !proto.Equal(pet, original.Pet) {
			t.Fatal("cold Pet changed", original.ID, err)
		}
		p, g := state(original.ID)
		battle, err := s.battlePlayer(p, g)
		if err != nil || !proto.Equal(battle, original.Battle) {
			t.Fatal("cold battle changed", original.ID, err)
		}
		if keepHighest {
			continue
		}
		call(t, original.ID, &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)})
		stamp++
		out, handled, err := s.DispatchP9PetRecycle(ctx, original.ID, &pb.CSPetReset{Guid: pet.Guid}, stamp)
		if err != nil || !handled {
			t.Fatal("highest reset", original.ID, err)
		}
		reset := out[0].(*pb.SCPetReset)
		if len(reset.GetGet().GetPets()) != int(1+original.Donors) {
			t.Fatal("donor refund count", original.ID, reset)
		}
		p, g = state(original.ID)
		if p.GetCoin() != original.Coin || p.GetDiamond() != original.Diamond-50 || !reflect.DeepEqual(items(g), original.Items) {
			t.Fatal("highest refund lost investment", original.ID)
		}
		for _, fresh := range reset.GetGet().GetPets() {
			if fresh.GetGuid() == pet.GetGuid() || !p9BasicDonor(fresh) {
				t.Fatal("reset reused grown identity", fresh)
			}
		}
	}
	if output := os.Getenv("HYJS_P9_HIGHEST_OUTPUT"); output != "" {
		records := []map[string]any{}
		for _, original := range cases {
			petWire, err := proto.Marshal(original.Pet)
			if err != nil {
				t.Fatal(err)
			}
			battleWire, err := proto.Marshal(original.Battle)
			if err != nil {
				t.Fatal(err)
			}
			records = append(records, map[string]any{"playerId": original.ID, "petId": original.Pet.GetId(), "petWire": fmt.Sprintf("%x", petWire), "battleWire": fmt.Sprintf("%x", battleWire), "initialCoin": original.Coin, "initialDiamond": original.Diamond, "initialItems": original.Items, "donors": original.Donors})
		}
		document := map[string]any{"scope": "prepared resources; actual highest level/breach/guardian/cold/refund; intimacy/natural/native gates remain; named terminal rounding compatibility", "battlePlayers": fixture, "records": records, "resetByGo": !keepHighest, "equipmentHighest": withEquipment, "strengthCalls": strengthCalls, "advanceCalls": advanceCalls, "awakeCalls": awakeCalls, "nativeCompared": false, "authoritative": false}
		encoded, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d original pets: %d upgrades, %d breaches, %d equipment strength calls, %d advances, %d awakes; all guarded/cold/refund checked", len(cases), levelCalls, breachCalls, strengthCalls, advanceCalls, awakeCalls)
}

// Grow all four original slots through their actual costs and donors, then compare
// the highest table properties, persisted investment and rejected upper bounds.
func p9HighestEquipment(t *testing.T, s *GameService, ctx context.Context, id int64, pet *pb.Pet, stamp *int64, oneKey ...bool) (*pb.Pet, int, int, int) {
	t.Helper()
	if len(pet.GetPetEquipList()) != 4 {
		t.Fatal("original red pet must have four equipment slots", pet.GetId())
	}
	before, _, err := s.p9PetProperties(pet)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]float64{}
	strengthCalls, advanceCalls, awakeCalls := 0, 0, 0
	call := func(request proto.Message) {
		t.Helper()
		*stamp++
		if _, handled, err := s.DispatchP9PetEquip(ctx, id, request, *stamp); !handled || err != nil {
			t.Fatalf("%T pet=%s: %v", request, pet.GetId(), err)
		}
		var err error
		pet, err = p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil {
			t.Fatal(err)
		}
	}
	deny := func(request proto.Message) {
		t.Helper()
		var previous, after db.Profile
		if err := db.GetIns().First(&previous, id).Error; err != nil {
			t.Fatal(err)
		}
		*stamp++
		if _, handled, err := s.DispatchP9PetEquip(ctx, id, request, *stamp); !handled || err == nil {
			t.Fatal("equipment highest boundary accepted", request)
		}
		if err := db.GetIns().First(&after, id).Error; err != nil || *after.Player != *previous.Player || *after.Game != *previous.Game {
			t.Fatal("equipment boundary changed profile", err)
		}
		current, err := p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil || !proto.Equal(current, pet) {
			t.Fatal("equipment boundary changed owned pet", err)
		}
	}
	for slot := 0; slot < 4; slot++ {
		equip := pet.GetPetEquipList()[slot]
		equipID := equip.GetEquipId()
		for {
			equip, err = s.p9OwnedEquip(pet, equipID)
			if err != nil {
				t.Fatal(err)
			}
			cap, err := s.p9EquipStrengthCap(pet, equip)
			if err != nil {
				t.Fatal(err)
			}
			if equip.GetStrengthLevel() < cap {
				if len(oneKey) > 0 && oneKey[0] {
					call(&pb.CSPetEquipStrengthAll{PetGuid: pet.Guid})
				} else {
					num := int32(1)
					if cap-equip.GetStrengthLevel() >= 5 {
						num = 5
					}
					call(&pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: proto.String(equipID), Num: proto.Int32(num)})
				}
				strengthCalls++
				continue
			}
			next := s.p9TablePair("PetEquipAdvance", "equipId", equipID, "level", strconv.Itoa(int(equip.GetAdvanceLevel()+1)))
			if next == nil {
				break
			}
			call(&pb.CSPetEquipAdvance{PetGuid: pet.Guid, EquipId: proto.String(equipID)})
			advanceCalls++
		}
		for s.p9TablePair("PetEquipAwake", "equipId", equipID, "level", strconv.Itoa(int(pet.GetPetEquipList()[slot].GetAwakeLevel()+1))) != nil {
			call(&pb.CSPetEquipAwake{PetGuid: pet.Guid, EquipId: proto.String(equipID)})
			awakeCalls++
		}
		equip = pet.GetPetEquipList()[slot]
		if equip.GetStrengthLevel() != 100 || equip.GetAdvanceLevel() != 12 || equip.GetAwakeLevel() != 10 {
			t.Fatal("highest original equipment not reached", equip)
		}
		for _, stage := range []struct {
			table string
			level int32
		}{{"PetEquipStrength", 100}, {"PetEquipAdvance", 12}, {"PetEquipAwake", 10}} {
			row := s.p9TablePair(stage.table, "equipId", equipID, "level", strconv.Itoa(int(stage.level)))
			if err = p9AddProperties(expected, row["prop"], 1); err != nil {
				t.Fatal(err)
			}
		}
		deny(&pb.CSPetEquipStrengthOne{PetGuid: pet.Guid, EquipId: proto.String(equipID), Num: proto.Int32(1)})
		deny(&pb.CSPetEquipAdvance{PetGuid: pet.Guid, EquipId: proto.String(equipID)})
		deny(&pb.CSPetEquipAwake{PetGuid: pet.Guid, EquipId: proto.String(equipID)})
	}
	after, _, err := s.p9PetProperties(pet)
	if err != nil {
		t.Fatal(err)
	}
	for key, added := range expected {
		if math.Abs(after[key]-before[key]-added) > 1e-6*math.Max(1, math.Abs(added)) {
			t.Fatalf("original highest equipment property %s mismatch: got=%g before=%g expected=%g", key, after[key], before[key], added)
		}
	}
	return pet, strengthCalls, advanceCalls, awakeCalls
}
