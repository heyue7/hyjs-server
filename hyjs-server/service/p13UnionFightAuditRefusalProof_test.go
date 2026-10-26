package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Uses actual finite P5-owned characters, protocol enrollment, matching, movement
// and ID3 capture. Only prepublished local rule/resource prerequisites are set;
// neither match identities nor P6 inputs/results are inserted by this fixture.
func p13UFAuditRefusalActualSetup(t *testing.T, withGuard bool) (*p1Fixture, int64, int64, *atomic.Int64, *db.P13UnionFightMatch, *db.P13UnionFightSettlementOffer) {
	t.Helper()
	f, id, enemy, clock, match := p13UFPartyAdmissionActualSetup(t)
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
	pub := match.StartAt - 120000
	source := db.P13UnionFightBattleAuditSource{Day: match.Day, Enabled: true, SeedSalt: 0, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1), PublishedAt: pub, Source: "isolated prepublished audit factor1 refusal; no war authority"}
	if err := db.GetIns().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.p13UFBattleAuditBuild(db.GetIns(), match, original, time.UnixMilli(pub+1)); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "isolated-audit-no-effect-v1", "source": "isolated original item311 finite award prerequisite", "published_at": pub, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "terminal_rule": constUtil.P13UFMixedTerminalRule, "draw_prestige": 5, "abort_rule": constUtil.P13UFTechnicalAbortRule, "finalization_grace_millis": 5000}).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.p13UFSettlementFreeze(db.GetIns(), match, time.UnixMilli(pub+1)); err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightSettlementOffer
	if err = db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if withGuard {
		var actor db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		guardID := "factory_guard_001"
		if actor.Camp == 1 {
			guardID = "factory_guard_002"
		}
		// Retained named local legacy fixture: original full actor template with
		// the existing frozen field HP1000; no native authority is assigned.
		monster := p13UFGuardAuditTemplate(original, guardID, 1000)
		wire, e := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
		if e != nil {
			t.Fatal(e)
		}
		if err = db.GetIns().Create(&db.P13UnionFightGuardAuditSource{Day: match.Day, GuardID: guardID, Enabled: true, Monster: wire, PublishedAt: pub, Source: "isolated existing legacy level1/blood1/pose3 original actor except explicit frozen field HP1000; no authority"}).Error; err != nil {
			t.Fatal(err)
		}
		snapshot, e := p13UFFieldStoredProjection(db.GetIns(), match.ID)
		if e != nil {
			t.Fatal(e)
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			t.Fatal("frozen building PB")
		}
		if err = f.s.p13UFGuardAuditBuild(db.GetIns(), match, buildings, original, time.UnixMilli(pub+1)); err != nil {
			t.Fatal(err)
		}
		clock.Store(match.StartAt + 100000)
		point, e := fieldPoint(original.table("UnionFightGuard", "UFGuardId", guardID)["Position"])
		if e != nil {
			t.Fatal(e)
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
		guid := p13UFFieldGUID(match.ID, "guard", guardID)
		if _, err = f.s.p13UFAskGuardAudit(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(guid)}, 71000, time.UnixMilli(clock.Load())); err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
			t.Fatal("actual guard refusal", err)
		}
		clock.Store(match.StartAt + 140000)
		var target db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&target, "match_id=? AND player_id=?", match.ID, enemy).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(target.X), Y: proto.Float32(target.Y), Z: proto.Float32(target.Z)}}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	before := map[int64]string{}
	for _, pid := range []int64{id, enemy} {
		var actor db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, pid).Error; err != nil {
			t.Fatal(err)
		}
		before[pid] = actor.FrameHash
	}
	if _, err = f.s.p13UFAskFightAudit(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 71001, time.UnixMilli(clock.Load())); err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("actual audit did not commit exact refusal", err)
	}
	for _, pid := range []int64{id, enemy} {
		var actor db.P13UnionFightFieldPlayer
		if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, pid).Error; err != nil {
			t.Fatal(err)
		}
		if actor.FrameHash != before[pid] {
			t.Fatal("refusal changed real player frame")
		}
	}
	audit, err := p13UFAuditRefusalsRead(db.GetIns().WithContext(f.ctx), match, true)
	wantAudit := 1
	if withGuard {
		wantAudit = 2
	}
	if err != nil || len(audit) != wantAudit || !audit[0].NoEffect || audit[0].Record.Authoritative {
		t.Fatal("actual refusal verification", audit, err)
	}
	if _, handled, e := f.s.p13UFTryPlayerBattle(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 71002, time.UnixMilli(clock.Load())); e != nil || !handled {
		t.Fatal("real ID3 capture after refusal", e)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err = db.GetIns().First(&attempt, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(attempt.DueAt)
	if err = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != constUtil.P13UFPlayerBattleSettled || attempt.ScopeProof != constUtil.P6UFPartyPlayerNativeProof || attempt.ScopeProof == "" {
		t.Fatal("actual certified ID3 settlement missing")
	}
	return f, id, enemy, clock, match, &offer
}

func TestP13UFAuditRefusalPartyThreeTerminalCold(t *testing.T) {
	f, _, _, clock, match, offer := p13UFAuditRefusalActualSetup(t, true)
	clock.Store(match.EndAt)
	var result *db.P13UnionFightSettlementResult
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		result, e = f.s.p13UFSettlePlayerCombat(tx, match, offer, time.UnixMilli(clock.Load()))
		return e
	}); err != nil || result == nil || result.ResultKind == constUtil.P13UFTechnicalAbortKind {
		t.Fatal("valid refused audit poisoned real PVP terminal", result, err)
	}
	var proof p13UFMixedProof
	if json.Unmarshal(result.CombatProof, &proof) != nil || len(proof.AuditRefusals) != 2 || !proof.AuditRefusals[0].NoEffect || proof.AuditRefusals[0].Record.Authoritative {
		t.Fatal("refusal not sealed independently")
	}
	raw := append([]byte(nil), result.CombatProof...)
	cold := &GameService{}
	if err := cold.p13UFCombatSettlementVerify(db.GetIns().WithContext(f.ctx), match, result); err != nil {
		t.Fatal("cold proof", err)
	}
	if again, err := cold.p13UFSettlePlayerCombat(db.GetIns().WithContext(f.ctx), match, offer, time.UnixMilli(clock.Load()+1000)); err != nil || again == nil || !bytes.Equal(raw, again.CombatProof) {
		t.Fatal("cold duplicate changed terminal", err)
	}
	// Each scalar/hash mutation is rejected by the lightweight consumer without
	// reexecuting P6 or removing the original audit from the source directory.
	var audit db.P13UnionFightBattleAuditAttempt
	if err := db.GetIns().First(&audit, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	originalAudit := audit
	for _, field := range []string{"refusal", "request_hash", "source_hash", "offer_hash", "input_hash", "outcome_hash", "replay_hash"} {
		if err := db.GetIns().Model(&audit).Update(field, "broken").Error; err != nil {
			t.Fatal(err)
		}
		if err := cold.p13UFCombatSettlementVerify(db.GetIns().WithContext(f.ctx), match, result); err == nil {
			t.Fatal("changed audit metadata accepted", field)
		}
		restore := map[string]any{"refusal": originalAudit.Refusal, "request_hash": originalAudit.RequestHash, "source_hash": originalAudit.SourceHash, "offer_hash": originalAudit.OfferHash, "input_hash": originalAudit.InputHash, "outcome_hash": originalAudit.OutcomeHash, "replay_hash": originalAudit.ReplayHash}
		if err := db.GetIns().Model(&audit).Update(field, restore[field]).Error; err != nil {
			t.Fatal(err)
		}
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", audit.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&record).Update("authoritative", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := cold.p13UFCombatSettlementVerify(db.GetIns().WithContext(f.ctx), match, result); err == nil {
		t.Fatal("audit promoted to combat authority")
	}
	if err := db.GetIns().Model(&record).Update("authoritative", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := cold.p13UFCombatSettlementVerify(db.GetIns().WithContext(f.ctx), match, result); err != nil {
		t.Fatal("restored original audit", err)
	}
}

func TestP13UFAuditRefusalDamagedClosedThenSingleAbort(t *testing.T) {
	f, _, _, clock, match, offer := p13UFAuditRefusalActualSetup(t, false)
	var audit db.P13UnionFightBattleAuditAttempt
	if err := db.GetIns().First(&audit, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&audit).Update("refusal", "unknown refusal").Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(match.EndAt)
	if result, err := f.s.p13UFSettlePlayerCombat(db.GetIns().WithContext(f.ctx), match, offer, time.UnixMilli(clock.Load())); err == nil || result != nil {
		t.Fatal("unrecognized audit was accepted", err)
	}
	clock.Store(match.EndAt + 5000)
	if err := f.s.p13UFSettleMatchCandidate(f.ctx, match.ID, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var result db.P13UnionFightSettlementResult
	if err := db.GetIns().First(&result, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if result.ResultKind != constUtil.P13UFTechnicalAbortKind || result.WinnerGuildID != "" || result.LoserGuildID != "" {
		t.Fatal("bad evidence invented winner", result.ResultKind)
	}
	seal := result.CombatProofHash
	if err := f.s.p13UFSettleMatchCandidate(f.ctx, match.ID, time.UnixMilli(clock.Load()+1000)); err != nil {
		t.Fatal(err)
	}
	var again db.P13UnionFightSettlementResult
	if err := db.GetIns().First(&again, "match_id=?", match.ID).Error; err != nil || again.CombatProofHash != seal || p1Count(t, &db.P13UnionFightSettlementResult{}, "match_id=?", match.ID) != 1 {
		t.Fatal("abort duplicated or changed", err)
	}
	if err := db.GetIns().First(&audit, "match_id=?", match.ID).Error; err != nil || !strings.Contains(audit.Refusal, "unknown") {
		t.Fatal("abort erased refused audit", err)
	}
}

func TestP13UFAuditRefusalEmptyDirectoryLegacyJSON(t *testing.T) {
	raw, err := json.Marshal(p13UFMixedProof{})
	if err != nil || bytes.Contains(raw, []byte("AuditRefusals")) {
		t.Fatal("empty audit directory changed old proof JSON", err)
	}
}
