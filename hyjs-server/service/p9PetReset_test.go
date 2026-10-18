package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP9NativePetReset 通过原培养操作建立实际投入，验证退款/旧GUID退休和重试。
// 隔离初始材料明示；本测试不替代材料自然来源与手机入口终验。
func TestP9NativePetReset(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pet-reset.db")
	open := func() {
		t.Helper()
		schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("isolated reset database required")
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
	for _, load := range []func(string) error{s.LoadTables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetRecycle, s.InitializeP9CopyIntimacy} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	account, err := LoginAccount(ctx, "p9-original-reset-fixture", "isolated-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "精灵重置隔离", "hero_043"); err != nil {
		t.Fatal(err)
	}
	id := account.Id
	var pet *pb.Pet
	if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		if _, err := s.GrantFixedReward(p, g, "coin,1000000;diamond,200;item_124,10;item_132,10"); err != nil {
			return err
		}
		pets, err := s.P9GrantPet(tx, p, "pet_002", 1)
		if err != nil {
			return err
		}
		pet = pets[0]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	state := func() (*pb.Player, *model.GameState, db.Profile) {
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
	before, _, _ := state()
	if _, err := s.p9PetLevelup(ctx, id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}, 1001); err != nil {
		t.Fatal(err)
	}
	if _, err := s.p9PetEat(ctx, id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_132"), Num: proto.Int32(2)}}}, 1002); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DispatchP9PetNext(ctx, id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 1003); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, err := s.ApplyP9CopyIntimacy(tx, id, p, g, "sweep:isolated-reset-proof")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, _, guarded := state()
	request := &pb.CSPetReset{Guid: pet.Guid}
	if _, err := s.p9PetReset(ctx, id, request, 1004); err == nil {
		t.Fatal("guarded reset allowed")
	}
	_, _, afterGuard := state()
	if *guarded.Player != *afterGuard.Player || *guarded.Game != *afterGuard.Game {
		t.Fatal("guarded failure charged")
	}
	if _, _, err := s.DispatchP9PetNext(ctx, id, &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 1005); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *pb.SCPetReset, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.p9PetReset(ctx, id, request, 1006)
			if err != nil {
				errs <- err
			} else {
				results <- result[0].(*pb.SCPetReset)
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Fatal(err)
	}
	var first *pb.SCPetReset
	for result := range results {
		if first == nil {
			first = result
		} else if !proto.Equal(first, result) {
			t.Fatal("concurrent receipt changed")
		}
	}
	if first == nil || len(first.GetGet().GetPets()) != 1 || len(first.GetCost()) != 2 {
		t.Fatal("original delete/add reset shape")
	}
	replacement := first.GetGet().GetPets()[0]
	if replacement.GetId() != pet.GetId() || replacement.GetGuid() == pet.GetGuid() || !p9BasicDonor(replacement) {
		t.Fatal("reset did not restore fresh base instance")
	}
	p, g, _ := state()
	if p.GetCoin() != before.GetCoin() || p.GetDiamond() != before.GetDiamond()-50 {
		t.Fatal("refund or original reset fee incorrect")
	}
	items := map[string]int32{}
	for _, item := range g.Items {
		items[item.GetId()] = item.GetNum()
	}
	if items["item_124"] != 10 || items["item_132"] != 10 || replacement.GetEat().GetExp() != 0 {
		t.Fatal("paid food refunded or free intimacy reset incorrectly")
	}
	var facts db.P9PetCopyGrant
	if err := db.GetIns().First(&facts, "player_id=? AND source_key=?", id, "sweep:isolated-reset-proof").Error; err != nil {
		t.Fatal(err)
	}
	var ledger []P9CopyIntimacyFact
	if json.Unmarshal(facts.Facts, &ledger) != nil || len(ledger) != 1 || ledger[0].GUID != pet.GetGuid() || ledger[0].Earned != 2 {
		t.Fatal("free source history rewritten")
	}
	var retired db.P9PetRetired
	if err := db.GetIns().First(&retired, "guid=?", pet.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	old := &pb.Pet{}
	if proto.Unmarshal(retired.Data, old) != nil || old.GetEat().GetExp() <= 2 {
		t.Fatal("retirement source lost")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	retry, err := s.p9PetReset(ctx, id, request, 1006)
	if err != nil || !proto.Equal(first, retry[0]) {
		t.Fatal("cold reopen duplicated reset", err)
	}
	if _, err = s.p9PetReset(ctx, id, request, 1007); err == nil {
		t.Fatal("retired GUID reset again")
	}
	owned, err := P9PetList(db.GetIns(), id)
	if err != nil || len(owned) != 1 || owned[0].GetGuid() != replacement.GetGuid() {
		t.Fatal("old receipt resurrected source")
	}
	if _, err := s.p9PetLevelup(ctx, id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}, 1001); err == nil {
		t.Fatal("retired GUID returned through stale growth receipt")
	}
	if _, _, err := s.DispatchP9PetNext(ctx, id, &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 1005); err == nil {
		t.Fatal("retired guardian returned through stale formation receipt")
	}
	t.Run("donor-refund-capacity-rollback-and-unknown-ledger", func(t *testing.T) {
		pet = replacement
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			_, err := s.GrantFixedReward(p, g, "coin,10000000;item_124,20000;item_126,5000;item_139,1000;item_144,1000;item_145,1000")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		refundBase, refundGame, _ := state()
		stamp := int64(2000)
		toLevel := func(level int32) {
			t.Helper()
			gain := -pet.GetExp()
			for i := pet.GetLevel(); i < level; i++ {
				need, err := s.p9LevelExp(pet, i)
				if err != nil {
					t.Fatal(err)
				}
				gain += need
			}
			costs := []*pb.BaseResource{}
			if gain/1000 > 0 {
				costs = append(costs, &pb.BaseResource{Id: proto.String("item_126"), Num: proto.Int32(int32(gain / 1000))})
			}
			if rem := gain % 1000; rem > 0 {
				costs = append(costs, &pb.BaseResource{Id: proto.String("item_124"), Num: proto.Int32(int32((rem + 99) / 100))})
			}
			stamp++
			result, err := s.p9PetLevelup(ctx, id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: costs}, stamp)
			if err != nil {
				t.Fatal(err)
			}
			pet = result[0].(*pb.SCPetLevelup).GetPet()
		}
		breach := func() {
			t.Helper()
			stamp++
			result, err := s.p9PetBreach(ctx, id, &pb.CSPetBreached{Guid: pet.Guid}, stamp)
			if err != nil {
				t.Fatal(err)
			}
			pet = result[0].(*pb.SCPetBreached).GetPet()
		}
		toLevel(30)
		breach()
		toLevel(50)
		var donor *pb.Pet
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			pets, err := s.P9GrantPet(tx, p, pet.GetId(), 1)
			if err == nil {
				donor = pets[0]
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		breach()
		if _, err := p9OwnedPet(db.GetIns(), id, donor.GetGuid()); err == nil {
			t.Fatal("donor not consumed")
		}
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.PetBagSize = proto.Int32(1); return nil }); err != nil {
			t.Fatal(err)
		}
		_, _, beforeFail := state()
		var retireBefore int64
		db.GetIns().Model(&db.P9PetRetired{}).Where("player_id=?", id).Count(&retireBefore)
		reset := &pb.CSPetReset{Guid: pet.Guid}
		if _, err := s.p9PetReset(ctx, id, reset, 2010); err == nil {
			t.Fatal("donor refund overflow allowed")
		}
		_, _, afterFail := state()
		var retireAfter int64
		db.GetIns().Model(&db.P9PetRetired{}).Where("player_id=?", id).Count(&retireAfter)
		if *beforeFail.Player != *afterFail.Player || *beforeFail.Game != *afterFail.Game || retireBefore != retireAfter {
			t.Fatal("capacity failure partially committed")
		}
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.PetBagSize = proto.Int32(100); return nil }); err != nil {
			t.Fatal(err)
		}
		result, err := s.p9PetReset(ctx, id, reset, 2010)
		if err != nil {
			t.Fatal(err)
		}
		returned := result[0].(*pb.SCPetReset).GetGet().GetPets()
		if len(returned) != 2 || returned[0].GetGuid() == pet.GetGuid() || returned[1].GetGuid() == donor.GetGuid() {
			t.Fatal("original donor refunds missing or old GUID restored")
		}
		p, g, _ := state()
		if p.GetCoin() != refundBase.GetCoin() || p.GetDiamond() != refundBase.GetDiamond()-50 {
			t.Fatal("growth and breach costs not refunded")
		}
		for _, item := range refundGame.Items {
			if simpleOwned(p, g, item.GetId()) != int64(item.GetNum()) {
				t.Fatal("material refund mismatch", item.GetId())
			}
		}
		// 明示旧档缺少投入账本；只变更隔离实例，未知成本不能凭等级猜算。
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			returned[0].Level = proto.Int32(2)
			return p9SavePet(tx, id, g, returned[0])
		}); err != nil {
			t.Fatal(err)
		}
		_, _, unknownBefore := state()
		if _, err := s.p9PetReset(ctx, id, reset, 2010); err == nil {
			t.Fatal("old base reset projection replaced later level2 instance")
		}
		if _, err := s.p9PetReset(ctx, id, &pb.CSPetReset{Guid: returned[0].Guid}, 2011); err == nil {
			t.Fatal("unknown trained investment fabricated")
		}
		_, _, unknownAfter := state()
		if *unknownBefore.Player != *unknownAfter.Player || *unknownBefore.Game != *unknownAfter.Game {
			t.Fatal("unknown ledger failure charged")
		}
	})

}
