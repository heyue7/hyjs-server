package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p13UFOpen(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Updates(map[string]any{"enrollment_enabled": true, "match_enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
}
func p13UFCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	messages, err := f.s.p13UFMutate(f.ctx, id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	out := proto.Clone(messages[0])
	proto.Reset(out)
	if err = proto.Unmarshal(raw, out); err != nil || !proto.Equal(out, messages[0]) {
		t.Fatal("original enrollment response", err)
	}
	return messages[0]
}
func p13UFMondays() time.Time {
	return time.Date(2026, 10, 19, 12, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
}

// TestP13UnionFight 核对完整原25首发边界、宣战名册快照、重试、截止与真实阵容恢复。
func TestP13UnionFightOriginalDeclarationRoster25AndDeadlineFrozenRestart(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := p13UFMondays()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("原宣战名册")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	if _, err := f.s.p13UFMutate(f.ctx, ids[0], &pb.CSUFUnionApply{}, 2, now); err == nil {
		t.Fatal("closed enrollment declared")
	}
	p13UFOpen(t)
	if _, err := f.s.p13UFMutate(f.ctx, ids[0], &pb.CSUFUnionApply{}, 2, now); err == nil {
		t.Fatal("level1 declaration")
	}
	db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2)
	// 名册采用真实注册/角色创建与原入会请求构建，覆盖25+替补和宣战后入会。
	for i := 0; i < 24; i++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p13-uf-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("原报名%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	joined := append([]int64{ids[0]}, ids[5:]...)
	joined = append(joined, ids[1])
	for i, id := range joined[1:] {
		p13Join(t, f, ids[0], id, guild, int64(100+i), now)
	}
	if _, err := f.s.p13UFMutate(f.ctx, ids[1], &pb.CSUFUnionApply{}, 2, now); err == nil {
		t.Fatal("member declaration")
	}
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 200, now)
	p13UFCall(t, f, ids[1], &pb.CSUFUnionApply{}, 201, now)
	p13UFCall(t, f, ids[0], &pb.CSUFUnionApply{}, 202, now)
	var count int64
	db.GetIns().Model(&db.P13UnionFightEntry{}).Where("guild_id=?", guild).Count(&count)
	if count != 1 {
		t.Fatal("duplicate declaration", count)
	}
	p13Join(t, f, ids[0], ids[2], guild, 203, now)
	if _, err := f.s.p13UFMutate(f.ctx, ids[2], &pb.CSUFPlayerApply{}, 204, now); err == nil {
		t.Fatal("postdeclare join acquired recorded qualification")
	}
	for i, id := range joined {
		reply := p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, int64(300+i), now.Add(time.Duration(i)*time.Millisecond)).(*pb.SCUFPlayerApply)
		want := int32(1)
		if i >= 25 {
			want = 0
		}
		if reply.GetIsFirst() != want {
			t.Fatal("original first25 boundary", i, reply)
		}
	}
	p13UFCall(t, f, joined[0], &pb.CSUFPlayerApply{}, 300, now)
	p13UFCall(t, f, joined[1], &pb.CSUFSetAutoFight{IsAuto: proto.Int32(1)}, 400, now)
	messages, err := f.s.p13UFQuery(f.ctx, ids[0], &pb.CSUFAppliedPlayer{}, now)
	if err != nil {
		t.Fatal(err)
	}
	reply := messages[0].(*pb.SCUFAppliedPlayer)
	if len(reply.Firsts) != 25 || len(reply.BackUps) != 1 || reply.Firsts[1].GetIsAutoFight() != 1 {
		t.Fatal("original roster lists", reply)
	}
	// 首发先腾出一位再提升替补，原剩余10次逐实际变更递减。
	if _, err = f.s.p13UFMutate(f.ctx, ids[1], &pb.CSUFAdjustFirstOn{OptPlayerId: proto.Int64(ids[0]), OptType: proto.Int32(0)}, 410, now); err == nil {
		t.Fatal("vice adjusted original chairman")
	}
	if _, err = f.s.p13UFMutate(f.ctx, joined[1], &pb.CSUFAdjustFirstOn{OptPlayerId: proto.Int64(joined[2]), OptType: proto.Int32(0)}, 410, now); err == nil {
		t.Fatal("ordinary adjusted first lineup")
	}
	drop := &pb.CSUFAdjustFirstOn{OptPlayerId: proto.Int64(joined[1]), OptType: proto.Int32(0)}
	left := p13UFCall(t, f, ids[0], drop, 411, now).(*pb.SCUFAdjustFirstOn)
	if left.GetAdjustTimes() != 9 {
		t.Fatal("first actual remaining adjustment", left)
	}
	p13UFCall(t, f, ids[0], drop, 411, now)
	for i := 0; i < 9; i++ {
		mode := int32(1)
		if i%2 == 1 {
			mode = 0
		}
		reply := p13UFCall(t, f, ids[0], &pb.CSUFAdjustFirstOn{OptPlayerId: proto.Int64(ids[1]), OptType: proto.Int32(mode)}, int64(412+i), now).(*pb.SCUFAdjustFirstOn)
		if reply.GetAdjustTimes() != int32(8-i) {
			t.Fatal("original10 adjustment cap", i, reply)
		}
	}
	if _, err = f.s.p13UFMutate(f.ctx, ids[0], &pb.CSUFAdjustFirstOn{OptPlayerId: proto.Int64(ids[1]), OptType: proto.Int32(0)}, 430, now); err == nil {
		t.Fatal("original first adjustment cap exceeded")
	}
	if err = updateProfile(f.ctx, joined[0], func(p *pb.Player, g *model.GameState) error {
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("item_302"), Num: proto.Int32(7)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p13UFCall(t, f, joined[0], &pb.CSUFSetFightItems{FightIndex: proto.Int32(0), ItemIds: []string{"item_302"}}, 440, now)
	for i := 1; i <= 3; i++ {
		reply := p13UFCall(t, f, joined[0], &pb.CSUFUpdateBattlePlayer{}, int64(450+i), now).(*pb.SCUFUpdateBattlePlayer)
		if reply.GetBattleArrayAdjustTime() != int32(i) {
			t.Fatal("original used count", i, reply)
		}
		p13UFCall(t, f, joined[0], &pb.CSUFUpdateBattlePlayer{}, int64(450+i), now)
	}
	if _, err = f.s.p13UFMutate(f.ctx, joined[0], &pb.CSUFUpdateBattlePlayer{}, 454, now); err == nil {
		t.Fatal("original3 updates exceeded")
	}
	p, game := p10LuckyProfile(t, joined[0])
	_ = p
	owned := int32(0)
	for _, item := range game.Items {
		if item.GetId() == "item_302" {
			owned = item.GetNum()
		}
	}
	if owned != 7 {
		t.Fatal("configuration consumed inventory")
	}
	stateMessages, err := f.s.p13UFQuery(f.ctx, joined[0], &pb.CSUFCurrentStageInfo{}, now)
	if err != nil {
		t.Fatal(err)
	}
	stateInfo := stateMessages[0].(*pb.SCUFCurrentStageInfo)
	if stateInfo.GetBattleArrayAdjustTime() != 3 || stateInfo.GetAdjustTimes() != 0 || len(stateInfo.ItemIds) != 1 || stateInfo.ItemIds[0] != "item_302" {
		t.Fatal("actual counters and original chosen items", stateInfo)
	}
	var frozen db.P13UnionFightFighter
	db.GetIns().First(&frozen, "guild_id=? AND player_id=?", guild, joined[0])
	party := &pb.BattlePlayer{}
	if proto.Unmarshal(frozen.Battle, party) != nil || len(party.Formation) == 0 {
		t.Fatal("actual uploaded formation")
	}
	before := append([]byte(nil), frozen.Battle...)
	updateProfile(f.ctx, joined[0], func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(99); return nil })
	tomorrow := now.Add(12 * time.Hour)
	if err = db.GetIns().Model(&db.P13UnionFightFighter{}).Where("guild_id=? AND player_id=?", guild, joined[0]).Update("snapshot_hash", "corrupted-isolated-fixture").Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFight(f.ctx, tomorrow); err == nil {
		t.Fatal("corrupted enrollment snapshot frozen")
	}
	if err = db.GetIns().Model(&db.P13UnionFightFighter{}).Where("guild_id=? AND player_id=?", guild, joined[0]).Update("snapshot_hash", p6Digest(before)).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFight(f.ctx, tomorrow); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFMutate(f.ctx, ids[2], &pb.CSUFPlayerApply{}, 500, tomorrow); err == nil {
		t.Fatal("cutoff registration")
	}
	if err = f.s.TickP13UnionFight(f.ctx, tomorrow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var entry db.P13UnionFightEntry
	db.GetIns().First(&entry, "guild_id=?", guild)
	if entry.FrozenAt == 0 || entry.EligibleCount != 26 || len(entry.FrozenTables) == 0 || p6Digest(entry.FrozenTables) != entry.ConfigurationHash || entry.EngineVersion == "" {
		t.Fatal("frozen eligible roster", entry)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&frozen, "guild_id=? AND player_id=?", guild, joined[0])
	if string(frozen.Battle) != string(before) || p6Digest(frozen.Battle) != frozen.SnapshotHash {
		t.Fatal("current changed character replaced uploaded battle")
	}
	p13UFCall(t, f, joined[0], &pb.CSUFPlayerApply{}, 300, tomorrow)
	if _, err = f.s.p13UFMutate(f.ctx, joined[0], &pb.CSUFSetAutoFight{IsAuto: proto.Int32(0)}, 600, tomorrow); err == nil {
		t.Fatal("postfreeze changed setting")
	}
}

// TestP13UnionFight 原自动宣战只在一三五凌晨前5分钟执行，手动并发不双声明。
func TestP13UnionFightOriginalAutoWindowManualConcurrentAndEmptyFreeze(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := p13UFMondays()
	p13GuildOpen(t, true)
	p13UFOpen(t)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("自动原窗口")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2)
	p13UFCall(t, f, ids[0], &pb.CSUFAutoApplySet{IsAuto: proto.Int32(1)}, 2, now)
	if err := f.s.TickP13UnionFight(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightEntry{}).Where("guild_id=?", guild).Count(&count)
	if count != 0 {
		t.Fatal("auto outside original window")
	}
	midnight := now.Add(-12 * time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				errs <- f.s.TickP13UnionFight(f.ctx, midnight.Add(time.Minute))
			} else {
				_, err := f.s.p13UFMutate(f.ctx, ids[0], &pb.CSUFUnionApply{}, 3, midnight.Add(time.Minute))
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	db.GetIns().Model(&db.P13UnionFightEntry{}).Where("guild_id=?", guild).Count(&count)
	if count != 1 {
		t.Fatal("auto/manual declaration duplicated")
	}
	if err := f.s.TickP13UnionFight(f.ctx, midnight.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var entry db.P13UnionFightEntry
	db.GetIns().First(&entry, "guild_id=?", guild)
	if entry.FrozenAt == 0 || entry.EligibleCount != 0 {
		t.Fatal("empty guild falsely entered matching", entry)
	}
	// 时钟按本服区时解释原星期，跨UTC日期仍是同一个报名日。
	_, day, stage, _ := p13UFClock(midnight.UTC())
	if day != "2026-10-19" || stage != 1 {
		t.Fatal(day, stage)
	}
}
