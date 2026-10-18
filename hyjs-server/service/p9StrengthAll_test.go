package service

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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

func TestP9StrengthAllPartialBalanceConcurrentColdAndExpiredReceipt(t *testing.T) {
	s, ctx, path := p9IntimacyFixture(t)
	account, err := LoginAccount(ctx, "p9-strength-all", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "一键隔离", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var pet *pb.Pet
	if err = updateProfileWithTx(ctx, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(1000000)
		p.Diamond = proto.Int64(1000)
		if _, err := s.GrantFixedReward(p, g, "item_124,1000"); err != nil {
			return err
		}
		pets, err := s.P9GrantPet(tx, p, "pet_046", 1)
		if err == nil {
			pet = pets[0]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The real level-material mutation makes this instance eligible for more than one level.
	if _, _, err = s.DispatchP9PetNext(ctx, account.Id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(100)}}}, 6400000); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.Coin = proto.Int64(3779); return nil }); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HYJS_P9_STRENGTH_ALL_KEEP") == "1" {
		if os.Getenv("HYJS_P9_INTIMACY_MYSQL_DATABASE") == "" {
			t.Fatal("one-key original HTTP fixture requires isolated InnoDB")
		}
		owned, err := p9OwnedPet(db.GetIns(), account.Id, pet.GetGuid())
		if err != nil {
			t.Fatal(err)
		}
		wire, err := proto.Marshal(owned)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(map[string]any{"playerId": account.Id, "petId": owned.GetId(), "petWire": fmt.Sprintf("%x", wire), "scope": "prepared finite pet/material/budget; actual paid pet level mutation; one-key delegated to original signed HTTP", "nativeCompared": false})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(os.Getenv("HYJS_P9_STRENGTH_ALL_OUTPUT"), encoded, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	request := &pb.CSPetEquipStrengthAll{PetGuid: pet.Guid}
	results := make(chan *pb.SCPetEquipStrengthAll, 8)
	errors := make(chan error, 8)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, handled, err := s.DispatchP9PetEquip(ctx, account.Id, request, 6400001)
			if err != nil {
				errors <- err
				return
			}
			if !handled {
				errors <- p5Error()
				return
			}
			results <- out[0].(*pb.SCPetEquipStrengthAll)
		}()
	}
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var first *pb.SCPetEquipStrengthAll
	for result := range results {
		if first == nil {
			first = result
		} else if !proto.Equal(first, result) {
			t.Fatal("concurrent aggregate receipts diverged")
		}
	}
	if len(first.GetPetEquip()) != 2 || len(first.GetCost()) != 1 || first.GetCost()[0].GetId() != "coin" || first.GetCost()[0].GetNum() != 2520 {
		t.Fatal("original first-row partial budget", first)
	}
	for i, equip := range first.GetPetEquip() {
		if equip.GetEquipId() != pet.GetPetEquipList()[i].GetEquipId() || equip.GetStrengthLevel() != 1 {
			t.Fatal("published original slot order", equip)
		}
	}
	var before db.Profile
	if err = db.GetIns().First(&before, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&before)
	if err != nil || p.GetCoin() != 1259 {
		t.Fatal("aggregate charge repeated", err)
	}
	var investment db.P9PetInvestment
	if err = db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil || !strings.Contains(investment.Source, constUtil.P9PetStrengthAllSource) {
		t.Fatal("local policy provenance missing", err)
	}
	pool, _ := db.GetIns().DB()
	_ = pool.Close()
	p9IntimacyOpen(t, ctx, path)
	cold, _ := db.GetIns().DB()
	t.Cleanup(func() { cold.Close() })
	out, _, err := s.DispatchP9PetEquip(ctx, account.Id, request, 6400001)
	if err != nil || !proto.Equal(first, out[0]) {
		t.Fatal("cold aggregate receipt", err)
	}
	if _, _, err = s.DispatchP9PetEquip(ctx, account.Id, request, 6400002); err == nil {
		t.Fatal("zero affordable progress accepted")
	}
	var after db.Profile
	if err = db.GetIns().First(&after, account.Id).Error; err != nil || *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("no-progress mutation", err)
	}
	if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.Coin = proto.Int64(100000000); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.DispatchP9PetEquip(ctx, account.Id, request, 6400003); err != nil {
		t.Fatal(err)
	}
	current, err := p9OwnedPet(db.GetIns(), account.Id, pet.GetGuid())
	if err != nil {
		t.Fatal(err)
	}
	for _, equip := range current.GetPetEquipList() {
		cap, err := s.p9EquipStrengthCap(current, equip)
		if err != nil || equip.GetStrengthLevel() != cap {
			t.Fatal("original level/advance limit", err)
		}
	}
	if err = db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	costs := map[string]int64{}
	if err = json.Unmarshal(investment.Costs, &costs); err != nil {
		t.Fatal(err)
	}
	if costs["coin"] <= 2520 || costs["item_124"] != 100 {
		t.Fatal("actual investment omitted paid levels", costs)
	}
	if _, _, err = s.DispatchP9PetEquip(ctx, account.Id, request, 6400001); err == nil {
		t.Fatal("old array projection overwrites later equipment")
	}
	var oldInvestment db.P9PetInvestment
	if err = db.GetIns().First(&oldInvestment, "guid=?", pet.GetGuid()).Error; err != nil || !reflect.DeepEqual(oldInvestment.Costs, investment.Costs) {
		t.Fatal("expired array receipt changed investment", err)
	}
}
