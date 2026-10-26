package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The new scenario earns first-map ingredients from actual frozen original
// gathering, crafts and consumes dishes, then exercises refund and reuse.
func TestP8CraftGrowth(t *testing.T) {
	base := context.Background()
	schema := os.Getenv("HYJS_P8_MYSQL_DATABASE")
	var err error
	if schema != "" {
		if !strings.HasPrefix(schema, "hyjs_test_p8_") {
			t.Fatal("isolated schema required")
		}
		err = db.InitGameMySQL(base, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
	} else {
		err = db.InitGame(base, filepath.Join(t.TempDir(), "craft-growth.db"))
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables} {
		if err = load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeGameplayPools, s.InitializeP2, s.InitializeP5, s.InitCooking, s.InitGems, s.InitializeP8Gather} {
		if err = init(base); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	contextFor := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(base, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
	}
	stamp := int64(9510000)
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
	fresh := func(name string) (int64, string) {
		t.Helper()
		account, err := LoginAccount(base, "p8-craft-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(base, account.Id, "料理回收隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		var guid string
		if err = updateProfile(base, account.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(20)
			gain, err := s.GrantFixedReward(p, g, "coin,1000000;diamond,200;hero_003,1;equip_037,1")
			if err == nil {
				guid = gain.GetWars()[0].GetGuid()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.fieldTransfer(base, account.Id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		if err = updateProfileWithTx(base, account.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			_, err := s.RefreshP8Resources(tx, p, g, time.Now(), true)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return account.Id, guid
	}
	cookingCount := func(id int64, key string) int32 {
		_, g := state(id)
		for _, v := range g.Cooking {
			if v.GetId() == key {
				return v.GetNum()
			}
		}
		return 0
	}
	eat := func(id int64, guid string, stamp int64) ([]proto.Message, error) {
		request := &pb.CSFishUsing{HeroGuid: proto.String(guid), CostItem: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(1)}}
		return s.fishUsing(contextFor(request), id, request, stamp)
	}
	t.Run("actual-first-map-fish-to-dishes-investment-half-refund-and-reuse", func(t *testing.T) {
		id, guid := fresh("natural")
		// Advance only disposable attempt ready_at and exhausted node refresh_at;
		// original weights/outputs remain frozen, fatigue is still actually paid.
		for i := 0; i < 100 && (cookingCount(id, "FishItem_007") < 15 || cookingCount(id, "FishItem_013") < 15 || cookingCount(id, "FishItem_019") < 15); i++ {
			rows, err := P8GatherNodes(db.GetIns(), id)
			if err != nil {
				t.Fatal(err)
			}
			var selected *db.P8ResourceNode
			for j := range rows {
				cfg, _ := p8DecodeConfig(rows[j].Snapshot)
				if cfg.Kind == "fish" && cfg.Level == 1 && rows[j].Remaining > 0 {
					selected = &rows[j]
					break
				}
			}
			if selected == nil {
				if err = db.GetIns().Model(&db.P8ResourceNode{}).Where("player_id=? AND remaining=0", id).Update("refresh_at", time.Now().UnixMilli()-1).Error; err != nil {
					t.Fatal(err)
				}
				if err = updateProfileWithTx(base, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
					_, err := s.RefreshP8Resources(tx, p, g, time.Now(), true)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				continue
			}
			cfg, _ := p8DecodeConfig(selected.Snapshot)
			if err = updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
				g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			stamp++
			started, err := s.p8StartGather(base, id, &pb.CSFieldResourceStartGather{PosId: proto.String(selected.PosID)}, stamp)
			if err != nil || started[0].(*pb.SCFieldResourceStartGather).GetResult() != 1 {
				t.Fatal("real fish start", err)
			}
			if err = db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", id).Update("ready_at", time.Now().UnixMilli()-1).Error; err != nil {
				t.Fatal(err)
			}
			stamp++
			claimed, err := s.p8ClaimGather(base, id, &pb.CSFieldGetResource{}, stamp)
			if err != nil || claimed[0].(*pb.SCFieldGetResource).GetCode() != 1 {
				t.Fatal("real fish source", err)
			}
		}
		for _, key := range []string{"FishItem_007", "FishItem_013", "FishItem_019"} {
			if cookingCount(id, key) < 15 {
				t.Fatal("finite original fish source did not supply recipe", key)
			}
		}
		request := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(3)}}
		stamp++
		made, err := s.fishCooking(base, id, request, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if cookingCount(id, "FishItem_029") != 3 || made[0].(*pb.SCFishCooking).GetAdd() != 75 {
			t.Fatal("actual crafted dishes")
		}
		stamp++
		if _, _, err = s.DispatchP5Next(base, id, &pb.CSFormationOn{FormatPos: proto.Int32(2), Id: proto.String("hero_003"), Guid: proto.String(guid)}, stamp); err != nil {
			t.Fatal(err)
		}
		beforeP, beforeG := state(id)
		before, err := s.battlePlayer(beforeP, beforeG)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			stamp++
			if _, err = eat(id, guid, stamp); err != nil {
				t.Fatal("real permanent use records investment", err)
			}
		}
		afterP, afterG := state(id)
		after, err := s.battlePlayer(afterP, afterG)
		if err != nil {
			t.Fatal(err)
		}
		var beforeAtt, afterAtt float64
		for _, h := range before.GetFormation() {
			if h.GetGuid() == guid {
				beforeAtt = h.GetProp().GetAtt()
			}
		}
		for _, h := range after.GetFormation() {
			if h.GetGuid() == guid {
				afterAtt = h.GetProp().GetAtt()
			}
		}
		if math.Abs(afterAtt-beforeAtt-300) > 1e-9 {
			t.Fatal("actual accumulated permanent att", beforeAtt, afterAtt)
		}
		power, err := s.formationFightPower(afterP, afterG)
		if err != nil || afterP.GetFightPower() != power {
			t.Fatal("stored power stale", err)
		}
		ledger, err := p5InvestmentState(afterG, guid, afterG.Warriors[1])
		if err != nil {
			t.Fatal(err)
		}
		if len(ledger.Costs) != 1 || ledger.Costs[0].GetId() != "FishItem_029" || ledger.Costs[0].GetNum() != 3 {
			t.Fatal("full actual consumed dish count")
		}
		if _, err = s.formationOff(base, id, &pb.CSFormationOff{FormatPos: proto.Int32(2), Guid: proto.String(guid), Id: proto.String("hero_003")}); err != nil {
			t.Fatal(err)
		}
		stamp++
		reset, err := s.p5WarriorReset(base, id, &pb.CSWarriorReset{Guid: proto.String(guid)}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if cookingCount(id, "FishItem_029") != 1 || reset[0].(*pb.SCWarriorReset).GetGet().GetSimps()[0].GetNum() != 1 {
			t.Fatal("original half accumulated dishes floor3/2")
		}
		_, g := state(id)
		if _, missing := p5Warrior(g, guid); missing == nil || p5KV(g, "p5.invest:"+guid) != "" {
			t.Fatal("partner GUID must retire and investment clear")
		}
		p, _ := state(id)
		if simpleOwned(p, g, "frag_hero_003") != 150 {
			t.Fatal("original partner fragments missing")
		}
		stamp++
		combined, combineErr := s.redCombine(base, id, &pb.CSCombine{TargetId: proto.String("hero_003")}, stamp)
		if combineErr != nil {
			t.Fatal("actual fragment recombination", combineErr)
		}
		guid = combined[0].(*pb.SCCombine).GetGet().GetWars()[0].GetGuid()
		stamp++
		if _, err = eat(id, guid, stamp); err != nil {
			t.Fatal("refunded actual dish not reusable", err)
		}
		stamp++
		reset, err = s.p5WarriorReset(base, id, &pb.CSWarriorReset{Guid: proto.String(guid)}, stamp)
		if err != nil {
			t.Fatal("one dish reset", err)
		}
		for _, r := range reset[0].(*pb.SCWarriorReset).GetGet().GetSimps() {
			if r.GetId() == "FishItem_029" {
				t.Fatal("one dish half integer boundary invented dish", r)
			}
		}
		if cookingCount(id, "FishItem_029") != 0 {
			t.Fatal("half refund invented a fraction")
		}
	})
	t.Run("permanent-food-concurrent-receipt-and-untracked-growth-rollback", func(t *testing.T) {
		id, guid := fresh("concurrent")
		if err = updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
			g.Cooking = []*pb.SimpleItem{{Id: proto.String("FishItem_029"), Num: proto.Int32(3)}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		stamp++
		fixed := stamp
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := eat(id, guid, fixed); errs <- err }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if cookingCount(id, "FishItem_029") != 2 {
			t.Fatal("concurrent dish duplicated")
		}
		if err = updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
			w, _ := p5Warrior(g, guid)
			w.Exp = proto.Int64(1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		p, g := state(id)
		raw, _ := json.Marshal(g)
		old := proto.Clone(p)
		stamp++
		if _, err = eat(id, guid, stamp); err == nil {
			t.Fatal("unknown growth should preserve investment guard")
		}
		p, g = state(id)
		now, _ := json.Marshal(g)
		if string(raw) != string(now) || !proto.Equal(old, p) {
			t.Fatal("failed investment changed dish/properties/profile")
		}
	})

	t.Run("original-gem-mount-and-unmount-store-real-party-power", func(t *testing.T) {
		id, _ := fresh("gems")
		p, g := state(id)
		equip := g.Equipment[0]
		if _, err = s.formationOn(base, id, &pb.CSFormationOn{FormatPos: proto.Int32(1), Id: equip.EquipId, Guid: equip.Guid}); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
			// Finite isolated gem inventory validates the new power hook. This case
			// does not claim a new natural gem source; natural fish is exercised above.
			g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_019"), Num: proto.Int32(1)}}
			return s.P8RefreshCraftPower(p, g)
		}); err != nil {
			t.Fatal(err)
		}
		p, g = state(id)
		before := p.GetFightPower()
		stamp++
		if _, err = s.gemMount(base, id, &pb.CSGemMount{Guid: equip.Guid, GemId: proto.String("MineItem_019"), Pos: proto.Int32(0)}, stamp); err != nil {
			t.Fatal(err)
		}
		p, g = state(id)
		expected, err := s.formationFightPower(p, g)
		if err != nil || p.GetFightPower() != expected || p.GetFightPower() != before+120 {
			t.Fatal("mounted original200att not stored as120power", before, p.GetFightPower(), err)
		}
		stamp++
		if _, err = s.gemUnmount(base, id, &pb.CSGemUnmount{Guid: equip.Guid, Pos: proto.Int32(0)}, stamp); err != nil {
			t.Fatal(err)
		}
		p, g = state(id)
		if p.GetFightPower() != before || len(g.Gems) != 1 || g.Gems[0].GetNum() != 1 {
			t.Fatal("unmount did not restore original power and actual gem")
		}
	})
}
