package service

import (
	"context"
	"encoding/json"
	"math"
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

func TestP9GuardGrowth(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-next.db")
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
	open := func() {
		t.Helper()
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("isolated schema rejected")
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
	for _, initialize := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
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
	stamp := int64(410000)
	fresh := func(name string, level int32) (int64, *pb.Pet) {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-next-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "守护成长隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(level)
			_, err := s.GrantFixedReward(p, g, "coin,10000000;item_124,20000;item_126,5000;item_139,1000;item_144,1000;item_145,1000;hero_020,1")
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
		out, handled, err := s.DispatchP9PetNext(ctx, id, request, stamp)
		if !handled || err != nil {
			t.Fatalf("%T handled=%v err=%v", b, handled, err)
		}
		return out[0]
	}
	deny := func(id int64, b proto.Message) {
		t.Helper()
		_, _, before := state(id)
		stamp++
		if _, handled, err := s.DispatchP9PetNext(ctx, id, b, stamp); err == nil || !handled {
			t.Fatal("denial missing", b, err)
		}
		_, _, after := state(id)
		if *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("failure changed profile")
		}
	}
	battle := func(id int64) *pb.BattlePlayer {
		t.Helper()
		p, g, _ := state(id)
		out, err := s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	on := func(id int64, pet *pb.Pet, pos int32) *pb.SCFormationOn {
		return call(id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(pos)}).(*pb.SCFormationOn)
	}
	toLevel := func(id int64, pet *pb.Pet, level int32) *pb.Pet {
		t.Helper()
		current, err := p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil {
			t.Fatal(err)
		}
		gain := -current.GetExp()
		for i := current.GetLevel(); i < level; i++ {
			need, err := s.p9LevelExp(current, i)
			if err != nil {
				t.Fatal(err)
			}
			gain += need
		}
		if gain <= 0 {
			t.Fatal("finite material gain", gain)
		}
		costs := []*pb.BaseResource{}
		if gain/1000 > 0 {
			costs = append(costs, &pb.BaseResource{Id: proto.String("item_126"), Num: proto.Int32(int32(gain / 1000))})
		}
		if remaining := gain % 1000; remaining > 0 {
			costs = append(costs, &pb.BaseResource{Id: proto.String("item_124"), Num: proto.Int32(int32((remaining + 99) / 100))})
		}
		return call(id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: costs}).(*pb.SCPetLevelup).GetPet()
	}
	t.Run("original-pos7-bound-property-and-owner-guid", func(t *testing.T) {
		id, pet := fresh("guard", 34)
		before := battle(id).GetFormation()[0].GetProp().GetAtt()
		reply := on(id, pet, 1)
		if reply.GetFormationItem().GetThings()[1].GetPos() != 7 {
			t.Fatal(reply)
		}
		after := battle(id).GetFormation()[0]
		move, err := s.P9OwnMovePlayerInfo(ctx, id)
		if err != nil || move[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()[0].GetInfo().GetPetId() != pet.GetId() {
			t.Fatal("original follow petId", err)
		}
		if math.Abs(after.GetProp().GetAtt()-before-15) > 1e-8 {
			t.Fatal("original additive guardian", before, after)
		}
		deny(id, &pb.CSFormationOn{Id: proto.String("pet_003"), Guid: pet.Guid, FormatPos: proto.Int32(1)})
		deny(id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(2)})
		p, g, _ := state(id)
		var companion *pb.Warrior
		for _, w := range g.Warriors {
			if w.GetId() == "hero_020" {
				companion = w
			}
		}
		if companion == nil {
			t.Fatal("actual reward hero missing")
		}
		if _, err := s.formationOn(ctx, id, &pb.CSFormationOn{Id: companion.Id, Guid: companion.Guid, FormatPos: proto.Int32(2)}); err != nil {
			t.Fatal(err)
		}
		deny(id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(2)})
		if p.GetHeroWarId() != g.Warriors[0].GetGuid() {
			t.Fatal("original main identity changed")
		}
		call(id, &pb.CSFormationOff{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)})
		move, err = s.P9OwnMovePlayerInfo(ctx, id)
		if err != nil || move[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()[0].GetInfo().GetPetId() != "" {
			t.Fatal("unload follow projection", err)
		}
		if math.Abs(battle(id).GetFormation()[0].GetProp().GetAtt()-before) > 1e-8 {
			t.Fatal("unequip kept effect")
		}
	})
	t.Run("upgrade-native-cost-concurrent-exact-ledger-and-reopen", func(t *testing.T) {
		id, pet := fresh("upgrade", 34)
		on(id, pet, 1)
		before := battle(id).GetFormation()[0].GetProp().GetAtt()
		stamp++
		frozen := stamp
		request := &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_126"), Num: proto.Int32(2)}}}
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		replies := make(chan string, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, _, err := s.DispatchP9PetNext(ctx, id, request, frozen)
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
		for r := range replies {
			if first == "" {
				first = r
			}
			if r != first {
				t.Fatal("receipt diverged")
			}
		}
		grown, err := p9OwnedPet(db.GetIns(), id, pet.GetGuid())
		if err != nil || grown.GetLevel() != 2 || grown.GetExp() != 500 {
			t.Fatal(grown, err)
		}
		p, g, saved := state(id)
		if p.GetCoin() != 9998000 || simpleOwned(p, g, "item_126") != 4998 {
			t.Fatal("repeated costs", p)
		}
		if math.Abs(battle(id).GetFormation()[0].GetProp().GetAtt()-before-15) > 1e-8 {
			t.Fatal("grown battle property")
		}
		var investment db.P9PetInvestment
		if err = db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil {
			t.Fatal(err)
		}
		costs := map[string]int64{}
		if err = json.Unmarshal(investment.Costs, &costs); err != nil || costs["coin"] != 2000 || costs["item_126"] != 2 {
			t.Fatal(costs, err)
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		out, _, err := s.DispatchP9PetNext(ctx, id, request, frozen)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := proto.Marshal(out[0])
		if string(data) != first {
			t.Fatal("reopen receipt")
		}
		_, _, now := state(id)
		if *saved.Player != *now.Player || *saved.Game != *now.Game {
			t.Fatal("replay changed owned source")
		}
		deny(id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1)}}})
		deny(id, &pb.CSPetLevelup{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_126"), Num: proto.Int32(1000)}}})
	})
	t.Run("first-breach-native-empty-cost-cap-and-effect", func(t *testing.T) {
		id, pet := fresh("breach", 34)
		on(id, pet, 1)
		deny(id, &pb.CSPetBreached{Guid: pet.Guid})
		grown := toLevel(id, pet, 30)
		if grown.GetLevel() != 30 {
			t.Fatal(grown)
		}
		before := battle(id).GetFormation()[0].GetProp().GetAtt()
		out := call(id, &pb.CSPetBreached{Guid: pet.Guid}).(*pb.SCPetBreached)
		if out.GetPet().GetBreached().GetLevel() != 1 || len(out.GetCost()) != 2 {
			t.Fatal(out)
		}
		if math.Abs(battle(id).GetFormation()[0].GetProp().GetAtt()-before-223) > 1e-8 {
			t.Fatal("original breakthrough constant")
		}
		p, g, _ := state(id)
		if simpleOwned(p, g, "item_139") != 950 || simpleOwned(p, g, "item_144") != 985 {
			t.Fatal("original breakthrough material")
		}
		deny(id, &pb.CSPetBreached{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1)}}})
	})
	t.Run("same-id-donor-guid-guard-and-growth-protection", func(t *testing.T) {
		id, pet := fresh("donor", 60)
		on(id, pet, 1)
		pet = toLevel(id, pet, 30)
		pet = call(id, &pb.CSPetBreached{Guid: pet.Guid}).(*pb.SCPetBreached).GetPet()
		pet = toLevel(id, pet, 50)
		var donors []*pb.Pet
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			var e error
			donors, e = s.P9GrantPet(tx, p, pet.GetId(), 3)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		// Finite same-ID original instance preparation exercises donor protection;
		// full ordinary recruit source remains explicitly unsupported.
		_, g, _ := state(id)
		var companion *pb.Warrior
		for _, w := range g.Warriors {
			if w.GetId() == "hero_020" {
				companion = w
			}
		}
		if _, err := s.formationOn(ctx, id, &pb.CSFormationOn{Id: companion.Id, Guid: companion.Guid, FormatPos: proto.Int32(2)}); err != nil {
			t.Fatal(err)
		}
		on(id, donors[0], 2)
		toLevel(id, donors[1], 2)
		out := call(id, &pb.CSPetBreached{Guid: pet.Guid}).(*pb.SCPetBreached)
		var used string
		for _, cost := range out.GetCost() {
			if cost.GetGuid() != "" {
				used = cost.GetGuid()
			}
		}
		if used != donors[2].GetGuid() {
			t.Fatal("protected instance consumed", used)
		}
		for _, kept := range donors[:2] {
			if _, err := p9OwnedPet(db.GetIns(), id, kept.GetGuid()); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := p9OwnedPet(db.GetIns(), id, donors[2].GetGuid()); err == nil {
			t.Fatal("donor not consumed")
		}
		var investment db.P9PetInvestment
		if err := db.GetIns().First(&investment, "guid=?", pet.GetGuid()).Error; err != nil {
			t.Fatal(err)
		}
		stored := []*pb.Pet{}
		if err := json.Unmarshal(investment.Donors, &stored); err != nil || len(stored) != 1 || !proto.Equal(stored[0], donors[2]) {
			t.Fatal("donor provenance", err)
		}
	})
}
