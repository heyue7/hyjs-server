package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Completed paid paths survive a Go timeout in their isolated storage. Recover
// those actual highest responses and verify cold reset replay without rerolling.
func TestP5OriginalBreachCommittedRecovery(t *testing.T) {
	source := os.Getenv("HYJS_P5_BREACH_RECOVERY_DB")
	if source == "" {
		t.Skip("explicit original paid breach timeout recovery")
	}
	absolute, e := filepath.Abs(source)
	if e != nil || !strings.Contains(absolute, "/build/candidates/p5-training-breach-highest-20261014/artifacts/") {
		t.Fatal("dedicated breach backup guard")
	}
	raw, e := readTestAnalysis75(absolute)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "breach-recovery.db")
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if e = db.InitGame(ctx, path); e != nil {
		t.Fatal(e)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if e = load("../../analysis/tables"); e != nil {
			t.Fatal(e)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if e = init(ctx); e != nil {
			t.Fatal(e)
		}
	}
	s.P5Ready = true
	requestCtx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, b)
	}
	var rows []db.WarriorGrowthReceipt
	if e = db.GetIns().Where("kind = ?", "breach").Order("player_id").Find(&rows).Error; e != nil {
		t.Fatal(e)
	}
	var actors []map[string]any
	operations := int64(0)
	partners, mains := 0, 0
	for _, receipt := range rows {
		highest := &pb.SCWarriorBreached{}
		if e = proto.Unmarshal(receipt.Response, highest); e != nil {
			t.Fatal(e)
		}
		w := highest.GetGuid()
		if w.GetBreached().GetLevel() != 25 {
			continue
		}
		request := &pb.CSWarriorBreached{}
		if e = proto.Unmarshal(receipt.Request, request); e != nil || request.GetGuid() != w.GetGuid() || request.GetType() != 2 {
			t.Fatal("actual original ten-step receipt", e)
		}
		var before, after db.Profile
		if e = db.GetIns().First(&before, receipt.PlayerID).Error; e != nil {
			t.Fatal(e)
		}
		p, g, e := decodeProfile(&before)
		if e != nil {
			t.Fatal(e)
		}
		current, e := p5Warrior(g, w.GetGuid())
		if e != nil {
			t.Fatal(e)
		}
		hero := s.table("Hero", "id", w.GetId())
		var baseline *pb.Warrior
		spent := int64(0)
		reset := &pb.CSWarriorReset{Guid: w.Guid}
		if hero["heroClass"] == "0" {
			partners++
			var cached db.P5MutationReceipt
			if e = db.GetIns().Where("player_id = ? AND operation = ? AND timestamp = ?", receipt.PlayerID, "warrior-reset", 100000).First(&cached).Error; e != nil {
				t.Fatal("completed real highest reset receipt missing", e)
			}
			response := &pb.SCWarriorReset{}
			if e = proto.Unmarshal(cached.Response, response); e != nil {
				t.Fatal(e)
			}
			if len(response.GetWarriors()) != 1 || !proto.Equal(current, response.GetWarriors()[0]) || current.GetBreached().GetLevel() != 1 {
				t.Fatal("real completed refund baseline")
			}
			baseline = current
			for _, item := range response.GetGet().GetSimps() {
				if item.GetId() == "item_006" {
					spent += int64(item.GetNum())
				}
			}
			if spent <= 0 || simpleOwned(p, g, "item_006") != 2000000 || p.GetDiamond() != 950 {
				t.Fatal("actual previously paid complete path refunded exactly")
			}
			replay, e := s.p5WarriorReset(requestCtx(reset), receipt.PlayerID, reset, 100000)
			if e != nil || !proto.Equal(replay[0], response) {
				t.Fatal("cold completed original reset replay", e)
			}
			if _, e = s.warriorBreached(requestCtx(request), receipt.PlayerID, request, receipt.Timestamp); e == nil {
				t.Fatal("cold old highest client snapshot accepted after reset")
			}
		} else {
			mains++
			if !proto.Equal(current, w) {
				t.Fatal("actual committed main25 state changed")
			}
			investment, e := p5InvestmentState(g, w.GetGuid(), current)
			if e != nil {
				t.Fatal(e)
			}
			baseline = &pb.Warrior{}
			if e = json.Unmarshal(investment.Baseline, baseline); e != nil || baseline.GetBreached().GetLevel() != 1 {
				t.Fatal("real main zero breach prerequisite", e)
			}
			for _, item := range investment.Costs {
				if item.GetId() == "item_006" {
					spent += int64(item.GetNum())
				}
			}
			if spent <= 0 || simpleOwned(p, g, "item_006") != 2000000-spent {
				t.Fatal("actual main highest exact paid ledger")
			}
			replay, e := s.warriorBreached(requestCtx(request), receipt.PlayerID, request, receipt.Timestamp)
			if e != nil || !proto.Equal(replay[0], highest) {
				t.Fatal("cold main highest original growth replay", e)
			}
			if _, e = s.p5WarriorReset(requestCtx(reset), receipt.PlayerID, reset, 100000); e == nil {
				t.Fatal("main highest protected reset")
			}
		}
		slot := &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: w.Id, Guid: w.Guid, Pos: proto.Int32(0)}}}
		actor, e := s.battleWarrior(p, w, g, slot)
		if e != nil || actor.GetBreachedLevel() != 25 {
			t.Fatal("actual highest original transport", e)
		}
		baseActor, e := s.battleWarrior(p, baseline, g, slot)
		if e != nil || !proto.Equal(actor.GetProp(), baseActor.GetProp()) {
			t.Fatal("original empty terminal25 property", e)
		}
		if e = db.GetIns().First(&after, receipt.PlayerID).Error; e != nil || *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("cold recovery or failed stale/protected requests mutated actual storage", e)
		}
		calls := receipt.Timestamp - 100
		if calls <= 0 || calls > 6000 {
			t.Fatal("actual committed original request count")
		}
		operations += calls
		actors = append(actors, map[string]any{"heroId": w.GetId(), "warrior": proto.Clone(w), "battleHero": proto.Clone(actor), "actualTenStepRequests": calls, "actualItem006Cost": spent, "visitedLevels": 25, "terminalPropertyPercent": 0, "recoveredFromRealCommittedGrowthReceipt": true})
	}
	if len(actors) != 56 || partners != 48 || mains != 8 {
		t.Fatal("actual interrupted complete path scope", len(actors), partners, mains)
	}
	if output := os.Getenv("HYJS_P5_BREACH_RECOVERY_ACTOR_OUTPUT"); output != "" {
		hashes := map[string]string{}
		for _, name := range []string{"Hero", "TalentAdvenced", "Talent", "HeroBreached", "Skill"} {
			b, e := readTestAnalysis75("../../analysis/tables/Table" + name + ".tsv")
			if e != nil {
				t.Fatal(e)
			}
			hashes[name] = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		b, e := json.MarshalIndent(map[string]any{"scope": "real completed first56 original zero→25 paths retained after Go default timeout; remaining20 separately completed", "characterPaths": actors, "actualTenStepRequests": operations, "originalTableSha256": hashes, "sourceBackupSha256": fmt.Sprintf("%x", sha256.Sum256(raw)), "dynamicCombatCertified": false}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.MkdirAll(filepath.Dir(output), 0755); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(output, append(b, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("actual completed paths recovered=%d paid ten-step requests=%d;48 partner cold refunds and8 main protection", len(actors), operations)
}
