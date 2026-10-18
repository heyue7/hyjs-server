package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestP13UnionFightMatchAdvanceIsolatedNativeClock 仅供明确隔离专库的原帧测试推进报名截止，不改系统时钟。
func TestP13UnionFightMatchAdvanceIsolatedNativeClock(t *testing.T) {
	if os.Getenv("HYJS_P13_MATCH_NATIVE_ADVANCE") != "1" {
		t.Skip("仅原帧隔离日历测试启用")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_") {
		t.Fatal("P13 match native isolation guard")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}, s: &GameService{}}
	f.open(t)
	t.Cleanup(func() { sql, _ := db.GetIns().DB(); sql.Close() })
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	start, _, _, _ := p13UFClock(time.Now())
	if start.Weekday() != time.Monday && start.Weekday() != time.Wednesday && start.Weekday() != time.Friday {
		t.Fatal("原报名日要求一三五")
	}
	if err := f.s.TickP13UnionFight(f.ctx, start.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightMatch{}).Count(&count).Error; err != nil || count != 4 {
		t.Fatal("6真实公会两组两轮预览", count, err)
	}
	var match db.P13UnionFightMatch
	if err := db.GetIns().Order("id").First(&match).Error; err != nil {
		t.Fatal(err)
	}
	var chairman db.P13GuildMember
	if err := db.GetIns().First(&chairman, "guild_id=? AND role=2", match.LeftGuildID).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, chairman.PlayerID, func(p *pb.Player, g *model.GameState) error {
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("item_302"), Num: proto.Int32(7)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, round := range []int32{1, 2} {
		p13UFCall(t, f, chairman.PlayerID, &pb.CSUFSetFightItems{FightIndex: proto.Int32(round), ItemIds: []string{"item_302"}}, 8000+int64(round), start.AddDate(0, 0, 1).Add(time.Hour))
	}
}

// TestP13UnionFight 匹配真实20个公会冻结报名，原比例逐组核对，重启/并发不重配且没有胜负。
func TestP13UnionFightMatchActual20GuildRatioFrozenPreviewConcurrencyAndRecovery(t *testing.T) {
	f, ids := p13GuildSetup(t)
	staticRows := 0
	for _, name := range []string{"UnionFightBaseCamp", "UnionFightBuff", "UnionFightCariot", "UnionFightFactory", "UnionFightGate", "UnionFightGuard", "UnionFightItem", "UnionFightNpc", "UnionFightRankReward", "UnionFightRankSpecialReward", "UnionFightResource", "UnionFightRevivePoint", "UnionFightTalk", "UnionFightTower", "UnionFightTransfer"} {
		staticRows += len(f.s.Tables[name])
	}
	if staticRows != 211 {
		t.Fatal("incomplete original guild battlefield table freeze", staticRows)
	}
	now := p13UFMondays()
	p13GuildOpen(t, true)
	p13UFOpen(t)
	for _, table := range []any{&db.P13UnionFightMatch{}, &db.P13UnionFightGroup{}, &db.P13UnionFightSchedule{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("match_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p13-match-%d-%d", i, time.Now().UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("匹配玩家%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(30)
			p.Diamond = proto.Int64(1000)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	var guilds []string
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("真实配对%02d", i))}, 1, now)
		guild := p13GuildID(t, f, id, now)
		guilds = append(guilds, guild)
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
			t.Fatal(err)
		}
		p13UFCall(t, f, id, &pb.CSUFUnionApply{}, 2, now)
		p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, 3, now)
	}
	next := now.Add(12 * time.Hour)
	if err := f.s.TickP13UnionFight(f.ctx, next); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightMatch{}).Count(&count)
	if count != 0 {
		t.Fatal("invented unconfigured calendar")
	}
	first := next.Add(20 * time.Hour).UnixMilli()
	schedule := db.P13UnionFightSchedule{Day: "2026-10-20", FirstStartAt: first, SecondStartAt: first + 600000, DurationMillis: 300000, Source: "明确隔离fixture日历；原运营时刻未知"}
	if err := db.GetIns().Create(&schedule).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.TickP13UnionFight(f.ctx, next.Add(time.Minute)) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	counts := map[string]int{}
	var groups []db.P13UnionFightGroup
	db.GetIns().Where("week_start=?", "2026-10-19").Find(&groups)
	for _, group := range groups {
		counts[group.Group]++
	}
	if counts["A"] != 3 || counts["B"] != 6 || counts["C"] != 6 || counts["D"] != 5 {
		t.Fatal("original four ratio rows", counts)
	}
	var rows []db.P13UnionFightMatch
	db.GetIns().Order("id").Find(&rows)
	if len(rows) != 18 {
		t.Fatal("20 guild pairs two rounds; odd ones remain unresolved", len(rows))
	}
	pairedGuild := rows[0].LeftGuildID
	viewer := int64(0)
	for i, guild := range guilds {
		if guild == pairedGuild {
			viewer = ids[i]
		}
	}
	before := rows[0]
	if viewer == 0 {
		t.Fatal("no actual matched viewer")
	}
	query, err := f.s.p13UFQuery(f.ctx, viewer, &pb.CSUFInfoPreView{}, next.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	preview := query[0].(*pb.SCUFInfoPreView)
	if preview.GetIsUnionApplied() != 1 || preview.FirstFightInfo.GetOurUnion().GetUnionId() != pairedGuild || preview.FirstFightInfo.GetEnemyUnion().GetUnionId() == pairedGuild || preview.SecondFightInfo == nil || preview.FirstFightInfo.FightResult != nil || preview.FirstFightInfo.GetIsRoundBye() != 0 {
		t.Fatal("actual original pre-match metadata", preview)
	}
	encoded, err := proto.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip := &pb.SCUFInfoPreView{}
	if proto.Unmarshal(encoded, roundtrip) != nil || !proto.Equal(roundtrip, preview) {
		t.Fatal("original binary preview")
	}
	query, err = f.s.p13UFQuery(f.ctx, viewer, &pb.CSUFCurrentStageInfo{}, next.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stage := query[0].(*pb.SCUFCurrentStageInfo)
	if stage.GetStage() != 3 || len(stage.FightTimeShow) != 2 || stage.FightTimeShow[0] != schedule.FirstStartAt || stage.FightTimeShow[1] != schedule.SecondStartAt+schedule.DurationMillis {
		t.Fatal("published preparation and original two range values", stage)
	}
	// 已配对名单保留原报名阵容，不用现在的角色改装或重复worker更换对手。
	if err = updateProfile(f.ctx, viewer, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(99); return nil }); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFight(f.ctx, next.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var after db.P13UnionFightMatch
	db.GetIns().First(&after, "id=?", before.ID)
	if string(after.Roster) != string(before.Roster) || after.SnapshotHash != before.SnapshotHash || after.LeftGuildID != before.LeftGuildID || after.RightGuildID != before.RightGuildID {
		t.Fatal("restart changed matched roster")
	}
	db.GetIns().Model(&db.P13UnionFightMatch{}).Count(&count)
	if count != 18 {
		t.Fatal("duplicated matched instances", count)
	}
	// 原每轮配置只选择真实背包种类，首场占用5的UI规则约束次场，配置不会预扣库存。
	if err = updateProfile(f.ctx, viewer, func(p *pb.Player, g *model.GameState) error {
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("item_302"), Num: proto.Int32(7)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	selection := &pb.CSUFSetFightItems{FightIndex: proto.Int32(1), ItemIds: []string{"item_302"}}
	if _, err = f.s.p13UFMutate(f.ctx, viewer, selection, 49, next.Add(time.Minute)); err == nil {
		t.Fatal("round item choice before original matching window ended")
	}
	itemErrs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.p13UFMutate(f.ctx, viewer, selection, 50, next.Add(time.Hour))
			itemErrs <- err
		}()
	}
	wg.Wait()
	close(itemErrs)
	for err := range itemErrs {
		if err != nil {
			t.Fatal(err)
		}
	}
	second := &pb.CSUFSetFightItems{FightIndex: proto.Int32(2), ItemIds: []string{"item_302"}}
	p13UFCall(t, f, viewer, second, 51, next.Add(time.Hour))
	if _, err = f.s.p13UFMutate(f.ctx, viewer, &pb.CSUFSetFightItems{FightIndex: proto.Int32(2), ItemIds: []string{"item_303"}}, 52, next.Add(time.Hour)); err == nil {
		t.Fatal("unowned original round item accepted")
	}
	query, err = f.s.p13UFQuery(f.ctx, viewer, &pb.CSUFInfoPreView{}, next.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	loaded := query[0].(*pb.SCUFInfoPreView)
	if len(loaded.FirstFightInfo.ItemIds) != 1 || loaded.FirstFightInfo.ItemIds[0] != "item_302" || len(loaded.SecondFightInfo.ItemIds) != 1 || loaded.SecondFightInfo.ItemIds[0] != "item_302" {
		t.Fatal("original per-player two-round choice", loaded)
	}
	var saved db.Profile
	db.GetIns().First(&saved, viewer)
	_, inventory, err := decodeProfile(&saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, stack := range inventory.Items {
		if stack.GetId() == "item_302" && stack.GetNum() != 7 {
			t.Fatal("configuration consumed original stock")
		}
	}
	if err = updateProfile(f.ctx, viewer, func(p *pb.Player, g *model.GameState) error {
		for _, stack := range g.Items {
			if stack.GetId() == "item_302" {
				stack.Num = proto.Int32(5)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFMutate(f.ctx, viewer, second, 53, next.Add(time.Hour)); err == nil {
		t.Fatal("second round did not subtract first original max5")
	}
	if _, err = f.s.p13UFMutate(f.ctx, viewer, selection, 54, time.UnixMilli(schedule.FirstStartAt+1)); err == nil {
		t.Fatal("round items changed after real start")
	}
	p13UFCall(t, f, viewer, selection, 50, time.UnixMilli(schedule.FirstStartAt+1))
	if err = db.GetIns().Model(&db.P13UnionFightMatch{}).Where("id=?", before.ID).Update("snapshot_hash", "isolated-corruption").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFQuery(f.ctx, viewer, &pb.CSUFInfoPreView{}, next.Add(time.Hour)); err == nil {
		t.Fatal("corrupt matched roster exposed")
	}
}
