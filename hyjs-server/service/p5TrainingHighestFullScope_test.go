package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The full training matrix uses the existing explicitly local random policy and
// original per-ID level100 limits. Finite prerequisites do not certify sources.
func TestP5OriginalTrainingHighestFullScope(t *testing.T) {
	if os.Getenv("HYJS_P5_TRAINING_FULL_SCOPE") != "1" {
		t.Skip("explicit all76 actual highest training matrix")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "training-highest.db")
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
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	requestCtx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, b)
	}
	receipts := map[int64]*pb.CSWarriorReset{}
	count, draws := 0, 0
	var actors []map[string]any
	for _, rule := range s.Tables["HeroTrainingMaxMin"] {
		if rule["warLv"] != "100" {
			continue
		}
		if selected := os.Getenv("HYJS_P5_TRAINING_HERO_ID"); selected != "" && selected != rule["warId"] {
			continue
		}
		hero := s.table("Hero", "id", rule["warId"])
		t.Run(rule["warId"], func(t *testing.T) {
			account, err := LoginAccount(ctx, "training-highest-"+rule["warId"], "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "原最高训练", "hero_043"); err != nil {
				t.Fatal(err)
			}
			var guid string
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				p.Diamond = proto.Int64(100000)
				p.WarriorBagSize = proto.Int32(1000)
				g.Formation = nil
				gain, e := s.GrantFixedReward(p, g, rule["warId"]+",1;item_007,100000")
				if e != nil {
					return e
				}
				guid = gain.GetWars()[0].GetGuid()
				w, e := p5Warrior(g, guid)
				if e != nil {
					return e
				}
				w.Level = proto.Int32(100)
				if hero["heroClass"] != "0" {
					p.HeroWarId = w.Guid
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			read := func() (*pb.Player, *model.GameState, *pb.Warrior) {
				t.Helper()
				var row db.Profile
				if e := db.GetIns().First(&row, account.Id).Error; e != nil {
					t.Fatal(e)
				}
				p, g, e := decodeProfile(&row)
				if e != nil {
					t.Fatal(e)
				}
				w, e := p5Warrior(g, guid)
				if e != nil {
					t.Fatal(e)
				}
				return p, g, w
			}
			initialP, initialG, baseline := read()
			limits, err := s.trainingLimits(initialP, baseline)
			if err != nil {
				t.Fatal(err)
			}
			for i, key := range []string{"hpMax", "attMax", "pdefMax", "mdefMax"} {
				v, e := strconv.Atoi(rule[key])
				if e != nil || limits[i] != int32(v) {
					t.Fatal("original level100 limit", key, e)
				}
			}
			slot := &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: baseline.Id, Guid: baseline.Guid, Pos: proto.Int32(0)}}}
			beforeActor, err := s.battleWarrior(initialP, baseline, initialG, slot)
			if err != nil {
				t.Fatal(err)
			}
			current := proto.Clone(baseline).(*pb.Warrior)
			stamp := int64(100)
			var train *pb.CSWarriorTraining
			var trainStamp int64
			iterations := 0
			for growthValues(current.GetTraining()) != limits && iterations < 60 {
				iterations++
				stamp++
				train = &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(3)}
				trainStamp = stamp
				out, e := s.warriorTraining(requestCtx(train), account.Id, train, stamp)
				if e != nil {
					t.Fatal("actual highest training", iterations, e)
				}
				r := out[0].(*pb.SCWarriorTraining)
				expected, e := p3Resources("item_007,30;diamond,50")
				if e != nil || !equipmentResourcesEqual(expected, r.GetCost()) {
					t.Fatal("original ten-step costs", e)
				}
				if !proto.Equal(r.GetGuid().GetTraining(), current.GetTraining()) {
					t.Fatal("preview applied before confirmation")
				}
				if r.GetGuid().GetTempTraining().GetState() == 0 {
					t.Fatal("positive original-limit local high training returned zero preview")
				}
				replay, e := s.warriorTraining(requestCtx(train), account.Id, train, stamp)
				if e != nil || !proto.Equal(out[0], replay[0]) {
					t.Fatal("current preview replay", e)
				}
				stamp++
				replace := &pb.CSWarriorTrainingReplace{Guid: proto.String(guid), Result: proto.Int32(1)}
				out, e = s.warriorTrainingReplace(requestCtx(replace), account.Id, replace, stamp)
				if e != nil {
					t.Fatal("actual confirmation", e)
				}
				current = out[0].(*pb.SCWarriorTrainingReplace).GetResult()
				draws += 10
			}
			if growthValues(current.GetTraining()) != limits {
				t.Fatal("highest original training unreachable under finite actual budget", iterations)
			}
			p, g, w := read()
			if simpleOwned(initialP, initialG, "item_007")-simpleOwned(p, g, "item_007") != int64(iterations*30) || initialP.GetDiamond()-p.GetDiamond() != int64(iterations*50) {
				t.Fatal("actual cumulative costs")
			}
			stamp++
			if _, err = s.warriorTraining(requestCtx(train), account.Id, train, stamp); err == nil {
				t.Fatal("max training charged")
			}
			maxActor, err := s.battleWarrior(p, w, g, slot)
			if err != nil {
				t.Fatal(err)
			}
			if maxActor.GetProp().GetHp()-beforeActor.GetProp().GetHp() != float64(limits[0]) || maxActor.GetProp().GetAtt()-beforeActor.GetProp().GetAtt() != float64(limits[1]) || maxActor.GetProp().GetPdef()-beforeActor.GetProp().GetPdef() != float64(limits[2]) || maxActor.GetProp().GetMdef()-beforeActor.GetProp().GetMdef() != float64(limits[3]) {
				t.Fatal("actual highest training battle effect", fmt.Sprint(limits))
			}
			actors = append(actors, map[string]any{"heroId": w.GetId(), "warrior": proto.Clone(w), "battleHero": proto.Clone(maxActor), "trainingMaximum": limits, "actualDraws": iterations * 10})
			reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
			if hero["heroClass"] == "0" {
				if _, err = s.p5WarriorReset(requestCtx(reset), account.Id, reset, 10000); err != nil {
					t.Fatal("highest training reset", err)
				}
				p, g, w = read()
				if !proto.Equal(w, baseline) || simpleOwned(p, g, "item_007") != simpleOwned(initialP, initialG, "item_007") || p.GetDiamond() != initialP.GetDiamond()-50 {
					t.Fatal("actual training exact refund")
				}
				if _, err = s.warriorTraining(requestCtx(train), account.Id, train, trainStamp); err == nil {
					t.Fatal("old highest preview survived reset")
				}
				receipts[account.Id] = reset
			} else if _, err = s.p5WarriorReset(requestCtx(reset), account.Id, reset, 10000); err == nil {
				t.Fatal("main training reset protection")
			}
		})
		count++
	}
	if os.Getenv("HYJS_P5_TRAINING_HERO_ID") == "" && count != 76 {
		t.Fatal("all original training identities", count)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	for id, request := range receipts {
		var before, after db.Profile
		if e := db.GetIns().First(&before, id).Error; e != nil {
			t.Fatal(e)
		}
		if _, e := s.p5WarriorReset(requestCtx(request), id, request, 10000); e != nil {
			t.Fatal(e)
		}
		if e := db.GetIns().First(&after, id).Error; e != nil || *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("cold highest training reset replay", e)
		}
	}
	if output := os.Getenv("HYJS_P5_TRAINING_ACTOR_OUTPUT"); output != "" {
		hashes := map[string]string{}
		for _, name := range []string{"Hero", "TalentAdvenced", "Talent", "HeroTrainingMaxMin", "HeroBreached", "Skill"} {
			data, e := readTestAnalysis75("../../analysis/tables/Table" + name + ".tsv")
			if e != nil {
				t.Fatal(e)
			}
			hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		raw, e := json.MarshalIndent(map[string]any{"date": "2026-10-14", "scope": "original individual level100 training maxima, actual existing local SQL random distribution; breach1/talent0/awake0", "characterPaths": actors, "actualDraws": draws, "originalTableSha256": hashes, "dynamicCombatCertified": false}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.MkdirAll(filepath.Dir(output), 0755); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(output, append(raw, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("actual paths=%d confirmed draws=%d, original individual maxima, local training distribution retained", count, draws)
}
