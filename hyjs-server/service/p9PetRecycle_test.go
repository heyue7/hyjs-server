package service

import (
	"context"
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

func TestP9PetRecycle(t *testing.T) {
	ctx := context.Background()
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
	path := filepath.Join(t.TempDir(), "p9-recycle.db")
	open := func() {
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
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetRecycle} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := int64(810000)
	fresh := func(name string) (int64, []*pb.Pet) {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-recycle-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "原精灵出售隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(34)
			_, err := s.GrantFixedReward(p, g, "coin,100000;item_153,1;item_154,2;item_124,1")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		pets := []*pb.Pet{}
		for _, key := range []string{"item_153", "item_154"} {
			num := int32(1)
			if key == "item_154" {
				num = 2
			}
			stamp++
			out, _, err := s.DispatchP9Pets(ctx, row.Id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(num)}}, stamp)
			if err != nil {
				t.Fatal(err)
			}
			pets = append(pets, out[0].(*pb.SCItemUse).GetGet().GetPets()...)
		}
		return row.Id, pets
	}
	state := func(id int64) (*pb.Player, db.Profile) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, _, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, row
	}
	list := func(id int64) []*pb.Pet {
		t.Helper()
		out, err := P9PetList(db.GetIns(), id)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	call := func(id int64, b proto.Message, at int64) (proto.Message, error) {
		raw, err := proto.Marshal(b)
		if err != nil {
			return nil, err
		}
		native := b.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, native); err != nil {
			return nil, err
		}
		out, handled, err := s.DispatchP9PetRecycle(ctx, id, native, at)
		if !handled {
			t.Fatalf("not handled %T", b)
		}
		if err != nil {
			return nil, err
		}
		return out[0], nil
	}
	t.Run("original-price-six-concurrent-exact-guid-and-reopen", func(t *testing.T) {
		id, pets := fresh("sale")
		p, _ := state(id)
		before := p.GetCoin()
		stamp++
		at := stamp
		b := &pb.CSPetSell{Guids: []string{pets[0].GetGuid(), pets[1].GetGuid()}}
		var wg sync.WaitGroup
		results := make(chan proto.Message, 6)
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := call(id, b, at)
				if err != nil {
					errs <- err
				} else {
					results <- out
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(results)
		for err := range errs {
			t.Fatal(err)
		}
		var first proto.Message
		for out := range results {
			if first == nil {
				first = out
			} else if !proto.Equal(first, out) {
				t.Fatal("receipt differed")
			}
		}
		sold := first.(*pb.SCPetSell)
		price := int64(tableNumber(s.table("Pet", "id", pets[0].GetId()), "selloutPrice") + tableNumber(s.table("Pet", "id", pets[1].GetId()), "selloutPrice"))
		p, _ = state(id)
		if p.GetCoin() != before+price || len(sold.GetGet().GetSimps()) != 1 || sold.GetGet().GetSimps()[0].GetId() != "coin" || int64(sold.GetGet().GetSimps()[0].GetNum()) != price || len(sold.GetCost()) != 2 {
			t.Fatalf("price/fee %+v", sold)
		}
		alive := list(id)
		if len(alive) != 1 || alive[0].GetGuid() != pets[2].GetGuid() {
			t.Fatal("wrong GUID deleted")
		}
		for _, pet := range pets[:2] {
			var retired db.P9PetRetired
			if err := db.GetIns().First(&retired, "guid=?", pet.GetGuid()).Error; err != nil {
				t.Fatal(err)
			}
			saved := &pb.Pet{}
			if err := proto.Unmarshal(retired.Data, saved); err != nil || !proto.Equal(saved, pet) {
				t.Fatal("retired source snapshot")
			}
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		retry, err := call(id, b, at)
		if err != nil || !proto.Equal(retry, first) {
			t.Fatalf("reopen retry %v", err)
		}
		_, prior := state(id)
		if _, err = call(id, &pb.CSPetSell{Guids: []string{pets[2].GetGuid()}}, at); err == nil {
			t.Fatal("timestamp collision")
		}
		_, after := state(id)
		if *prior.Player != *after.Player || *prior.Game != *after.Game {
			t.Fatal("collision changed source")
		}
	})
	t.Run("guarded-trained-instance-and-failed-batch-protection", func(t *testing.T) {
		id, pets := fresh("protection")
		stamp++
		if _, _, err := s.DispatchP9PetNext(ctx, id, &pb.CSFormationOn{Id: pets[0].Id, Guid: pets[0].Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
			t.Fatal(err)
		}
		stamp++
		if _, _, err := s.DispatchP9PetNext(ctx, id, &pb.CSPetLevelup{Guid: pets[1].Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}, stamp); err != nil {
			t.Fatal(err)
		}
		_, prior := state(id)
		for _, b := range []proto.Message{&pb.CSPetSell{Guids: []string{pets[2].GetGuid(), pets[0].GetGuid()}}, &pb.CSPetSell{Guids: []string{pets[1].GetGuid()}}, &pb.CSPetSell{Guids: []string{pets[2].GetGuid(), pets[2].GetGuid()}}, &pb.CSPetReset{Guid: pets[1].Guid}, &pb.CSPetDecompose{Guids: []string{pets[2].GetGuid(), pets[0].GetGuid()}}} {
			stamp++
			if _, err := call(id, b, stamp); err == nil {
				t.Fatalf("unsafe request %T", b)
			}
		}
		_, after := state(id)
		if *prior.Player != *after.Player || *prior.Game != *after.Game || len(list(id)) != 3 {
			t.Fatal("denied batch changed source")
		}
		var count int64
		db.GetIns().Model(&db.P9PetRetired{}).Where("player_id=?", id).Count(&count)
		if count != 0 {
			t.Fatal("partial retirement")
		}
		other, foreign := fresh("foreign")
		_ = other
		stamp++
		if _, err := call(id, &pb.CSPetSell{Guids: []string{pets[2].GetGuid(), foreign[0].GetGuid()}}, stamp); err == nil {
			t.Fatal("foreign owner batch")
		}
		if len(list(id)) != 3 {
			t.Fatal("foreign partial delete")
		}
	})
	t.Run("same-id-guid-safety-and-credit-overflow-rollback", func(t *testing.T) {
		id, pets := fresh("overflow")
		if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.Coin = proto.Int64(9223372036854775807); return nil }); err != nil {
			t.Fatal(err)
		}
		stamp++
		if _, err := call(id, &pb.CSPetSell{Guids: []string{pets[0].GetGuid()}}, stamp); err == nil {
			t.Fatal("coin overflow accepted")
		}
		if len(list(id)) != 3 {
			t.Fatal("overflow partial delete")
		}
		var count int64
		db.GetIns().Model(&db.P9PetRetired{}).Where("player_id=?", id).Count(&count)
		if count != 0 {
			t.Fatal("overflow retirement")
		}
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			p.Coin = proto.Int64(100)
			_, err := s.P9GrantPet(tx, p, "pet_002", 3)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		owned := list(id)
		same := []*pb.Pet{}
		for _, pet := range owned {
			if pet.GetId() == "pet_002" {
				same = append(same, pet)
			}
		}
		if len(same) < 3 {
			t.Fatal("bounded donor preparation")
		}
		stamp++
		if _, _, err := s.DispatchP9PetNext(ctx, id, &pb.CSFormationOn{Id: same[0].Id, Guid: same[0].Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
			t.Fatal(err)
		}
		stamp++
		if _, err := call(id, &pb.CSPetSell{Guids: []string{same[1].GetGuid()}}, stamp); err != nil {
			t.Fatal(err)
		}
		survivors := list(id)
		found := false
		for _, pet := range survivors {
			if pet.GetGuid() == same[0].GetGuid() {
				found = true
			}
		}
		if !found {
			t.Fatal("same id guarded GUID erased")
		}
	})
}
