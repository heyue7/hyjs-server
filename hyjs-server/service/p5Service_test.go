package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
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

// TestP5Domain creates only disposable SQLite or a dedicated hyjs_test_p5_* database.
func TestP5Domain(t *testing.T) {
	ctx := context.Background()
	schema := os.Getenv("HYJS_P5_MYSQL_DATABASE")
	path := filepath.Join(t.TempDir(), "p5.db")
	open := func() {
		t.Helper()
		var err error
		if schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p5_") {
				t.Fatal("P5 database rejected")
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
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitEquipmentMutations, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	fresh := func(name string, level int32, resources string) int64 {
		t.Helper()
		row, err := LoginAccount(ctx, "demo-p5-"+name, "test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "养成测试", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(level)
			if resources != "" {
				_, err := s.GrantFixedReward(p, g, resources)
				return err
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return row.Id
	}
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
	stamp := int64(1000)
	call := func(id int64, b proto.Message) []proto.Message {
		t.Helper()
		raw, err := proto.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		request := b.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, request); err != nil {
			t.Fatal(err)
		}
		stamp++
		messages, handled, err := s.DispatchP5(ctx, id, request, stamp)
		if err != nil || !handled {
			t.Fatalf("P5 %T: handled=%v err=%v", b, handled, err)
		}
		return messages
	}
	deny := func(id int64, b proto.Message) {
		t.Helper()
		stamp++
		_, handled, err := s.DispatchP5(ctx, id, b, stamp)
		if !handled || err == nil {
			t.Fatalf("P5 denial %T handled=%v err=%v", b, handled, err)
		}
	}
	warrior := func(id int64, key string) *pb.Warrior {
		t.Helper()
		_, g := state(id)
		for _, w := range g.Warriors {
			if w.GetId() == key {
				return w
			}
		}
		t.Fatal("missing fixture hero", key)
		return nil
	}

	t.Run("native_basic_items_retry_collision_and_overflow_rollback", func(t *testing.T) {
		id := fresh("consumable", 1, "item_001,2;item_002,1;item_062,1")
		request := &pb.CSItemUse{CostItem: p2Item("item_001", 1)}
		response := call(id, request)
		saved := stamp
		replayed, handled, err := s.DispatchP5(ctx, id, request, saved)
		if err != nil || !handled || !proto.Equal(response[0], replayed[0]) {
			t.Fatal("consumable receipt", err)
		}
		if _, _, err = s.DispatchP5(ctx, id, &pb.CSItemUse{CostItem: p2Item("item_002", 1)}, saved); err == nil {
			t.Fatal("same receipt accepted changed input")
		}
		p, g := state(id)
		if p.GetEnergy() != 110 || simpleOwned(p, g, "item_001") != 1 || simpleOwned(p, g, "item_002") != 1 {
			t.Fatal("consumable applied twice or collision consumed")
		}
		deny(id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_001"), Guid: proto.String("forged"), Num: proto.Int32(1)}})
		deny(id, &pb.CSItemUse{CostItem: p2Item("item_001", 1), GetItemIndex: proto.Int32(1)})
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Stamina = proto.Int32(math.MaxInt32); return nil }); err != nil {
			t.Fatal(err)
		}
		deny(id, &pb.CSItemUse{CostItem: p2Item("item_002", 1)})
		p, g = state(id)
		if p.GetStamina() != math.MaxInt32 || simpleOwned(p, g, "item_002") != 1 {
			t.Fatal("overflow consumed basic item")
		}
		call(id, &pb.CSItemUse{CostItem: p2Item("item_062", 1)})
		p, g = state(id)
		if p.GetCoin() != 10000 || simpleOwned(p, g, "item_062") != 0 {
			t.Fatal("native coin item mismatch")
		}
	})
	t.Run("fixed_and_selected_bundles_capacity_and_invalid_selection", func(t *testing.T) {
		id := fresh("boxes", 1, "item_034,2;item_067,2")
		out := call(id, &pb.CSItemUse{CostItem: p2Item("item_034", 2)})[0].(*pb.SCItemUse)
		if len(out.GetGet().GetWars()) != 20 {
			t.Fatal("fixed exp bundle", out)
		}
		p, g := state(id)
		if p.GetCoin() != 100000 || simpleOwned(p, g, "item_034") != 0 {
			t.Fatal("fixed bundle resources")
		}
		deny(id, &pb.CSItemUse{CostItem: p2Item("item_067", 1), GetItemIndex: proto.Int32(999)})
		p, g = state(id)
		if simpleOwned(p, g, "item_067") != 2 {
			t.Fatal("invalid choice consumed box")
		}
		choice := call(id, &pb.CSItemUse{CostItem: p2Item("item_067", 1), GetItemIndex: proto.Int32(1)})[0].(*pb.SCItemUse)
		if choice.GetGet().GetWars()[0].GetId() != "hero_011" {
			t.Fatal("original choice mismatch")
		}
		overflow := fresh("box-capacity", 1, "item_035,1")
		if err := updateProfile(ctx, overflow, func(p *pb.Player, g *model.GameState) error { p.WarriorBagSize = proto.Int32(1); return nil }); err != nil {
			t.Fatal(err)
		}
		deny(overflow, &pb.CSItemUse{CostItem: p2Item("item_035", 1)})
		p, g = state(overflow)
		if p.GetCoin() != 0 || simpleOwned(p, g, "item_035") != 1 || len(g.Warriors) != 1 {
			t.Fatal("capacity not atomic")
		}
	})
	t.Run("ordinary_fragment_combine_and_native_soul_prestige_sale", func(t *testing.T) {
		id := fresh("fragments", 1, "frag_hero_020,42;frag_equip_001,3")
		combined, err := s.redCombine(ctx, id, &pb.CSCombine{TargetId: proto.String("hero_020")}, 101)
		if err != nil || combined[0].(*pb.SCCombine).GetGet().GetWars()[0].GetId() != "hero_020" {
			t.Fatal("ordinary combine", err)
		}
		call(id, &pb.CSWarriorFragSell{Sell: []*pb.BaseResource{p2Item("frag_hero_020", 2)}})
		call(id, &pb.CSEquipmentFragSell{Cost: []*pb.BaseResource{p2Item("frag_equip_001", 3)}})
		p, g := state(id)
		if p.GetCoin() != 0 || p.GetPoint() != 30 || simpleOwned(p, g, "item_033") != 14 {
			t.Fatal("native sale currencies", p)
		}
		deny(id, &pb.CSWarriorFragSell{Sell: []*pb.BaseResource{p2Item("frag_hero_020", 1)}})
		deny(id, &pb.CSEquipmentFragSell{Cost: []*pb.BaseResource{p2Item("frag_equip_001", 1), p2Item("frag_equip_001", 1)}})
	})
	t.Run("helper_unlock_duplicate_formation_and_recycle_protection", func(t *testing.T) {
		id := fresh("helper", 24, "hero_001,1;hero_002,1")
		a := warrior(id, "hero_001")
		deny(id, &pb.CSHelperOn{Guid: proto.String(a.GetGuid()), Pos: proto.Int32(1)})
		if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(25); return nil }); err != nil {
			t.Fatal(err)
		}
		call(id, &pb.CSHelperOn{Guid: proto.String(a.GetGuid()), Pos: proto.Int32(1)})
		p, g := state(id)
		if len(formationListResult(g).GetHelperList()) != 1 {
			t.Fatal("helper protocol projection")
		}
		deny(id, &pb.CSHelperOn{Guid: proto.String(a.GetGuid()), Pos: proto.Int32(2)})
		deny(id, &pb.CSWarriorSell{Guid: []string{a.GetGuid()}})
		if _, err := s.formationOn(ctx, id, &pb.CSFormationOn{Id: proto.String(a.GetId()), Guid: proto.String(a.GetGuid()), FormatPos: proto.Int32(2)}); err == nil {
			t.Fatal("helper simultaneously in battle")
		}
		call(id, &pb.CSHelperOff{Pos: proto.Int32(1)})
		p, g = state(id)
		if err := P5GuardInstance(p, g, a.GetGuid()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("relations_subject_scope_and_original_six_partner_blessing", func(t *testing.T) {
		id := fresh("relations", 100, "hero_001,1;hero_002,1;hero_007,1;hero_008,1;hero_009,1;hero_010,1;hero_011,1")
		subject := warrior(id, "hero_001")
		if _, err := s.formationOn(ctx, id, &pb.CSFormationOn{Id: proto.String(subject.GetId()), Guid: proto.String(subject.GetGuid()), FormatPos: proto.Int32(2)}); err != nil {
			t.Fatal(err)
		}
		keys := []string{"hero_002", "hero_007", "hero_008", "hero_009", "hero_010", "hero_011"}
		for i, key := range keys {
			w := warrior(id, key)
			if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
				v, _ := p5Warrior(g, w.GetGuid())
				v.Level = proto.Int32(10)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			call(id, &pb.CSHelperOn{Guid: proto.String(w.GetGuid()), Pos: proto.Int32(int32(i + 1))})
		}
		p, g := state(id)
		w, _ := p5Warrior(g, subject.GetGuid())
		values, percent := map[string]float64{}, map[string]float64{}
		if err := s.AddP5StaticBonus(p, g, w, values, percent); err != nil {
			t.Fatal(err)
		}
		if math.Abs(percent["att"]-.46) > 1e-8 || values["hp"] != 1000 || values["att"] != 50 {
			t.Fatal("relation/partner scope", values, percent)
		}
		p.Level = proto.Int32(99)
		values = map[string]float64{}
		if err := s.AddP5StaticBonus(p, g, w, values, map[string]float64{}); err != nil || values["att"] != 0 {
			t.Fatal("blessing gate", err, values)
		}
		call(id, &pb.CSHelperOff{Pos: proto.Int32(6)})
		p, g = state(id)
		values = map[string]float64{}
		if err := s.AddP5StaticBonus(p, g, w, values, map[string]float64{}); err != nil || values["att"] != 0 {
			t.Fatal("requires six helpers", err, values)
		}
	})
	t.Run("skill_order_server_owned_permutation", func(t *testing.T) {
		id := fresh("skill", 1, "")
		p, _ := state(id)
		call(id, &pb.CSSetSkillOrder{HeroGuid: proto.String(p.GetHeroWarId()), SkillOrder: proto.String("043_2,043_1")})
		w := warrior(id, "hero_043")
		if w.GetSkillOrder() != "043_2,043_1" {
			t.Fatal("order not persisted")
		}
		p, g := state(id)
		battle, err := s.battlePlayer(p, g)
		if err != nil || battle.GetFormation()[0].GetSkillOrder() != "043_2,043_1,043_3" {
			t.Fatal("skill snapshot dropped ultimate", err, battle)
		}
		deny(id, &pb.CSSetSkillOrder{HeroGuid: proto.String(w.GetGuid()), SkillOrder: proto.String("043_1,043_1")})
	})
	t.Run("world_seed_order_rewards_quality_and_identity", func(t *testing.T) {
		id := fresh("seed", 12, "item_017,20;equip_001,1")
		p, _ := state(id)
		mainGUID := p.GetHeroWarId()
		deny(id, &pb.CSLightenWorldSeed{Id: proto.Int32(5)})
		for next := int32(2); next <= 10; next++ {
			call(id, &pb.CSLightenWorldSeed{Id: proto.Int32(next - 1)})
		}
		p, g := state(id)
		main, err := p5Warrior(g, mainGUID)
		if err != nil || main.GetId() != "hero_044" || p.GetHeroWarId() != mainGUID || p.GetCurrentWorldSeedId() != 10 || simpleOwned(p, g, "item_052") != 1 || simpleOwned(p, g, "item_017") != 11 {
			t.Fatal("seed identity", err, p, main)
		}
		if formationHero(g.Formation[0]).GetId() != "hero_044" {
			t.Fatal("formation identity stale")
		}
		deny(id, &pb.CSLightenWorldSeed{Id: proto.Int32(9)})
		swapped, err := s.SwapP5MainHero(ctx, id, 101, 8001)
		if err != nil || swapped[0].(*pb.SCWarriorGet).GetWarrior()[0].GetId() != "hero_048" {
			t.Fatal("bounded swap API", err)
		}
		p, g = state(id)
		if p.GetHeroWarId() != mainGUID {
			t.Fatal("swap duplicated identity")
		}
	})
	t.Run("tracked_training_reset_exact_refund_fee_and_replay", func(t *testing.T) {
		id := fresh("reset", 25, "coin,1000000;diamond,100;item_007,20;hero_020,1")
		w := warrior(id, "hero_020")
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
			value, _ := p5Warrior(g, w.GetGuid())
			value.Level = proto.Int32(10)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		w = warrior(id, "hero_020")
		before, _ := state(id)
		beforeW := proto.Clone(w).(*pb.Warrior)
		if _, err := s.warriorTraining(ctx, id, &pb.CSWarriorTraining{Guid: proto.String(w.GetGuid()), CostType: proto.Int32(1), NumType: proto.Int32(1)}, 9001); err != nil {
			t.Fatal(err)
		}
		if _, err := s.warriorTrainingReplace(ctx, id, &pb.CSWarriorTrainingReplace{Guid: proto.String(w.GetGuid()), Result: proto.Int32(1)}, 9002); err != nil {
			t.Fatal(err)
		}
		request := &pb.CSWarriorReset{Guid: proto.String(w.GetGuid())}
		messages := call(id, request)
		savedStamp := stamp
		after, g := state(id)
		restored, _ := p5Warrior(g, w.GetGuid())
		if !proto.Equal(restored, beforeW) || after.GetCoin() != before.GetCoin() || after.GetDiamond() != before.GetDiamond()-50 {
			t.Fatal("tracked reset did not restore exact baseline", restored, beforeW, after)
		}
		repeat, handled, err := s.DispatchP5(ctx, id, request, savedStamp)
		if err != nil || !handled || !proto.Equal(messages[0], repeat[0]) {
			t.Fatal("reset retry", err)
		}
		deny(id, request)
		again, _ := state(id)
		if again.GetDiamond() != after.GetDiamond() {
			t.Fatal("failed reset charged")
		}
	})
	t.Run("equipment_one_key_actual_cost_and_reset_baseline", func(t *testing.T) {
		id := fresh("equip-reset", 16, "coin,1000000;diamond,100;equip_001,1")
		before, g := state(id)
		equip := g.Equipment[0]
		out := call(id, &pb.CSEquipStrengthenOneKey{Guid: proto.String(equip.GetGuid())})[0].(*pb.SCEquipStrengthenOneKey)
		after, _ := state(id)
		if out.GetCostTimes() < 1 || after.GetCoin() != before.GetCoin()-int64(out.GetCost()[0].GetNum()) || out.GetEquip().GetLevel() != 1+out.GetCostTimes() {
			t.Fatal("auto actual fees")
		}
		deny(id, &pb.CSEquipStrengthenOneKey{Guid: proto.String(equip.GetGuid())})
		call(id, &pb.CSEquipReBuild{Guid: proto.String(equip.GetGuid())})
		after, g = state(id)
		if g.Equipment[0].GetLevel() != 1 || after.GetCoin() != before.GetCoin() || after.GetDiamond() != 50 {
			t.Fatal("auto reset refund", after, g.Equipment[0])
		}
	})
	t.Run("batch_recycle_atomic_guards_legacy_progress_and_history", func(t *testing.T) {
		id := fresh("recycle", 16, "hero_020,1;hero_021,1;equip_001,1;trea_001,1")
		p, g := state(id)
		a := warrior(id, "hero_020")
		deny(id, &pb.CSWarriorSell{Guid: []string{a.GetGuid(), p.GetHeroWarId()}})
		call(id, &pb.CSWarriorSell{Guid: []string{a.GetGuid()}})
		book := call(id, &pb.CSWarriorHandbook{})[0].(*pb.SCWarriorHandbook)
		found := false
		for _, key := range book.GetIdList() {
			found = found || key == a.GetId()
		}
		if !found {
			t.Fatal("sold collection lost")
		}
		b := warrior(id, "hero_021")
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
			w, _ := p5Warrior(g, b.GetGuid())
			w.Level = proto.Int32(2)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		deny(id, &pb.CSWarriorSell{Guid: []string{b.GetGuid()}})
		p, g = state(id)
		equip := g.Equipment[0]
		call(id, &pb.CSEquipmentSell{Cost: []*pb.BaseResource{{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), Num: proto.Int32(1)}}})
		p, g = state(id)
		if len(g.Equipment) != 0 {
			t.Fatal("equipment retained")
		}
		if len(g.Treasures) != 1 {
			t.Fatal("treasure fixture")
		}
		call(id, &pb.CSTreasureSell{Guids: []string{g.Treasures[0].GetGuid()}})
	})
	t.Run("traced_refine_and_treasure_sale_refund_and_batch_rollback", func(t *testing.T) {
		s.P5Ready = true
		id := fresh("traced-sale", 16, "coin,1000000;item_011,2;equip_001,1;equip_002,1;trea_001,1;trea_005,1")
		before, g := state(id)
		equip := g.Equipment[0]
		legacy := g.Equipment[1]
		request := &pb.CSEquipRefine{Guid: proto.String(equip.GetGuid()), Cost: []*pb.BaseResource{p2Item("item_011", 1)}}
		nativeCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
		if _, err := s.equipmentRefine(nativeCtx, id, request, 220001); err != nil {
			t.Fatal("actual refinement", err)
		}
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
			x, err := s.equipmentRecord(g, legacy.GetGuid())
			if err == nil {
				x.RefineExp = proto.Int32(1)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		cost := &pb.BaseResource{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), Num: proto.Int32(1)}
		legacyCost := &pb.BaseResource{Id: proto.String(legacy.GetEquipId()), Guid: proto.String(legacy.GetGuid()), Num: proto.Int32(1)}
		deny(id, &pb.CSEquipmentSell{Cost: []*pb.BaseResource{cost, legacyCost}})
		p, g := state(id)
		if len(g.Equipment) != 2 || p.GetCoin() != before.GetCoin() || simpleOwned(p, g, "item_011") != 1 || p5KV(g, "p5.invest:"+equip.GetGuid()) == "" {
			t.Fatal("later invalid batch member lost first refund or instance")
		}
		deny(id, &pb.CSEquipDecompose{Guid: []string{legacy.GetGuid()}})
		call(id, &pb.CSEquipmentSell{Cost: []*pb.BaseResource{cost}})
		p, g = state(id)
		if len(g.Equipment) != 1 || simpleOwned(p, g, "item_011") != 2 || p5KV(g, "p5.invest:"+equip.GetGuid()) != "" {
			t.Fatal("tracked partial refine sale lost investment")
		}
		var target, food *pb.Treasure
		for _, value := range g.Treasures {
			if value.GetTreasureId() == "trea_001" {
				target = value
			} else {
				food = value
			}
		}
		beforeTreasure, _ := state(id)
		growth := &pb.CSTreasureStrengthen{TargetId: proto.String(target.GetGuid()), Guids: []string{food.GetGuid()}}
		nativeCtx = context.WithValue(nativeCtx, constUtil.GameRequestContextKey, growth)
		if _, err := s.treasureStrengthen(nativeCtx, id, growth); err != nil {
			t.Fatal(err)
		}
		call(id, &pb.CSTreasureSell{Guids: []string{target.GetGuid()}})
		p, g = state(id)
		price := int64(tableNumber(s.table("Treasure", "id", target.GetTreasureId()), "sellPrice"))
		if p.GetCoin() != beforeTreasure.GetCoin()+price || len(g.Treasures) != 1 || g.Treasures[0].GetTreasureId() != food.GetTreasureId() || g.Treasures[0].GetGuid() == food.GetGuid() {
			t.Fatal("tracked treasure sale refund or identity", p, g.Treasures)
		}
	})
	t.Run("collection_achievement_snapshot_and_original_title_expiry", func(t *testing.T) {
		id := fresh("collect", 100, "hero_001,1;hero_002,1;hero_003,1;hero_004,1;hero_005,1")
		list := call(id, &pb.CSAchievementList{})[0].(*pb.SCAchievementList)
		if len(list.GetList()) != 3 {
			t.Fatal("finite achievement catalog")
		}
		if err := db.GetIns().Model(&db.P5Achievement{}).Where("id=?", "100001").Update("reward", "diamond,9999").Error; err != nil {
			t.Fatal(err)
		}
		call(id, &pb.CSGetAchievementReward{Id: proto.String("100001")})
		p, _ := state(id)
		if p.GetDiamond() != 30 {
			t.Fatal("achievement reward rewritten")
		}
		deny(id, &pb.CSGetAchievementReward{Id: proto.String("100001")})
		for index, key := range []string{"hero_001", "hero_002", "hero_003"} {
			w := warrior(id, key)
			if _, err := s.formationOn(ctx, id, &pb.CSFormationOn{Id: proto.String(w.GetId()), Guid: proto.String(w.GetGuid()), FormatPos: proto.Int32(int32(index + 2))}); err != nil {
				t.Fatal(err)
			}
		}
		// Explicit local two-hour fixture verifies the duration unit; original early titles are permanent.
		s.table("Title", "titleId", "title_001")["effectTime"] = "2"
		changed := call(id, &pb.CSChangeTitle{TitleId: proto.String("title_001")})[0].(*pb.SCChangeTitle)
		for _, title := range changed.GetTitleList() {
			if title.GetTitleId() == "title_001" {
				remaining := title.GetDeadTime() - time.Now().UnixMilli()
				if remaining < 7100000 || remaining > 7200000 {
					t.Fatal("title local hours policy", remaining)
				}
			}
		}
		p, g := state(id)
		values := map[string]float64{}
		if err := s.p5TitleBonus(g, values); err != nil || values["hp"] != 50 {
			t.Fatal("title native property", err, values)
		}
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
			var title p5TitleState
			json.Unmarshal([]byte(p5KV(g, "p5.title:title_001")), &title)
			title.DeadTime = time.Now().Add(-time.Minute).UnixMilli()
			raw, _ := json.Marshal(title)
			p5SetKV(g, "p5.title:title_001", string(raw))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		call(id, &pb.CSTitleList{})
		p, g = state(id)
		values = map[string]float64{}
		if err := s.p5TitleBonus(g, values); err != nil || values["hp"] != 0 || p5KV(g, "p5.current-title") != "" {
			t.Fatal("expired title active", err, values)
		}
	})

	t.Run("native_strength_save_hook_team_auto_unmount_and_traced_decompose", func(t *testing.T) {
		id := fresh("team-auto", 16, "coin,1000000;equip_001,1;equip_002,1")
		before, g := state(id)
		first := g.Equipment[0]
		second := g.Equipment[1]
		s.P5Ready = true
		ordinary := &pb.CSEquipStrengthen{Guid: proto.String(first.GetGuid()), Times: proto.Int32(1), Cost: []*pb.BaseResource{p2Item("coin", 8)}}
		nativeCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, ordinary)
		if _, err := s.equipmentStrengthen(nativeCtx, id, ordinary); err != nil {
			t.Fatal("original strengthen", err)
		}
		p, g := state(id)
		if p5KV(g, "p5.invest:"+first.GetGuid()) == "" {
			t.Fatal("shared investment hook missing")
		}
		for _, equip := range []*pb.Equipment{first, second} {
			if _, err := s.formationOn(ctx, id, &pb.CSFormationOn{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), FormatPos: proto.Int32(1)}); err != nil {
				t.Fatal("mount", err)
			}
		}
		beforeAuto, _ := state(id)
		out := call(id, &pb.CSEquipStrengthenMoreOneKey{FormatPos: proto.Int32(0)})[0].(*pb.SCEquipStrengthenMoreOneKey)
		afterAuto, _ := state(id)
		if len(out.GetEquips()) != 2 || afterAuto.GetCoin() != beforeAuto.GetCoin()-int64(out.GetCost()[0].GetNum()) {
			t.Fatal("team auto fees", out)
		}
		deny(id, &pb.CSEquipDecompose{Guid: []string{first.GetGuid(), second.GetGuid()}})
		for _, equip := range []*pb.Equipment{first, second} {
			if _, err := s.formationOff(ctx, id, &pb.CSFormationOff{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), FormatPos: proto.Int32(1)}); err != nil {
				t.Fatal("unmount", err)
			}
		}
		call(id, &pb.CSEquipDecompose{Guid: []string{first.GetGuid(), second.GetGuid()}})
		p, g = state(id)
		if len(g.Equipment) != 0 || p.GetCoin() != before.GetCoin()+2000 || p.GetPoint() != 160 {
			t.Fatal("traced decompose returns", p, len(g.Equipment))
		}
	})

	t.Run("original_treasure_strength_target_guid_and_exact_rebuild", func(t *testing.T) {
		id := fresh("treasure-reset", 16, "coin,1000000;diamond,100;trea_001,1;trea_005,1")
		before, g := state(id)
		var target, food *pb.Treasure
		for _, value := range g.Treasures {
			if value.GetTreasureId() == "trea_001" {
				target = value
			} else if value.GetTreasureId() == "trea_005" {
				food = value
			}
		}
		if target == nil || food == nil {
			t.Fatal("treasure fixture missing")
		}
		s.P5Ready = true
		request := &pb.CSTreasureStrengthen{TargetId: proto.String(target.GetGuid()), Guids: []string{food.GetGuid()}}
		nativeCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
		if _, err := s.treasureStrengthen(nativeCtx, id, request); err != nil {
			t.Fatal("native treasure growth", err)
		}
		_, g = state(id)
		if p5KV(g, "p5.invest:"+target.GetGuid()) == "" {
			t.Fatal("targetId investment not captured")
		}
		call(id, &pb.CStreasureRebuild{TargetId: proto.String(target.GetGuid())})
		after, g := state(id)
		if after.GetCoin() != before.GetCoin() || after.GetDiamond() != 50 || len(g.Treasures) != 2 {
			t.Fatal("treasure actual refund", after, len(g.Treasures))
		}
		restored, err := s.treasureRecord(g, target.GetGuid())
		if err != nil || restored.GetStrengthLevel() != 1 || restored.GetCurrentExp() != 0 {
			t.Fatal("treasure baseline", err, restored)
		}
		for _, value := range g.Treasures {
			if value.GetTreasureId() == food.GetTreasureId() && value.GetGuid() == food.GetGuid() {
				t.Fatal("consumed material instance identity reused")
			}
		}
	})
	t.Run("parallel_same_receipt_once_and_restart", func(t *testing.T) {
		id := fresh("parallel", 16, "coin,1000000;equip_001,1")
		_, g := state(id)
		request := &pb.CSEquipStrengthenOneKey{Guid: proto.String(g.Equipment[0].GetGuid())}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, handled, err := s.DispatchP5(ctx, id, request, 99999)
				if !handled {
					err = fmt.Errorf("not handled")
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var count int64
		db.GetIns().Model(&db.P5MutationReceipt{}).Where("player_id=? AND operation=?", id, "equipment-one-key").Count(&count)
		if count != 1 {
			t.Fatal("parallel duplicate receipt")
		}
		before, _ := state(id)
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		if err := s.InitializeP5(ctx); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.DispatchP5(ctx, id, request, 99999); err != nil {
			t.Fatal(err)
		}
		after, _ := state(id)
		if after.GetCoin() != before.GetCoin() {
			t.Fatal("restart credited/charged again")
		}
	})
	if schema != "" {
		var count int64
		if err := db.GetIns().Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name LIKE 'p5_%' AND engine <> 'InnoDB'", schema).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("P5 tables require InnoDB", err, count)
		}
	}
}
