package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

// Every original trainable identity traverses breach1..25 by real original
// weighted experience draws. Original terminal25 has no property percent values.
func TestP5OriginalBreachHighestFullScope(t *testing.T) {
	if os.Getenv("HYJS_P5_BREACH_FULL_SCOPE") != "1" {
		t.Skip("explicit76 complete actual breach25 paths")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "breach-highest.db")
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
	highest := s.table("HeroBreached", "id", "25")
	if highest == nil || highest["Exp"] != "-1.0" || highest["AttAddPercent"] != "" || s.table("HeroBreached", "id", "26") != nil {
		t.Fatal("original terminal25 drift")
	}
	var actors []map[string]any
	receipts := map[int64]*pb.CSWarriorReset{}
	count, operations := 0, 0
	for _, trainingRule := range s.Tables["HeroTrainingMaxMin"] {
		if trainingRule["warLv"] != "100" {
			continue
		}
		heroID := trainingRule["warId"]
		if selected := os.Getenv("HYJS_P5_BREACH_HERO_ID"); selected != "" && !strings.Contains(","+selected+",", ","+heroID+",") {
			continue
		}
		hero := s.table("Hero", "id", heroID)
		t.Run(heroID, func(t *testing.T) {
			account, err := LoginAccount(ctx, "breach-highest-"+heroID, "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "原突破最高", "hero_043"); err != nil {
				t.Fatal(err)
			}
			var guid string
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				p.Diamond = proto.Int64(1000)
				p.WarriorBagSize = proto.Int32(1000)
				g.Formation = nil
				gain, e := s.GrantFixedReward(p, g, heroID+",1;item_006,2000000")
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
			slot := &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: baseline.Id, Guid: baseline.Guid, Pos: proto.Int32(0)}}}
			beforeActor, err := s.battleWarrior(initialP, baseline, initialG, slot)
			if err != nil {
				t.Fatal(err)
			}
			current := proto.Clone(baseline).(*pb.Warrior)
			spent := int64(0)
			stamp := int64(100)
			request := &pb.CSWarriorBreached{Guid: proto.String(guid), Type: proto.Int32(2)}
			visited := map[int32]bool{1: true}
			calls := 0
			for current.GetBreached().GetLevel() < 25 && calls < 6000 {
				calls++
				stamp++
				out, e := s.warriorBreached(requestCtx(request), account.Id, request, stamp)
				if e != nil {
					t.Fatal("actual original breach path", calls, e)
				}
				r := out[0].(*pb.SCWarriorBreached)
				old := current.GetBreached().GetLevel()
				current = r.GetGuid()
				now := current.GetBreached().GetLevel()
				if now < old || now > 25 || current.GetBreached().GetExp() < 0 {
					t.Fatal("breach level/state movement")
				}
				for level := old; level <= now; level++ {
					visited[level] = true
				}
				if len(r.GetCost()) != 1 || r.GetCost()[0].GetId() != "item_006" || r.GetCost()[0].GetNum() <= 0 {
					t.Fatal("original breach actual currency")
				}
				spent += int64(r.GetCost()[0].GetNum())
				if now == old {
					per, _ := strconv.Atoi(s.table("HeroBreached", "id", strconv.Itoa(int(old)))["Cost"][9:])
					if int64(r.GetCost()[0].GetNum()) != int64(per*10) {
						t.Fatal("same stage ten original costs")
					}
				}
				if now == 25 {
					replay, e := s.warriorBreached(requestCtx(request), account.Id, request, stamp)
					if e != nil || !proto.Equal(out[0], replay[0]) {
						t.Fatal("highest breach current replay", e)
					}
				}
				operations++
			}
			if current.GetBreached().GetLevel() != 25 || current.GetBreached().GetExp() != 0 || len(visited) != 25 {
				t.Fatal("full original stages1..25 did not finish", calls, len(visited))
			}
			p, g, w := read()
			if simpleOwned(initialP, initialG, "item_006")-simpleOwned(p, g, "item_006") != spent {
				t.Fatal("actual full path investment")
			}
			if _, e := s.warriorBreached(requestCtx(request), account.Id, request, stamp+1); e == nil {
				t.Fatal("original beyond25 accepted")
			}
			actor, e := s.battleWarrior(p, w, g, slot)
			if e != nil || actor.GetBreachedLevel() != 25 || !proto.Equal(actor.GetProp(), beforeActor.GetProp()) {
				t.Fatal("original terminal25 empty property or transport changed", e)
			}
			actors = append(actors, map[string]any{"heroId": heroID, "warrior": proto.Clone(w), "battleHero": proto.Clone(actor), "actualTenStepRequests": calls, "actualItem006Cost": spent, "visitedLevels": 25, "terminalPropertyPercent": 0})
			reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
			if hero["heroClass"] == "0" {
				if _, e := s.p5WarriorReset(requestCtx(reset), account.Id, reset, 100000); e != nil {
					t.Fatal("highest breach reset", e)
				}
				p, g, w = read()
				if !proto.Equal(w, baseline) || simpleOwned(p, g, "item_006") != simpleOwned(initialP, initialG, "item_006") {
					t.Fatal("highest breach exact refund")
				}
				if _, e := s.warriorBreached(requestCtx(request), account.Id, request, stamp); e == nil {
					t.Fatal("old breach25 client projection after reset")
				}
				receipts[account.Id] = reset
			} else if _, e := s.p5WarriorReset(requestCtx(reset), account.Id, reset, 100000); e == nil {
				t.Fatal("main breach reset protected")
			}
		})
		count++
	}
	if os.Getenv("HYJS_P5_BREACH_HERO_ID") == "" && count != 76 {
		t.Fatal("original full76 identities", count)
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
		if _, e := s.p5WarriorReset(requestCtx(request), id, request, 100000); e != nil {
			t.Fatal(e)
		}
		if e := db.GetIns().First(&after, id).Error; e != nil || *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("cold breach reset replay", e)
		}
	}
	if output := os.Getenv("HYJS_P5_BREACH_ACTOR_OUTPUT"); output != "" {
		hashes := map[string]string{}
		for _, name := range []string{"Hero", "TalentAdvenced", "Talent", "HeroBreached", "Skill"} {
			raw, e := os.ReadFile("../../analysis/tables/Table" + name + ".tsv")
			if e != nil {
				t.Fatal(e)
			}
			hashes[name] = fmt.Sprintf("%x", sha256.Sum256(raw))
		}
		raw, e := json.MarshalIndent(map[string]any{"scope": "actual breach1..25 all original configured paths; training0/talent0/awake0/level100", "characterPaths": actors, "actualTenStepRequests": operations, "originalTableSha256": hashes, "randomPolicy": "original weighted gains/probabilities; declared local criticalMultiplier2 and resetSeconds0", "dynamicCombatCertified": false}, "", "  ")
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
	t.Logf("actual original highest paths=%d real ten-step requests=%d; terminal25 zero property percent retained", count, operations)
}
