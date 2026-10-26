package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13FunctionalSetup(t *testing.T) (*p1Fixture, []int64) {
	t.Helper()
	// Explicit isolated p13 fixture; no production policy/source is touched.
	f, ids := p13GuildGiftSetup(t)
	if f.mysql.Database != "" && !strings.HasPrefix(f.mysql.Database, "hyjs_test_p13_") {
		t.Fatal("functional fixture requires isolated P13 database")
	}
	if f.mysql.Database == "" && (db.GetIns().Dialector.Name() != "sqlite" || !strings.HasSuffix(f.path, "/p13.db")) {
		t.Fatal("functional fixture requires its temporary P13 SQLite store")
	}
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var models []any
	shape := reflect.TypeOf(p13FunctionalDay{})
	for i := 0; i < shape.NumField(); i++ {
		models = append(models, reflect.New(shape.Field(i).Type.Elem()).Interface())
	}
	models = append(models, &db.P13UnionFightGuardBattlePolicy{}, &db.P13UnionFightPlayerBattlePolicy{}, &db.P13UnionFightFieldPolicy{}, &db.P13UnionFightItemPolicy{}, &db.P13UnionFightChatPolicy{}, &db.P13UnionFightSettlementPolicy{}, &db.P13UnionFightWeeklyPolicy{}, &db.P13UnionFightWeeklyOffer{}, &db.P13UnionFightMonthlyPolicy{}, &db.P13UnionFightMonthlyOffer{}, &db.P13UnionFightFieldPlayer{}, &db.P13UnionFightField{}, &db.P13UnionFightFighter{}, &db.P13UnionFightLoadout{}, &db.P13UnionFightMatch{}, &db.P13UnionFightEntry{}, &db.P13UnionFightGroup{}, &db.P13UnionFightSchedule{})
	models = append(models, &db.P13UnionFightGuardAuditOffer{}, &db.P13UnionFightBattleAuditOffer{}, &db.P13UnionFightPlayerBattleOffer{}, &db.P13UnionFightFactoryState{}, &db.P13UnionFightFactoryCycleOffer{}, &db.P13UnionFightFactoryCycleState{}, &db.P13UnionFightCarMotionOffer{}, &db.P13UnionFightCarMotionState{}, &db.P13UnionFightCarAttackOffer{}, &db.P13UnionFightCarAttackState{}, &db.P13UnionFightTowerState{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightGateItemOffer{}, &db.P13UnionFightGateState{}, &db.P13UnionFightCarItemOffer{}, &db.P13UnionFightSpeedItemOffer{}, &db.P13UnionFightHealAuraOffer{}, &db.P13UnionFightWarDrumOffer{}, &db.P13UnionFightResourceNode{}, &db.P13UnionFightTransportScope{}, &db.P13UnionFightRevivePointOffer{}, &db.P13UnionFightRevivePointState{}, &db.P13UnionFightShopOffer{}, &db.P13UnionFightShopStock{})
	models = append(models, &db.P13GuildPolicy{}, &db.P13UnionFightPolicy{})
	// Capture typed rows before clearing the isolated fixture. Direct model reads
	// preserve opaque Rules/PB bytes and all Player policy IDs, including 1/2.
	// Empty slices also record that a table had no prior rows.
	prior := make([]reflect.Value, len(models))
	for i, m := range models {
		prior[i] = reflect.New(reflect.SliceOf(reflect.TypeOf(m).Elem()))
		if err := db.GetIns().Find(prior[i].Interface()).Error; err != nil {
			t.Fatal("snapshot isolated functional fixture", err)
		}
	}
	t.Cleanup(func() {
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			for _, m := range models {
				if err := tx.Where("1=1").Delete(m).Error; err != nil {
					return err
				}
			}
			for i, rows := range prior {
				if rows.Elem().Len() > 0 {
					statement := &gorm.Statement{DB: tx}
					if err := statement.Parse(models[i]); err != nil {
						return err
					}
					// Maps preserve zero timestamp/default fields exactly; ordinary
					// struct Create would apply GORM's automatic creation values.
					values := make([]map[string]any, rows.Elem().Len())
					for n := range values {
						values[n] = map[string]any{}
						for _, field := range statement.Schema.Fields {
							if field.DBName != "" {
								value, _ := field.ValueOf(tx.Statement.Context, rows.Elem().Index(n))
								values[n][field.DBName] = value
							}
						}
					}
					// GORM 1.31's SQLite map batch triggered an out-of-range
					// index in its reflective slice writeback after Create.
					// Restore each small fixture row through supported map Create;
					// explicit zero fields and the enclosing rollback stay exact.
					for _, value := range values {
						if err := tx.Model(models[i]).Create(value).Error; err != nil {
							return err
						}
					}
				}
			}
			return nil
		}); err != nil {
			t.Error("restore complete prior functional fixture", err)
		}
	})
	clear := func(cleanup bool) {
		for _, m := range models {
			if err := db.GetIns().Where("1=1").Delete(m).Error; err != nil {
				if cleanup {
					t.Error(err)
				} else {
					t.Fatal(err)
				}
			}
		}
	}
	clear(false)
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, ids
}

func p13FunctionalScheduleFor(day time.Time, now time.Time) *db.P13UnionFightSchedule {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	d := day.In(zone)
	start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, zone)
	return &db.P13UnionFightSchedule{Day: start.Format("2006-01-02"), RegistrationDay: start.AddDate(0, 0, -1).Format("2006-01-02"), Enabled: true, Version: "isolated-functional-typed-v1", PublishedAt: now.UnixMilli(), EnrollmentStartAt: start.AddDate(0, 0, -1).Add(time.Hour).UnixMilli(), EnrollmentEndAt: start.Add(-time.Hour).UnixMilli(), FirstStartAt: start.Add(19 * time.Hour).UnixMilli(), SecondStartAt: start.Add(20 * time.Hour).UnixMilli(), DurationMillis: 1800000, PreparationMillis: 300000, Source: "isolated future schedule; real protocol remains required"}
}

func p13FunctionalFuture(now time.Time) time.Time {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	n := now.In(zone)
	return time.Date(n.Year(), n.Month()+2, 15, 0, 0, 0, 0, zone)
}

func p13FunctionalPrepare(t *testing.T, f *p1Fixture, sch *db.P13UnionFightSchedule, now time.Time) (*P13UFFunctionalRulesSummary, error) {
	t.Helper()
	var summary *P13UFFunctionalRulesSummary
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var err error
		summary, err = f.s.PrepareP13UnionFightFunctionalRules(tx, sch, now)
		if err != nil {
			return err
		}
		return tx.Save(sch).Error
	})
	return summary, err
}

func p13FunctionalCountDay(t *testing.T, day string) int64 {
	t.Helper()
	shape := reflect.TypeOf(p13FunctionalDay{})
	var total int64
	for i := 0; i < shape.NumField(); i++ {
		var count int64
		if err := db.GetIns().Model(reflect.New(shape.Field(i).Type.Elem()).Interface()).Where("day=?", day).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		total += count
	}
	return total
}

func TestP13UFFunctionalTwoFutureDaysTyped57AndStableGlobals(t *testing.T) {
	f, _ := p13FunctionalSetup(t)
	now := time.Now().Add(time.Minute)
	day := p13FunctionalFuture(now)
	// The producer preserves the operator's current enrollment/calendar choices.
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Updates(map[string]any{"enrollment_enabled": false, "match_enabled": true, "custom_calendar_enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	one := p13FunctionalScheduleFor(day, now)
	a, err := p13FunctionalPrepare(t, f, one, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.DayCreated != 57 || a.DayReused != 0 || p13FunctionalCountDay(t, one.Day) != 57 || a.GuardProof != constUtil.P6UFGuardMirrorNativeProof || a.PlayerProof != constUtil.P6UFPartyPlayerNativeProof {
		t.Fatal("incomplete typed publication", a)
	}
	var guard db.P13UnionFightGuardBattlePolicy
	var player db.P13UnionFightPlayerBattlePolicy
	var switches db.P13UnionFightPolicy
	if err = db.GetIns().First(&guard, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&player, "id=3").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&switches, 1).Error; err != nil {
		t.Fatal(err)
	}
	if guard.StartsAt != guard.PublishedAt || guard.EndsAt != p13FunctionalWindowEnd() || player.StartsAt != player.PublishedAt || player.EndsAt != guard.EndsAt || switches.EnrollmentEnabled || !switches.MatchEnabled || switches.CustomCalendarEnabled {
		t.Fatal("global window/switch contract", guard, player, switches)
	}
	var monsters []db.P13UnionFightGuardAuditSource
	if err = db.GetIns().Where("day=?", one.Day).Find(&monsters).Error; err != nil {
		t.Fatal(err)
	}
	if len(monsters) != 10 {
		t.Fatal("full10 templates")
	}
	for _, source := range monsters {
		expected, e := f.s.P13UnionFightMirrorGuardTemplate(source.GuardID)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := proto.MarshalOptions{Deterministic: true}.Marshal(expected)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(source.Monster, raw) {
			t.Fatal("not exact original full monster", source.GuardID)
		}
	}
	var shops []db.P13UnionFightShopSource
	if err = db.GetIns().Where("day=?", one.Day).Find(&shops).Error; err != nil {
		t.Fatal(err)
	}
	if len(shops) != 5 {
		t.Fatal("not five original normal slots")
	}
	for _, shop := range shops {
		if shop.ItemID < "ufitem_016" || shop.ItemID > "ufitem_020" || shop.CostResource != "" || shop.Quantity != 1 || shop.BuyCount != 5 || shop.RefreshMillis != 86400000 {
			t.Fatal("unexpected SKU", shop)
		}
	}
	two := p13FunctionalScheduleFor(day.AddDate(0, 0, 1), now)
	b, err := p13FunctionalPrepare(t, f, two, now)
	if err != nil {
		t.Fatal(err)
	}
	if b.DayCreated != 57 || b.GlobalsChanged != 0 || a.GlobalContentHash != b.GlobalContentHash {
		t.Fatal("subsequent day rewrote globals", a, b)
	}
	var guardAfter db.P13UnionFightGuardBattlePolicy
	var playerAfter db.P13UnionFightPlayerBattlePolicy
	if err = db.GetIns().First(&guardAfter, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&playerAfter, "id=3").Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(guard, guardAfter) || !reflect.DeepEqual(player, playerAfter) {
		t.Fatal("global publication changed")
	}
	c, err := p13FunctionalPrepare(t, f, one, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.DayCreated != 0 || c.DayReused != 57 || c.DayContentHash != a.DayContentHash {
		t.Fatal("same day was republished", c)
	}
	if p1Count(t, &db.P13UnionFightMatch{}, "1=1") != 0 || p1Count(t, &db.P13UnionFightGuardAuditOffer{}, "1=1") != 0 || p1Count(t, &db.P13UnionFightPlayerBattleOffer{}, "1=1") != 0 || p1Count(t, &db.RewardDelivery{}, "source_key LIKE ?", "p13-war:%") != 0 {
		t.Fatal("producer manufactured match/combat offer/reward")
	}
	for _, m := range a.Monthly {
		if !m.PolicyReady || !m.OfferReady {
			t.Fatal("future original month not genuinely armed", m)
		}
	}
}

func TestP13UFFunctionalUnknownExtraSourceAndRewardFailureAtomic(t *testing.T) {
	for _, kind := range []string{"unknown-source", "extra-sku", "unsupported-reward"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := p13FunctionalSetup(t)
			now := time.Now().Add(time.Minute)
			sch := p13FunctionalScheduleFor(p13FunctionalFuture(now), now)
			// Prove this exact fixture reaches a complete successful plan before
			// introducing the targeted corruption. Roll back all publication rows.
			rollback := errors.New("explicit successful publication rollback")
			err := db.GetIns().Transaction(func(tx *gorm.DB) error {
				result, e := f.s.PrepareP13UnionFightFunctionalRules(tx, sch, now)
				if e != nil {
					return e
				}
				if result.DayCreated != constUtil.P13UFFunctionalDayRows {
					return fmt.Errorf("baseline did not prepare all day sources: %+v", result)
				}
				return rollback
			})
			if !errors.Is(err, rollback) || p13FunctionalCountDay(t, sch.Day) != 0 || p1Count(t, &db.P13UnionFightMonthlyOffer{}, "1=1") != 0 {
				t.Fatal("valid baseline or caller rollback failed", err)
			}
			if kind == "unsupported-reward" {
				f.s.Tables["UnionFightRankSpecialReward"][0]["reward"] = "unrestored_guild_currency,1"
			} else {
				id, item, source := "local-own-token-016", "ufitem_016", "operator-custom"
				if kind == "extra-sku" {
					id, item, source = "unapproved-diamond-slot", "ufitem_011", constUtil.P13UFFunctionalSource
				}
				if err := db.GetIns().Create(&db.P13UnionFightShopSource{Day: sch.Day, ShopItemID: id, Enabled: true, ItemID: item, CostResource: "diamond", Price: 20, Quantity: 1, BuyCount: 5, BuyCondition: 0, Priority: 0, RefreshMillis: 86400000, PublishedAt: time.Now().UnixMilli(), Source: source}).Error; err != nil {
					t.Fatal(err)
				}
			}
			_, err = p13FunctionalPrepare(t, f, sch, now)
			wantReason := constUtil.P13UFFunctionalConflict
			if kind == "unsupported-reward" {
				wantReason = "原公会战奖励资源尚未完整恢复：unrestored_guild_currency,1"
			}
			if !errors.Is(err, constUtil.ErrAdminConfiguration) || !strings.Contains(err.Error(), wantReason) {
				t.Fatal("targeted source/reward rejection was not reached", kind, err)
			}
			var field db.P13UnionFightFieldPolicy
			var guard db.P13UnionFightGuardBattlePolicy
			if err := db.GetIns().First(&field, 1).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().First(&guard, 1).Error; err != nil {
				t.Fatal(err)
			}
			if field.Enabled || guard.PublishedAt != 0 || p1Count(t, &db.P13UnionFightSchedule{}, "day=?", sch.Day) != 0 || p1Count(t, &db.P13UnionFightMonthlyOffer{}, "1=1") != 0 {
				t.Fatal("failed prepare partially committed")
			}
			want := int64(1)
			if kind == "unsupported-reward" {
				want = 0
			}
			if p13FunctionalCountDay(t, sch.Day) != want {
				t.Fatal("unexpected day writes on failed plan")
			}
		})
	}
}

func TestP13UFFunctionalLongWindowRealEnrollmentTwoRoundsBuildAndBusyConversion(t *testing.T) {
	f, ids := p13FunctionalSetup(t)
	now := time.Now().Add(time.Minute)
	sch := p13FunctionalScheduleFor(p13FunctionalFuture(now), now)
	if _, err := p13FunctionalPrepare(t, f, sch, now); err != nil {
		t.Fatal(err)
	}
	p13GuildOpen(t, true)
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Updates(map[string]any{"war_enabled": true, "enrollment_enabled": true, "match_enabled": true, "custom_calendar_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	for len(ids) < 10 {
		ids = append(ids, p13GiftAdditionalPlayer(t, f, len(ids)))
	}
	register := time.UnixMilli(sch.EnrollmentStartAt).Add(time.Hour)
	clock := &atomic.Int64{}
	clock.Store(register.UnixMilli())
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return time.UnixMilli(clock.Load()) })
	var guilds []string
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("方案场%d", i))}, 1, register)
		guild := p13GuildID(t, f, id, register)
		guilds = append(guilds, guild)
		// Explicit finite level2 access prerequisite, matching existing field tests.
		// It grants no P6 result, roster, match, winner, HP or battle authority.
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
			t.Fatal(err)
		}
		p13UFCall(t, f, id, &pb.CSUFUnionApply{}, 2, register)
		p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, 3, register)
	}
	matched := time.UnixMilli(sch.EnrollmentEndAt).Add(time.Millisecond)
	clock.Store(matched.UnixMilli())
	if err := f.s.TickP13UnionFight(f.ctx, matched); err != nil {
		t.Fatal(err)
	}
	var allMatches []db.P13UnionFightMatch
	if err := db.GetIns().Where("day=?", sch.Day).Order("round,id").Find(&allMatches).Error; err != nil {
		t.Fatal(err)
	}
	if len(allMatches) != 8 || p1Count(t, &db.P13UnionFightEntry{}, "day=?", sch.RegistrationDay) != 10 || p1Count(t, &db.P13UnionFightEntry{}, "day=? AND frozen_at>0 AND match_processed=?", sch.RegistrationDay, true) != 10 {
		t.Fatal("real ten-guild enrollment did not freeze and produce eight matches", len(allMatches))
	}
	rounds := map[int32]int{}
	for _, match := range allMatches {
		rounds[match.Round]++
	}
	if rounds[1] != 4 || rounds[2] != 4 || len(rounds) != 2 {
		t.Fatal("unexpected real round distribution", rounds)
	}
	// The original grouping leaves two of ten guilds unpaired. Select an
	// actually matched guild rather than assuming creation order gets a match.
	selectedGuild := allMatches[0].LeftGuildID
	var selectedPlayer int64
	for i, guild := range guilds {
		if guild == selectedGuild {
			selectedPlayer = ids[i]
			break
		}
	}
	if selectedPlayer == 0 {
		t.Fatal("real matched guild has no enrolled fixture owner", selectedGuild)
	}
	var matches []db.P13UnionFightMatch
	for _, match := range allMatches {
		if match.LeftGuildID == selectedGuild || match.RightGuildID == selectedGuild {
			matches = append(matches, match)
		}
	}
	if len(matches) != 2 {
		t.Fatal("not two actual matched rounds", matches)
	}
	for _, match := range matches {
		at := time.UnixMilli(match.StartAt - 60000)
		clock.Store(at.UnixMilli())
		if _, err := f.s.p13UFFieldEnter(f.ctx, selectedPlayer, at); err != nil {
			t.Fatal("long-window actual field entry", match.Round, err)
		}
		if _, err := f.s.p13UFFieldFrames(f.ctx, selectedPlayer, &pb.CSCurrentPosition{}, at); err != nil {
			t.Fatal("original current-position handshake", err)
		}
		var offer db.P13UnionFightPlayerBattleOffer
		if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
			t.Fatal(err)
		}
		var rules p13UFPlayerBattleRules
		if json.Unmarshal([]byte(offer.Snapshot), &rules) != nil || !p13UFPlayerBattleRulesValid(&rules) || rules.Policy.EndsAt != p13FunctionalWindowEnd() || rules.Policy.PublishedAt > match.CreatedAt {
			t.Fatal("long policy did not freeze with real match", rules)
		}
		if p1Count(t, &db.P13UnionFightGuardAuditOffer{}, "match_id=?", match.ID) != 10 || p1Count(t, &db.P13UnionFightTowerState{}, "match_id=?", match.ID) != 6 {
			t.Fatal("not all actual typed sources built")
		}
	}
	// Genuine matching has already frozen the weekly offers. Planning after
	// the first start must preserve their complete immutable source bytes.
	week := p13UFWeek(time.UnixMilli(sch.FirstStartAt))
	var weeklyBefore []db.P13UnionFightWeeklyOffer
	if err := db.GetIns().Where("week_start=?", week).Order("`group`").Find(&weeklyBefore).Error; err != nil {
		t.Fatal(err)
	}
	if len(weeklyBefore) == 0 {
		t.Fatal("real matcher did not freeze weekly offers")
	}
	weeklyBytes := func(rows []db.P13UnionFightWeeklyOffer) ([]byte, error) {
		rules := make([][]byte, len(rows))
		for i := range rows {
			rules[i] = rows[i].Rules
		}
		return json.Marshal(struct {
			Rows  []db.P13UnionFightWeeklyOffer
			Rules [][]byte // The model deliberately omits Rules from ordinary JSON.
		}{rows, rules})
	}
	beforeRaw, err := weeklyBytes(weeklyBefore)
	if err != nil {
		t.Fatal(err)
	}
	var late P13UFFunctionalRulesSummary
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		plan, e := f.s.p13FunctionalPeriodPlan(tx, sch, time.UnixMilli(clock.Load()), time.Now().UnixMilli(), &late)
		if e == nil && len(plan.freezeGroups) != 0 {
			return errors.New("late plan attempted to refreeze existing weekly offers")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	foundLate := false
	for _, state := range late.Weekly {
		if state.Group == matches[0].Group {
			foundLate = state.Status == "existing-offer-preserved" && state.OfferReady
		}
	}
	var weeklyAfter []db.P13UnionFightWeeklyOffer
	if err := db.GetIns().Where("week_start=?", week).Order("`group`").Find(&weeklyAfter).Error; err != nil {
		t.Fatal(err)
	}
	afterRaw, err := weeklyBytes(weeklyAfter)
	if err != nil {
		t.Fatal(err)
	}
	if !foundLate || p6Digest(beforeRaw) != p6Digest(afterRaw) || !bytes.Equal(beforeRaw, afterRaw) {
		t.Fatal("existing weekly offers changed after start", late.Weekly)
	}
	// Simulate the explicitly named former finite operator window, not a combat
	// fixture. The next date needs conversion while genuine matches still wait.
	if err := db.GetIns().Model(&db.P13UnionFightGuardBattlePolicy{}).Where("id=1").Update("ends_at", sch.SecondStartAt+sch.DurationMillis+60000).Error; err != nil {
		t.Fatal(err)
	}
	next := p13FunctionalScheduleFor(time.UnixMilli(sch.FirstStartAt).AddDate(0, 1, 0), now)
	if _, err := p13FunctionalPrepare(t, f, next, time.UnixMilli(clock.Load())); err == nil || !strings.Contains(err.Error(), constUtil.P13UFFunctionalPending) {
		t.Fatal("live/awaiting matches permitted global conversion", err)
	}
	if p13FunctionalCountDay(t, next.Day) != 0 {
		t.Fatal("busy conversion wrote next day")
	}
}

func TestP13UFFunctionalPeriodNoRetroAndTransactionRequired(t *testing.T) {
	f, _ := p13FunctionalSetup(t)
	now := time.Now().Add(time.Minute)
	sch := p13FunctionalScheduleFor(p13FunctionalFuture(now), now)
	if _, err := f.s.PrepareP13UnionFightFunctionalRules(db.GetIns().WithContext(f.ctx), sch, now); err == nil {
		t.Fatal("publisher accepted nontransactional DB")
	}
	current := p13FunctionalScheduleFor(now, now)
	var summary P13UFFunctionalRulesSummary
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		_, e := f.s.p13FunctionalPeriodPlan(tx, current, now, time.Now().UnixMilli(), &summary)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range summary.Monthly {
		if m.Period == now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01") && (m.OfferReady || m.PolicyReady || m.Status != "month-started-no-retro-policy-or-offer") {
			t.Fatal("current month backfilled", m)
		}
	}
	if p1Count(t, &db.P13UnionFightMonthlyPolicy{}, "1=1") != 0 || p1Count(t, &db.P13UnionFightMonthlyOffer{}, "1=1") != 0 {
		t.Fatal("read-only period plan wrote rows")
	}
	// A failed enclosing transaction also rolls back a fully validated producer.
	rollback := errors.New("isolated rollback probe")
	err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := f.s.PrepareP13UnionFightFunctionalRules(tx, sch, now); e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(err, rollback) || p13FunctionalCountDay(t, sch.Day) != 0 {
		t.Fatal("producer escaped caller transaction", err)
	}
}
