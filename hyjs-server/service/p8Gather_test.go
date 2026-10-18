package service

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP8Gather 使用有限隔离账号及原随机表；时间边界通过独立采集行推进，不注入采集奖励。
func TestP8Gather(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p8.db")
	schema := os.Getenv("HYJS_P8_MYSQL_DATABASE")
	open := func() {
		t.Helper()
		var err error
		if schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p8_") {
				t.Fatal("P8 database rejected")
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
	if err := s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{s.InitializeP1, s.InitializeP8Gather, s.InitGems, s.InitCooking} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	state := func(id int64) (*pb.Player, *model.GameState, db.Profile) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g, row
	}
	fresh := func(name string, level int32) int64 {
		t.Helper()
		row, err := LoginAccount(ctx, "p8-"+name, "test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "采集隔离回归", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(level)
			_, err := s.GrantFixedReward(p, g, "coin,10000000;equip_037,1")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if level >= 18 {
			if _, err = s.fieldTransfer(ctx, row.Id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
				t.Fatal(err)
			}
			if err = updateProfileWithTx(ctx, row.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				_, err := s.RefreshP8Resources(tx, p, g, time.Now(), true)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
		return row.Id
	}
	nodes := func(id int64) []db.P8ResourceNode {
		t.Helper()
		rows, err := P8GatherNodes(db.GetIns(), id)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	qualify := func(id int64) {
		t.Helper()
		// 一级/二级点都来自第一图，初始二级资格仅属于本隔离账号的有限准备。
		if err := db.GetIns().Save(&db.GemCraftState{PlayerID: id, Level: 2}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 2}).Error; err != nil {
			t.Fatal(err)
		}
	}
	move := func(id int64, node db.P8ResourceNode) {
		t.Helper()
		cfg, err := p8DecodeConfig(node.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			if err := s.P8CancelGather(tx, id, g); err != nil {
				return err
			}
			g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	ready := func(id int64) {
		t.Helper()
		if result := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=? AND active=?", id, true).Update("ready_at", time.Now().UnixMilli()-1); result.Error != nil || result.RowsAffected != 1 {
			t.Fatal("isolated timer advance", result.Error)
		}
	}
	stamp := int64(90000)
	call := func(id int64, b proto.Message) []proto.Message {
		t.Helper()
		raw, err := proto.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		decoded := b.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, handled, err := s.DispatchP8Gather(ctx, id, decoded, stamp)
		if !handled || err != nil {
			t.Fatalf("P8 %T handled=%v err=%v", b, handled, err)
		}
		return out
	}
	deny := func(id int64, b proto.Message) {
		t.Helper()
		_, _, before := state(id)
		stamp++
		_, handled, err := s.DispatchP8Gather(ctx, id, b, stamp)
		if !handled || err == nil {
			t.Fatalf("P8 denial %T handled=%v err=%v", b, handled, err)
		}
		_, _, after := state(id)
		if *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("failed gathering changed profile")
		}
	}
	stack := func(items []*pb.SimpleItem, id string) int64 {
		var n int64
		for _, item := range items {
			if item.GetId() == id {
				n += int64(item.GetNum())
			}
		}
		return n
	}
	gather := func(id int64, kind string) *pb.SCFieldGetResource {
		t.Helper()
		rows := nodes(id)
		var node *db.P8ResourceNode
		for i := range rows {
			cfg, _ := p8DecodeConfig(rows[i].Snapshot)
			if cfg.Kind == kind && rows[i].Remaining > 0 {
				node = &rows[i]
				break
			}
		}
		if node == nil {
			// 独立时间推进触发原60秒重生，保留原随机产出与实际疲劳消费。
			if err := db.GetIns().Model(&db.P8ResourceNode{}).Where("player_id=? AND remaining=0", id).Update("refresh_at", time.Now().UnixMilli()-1).Error; err != nil {
				t.Fatal(err)
			}
			if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				_, err := s.RefreshP8Resources(tx, p, g, time.Now(), false)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			rows = nodes(id)
			for i := range rows {
				cfg, _ := p8DecodeConfig(rows[i].Snapshot)
				if cfg.Kind == kind && rows[i].Remaining > 0 {
					node = &rows[i]
					break
				}
			}
		}
		if node == nil {
			t.Fatal("no original resource available")
		}
		move(id, *node)
		start := call(id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})[0].(*pb.SCFieldResourceStartGather)
		if start.GetResult() != 1 || start.GetNeed()[0].GetNum() != 2 {
			t.Fatal("native gathering start", start)
		}
		ready(id)
		out := call(id, &pb.CSFieldGetResource{})[0].(*pb.SCFieldGetResource)
		if out.GetCode() != 1 || len(out.GetCost()) != 1 || out.GetCost()[0].GetNum() != 2 || len(out.GetRes().GetSimps()) < 1 {
			t.Fatal("actual gathering claim", out)
		}
		return out
	}

	t.Run("map_points_distance_level_and_original_timer", func(t *testing.T) {
		id := fresh("gate", 17)
		deny(id, &pb.CSFieldResourceStartGather{PosId: proto.String("mine_pos_1")})
		if _, err := s.fieldTransfer(ctx, id, &pb.CSTransferScene{Target: proto.String("field_001")}); err == nil {
			t.Fatal("17 entered field")
		}
		id = fresh("points", 18)
		rows := nodes(id)
		counts := map[string]int{}
		for _, node := range rows {
			cfg, _ := p8DecodeConfig(node.Snapshot)
			counts[cfg.Kind]++
			if cfg.Times != 5 || cfg.Fatigue != 2 || cfg.CooldownMS != 60000 || cfg.Kind == "mine" && cfg.GatherMS != 3200 || cfg.Kind == "fish" && cfg.GatherMS != 5000 {
				t.Fatal("original resource values", cfg)
			}
		}
		if len(rows) != 18 || counts["mine"] != 10 || counts["fish"] != 8 {
			t.Fatal("field original max nodes", counts)
		}
		qualify(id)
		node := rows[0]
		deny(id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})
		move(id, node)
		cost := call(id, &pb.CSGatherCost{PosId: proto.String(node.PosID)})[0].(*pb.SCGatherCost)
		if cost.GetNeed()[0].GetId() != "tiredness" || cost.GetNeed()[0].GetNum() != 2 {
			t.Fatal("native gather cost")
		}
		call(id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})
		var pending db.P8GatherAttempt
		if err := db.GetIns().First(&pending, "player_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		cfg, _ := p8DecodeConfig(node.Snapshot)
		if pending.ReadyAt-pending.StartedAt != cfg.GatherMS {
			t.Fatal("server gathering timer")
		}
		deny(id, &pb.CSFieldGetResource{})
		_, g, _ := state(id)
		if g.Field.Side.GetTiredness() != 200 {
			t.Fatal("start charged fatigue")
		}
		missing := call(id, &pb.CSSideResourceCheck{PosId: proto.String("mine_pos_999")})[0].(*pb.SCSideResourceCheck)
		if missing.GetCode() != 2003 {
			t.Fatal("unknown resource shape")
		}
		for _, n := range rows {
			cfg, _ := p8DecodeConfig(n.Snapshot)
			if cfg.Level == 2 {
				move(id, n)
				if err := db.GetIns().Save(&db.GemCraftState{PlayerID: id, Level: 1}).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 1}).Error; err != nil {
					t.Fatal(err)
				}
				deny(id, &pb.CSFieldResourceStartGather{PosId: proto.String(n.PosID)})
				break
			}
		}
	})
	t.Run("cancel_frozen_reward_parallel_receipt_and_database_reopen", func(t *testing.T) {
		id := fresh("resume", 18)
		qualify(id)
		node := nodes(id)[0]
		move(id, node)
		request := &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)}
		first := call(id, request)
		startStamp := stamp
		var pending db.P8GatherAttempt
		db.GetIns().First(&pending, "player_id=?", id)
		if _, _, err := s.DispatchP8Gather(ctx, id, &pb.CSFieldResourceStartGather{PosId: proto.String("other")}, startStamp); err == nil {
			t.Fatal("start receipt collision")
		}
		move(id, node)
		if call(id, &pb.CSFieldGetResource{})[0].(*pb.SCFieldGetResource).GetCode() != 2003 {
			t.Fatal("cancelled gather granted")
		}
		_, g, _ := state(id)
		if g.Field.Side.GetTiredness() != 200 || g.Field.Side.GetState() != 0 {
			t.Fatal("cancel charged or kept activity")
		}
		call(id, request)
		db.GetIns().First(&pending, "player_id=?", id)
		frozen := pending.Rewards
		ready(id)
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		var wait sync.WaitGroup
		errs := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_, _, err := s.DispatchP8Gather(ctx, id, &pb.CSFieldGetResource{}, 880001)
				errs <- err
			}()
		}
		wait.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal("parallel completed gathering", err)
			}
		}
		out, _, err := s.DispatchP8Gather(ctx, id, &pb.CSFieldGetResource{}, 880001)
		if err != nil {
			t.Fatal(err)
		}
		result := out[0].(*pb.SCFieldGetResource)
		got := map[string]int32{}
		for _, item := range result.GetRes().GetSimps() {
			got[item.GetId()] += item.GetNum()
		}
		for _, spec := range strings.Split(frozen, ";") {
			pair := strings.Split(spec, ",")
			n, _ := strconv.Atoi(pair[1])
			if got[pair[0]] != int32(n) {
				t.Fatal("frozen original weighted reward changed")
			}
		}
		var count int64
		db.GetIns().Model(&db.P8GatherReceipt{}).Where("player_id=? AND operation=?", id, constUtil.P8GatherClaimOperation).Count(&count)
		if count != 2 {
			t.Fatal("claim receipt count", count)
		}
		_, g, _ = state(id)
		if g.Field.Side.GetTiredness() != 198 {
			t.Fatal("parallel fatigue duplication")
		}
		replay, _, err := s.DispatchP8Gather(ctx, id, request, startStamp)
		if err != nil || !proto.Equal(replay[0], first[0]) {
			t.Fatal("historical start lost", err)
		}
	})
	t.Run("overflow_failure_preserves_attempt_node_fatigue_and_retries", func(t *testing.T) {
		id := fresh("overflow", 18)
		qualify(id)
		var node db.P8ResourceNode
		for _, row := range nodes(id) {
			cfg, _ := p8DecodeConfig(row.Snapshot)
			if cfg.Kind == "mine" {
				node = row
				break
			}
		}
		move(id, node)
		call(id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})
		var pending db.P8GatherAttempt
		db.GetIns().First(&pending, "player_id=?", id)
		key := strings.Split(pending.Rewards, ",")[0]
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
			g.Gems = []*pb.SimpleItem{{Id: proto.String(key), Num: proto.Int32(math.MaxInt32)}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		ready(id)
		deny(id, &pb.CSFieldGetResource{})
		failedStamp := stamp
		db.GetIns().First(&pending, "player_id=?", id)
		_, g, _ := state(id)
		if !pending.Active || g.Field.Side.GetTiredness() != 200 {
			t.Fatal("failed grant consumed attempt")
		}
		for _, current := range nodes(id) {
			if current.PosID == node.PosID && current.Remaining != 5 {
				t.Fatal("failed grant consumed node")
			}
		}
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error { g.Gems = nil; return nil }); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.DispatchP8Gather(ctx, id, &pb.CSFieldGetResource{}, failedStamp); err != nil {
			t.Fatal("failed frame could not retry", err)
		}
	})
	t.Run("five_uses_cooldown_and_original_weighted_respawn", func(t *testing.T) {
		id := fresh("respawn", 18)
		qualify(id)
		node := nodes(id)[0]
		for n := 0; n < 5; n++ {
			move(id, node)
			call(id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})
			ready(id)
			call(id, &pb.CSFieldGetResource{})
		}
		var saved db.P8ResourceNode
		db.GetIns().First(&saved, "player_id=? AND pos_id=?", id, node.PosID)
		if saved.Remaining != 0 || saved.RefreshAt <= time.Now().UnixMilli() {
			t.Fatal("original five-use cooldown")
		}
		if call(id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})[0].(*pb.SCFieldResourceStartGather).GetResult() != 2003 {
			t.Fatal("depleted node allowed gather")
		}
		if err := db.GetIns().Model(&saved).Update("refresh_at", time.Now().UnixMilli()-1).Error; err != nil {
			t.Fatal(err)
		}
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			messages, err := s.RefreshP8Resources(tx, p, g, time.Now(), false)
			if err == nil && len(messages) == 0 {
				t.Fatal("respawn not pushed")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		db.GetIns().First(&saved, "player_id=? AND pos_id=?", id, node.PosID)
		if saved.Remaining != 5 || saved.Generation != node.Generation+1 || saved.RefreshAt != 0 {
			t.Fatal("resource respawn state", saved)
		}
	})
	t.Run("actual_mine_to_refine_craft_mount_and_battle_effect", func(t *testing.T) {
		id := fresh("gem-chain", 18)
		qualify(id)
		_, g, _ := state(id)
		equip := g.Equipment[0]
		holes, err := s.gemHoles(equip)
		if err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{}
		for _, color := range holes {
			allowed[strconv.Itoa(int(color))] = true
		}
		target := ""
		gathers := 0
		for gathers < 90 && target == "" {
			gather(id, "mine")
			gathers++
			_, g, _ = state(id)
			for _, raw := range []string{"MineItem_007", "MineItem_008"} {
				n := int32(stack(g.Gems, raw) / 5 * 5)
				if n >= 5 {
					stamp++
					if _, err := s.gemRefine(ctx, id, &pb.CSMineRefine{MineId: proto.String(raw), Num: proto.Int32(n)}, stamp); err != nil {
						t.Fatal("actual collected ore refine", err)
					}
				}
			}
			p, current, _ := state(id)
			for _, row := range s.Tables["MineSkill"] {
				if row["Type"] != "magicGem" || row["LevelLimit"] != "1" || !allowed[row["Color"]] || row["SkillNeeded"] != "" {
					continue
				}
				costs, err := p3Resources(row["Material"])
				if err != nil {
					continue
				}
				available := true
				for _, cost := range costs {
					have := stack(current.Gems, cost.GetId())
					if cost.GetId() == "coin" {
						have = p.GetCoin()
					}
					available = available && have >= int64(cost.GetNum())
				}
				if available {
					target = row["MineItemID"]
					break
				}
			}
		}
		if target == "" {
			t.Fatal("actual original gathering never provided one gem recipe")
		}
		stamp++
		crafted, err := s.gemMake(ctx, id, &pb.CSGemMake{GemId: proto.String(target), Num: proto.Int32(1)}, stamp)
		if err != nil || len(crafted[0].(*pb.SCGemMake).GetRes().GetSimps()) != 1 {
			t.Fatal("gem from actual gathered materials", err)
		}
		if _, err := s.equipmentFormationOn(ctx, id, &pb.CSFormationOn{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), FormatPos: proto.Int32(1)}); err != nil {
			t.Fatal(err)
		}
		p, g, _ := state(id)
		before, err := s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		stamp++
		if _, err := s.gemMountOrChange(ctx, id, &pb.CSGemMountOrChange{Guid: proto.String(equip.GetGuid()), GemId: proto.String(target)}, stamp); err != nil {
			t.Fatal(err)
		}
		p, g, _ = state(id)
		after, err := s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		first, second := before.GetFormation()[0].GetProp(), after.GetFormation()[0].GetProp()
		if second.GetHp() <= first.GetHp() && second.GetAtt() <= first.GetAtt() && second.GetPdef() <= first.GetPdef() && second.GetMdef() <= first.GetMdef() && second.GetHit() <= first.GetHit() && second.GetDodge() <= first.GetDodge() && second.GetCri() <= first.GetCri() && second.GetAntiCri() <= first.GetAntiCri() {
			t.Fatal("actual crafted gem had no battle property effect", target)
		}
		_, exp, err := s.GemCraftLevel(ctx, id)
		if err != nil || exp < 50 {
			t.Fatal("native refining/craft experience", err, exp)
		}
	})
	t.Run("actual_fishing_to_cooking_use_and_persistent_effect", func(t *testing.T) {
		id := fresh("cooking-chain", 18)
		qualify(id)
		target := ""
		for n := 0; n < 80 && target == ""; n++ {
			gather(id, "fish")
			p, g, _ := state(id)
			for _, row := range s.Tables["FishSkill"] {
				level, _ := strconv.Atoi(row["LevelLimit"])
				if row["Type"] != "dish" || !strings.HasPrefix(row["ValueType"], "permanent") || level > 2 || row["SkillNeeded"] != "" {
					continue
				}
				costs, err := p3Resources(row["Material"])
				if err != nil {
					continue
				}
				available := true
				for _, cost := range costs {
					have := stack(g.Cooking, cost.GetId())
					if cost.GetId() == "coin" {
						have = p.GetCoin()
					}
					available = available && have >= int64(cost.GetNum())
				}
				if available {
					target = row["FishItemID"]
					break
				}
			}
		}
		if target == "" {
			t.Fatal("actual fish did not unlock an executable dish")
		}
		stamp++
		cooked, err := s.fishCooking(ctx, id, &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String(target), Num: proto.Int32(1)}}, stamp)
		if err != nil || cooked[0].(*pb.SCFishCooking).GetAdd() < 1 {
			t.Fatal("cooking actual fish", err)
		}
		p, _, _ := state(id)
		stamp++
		if _, err := s.fishUsing(ctx, id, &pb.CSFishUsing{CostItem: &pb.BaseResource{Id: proto.String(target), Num: proto.Int32(1)}, HeroGuid: proto.String(p.GetHeroWarId())}, stamp); err != nil {
			t.Fatal("actual dish use", err)
		}
		_, g, _ := state(id)
		w, err := p5Warrior(g, p.GetHeroWarId())
		if err != nil {
			t.Fatal(err)
		}
		if w.GetCooking().GetProp() == nil || stack(g.Cooking, target) != 0 {
			t.Fatal("dish effect or cost not saved")
		}
		snapshot, _ := json.Marshal(w.GetCooking())
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		_, g, _ = state(id)
		w, _ = p5Warrior(g, p.GetHeroWarId())
		reloaded, _ := json.Marshal(w.GetCooking())
		if string(snapshot) != string(reloaded) {
			t.Fatal("cooking effect lost on reconnect")
		}
	})
}
