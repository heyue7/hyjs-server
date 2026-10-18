package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func gemFixture(t *testing.T, mysql bool) (*GameService, int64, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "gem.db")
	if mysql {
		name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
		if !strings.HasPrefix(name, "hyjs_test_") {
			t.Skip("isolated MySQL database required")
		}
		if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); err != nil {
			t.Fatal(err)
		}
	} else if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitGems(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := LoginAccount(ctx, fmt.Sprintf("gem-isolated-%d", time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, profile.Id, "isolated", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, profile.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Coin = proto.Int32(60), proto.Int64(10000000)
		g.Equipment = []*pb.Equipment{{Guid: proto.String("gem-test-equip"), EquipId: proto.String("equip_037"), Level: proto.Int32(1)}}
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(1), Id: proto.String("equip_037"), Guid: proto.String("gem-test-equip")})
		for _, key := range []string{"MineItem_007", "MineItem_013", "MineItem_014", "MineItem_015", "MineItem_016", "MineItem_017", "MineItem_018", "MineItem_081", "MineItem_084"} {
			g.Gems = append(g.Gems, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(2000)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, profile.Id, path
}

func gemProfile(t *testing.T, id int64) (*pb.Player, *model.GameState) {
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

func gemCount(g *model.GameState, key string) int32 {
	for _, item := range g.Gems {
		if item.GetId() == key {
			return item.GetNum()
		}
	}
	return 0
}

func gemCraftAndMountCheck(t *testing.T, mysql bool) {
	s, id, _ := gemFixture(t, mysql)
	ctx := context.Background()
	p, g := gemProfile(t, id)
	before, err := s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	request := &pb.CSGemMake{GemId: proto.String("MineItem_019"), Num: proto.Int32(20)}
	messages, err := s.gemMake(ctx, id, request, 100)
	if err != nil {
		t.Fatal(err)
	}
	result := messages[0].(*pb.SCGemMake)
	if result.GetLevel() != 2 || result.GetExp() != 0 || result.GetAdd() != 1000 || len(result.GetCost()) != 4 || result.GetRes().GetSimps()[0].GetNum() != 20 {
		t.Fatalf("craft response %v", result)
	}
	p, g = gemProfile(t, id)
	if p.GetCoin() != 9800000 || gemCount(g, "MineItem_081") != 1800 || gemCount(g, "MineItem_013") != 1960 || gemCount(g, "MineItem_084") != 1800 || gemCount(g, "MineItem_019") != 20 {
		t.Fatalf("craft inventory coin=%d gems=%v", p.GetCoin(), g.Gems)
	}
	mount := &pb.CSGemMountOrChange{Guid: proto.String("gem-test-equip"), GemId: proto.String("MineItem_019")}
	if _, err = s.gemMountOrChange(ctx, id, mount, 200); err != nil {
		t.Fatal(err)
	}
	p, g = gemProfile(t, id)
	after, err := s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	if after.Formation[0].Prop.GetAtt() <= before.Formation[0].Prop.GetAtt() {
		t.Fatalf("gem missing from battle before=%v after=%v", before.Formation[0], after.Formation[0])
	}
	if _, err = s.gemMake(ctx, id, &pb.CSGemMake{GemId: proto.String("MineItem_020"), Num: proto.Int32(1)}, 101); err != nil {
		t.Fatal(err)
	}
	mount.GemId = proto.String("MineItem_020")
	changed, err := s.gemMountOrChange(ctx, id, mount, 201)
	if err != nil {
		t.Fatal(err)
	}
	change := changed[0].(*pb.SCGemMountOrChange)
	if change.GetEquip().Gems[0] != "MineItem_020" || len(change.Cost) != 1 || change.Res.Simps[0].GetId() != "MineItem_019" {
		t.Fatalf("replacement result %v", change)
	}
	p, g = gemProfile(t, id)
	if gemCount(g, "MineItem_019") != 20 || gemCount(g, "MineItem_020") != 0 {
		t.Fatal("replacement did not return old gem")
	}
	if _, err = s.gemUnmount(ctx, id, &pb.CSGemUnmount{Guid: proto.String("gem-test-equip"), Pos: proto.Int32(0)}, 300); err != nil {
		t.Fatal(err)
	}
	p, g = gemProfile(t, id)
	unmounted, err := s.battlePlayer(p, g)
	if err != nil || unmounted.Formation[0].Prop.GetAtt() != before.Formation[0].Prop.GetAtt() || gemCount(g, "MineItem_020") != 1 {
		t.Fatalf("unmount result err=%v battle=%v", err, unmounted)
	}
}

func TestGemCraftMountBattle(t *testing.T)      { gemCraftAndMountCheck(t, false) }
func TestGemMySQLCraftMountBattle(t *testing.T) { gemCraftAndMountCheck(t, true) }

func gemConcurrentCheck(t *testing.T, mysql bool) {
	s, id, _ := gemFixture(t, mysql)
	ctx := context.Background()
	request := &pb.CSGemMake{GemId: proto.String("MineItem_019"), Num: proto.Int32(1)}
	var wg sync.WaitGroup
	results := make(chan proto.Message, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.gemMake(ctx, id, request, 100)
			if e == nil {
				results <- r[0]
			}
			errors <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first proto.Message
	for result := range results {
		if first == nil {
			first = result
		}
		if !proto.Equal(first, result) {
			t.Fatal("retries returned different result")
		}
	}
	p, g := gemProfile(t, id)
	if p.GetCoin() != 9990000 || gemCount(g, "MineItem_019") != 1 || gemCount(g, "MineItem_081") != 1990 {
		t.Fatal("concurrent retry duplicated crafting")
	}
	if _, err := s.gemMake(ctx, id, &pb.CSGemMake{GemId: proto.String("MineItem_029"), Num: proto.Int32(1)}, 100); err == nil {
		t.Fatal("timestamp collision accepted changed input")
	}
	if _, err := s.gemMake(ctx, id, request, 99); err == nil {
		t.Fatal("stale request accepted")
	}
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		for _, item := range g.Gems {
			if item.GetId() == "MineItem_081" {
				item.Num = proto.Int32(10)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	success := make(chan bool, 2)
	for i := int64(101); i <= 102; i++ {
		wg.Add(1)
		go func(stamp int64) {
			defer wg.Done()
			_, err := s.gemMake(ctx, id, request, stamp)
			success <- err == nil
		}(i)
	}
	wg.Wait()
	close(success)
	var count int
	for ok := range success {
		if ok {
			count++
		}
	}
	p, g = gemProfile(t, id)
	if count != 1 || gemCount(g, "MineItem_019") != 2 || gemCount(g, "MineItem_081") != 0 || p.GetCoin() != 9980000 {
		t.Fatal("concurrent distinct requests overspent limited inventory")
	}
}

func TestGemConcurrentReceipts(t *testing.T)      { gemConcurrentCheck(t, false) }
func TestGemMySQLConcurrentReceipts(t *testing.T) { gemConcurrentCheck(t, true) }

func TestGemIllegalAndRollback(t *testing.T) {
	s, id, _ := gemFixture(t, false)
	ctx := context.Background()
	var before db.Profile
	db.GetIns().First(&before, id)
	for _, request := range []*pb.CSGemMake{{GemId: proto.String("MineItem_019"), Num: proto.Int32(-1)}, {GemId: proto.String("MineItem_019"), Num: proto.Int32(1001)}, {GemId: proto.String("MineItem_020"), Num: proto.Int32(1)}, {GemId: proto.String("foreign"), Num: proto.Int32(1)}, {GemId: proto.String("MineItem_079"), Num: proto.Int32(1)}} {
		if _, err := s.gemMake(ctx, id, request, 100); err == nil {
			t.Fatalf("illegal recipe accepted %v", request)
		}
	}
	s.table("MineSkill", "MineItemID", "MineItem_019")["MakingExp"] = "-1"
	if _, err := s.gemMake(ctx, id, &pb.CSGemMake{GemId: proto.String("MineItem_019"), Num: proto.Int32(1)}, 100); err == nil {
		t.Fatal("bad experience accepted")
	}
	var after db.Profile
	db.GetIns().First(&after, id)
	if *before.Game != *after.Game || *before.Player != *after.Player {
		t.Fatal("late failure did not roll back materials")
	}
	var count int64
	db.GetIns().Model(&db.GemMutationReceipt{}).Where("player_id = ?", id).Count(&count)
	if count != 0 {
		t.Fatal("failed request wrote receipt")
	}
	if _, err := s.gemMount(ctx, id, &pb.CSGemMount{Guid: proto.String("gem-test-equip"), GemId: proto.String("MineItem_013"), Pos: proto.Int32(0)}, 200); err == nil {
		t.Fatal("crystal mounted as gem")
	}
	if _, err := s.gemMount(ctx, id, &pb.CSGemMount{Guid: proto.String("foreign"), GemId: proto.String("MineItem_019"), Pos: proto.Int32(0)}, 200); err == nil {
		t.Fatal("foreign equipment mounted")
	}
	if err := s.equipmentGemProperties(&pb.Equipment{EquipId: proto.String("equip_037"), Gems: []string{"MineItem_029"}}, map[string]float64{}); err == nil {
		t.Fatal("wrong color property accepted")
	}
	if err := s.equipmentGemProperties(&pb.Equipment{EquipId: proto.String("equip_001"), Gems: []string{"MineItem_019"}}, map[string]float64{}); err == nil {
		t.Fatal("holeless equipment property accepted")
	}
}

func TestGemRefineRestartAndRulePersistence(t *testing.T) {
	s, id, path := gemFixture(t, false)
	ctx := context.Background()
	request := &pb.CSMineRefine{MineId: proto.String("MineItem_007"), Num: proto.Int32(100)}
	responses, err := s.gemRefine(ctx, id, request, 100)
	if err != nil {
		t.Fatal(err)
	}
	result := responses[0].(*pb.SCMineRefine)
	if result.GetLevel() != 2 || result.GetExp() != 0 || result.GetAdd() != 1000 || result.Cost[0].GetNum() != 100 {
		t.Fatalf("refine experience %v", result)
	}
	var magic, powder, crystal int32
	for _, item := range result.Res.Simps {
		switch item.GetId() {
		case "MineItem_081", "MineItem_082":
			magic += item.GetNum()
		case "MineItem_084":
			powder += item.GetNum()
		case "MineItem_013", "MineItem_014", "MineItem_015", "MineItem_016", "MineItem_017", "MineItem_018":
			crystal += item.GetNum()
		default:
			t.Fatalf("unexpected ore result %v", item)
		}
	}
	if magic != 20 || powder < 20 || powder > 40 || crystal != 20 {
		t.Fatalf("original/local reward rule magic=%d powder=%d crystal=%d", magic, powder, crystal)
	}
	connection, _ := db.GetIns().DB()
	connection.Close()
	if err = db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.GemRule{}).Where("id = ?", 1).Update("crystal_per_batch", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.InitGems(ctx); err != nil {
		t.Fatal(err)
	}
	retry, err := s.gemRefine(ctx, id, request, 100)
	if err != nil || !proto.Equal(result, retry[0]) {
		t.Fatalf("restart rolled random rewards err=%v", err)
	}
	level, exp, err := s.GemCraftLevel(ctx, id)
	if err != nil || level != 2 || exp != 0 {
		t.Fatalf("lost craft level %d/%d/%v", level, exp, err)
	}
	var rule db.GemRule
	db.GetIns().Take(&rule, 1)
	if rule.CrystalPerBatch != 2 {
		t.Fatal("restart reset SQL operations rule")
	}
	_, g := gemProfile(t, id)
	if gemCount(g, "MineItem_007") != 1900 {
		t.Fatal("restart retry charged ore twice")
	}
	if _, err = s.gemRefine(ctx, id, &pb.CSMineRefine{MineId: proto.String("MineItem_007"), Num: proto.Int32(6)}, 101); err == nil {
		t.Fatal("non-batch ore refine accepted")
	}
}

func TestGemCombineAndSixProperties(t *testing.T) {
	s, id, _ := gemFixture(t, false)
	ctx := context.Background()
	if err := db.GetIns().Create(&db.GemCraftState{PlayerID: id, Level: 10}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		for key, n := range map[string]int32{"MineItem_019": 4, "MineItem_029": 1, "MineItem_079": 6} {
			g.Gems = append(g.Gems, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(n)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	response, err := s.gemCombineThree(ctx, id, &pb.CSGemCombineByThree{GemId: proto.String("MineItem_020")}, 100)
	if err != nil || response[0].(*pb.SCGemCombineByThree).Cost[0].GetId() != "MineItem_019" {
		t.Fatalf("target-id combine err=%v response=%v", err, response)
	}
	if _, err = s.gemCombineTwo(ctx, id, &pb.CSGemCombineByTwo{GemId: []string{"MineItem_019", "MineItem_029", "MineItem_039"}}, 200); err != nil {
		t.Fatal(err)
	}
	if _, err = s.gemCombineTwo(ctx, id, &pb.CSGemCombineByTwo{GemId: []string{"MineItem_019", "MineItem_019", "MineItem_039"}}, 201); err == nil {
		t.Fatal("same-color transform accepted")
	}
	if _, err = s.gemCombineSix(ctx, id, &pb.CSGemCombineBySix{GemId: proto.String("MineItem_080")}, 300); err != nil {
		t.Fatal(err)
	}
	_, g := gemProfile(t, id)
	if gemCount(g, "MineItem_019") != 0 || gemCount(g, "MineItem_020") != 1 || gemCount(g, "MineItem_039") != 1 || gemCount(g, "MineItem_079") != 0 || gemCount(g, "MineItem_080") != 1 {
		t.Fatal("combine inventory failed")
	}
	values := map[string]float64{}
	if err = s.equipmentGemProperties(&pb.Equipment{EquipId: proto.String("equip_037"), Gems: []string{"MineItem_080", "MineItem_059", "MineItem_039", "MineItem_029"}}, values); err != nil {
		t.Fatal(err)
	}
	if values["att"] != 1200 || values["hp"] != 5320 || values["pdef"] != 420 || values["mdef"] != 360 || values["cri"] != 2.4 || values["antiCri"] != 1.8 {
		t.Fatalf("six raw attribute fields %v", values)
	}
}

// TestGemBattlePercentOrderMatchesOriginalClient 宝石在英雄百分比乘算之后累加，料理仍在基础层。
func TestGemBattlePercentOrderMatchesOriginalClient(t *testing.T) {
	s, id, _ := gemFixture(t, false)
	ctx := context.Background()
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				w.Breached = &pb.Breached{Level: proto.Int32(2)}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, g := gemProfile(t, id)
	before, err := s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"MineItem_019", "MineItem_029"} {
		if _, err = s.gemMake(ctx, id, &pb.CSGemMake{GemId: proto.String(key), Num: proto.Int32(1)}, int64(100+i)); err != nil {
			t.Fatal(err)
		}
	}
	for i, key := range []string{"MineItem_019", "MineItem_029"} {
		pos := int32(0)
		if i == 1 {
			pos = 3
		}
		if _, err = s.gemMount(ctx, id, &pb.CSGemMount{Guid: proto.String("gem-test-equip"), GemId: proto.String(key), Pos: proto.Int32(pos)}, int64(200+i)); err != nil {
			t.Fatal(err)
		}
	}
	p, g = gemProfile(t, id)
	after, err := s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	var (
		base  = before.GetFormation()[0]
		actor = after.GetFormation()[0]
	)
	if delta := actor.GetProp().GetAtt() - base.GetProp().GetAtt(); math.Abs(delta-200) > 1e-8 {
		t.Fatalf("APK adds gems after percentage multiplication; attack gain=%v, expected=200", delta)
	}
	if delta := actor.GetProp().GetHp() - base.GetProp().GetHp(); math.Abs(delta-760) > 1e-8 {
		t.Fatalf("APK adds gems after percentage multiplication; HP gain=%v, expected=760", delta)
	}
	if actor.GetInst().GetCurHp() != actor.GetProp().GetHp() {
		t.Fatal("battle starts without the gem HP represented by its final properties")
	}
}
