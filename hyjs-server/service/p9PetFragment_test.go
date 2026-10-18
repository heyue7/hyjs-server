package service

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9PetFragments(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-fragments.db")
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
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
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeP9Pets, s.InitializeP9PetNext} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := int64(1010000)
	fresh := func(name, resources string) int64 {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-frags-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "原精灵碎片隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(34)
			_, err := s.GrantFixedReward(p, g, resources)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return row.Id
	}
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
	call := func(id int64, b proto.Message, at int64) (proto.Message, error) {
		raw, err := proto.Marshal(b)
		if err != nil {
			return nil, err
		}
		native := b.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, native); err != nil {
			return nil, err
		}
		out, handled, err := s.DispatchP9PetFragments(ctx, id, native, at)
		if !handled {
			t.Fatalf("not handled %T", b)
		}
		if err != nil {
			return nil, err
		}
		return out[0], nil
	}
	t.Run("original-50-cost-six-concurrent-unique-instance-and-reopen", func(t *testing.T) {
		id := fresh("combine", "frag_pet_002,100")
		stamp++
		at := stamp
		b := &pb.CSCombine{TargetId: proto.String("pet_002")}
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
		close(results)
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		var first proto.Message
		for out := range results {
			if first == nil {
				first = out
			} else if !proto.Equal(first, out) {
				t.Fatal("duplicate combine")
			}
		}
		combined := first.(*pb.SCCombine)
		if len(combined.GetGet().GetPets()) != 1 || combined.GetGet().GetPets()[0].GetGuid() == "" || combined.GetCost()[0].GetNum() != 50 {
			t.Fatalf("native result %+v", combined)
		}
		p, g, _ := state(id)
		_ = p
		var count int32
		for _, v := range g.Items {
			if v.GetId() == "frag_pet_002" {
				count = v.GetNum()
			}
		}
		if count != 50 {
			t.Fatal("repeated cost")
		}
		owned, err := P9PetList(db.GetIns(), id)
		if err != nil || len(owned) != 1 || !proto.Equal(owned[0], combined.GetGet().GetPets()[0]) {
			t.Fatal("real instance source")
		}
		var instance db.P9PetInstance
		if err = db.GetIns().First(&instance, "guid=?", owned[0].GetGuid()).Error; err != nil || !strings.Contains(instance.Source, "TableFrag.frag_pet_002=50") {
			t.Fatal("combine source rule")
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		retry, err := call(id, b, at)
		if err != nil || !proto.Equal(first, retry) {
			t.Fatalf("combine restart %v", err)
		}
	})
	t.Run("original-fragment-sale-coin-currency-and-atomic-capacity-gates", func(t *testing.T) {
		id := fresh("sale", "coin,100;frag_pet_002,55;frag_pet_009,20;frag_hero_001,10")
		stamp++
		out, err := call(id, &pb.CSPetFragSell{Sells: []*pb.BaseResource{{Id: proto.String("frag_pet_002"), Num: proto.Int32(5)}, {Id: proto.String("frag_pet_009"), Num: proto.Int32(3)}}}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		sold := out.(*pb.SCPetFragSell)
		if len(sold.GetGet().GetSimps()) != 1 || sold.GetGet().GetSimps()[0].GetId() != "coin" || sold.GetGet().GetSimps()[0].GetNum() != 16 {
			t.Fatal("pet fragment original preview uses coin")
		}
		p, _, prior := state(id)
		if p.GetCoin() != 116 || p.GetCoinpet() != 0 {
			t.Fatal("wrong currency")
		}
		for _, b := range []proto.Message{&pb.CSPetFragSell{Sells: []*pb.BaseResource{{Id: proto.String("frag_pet_002"), Num: proto.Int32(5)}, {Id: proto.String("frag_hero_001"), Num: proto.Int32(1)}}}, &pb.CSPetFragSell{Sells: []*pb.BaseResource{{Id: proto.String("frag_pet_002"), Num: proto.Int32(5)}, {Id: proto.String("frag_pet_002"), Num: proto.Int32(1)}}}, &pb.CSCombine{TargetId: proto.String("pet_999")}} {
			stamp++
			if _, err = call(id, b, stamp); err == nil {
				t.Fatal("invalid fragment accepted")
			}
		}
		_, _, after := state(id)
		if *prior.Game != *after.Game || *prior.Player != *after.Player {
			t.Fatal("mixed/invalid fragment partial mutation")
		}
		if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.PetBagSize = proto.Int32(0); return nil }); err != nil {
			t.Fatal(err)
		}
		_, _, prior = state(id)
		stamp++
		if _, err = call(id, &pb.CSCombine{TargetId: proto.String("pet_002")}, stamp); err == nil {
			t.Fatal("capacity bypass")
		}
		_, _, after = state(id)
		if *prior.Game != *after.Game {
			t.Fatal("full bag consumed fragments")
		}
		if _, handled, _ := s.DispatchP9PetFragments(ctx, id, &pb.CSCombine{TargetId: proto.String("trea_001")}, stamp); handled {
			t.Fatal("intercepted other original domain")
		}
	})
	t.Run("actual-original-item155-box-to-fragment-list-to-combine", func(t *testing.T) {
		id := fresh("actual-box", "item_155,1000")
		for i := 0; i < 10; i++ {
			stamp++
			if _, err := s.p5BoxUse(ctx, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_155"), Num: proto.Int32(100)}}, stamp); err != nil {
				t.Fatal(err)
			}
		}
		listed, err := s.treasureFragmentList(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		frags := listed[0].(*pb.SCFragmentList).GetFragPetList()
		sort.Slice(frags, func(i, j int) bool { return frags[i].GetId() < frags[j].GetId() })
		var target string
		var selected *pb.SimpleItem
		var sum int32
		for _, frag := range frags {
			sum += frag.GetNum()
			row := s.table("Frag", "id", frag.GetId())
			if selected == nil && int64(frag.GetNum()) >= int64(tableNumber(row, "num")) {
				target = row["target"]
				selected = frag
			}
		}
		if sum != 5000 || selected == nil {
			t.Fatal("original finite box weight or pigeonhole capacity")
		}
		stamp++
		out, err := call(id, &pb.CSCombine{TargetId: proto.String(target)}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		combined := out.(*pb.SCCombine)
		if combined.GetGet().GetPets()[0].GetId() != target || combined.GetCost()[0].GetId() != selected.GetId() {
			t.Fatal("actual source chain")
		}
	})
}
