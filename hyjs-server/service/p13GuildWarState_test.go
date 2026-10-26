package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func p13MasterAdmin(t *testing.T, f *p1Fixture) (context.Context, *model.AdminPrincipal) {
	t.Helper()
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	return context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1)), actor
}
func p13MasterSave(t *testing.T, f *p1Fixture, ctx context.Context, actor *model.AdminPrincipal, enabled bool) *model.AdminGuildSettingDTO {
	t.Helper()
	before, err := f.s.AdminGuildSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.s.AdminSaveGuildSettings(ctx, actor, model.AdminGuildSettingSaveRequest{Version: before.Version, Reason: "isolated master switch fixture", Policies: &model.AdminGuildPolicySave{UnionFight: &model.AdminGuildUnionFightPolicySave{WarEnabled: &enabled}}})
	if err != nil {
		t.Fatal(err)
	}
	return after
}

func TestP13WarMasterDefaultAdminPartialCASRollbackAndAuthenticatedState(t *testing.T) {
	f, ids := p13GuildSetup(t)
	// Isolated singleton reinitialization proves the persisted default, without
	// manufacturing any declaration, actor, combat result or award.
	if err := db.GetIns().Delete(&db.P13UnionFightPolicy{}, "id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	var policy db.P13UnionFightPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil || policy.WarEnabled {
		t.Fatal("master default must be off", err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	out, err := f.s.P13GuildWarState(f.ctx, ids[0], profile.Session)
	if err != nil || out.WarEnabled {
		t.Fatal("authenticated default state", out, err)
	}
	ctx, operator := p13MasterAdmin(t, f)
	before, err := f.s.AdminGuildSettings(ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	req := model.AdminGuildSettingSaveRequest{Version: before.Version, Reason: "isolated partial master publication", Policies: &model.AdminGuildPolicySave{UnionFight: &model.AdminGuildUnionFightPolicySave{WarEnabled: &enabled}}}
	viewer := adminGuildTestActor(t, constUtil.AdminRoleViewer)
	if _, err = f.s.AdminSaveGuildSettings(ctx, viewer, req); err == nil {
		t.Fatal("viewer changed master")
	}
	opened, err := f.s.AdminSaveGuildSettings(ctx, operator, req)
	if err != nil || !opened.Policies.UnionFight.WarEnabled {
		t.Fatal("partial enable", err)
	}
	var after db.P13UnionFightPolicy
	if err = db.GetIns().First(&after, 1).Error; err != nil {
		t.Fatal(err)
	}
	expected := policy
	expected.WarEnabled = true
	a, _ := json.Marshal(after)
	b, _ := json.Marshal(expected)
	if !bytes.Equal(a, b) {
		t.Fatal("partial master update changed source/leaf/calendar policy")
	}
	if _, err = f.s.AdminSaveGuildSettings(ctx, operator, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale master CAS accepted", err)
	}
	// Omitting the pointer preserves the true master even with a partial UF object.
	omitted, err := f.s.AdminSaveGuildSettings(ctx, operator, model.AdminGuildSettingSaveRequest{Version: opened.Version, Reason: "isolated omitted master", Policies: &model.AdminGuildPolicySave{UnionFight: &model.AdminGuildUnionFightPolicySave{}}})
	if err != nil || !omitted.Policies.UnionFight.WarEnabled {
		t.Fatal("omitted pointer cleared master", err)
	}
	disabled := false
	bad := omitted.Policies.UnionFight.Field
	bad.Enabled = true
	bad.PreparationMillis = -1
	_, err = f.s.AdminSaveGuildSettings(ctx, operator, model.AdminGuildSettingSaveRequest{Version: omitted.Version, Reason: "isolated rollback", Policies: &model.AdminGuildPolicySave{UnionFight: &model.AdminGuildUnionFightPolicySave{WarEnabled: &disabled, Field: &bad}}})
	if err == nil {
		t.Fatal("invalid leaf accepted")
	}
	if err = db.GetIns().First(&after, 1).Error; err != nil || !after.WarEnabled {
		t.Fatal("failed save leaked master close", err)
	}
	out, err = f.s.P13GuildWarState(f.ctx, ids[0], profile.Session)
	if err != nil || !out.WarEnabled {
		t.Fatal("live state did not reflect committed master", err)
	}
	wrong := append([]byte(nil), profile.Session...)
	wrong[0] ^= 1
	if _, err = f.s.P13GuildWarState(f.ctx, ids[0], wrong); err == nil {
		t.Fatal("wrong session received state")
	}
	originalSession := append([]byte(nil), profile.Session...)
	rotated := bytes.Repeat([]byte{0x41}, 16)
	if err = db.GetIns().Model(&profile).Update("session", rotated).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P13GuildWarState(f.ctx, ids[0], originalSession); err == nil {
		t.Fatal("old session survived rotation")
	}
	if _, err = f.s.P13GuildWarState(f.ctx, ids[0], rotated); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&profile).Update("server_id", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P13GuildWarState(f.ctx, ids[0], rotated); err == nil {
		t.Fatal("foreign realm received state")
	}
}

func TestP13WarMasterCloseKeepsAcceptedDeclarationsAndReceiptsMatching(t *testing.T) {
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	p13UFOpen(t)
	now := p13UFMondays()
	guilds := []string{}
	pendingMember := p13GiftAdditionalPlayer(t, f, 70)
	// Six accepted real guilds produce two paired groups with the unchanged
	// original 15/30/30/25 split; four guilds each occupy a separate group.
	ids = append(ids, p13GiftAdditionalPlayer(t, f, 71), p13GiftAdditionalPlayer(t, f, 72))
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("关停真实%d", i))}, 1, now)
		guild := p13GuildID(t, f, id, now)
		guilds = append(guilds, guild)
		if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			p13GuildCall(t, f, pendingMember, &pb.CSUnionApply{UnionId: proto.String(guild)}, 50, now)
			p13GuildCall(t, f, id, &pb.CSUnionAgreeApply{PlayerId: proto.Int64(pendingMember)}, 51, now)
		}
		if i != 4 {
			p13UFCall(t, f, id, &pb.CSUFUnionApply{}, 200, now)
			p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, 201, now)
		}
	}
	// A real previously opted-in fifth guild must not acquire a new declaration.
	p13UFCall(t, f, ids[4], &pb.CSUFAutoApplySet{IsAuto: proto.Int32(1)}, 202, now)
	if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("match_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	next := now.Add(12 * time.Hour)
	first := next.Add(20 * time.Hour).UnixMilli()
	if err := db.GetIns().Create(&db.P13UnionFightSchedule{Day: "2026-10-20", FirstStartAt: first, SecondStartAt: first + 600000, DurationMillis: 300000, Source: "isolated existing accepted schedule"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, operator := p13MasterAdmin(t, f)
	p13MasterSave(t, f, ctx, operator, false)
	if _, err := f.s.p13UFMutate(f.ctx, ids[4], &pb.CSUFUnionApply{}, 203, now); err == nil || err.Error() != constUtil.P13UnionFightMasterClosed {
		t.Fatal("new declaration accepted", err)
	}
	if _, err := f.s.p13UFMutate(f.ctx, pendingMember, &pb.CSUFPlayerApply{}, 204, now); err == nil || err.Error() != constUtil.P13UnionFightMasterClosed {
		t.Fatal("first member application bypassed master", err)
	}
	var unaccepted db.P13UnionFightFighter
	if err := db.GetIns().First(&unaccepted, "player_id=? AND guild_id=?", pendingMember, guilds[0]).Error; err != nil || unaccepted.AppliedAt != 0 {
		t.Fatal("rejected signup changed frozen eligibility", err)
	}
	if _, err := f.s.p13UFMutate(f.ctx, ids[4], &pb.CSUFAutoApplySet{IsAuto: proto.Int32(1)}, 205, now); err == nil || err.Error() != constUtil.P13UnionFightMasterClosed {
		t.Fatal("new auto opt-in bypassed master", err)
	}
	// Exact original receipts remain restorable while the master is off.
	p13UFCall(t, f, ids[0], &pb.CSUFUnionApply{}, 200, now)
	p13UFCall(t, f, ids[0], &pb.CSUFPlayerApply{}, 201, now)
	if err := f.s.p13UFAdvanceEnrollmentAndMatch(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	if got := p1Count(t, &db.P13UnionFightEntry{}, "guild_id=?", guilds[4]); got != 0 {
		t.Fatal("automatic declaration bypassed closed master", got)
	}
	if err := f.s.p13UFAdvanceEnrollmentAndMatch(f.ctx, next); err != nil {
		t.Fatal(err)
	}
	if got := p1Count(t, &db.P13UnionFightEntry{}, "frozen_at>0 AND match_processed=1"); got != 6 {
		t.Fatal("accepted entries not frozen/matched", got)
	}
	if got := p1Count(t, &db.P13UnionFightMatch{}, "day=?", "2026-10-20"); got != 4 {
		t.Fatal("accepted two-round matches missing", got)
	}
	if err := f.s.p13UFAdvanceEnrollmentAndMatch(f.ctx, next); err != nil {
		t.Fatal(err)
	}
	if got := p1Count(t, &db.P13UnionFightMatch{}, "day=?", "2026-10-20"); got != 4 {
		t.Fatal("duplicate matching after close", got)
	}
}

type p13MasterClosingContextKey struct{}

func TestP13WarMasterCloseLinearizesBeforeConcurrentFieldEntry(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated real admission linearization"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, operator := p13MasterAdmin(t, f)
	before, err := f.s.AdminGuildSettings(ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	locked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	callback := "master-close-holds-real-guild-lock"
	if err = db.GetIns().Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(p13MasterClosingContextKey{}) == true && tx.Statement.Table == "p13_guild_policies" && tx.Statement.Clauses["FOR"].Name != "" {
			once.Do(func() {
				close(locked)
				select {
				case <-release:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Query().Remove(callback) })
	closed := make(chan error, 1)
	disabled := false
	go func() {
		_, e := f.s.AdminSaveGuildSettings(context.WithValue(ctx, p13MasterClosingContextKey{}, true), operator, model.AdminGuildSettingSaveRequest{Version: before.Version, Reason: "isolated concurrent close", Policies: &model.AdminGuildPolicySave{UnionFight: &model.AdminGuildUnionFightPolicySave{WarEnabled: &disabled}}})
		closed <- e
	}()
	select {
	case <-locked:
	case e := <-closed:
		t.Fatal("close did not acquire guild lock", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	entered := make(chan error, 1)
	go func() { _, e := f.s.p13UFFieldEnter(ctx, ids[0], time.UnixMilli(clock.Load())); entered <- e }()
	select {
	case e := <-entered:
		close(release)
		t.Fatal("entry escaped held guild lock", e)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err = <-closed; err != nil {
		t.Fatal(err)
	}
	if err = <-entered; err == nil || err.Error() != constUtil.P13UnionFightMasterClosed {
		t.Fatal("entry committed after close", err)
	}
	if got := p1Count(t, &db.P13UnionFightFieldPlayer{}, "match_id=?", match.ID); got != 0 {
		t.Fatal("closed entry manufactured actor", got)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P13GuildMapEntry(f.ctx, ids[0], profile.Session); err == nil || err.Error() != constUtil.P13UnionFightMasterClosed {
		t.Fatal("map preflight bypassed master", err)
	}
}

func TestP13WarMasterAlreadyEnteredRealPKAndHistoricalRewardContinue(t *testing.T) {
	f, id, enemy, clock, match := p13UFPlayerBattle172ActualSetup(t)
	for _, pid := range []int64{id, enemy} {
		var profile db.Profile
		if err := db.GetIns().First(&profile, pid).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, profile.Session, func(proto.Message) error { return nil }))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, pid, false); err != nil {
			t.Fatal(err)
		}
	}
	published := match.StartAt - 60000
	if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "isolated-master-old-round-v1", "source": "isolated actual engine and published original item311 example", "published_at": published - 1, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "terminal_rule": "", "draw_prestige": 0, "abort_rule": "", "finalization_grace_millis": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13UFSettlementFreeze(db.GetIns(), match, time.UnixMilli(published)); err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightSettlementOffer
	if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	frozenOffer, _ := json.Marshal(offer)
	ctx, operator := p13MasterAdmin(t, f)
	p13MasterSave(t, f, ctx, operator, false)
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		_, _, _, _, err := f.s.p13UFFieldCurrent(tx, id, time.UnixMilli(clock.Load()))
		return err
	}); err != nil {
		t.Fatal("close froze admitted actor", err)
	}
	if _, handled, err := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 88201); err != nil || !handled {
		t.Fatal("close blocked real in-field PK", err)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(attempt.DueAt)
	if err := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil || attempt.Status != constUtil.P13UFPlayerBattleSettled {
		t.Fatal("close blocked accepted terminal", err)
	}
	clock.Store(match.EndAt)
	result, err := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
	if err != nil || result == nil || result.LeftKills+result.RightKills != 1 {
		t.Fatal("close blocked genuine natural terminal", err)
	}
	var retained db.P13UnionFightSettlementOffer
	if err = db.GetIns().First(&retained, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	retainedJSON, _ := json.Marshal(retained)
	if !bytes.Equal(frozenOffer, retainedJSON) {
		t.Fatal("master rewrote frozen offer")
	}
	f.open(t)
	if err = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); err != nil {
		t.Fatal("cold prior round proof", err)
	}
	for _, at := range []int64{result.AwardAt, result.AwardAt + 1} {
		if err = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(at)); err != nil {
			t.Fatal("master blocked historical award", err)
		}
	}
	if got := p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-war:"+match.ID); got != 2 {
		t.Fatal("historical award not exactly once", got)
	}
}
