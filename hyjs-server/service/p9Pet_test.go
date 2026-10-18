package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9PetInstances(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9.db")
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
	open := func() {
		t.Helper()
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("P9 isolated schema rejected")
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
	for _, initialize := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	fresh := func(name string, level int32, bag int32) int64 {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "精灵隔离回归", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(level)
			p.PetBagSize = proto.Int32(bag)
			_, err := s.GrantFixedReward(p, g, "item_154,3;item_153,1")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return row.Id
	}
	stamp := int64(310000)
	call := func(id int64, b proto.Message) proto.Message {
		t.Helper()
		raw, err := proto.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		decoded := b.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, handled, err := s.DispatchP9Pets(ctx, id, decoded, stamp)
		if err != nil || !handled {
			t.Fatalf("%T handled=%v err=%v", b, handled, err)
		}
		return out[0]
	}
	snapshot := func(id int64) (db.Profile, []*pb.Pet) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		pets, err := P9PetList(db.GetIns(), id)
		if err != nil {
			t.Fatal(err)
		}
		return row, pets
	}
	deny := func(id int64, b proto.Message) {
		t.Helper()
		before, pets := snapshot(id)
		stamp++
		if _, handled, err := s.DispatchP9Pets(ctx, id, b, stamp); err == nil || !handled {
			t.Fatal("P9 denial missing", b, err)
		}
		after, now := snapshot(id)
		if *before.Player != *after.Player || *before.Game != *after.Game || len(pets) != len(now) {
			t.Fatal("failed request changed source")
		}
	}
	t.Run("original-34-gate-and-complete-first-pet", func(t *testing.T) {
		id := fresh("gate", 33, 100)
		if len(call(id, &pb.CSPetList{}).(*pb.SCPetList).GetPetList()) != 0 {
			t.Fatal("empty inventory changed")
		}
		deny(id, &pb.CSPetFirstRecruit{})
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(34); return nil }); err != nil {
			t.Fatal(err)
		}
		first := call(id, &pb.CSPetFirstRecruit{}).(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
		if first.GetId() != "pet_002" || first.GetGuid() == "" || first.GetLevel() != 1 || first.GetInitProp() != "PetTalent_11" || first.GetEat() == nil || first.GetBreached() == nil || len(first.GetTalentList()) != 2 {
			t.Fatal(first)
		}
		if list := call(id, &pb.CSPetList{}).(*pb.SCPetList).GetPetList(); len(list) != 1 || !proto.Equal(first, list[0]) {
			t.Fatal("list is not actual inventory")
		}
		deny(id, &pb.CSPetFirstRecruit{})
	})
	t.Run("eight-concurrent-exact-first-receipt-and-process-store-reopen", func(t *testing.T) {
		id := fresh("receipts", 34, 100)
		stamp++
		frozen := stamp
		var wg sync.WaitGroup
		errors := make(chan error, 8)
		replies := make(chan string, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, _, err := s.DispatchP9Pets(ctx, id, &pb.CSPetFirstRecruit{}, frozen)
				if err != nil {
					errors <- err
					return
				}
				data, err := proto.Marshal(out[0])
				if err != nil {
					errors <- err
				} else {
					replies <- string(data)
				}
			}()
		}
		wg.Wait()
		close(errors)
		close(replies)
		for err := range errors {
			t.Fatal(err)
		}
		first := ""
		for reply := range replies {
			if first == "" {
				first = reply
			}
			if first != reply {
				t.Fatal("GUID duplicated across receipt")
			}
		}
		_, pets := snapshot(id)
		if len(pets) != 1 {
			t.Fatal("concurrent duplicate", pets)
		}
		collision := &pb.CSPetFirstRecruit{}
		collision.ProtoReflect().SetUnknown([]byte{8, 1})
		if _, _, err := s.DispatchP9Pets(ctx, id, collision, frozen); err == nil {
			t.Fatal("receipt collision accepted")
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		out, _, err := s.DispatchP9Pets(ctx, id, &pb.CSPetFirstRecruit{}, frozen)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := proto.Marshal(out[0])
		if string(data) != first {
			t.Fatal("reopen changed unique GUID")
		}
	})
	t.Run("original-box-weighted-real-pets-and-batch-rollback", func(t *testing.T) {
		id := fresh("boxes", 34, 10)
		box := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_154"), Num: proto.Int32(3)}}
		out := call(id, box).(*pb.SCItemUse)
		if len(out.GetGet().GetPets()) != 3 {
			t.Fatal(out)
		}
		seen := map[string]bool{}
		for _, pet := range out.GetGet().GetPets() {
			if seen[pet.GetGuid()] || s.table("Pet", "id", pet.GetId()) == nil {
				t.Fatal("nonoriginal or duplicate instance", pet)
			}
			seen[pet.GetGuid()] = true
		}
		_, pets := snapshot(id)
		if len(pets) != 3 {
			t.Fatal(pets)
		}
		red := call(id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_153"), Num: proto.Int32(1)}}).(*pb.SCItemUse).GetGet().GetPets()[0]
		if s.table("Pet", "id", red.GetId())["quality"] != "23" || len(red.GetPetEquipList()) != 4 {
			t.Fatal("original red pet/equip projection", red)
		}
		small := fresh("capacity", 34, 1)
		call(small, &pb.CSPetFirstRecruit{})
		deny(small, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_154"), Num: proto.Int32(2)}})
		var row db.Profile
		if err := db.GetIns().First(&row, small).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil || simpleOwned(p, g, "item_154") != 3 {
			t.Fatal("capacity partial deduction", err)
		}
	})
	t.Run("unknown-ordinary-pool-explicit-denial", func(t *testing.T) {
		id := fresh("unknown", 34, 100)
		deny(id, &pb.CSPetRecruit{Free: proto.Int32(0), NumType: proto.Int32(1)})
		deny(id, &pb.CSPetRecruit{Free: proto.Int32(1), NumType: proto.Int32(2)})
	})
}
