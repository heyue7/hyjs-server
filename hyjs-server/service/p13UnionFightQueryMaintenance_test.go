package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFQueryActiveWarsDoNotRunSettlement(t *testing.T) {
	f, ids, first, clock, _ := p13UFFieldSetup(t)
	clock.Store(first + 1000)
	now := time.UnixMilli(clock.Load())
	var reads []string
	name := "p13_query_no_global_settlement"
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) { reads = append(reads, tx.Statement.SQL.String()) }); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(name)
	for _, req := range []proto.Message{&pb.CSUFInfoPreView{}, &pb.CSUFCurrentStageInfo{}} {
		if _, err := f.s.p13UFQuery(f.ctx, ids[0], req, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range reads {
		lower := strings.ToLower(sql)
		if strings.Contains(lower, "p13_union_fight_settlement_results") {
			// Preview needs the existing single-match Win/Loss state, never the
			// awards/proof payload or a global result/maintenance directory.
			from := strings.Index(lower, " from ")
			where := strings.Index(lower, " where ")
			if !strings.HasPrefix(lower, "select ") || from < 0 || where < from {
				t.Fatal("preview scanned unbounded result history", sql)
			}
			allowed := map[string]bool{"match_id": true, "result_kind": true, "winner_guild_id": true, "loser_guild_id": true}
			columns := strings.Split(lower[len("select "):from], ",")
			seen := map[string]bool{}
			for _, column := range columns {
				column = strings.Trim(strings.TrimSpace(column), "`\"")
				if !allowed[column] || seen[column] {
					t.Fatal("preview read non-state result payload", sql)
				}
				seen[column] = true
			}
			predicate := strings.NewReplacer("`", "", "\"", "", " ", "").Replace(lower[where+len(" where "):])
			predicate = strings.Split(strings.Split(predicate, "orderby")[0], "limit")[0]
			if len(seen) != len(allowed) || predicate != "match_id=?" {
				t.Fatal("preview result is not exact single-match state metadata", sql)
			}
			continue
		}
		if strings.Contains(lower, "p6_battle_records") || strings.Contains(lower, "p13_union_fight_settlement_") || strings.Contains(lower, "p13_union_fight_guard_battle_attempts") {
			t.Fatal("preview invoked settlement/native source", sql)
		}
	}
	var results int64
	if err := db.GetIns().Model(&db.P13UnionFightSettlementResult{}).Count(&results).Error; err != nil || results != 0 {
		t.Fatal("preview produced a result/award", results, err)
	}
}

func TestP13UFQueryDueDeadlineStillFreezesAndMatches(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	p13UFOpen(t)
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("match_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for len(ids) < 10 {
		ids = append(ids, p13GiftAdditionalPlayer(t, f, len(ids)))
	}
	now := p13UFMondays()
	for i, id := range ids[:10] {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("查询截止%02d", i))}, int64(700+i), now)
		guild := p13GuildID(t, f, id, now)
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
			t.Fatal(err)
		}
		p13UFCall(t, f, id, &pb.CSUFUnionApply{}, int64(710+i), now)
		p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, int64(720+i), now)
	}
	next := now.Add(12 * time.Hour)
	first := next.Add(20 * time.Hour).UnixMilli()
	if err := db.GetIns().Create(&db.P13UnionFightSchedule{Day: "2026-10-20", FirstStartAt: first, SecondStartAt: first + 600000, DurationMillis: 300000, Source: "isolated real query deadline/matcher regression"}).Error; err != nil {
		t.Fatal(err)
	}
	if due, err := f.s.p13UFQueryMaintenanceDue(f.ctx, now); err != nil || due {
		t.Fatal("no due work before actual cutoff", due, err)
	}
	if _, err := f.s.p13UFQuery(f.ctx, ids[0], &pb.CSUFInfoPreView{}, next); err != nil {
		t.Fatal(err)
	}
	var entries []db.P13UnionFightEntry
	if err := db.GetIns().Where("day=?", p13UFDay(now)).Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatal("actual declarations", len(entries))
	}
	for _, e := range entries {
		if e.FrozenAt == 0 || !e.MatchProcessed || len(e.FrozenTables) == 0 {
			t.Fatal("query lost deadline freeze/match", e.GuildID)
		}
	}
	var matches int64
	if err := db.GetIns().Model(&db.P13UnionFightMatch{}).Where("day=?", "2026-10-20").Count(&matches).Error; err != nil || matches != 8 {
		t.Fatal("real two rounds not matched", matches, err)
	}
	for _, round := range []int32{1, 2} {
		var count int64
		if err := db.GetIns().Model(&db.P13UnionFightMatch{}).Where("day=? AND round=?", "2026-10-20", round).Count(&count).Error; err != nil || count != 4 {
			t.Fatal("original grouping must yield four actual matches per round", round, count, err)
		}
	}
	if due, err := f.s.p13UFQueryMaintenanceDue(f.ctx, next); err != nil || due {
		t.Fatal("completed admission still treated as due", due, err)
	}
}

func TestP13UFSettlementWorkerActiveWarsReadOnlyTerminalMetadata(t *testing.T) {
	f, _, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "enabled": true, "version": "active-war-metadata-regression", "source": "isolated prepublished settlement; active wars must not audit bodies", "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "published_at": clock.Load() - 1}).Error; err != nil {
		t.Fatal(err)
	}
	// Prepublish using the production worker before measuring active candidates.
	if err := f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 1000)
	var reads []string
	name := "p13_active_worker_metadata"
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) { reads = append(reads, tx.Statement.SQL.String()) }); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(name)
	if err := f.s.TickP13UnionFightSettlement(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	foundTerminal := false
	for _, sql := range reads {
		lower := strings.ToLower(sql)
		if strings.Contains(lower, "p13_union_fight_guard_battle_attempts") {
			foundTerminal = true
			if strings.Contains(lower, "select *") {
				t.Fatal("terminal gate read full attempt", sql)
			}
		}
		if strings.Contains(lower, "p6_battle_records") || strings.Contains(lower, "p13_union_fight_field_players") || strings.Contains(lower, "p13_union_fight_player_battle_attempts") {
			t.Fatal("active worker performed full combat/actor audit", sql)
		}
	}
	if !foundTerminal {
		t.Fatal("fixture did not enter active terminal candidate gate")
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightSettlementResult{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("active worker emitted premature award", count, err)
	}
}

func TestP13UFQueryOriginalAutoWindowStillDeclares(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	p13UFOpen(t)
	now := p13UFMondays().Add(-12*time.Hour + time.Minute)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("查询自动宣战")}, 731, now)
	guild := p13GuildID(t, f, ids[0], now)
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"level": 2, "union_fight_auto_apply": true}).Error; err != nil {
		t.Fatal(err)
	}
	if due, err := f.s.p13UFQueryMaintenanceDue(f.ctx, now); err != nil || !due {
		t.Fatal("original automatic declaration not recognized", due, err)
	}
	if _, err := f.s.p13UFQuery(f.ctx, ids[0], &pb.CSUFCurrentStageInfo{}, now); err != nil {
		t.Fatal(err)
	}
	var entry db.P13UnionFightEntry
	if err := db.GetIns().First(&entry, "guild_id=? AND day=?", guild, p13UFDay(now)).Error; err != nil || entry.AppliedAt != now.UnixMilli() {
		t.Fatal("actual auto declaration missing", err)
	}
	if due, err := f.s.p13UFQueryMaintenanceDue(f.ctx, now); err != nil || due {
		t.Fatal("declared guild causes repeated maintenance", due, err)
	}
}

func TestP13UFSettlementMaintenanceDoesNotAdvanceTerminalCursor(t *testing.T) {
	f, _, _, clock, _ := p13UFFieldSetup(t)
	p13UFSettlementCursor.Lock()
	beforeDB, beforeEnd, beforeID := p13UFSettlementCursor.DB, p13UFSettlementCursor.EndAt, p13UFSettlementCursor.ID
	p13UFSettlementCursor.Unlock()
	if err := f.s.TickP13UnionFightSettlementMaintenance(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	p13UFSettlementCursor.Lock()
	defer p13UFSettlementCursor.Unlock()
	if p13UFSettlementCursor.DB != beforeDB || p13UFSettlementCursor.EndAt != beforeEnd || p13UFSettlementCursor.ID != beforeID {
		t.Fatal("slow maintenance competed for terminal cursor")
	}
}

func TestP13UFSettlementQueryTargetsOnlyLatestEnteredMatch(t *testing.T) {
	f, id, _, clock, match := p13UFPlayerBattle172ActualSetup(t)
	// The production setup has real entered actors. No forged settled result is
	// supplied: an active match must remain unavailable, without auditing others.
	var reads []string
	name := "p13_result_target_only"
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) { reads = append(reads, tx.Statement.SQL.String()) }); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(name)
	if _, err := f.s.p13UFSettlementQuery(f.ctx, id, &pb.CSUFFightResult{}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("active actual match fabricated a final result")
	}
	if _, err := f.s.p13UFSettlementQuery(f.ctx, id, &pb.CSUFUnionResultInfo{}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	for _, sql := range reads {
		lower := strings.ToLower(sql)
		if strings.Contains(lower, "p6_battle_records") {
			t.Fatal("result query evaluated native history", sql)
		}
		if strings.Contains(lower, "p13_union_fight_settlement_results") && !strings.Contains(lower, "where") && !strings.Contains(lower, " on ") {
			t.Fatal("result query scanned global result history", sql)
		}
		if strings.Contains(lower, "m.end_at>") {
			t.Fatal("request advanced global terminal cursor", sql)
		}
	}
	var actors []db.P13UnionFightFieldPlayer
	if err := db.GetIns().Where("player_id=?", id).Find(&actors).Error; err != nil || len(actors) != 1 || actors[0].MatchID != match.ID {
		t.Fatal("fixture latest entered identity", err)
	}
}
