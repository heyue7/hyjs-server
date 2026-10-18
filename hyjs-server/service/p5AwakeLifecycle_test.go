package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Talent investment must continue through both awakening stages and fusion.
// A single ledger records each actual cost and permits a verified full reset.
func TestP5OriginalAwakeFullLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "awake-lifecycle.db")
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
	for _, initialize := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeWarriorAwake, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	resetReceipts := map[int64]*pb.CSWarriorReset{}
	requestContext := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, b)
	}
	count := 0
	exported := []map[string]any{}
	for _, hero := range s.Tables["Hero"] {
		if hero["heroClass"] != "0" || hero["IsOpenAwake"] != "1" {
			continue
		}
		t.Run(hero["id"], func(t *testing.T) {
			account, err := LoginAccount(ctx, "p5-awake-lifecycle-"+hero["id"], "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "觉醒全生命周期", "hero_043"); err != nil {
				t.Fatal(err)
			}
			guid := ""
			resources := map[string]bool{"item_005": true, "item_312": true}
			for stage := int32(1); stage <= 2; stage++ {
				for _, part := range strings.Split(s.awakeTable("HeroAwakenCost", hero["id"], stage)["itemCost"], ";") {
					resources[strings.Split(part, ",")[0]] = true
				}
			}
			resourceIDs := make([]string, 0, len(resources))
			for resource := range resources {
				resourceIDs = append(resourceIDs, resource)
			}
			slices.Sort(resourceIDs)
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000)
				p.Diamond = proto.Int64(1000)
				p.WarriorBagSize = proto.Int32(1000)
				specs := []string{fmt.Sprintf("%s,8", hero["id"])}
				for _, resource := range resourceIDs {
					specs = append(specs, resource+",100000")
				}
				gain, err := s.GrantFixedReward(p, g, strings.Join(specs, ";"))
				if err != nil {
					return err
				}
				guid = gain.GetWars()[0].GetGuid()
				w, err := p5Warrior(g, guid)
				if err != nil {
					return err
				}
				w.Level = proto.Int32(100)
				return nil
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
				w, err := p5Warrior(g, guid)
				if err != nil {
					t.Fatal(err)
				}
				return p, g, w
			}
			originalPlayer, originalGame, originalWarrior := read()
			talent := &pb.CSWarriorTalent{Guid: proto.String(guid), Talent: proto.String("1"), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(10000)}, {Id: proto.String("item_005"), Num: proto.Int32(30)}}}
			if _, err = s.warriorTalent(requestContext(talent), account.Id, talent); err != nil {
				t.Fatal("initial talent", err)
			}
			awake := &pb.CSWarriorAwake{Guid: proto.String(guid)}
			for stage := int32(1); stage <= 2; stage++ {
				out, err := s.warriorAwake(requestContext(awake), account.Id, awake, int64(stage))
				if err != nil || out[0].(*pb.SCWarriorAwake).GetNewWarrior().GetAwake().GetStage() != stage {
					t.Fatal("tracked talent to awakening stage", stage, err)
				}
				pBefore, gBefore, _ := read()
				replay, err := s.warriorAwake(requestContext(awake), account.Id, awake, int64(stage))
				if err != nil || !proto.Equal(out[0], replay[0]) {
					t.Fatal("awakening receipt", err)
				}
				pAfter, gAfter, _ := read()
				if pBefore.GetCoin() != pAfter.GetCoin() || len(gBefore.Warriors) != len(gAfter.Warriors) {
					t.Fatal("awakening receipt duplicated consumption")
				}
			}
			var totalFuseExp int64
			for _, row := range s.Tables["HeroAwakenFuseLevel"] {
				if row["quality"] == hero["quality"] {
					exp, err := strconv.ParseInt(row["needExp"], 10, 64)
					if err != nil {
						t.Fatal(err)
					}
					totalFuseExp += exp
				}
			}
			// Each material has the exact original 500 experience. Consume the
			// full original path total and retain the configured highest100.
			fuse := &pb.CSAwakeFuse{Guid: proto.String(guid), Cost: []*pb.BaseResource{{Id: proto.String("item_312"), Num: proto.Int32(int32((totalFuseExp + 499) / 500))}}}
			out, err := s.warriorAwakeFuse(requestContext(fuse), account.Id, fuse, 3)
			if err != nil || out[0].(*pb.SCAwakeFuse).GetNewWarrior().GetAwake().GetFuseLevel() != 100 {
				t.Fatal("both awakening stages to original fusion100", err)
			}
			if _, err = s.warriorAwakeFuse(requestContext(fuse), account.Id, fuse, 4); err == nil {
				t.Fatal("fusion beyond original100")
			}
			if _, err = s.warriorAwake(requestContext(awake), account.Id, awake, 4); err == nil {
				t.Fatal("awakening beyond original2")
			}
			p, g, w := read()
			if _, err = p5InvestmentState(g, guid, w); err != nil {
				t.Fatal("fusion corrupted actual investment ledger", err)
			}
			actor, err := s.battleWarrior(p, w, g, &pb.FormationItem{FormatPos: proto.Int32(2), BattlePos: proto.Int32(2), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}})
			if err != nil || actor.GetAwakeStage() != 2 || actor.GetAwakenSkill() != hero["AwakeSkillId"]+",2" {
				t.Fatal("highest awakening battle projection", err)
			}
			order := &pb.CSSetSkillOrder{HeroGuid: proto.String(guid), SkillOrder: proto.String(hero["AwakeSkillId"] + "," + hero["SkillId2"] + "," + hero["SkillId1"])}
			if _, err = s.p5SkillOrder(requestContext(order), account.Id, order, 50); err != nil {
				t.Fatal("highest awakening original three-slot reorder", err)
			}
			if os.Getenv("HYJS_P5_AWAKE_BATTLE_ACTOR_OUTPUT") != "" {
				p, g, w = read()
				actor, err = s.battleWarrior(p, w, g, &pb.FormationItem{FormatPos: proto.Int32(2), BattlePos: proto.Int32(2), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}})
				if err != nil || actor.GetBreachedLevel() != w.GetBreached().GetLevel() {
					t.Fatal("highest awake actor original breach transport missing", err)
				}
				exported = append(exported, map[string]any{"heroId": hero["id"], "warrior": proto.Clone(w), "battleHero": actor, "awakeStage": w.GetAwake().GetStage(), "fuseLevel": w.GetAwake().GetFuseLevel()})
			}
			reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
			if _, err = s.p5WarriorReset(requestContext(reset), account.Id, reset, 5); err != nil {
				t.Fatal("highest full lifecycle reset", err)
			}
			p, g, w = read()
			expected := proto.Clone(originalWarrior).(*pb.Warrior)
			expected.SkillOrder = proto.String(hero["SkillId2"] + "," + hero["SkillId1"])
			if !proto.Equal(w, expected) || p.GetCoin() != originalPlayer.GetCoin() || len(g.Warriors) != len(originalGame.Warriors) {
				t.Fatal("reset lost target baseline, actual coin or consumed same-card instances")
			}
			if _, err := s.warriorAwake(requestContext(awake), account.Id, awake, 2); err == nil {
				t.Fatal("old awake2 client projection accepted after reset")
			}
			if _, err := s.warriorAwakeFuse(requestContext(fuse), account.Id, fuse, 3); err == nil {
				t.Fatal("old fuse100 client projection accepted after reset")
			}
			actor, err = s.battleWarrior(p, w, g, &pb.FormationItem{FormatPos: proto.Int32(2), BattlePos: proto.Int32(2), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}})
			if err != nil || actor.GetAwakeStage() != 0 || actor.GetAwakenSkill() != "" || actor.GetSkillOrder() != expected.GetSkillOrder()+","+hero["UltSkillId"] {
				t.Fatal("reset retained unavailable awakening in normal order", err)
			}
			for _, item := range resourceIDs {
				if simpleOwned(p, g, item) != simpleOwned(originalPlayer, originalGame, item) {
					t.Fatal("reset did not refund exact actual lifecycle resources", item)
				}
			}
			resetReceipts[account.Id] = reset
		})
		count++
	}
	if count != 15 {
		t.Fatal("original enabled awakening scope changed", count)
	}
	if output := os.Getenv("HYJS_P5_AWAKE_BATTLE_ACTOR_OUTPUT"); output != "" {
		hashes := map[string]string{}
		for _, name := range []string{"Hero", "TalentAdvenced", "HeroAwakenAdd", "HeroAwakenFuseProp", "HeroAwakenFuseLevel", "Skill"} {
			data, err := os.ReadFile(filepath.Join("../../analysis/tables", "Table"+name+".tsv"))
			if err != nil {
				t.Fatal(err)
			}
			hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		data, err := json.MarshalIndent(map[string]any{"originalTableSha256": hashes, "characterPaths": exported, "actualAwakeningMutations": 30, "preparation": "finite isolated original zero-stage reward instances, talent1 and actual awake1/2 then fuse100; level100 prerequisite", "dynamicCombatCertified": false}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	for playerID, reset := range resetReceipts {
		var before db.Profile
		if err := db.GetIns().First(&before, playerID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.p5WarriorReset(requestContext(reset), playerID, reset, 5); err != nil {
			t.Fatal("restart reset receipt missing", err)
		}
		var after db.Profile
		if err := db.GetIns().First(&after, playerID).Error; err != nil || *after.Game != *before.Game || *after.Player != *before.Player {
			t.Fatal("restart reset duplicated resources or changed target", err)
		}
	}
}
