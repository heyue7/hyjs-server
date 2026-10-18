package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9PetIntimacy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-intimacy.db")
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
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := int64(910000)
	fresh := func(name string) (int64, *pb.Pet) {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-food-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "原精灵喂食隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(34)
			_, err := s.GrantFixedReward(p, g, "item_128,100;item_129,100;item_133,100;item_138,100;item_124,100")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, _, err := s.DispatchP9Pets(ctx, row.Id, &pb.CSPetFirstRecruit{}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		return row.Id, out[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
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
		out, handled, err := s.DispatchP9PetIntimacy(ctx, id, native, at)
		if !handled {
			t.Fatalf("not handled %T", b)
		}
		if err != nil {
			return nil, err
		}
		return out[0], nil
	}
	t.Run("native-six-concurrent-food-proportions-ledger-and-reopen", func(t *testing.T) {
		id, pet := fresh("food")
		stamp++
		at := stamp
		b := &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(1)}}}
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
				t.Fatal("repeat altered food")
			}
		}
		eaten := first.(*pb.SCPetEat)
		if eaten.GetPet().GetEat().GetExp() != 20 || eaten.GetPet().GetEat().GetLevel() != 0 || len(eaten.GetCost()) != 1 || eaten.GetCost()[0].GetNum() != 1 {
			t.Fatalf("same nature %+v", eaten)
		}
		_, g, _ := state(id)
		var itemNum int32
		for _, item := range g.Items {
			if item.GetId() == "item_129" {
				itemNum = item.GetNum()
			}
		}
		if itemNum != 99 {
			t.Fatal("concurrent overcharge")
		}
		for _, pair := range []struct {
			id   string
			want int32
		}{{"item_128", 30}, {"item_133", 60}, {"item_138", 160}} {
			stamp++
			out, err := call(id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String(pair.id), Num: proto.Int32(1)}}}, stamp)
			if err != nil {
				t.Fatal(err)
			}
			if out.(*pb.SCPetEat).GetPet().GetEat().GetExp() != pair.want {
				t.Fatalf("food ratio %s", pair.id)
			}
		}
		var investment db.P9PetInvestment
		if err := db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil {
			t.Fatal(err)
		}
		var costs map[string]int64
		if err := json.Unmarshal(investment.Costs, &costs); err != nil || costs["item_129"] != 1 || costs["item_128"] != 1 || costs["item_138"] != 1 || costs["coin"] != 0 {
			t.Fatal("actual food ledger")
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		_, _, beforeRetry := state(id)
		if _, err := call(id, b, at); err == nil {
			t.Fatal("old20 intimacy projection replaced current160 after reopen")
		}
		_, _, afterRetry := state(id)
		if *beforeRetry.Player != *afterRetry.Player || *beforeRetry.Game != *afterRetry.Game {
			t.Fatal("expired food receipt changed actual inventory")
		}
		stamp++
		shown, err := call(id, &pb.CSPetHudongShow{Guid: pet.Guid}, stamp)
		if err != nil || shown.(*pb.SCPetHudongShow).GetPet().GetEat().GetExp() != 160 {
			t.Fatalf("native owned food query %v", err)
		}
	})
	t.Run("random-unlock-overflow-material-owner-and-interaction-gates", func(t *testing.T) {
		id, pet := fresh("gates")
		_, _, prior := state(id)
		invalid := []proto.Message{&pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(32)}}}, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(1)}, {Id: proto.String("item_128"), Num: proto.Int32(1)}}}, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(2147483647)}}}, &pb.CSPetHudong{Guid: pet.Guid, Type: proto.Int32(1)}}
		for _, b := range invalid {
			stamp++
			if _, err := call(id, b, stamp); err == nil {
				t.Fatalf("unverified/invalid %T", b)
			}
		}
		_, _, after := state(id)
		if *prior.Player != *after.Player || *prior.Game != *after.Game {
			t.Fatal("rejected food changed inventory")
		}
		stamp++
		out, err := call(id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(31)}}}, stamp)
		if err != nil || out.(*pb.SCPetEat).GetPet().GetEat().GetExp() != 620 {
			t.Fatalf("confirmed pre-unlock food %v", err)
		}
		_, _, prior = state(id)
		stamp++
		if _, err = call(id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(1)}}}, stamp); err == nil {
			t.Fatal("unknown random unlock consumed food")
		}
		_, _, after = state(id)
		if *prior.Game != *after.Game {
			t.Fatal("unlock charged")
		}
		other, foreign := fresh("foreign")
		_ = other
		stamp++
		if _, err = call(id, &pb.CSPetHudongShow{Guid: foreign.Guid}, stamp); err == nil {
			t.Fatal("foreign owned query")
		}
	})
	t.Run("original-daily-pet-food-production-with-isolated-saturday-clock", func(t *testing.T) {
		if err := s.InitializeDailyCopy(ctx); err != nil {
			t.Fatal(err)
		}
		row, err := LoginAccount(ctx, "p9-food-natural-source", "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "精灵原副本食物", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(34); return nil }); err != nil {
			t.Fatal(err)
		}
		stamp++
		first, _, err := s.DispatchP9Pets(ctx, row.Id, &pb.CSPetFirstRecruit{}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		pet := first[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
		saturday := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
		for _, chapter := range []string{"31", "34"} {
			stamp++
			started, err := s.dailyCopyAt(ctx, row.Id, &pb.CSDailyCopyFightStart{ChapterId: proto.String(chapter)}, stamp, saturday)
			if err != nil {
				t.Fatal(err)
			}
			fight := started[0].(*pb.SCDailyCopyFightStart)
			stamp++
			won, err := s.dailyCopyAt(ctx, row.Id, dailyCopyWin(fight, chapter), stamp, saturday)
			if err != nil {
				t.Fatal(err)
			}
			gain := won[0].(*pb.SCDailyCopyFightEnd).GetGet().GetSimps()
			if len(gain) != 1 || gain[0].GetNum() != 6 {
				t.Fatalf("original food yield %+v", gain)
			}
		}
		stamp++
		fed, err := call(row.Id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_128"), Num: proto.Int32(6)}, {Id: proto.String("item_129"), Num: proto.Int32(6)}}}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if fed.(*pb.SCPetEat).GetPet().GetEat().GetExp() != 180 {
			t.Fatal("actual original daily foods not consumed")
		}
		p, g, _ := state(row.Id)
		if p.GetStamina() != 40 {
			t.Fatal("original two daily entries cost 60 stamina")
		}
		for _, item := range g.Items {
			if (item.GetId() == "item_128" || item.GetId() == "item_129") && item.GetNum() != 0 {
				t.Fatal("natural source duplicate")
			}
		}
	})

}
