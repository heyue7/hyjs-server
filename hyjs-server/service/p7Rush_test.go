package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7Setup(t *testing.T) *p1Fixture {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p7.db")}
	if name := os.Getenv("HYJS_P7_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p7_") {
			t.Fatal("P7 requires isolated database")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P7_MYSQL_HOST"), User: os.Getenv("HYJS_P7_MYSQL_USER"), Password: os.Getenv("HYJS_P7_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP7Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, fn := range []func(context.Context) error{f.s.InitializeP1, f.s.InitializeP6, f.s.InitCooking, f.s.InitializeDailyCopy, f.s.InitializeP7} {
		if err := fn(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	// 同一专属MySQL库顺序跑多个夹具时，不继承前个夹具显式发布的Re策略。
	for _, table := range []any{&db.P7RushPolicy{}, &db.P7ReRushPolicy{}, &db.P7RushManualPolicy{}, &db.P7RushStageRule{}, &db.P7RushState{}, &db.P7RushProgress{}, &db.P7RushFight{}, &db.P7RushReceipt{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP7(f.ctx); err != nil {
		t.Fatal(err)
	}
	a, err := LoginAccount(f.ctx, "p7-"+t.Name()+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, a.Id, "试炼门禁回归", "hero_047"); err != nil {
		t.Fatal(err)
	}
	f.id = a.Id
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(22)
		p.Diamond = proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d, _ := db.GetIns().DB(); d.Close() })
	return f
}

// p7DiagnosticStage 使用当前原Hero完整技能与基础属性作隔离计算样本，不冒充恢复了原普通楼层。
func p7DiagnosticStage(t *testing.T, f *p1Fixture) db.P7RushStageRule {
	t.Helper()
	actor, err := f.s.actor(f.s.table("Hero", "id", "hero_047"), "p7-original-enemy", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	monster := &pb.BattleMonster{Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}, Formation: []*pb.BattleHero{actor}}
	raw, err := proto.Marshal(monster)
	if err != nil {
		t.Fatal(err)
	}
	cfg := db.P7RushStageRule{ID: "isolated:trial:1", Kind: 1, TaskID: "diagnostic-01", Floor: 1, Star: 1, MinLevel: 22, Enabled: true, Scene: "map_grass_001", MaxRound: 40, Enemies: raw, Reward: "point,10", Source: "专属诊断样本：原hero_047完整技能/基础属性，仅核对真实profile对原演员的计算与回放，不代表原试炼楼层"}
	if err = db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestP7RejectsMissingOriginalMonsterInstanceBeforeFreezing(t *testing.T) {
	for _, kind := range []int32{constUtil.P7RushTrial, constUtil.P7RushElite} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			f := p7Setup(t)
			cfg := p7DiagnosticStage(t, f)
			if kind == constUtil.P7RushElite {
				cfg.Kind, cfg.Star, cfg.Floor = kind, 0, 0
			}
			monster := &pb.BattleMonster{}
			if err := proto.Unmarshal(cfg.Enemies, monster); err != nil {
				t.Fatal(err)
			}
			monster.Inst = nil
			raw, err := proto.Marshal(monster)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Enemies = raw
			if err = db.GetIns().Save(&cfg).Error; err != nil {
				t.Fatal(err)
			}
			before, gameBefore := f.profile(t)
			request := &pb.CSRushFightStart{RushType: proto.Int32(kind), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(cfg.Star)}
			if _, err = f.s.PrepareP7DiagnosticFight(f.ctx, f.id, request, 901); err == nil {
				t.Fatal("configuration that crashes the original monster constructor was accepted")
			}
			if p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7RushReceipt{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P6BattleRecord{}, "player_id=?", f.id) != 0 {
				t.Fatal("invalid original callback left a frozen battle or receipt")
			}
			after, gameAfter := f.profile(t)
			beforeJSON, _ := json.Marshal(gameBefore)
			afterJSON, _ := json.Marshal(gameAfter)
			if !proto.Equal(before, after) || string(beforeJSON) != string(afterJSON) {
				t.Fatal("invalid original callback changed the player's profile")
			}
		})
	}
}

func TestP7OriginalEliteDirectoryVIPBudgetsQueriesAndDayReset(t *testing.T) {
	f := p7Setup(t)
	now := time.Now()
	for i := 0; i < 2; i++ {
		r, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, now)
		if err != nil {
			t.Fatal(err)
		}
		info := r[0].(*pb.SCGetRushEliteBossInfo)
		if len(info.GetBossInfoList()) != len(f.s.Tables["RushEliteBoss"]) || info.GetLeftTime() != 3 || info.GetTotalTime() != 3 || info.GetDiamondLeftTime() != 3 || info.GetBossInfoList()[0].GetFirstRewardList()[0].GetNum() != 2000 || info.GetBossInfoList()[0].GetGeneralRewardList()[0].GetNum() != 40000 {
			t.Fatal("original elite catalog/VIP numbers", info)
		}
		for _, boss := range info.GetBossInfoList() {
			if boss.GetIsOpen() != 0 || boss.GetIsSuccess() != 0 {
				t.Fatal("missing original content open")
			}
		}
	}
	if _, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, now); err == nil {
		t.Fatal("missing normal floors accepted")
	}
	if err := db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", f.id).Updates(map[string]any{"elite_used": 2, "elite_bought": 1}).Error; err != nil {
		t.Fatal(err)
	}
	r, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, now.AddDate(0, 0, 1))
	if err != nil || r[0].(*pb.SCGetRushEliteBossInfo).GetLeftTime() != 3 {
		t.Fatal("elite next day reset", err)
	}
	p, g := f.profile(t)
	if p.GetDiamond() != 1000 || p.GetPoint() != 0 || len(g.Warriors) != 1 || p1Count(t, &db.EventFact{}, "player_id=?", f.id) != 0 {
		t.Fatal("queries fabricated progress or rewards")
	}
}

func TestP7SQLAndClientWinCannotBypassNativeAuthority(t *testing.T) {
	f := p7Setup(t)
	cfg := p7DiagnosticStage(t, f)
	if err := db.GetIns().Model(&db.P7RushPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P6CombatRule{}).Where("id=?", 1).Updates(map[string]any{"authoritative_enabled": true, "native_capability_hash": P6CapabilityHash()}).Error; err != nil {
		t.Fatal(err)
	}
	requests := []proto.Message{&pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(1)}, &pb.CSRushEliteBossBuyTime{}, &pb.CSRushResetChallengeCount{}, &pb.CSRush3Hit{}, &pb.CSRushToTop{}, &pb.CSRushMazeTraderBuy{}}
	for _, request := range requests {
		if _, err := f.s.P7RushMutation(f.ctx, f.id, request, 1); err == nil {
			t.Fatal("non-authoritative operation succeeded", request)
		}
	}
	if p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7RushReceipt{}, "player_id=?", f.id) != 0 || p1Count(t, &db.EventFact{}, "player_id=?", f.id) != 0 {
		t.Fatal("closed gate persisted operation")
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 1000 || p.GetPoint() != 0 {
		t.Fatal("closed gate changed resources")
	}
}

func TestP7RealProfileDiagnosticFreezeConcurrencyReplayAndRestart(t *testing.T) {
	f := p7Setup(t)
	cfg := p7DiagnosticStage(t, f)
	request := &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(1)}
	var wg sync.WaitGroup
	responses := make(chan *pb.SCRushFightStart, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, request, 100)
			responses <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(responses)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *pb.SCRushFightStart
	for r := range responses {
		if first == nil {
			first = r
		} else if !proto.Equal(first, r) {
			t.Fatal("concurrent source not frozen")
		}
	}
	if first.GetBattlePlayer().GetPlayerId() != f.id || first.GetBattlePlayer().GetLevel() != 22 || first.GetBattlePlayer().GetFormation()[0].GetSkillOrder() != "047_1,047_2,047_3" {
		t.Fatal("not actual full player formation", first)
	}
	computed, err := f.s.ComputeP7Diagnostic(f.ctx, f.id, first.GetFightId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if computed.Authoritative || computed.Winner < 0 || computed.Winner > 2 || len(computed.Events) < 1 || computed.FinalRound < 1 {
		t.Fatal("no actual whole computation")
	}
	checked, err := f.s.ComputeP7Diagnostic(f.ctx, f.id, first.GetFightId(), computed.Operates)
	if err != nil || checked.Winner != computed.Winner || !proto.Equal(checked.Operates, computed.Operates) {
		t.Fatal("operation replay changed", err)
	}
	if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushFightEnd{FightId: first.FightId, Result: proto.Int32(1), OptRecord: computed.Operates}, 101); err == nil {
		t.Fatal("diagnostic client win generated rewards")
	}
	p, g := f.profile(t)
	if p.GetDiamond() != 1000 || p.GetPoint() != 0 || len(g.Warriors) != 1 {
		t.Fatal("diagnostic consumed/rewarded resources")
	}
	if p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 1 || p1Count(t, &db.P7RushProgress{}, "player_id=?", f.id) != 0 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7RushStarEvent) != 0 {
		t.Fatal("diagnostic fake trial progression")
	}
	var stage db.P7RushFight
	if err = db.GetIns().Where("player_id=?", f.id).First(&stage).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot db.P7RushStageRule
	if json.Unmarshal(stage.Rule, &snapshot) != nil || snapshot.Reward != "point,10" {
		t.Fatal("reward rule snapshot missing")
	}
	if err = db.GetIns().Model(&cfg).Update("reward", "diamond,9999").Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeP7(f.ctx); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, request, 100)
	if err != nil || !proto.Equal(first, again) {
		t.Fatal("restarted original frame changed", err)
	}
	replay, err := f.s.fightPlayback(f.ctx, f.id, &pb.CSFightPlayback{FightId: first.FightId, ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(replay[0].(*pb.SCFightPlayback).GetOptRecord(), computed.Operates) {
		t.Fatal("P6 frozen replay lost", err)
	}
}

func TestP7OriginalRushCallbacksFreezeExactBattleModes(t *testing.T) {
	for _, test := range []struct {
		name string
		kind int32
		mode int32
	}{{"trial-original-true", constUtil.P7RushTrial, 9}, {"elite-original-false", constUtil.P7RushElite, 10}} {
		t.Run(test.name, func(t *testing.T) {
			f := p7Setup(t)
			cfg := p7DiagnosticStage(t, f)
			if test.kind == constUtil.P7RushElite {
				cfg.Kind, cfg.Star, cfg.Floor = test.kind, 0, 0
				if err := db.GetIns().Save(&cfg).Error; err != nil {
					t.Fatal(err)
				}
			}
			// 原客户端先查询布阵；这一步自然迁移旧顺序站位为原战术坐标。
			if _, err := f.s.formationList(f.ctx, f.id); err != nil {
				t.Fatal(err)
			}
			before, gameBefore := f.profile(t)
			request := &pb.CSRushFightStart{RushType: proto.Int32(test.kind), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(cfg.Star)}
			start, err := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, request, 310)
			if err != nil {
				t.Fatal(err)
			}
			var replay db.P6BattleRecord
			if err = db.GetIns().First(&replay, "player_id=? AND source=? AND source_fight=?", f.id, constUtil.P7RushReplaySource, start.GetFightId()).Error; err != nil {
				t.Fatal(err)
			}
			frozen, decodeErr := decodeP6Record(&replay)
			if decodeErr != nil {
				t.Fatal("original frozen Rush source invalid", decodeErr)
			}
			if frozen.BattleType != test.mode || replay.Authoritative {
				t.Fatal("original Rush callback mode or authority changed", frozen.BattleType)
			}
			originalInput := append([]byte(nil), replay.Input...)
			f.restart(t)
			again, err := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, request, 310)
			if err != nil || !proto.Equal(start, again) {
				t.Fatal("cold original Rush frame changed", err)
			}
			if err = db.GetIns().First(&replay, "id=?", replay.ID).Error; err != nil || string(replay.Input) != string(originalInput) {
				t.Fatal("cold frozen source changed", err)
			}
			after, gameAfter := f.profile(t)
			beforeJSON, _ := json.Marshal(gameBefore)
			afterJSON, _ := json.Marshal(gameAfter)
			if !proto.Equal(before, after) || string(beforeJSON) != string(afterJSON) {
				t.Fatal("diagnostic mode fix changed actual profile resources")
			}
			if _, err = f.s.P7RushMutation(f.ctx, f.id, request, 311); err == nil {
				t.Fatal("closed original Rush authority gate opened")
			}
		})
	}
}

func TestP7FrozenTamperOwnershipAndActivityGate(t *testing.T) {
	f := p7Setup(t)
	cfg := p7DiagnosticStage(t, f)
	r, err := f.s.PrepareP7DiagnosticFight(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String(cfg.TaskID), Star: proto.Int32(1)}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ComputeP7Diagnostic(f.ctx, f.id+100, r.GetFightId(), nil); err == nil {
		t.Fatal("foreign diagnostic readable")
	}
	if err = db.GetIns().Model(&db.P7RushFight{}).Where("id=?", r.GetFightId()).Update("rule", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ComputeP7Diagnostic(f.ctx, f.id, r.GetFightId(), nil); err == nil {
		t.Fatal("changed rule accepted")
	}
	var activity db.PeriodInstance
	if err = db.GetIns().Where("kind=?", constUtil.P7RushStarEvent).First(&activity).Error; err != nil || activity.Enabled {
		t.Fatal("no actual stars yet activity open", err)
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7RushProgress{}, "player_id=?", f.id) != 0 {
		t.Fatal("gate created reward or progress")
	}
}

// TestP7ProtocolDiagnosticSeed 为专属MySQL原录像帧准备可追溯诊断，普通测试仍验证计算。
func TestP7ProtocolDiagnosticSeed(t *testing.T) {
	f := p7Setup(t)
	p7DiagnosticStage(t, f)
	account, err := LoginAccount(f.ctx, "p7-native-diagnostic", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "原帧冻结诊断", "hero_047"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, account.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(22); return nil }); err != nil {
		t.Fatal(err)
	}
	start, err := f.s.PrepareP7DiagnosticFight(f.ctx, account.Id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("diagnostic-01"), Star: proto.Int32(1)}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ComputeP7Diagnostic(f.ctx, account.Id, start.GetFightId(), nil); err != nil {
		t.Fatal(err)
	}
}
