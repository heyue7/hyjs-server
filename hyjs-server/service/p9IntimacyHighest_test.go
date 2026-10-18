package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
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

func p9IntimacyFixture(t *testing.T) (*GameService, context.Context, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "intimacy-highest.db")
	p9IntimacyOpen(t, ctx, path)
	pool, _ := db.GetIns().DB()
	t.Cleanup(func() { pool.Close() })
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetRecycle, s.InitializeP9CopyIntimacy, s.InitializeP9PetIntimacy} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return s, ctx, path
}

// p9IntimacyOpen keeps SQL verification in explicitly isolated schemas.
func p9IntimacyOpen(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	var err error
	if schema := os.Getenv("HYJS_P9_INTIMACY_MYSQL_DATABASE"); schema != "" {
		if !strings.HasPrefix(schema, "hyjs_test_p9_") {
			t.Fatal("P9 intimacy isolated schema guard")
		}
		err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
	} else {
		err = db.InitGame(ctx, path)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestP9IntimacyHighestAll78ActualFoodRandomPropsColdAndRefund(t *testing.T) {
	s, ctx, path := p9IntimacyFixture(t)
	type saved struct {
		id     int64
		pet    *pb.Pet
		coin   int64
		food   int32
		battle *pb.BattlePlayer
	}
	cases := []saved{}
	stamp := int64(4600000)
	for _, row := range s.Tables["Pet"] {
		key := row["id"]
		if key == "" {
			continue
		}
		t.Run(key, func(t *testing.T) {
			account, err := LoginAccount(ctx, "p9-intimacy-highest-"+key, "local-test-password")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "亲密最高隔离", "hero_043"); err != nil {
				t.Fatal(err)
			}
			var pet *pb.Pet
			if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(100000000)
				p.Diamond = proto.Int64(1000)
				if _, err := s.GrantFixedReward(p, g, "item_128,10000"); err != nil {
					return err
				}
				pets, err := s.P9GrantPet(tx, p, key, 1)
				if err != nil {
					return err
				}
				pet = pets[0]
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			base, _, err := s.p9PetProperties(pet)
			if err != nil {
				t.Fatal(err)
			}
			unit := int64(tableNumber(s.table("Item", "itemId", "item_128"), "exp")) * int64(tableNumber(s.table("GameConfigure", "StaticName", "PetDifferentNatureIntimacyProportion"), "NameValue"))
			if s.table("Item", "itemId", "item_128")["nature"] == row["nature"] {
				unit = int64(tableNumber(s.table("Item", "itemId", "item_128"), "exp")) * int64(tableNumber(s.table("GameConfigure", "StaticName", "PetSameNatureIntimacyProportion"), "NameValue"))
			}
			if unit < 1 {
				t.Fatal("missing original food gain")
			}
			spent := int32(0)
			for pet.GetEat().GetLevel() < 7 {
				current := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(pet.GetEat().GetLevel())))
				need := int64(tableNumber(current, "intimacy")) - int64(pet.GetEat().GetExp())
				num := int32((need + unit - 1) / unit)
				if num > 1000 {
					num = 1000
				}
				if num < 1 {
					t.Fatal("invalid original food threshold")
				}
				stamp++
				request := &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(num)}}}
				out, handled, err := s.DispatchP9PetIntimacy(ctx, account.Id, request, stamp)
				if !handled || err != nil {
					t.Fatal(err)
				}
				pet = out[0].(*pb.SCPetEat).GetPet()
				spent += num
				unlocked := int(tableNumber(s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(pet.GetEat().GetLevel()))), "additionalTalent"))
				if len(pet.GetRandPropList()) != unlocked {
					t.Fatal("actual original slot count mismatch")
				}
				seen := map[string]bool{pet.GetInitProp(): true}
				for _, prop := range pet.GetRandPropList() {
					if seen[prop] {
						t.Fatal("duplicate allocated property")
					}
					seen[prop] = true
				}
			}
			if pet.GetEat().GetExp() != 0 {
				t.Fatal("highest terminal experience not zero")
			}
			expected := map[string]float64{}
			for _, prop := range pet.GetRandPropList() {
				if err = p9AddProperties(expected, s.p9TablePair("PetTalent", "petTalentId", prop, "talentLevel", "0")["talent"], 1); err != nil {
					t.Fatal(err)
				}
			}
			actual, _, err := s.p9PetProperties(pet)
			if err != nil {
				t.Fatal(err)
			}
			for property, added := range expected {
				if math.Abs(actual[property]-base[property]-added) > 1e-8 {
					t.Fatal("highest intimacy property mismatch", property)
				}
			}
			var before db.Profile
			if err = db.GetIns().First(&before, account.Id).Error; err != nil {
				t.Fatal(err)
			}
			stamp++
			if _, _, err = s.DispatchP9PetIntimacy(ctx, account.Id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(1)}}}, stamp); err == nil {
				t.Fatal("highest food accepted")
			}
			var after db.Profile
			if err = db.GetIns().First(&after, account.Id).Error; err != nil || *after.Player != *before.Player || *after.Game != *before.Game {
				t.Fatal("highest denial changed profile", err)
			}
			stamp++
			if _, _, err = s.DispatchP9PetNext(ctx, account.Id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().First(&after, account.Id).Error; err != nil {
				t.Fatal(err)
			}
			p, g, err := decodeProfile(&after)
			if err != nil {
				t.Fatal(err)
			}
			battle, err := s.battlePlayer(p, g)
			if err != nil {
				t.Fatal(err)
			}
			cases = append(cases, saved{account.Id, proto.Clone(pet).(*pb.Pet), p.GetCoin(), 10000 - spent, battle})
		})
	}
	if t.Failed() {
		return
	}
	if len(cases) != 78 {
		t.Fatal("all78 intimacy coverage", len(cases))
	}
	pool, _ := db.GetIns().DB()
	_ = pool.Close()
	p9IntimacyOpen(t, ctx, path)
	cold, _ := db.GetIns().DB()
	t.Cleanup(func() { cold.Close() })
	records := []map[string]any{}
	keepHighest := os.Getenv("HYJS_P9_INTIMACY_KEEP") == "1"
	for _, old := range cases {
		pet, err := p9OwnedPet(db.GetIns(), old.id, old.pet.GetGuid())
		if err != nil || !proto.Equal(pet, old.pet) {
			t.Fatal("cold random properties changed", err)
		}
		var profile db.Profile
		if err = db.GetIns().First(&profile, old.id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		battle, err := s.battlePlayer(p, g)
		if err != nil || !proto.Equal(battle, old.battle) {
			t.Fatal("cold guardian changed", err)
		}
		petWire, err := proto.Marshal(pet)
		if err != nil {
			t.Fatal(err)
		}
		battleWire, err := proto.Marshal(battle)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, map[string]any{"playerId": old.id, "petId": pet.GetId(), "petWire": fmt.Sprintf("%x", petWire), "battleWire": fmt.Sprintf("%x", battleWire), "initialCoin": old.coin, "initialDiamond": 1000, "initialItems": map[string]int32{"item_128": 10000}, "spentFood": 10000 - old.food, "donors": 0})
		if keepHighest {
			continue
		}
		stamp++
		if _, _, err = s.DispatchP9PetNext(ctx, old.id, &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, handled, err := s.DispatchP9PetRecycle(ctx, old.id, &pb.CSPetReset{Guid: pet.Guid}, stamp)
		if !handled || err != nil {
			t.Fatal("food refund", err)
		}
		fresh := out[0].(*pb.SCPetReset).GetGet().GetPets()
		if len(fresh) != 1 || len(fresh[0].GetRandPropList()) != 0 || fresh[0].GetEat().GetLevel() != 0 {
			t.Fatal("reset retained unlocked random properties")
		}
		if err = db.GetIns().First(&profile, old.id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err = decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		var food int32
		for _, item := range g.Items {
			if item.GetId() == "item_128" {
				food = item.GetNum()
			}
		}
		if p.GetCoin() != old.coin || food != 10000 {
			t.Fatal("food investment was not fully refunded")
		}
	}
	if output := os.Getenv("HYJS_P9_INTIMACY_OUTPUT"); output != "" {
		encoded, err := json.MarshalIndent(map[string]any{"records": records, "resetByGo": !keepHighest, "scope": "prepared finite food; actual 78 original intimacy maximum, frozen local random allocation, guardian and cold; natural and native gates remain", "nativeCompared": false, "authoritative": false}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("78 pets actual food -> all original intimacy 0..7 -> allocated properties -> guardian -> cold -> exact food refund; local probability policy, no natural/native claim")
}

func TestP9IntimacyDeferredCreditConcurrentColdAndImmutableOldReceipt(t *testing.T) {
	s, ctx, path := p9IntimacyFixture(t)
	account, err := LoginAccount(ctx, "p9-deferred-credit", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "留账隔离", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var pet *pb.Pet
	if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		pets, err := s.P9GrantPet(tx, p, "pet_002", 1)
		if err != nil {
			return err
		}
		pet = pets[0]
		pet.Eat.Exp = proto.Int32(625)
		return p9SavePet(tx, account.Id, g, pet)
	}); err != nil {
		t.Fatal(err)
	}
	fact := []P9CopyIntimacyFact{{GUID: pet.GetGuid(), Earned: 2, Applied: 0, Deferred: 2, Capped: 0}}
	raw, _ := json.Marshal(fact)
	frozen := []byte("immutable-original-receipt")
	grant := db.P9PetCopyGrant{PlayerID: account.Id, SourceKey: "copy:actual-legacy-award", CompletedCount: 1, Facts: raw, Response: frozen, CreatedAt: 1, Source: "actual deferred credit fixture; resources not refunded"}
	if err = db.GetIns().Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan *pb.Pet, 8)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			out, _, err := s.DispatchP9PetIntimacy(ctx, account.Id, &pb.CSPetHudongShow{Guid: pet.Guid}, 1)
			if err != nil {
				errs <- err
				return
			}
			replies <- out[0].(*pb.SCPetHudongShow).GetPet()
		}()
	}
	wait.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		t.Fatal(err)
	}
	var first *pb.Pet
	for response := range replies {
		if first == nil {
			first = response
		} else if !proto.Equal(first, response) {
			t.Fatal("concurrent deferred draws diverged")
		}
	}
	if first.GetEat().GetLevel() != 1 || first.GetEat().GetExp() != 1 || len(first.GetRandPropList()) != 1 {
		t.Fatal("deferred points were not applied exactly once", first)
	}
	var journal []db.P9PetDeferredConsumption
	if err = db.GetIns().Find(&journal).Error; err != nil || len(journal) != 1 || journal[0].Applied != 2 || journal[0].Capped != 0 {
		t.Fatal("deferred journal mismatch", err)
	}
	if err = db.GetIns().First(&grant, "player_id=? AND source_key=?", account.Id, grant.SourceKey).Error; err != nil || !reflect.DeepEqual(grant.Response, frozen) || !reflect.DeepEqual(grant.Facts, raw) {
		t.Fatal("original receipt changed", err)
	}
	pool, _ := db.GetIns().DB()
	_ = pool.Close()
	p9IntimacyOpen(t, ctx, path)
	cold, _ := db.GetIns().DB()
	t.Cleanup(func() { cold.Close() })
	out, _, err := s.DispatchP9PetIntimacy(ctx, account.Id, &pb.CSPetHudongShow{Guid: pet.Guid}, 2)
	if err != nil || !proto.Equal(first, out[0].(*pb.SCPetHudongShow).GetPet()) {
		t.Fatal("cold deferred credit reissued", err)
	}
	t.Log(fmt.Sprintf("original grant remains frozen; applied=%d capped=%d; eight concurrent then cold replay preserves random choice", journal[0].Applied, journal[0].Capped))
}

func TestP9IntimacyPublishedPolicyAndTerminalOvershootRollback(t *testing.T) {
	s, ctx, _ := p9IntimacyFixture(t)
	account, err := LoginAccount(ctx, "p9-intimacy-policy", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "策略隔离", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var pet *pb.Pet
	if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		if _, err := s.GrantFixedReward(p, g, "item_128,100"); err != nil {
			return err
		}
		pets, err := s.P9GrantPet(tx, p, "pet_046", 1)
		if err != nil {
			return err
		}
		pet = pets[0]
		pet.Eat.Exp = proto.Int32(625)
		return p9SavePet(tx, account.Id, g, pet)
	}); err != nil {
		t.Fatal(err)
	}
	stamp := int64(5700000)
	feed := func(num int32) error {
		stamp++
		_, _, err := s.DispatchP9PetIntimacy(ctx, account.Id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(num)}}}, stamp)
		return err
	}
	if err = feed(1); err != nil {
		t.Fatal(err)
	}
	pet, err = p9OwnedPet(db.GetIns(), account.Id, pet.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	if len(pet.GetRandPropList()) != 1 {
		t.Fatal("first actual allocation missing")
	}
	var originalRule db.P9PetIntimacyRule
	if err = db.GetIns().First(&originalRule, "quality=?", 23).Error; err != nil {
		t.Fatal(err)
	}
	setBoundary := func(level int32) {
		t.Helper()
		if err := updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			owned, err := p9OwnedPet(tx, account.Id, pet.GetGuid())
			if err != nil {
				return err
			}
			owned.Eat.Level = proto.Int32(level)
			owned.Eat.Exp = proto.Int32(int32(tableNumber(s.p9TablePair("PetIntimacy", "quality", "23", "intimacyLevel", strconv.Itoa(int(level))), "intimacy")) - 1)
			return p9SavePet(tx, account.Id, g, owned)
		}); err != nil {
			t.Fatal(err)
		}
	}
	setBoundary(2)
	deny := func(num int32) {
		t.Helper()
		var before, after db.Profile
		var allocationsBefore, allocationsAfter []db.P9PetTalentAllocation
		if err := db.GetIns().First(&before, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Find(&allocationsBefore).Error; err != nil {
			t.Fatal(err)
		}
		if err := feed(num); err == nil {
			t.Fatal("invalid policy/overshoot accepted")
		}
		if err := db.GetIns().First(&after, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Find(&allocationsAfter).Error; err != nil {
			t.Fatal(err)
		}
		if *before.Player != *after.Player || *before.Game != *after.Game || !reflect.DeepEqual(allocationsBefore, allocationsAfter) {
			t.Fatal("failed allocation changed profile/audit")
		}
	}
	for _, changed := range []db.P9PetIntimacyRule{
		{Quality: 23, Enabled: false, Version: 1, Pool: originalRule.Pool, Source: originalRule.Source},
		{Quality: 23, Enabled: true, Version: 1, Pool: []byte(`["PetTalent_2","PetTalent_2","PetTalent_3","PetTalent_4","PetTalent_5"]`), Source: originalRule.Source},
		{Quality: 23, Enabled: true, Version: 2, Pool: originalRule.Pool, Source: originalRule.Source},
	} {
		if err = db.GetIns().Save(&changed).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.InitializeP9PetIntimacy(ctx); err != nil {
			t.Fatal(err)
		}
		var kept db.P9PetIntimacyRule
		if err = db.GetIns().First(&kept, "quality=?", 23).Error; err != nil || kept.Enabled != changed.Enabled || kept.Version != changed.Version || string(kept.Pool) != string(changed.Pool) {
			t.Fatal("startup overwrote operator policy", err)
		}
		deny(1)
	}
	if err = db.GetIns().Save(&originalRule).Error; err != nil {
		t.Fatal(err)
	}
	// Advance through real food before preparing only the final overflow boundary.
	if err = feed(1); err != nil {
		t.Fatal(err)
	}
	setBoundary(4)
	if err = feed(1); err != nil {
		t.Fatal(err)
	}
	setBoundary(6)
	deny(2)
	if err = feed(1); err != nil {
		t.Fatal(err)
	}
	current, err := p9OwnedPet(db.GetIns(), account.Id, pet.GetGuid())
	if err != nil || current.GetEat().GetLevel() != 7 || current.GetEat().GetExp() != 0 || len(current.GetRandPropList()) != 4 {
		t.Fatal("minimal final food rounding did not reach original limit", err)
	}
	var allocation db.P9PetTalentAllocation
	if err = db.GetIns().First(&allocation, "guid=?", pet.GetGuid()).Error; err != nil || !strings.Contains(allocation.Source, constUtil.P9IntimacyTerminalSource) {
		t.Fatal("named terminal compatibility source absent", err)
	}
}

func TestP9IntimacyImportedAttributeOrderAndDuplicatesRemainUnchanged(t *testing.T) {
	s, ctx, _ := p9IntimacyFixture(t)
	account, err := LoginAccount(ctx, "p9-imported-intimacy", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "历史属性隔离", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var pet *pb.Pet
	if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		if _, err := s.GrantFixedReward(p, g, "item_128,1"); err != nil {
			return err
		}
		pets, err := s.P9GrantPet(tx, p, "pet_002", 1)
		if err != nil {
			return err
		}
		pet = pets[0]
		pet.RandPropList = []string{"PetTalent_12", "PetTalent_12"}
		pet.Eat.Level = proto.Int32(4)
		pet.Eat.Exp = proto.Int32(9399)
		return p9SavePet(tx, account.Id, g, pet)
	}); err != nil {
		t.Fatal(err)
	}
	out, handled, err := s.DispatchP9PetIntimacy(ctx, account.Id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(1)}}}, 5900000)
	if !handled || err != nil {
		t.Fatal(err)
	}
	current := out[0].(*pb.SCPetEat).GetPet()
	if current.GetEat().GetLevel() != 5 || len(current.GetRandPropList()) != 3 || current.GetRandPropList()[0] != "PetTalent_12" || current.GetRandPropList()[1] != "PetTalent_12" || current.GetRandPropList()[2] == "PetTalent_12" || current.GetRandPropList()[2] == pet.GetInitProp() {
		t.Fatal("imported allocation was rerolled or normalized", current)
	}
}

func TestP9IntimacyDisabledPolicyDefersCopyCreditWithoutBlockingSettlement(t *testing.T) {
	s, ctx, _ := p9IntimacyFixture(t)
	account, err := LoginAccount(ctx, "p9-paused-policy-credit", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "暂停策略留账", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var pet *pb.Pet
	if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		pets, err := s.P9GrantPet(tx, p, "pet_002", 1)
		if err != nil {
			return err
		}
		pet = pets[0]
		pet.Eat.Exp = proto.Int32(625)
		return p9SavePet(tx, account.Id, g, pet)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.DispatchP9PetNext(ctx, account.Id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 6100000); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P9PetIntimacyRule{}).Where("quality=?", 20).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		adds, err := s.ApplyP9CopyIntimacy(tx, account.Id, p, g, "sweep:disabled-policy-fixture")
		if err != nil {
			return err
		}
		if len(adds) != 0 {
			t.Fatal("unsupported new attribute applied")
		}
		return nil
	}); err != nil {
		t.Fatal("disabled optional policy blocked the caller transaction", err)
	}
	var grant db.P9PetCopyGrant
	if err = db.GetIns().First(&grant, "player_id=?", account.Id).Error; err != nil {
		t.Fatal(err)
	}
	facts := []P9CopyIntimacyFact{}
	if err = json.Unmarshal(grant.Facts, &facts); err != nil || len(facts) != 1 || facts[0].Earned != 2 || facts[0].Deferred != 2 || facts[0].Applied != 0 {
		t.Fatal("copy credit lost", err)
	}
	show := func() *pb.Pet {
		t.Helper()
		out, handled, err := s.DispatchP9PetIntimacy(ctx, account.Id, &pb.CSPetHudongShow{Guid: pet.Guid}, 6100001)
		if !handled || err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCPetHudongShow).GetPet()
	}
	current := show()
	if current.GetEat().GetExp() != 625 || len(current.GetRandPropList()) != 0 {
		t.Fatal("disabled policy consumed deferred credit")
	}
	var count int64
	if err = db.GetIns().Model(&db.P9PetDeferredConsumption{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("disabled policy wrote a consumption", err)
	}
	if err = db.GetIns().Model(&db.P9PetIntimacyRule{}).Where("quality=?", 20).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	current = show()
	if current.GetEat().GetLevel() != 1 || current.GetEat().GetExp() != 1 || len(current.GetRandPropList()) != 1 {
		t.Fatal("reenabled policy did not consume exact credit")
	}
	if !proto.Equal(current, show()) {
		t.Fatal("deferred credit reissued")
	}
}
