package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFAbortTestFixture(t *testing.T) (*p1Fixture, *db.P13UnionFightMatch, *db.P13UnionFightSettlementOffer, *db.P13UnionFightSettlementPolicy, *atomic.Int64, int64) {
	t.Helper()
	f, ids, _, clock, match := p13UFFieldSetup(t)
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(clock.Load())
	if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "isolated-technical-abort-test", "source": "isolated policy fixture, not production publication", "published_at": now.UnixMilli() - 1, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "factory_mission_prestige": 0, "terminal_rule": constUtil.P13UFMixedTerminalRule, "draw_prestige": 5, "abort_rule": constUtil.P13UFTechnicalAbortRule, "finalization_grace_millis": 5000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13UFSettlementFreeze(db.GetIns(), match, now); err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightSettlementOffer
	if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	rule, err := p13UFSettlementRules(&offer, match)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated pre-existing field lifecycle fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, ids[0], now); err != nil {
		t.Fatal(err)
	}
	return f, match, &offer, rule, clock, ids[0]
}

func p13UFAbortTestSeal(t *testing.T, f *p1Fixture, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, rule *db.P13UnionFightSettlementPolicy, at int64) *db.P13UnionFightSettlementResult {
	t.Helper()
	p13UFFinalizationFence.Lock()
	defer p13UFFinalizationFence.Unlock()
	var result *db.P13UnionFightSettlementResult
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var err error
		result, err = p13UFTechnicalAbortSeal(tx, match, offer, rule, time.UnixMilli(at))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestP13UFTechnicalAbortDeadlineMetadataColdAndNoRewards(t *testing.T) {
	f, match, offer, rule, clock, id := p13UFAbortTestFixture(t)
	deadline := match.EndAt + 5000
	if result := p13UFAbortTestSeal(t, f, match, offer, rule, deadline-1); result != nil {
		t.Fatal("grace ended early")
	}
	// Deliberately untrusted captured bytes are evidence of unresolved work,
	// never a fabricated battle loss or a native-authoritative test bypass.
	record := db.P6BattleRecord{ID: p6Digest([]byte(t.Name())), P13MatchID: match.ID, PlayerID: id, Source: constUtil.P13UFPlayerBattleSource, SourceFight: "untrusted-pending", Status: constUtil.P6BattleCaptured, CreatedAt: match.StartAt, Input: []byte("damaged retained capture"), InputHash: "bad", Authoritative: false}
	if err := db.GetIns().Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	extra := []db.P6BattleRecord{}
	for i := 0; i < 129; i++ {
		row := record
		row.ID = p6Digest([]byte(fmt.Sprintf("%s/%d", t.Name(), i)))
		row.SourceFight = fmt.Sprintf("untrusted-%d", i)
		extra = append(extra, row)
	}
	if err := db.GetIns().Create(&extra).Error; err != nil {
		t.Fatal(err)
	}
	callback := "technical-abort-no-replay-payload"
	if err := db.GetIns().Callback().Row().Before("gorm:row").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p6_battle_records" {
			for _, column := range tx.Statement.Selects {
				if column == "input" || column == "outcome" || column == "operates" {
					t.Errorf("abort read replay payload %s", column)
				}
			}
			if limit, ok := tx.Statement.Clauses["LIMIT"].Expression.(clause.Limit); !ok || limit.Limit == nil || *limit.Limit > 128 {
				t.Error("abort source query lacks128 bound")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Row().Remove(callback) })
	clock.Store(deadline)
	result := p13UFAbortTestSeal(t, f, match, offer, rule, deadline)
	if result == nil || result.ResultKind != constUtil.P13UFTechnicalAbortKind || result.WinnerGuildID != "" || result.LoserGuildID != "" || string(result.Awards) != "[]" || result.WinTaskPrestige != 0 || result.LoseTaskPrestige != 0 {
		t.Fatal("abort created normal economics", result)
	}
	if err := p13UFCombatProofVerify(db.GetIns(), match, result); err != nil {
		t.Fatal("sealed abort metadata", err)
	}
	if err := db.GetIns().Model(&record).Updates(map[string]any{"input": []byte{}, "outcome": []byte{}, "operates": []byte{}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := p13UFCombatProofVerify(db.GetIns(), match, result); err != nil {
		t.Fatal("24h replay payload reclamation invalidated business proof", err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightSettlementPrestige{}).Where("guild_id IN ?", []string{match.LeftGuildID, match.RightGuildID}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("abort issued prestige", count, err)
	}
	if err := f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(deadline+86400000)); err != nil {
		t.Fatal(err)
	}
	again := p13UFAbortTestSeal(t, f, match, offer, rule, deadline+86400000)
	if again.CombatProofHash != result.CombatProofHash || again.SettledAt != result.SettledAt {
		t.Fatal("cold retry rewrote terminal")
	}
	called := false
	if err := p13UFPublishBeforeAbort(f.ctx, match.ID, func() { called = true }); err != nil || called {
		t.Fatal("post-abort publication escaped", called, err)
	}
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		stop, err := p13UFCombatCommitGate(tx, match.ID, time.UnixMilli(deadline+1))
		if err != nil {
			return err
		}
		if !stop {
			return errors.New("late source commit admitted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFSettlementQuery(f.ctx, id, &pb.CSUFFightResult{}, time.UnixMilli(deadline))
	if err != nil || len(frames) != 1 {
		t.Fatal("abort notice query", frames, err)
	}
	notice, ok := frames[0].(*pb.SCUFMessage)
	if !ok || notice.GetMsg() != constUtil.P13UFTechnicalAbortMessage {
		t.Fatal("abort became normal loss window", frames)
	}
	if err := db.GetIns().Model(&record).Update("status", constUtil.P6BattleComputed).Error; err != nil {
		t.Fatal(err)
	}
	if p13UFCombatProofVerify(db.GetIns(), match, result) == nil {
		t.Fatal("changed source metadata accepted")
	}
	if err := db.GetIns().Model(&record).Update("status", constUtil.P6BattleCaptured).Error; err != nil {
		t.Fatal(err)
	}
	appended := record
	appended.ID = p6Digest([]byte("postseal/" + t.Name()))
	appended.SourceFight = "postseal"
	if err := db.GetIns().Create(&appended).Error; err != nil {
		t.Fatal(err)
	}
	if p13UFCombatProofVerify(db.GetIns(), match, result) == nil {
		t.Fatal("postseal source append accepted")
	}
	if err := db.GetIns().Delete(&appended).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Delete(&extra[len(extra)-1]).Error; err != nil {
		t.Fatal(err)
	}
	if p13UFCombatProofVerify(db.GetIns(), match, result) == nil {
		t.Fatal("source tail deletion accepted")
	}
}

func TestP13UFTechnicalAbortDamagedRulesRosterRemainEvidence(t *testing.T) {
	f, match, offer, _, clock, id := p13UFAbortTestFixture(t)
	offer.Rules = []byte("broken frozen reward rules")
	match.Roster = []byte("broken frozen roster")
	if err := db.GetIns().Model(offer).Update("rules", offer.Rules).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(match).Update("roster", match.Roster).Error; err != nil {
		t.Fatal(err)
	}
	rule, err := p13UFAbortFrozenRule(offer, match)
	if err != nil || rule == nil {
		t.Fatal(err)
	}
	clock.Store(match.EndAt + 5000)
	result := p13UFAbortTestSeal(t, f, match, offer, rule, clock.Load())
	if err = p13UFTechnicalAbortVerify(db.GetIns(), match, result); err != nil {
		t.Fatal(err)
	}
	var proof p13UFAbortProof
	if json.Unmarshal(result.CombatProof, &proof) != nil || proof.ObservedRulesSHA != p6Digest(offer.Rules) || proof.ObservedRosterSHA != p6Digest(match.Roster) || len(proof.Participants) != 1 || proof.Participants[0].PlayerID != id {
		t.Fatal("damaged originals lost identity/evidence")
	}
	changed := *offer
	changed.AbortRule = ""
	changed.FinalizationGraceMillis = 0
	if got, err := p13UFAbortFrozenRule(&changed, match); err != nil || got != nil {
		t.Fatal("old offer silently upgraded", got, err)
	}
}

func TestP13UFTechnicalAbortPolicyCompatibilityAndEndEquality(t *testing.T) {
	old := db.P13UnionFightSettlementPolicy{}
	raw, _ := json.Marshal(old)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	if _, ok := decoded["abort_rule"]; ok {
		t.Fatal("legacy policy hash changed")
	}
	if _, ok := decoded["finalization_grace_millis"]; ok {
		t.Fatal("legacy grace hash changed")
	}
	if !p13UFAbortRuleValid(&old) || p13UFAbortEnabled(&old) {
		t.Fatal("legacy auto-enabled abort")
	}
	rule := db.P13UnionFightSettlementPolicy{TerminalRule: constUtil.P13UFMixedTerminalRule, AbortRule: constUtil.P13UFTechnicalAbortRule, FinalizationGraceMillis: 5000}
	match := &db.P13UnionFightMatch{StartAt: 1000, EndAt: 10000, LeftGuildID: "left", RightGuildID: "right"}
	if at, err := p13UFAbortDeadline(match, &rule); err != nil || at != 15000 {
		t.Fatal(at, err)
	}
	rule.FinalizationGraceMillis = 5001
	if p13UFAbortRuleValid(&rule) {
		t.Fatal("unknown grace admitted")
	}
	before := p13UFMixedFixturePlayer(1, 0, 100)
	event := p13UFMixedEvent{Kind: "guard", Key: "equal-end", CreatedAt: 5000, DueAt: 10000, AppliedAt: 10001, Before: []*pb.InstUFPlayer{before}, After: []*pb.InstUFPlayer{before}}
	if _, err := p13UFMixedFold(match, map[int64]string{1: "left"}, []p13UFMixedEvent{event}, 10000); err != nil {
		t.Fatal("Due==End excluded", err)
	}
	event.DueAt = 10001
	if _, err := p13UFMixedFold(match, map[int64]string{1: "left"}, []p13UFMixedEvent{event}, 10000); err == nil {
		t.Fatal("Due>End admitted")
	}
}

func TestP13UFTechnicalAbortWeeklyMonthlyNeverCountTwoLosses(t *testing.T) {
	match := &db.P13UnionFightMatch{LeftGuildID: "left", RightGuildID: "right", StartAt: 100, EndAt: 1000}
	rule := &db.P13UnionFightSettlementPolicy{TerminalRule: constUtil.P13UFMixedTerminalRule, AbortRule: constUtil.P13UFTechnicalAbortRule, FinalizationGraceMillis: 5000, DrawPrestige: 5, WinPrestige: 50, LosePrestige: 25}
	result := &db.P13UnionFightSettlementResult{MatchID: "fixture", ResultKind: constUtil.P13UFTechnicalAbortKind, TerminalAt: 1000, EndAt: 1000, OfferHash: "frozen", Source: constUtil.P13UFTechnicalAbortSource}
	for _, guild := range []string{"left", "right"} {
		base, extra, err := p13UFMixedRoundPrestige(match, result, rule, guild)
		if err != nil || base != 0 || extra != 0 {
			t.Fatal("abort counted normal prestige", guild, base, extra, err)
		}
	}
	rule.AbortRule = ""
	rule.FinalizationGraceMillis = 0
	if p13UFMixedRoundIdentity(match, result, rule) {
		t.Fatal("old publication accepted new abort authority")
	}
}
