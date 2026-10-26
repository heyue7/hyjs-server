package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFMixedFixturePlayer(id int64, camp int32, hp float64) *pb.InstUFPlayer {
	return &pb.InstUFPlayer{Guid: proto.Int64(id), Camp: proto.Int32(camp), State: proto.Int32(0), OwnToken: proto.Float64(0), OwnScore: proto.Float64(0), FightInfo: &pb.InstFightable{Guid: proto.Int64(id), Index: []int32{0}, HpPos: []float64{hp}, MaxHpPos: []float64{100}, Anger: proto.Float64(0)}}
}

// These mapping fixtures have no native authority. Production still loads and
// authenticates every P6/guard/tower source before this pure chain consumer.
func TestP13UFMixedFoldGuardTowerDeathNeverBecomesPlayerKill(t *testing.T) {
	match := &db.P13UnionFightMatch{ID: "fixture", StartAt: 100, EndAt: 100000, LeftGuildID: "left", RightGuildID: "right"}
	before := p13UFMixedFixturePlayer(1, 0, 100)
	guardAfter := proto.Clone(before).(*pb.InstUFPlayer)
	guardAfter.FightInfo.HpPos[0] = 50
	dead := proto.Clone(guardAfter).(*pb.InstUFPlayer)
	dead.FightInfo.HpPos[0] = 0
	dead.DeadNum = proto.Int32(1)
	dead.State = proto.Int32(3)
	events := []p13UFMixedEvent{{Kind: "tower", Key: "2/1", CreatedAt: 6000, DueAt: 6600, AppliedAt: 6600, Before: []*pb.InstUFPlayer{guardAfter}, After: []*pb.InstUFPlayer{dead}, DeadPlayerID: 1, ReviveSeconds: 5}, {Kind: "guard", Key: "guard", CreatedAt: 1000, DueAt: 6000, AppliedAt: 6000, Before: []*pb.InstUFPlayer{before}, After: []*pb.InstUFPlayer{guardAfter}}}
	ledger, err := p13UFMixedFold(match, map[int64]string{1: "left"}, events, match.EndAt)
	if err != nil || ledger.Kills != [2]int32{} || ledger.PlayerKills[1] != 0 || ledger.PlayerDeaths[1] != 1 || ledger.PlayerScores[1] != 0 {
		t.Fatal("environment/guard manufactured player kill", ledger, err)
	}
	for i := range events {
		if events[i].Kind == "tower" {
			events[i].Before[0] = proto.Clone(before).(*pb.InstUFPlayer)
		}
	}
	if _, err = p13UFMixedFold(match, map[int64]string{1: "left"}, events, match.EndAt); err == nil {
		t.Fatal("discontinuous HP prefix accepted")
	}
}
func TestP13UFMixedCutoffBaseAndSimultaneousRemainExplicit(t *testing.T) {
	match := &db.P13UnionFightMatch{StartAt: 100, EndAt: 10000}
	event := p13UFMixedEvent{DueAt: 7000, Before: []*pb.InstUFPlayer{p13UFMixedFixturePlayer(1, 0, 100)}, Building: &p13UFGuardBuildingProof{Event: "base-defeated", GuardCamp: 1}}
	at, winner, reason, err := p13UFMixedCutoff(match, []p13UFMixedEvent{event})
	if err != nil || at != 7000 || winner != 0 || reason != "base-defeated" {
		t.Fatal(at, winner, reason, err)
	}
	other := event
	other.Before = []*pb.InstUFPlayer{p13UFMixedFixturePlayer(2, 1, 100)}
	other.Building = &p13UFGuardBuildingProof{Event: "base-defeated", GuardCamp: 0}
	at, winner, reason, err = p13UFMixedCutoff(match, []p13UFMixedEvent{event, other})
	if err != nil || at != 7000 || winner != -2 || reason != "simultaneous-bases" {
		t.Fatal("simultaneous bases invented winner", at, winner, reason, err)
	}
	if match.EndAt != 10000 {
		t.Fatal("base cutoff changed scheduled duration")
	}
}
func TestP13UFMixedLegacyPolicyAndResultJSONRemainExact(t *testing.T) {
	policy, _ := json.Marshal(db.P13UnionFightSettlementPolicy{})
	result, _ := json.Marshal(db.P13UnionFightSettlementResult{})
	if strings.Contains(string(policy), "terminal_rule") || strings.Contains(string(policy), "draw_prestige") || strings.Contains(string(result), "result_kind") || strings.Contains(string(result), "terminal_at") {
		t.Fatal("zero-value new fields changed legacy JSON")
	}
	proof := p13UFCombatProof{Schema: constUtil.P13UFCombatProofSchema}
	raw, _ := json.Marshal(proof)
	if p13UFMixedSchema(raw) {
		t.Fatal("legacy v1 reclassified")
	}
}

// The environmental journal uses a retained legacy source contract exclusively
// inside this isolated fixture; it does not publish production tower rules.
func TestP13UFMixedTowerPersistedSealAndMetaDetectMutation(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	// Mixed sealing checks the real P6 directory, including the empty case.
	// Initialize the domain without inserting any authoritative battle fixture.
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFightTower(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFightShopPurchase(f.ctx); err != nil {
		t.Fatal(err)
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("roster")
	}
	enemy := roster.Right[0].PlayerID
	if enemy == ids[0] {
		enemy = roster.Left[0].PlayerID
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated mixed field fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{ids[0], enemy} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	published := match.StartAt - 60000
	if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "isolated-mixed-v2", "source": "isolated item311 example only", "published_at": published - 1, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "terminal_rule": constUtil.P13UFMixedTerminalRule, "draw_prestige": 5}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13UFSettlementFreeze(db.GetIns(), match, time.UnixMilli(published)); err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightSettlementOffer
	if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	before, err := p13UFFieldReadActor(&actor)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, readErr := p13UFFieldStoredProjection(db.GetIns(), match.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	buildings := &pb.SCUFBuildingRefresh{}
	if readErr = proto.Unmarshal(snapshot.Buildings, buildings); readErr != nil {
		t.Fatal(readErr)
	}
	var actualTower *pb.InstUFTower
	for _, candidate := range buildings.TowerInfos {
		if int64(candidate.GetGuard().GetCamp()) != actor.Camp {
			actualTower = candidate
			break
		}
	}
	if actualTower == nil {
		t.Fatal("enemy original tower missing")
	}
	tower := &db.P13UnionFightTowerState{MatchID: match.ID, TowerID: 1, GuardGUID: 9, Camp: 1 - actor.Camp, Range: 1.4, HPPercent: 50, CooldownMillis: 5000, ReleaseMillis: 600, AnimationMillis: 1000, ReviveSeconds: constUtil.P13UnionFightTowerReviveSeconds, MatchHash: match.SnapshotHash, Version: "isolated-legacy-percent-only", PublishedAt: published, Source: "legacy isolated lifecycle fixture", Active: true, NextFireAt: match.StartAt + 5000, Sequence: 1}
	tower.TowerID = int64(actualTower.GetId())
	tower.GuardGUID = actualTower.GetGuard().GetGuid()
	tower.Camp = int64(actualTower.GetGuard().GetCamp())
	tower.TowerGUID = actualTower.GetGuid()
	tower.SourceHash = p13UFTowerSourceHash(tower)
	tower.StateHash = p13UFTowerStateHash(tower)
	if err = db.GetIns().Create(tower).Error; err != nil {
		t.Fatal(err)
	}
	after := proto.Clone(before).(*pb.InstUFPlayer)
	for i, hp := range after.FightInfo.HpPos {
		after.FightInfo.HpPos[i] = hp / 2
	}
	hit := &db.P13UnionFightTowerHit{MatchID: match.ID, TowerGUID: tower.TowerGUID, Sequence: 1, TargetKind: "player", TargetGUID: actor.PlayerID, DueAt: match.StartAt + 600, AppliedAt: match.StartAt + 600, TowerSourceHash: tower.SourceHash}
	hit.Before, _ = proto.Marshal(before)
	hit.After, _ = proto.Marshal(after)
	hit.BeforeHash = p6Digest(hit.Before)
	hit.AfterHash = p6Digest(hit.After)
	hit.HitHash = p13UFTowerHitHash(hit)
	if err = p13UFTowerHitVerify(hit, tower); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(hit).Error; err != nil {
		t.Fatal(err)
	}
	if err = p13UFFieldSaveActor(db.GetIns(), &actor, after); err != nil {
		t.Fatal(err)
	}
	clock.Store(match.EndAt)
	result, err := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
	if err != nil || result == nil || !p13UFMixedSchema(result.CombatProof) {
		t.Fatal("environmental terminal did not use mixed source", result, err)
	}
	if err = p13UFCombatProofVerify(db.GetIns(), match, result); err != nil {
		t.Fatal("persisted lightweight mixed proof", err)
	}
	if result.LeftKills != 0 || result.RightKills != 0 {
		t.Fatal("environmental damage manufactured kills")
	}
	if err = db.GetIns().Model(hit).Update("after_hash", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if p13UFCombatProofVerify(db.GetIns(), match, result) == nil {
		t.Fatal("changed source metadata accepted")
	}
	if err = db.GetIns().Model(hit).Update("after_hash", hit.AfterHash).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := *hit
	duplicate.Sequence = 2
	duplicate.HitHash = p13UFTowerHitHash(&duplicate)
	if err = db.GetIns().Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	if p13UFCombatProofVerify(db.GetIns(), match, result) == nil {
		t.Fatal("postseal new source accepted")
	}
}

func TestP13UFMixedDrawBusinessUsesNamedRuleAndNoWinner(t *testing.T) {
	roster := p13UFMatchRoster{Left: []p13UFMatchFighter{{P13UnionFightFighter: db.P13UnionFightFighter{PlayerID: 1}}}, Right: []p13UFMatchFighter{{P13UnionFightFighter: db.P13UnionFightFighter{PlayerID: 2}}}}
	raw, _ := json.Marshal(roster)
	match := &db.P13UnionFightMatch{StartAt: 100, EndAt: 10000, LeftGuildID: "left", RightGuildID: "right", Roster: raw}
	match.LeftPreview, _ = proto.Marshal(&pb.UFInfoPreView{FightPower: proto.Int64(1000)})
	match.RightPreview, _ = proto.Marshal(&pb.UFInfoPreView{FightPower: proto.Int64(1000)})
	proof := &p13UFMixedProof{TerminalRule: constUtil.P13UFMixedTerminalRule, TerminalAt: 10000, Winner: -2, ResultKind: "draw", Reason: "equal-comparison", Reached: [2]int64{100, 100}}
	result := &db.P13UnionFightSettlementResult{LeftPower: 1000, RightPower: 1000}
	if err := p13UFMixedProofBusiness(match, result, proof); err != nil {
		t.Fatal("named exact comparison draw", err)
	}
	proof.TerminalRule = ""
	if p13UFMixedProofBusiness(match, result, proof) == nil {
		t.Fatal("legacy policy silently gained draw")
	}
}
