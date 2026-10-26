package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original equipment matrix uses actual finite coins/stones/star resources.
// Both original quoted five-level and one-key paths reach260; refinement reaches80.
func TestP5OriginalEquipmentHighestFullScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "equipment-highest.db")
	open := func() {
		t.Helper()
		var err error
		if schema := os.Getenv("HYJS_P5_TALENT_MYSQL_DATABASE"); schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p5_talent_") {
				t.Fatal("dedicated full growth schema required")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		} else {
			err = db.InitGame(ctx, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitEquipmentMutations, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	requestCtx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, b)
	}
	receipts := map[int64]*pb.CSEquipReBuild{}
	if len(s.Tables["Equip"]) != 36 {
		t.Fatal("original equipment count drift")
	}
	for _, config := range s.Tables["Equip"] {
		t.Run(config["id"], func(t *testing.T) {
			account, err := LoginAccount(ctx, "p5-equipment-highest-"+config["id"], "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "原装备最高成长", "hero_043"); err != nil {
				t.Fatal(err)
			}
			var guid string
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				p.Diamond = proto.Int64(1000)
				p.EquipBagSize = proto.Int32(1000)
				gain, err := s.GrantFixedReward(p, g, config["id"]+",1;item_008,100000;item_459,100000;item_460,100000;item_461,100000;item_462,100000;item_463,100000;item_464,100000")
				if err == nil {
					guid = gain.GetEquips()[0].GetGuid()
				}
				if err == nil && tableNumber(config, "quality") >= 20 {
					_, err = s.GrantFixedReward(p, g, "frag_"+config["id"]+",100000")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			read := func() (*pb.Player, *model.GameState, *pb.Equipment) {
				t.Helper()
				var row db.Profile
				if err := db.GetIns().First(&row, account.Id).Error; err != nil {
					t.Fatal(err)
				}
				p, g, err := decodeProfile(&row)
				if err != nil {
					t.Fatal(err)
				}
				e, err := s.equipmentRecord(g, guid)
				if err != nil {
					t.Fatal(err)
				}
				return p, g, e
			}
			initialP, initialG, _ := read()
			var totalCoin int64
			for level := 1; level < 260; level++ {
				for _, r := range s.Tables["EquipExp"] {
					if r["quality"] == config["quality"] && r["level"] == strconv.Itoa(level) {
						cost, err := strconv.ParseInt(r["coin"], 10, 64)
						if err != nil {
							t.Fatal(err)
						}
						totalCoin += cost
					}
				}
			}
			quoted := int64(0)
			for level := 1; level <= 5; level++ {
				for _, r := range s.Tables["EquipExp"] {
					if r["quality"] == config["quality"] && r["level"] == strconv.Itoa(level) {
						v, _ := strconv.ParseInt(r["coin"], 10, 64)
						quoted += v
					}
				}
			}
			first := &pb.CSEquipStrengthen{Guid: proto.String(guid), Times: proto.Int32(5), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(quoted))}}}
			if _, err = s.equipmentStrengthen(requestCtx(first), account.Id, first); err != nil {
				t.Fatal("original quoted5", err)
			}
			key := &pb.CSEquipStrengthenOneKey{Guid: proto.String(guid)}
			result, err := s.p5StrengthenOne(requestCtx(key), account.Id, key, 10)
			if err != nil || result[0].(*pb.SCEquipStrengthenOneKey).GetEquip().GetLevel() != 260 {
				t.Fatal("original one-key highest260", err)
			}
			p, g, e := read()
			if initialP.GetCoin()-p.GetCoin() != totalCoin {
				t.Fatal("highest260 exact original cost")
			}
			if _, err = s.p5StrengthenOne(requestCtx(key), account.Id, key, 11); err == nil {
				t.Fatal("highest one-key overflow accepted")
			}
			levels, err := s.equipmentRefineLevels(e)
			if err != nil {
				t.Fatal(err)
			}
			var totalExp int64
			for _, exp := range levels {
				totalExp += exp
			}
			materials := int32((totalExp + 49) / 50)
			refine := &pb.CSEquipRefine{Guid: proto.String(guid), Cost: []*pb.BaseResource{{Id: proto.String("item_008"), Num: proto.Int32(materials)}}}
			if _, err = s.equipmentRefine(requestCtx(refine), account.Id, refine, 20); err != nil {
				t.Fatal("original highest80", err)
			}
			p, g, e = read()
			if e.GetRefineLv() != 80 || int64(e.GetRefineExp()) != int64(materials)*50-totalExp || simpleOwned(initialP, initialG, "item_008")-simpleOwned(p, g, "item_008") != int64(materials) {
				t.Fatal("highest80 original resource/remainder")
			}
			if _, err = s.equipmentRefine(requestCtx(refine), account.Id, refine, 21); err == nil {
				t.Fatal("highest refinement overflow accepted")
			}
			star := &pb.CSEquipStar{Guid: proto.String(guid)}
			if tableNumber(config, "quality") >= 20 {
				for level := int32(1); level <= 5; level++ {
					out, err := s.equipmentStar(requestCtx(star), account.Id, star, 30+int64(level))
					if err != nil || out[0].(*pb.SCEquipStar).GetEquip().GetStar() != level {
						t.Fatal("original highest5 stars", level, err)
					}
				}
				if _, err = s.equipmentStar(requestCtx(star), account.Id, star, 36); err == nil {
					t.Fatal("highest star overflow accepted")
				}
			} else if _, err = s.equipmentStar(requestCtx(star), account.Id, star, 31); err == nil {
				t.Fatal("original low-quality star gate bypass")
			}
			p, g, e = read()
			var lastForgeRequest *pb.CSEquipForge
			var lastForgeStamp int64
			var forgeCoins, failedForgeFragments int64
			if tableNumber(config, "quality") >= 20 {
				maximum := int32(8)
				if config["quality"] == "23" {
					maximum = 15
				}
				for attempt := int64(1); e.GetForgeLevel() < maximum && attempt <= 2000; attempt++ {
					costs, _, err := s.equipmentForgeCosts(e)
					if err != nil {
						t.Fatal(err)
					}
					request := &pb.CSEquipForge{Guid: e.Guid, Cost: costs}
					before := e.GetForgeLevel()
					out, err := s.equipmentForge(requestCtx(request), account.Id, request, 1000+attempt)
					if err != nil {
						t.Fatal("original forge highest path", config["id"], before, err)
					}
					result := out[0].(*pb.SCEquipForge)
					for _, cost := range costs {
						if cost.GetId() == "coin" {
							forgeCoins += int64(cost.GetNum())
						}
						if strings.HasPrefix(cost.GetId(), "frag_equip_") && result.GetResult() == 0 {
							failedForgeFragments += int64(cost.GetNum())
						}
					}
					lastForgeRequest, lastForgeStamp = request, 1000+attempt
					if result.GetEquip().GetForgeLevel() != before+result.GetResult() {
						t.Fatal("original forge success/failure level")
					}
					replay, err := s.equipmentForge(requestCtx(request), account.Id, request, 1000+attempt)
					if err != nil || !proto.Equal(out[0], replay[0]) {
						t.Fatal("original forge receipt", err)
					}
					p, g, e = read()
				}
				if e.GetForgeLevel() != maximum {
					t.Fatal("original forge finite attempt budget did not reach highest")
				}
				if _, _, err := s.equipmentForgeCosts(e); err == nil {
					t.Fatal("past original highest forge configured")
				}
			}
			slot := &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: initialG.Warriors[0].Id, Guid: initialG.Warriors[0].Guid, Pos: proto.Int32(0)}, {Id: e.EquipId, Guid: e.Guid, Pos: proto.Int32(int32(tableNumber(config, "type")))}}}
			actor, err := s.battleWarrior(p, g.Warriors[0], g, slot)
			if err != nil || actor.GetProp().GetHp() <= 0 {
				t.Fatal("highest real equipment battle projection", err)
			}
			reset := &pb.CSEquipReBuild{Guid: proto.String(guid)}
			if _, err = s.p5EquipmentReset(requestCtx(reset), account.Id, reset, 100); err != nil {
				t.Fatal("highest full investment reset", err)
			}
			var resetRow db.Profile
			if err := db.GetIns().First(&resetRow, account.Id).Error; err != nil {
				t.Fatal(err)
			}
			p, g, err = decodeProfile(&resetRow)
			if err != nil {
				t.Fatal(err)
			}
			if _, lookupErr := s.equipmentRecord(g, guid); lookupErr == nil || p.GetCoin() != initialP.GetCoin()-forgeCoins {
				t.Fatal("reset consumed GUID and exact refundable investment; forge fees remain spent")
			}
			if _, err := s.equipmentRefine(requestCtx(refine), account.Id, refine, 20); err == nil {
				t.Fatal("old refine80 client projection accepted after reset")
			}
			if _, err := s.p5StrengthenOne(requestCtx(key), account.Id, key, 10); err == nil {
				t.Fatal("old one-key260 client projection accepted after reset")
			}
			if lastForgeRequest != nil {
				if _, err := s.equipmentForge(requestCtx(lastForgeRequest), account.Id, lastForgeRequest, lastForgeStamp); err == nil {
					t.Fatal("old highest Forge client projection accepted after reset")
				}
			}
			if tableNumber(config, "quality") >= 20 {
				if _, err := s.equipmentStar(requestCtx(star), account.Id, star, 35); err == nil {
					t.Fatal("old star5 client projection accepted after reset")
				}
			}
			for _, resource := range []string{"item_008", "item_459", "item_460", "item_461", "item_462", "item_463", "item_464"} {
				if simpleOwned(p, g, resource) != simpleOwned(initialP, initialG, resource) {
					t.Fatal("highest exact original resource refund", resource)
				}
			}
			if tableNumber(config, "quality") >= 20 && simpleOwned(p, g, "frag_"+config["id"]) != simpleOwned(initialP, initialG, "frag_"+config["id"])+int64(tableNumber(s.table("Frag", "id", "frag_"+config["id"]), "num"))-failedForgeFragments {
				t.Fatal("highest original forge actual fragment costs not refunded")
			}
			receipts[account.Id] = reset
		})
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	for playerID, reset := range receipts {
		var before, after db.Profile
		if err := db.GetIns().First(&before, playerID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.p5EquipmentReset(requestCtx(reset), playerID, reset, 100); err != nil {
			t.Fatal("highest equipment reset restart", err)
		}
		if err := db.GetIns().First(&after, playerID).Error; err != nil || *before.Game != *after.Game || *before.Player != *after.Player {
			t.Fatal(fmt.Sprint("highest reset replay changed resources", err))
		}
	}
}
