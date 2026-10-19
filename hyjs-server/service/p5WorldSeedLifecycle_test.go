package service

import (
	"context"
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

// All 85 original nodes and both original main identities retain their real
// GUID and growth ledger across the original purple/orange identity changes.
func TestP5OriginalWorldSeedFullLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "worldseed-lifecycle.db")
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
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	if len(s.Tables["WorldSeed"]) != 85 {
		t.Fatal("original full worldseed count drift")
	}
	requestContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
	}
	for _, mainID := range []string{"hero_043", "hero_047"} {
		t.Run(mainID, func(t *testing.T) {
			account, err := LoginAccount(ctx, "p5-worldseed-"+mainID, "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "原世界之种", mainID); err != nil {
				t.Fatal(err)
			}
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(100000000)
				p.Diamond = proto.Int64(1000)
				_, err := s.GrantFixedReward(p, g, "item_005,100000;item_007,1000;item_017,100;equip_001,1")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			read := func() (*pb.Player, *model.GameState, *pb.Warrior) {
				t.Helper()
				var profile db.Profile
				if err := db.GetIns().First(&profile, account.Id).Error; err != nil {
					t.Fatal(err)
				}
				p, g, err := decodeProfile(&profile)
				if err != nil {
					t.Fatal(err)
				}
				w, err := p5Warrior(g, p.GetHeroWarId())
				if err != nil {
					t.Fatal(err)
				}
				return p, g, w
			}
			p, g, w := read()
			identityGUID := w.GetGuid()
			equip := g.Equipment[0]
			if _, err = s.formationOn(ctx, account.Id, &pb.CSFormationOn{Guid: equip.Guid, Id: equip.EquipId, FormatPos: proto.Int32(1)}); err != nil {
				t.Fatal(err)
			}
			activate := func(tier int32) {
				t.Helper()
				_, _, current := read()
				node := s.p9TablePair("TalentAdvenced", "warId", current.GetId(), "talentLevel", strconv.Itoa(int(tier)))
				request := &pb.CSWarriorTalent{Guid: current.Guid, Talent: proto.String(strconv.Itoa(int(tier)))}
				for _, part := range strings.Split(node["cost"], ";") {
					parts := strings.Split(part, ",")
					num, _ := strconv.Atoi(parts[1])
					request.Cost = append(request.Cost, &pb.BaseResource{Id: proto.String(parts[0]), Num: proto.Int32(int32(num))})
				}
				if _, err := s.warriorTalent(requestContext(request), account.Id, request); err != nil {
					t.Fatal("continued main talent after original quality transition", current.GetId(), tier, err)
				}
			}
			activate(1)
			stamp := int64(100)
			for _, node := range s.Tables["WorldSeed"] {
				nodeID := p5SeedID(node)
				if nodeID == 1 {
					continue
				}
				stamp++
				request := &pb.CSLightenWorldSeed{Id: proto.Int32(nodeID - 1)}
				out, err := s.lightenWorldSeed(requestContext(request), account.Id, request, stamp)
				if err != nil {
					t.Fatal("original worldseed node", nodeID, err)
				}
				replay, err := s.lightenWorldSeed(requestContext(request), account.Id, request, stamp)
				if err != nil || !proto.Equal(out[0], replay[0]) {
					t.Fatal("original node receipt", nodeID, err)
				}
				p, g, w = read()
				if p.GetHeroWarId() != identityGUID || w.GetGuid() != identityGUID || p.GetCurrentWorldSeedId() != nodeID {
					t.Fatal("identity or original worldseed node lost", nodeID)
				}
				if node["type"] == "3" {
					if _, err = p5InvestmentState(g, identityGUID, w); err != nil {
						t.Fatal("original quality transition broke tracked main growth", nodeID, err)
					}
					activate(w.GetTalent() + 1)
				}
			}
			p, g, w = read()
			if p.GetCurrentWorldSeedId() != 85 || simpleOwned(p, g, "item_017") != 16 || w.GetTalent() != 3 || s.table("Hero", "id", w.GetId())["quality"] != "20" {
				t.Fatal("original85 path, two original quality transitions or exact seed costs differ")
			}
			if g.Equipment[0].GetGuid() != equip.GetGuid() || g.Equipment[0].GetLastHeroId() != w.GetId() {
				t.Fatal("equipment ownership lost during original quality transitions")
			}
			if _, err = s.lightenWorldSeed(ctx, account.Id, &pb.CSLightenWorldSeed{Id: proto.Int32(85)}, stamp+1); err == nil {
				t.Fatal("past original worldseed85 accepted")
			}
			training := &pb.CSWarriorTraining{Guid: w.Guid, CostType: proto.Int32(3), NumType: proto.Int32(1)}
			if _, err = s.warriorTraining(requestContext(training), account.Id, training, stamp+2); err != nil {
				t.Fatal("continued main training after highest worldseed", err)
			}
			_, g, w = read()
			investment, err := p5InvestmentState(g, identityGUID, w)
			if err != nil {
				t.Fatal("final main training investment integrity", err)
			}
			for _, cost := range investment.Costs {
				if cost.GetId() == "item_017" {
					t.Fatal("global worldseed expense became refundable main investment")
				}
			}
			reset := &pb.CSWarriorReset{Guid: w.Guid}
			if _, err = s.p5WarriorReset(requestContext(reset), account.Id, reset, stamp+3); err == nil {
				t.Fatal("main identity protection lost")
			}
		})
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	for _, mainID := range []string{"hero_043", "hero_047"} {
		var profile db.Profile
		if err := db.GetIns().Where("uid=?", "p5-worldseed-"+mainID).First(&profile).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		w, err := p5Warrior(g, p.GetHeroWarId())
		if err != nil || p.GetCurrentWorldSeedId() != 85 {
			t.Fatal("highest original worldseed restart", err)
		}
		if _, err = p5InvestmentState(g, w.GetGuid(), w); err != nil {
			t.Fatal("highest transformed main investment restart", err)
		}
	}
}
