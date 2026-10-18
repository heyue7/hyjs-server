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

// TestP5OriginalTalentFullScope executes every original activation against saved
// profiles. Test provisions are explicit; this does not certify natural sources,
// native dynamic combat, or phone coverage for the 76 original character IDs.
func TestP5OriginalTalentFullScope(t *testing.T) {
	ctx := context.Background()
	schema := os.Getenv("HYJS_P5_TALENT_MYSQL_DATABASE")
	path := filepath.Join(t.TempDir(), "talent-scope.db")
	open := func() {
		t.Helper()
		var err error
		if schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p5_talent_") {
				t.Fatal("dedicated talent test schema required")
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
	groups := map[string][]map[string]string{}
	for _, row := range s.Tables["TalentAdvenced"] {
		groups[row["warId"]] = append(groups[row["warId"]], row)
	}
	if len(groups) != 76 || len(s.Tables["TalentAdvenced"]) != 1003 {
		t.Fatal("original full-scope count drift", len(groups), len(s.Tables["TalentAdvenced"]))
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	read := func(id int64) (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().WithContext(ctx).First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	activated := 0
	for _, heroID := range keys {
		t.Run(heroID, func(t *testing.T) {
			account, err := LoginAccount(ctx, "p5-full-talent-"+heroID, "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "全量天赋", "hero_043"); err != nil {
				t.Fatal(err)
			}
			guid := heroID + "-target"
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(100000000)
				p.WarriorBagSize = proto.Int32(1000)
				p.NoviceGuideSchedule = proto.Int32(8)
				g.Formation = nil
				g.Warriors = []*pb.Warrior{{Id: proto.String(heroID), Guid: proto.String(guid), Level: proto.Int32(100), Talent: proto.Int32(0), Training: &pb.Training{}, TempTraining: &pb.Training{}, Breached: &pb.Breached{Level: proto.Int32(1)}, Awake: &pb.Awake{}}}
				if s.table("Hero", "id", heroID)["heroClass"] != "0" {
					p.HeroWarId = proto.String(guid)
				}
				_, err := s.GrantFixedReward(p, g, "item_005,100000;item_047,100000;item_470,100000")
				if err != nil {
					return err
				}
				// Material instances use the genuine reward constructor and exact
				// original cost ID, including heroine variants using hero_062.
				amounts := map[string]int{}
				for _, node := range groups[heroID] {
					for _, part := range strings.Split(node["cost"], ";") {
						pair := strings.Split(part, ",")
						if len(pair) == 2 && strings.HasPrefix(pair[0], "hero_") {
							num, _ := strconv.Atoi(pair[1])
							amounts[pair[0]] += num
						}
					}
				}
				for material, num := range amounts {
					if _, err := s.GrantFixedReward(p, g, fmt.Sprintf("%s,%d", material, num)); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var maxTrunk int
			for _, node := range groups[heroID] {
				if node["talentLevel"] == "0" {
					continue
				}
				p, g := read(account.Id)
				request := &pb.CSWarriorTalent{Guid: proto.String(guid), Talent: proto.String(node["talentLevel"])}
				for _, part := range strings.Split(node["cost"], ";") {
					pair := strings.Split(part, ",")
					num, _ := strconv.Atoi(pair[1])
					if strings.HasPrefix(pair[0], "hero_") {
						count := 0
						for _, donor := range g.Warriors {
							if donor.GetId() == pair[0] && donor.GetGuid() != guid && talentMaterialUnused(donor) && count < num {
								request.Cost = append(request.Cost, &pb.BaseResource{Id: proto.String(heroID), Guid: donor.Guid})
								count++
							}
						}
					} else {
						request.Cost = append(request.Cost, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(num))})
					}
				}
				requestCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
				out, err := s.warriorTalent(requestCtx, account.Id, request)
				if err != nil {
					t.Fatalf("original node=%s tier=%s cost=%s: %v", node["id"], node["talentLevel"], node["cost"], err)
				}
				updated := out[0].(*pb.SCWarriorTalent).GetGuid()
				if node["trunck"] == "1" {
					tier, _ := strconv.Atoi(node["talentLevel"])
					maxTrunk = max(maxTrunk, tier)
					if updated.GetTalent() != int32(tier) {
						t.Fatal("wrong trunk state")
					}
				} else if !slices.Contains(updated.GetTalentBranch(), node["talentLevel"]) {
					t.Fatal("branch not saved")
				}
				p2, g2 := read(account.Id)
				consumed := 0
				for _, cost := range out[0].(*pb.SCWarriorTalent).GetCost() {
					if cost.GetGuid() == "" {
						if simpleOwned(p, g, cost.GetId())-simpleOwned(p2, g2, cost.GetId()) != int64(cost.GetNum()) {
							t.Fatal("exact original resource cost mismatch", node["id"], cost.GetId())
						}
					} else {
						consumed++
						if _, err := p5Warrior(g2, cost.GetGuid()); err == nil {
							t.Fatal("consumed original GUID still owned", node["id"])
						}
					}
				}
				if len(g.Warriors)-len(g2.Warriors) != consumed {
					t.Fatal("material instance count mismatch", node["id"])
				}
				beforeGame, err := json.Marshal(g2)
				if err != nil {
					t.Fatal(err)
				}
				beforeCoin := p2.GetCoin()
				if _, err = s.warriorTalent(requestCtx, account.Id, request); err == nil {
					t.Fatal("duplicate activation accepted")
				}
				p3, g3 := read(account.Id)
				afterGame, err := json.Marshal(g3)
				if err != nil {
					t.Fatal(err)
				}
				if p3.GetCoin() != beforeCoin || string(beforeGame) != string(afterGame) {
					t.Fatal("failed duplicate changed full inventory or growth")
				}
				actor, err := s.battleWarrior(p3, updated, g3, &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: updated.Id, Guid: updated.Guid}}})
				if err != nil {
					t.Fatalf("original node=%s battle projection: %v", node["id"], err)
				}
				if actor.GetProp().GetHp() <= 0 || actor.GetProp().GetAtt() <= 0 || node["talentId"] != "" && !slices.Contains(actor.GetTalentList(), node["talentId"]) {
					t.Fatal("missing real attributes or activated talent", node["id"])
				}
				activated++
			}
			if _, err = s.warriorTalent(ctx, account.Id, &pb.CSWarriorTalent{Guid: proto.String(guid), Talent: proto.String(strconv.Itoa(maxTrunk + 1))}); err == nil {
				t.Fatal("highest trunk overflow accepted")
			}
		})
	}
	if activated != 927 {
		t.Fatal("not all original nonzero nodes activated", activated)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	exported := make([]map[string]any, 0, len(keys))
	for _, heroID := range keys {
		var row db.Profile
		if err := db.GetIns().Where("uid=?", "p5-full-talent-"+heroID).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		p, g := read(row.Id)
		w, err := p5Warrior(g, heroID+"-target")
		if err != nil || w.GetTalent() == 0 {
			t.Fatal("full-scope restart state missing", heroID, err)
		}
		if os.Getenv("HYJS_P5_BATTLE_ACTOR_OUTPUT") != "" {
			actor, err := s.battleWarrior(p, w, g, &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: w.Id, Guid: w.Guid}}})
			if err != nil {
				t.Fatal("highest persisted battle actor export", heroID, err)
			}
			if actor.GetBreachedLevel() != w.GetBreached().GetLevel() {
				t.Fatal("highest battle actor original breach transport missing", heroID)
			}
			nodes := []string{}
			for _, node := range groups[heroID] {
				if node["talentLevel"] != "0" {
					nodes = append(nodes, node["id"])
				}
			}
			exported = append(exported, map[string]any{"heroId": heroID, "warrior": w, "battleHero": actor, "nodeIds": nodes, "highestTrunk": w.GetTalent(), "branchIds": w.GetTalentBranch()})
		}
	}
	if output := os.Getenv("HYJS_P5_BATTLE_ACTOR_OUTPUT"); output != "" {
		hashes := map[string]string{}
		for _, name := range []string{"Hero", "TalentAdvenced", "Talent", "Skill"} {
			data, err := os.ReadFile(filepath.Join("../../analysis/tables", "Table"+name+".tsv"))
			if err != nil {
				t.Fatal(err)
			}
			hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		data, err := json.MarshalIndent(map[string]any{"originalTableSha256": hashes, "actualActivations": activated, "characterPaths": exported, "preparation": "finite isolated resources, original zero talent, real 927 activations then storage reopen; level100 prerequisite", "dynamicCombatCertified": false}, "", "  ")
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
	t.Logf("OriginalTalent full scope: 1003 rows, 76 zero baselines, %d actual activations, 76 highest paths persisted", activated)
}

// Original green partners have no talent rows or skills. Their original base
// properties and real equipment still project, while forged talent state fails.
func TestP5OriginalGreenPartnerProjection(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range s.Tables["Hero"] {
		if row["heroClass"] != "0" || row["quality"] != "12" {
			continue
		}
		p := &pb.Player{PlayerId: proto.Int64(77), HeroWarId: proto.String("other-main"), Level: proto.Int32(100), WarriorBagSize: proto.Int32(100), EquipBagSize: proto.Int32(100)}
		g := &model.GameState{}
		gain, err := s.GrantFixedReward(p, g, row["id"]+",1;equip_001,1")
		if err != nil {
			t.Fatal(err)
		}
		w := gain.GetWars()[0]
		slot := &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}}
		base, err := s.battleWarrior(p, w, g, slot)
		if err != nil {
			t.Fatal("green original base", row["id"], err)
		}
		if base.GetProp().GetHp() != tableNumber(row, "hpBase") || base.GetProp().GetAtt() != tableNumber(row, "attBase") || base.GetSkillOrder() != ",," || len(base.GetTalentList()) != 0 {
			t.Fatal("green properties or skill slots changed", row["id"])
		}
		w.Level = proto.Int32(100)
		grown, err := s.battleWarrior(p, w, g, slot)
		if err != nil || !proto.Equal(base.GetProp(), grown.GetProp()) {
			t.Fatal("green invented talent growth", row["id"], err)
		}
		equipment := gain.GetEquips()[0]
		slot.Things = append(slot.Things, &pb.FormationThing{Id: equipment.EquipId, Guid: equipment.Guid, Pos: proto.Int32(1)})
		mounted, err := s.battleWarrior(p, w, g, slot)
		if err != nil || mounted.GetProp().GetAtt() <= grown.GetProp().GetAtt() {
			t.Fatal("green real equipment properties missing", row["id"], err)
		}
		invalid := proto.Clone(w).(*pb.Warrior)
		invalid.Talent = proto.Int32(1)
		if _, err := s.battleWarrior(p, invalid, g, slot); err == nil {
			t.Fatal("green forged talent accepted", row["id"])
		}
		w.TalentBranch = []string{"4-1"}
		if _, err := s.battleWarrior(p, w, g, slot); err == nil {
			t.Fatal("green forged branch accepted", row["id"])
		}
		count++
	}
	if count != 6 {
		t.Fatal("original green scope drift", count)
	}
}
