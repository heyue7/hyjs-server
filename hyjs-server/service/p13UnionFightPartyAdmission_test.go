package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Same real account/guild/enrollment/matcher/entry path as the ordinary172
// fixture; only the operator policy published BEFORE matching is ID3.
// The default owned hero remains a valid one-actor current-party formation.
func p13UFPartyAdmissionActualSetup(t *testing.T) (*p1Fixture, int64, int64, *atomic.Int64, *db.P13UnionFightMatch) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	p13UFOpen(t)
	if e := f.s.InitializeP6(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeP13UnionFightPlayerBattle(f.ctx); e != nil {
		t.Fatal(e)
	}
	var closed db.P13UnionFightPlayerBattlePolicy
	if e := db.GetIns().First(&closed, 1).Error; e != nil || closed.Enabled {
		t.Fatal("new player battle policy not defaultoff", e)
	}
	for i := 0; i < 5; i++ {
		login, e := LoginAccount(f.ctx, fmt.Sprintf("ordinary172-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-local-pass")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("实源17%d", i), "hero_047"); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, login.Id)
	}
	for i, id := range ids {
		level := int32(30)
		if i == 0 {
			level = 50
		}
		if e := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(level)
			p.Coin = proto.Int64(20000000)
			p.Diamond = proto.Int64(1000)
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	now := p13UFMondays()
	if e := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("match_enabled", true).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "finite isolated normal war field; playerHP comes only from actual frozen P5"}).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&db.P13UnionFightPlayerBattlePolicy{}).Where("id=?", constUtil.P13UFPlayerPartyPolicyID).Updates(map[string]any{"enabled": true, "version": constUtil.P13UFPlayerPartyPolicyVersion, "source": "prepublished isolated current-party-v1 admission; actual owned single-hero P5 formations; existing approved native party authority", "published_at": now.UnixMilli(), "starts_at": now.UnixMilli(), "ends_at": now.Add(72 * time.Hour).UnixMilli(), "revive_rule": constUtil.P13UFPlayerBattleReviveRule}).Error; e != nil {
		t.Fatal(e)
	}
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("普通战会%d", i))}, int64(100+i), now)
		guild := p13GuildID(t, f, id, now)
		if e := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; e != nil {
			t.Fatal(e)
		}
		p13UFCall(t, f, id, &pb.CSUFUnionApply{}, int64(200+i), now)
		p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, int64(300+i), now)
	}
	next := now.Add(12 * time.Hour)
	first := next.Add(20 * time.Hour).UnixMilli()
	if e := db.GetIns().Create(&db.P13UnionFightSchedule{Day: "2026-10-20", FirstStartAt: first, SecondStartAt: first + 600000, DurationMillis: 300000, Source: "finite isolated published calendar; original server dates unknown"}).Error; e != nil {
		t.Fatal(e)
	}
	if e := f.s.TickP13UnionFight(f.ctx, next); e != nil {
		t.Fatal(e)
	}
	guild := p13GuildID(t, f, ids[0], now)
	var match db.P13UnionFightMatch
	if e := db.GetIns().First(&match, "round=1 AND (left_guild_id=? OR right_guild_id=?)", guild, guild).Error; e != nil {
		t.Fatal(e)
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("actual roster")
	}
	enemy := int64(0)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, entry := range side {
			if entry.GuildID != guild {
				enemy = entry.PlayerID
				break
			}
		}
	}
	if enemy <= 0 {
		t.Fatal("actual drawn enemy missing")
	}
	clock := &atomic.Int64{}
	clock.Store(first - 60000)
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return time.UnixMilli(clock.Load()) })
	for _, id := range []int64{ids[0], enemy} {
		if _, e := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); e != nil {
			t.Fatal(e)
		}
		if _, e := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); e != nil {
			t.Fatal(e)
		}
	}
	// Real original movement, with enough elapsed clock allowance, reaches the
	// actual enemy position. No participant row position or HP is injected.
	clock.Store(first + 60000)
	var target db.P13UnionFightFieldPlayer
	if e := db.GetIns().First(&target, "match_id=? AND player_id=?", match.ID, enemy).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(target.X), Y: proto.Float32(target.Y), Z: proto.Float32(target.Z)}}, time.UnixMilli(clock.Load())); e != nil {
		t.Fatal(e)
	}
	return f, ids[0], enemy, clock, &match
}

func TestP13UFPartyPolicyThreeActualAdmissionCaptureAndTerminal(t *testing.T) {
	f, id, enemy, clock, match := p13UFPartyAdmissionActualSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
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
	var offer db.P13UnionFightPlayerBattleOffer
	if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	var rules p13UFPlayerBattleRules
	if json.Unmarshal([]byte(offer.Snapshot), &rules) != nil || rules.Policy.ID != constUtil.P13UFPlayerPartyPolicyID || rules.Policy.Version != constUtil.P13UFPlayerPartyPolicyVersion {
		t.Fatal("real match did not freeze ID3", rules.Policy.ID)
	}
	replies, handled, err := f.s.p13UFTryPlayerBattle(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0, now())
	if err != nil || !handled || len(replies) != 2 {
		t.Fatal("published ID3 fell through player capture", handled, err)
	}
	for _, reply := range replies {
		if _, ok := reply.(*pb.SCUFPlayerFightBegin); !ok {
			t.Fatal("actual capture did not return Begin", reply)
		}
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.CalculationStage != constUtil.P13UFPlayerBattleCaptured || attempt.Status != constUtil.P13UFPlayerBattlePending || len(attempt.Terminal) != 0 || attempt.ScopeProof != "" {
		t.Fatal("capture fabricated a terminal")
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil || record.Status != constUtil.P6BattleCaptured || record.Authoritative {
		t.Fatal("real uncomputed P6 capture", err)
	}
	clock.Store(attempt.DueAt)
	if err := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != constUtil.P13UFPlayerBattleSettled || attempt.ScopeProof != constUtil.P6UFPartyPlayerNativeProof || attempt.ScopeProof == "" {
		t.Fatal("ID3 worker did not settle with independent party proof", attempt.Status, attempt.CalculationStage)
	}
	if _, _, _, verified, err := f.s.p13UFPlayerBattleVerify(db.GetIns().WithContext(f.ctx), &attempt); err != nil || verified.Policy.ID != constUtil.P13UFPlayerPartyPolicyID {
		t.Fatal("settled real party source verification", err)
	}
	if err := db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil || !record.Authoritative {
		t.Fatal("worker failed to certify actual capture", err)
	}
}
