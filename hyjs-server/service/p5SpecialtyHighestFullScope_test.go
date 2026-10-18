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

// Explicit 26460-operation matrix keeps normal fast unit runs bounded. It checks
// all49 eligible original IDs under the existing declared specialty unlock policy.
func TestP5OriginalSpecialtyHighestFullScope(t *testing.T) {
	if os.Getenv("HYJS_P5_SPECIALTY_FULL_SCOPE") != "1" {
		t.Skip("explicit all49 highest specialty matrix")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "specialty-highest.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks, s.InitSkillSpecialty} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	requestCtx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, b)
	}
	count := 0
	mutations := 0
	receipts := map[int64]*pb.CSWarriorReset{}
	for _, hero := range s.Tables["Hero"] {
		if hero["quality"] != "20" && hero["quality"] != "23" {
			continue
		}
		if hero["heroClass"] != "0" && hero["heroClass"] != "100" && hero["heroClass"] != "101" {
			continue
		}
		if selected := os.Getenv("HYJS_P5_SPECIALTY_HERO_ID"); selected != "" && selected != hero["id"] {
			continue
		}
		t.Run(hero["id"], func(t *testing.T) {
			account, err := LoginAccount(ctx, "specialty-highest-"+hero["id"], "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "原专精最高成长", "hero_043"); err != nil {
				t.Fatal(err)
			}
			guid := hero["id"] + "-specialty-target"
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				p.Diamond = proto.Int64(1000)
				p.WarriorBagSize = proto.Int32(1000)
				g.Formation = nil
				g.Warriors = []*pb.Warrior{{Id: proto.String(hero["id"]), Guid: proto.String(guid), Level: proto.Int32(100), Talent: proto.Int32(0), Training: &pb.Training{}, TempTraining: &pb.Training{}, Breached: &pb.Breached{Level: proto.Int32(1)}, Awake: &pb.Awake{}}}
				if hero["heroClass"] != "0" {
					p.HeroWarId = proto.String(guid)
				}
				specs := []string{}
				for number := 439; number <= 454; number++ {
					specs = append(specs, fmt.Sprintf("item_%03d,100000", number))
				}
				_, err := s.GrantFixedReward(p, g, strings.Join(specs, ";"))
				return err
			}); err != nil {
				t.Fatal(err)
			}
			read := func() (*pb.Player, *model.GameState, *pb.Warrior) {
				t.Helper()
				var row db.Profile
				if err := db.GetIns().First(&row, account.Id).Error; err != nil {
					t.Fatal(err)
				}
				p, g, err := decodeProfile(&row)
				if err != nil {
					t.Fatal(err)
				}
				w, err := p5Warrior(g, guid)
				if err != nil {
					t.Fatal(err)
				}
				return p, g, w
			}
			initialP, initialG, baseline := read()
			current := proto.Clone(baseline).(*pb.Warrior)
			spent := map[string]int64{}
			stamp := int64(100)
			var lastPaidRequest proto.Message
			var lastPaidStamp int64
			for kind := int32(1); kind <= 5; kind++ {
				level, strength := int32(1), int32(1)
				if kind >= 4 {
					present := false
					for _, entry := range current.GetSkillRefine() {
						if entry.GetType() == kind {
							level, strength = entry.GetLevel(), entry.GetStrengthLevel()
							present = true
						}
					}
					if !present {
						t.Fatal("original canOpen did not expose next specialty", kind)
					}
				}
				for level < 100 {
					next, err := s.specialtyTable("SkillSpecialty", "level", kind, level+1)
					if err != nil {
						t.Fatal(err)
					}
					need, _ := strconv.Atoi(next["needStrengthLevel"])
					strengthening := strength < int32(need)
					var request proto.Message = &pb.CSUpRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(kind)}
					costSpec := next["levelUpCost"]
					if strengthening {
						request = &pb.CSStrengthRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(kind)}
						row, err := s.specialtyTable("SkillSpecialtyStrength", "strengthLevel", kind, strength+1)
						if err != nil {
							t.Fatal(err)
						}
						costSpec = row["strengthCost"]
					}
					stamp++
					out, err := s.skillSpecialtyUpgrade(requestCtx(request), account.Id, request, stamp)
					if err != nil {
						t.Fatal("actual specialty path", kind, level, strength, err)
					}
					lastPaidRequest, lastPaidStamp = request, stamp
					var entries []*pb.SkillRefine
					var costs []*pb.BaseResource
					if strengthening {
						r := out[0].(*pb.SCStrengthRoleSkillRefineLevel)
						entries, costs = r.GetSkillRefine(), r.GetCost()
						strength++
					} else {
						r := out[0].(*pb.SCUpRoleSkillRefineLevel)
						entries, costs = r.GetSkillRefine(), r.GetCost()
						level++
					}
					expected, err := p3Resources(costSpec)
					if err != nil || !equipmentResourcesEqual(expected, costs) {
						t.Fatal("exact original specialty cost", kind, level, strength, err)
					}
					for _, cost := range costs {
						spent[cost.GetId()] += int64(cost.GetNum())
					}
					current.SkillRefine = entries
					for _, entry := range entries {
						if entry.GetType() == kind && (entry.GetLevel() != level || entry.GetStrengthLevel() != strength) {
							t.Fatal("specialty persisted increment")
						}
					}
					mutations++
					if level == 100 {
						replay, err := s.skillSpecialtyUpgrade(requestCtx(request), account.Id, request, stamp)
						if err != nil || !proto.Equal(out[0], replay[0]) {
							t.Fatal("highest specialty receipt", err)
						}
					}
				}
				if strength != 10 {
					t.Fatal("highest100 missing highest strengthening10", kind, strength)
				}
				stamp++
				if _, err := s.skillSpecialtyUpgrade(ctx, account.Id, &pb.CSUpRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(kind)}, stamp); err == nil {
					t.Fatal("highest specialty level overflow")
				}
				stamp++
				if _, err := s.skillSpecialtyUpgrade(ctx, account.Id, &pb.CSStrengthRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(kind)}, stamp); err == nil {
					t.Fatal("highest specialty strengthening overflow")
				}
			}
			p, g, w := read()
			if len(w.GetSkillRefine()) != 5 {
				t.Fatal("highest all5 specialties missing")
			}
			for key, number := range spent {
				if simpleOwned(initialP, initialG, key)-simpleOwned(p, g, key) != number {
					t.Fatal("highest specialty inventory cost", key)
				}
			}
			if _, err := s.battleWarrior(p, w, g, &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}}); err != nil {
				t.Fatal("highest specialty battle projection", err)
			}
			reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
			if hero["heroClass"] == "0" {
				if _, err := s.p5WarriorReset(requestCtx(reset), account.Id, reset, 100000); err != nil {
					t.Fatal("highest all specialty actual reset", err)
				}
				p, g, w = read()
				if !proto.Equal(w, baseline) {
					t.Fatal("highest specialty baseline not restored")
				}
				if os.Getenv("HYJS_P5_SPECIALTY_CHECK_OLD_RECEIPT") == "1" {
					if _, err := s.skillSpecialtyUpgrade(requestCtx(lastPaidRequest), account.Id, lastPaidRequest, lastPaidStamp); err == nil {
						t.Fatal("old highest specialty receipt accepted after actual reset")
					}
				}
				for key := range spent {
					if simpleOwned(initialP, initialG, key) != simpleOwned(p, g, key) {
						t.Fatal("highest specialty exact actual refund", key)
					}
				}
				receipts[account.Id] = reset
			} else if _, err := s.p5WarriorReset(requestCtx(reset), account.Id, reset, 100000); err == nil {
				t.Fatal("original main protection lost")
			}
		})
		count++
	}
	if os.Getenv("HYJS_P5_SPECIALTY_HERO_ID") == "" && (count != 49 || mutations != 26460) {
		t.Fatal("original specialty eligible49 mutation scope", count, mutations)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	for id, request := range receipts {
		var before, after db.Profile
		if err := db.GetIns().First(&before, id).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.p5WarriorReset(requestCtx(request), id, request, 100000); err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().First(&after, id).Error; err != nil || *before.Game != *after.Game || *before.Player != *after.Player {
			t.Fatal("specialty reset replay duplicated costs", err)
		}
	}
	t.Logf("actual highest specialty paths=%d original upgrades=%d; five types100/10, exact costs and replay", count, mutations)
}
