package service

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p13UFGuardBuildingProof preserves consumption independently of visual overlay.
// Gate is the exact admission-bound destroyed gate snapshot; factory snapshots
// describe only the atomic pause mutation, not later legitimate recovery.
type p13UFGuardBuildingProof struct {
	Schema, MatchID, AttemptID, Event                                 string
	Seal, TerminalHash, P6RecordID                                    string
	GuardGUID, BindGUID, GuardGeneration, GuardCamp, DueAt, AppliedAt int64
	Gate                                                              *db.P13UnionFightGateState `json:",omitempty"`
	GateFrame                                                         []byte                     `json:",omitempty"`
	FactorySourceHash                                                 string                     `json:",omitempty"`
	FactoryBefore, FactoryAfter                                       []byte                     `json:",omitempty"`
	FactoryBeforeHash, FactoryAfterHash                               string                     `json:",omitempty"`
	FactoryTimerBefore, FactoryTimerAfter                             *p13UFGuardFactoryTimer    `json:",omitempty"`
}

// The optional timer proof records the exact paused production identity. It
// contains no car/player payload and does not change the static factory source.
type p13UFGuardFactoryTimer struct {
	StartedAt, AnimationAt, DueAt, CompletedAt, PausedAt, RemainingMillis int64
	Cycle                                                                 *db.P13UnionFightFactoryCycleState `json:",omitempty"`
}

func p13UFGuardFactoryTimerOf(factory *db.P13UnionFightFactoryState, cycle *db.P13UnionFightFactoryCycleState) *p13UFGuardFactoryTimer {
	result := &p13UFGuardFactoryTimer{StartedAt: factory.StartedAt, AnimationAt: factory.AnimationAt, DueAt: factory.DueAt, CompletedAt: factory.CompletedAt, PausedAt: factory.PausedAt, RemainingMillis: factory.RemainingMillis}
	if cycle != nil {
		copy := *cycle
		result.Cycle = &copy
	}
	return result
}
func p13UFGuardFactoryPauseProof(before, after *p13UFGuardFactoryTimer, row *db.P13UnionFightGuardBattleAttempt, bind int64) bool {
	if before == nil || after == nil || before.PausedAt != 0 || before.RemainingMillis != 0 || after.StartedAt != before.StartedAt || after.AnimationAt != before.AnimationAt || after.DueAt != before.DueAt || after.CompletedAt != before.CompletedAt {
		return false
	}
	active := before.CompletedAt == 0 && before.DueAt > 0
	if active {
		if before.StartedAt <= 0 || before.AnimationAt < before.StartedAt || before.AnimationAt > before.DueAt || after.PausedAt != row.DueAt || after.RemainingMillis != max(int64(0), before.DueAt-row.DueAt) {
			return false
		}
	} else if after.PausedAt != 0 || after.RemainingMillis != 0 {
		return false
	}
	if (before.Cycle == nil) != (after.Cycle == nil) {
		return false
	}
	if before.Cycle != nil {
		b, a := before.Cycle, after.Cycle
		if b.MatchID != row.MatchID || b.FactoryGUID != bind || b.StateHash != p13UFFactoryCycleStateHash(b) || a.StateHash != p13UFFactoryCycleStateHash(a) || b.PausedAt != 0 || b.RemainingMillis != 0 {
			return false
		}
		if b.BufferedAt > 0 && (b.NextBornAt != 0 || b.ProductionStartedAt <= 0 || b.BufferedAt-b.ProductionStartedAt < 120000 || b.AnimationPlayed) {
			return false
		}
		expected := *b
		if b.ClosedAt == 0 && b.BufferedAt == 0 {
			expected.PausedAt = row.DueAt
			expected.RemainingMillis = max(int64(0), b.NextBornAt-row.DueAt)
		}
		expected.StateHash = p13UFFactoryCycleStateHash(&expected)
		if expected != *a {
			return false
		}
	}
	return true
}

func p13UFGuardBuildingProofSeal(proof p13UFGuardBuildingProof) string {
	proof.Seal = ""
	raw, _ := json.Marshal(proof)
	return p6Digest(raw)
}
func p13UFGuardBuildingSeal(row *db.P13UnionFightGuardBattleAttempt, state *db.P13UnionFightGuardState, now time.Time, proof p13UFGuardBuildingProof) error {
	proof.Schema = constUtil.P13UFMixedBuildingSchema
	proof.MatchID = row.MatchID
	proof.AttemptID = row.ID
	proof.Event = row.BuildingEvent
	proof.GuardGUID = row.GuardGUID
	proof.BindGUID = state.BindGUID
	proof.GuardGeneration = row.GuardGeneration
	proof.GuardCamp = state.Camp
	proof.DueAt = row.DueAt
	proof.AppliedAt = now.UnixMilli()
	if proof.Gate != nil {
		proof.GateFrame = append([]byte(nil), proof.Gate.Frame...)
	}
	proof.TerminalHash = row.TerminalHash
	proof.P6RecordID = row.P6RecordID
	proof.Seal = p13UFGuardBuildingProofSeal(proof)
	raw, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	row.BuildingAppliedAt = now.UnixMilli()
	row.BuildingProof = raw
	row.SourceHash = p13UFGuardBattleHash(row)
	return nil
}
func p13UFGuardBuildingProofRead(row *db.P13UnionFightGuardBattleAttempt) (*p13UFGuardBuildingProof, error) {
	var proof p13UFGuardBuildingProof
	reject := func() (*p13UFGuardBuildingProof, error) { return nil, errors.New(constUtil.P13UFGuardConflict) }
	if row == nil || !row.GuardDefeated || row.Status != constUtil.P13UFGuardSettled || row.BuildingAppliedAt != row.SettledAt || row.SourceHash != p13UFGuardBattleHash(row) || json.Unmarshal(row.BuildingProof, &proof) != nil || proof.Schema != constUtil.P13UFMixedBuildingSchema || proof.Seal != p13UFGuardBuildingProofSeal(proof) || proof.TerminalHash != row.TerminalHash || proof.P6RecordID != row.P6RecordID || proof.MatchID != row.MatchID || proof.AttemptID != row.ID || proof.Event != row.BuildingEvent || proof.GuardGUID != row.GuardGUID || proof.GuardGeneration != row.GuardGeneration || proof.DueAt != row.DueAt || proof.AppliedAt != row.BuildingAppliedAt {
		return reject()
	}
	guard := &pb.InstUFGuard{}
	if proto.Unmarshal(row.GuardAfter, guard) != nil || guard.GetGuid() != proof.GuardGUID || guard.GetBindGuid() != proof.BindGUID || int64(guard.GetCamp()) != proof.GuardCamp || guard.GetState() != 2 || len(guard.GetFightInfo().GetHpPos()) != 1 || guard.GetFightInfo().HpPos[0] != 0 {
		return reject()
	}
	switch proof.Event {
	case "tower-defeated":
		if proof.Gate != nil || len(proof.FactoryBefore) > 0 || len(proof.FactoryAfter) > 0 {
			return reject()
		}
	case "factory-defeated":
		before, after := &pb.InstUFCariotFactory{}, &pb.InstUFCariotFactory{}
		if len(proof.FactorySourceHash) != 64 || proof.FactoryBeforeHash != p6Digest(proof.FactoryBefore) || proof.FactoryAfterHash != p6Digest(proof.FactoryAfter) || proto.Unmarshal(proof.FactoryBefore, before) != nil || proto.Unmarshal(proof.FactoryAfter, after) != nil || before.GetGuid() != proof.BindGUID || after.GetGuid() != proof.BindGUID || after.GetState() != 2 || !proto.Equal(after.GetGuard(), guard) {
			return reject()
		}
		var rules p13UFGuardBattleRules
		if json.Unmarshal(row.Rules, &rules) != nil {
			return reject()
		}
		if rules.LocalRule == constUtil.P13UFGuardMirrorRule {
			if !p13UFGuardFactoryPauseProof(proof.FactoryTimerBefore, proof.FactoryTimerAfter, row, proof.BindGUID) {
				return reject()
			}
		} else if proof.FactoryTimerBefore != nil || proof.FactoryTimerAfter != nil {
			return reject()
		}
	case "base-defeated":
		var rules p13UFGuardBattleRules
		if proof.Gate == nil || json.Unmarshal(row.Rules, &rules) != nil || rules.BuildingKind != "base" {
			return reject()
		}
		proof.Gate.Frame = append([]byte(nil), proof.GateFrame...)
		gate, err := p13UFGateRead(proof.Gate)
		if err != nil || proof.Gate.MatchID != row.MatchID || proof.Gate.Camp != proof.GuardCamp || !p13UFGuardBaseGateAllowed(rules.LocalRule, proof.Gate.Camp, proof.Gate.GateID, rules.MatchStartAt, rules.MatchEndAt, row.CreatedAt, gate) {
			return reject()
		}
		if p13UFGuardBaseGateProof(row.MatchID, row.PlayerID, row.GuardGUID, proof.Gate, rules.LocalRule, rules.MatchStartAt, rules.MatchEndAt, row.CreatedAt) != rules.GateProof {
			return reject()
		}
	default:
		return reject()
	}
	return &proof, nil
}

// Empty rule retains the historical gate-proof bytes and admission semantics.
func p13UFGuardBaseGateAllowed(rule string, camp, gateID, start, end, at int64, gate *pb.InstUFGate) bool {
	if gate == nil || gate.GetHp() != 0 {
		return false
	}
	if rule == "" || rule == constUtil.P13UFGuardLocalRule {
		return true
	}
	if rule != constUtil.P13UFGuardMirrorRule || start <= 0 || end <= start || at < start+(end-start)/2 || at >= end {
		return false
	}
	return camp == 0 && (gateID == 1 || gateID == 2) || camp == 1 && (gateID == 5 || gateID == 6)
}

func p13UFGuardBaseGateProof(match string, player, guard int64, gate *db.P13UnionFightGateState, rule string, start, end, at int64) string {
	if rule != constUtil.P13UFGuardMirrorRule {
		rule, start, end, at = "", 0, 0, 0
	}
	raw, _ := json.Marshal(struct {
		Match                               string
		Player, Guard, Gate                 int64
		GateHash                            string
		Rule                                string `json:",omitempty"`
		MatchStartAt, MatchEndAt, CreatedAt int64  `json:",omitempty"`
	}{match, player, guard, gate.GUID, gate.SourceHash, rule, start, end, at})
	return p6Digest(raw)
}

// p13UFGuardBuildingBaseCheck binds admission to an authenticated destroyed enemy
// gate in this match. The proof is frozen by the guard attempt, not reinterpreted
// from an untrusted visual frame at settlement.
func p13UFGuardBuildingBaseCheck(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, guard *pb.InstUFGuard) (string, error) {
	if tx == nil || actor == nil || guard == nil || actor.MatchID == "" || actor.Camp < 0 || actor.Camp > 1 || guard.GetCamp() < 0 || guard.GetCamp() > 1 || int64(guard.GetCamp()) == actor.Camp {
		return "", errors.New(constUtil.P13UFGuardConflict)
	}
	var match db.P13UnionFightMatch
	if err := tx.Select("id,start_at,end_at").First(&match, "id=?", actor.MatchID).Error; err != nil {
		return "", err
	}
	snapshot, err := p13UFFieldStoredProjection(tx, actor.MatchID)
	if err != nil {
		return "", err
	}
	var buildings pb.SCUFBuildingRefresh
	if proto.Unmarshal(snapshot.Buildings, &buildings) != nil || p13UFGuardBinding(&buildings, guard.GetGuid()) != "base" {
		return "", errors.New(constUtil.P13UFGuardConflict)
	}
	found := false
	for _, base := range buildings.BaseCampInfos {
		if base.GetGuard().GetGuid() == guard.GetGuid() && base.GetGuard().GetBindGuid() == guard.GetBindGuid() && base.GetGuard().GetCamp() == guard.GetCamp() {
			found = true
			break
		}
	}
	if !found {
		return "", errors.New(constUtil.P13UFGuardConflict)
	}
	var rows []db.P13UnionFightGateState
	if err := tx.Where("match_id=? AND camp=?", actor.MatchID, guard.GetCamp()).Order("guid").Find(&rows).Error; err != nil {
		return "", err
	}
	for i := range rows {
		frame, err := p13UFGateRead(&rows[i])
		if err != nil {
			return "", err
		}
		at := p13UFNow(tx.Statement.Context).UnixMilli()
		if p13UFGuardBaseGateAllowed(snapshot.GuardRule, rows[i].Camp, rows[i].GateID, match.StartAt, match.EndAt, at, frame) {
			return p13UFGuardBaseGateProof(actor.MatchID, actor.PlayerID, guard.GetGuid(), &rows[i], snapshot.GuardRule, match.StartAt, match.EndAt, at), nil
		}
	}
	return "", errors.New(constUtil.P13UFGuardConflict)
}

// p13UFGuardBuildingApply is a transaction hook, called once after verified guard
// settlement has saved row/state and before commit. Base events remain durable
// on that same ledger for the separate winner ledger consumer.
func (s *GameService) p13UFGuardBuildingApply(tx *gorm.DB, row *db.P13UnionFightGuardBattleAttempt, state *db.P13UnionFightGuardState, now time.Time) ([]proto.Message, error) {
	if tx == nil || row == nil || state == nil || row.SourceHash != p13UFGuardBattleHash(row) || row.BuildingAppliedAt != 0 || len(row.BuildingProof) != 0 || row.Status != constUtil.P13UFGuardSettled || !row.GuardDefeated || row.BuildingEvent == "" || row.MatchID != state.MatchID || row.GuardGUID != state.GuardGUID || row.BuildingEvent != state.BuildingEvent || row.DueAt != state.DefeatedAt || row.SettledAt <= 0 || row.DueAt > now.UnixMilli() || row.SettledAt > now.UnixMilli() {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	guard, err := p13UFGuardStateFrame(state)
	if err != nil {
		return nil, err
	}
	if guard.GetState() != 2 || guard.GetFightInfo() == nil || len(guard.FightInfo.HpPos) != 1 || guard.FightInfo.HpPos[0] != 0 {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	switch state.BuildingKind {
	case "tower":
		if state.BuildingEvent != "tower-defeated" || state.RecoverAt-state.DefeatedAt != 120000 {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		if err := p13UFGuardBuildingSeal(row, state, now, p13UFGuardBuildingProof{}); err != nil {
			return nil, err
		}
		return []proto.Message{&pb.SCUFTowerCrash{TowerGuid: proto.Int64(state.BindGUID)}}, nil
	case "factory":
		if state.BuildingEvent != "factory-defeated" || state.RecoverAt-state.DefeatedAt != 300000 {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		var match db.P13UnionFightMatch
		if err := tx.First(&match, "id=?", row.MatchID).Error; err != nil {
			return nil, err
		}
		factory, frame, _, err := s.p13UFFactoryRead(tx, &match, state.BindGUID)
		if err != nil {
			return nil, err
		}
		if factory.Camp != state.Camp || frame.GetGuard().GetGuid() != state.GuardGUID {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		beforeRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(frame)
		if err != nil {
			return nil, err
		}
		frame.State = proto.Int32(2)
		frame.StateLeftTime = proto.Float32(float32(max(int64(0), state.RecoverAt-now.UnixMilli())) / 1000)
		frame.StateTotalTime = proto.Float32(300)
		frame.Guard = guard
		var timerBefore, timerAfter *p13UFGuardFactoryTimer
		if state.GuardRule == constUtil.P13UFGuardMirrorRule {
			cycle, err := p13UFGuardFactoryPauseCycle(tx, factory, state.DefeatedAt)
			if err != nil {
				return nil, err
			}
			timerBefore = p13UFGuardFactoryTimerOf(factory, cycle)
			if factory.CompletedAt == 0 && factory.DueAt > 0 {
				factory.PausedAt = state.DefeatedAt
				factory.RemainingMillis = max(int64(0), factory.DueAt-state.DefeatedAt)
			}
			if cycle != nil && cycle.ClosedAt == 0 && cycle.BufferedAt == 0 {
				cycle.PausedAt = state.DefeatedAt
				cycle.RemainingMillis = max(int64(0), cycle.NextBornAt-state.DefeatedAt)
				cycle.StateHash = p13UFFactoryCycleStateHash(cycle)
				if err = tx.Save(cycle).Error; err != nil {
					return nil, err
				}
			}
			timerAfter = p13UFGuardFactoryTimerOf(factory, cycle)
		} else if factory.CompletedAt == 0 {
			factory.StartedAt = 0
			factory.AnimationAt = 0
			factory.DueAt = 0
		}
		if err := p13UFFactorySave(tx, factory, frame); err != nil {
			return nil, err
		}
		afterRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(frame)
		if err != nil {
			return nil, err
		}
		if err = p13UFGuardBuildingSeal(row, state, now, p13UFGuardBuildingProof{FactoryTimerBefore: timerBefore, FactoryTimerAfter: timerAfter, FactorySourceHash: factory.SourceHash, FactoryBefore: beforeRaw, FactoryAfter: afterRaw, FactoryBeforeHash: p6Digest(beforeRaw), FactoryAfterHash: p6Digest(afterRaw)}); err != nil {
			return nil, err
		}
		return []proto.Message{&pb.SCUFBuildingRefresh{FactoryInfos: []*pb.InstUFCariotFactory{frame}}}, nil
	case "base":
		if state.BuildingEvent != "base-defeated" || state.RecoverAt != 0 {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		var gates []db.P13UnionFightGateState
		if err := tx.Where("match_id=? AND camp=?", row.MatchID, state.Camp).Order("guid").Limit(5).Find(&gates).Error; err != nil {
			return nil, err
		}
		if len(gates) > 4 {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		var rules p13UFGuardBattleRules
		if json.Unmarshal(row.Rules, &rules) != nil {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		for i := range gates {
			gate, err := p13UFGateRead(&gates[i])
			if err != nil {
				return nil, err
			}
			if !p13UFGuardBaseGateAllowed(rules.LocalRule, gates[i].Camp, gates[i].GateID, rules.MatchStartAt, rules.MatchEndAt, row.CreatedAt, gate) {
				continue
			}
			if p13UFGuardBaseGateProof(row.MatchID, row.PlayerID, row.GuardGUID, &gates[i], rules.LocalRule, rules.MatchStartAt, rules.MatchEndAt, row.CreatedAt) != rules.GateProof {
				continue
			}
			if err = p13UFGuardBuildingSeal(row, state, now, p13UFGuardBuildingProof{Gate: &gates[i]}); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	return nil, errors.New(constUtil.P13UFGuardConflict)
}

func p13UFGuardFactoryResumeTimer(factory *db.P13UnionFightFactoryState, recoverAt int64) error {
	if factory == nil || factory.PausedAt <= 0 || factory.RemainingMillis < 0 || factory.PausedAt >= recoverAt || factory.DueAt < factory.AnimationAt || recoverAt > math.MaxInt64-factory.RemainingMillis {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	animationMillis := factory.DueAt - factory.AnimationAt
	factory.DueAt = recoverAt + factory.RemainingMillis
	factory.AnimationAt = max(recoverAt, factory.DueAt-animationMillis)
	factory.PausedAt = 0
	factory.RemainingMillis = 0
	return nil
}
func p13UFGuardFactoryResumeCycle(cycle *db.P13UnionFightFactoryCycleState, recoverAt int64) error {
	if cycle == nil || cycle.PausedAt <= 0 || cycle.RemainingMillis < 0 || cycle.PausedAt >= recoverAt || recoverAt > math.MaxInt64-cycle.RemainingMillis {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	cycle.NextBornAt = recoverAt + cycle.RemainingMillis
	cycle.PausedAt = 0
	cycle.RemainingMillis = 0
	return nil
}

// Load/create the next cycle from the already frozen quote. Defeat freezes a
// production timer even if its first maintenance observation has not run yet.
func p13UFGuardFactoryPauseCycle(tx *gorm.DB, factory *db.P13UnionFightFactoryState, at int64) (*db.P13UnionFightFactoryCycleState, error) {
	if factory.PausedAt != 0 || factory.RemainingMillis != 0 {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	var cycle db.P13UnionFightFactoryCycleState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cycle, "match_id=? AND factory_guid=?", factory.MatchID, factory.GUID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	exists := err == nil
	if !exists && factory.CompletedAt <= 0 {
		return nil, nil
	}
	var offer db.P13UnionFightFactoryCycleOffer
	err = tx.First(&offer, "match_id=? AND factory_guid=?", factory.MatchID, factory.GUID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && !exists {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if offer.SourceHash != p13UFFactoryCycleOfferHash(&offer) || !p13UFFactoryCapacityValid(offer.Rule, offer.BufferCapacity, offer.MaxAlive) || offer.CraftMillis != 120000 {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	if !exists {
		cycle = db.P13UnionFightFactoryCycleState{MatchID: factory.MatchID, FactoryGUID: factory.GUID, Generation: 2, NextBornAt: factory.CompletedAt + offer.CraftMillis, OfferHash: offer.SourceHash}
		if offer.Rule == constUtil.P13UnionFightFactoryBufferRule {
			cycle.ProductionStartedAt = factory.CompletedAt
		}
		cycle.StateHash = p13UFFactoryCycleStateHash(&cycle)
	} else if cycle.StateHash != p13UFFactoryCycleStateHash(&cycle) || cycle.OfferHash != offer.SourceHash || cycle.PausedAt != 0 || cycle.RemainingMillis != 0 {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	if offer.Rule == constUtil.P13UnionFightFactoryBufferRule && !p13UFFactoryBufferStateValid(&cycle, &offer, factory.CompletedAt) {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	return &cycle, nil
}

// p13UFGuardBuildingAdvance restores only frozen guard inputs, with bounded
// reads and row locks supplied by the match maintenance transaction. Paused
// factory cycles preserve legacy full-craft recovery; the mirror rule resumes
// the frozen remainder from its original logical recovery clock.
func (s *GameService) p13UFGuardBuildingAdvance(tx *gorm.DB, match *db.P13UnionFightMatch, now time.Time) ([]proto.Message, error) {
	if tx == nil || match == nil {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	if now.UnixMilli() >= match.EndAt {
		return nil, nil
	}
	var rows []db.P13UnionFightGuardState
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND recover_at>0 AND recover_at<=?", match.ID, now.UnixMilli()).Order("guard_guid").Limit(16).Find(&rows).Error; err != nil {
		return nil, err
	}
	var messages []proto.Message
	for i := range rows {
		state := &rows[i]
		if _, err := p13UFGuardStateFrame(state); err != nil {
			return nil, err
		}
		if state.PendingAttemptID != "" || state.Generation == math.MaxInt64 || (state.BuildingKind != "tower" && state.BuildingKind != "factory") || state.BuildingEvent != state.BuildingKind+"-defeated" {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		var guard pb.InstUFGuard
		if proto.Unmarshal(state.InitialFrame, &guard) != nil || guard.GetGuid() != state.GuardGUID || guard.GetBindGuid() != state.BindGUID || guard.GetCamp() != int32(state.Camp) || guard.GetUFGuardId() != state.GuardID || guard.GetState() != 0 {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		recoverAt := state.RecoverAt
		state.Frame = append([]byte(nil), state.InitialFrame...)
		state.FrameHash = state.InitialHash
		state.DefeatedAt = 0
		state.RecoverAt = 0
		state.BuildingEvent = ""
		state.Generation++
		state.SourceHash = p13UFGuardStateHash(state)
		if state.BuildingKind == "factory" {
			factory, frame, definition, err := s.p13UFFactoryRead(tx, match, state.BindGUID)
			if err != nil {
				return nil, err
			}
			if factory.Camp != state.Camp || frame.GetState() != 2 || frame.GetGuard().GetGuid() != state.GuardGUID {
				return nil, errors.New(constUtil.P13UFGuardConflict)
			}
			frame.State = proto.Int32(0)
			frame.StateLeftTime = proto.Float32(0)
			frame.StateTotalTime = proto.Float32(0)
			frame.Guard = &guard
			if state.GuardRule == constUtil.P13UFGuardMirrorRule && factory.PausedAt > 0 {
				if err = p13UFGuardFactoryResumeTimer(factory, recoverAt); err != nil {
					return nil, err
				}
			} else if factory.CompletedAt == 0 && factory.ChariotHP > 0 && frame.GetIsOpen() == 1 {
				if err = s.p13UFFactoryStart(factory, definition, now.UnixMilli()); err != nil {
					return nil, err
				}
			}
			var cycle db.P13UnionFightFactoryCycleState
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cycle, "match_id=? AND factory_guid=?", match.ID, state.BindGUID).Error
			if err == nil {
				var offer db.P13UnionFightFactoryCycleOffer
				if err = tx.First(&offer, "match_id=? AND factory_guid=?", match.ID, state.BindGUID).Error; err != nil {
					return nil, err
				}
				if cycle.StateHash != p13UFFactoryCycleStateHash(&cycle) || cycle.OfferHash != offer.SourceHash || offer.SourceHash != p13UFFactoryCycleOfferHash(&offer) || !p13UFFactoryCapacityValid(offer.Rule, offer.BufferCapacity, offer.MaxAlive) || offer.CraftMillis <= 0 {
					return nil, errors.New(constUtil.P13UFGuardConflict)
				}
				if cycle.ClosedAt == 0 {
					if cycle.BufferedAt > 0 {
						if offer.Rule != constUtil.P13UnionFightFactoryBufferRule || !p13UFFactoryBufferStateValid(&cycle, &offer, factory.CompletedAt) {
							return nil, errors.New(constUtil.P13UFGuardConflict)
						}
					} else if state.GuardRule == constUtil.P13UFGuardMirrorRule && cycle.PausedAt > 0 {
						if err = p13UFGuardFactoryResumeCycle(&cycle, recoverAt); err != nil {
							return nil, err
						}
					} else {
						cycle.NextBornAt = now.UnixMilli() + offer.CraftMillis
						if offer.Rule == constUtil.P13UnionFightFactoryBufferRule {
							cycle.ProductionStartedAt = now.UnixMilli()
						}
					}
					cycle.AnimationPlayed = false
					cycle.StateHash = p13UFFactoryCycleStateHash(&cycle)
					if err = tx.Save(&cycle).Error; err != nil {
						return nil, err
					}
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			} else if factory.CompletedAt > 0 {
				var offer db.P13UnionFightFactoryCycleOffer
				err = tx.First(&offer, "match_id=? AND factory_guid=?", match.ID, state.BindGUID).Error
				if err == nil {
					if offer.SourceHash != p13UFFactoryCycleOfferHash(&offer) || !p13UFFactoryCapacityValid(offer.Rule, offer.BufferCapacity, offer.MaxAlive) || offer.CraftMillis <= 0 {
						return nil, errors.New(constUtil.P13UFGuardConflict)
					}
					cycle = db.P13UnionFightFactoryCycleState{MatchID: match.ID, FactoryGUID: state.BindGUID, Generation: 2, NextBornAt: now.UnixMilli() + offer.CraftMillis, OfferHash: offer.SourceHash}
					if offer.Rule == constUtil.P13UnionFightFactoryBufferRule {
						cycle.ProductionStartedAt = now.UnixMilli()
					}
					cycle.StateHash = p13UFFactoryCycleStateHash(&cycle)
					if err = tx.Create(&cycle).Error; err != nil {
						return nil, err
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, err
				}
			}
			if err = p13UFFactorySave(tx, factory, frame); err != nil {
				return nil, err
			}
			messages = append(messages, &pb.SCUFBuildingRefresh{FactoryInfos: []*pb.InstUFCariotFactory{frame}})
		} else {
			snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
			if err != nil {
				return nil, err
			}
			var buildings pb.SCUFBuildingRefresh
			if proto.Unmarshal(snapshot.Buildings, &buildings) != nil {
				return nil, errors.New(constUtil.P13UFGuardConflict)
			}
			found := false
			for _, tower := range buildings.TowerInfos {
				if tower.GetGuid() == state.BindGUID && tower.GetGuard().GetGuid() == state.GuardGUID {
					tower.Guard = &guard
					messages = append(messages, &pb.SCUFBuildingRefresh{TowerInfos: []*pb.InstUFTower{tower}})
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New(constUtil.P13UFGuardConflict)
			}
		}
		if err := tx.Save(state).Error; err != nil {
			return nil, err
		}
	}
	return messages, nil
}
