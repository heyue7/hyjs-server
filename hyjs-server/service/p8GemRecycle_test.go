package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestP8GemRecycle(t *testing.T) {
	ctx := context.Background()
	schema := os.Getenv("HYJS_P8_MYSQL_DATABASE")
	var err error
	if schema != "" {
		if !strings.HasPrefix(schema, "hyjs_test_p8_") {
			t.Fatal("isolated schema required")
		}
		err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
	} else {
		err = db.InitGame(ctx, filepath.Join(t.TempDir(), "gem-recycle.db"))
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err = load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitGems, s.InitCooking, s.InitEquipmentMutations} {
		if err = init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := int64(9710000)
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
	fresh := func(name string) (int64, []*pb.Equipment) {
		t.Helper()
		account, err := LoginAccount(ctx, "p8-gem-recycle-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, account.Id, "原宝石回收", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(20)
			if _, err := s.GrantFixedReward(p, g, "coin,1000000;diamond,100;equip_037,2"); err != nil {
				return err
			}
			// Finite isolated original gems exercise physical movement/ownership;
			// no mining source restoration is claimed by this recycle scenario.
			g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_019"), Num: proto.Int32(1)}, {Id: proto.String("MineItem_020"), Num: proto.Int32(1)}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		_, g := state(account.Id)
		return account.Id, g.Equipment
	}
	count := func(id int64, key string) int32 {
		_, g := state(id)
		for _, v := range g.Gems {
			if v.GetId() == key {
				return v.GetNum()
			}
		}
		return 0
	}
	mount := func(id int64, e *pb.Equipment) {
		t.Helper()
		stamp++
		if _, err := s.gemMount(ctx, id, &pb.CSGemMount{Guid: e.Guid, GemId: proto.String("MineItem_019"), Pos: proto.Int32(0)}, stamp); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("tracked-strengthening-swap-reset-returns-only-current-physical-gem", func(t *testing.T) {
		id, equips := fresh("tracked")
		equip := equips[0]
		mount(id, equip)
		stamp++
		strength, err := s.p5StrengthenOne(ctx, id, &pb.CSEquipStrengthenOneKey{Guid: equip.Guid}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if strength[0].(*pb.SCEquipStrengthenOneKey).GetEquip().GetLevel() < 2 {
			t.Fatal("actual strengthening source")
		}
		p, g := state(id)
		beforeResetCoin := p.GetCoin()
		tracked, _ := s.equipmentRecord(g, equip.GetGuid())
		ledger, err := p5InvestmentState(g, equip.GetGuid(), tracked)
		if err != nil {
			t.Fatal(err)
		}
		spent := ledger.Costs[0].GetNum()
		stamp++
		if _, err = s.gemMountOrChange(ctx, id, &pb.CSGemMountOrChange{Guid: equip.Guid, GemId: proto.String("MineItem_020")}, stamp); err != nil {
			t.Fatal("authorized transfer broke investment", err)
		}
		if count(id, "MineItem_019") != 1 || count(id, "MineItem_020") != 0 {
			t.Fatal("swap actual gem ownership")
		}
		stamp++
		reset, err := s.p5EquipmentReset(ctx, id, &pb.CSEquipReBuild{Guid: equip.Guid}, stamp)
		if err != nil {
			t.Fatal("tracked reset with mounted current gem", err)
		}
		p, g = state(id)
		_, restoredErr := s.equipmentRecord(g, equip.GetGuid())
		if p.GetCoin() != beforeResetCoin+int64(spent) || p.GetDiamond() != 50 || restoredErr == nil {
			t.Fatal("exact reset resources and emptied original slots")
		}
		if count(id, "MineItem_019") != 1 || count(id, "MineItem_020") != 1 {
			t.Fatal("baseline gem duplicated or current gem lost")
		}
		if len(reset[0].(*pb.SCEquipReBuild).GetGet().GetEquips()) != 0 {
			t.Fatal("native reset exposes old gem slot")
		}
		// The original reset receipt returns the same movement, never another gem.
		if _, err = s.p5EquipmentReset(ctx, id, &pb.CSEquipReBuild{Guid: equip.Guid}, stamp); err != nil {
			t.Fatal(err)
		}
		if count(id, "MineItem_020") != 1 {
			t.Fatal("replayed physical gem duplicated")
		}
	})
	t.Run("basic-mount-six-concurrent-decomposes-return-gem-before-owned-instance-deletion", func(t *testing.T) {
		id, equips := fresh("basic")
		equip := equips[0]
		mount(id, equip)
		stamp++
		fixed := stamp
		request := &pb.CSEquipDecompose{Guid: []string{equip.GetGuid()}}
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := s.p5EquipmentDecompose(ctx, id, request, fixed); errs <- err }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if count(id, "MineItem_019") != 1 {
			t.Fatal("basic decomposition lost or duplicated mounted gem")
		}
		_, g := state(id)
		if _, err = s.equipmentRecord(g, equip.GetGuid()); err == nil {
			t.Fatal("decomposed exact instance survived")
		}
		if len(g.Equipment) != 1 || g.Equipment[0].GetGuid() != equips[1].GetGuid() {
			t.Fatal("other same-ID owned equipment altered")
		}
	})
	t.Run("batch-invalid-second-colour-and-inventory-overflow-roll-back-all-gem-movements", func(t *testing.T) {
		id, equips := fresh("rollback")
		mount(id, equips[0])
		if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
			e, _ := s.equipmentRecord(g, equips[1].GetGuid())
			e.Gems = []string{"", "MineItem_020"}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		p, g := state(id)
		before, _ := json.Marshal(g)
		old := proto.Clone(p)
		stamp++
		if _, err = s.p5EquipmentDecompose(ctx, id, &pb.CSEquipDecompose{Guid: []string{equips[0].GetGuid(), equips[1].GetGuid()}}, stamp); err == nil {
			t.Fatal("invalid batch gem colour accepted")
		}
		p, g = state(id)
		after, _ := json.Marshal(g)
		if string(before) != string(after) || !proto.Equal(old, p) {
			t.Fatal("failed second item lost first gem/instance")
		}
		if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
			g.Gems = append(g.Gems, &pb.SimpleItem{Id: proto.String("MineItem_019"), Num: proto.Int32(math.MaxInt32)})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		p, g = state(id)
		before, _ = json.Marshal(g)
		old = proto.Clone(p)
		stamp++
		if _, err = s.p5EquipmentDecompose(ctx, id, &pb.CSEquipDecompose{Guid: []string{equips[0].GetGuid()}}, stamp); err == nil {
			t.Fatal("overflowing gem refund accepted")
		}
		p, g = state(id)
		after, _ = json.Marshal(g)
		if string(before) != string(after) || !proto.Equal(old, p) {
			t.Fatal("overflow lost gem/instance")
		}
	})
}
