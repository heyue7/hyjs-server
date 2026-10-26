package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"
)

type p13UFGuardBattleRules struct {
	Policy                                    db.P13UnionFightGuardBattlePolicy
	Proof, LocalRule, BuildingKind, GateProof string
	DurationMillis, RecoverySeconds           int64
	MatchStartAt                              int64 `json:",omitempty"`
	MatchEndAt                                int64 `json:",omitempty"`
	ReviveSeconds                             []int64
}

type p13UFGuardBaseGateCheck func(*gorm.DB, *db.P13UnionFightFieldPlayer, *pb.InstUFGuard) (string, error)

func (s *GameService) InitializeP13UnionFightGuardBattle(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightGuardBattlePolicy{}, &db.P13UnionFightGuardState{}, &db.P13UnionFightGuardBattleAttempt{}); err != nil {
		return err
	}
	return tx.Where("id=?", 1).FirstOrCreate(&db.P13UnionFightGuardBattlePolicy{ID: 1, Version: "guard-pending-v1-default-off"}).Error
}
func p13UFGuardStateHash(row *db.P13UnionFightGuardState) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFGuardBattleHash(row *db.P13UnionFightGuardBattleAttempt) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFGuardStateFrame(row *db.P13UnionFightGuardState) (*pb.InstUFGuard, error) {
	frame := &pb.InstUFGuard{}
	if row == nil || !p13UFGuardTemplateRuleKnown(row.GuardRule) || row.Generation < 1 || row.SourceHash != p13UFGuardStateHash(row) || row.FrameHash != p6Digest(row.Frame) || row.InitialHash != p6Digest(row.InitialFrame) || proto.Unmarshal(row.Frame, frame) != nil || frame.GetGuid() != row.GuardGUID || frame.GetUFGuardId() != row.GuardID || frame.GetCamp() != int32(row.Camp) || frame.GetBindGuid() != row.BindGUID {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	return frame, nil
}
func p13UFGuardBinding(buildings *pb.SCUFBuildingRefresh, guid int64) string {
	for _, b := range buildings.BaseCampInfos {
		if b.GetGuard().GetGuid() == guid {
			return "base"
		}
	}
	for _, b := range buildings.TowerInfos {
		if b.GetGuard().GetGuid() == guid {
			return "tower"
		}
	}
	for _, b := range buildings.FactoryInfos {
		if b.GetGuard().GetGuid() == guid {
			return "factory"
		}
	}
	return ""
}

// Overlay never rewrites the immutable field or manufactures a recovered guard.
// Recovery and factory/base integration must consume the durable building event.
func p13UFGuardBattleOverlay(tx *gorm.DB, scope string, buildings *pb.SCUFBuildingRefresh) error {
	var rows []db.P13UnionFightGuardState
	if err := tx.Where("match_id=?", scope).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		state := &rows[i]
		frame, err := p13UFGuardStateFrame(state)
		if err != nil {
			return err
		}
		if p13UFGuardBinding(buildings, state.GuardGUID) != state.BuildingKind {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		found := false
		for _, b := range buildings.BaseCampInfos {
			if b.GetGuard().GetGuid() == state.GuardGUID {
				b.Guard = frame
				found = true
			}
		}
		for _, b := range buildings.TowerInfos {
			if b.GetGuard().GetGuid() == state.GuardGUID {
				b.Guard = frame
				if state.DefeatedAt > 0 {
					b.State = proto.Int32(1)
				}
				found = true
			}
		}
		for _, b := range buildings.FactoryInfos {
			if b.GetGuard().GetGuid() == state.GuardGUID {
				b.Guard = frame
				if state.DefeatedAt > 0 {
					b.State = proto.Int32(2)
				}
				found = true
			}
		}
		if !found {
			return errors.New(constUtil.P13UFGuardConflict)
		}
	}
	return nil
}
func p13UFGuardRulesValid(r *p13UFGuardBattleRules) bool {
	if r == nil || !r.Policy.Enabled || r.Policy.ID != 1 || strings.TrimSpace(r.Policy.Version) == "" || strings.TrimSpace(r.Policy.Source) == "" || r.Policy.PublishedAt <= 0 || r.Policy.StartsAt < r.Policy.PublishedAt || r.Policy.EndsAt <= r.Policy.StartsAt || (r.LocalRule != constUtil.P13UFGuardLocalRule && r.LocalRule != constUtil.P13UFGuardMirrorRule) || r.DurationMillis != 5000 || len(r.ReviveSeconds) < 1 || len(r.ReviveSeconds) > 64 {
		return false
	}
	if r.LocalRule == constUtil.P13UFGuardMirrorRule && (r.MatchStartAt <= 0 || r.MatchEndAt <= r.MatchStartAt) {
		return false
	}
	if r.LocalRule == constUtil.P13UFGuardLocalRule && (r.MatchStartAt != 0 || r.MatchEndAt != 0) {
		return false
	}
	if r.BuildingKind != "base" && r.BuildingKind != "tower" && r.BuildingKind != "factory" {
		return false
	}
	if r.BuildingKind == "base" && len(r.GateProof) != 64 {
		return false
	}
	if r.BuildingKind == "base" {
		if _, err := hex.DecodeString(r.GateProof); err != nil {
			return false
		}
	}
	for _, v := range r.ReviveSeconds {
		if v < 1 || v > 86400 {
			return false
		}
	}
	return (r.BuildingKind == "base" && r.RecoverySeconds == 0) || (r.BuildingKind == "tower" && r.RecoverySeconds == 120) || (r.BuildingKind == "factory" && r.RecoverySeconds == 300)
}

// New independent producer. Disabled policy falls through to the old audit;
// enabled but unproved policy refuses without capture, state or wallet changes.
func (s *GameService) p13UFTryGuardBattle(ctx context.Context, id int64, request *pb.CSUFPlayerAskFight, stamp int64, now time.Time, baseCheck ...p13UFGuardBaseGateCheck) ([]proto.Message, bool, error) {
	handled := false
	var reply []proto.Message
	scope := ""
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if request == nil || request.GetTargetGuid() <= 0 || id <= 0 || stamp < 0 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var policy db.P13UnionFightGuardBattlePolicy
		if err := tx.First(&policy, 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		// Historical positive retry identity is resolved before new qualification.
		var old db.P13UnionFightGuardBattleAttempt
		query := tx.Where("player_id=? AND client_stamp=?", id, stamp)
		if stamp == 0 {
			query = query.Where("guard_guid=? AND status=?", request.GetTargetGuid(), constUtil.P13UFGuardPending)
		}
		if err := query.First(&old).Error; err == nil {
			handled = true
			if old.GuardGUID != request.GetTargetGuid() {
				return errors.New(constUtil.P13UFGuardConflict)
			}
			var e error
			reply, e = s.p13UFGuardBattleReceipt(tx, &old)
			return e
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !policy.Enabled {
			return nil
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		guards, err := p13UFGuardAuditUnits(buildings)
		if err != nil {
			return err
		}
		initial := guards[request.GetTargetGuid()]
		if initial == nil {
			return nil
		}
		// Old audit receipts stay on their old refusal route, including a live policy change.
		var audits int64
		if err = tx.Model(&db.P13UnionFightGuardAuditAttempt{}).Where("match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Count(&audits).Error; err != nil {
			return err
		}
		if audits > 0 {
			return nil
		}
		handled = true
		proof, err := p6UFGuardProofForRule(snapshot.GuardRule)
		if err != nil {
			return err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget || actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli()+5000 > match.EndAt {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		if frame.GetKillNum() < 0 || frame.GetDeadNum() < 0 || frame.GetDeadNum() == math.MaxInt32 || !combatFinite(frame.GetOwnScore()) || !combatFinite(frame.GetOwnToken()) || frame.GetOwnScore() < 0 || frame.GetOwnToken() < 0 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		if err = p13UFPlayerBattleAction(tx, actor, frame); err != nil {
			return err
		}
		if err = p13UFGuardBattleAction(tx, actor, frame); err != nil {
			return err
		}
		if err = p13UFTowerPendingLife(tx, match.ID, id, frame.GetDeadNum()); err != nil {
			return err
		}
		var quote db.P13UnionFightGuardAuditOffer
		var common db.P13UnionFightBattleAuditOffer
		if err = tx.First(&quote, "match_id=? AND guard_guid=?", match.ID, initial.GetGuid()).Error; err != nil {
			return err
		}
		if err = tx.First(&common, "match_id=?", match.ID).Error; err != nil {
			return err
		}
		if !p13UFGuardTemplateRuleKnown(quote.GuardRule) || quote.GuardRule != snapshot.GuardRule || quote.SourceHash != p13UFGuardOfferHash(&quote) || quote.BattleOfferHash != common.SourceHash || !p13UFBattleAuditQuoteValid(&common) {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		var state db.P13UnionFightGuardState
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "match_id=? AND guard_guid=?", match.ID, initial.GetGuid()).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			raw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(initial)
			state = db.P13UnionFightGuardState{GuardRule: quote.GuardRule, MatchID: match.ID, GuardGUID: initial.GetGuid(), GuardID: quote.GuardID, HeroID: quote.HeroID, Camp: quote.Camp, BindGUID: quote.BindGUID, BuildingKind: p13UFGuardBinding(buildings, initial.GetGuid()), Generation: 1, OfferHash: quote.SourceHash, InitialFrame: raw, InitialHash: p6Digest(raw), Frame: raw, FrameHash: p6Digest(raw)}
			state.SourceHash = p13UFGuardStateHash(&state)
			if err = tx.Create(&state).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		guard, err := p13UFGuardStateFrame(&state)
		if err != nil {
			return err
		}
		if state.GuardRule != quote.GuardRule || state.OfferHash != quote.SourceHash || state.PendingAttemptID != "" || state.DefeatedAt != 0 || guard.GetState() != 0 || guard.GetCamp() == int32(actor.Camp) || p13UFResourceDistance(actor, guard.GetInitPosition().GetX(), guard.GetInitPosition().GetY(), guard.GetInitPosition().GetZ()) > quote.ArriveRange {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		rules := p13UFGuardBattleRules{Policy: policy, Proof: proof, LocalRule: constUtil.P13UFGuardLocalRule, BuildingKind: state.BuildingKind, DurationMillis: 5000}
		if quote.GuardRule == constUtil.P13UFGuardMirrorRule {
			rules.LocalRule = constUtil.P13UFGuardMirrorRule
			rules.MatchStartAt, rules.MatchEndAt = match.StartAt, match.EndAt
		}
		if state.BuildingKind == "base" {
			if len(baseCheck) != 1 || baseCheck[0] == nil {
				return errors.New("基地守卫门通行认证尚未集成")
			}
			rules.GateProof, err = baseCheck[0](tx, actor, guard)
			if err != nil {
				return err
			}
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		duration, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightDuration")["NameValue"], 64)
		if err != nil || duration != 5 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		for _, v := range strings.Split(original.table("GameConfigure", "StaticName", "UnionFightReviveTime")["NameValue"], ",") {
			seconds, e := strconv.ParseInt(v, 10, 64)
			if e != nil {
				return e
			}
			rules.ReviveSeconds = append(rules.ReviveSeconds, seconds)
		}
		switch state.BuildingKind {
		case "tower":
			value, e := strconv.ParseInt(original.table("UnionFightTower", "UFGuardId", state.GuardID)["RecoveryTime"], 10, 64)
			if e != nil {
				return e
			}
			rules.RecoverySeconds = value
		case "factory":
			value, e := strconv.ParseInt(original.table("UnionFightFactory", "UFGuardId", state.GuardID)["RebuildTime"], 10, 64)
			if e != nil {
				return e
			}
			rules.RecoverySeconds = value
		}
		if !p13UFGuardRulesValid(&rules) || policy.PublishedAt > actor.JoinedAt || now.UnixMilli() < policy.StartsAt || now.UnixMilli()+5000 >= policy.EndsAt {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		internal := stamp
		if internal == 0 {
			n, e := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
			if e != nil {
				return e
			}
			internal = n.Int64() + 1
		}
		row := db.P13UnionFightGuardBattleAttempt{BuildingKind: rules.BuildingKind, ID: p6Digest([]byte(fmt.Sprintf("%s/%d/%d/guard", match.ID, id, internal))), MatchID: match.ID, PlayerID: id, GuardGUID: guard.GetGuid(), GuardGeneration: state.Generation, Stamp: internal, ClientStamp: stamp, Status: constUtil.P13UFGuardPending, CalculationStage: constUtil.P13UFGuardCaptured, OfferHash: quote.SourceHash, CreatedAt: now.UnixMilli(), DueAt: now.UnixMilli() + 5000}
		row.FightID = strconv.FormatInt(p13UFFieldGUID(match.ID, "guard-battle", row.ID), 10)
		requestRaw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		row.RequestHash = p6Digest(requestRaw)
		row.Rules, _ = json.Marshal(rules)
		row.PlayerBefore, _ = proto.MarshalOptions{Deterministic: true}.Marshal(frame)
		row.GuardBefore, _ = proto.MarshalOptions{Deterministic: true}.Marshal(guard)
		attacker, applied, err := s.p13UFFieldBattle(tx, id, now)
		if err != nil {
			return err
		}
		monster, err := p13UFGuardAuditMonster(&quote, guard)
		if err != nil {
			return err
		}
		applied["guard-battle"] = []string{constUtil.P13UFGuardBattleSource + ";match=" + match.ID + ";attempt=" + row.ID + ";offer=" + quote.SourceHash + ";guard=" + strconv.FormatInt(row.GuardGUID, 10) + ";playerFrame=" + p6Digest(row.PlayerBefore) + ";guardFrame=" + p6Digest(row.GuardBefore)}
		engine, err := s.p13UFBattleAuditFrozenEngine(tx, match, applied)
		if err != nil {
			return err
		}
		seedRaw, _ := hex.DecodeString(p6Digest([]byte(row.FightID + "/" + common.SourceHash + "/" + quote.SourceHash + "/" + strconv.FormatInt(common.SeedSalt, 10))))
		input, err := engine.FreezeBattleInput(model.FrozenBattleInput{Attacker: attacker, Monster: monster, Seed: int64(binary.BigEndian.Uint32(seedRaw[:4]) & 0x7fffffff), MaxRound: 40, BattleType: 14, AttackerDamageFactor: common.AttackerDamageFactor, DefenderDamageFactor: common.DefenderDamageFactor, AppliedProperties: applied, ClientVersion: match.ClientVersion})
		if err != nil {
			return err
		}
		if err = validateP6UFGuardMechanismScope(input); err != nil {
			return err
		}
		if _, err = engine.ValidateP6UFGuardMechanisms(ctx, input, id, frame, guard, &quote); err != nil {
			return err
		}
		row.P6RecordID, err = engine.CaptureBattleReplay(ctx, tx, id, constUtil.P13UFGuardBattleSource, row.FightID, *input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
			return err
		}
		if record.P13MatchID != match.ID {
			return errors.New("新守卫来源未完成严格比赛归属登记")
		}
		row.InputHash = record.InputHash
		frame.State = proto.Int32(1)
		frame.LeftTime = proto.Float32(5)
		frame.TotalTime = proto.Float32(5)
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		guard.State = proto.Int32(1)
		state.Frame, _ = proto.MarshalOptions{Deterministic: true}.Marshal(guard)
		state.FrameHash = p6Digest(state.Frame)
		state.PendingAttemptID = row.ID
		state.SourceHash = p13UFGuardStateHash(&state)
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		row.ReservationHash = state.SourceHash
		row.SourceHash = p13UFGuardBattleHash(&row)
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightSettlementIntent{MatchID: match.ID, PlayerID: id, TargetID: guard.GetGuid(), Stamp: stamp, ObservedAt: now.UnixMilli(), Source: constUtil.P13UFGuardBattleSource}).Error; err != nil {
			return err
		}
		reply = p13UFGuardBegin(&row)
		scope = match.ID
		fresh = true
		return nil
	})
	if err == nil && fresh {
		for _, message := range reply {
			s.p13UFFieldBroadcast(ctx, scope, id, message)
		}
	}
	return reply, handled, err
}
func p13UFGuardBegin(row *db.P13UnionFightGuardBattleAttempt) []proto.Message {
	return []proto.Message{&pb.SCUFPlayerFightBegin{Guid: proto.Int64(row.PlayerID), TargetGuid: proto.Int64(row.GuardGUID)}, &pb.SCUFGuardFightBegin{Guid: proto.Int64(row.GuardGUID)}}
}

// Verification closes both independent journals and the exact pre-result
// identity. It does not inspect current enabled policy or recompute a battle.
func (s *GameService) p13UFGuardBattleVerify(tx *gorm.DB, row *db.P13UnionFightGuardBattleAttempt) (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, *p13UFGuardBattleRules, error) {
	if row == nil {
		return nil, nil, nil, errors.New(constUtil.P13UFGuardConflict)
	}
	var offer db.P13UnionFightGuardAuditOffer
	if err := tx.First(&offer, "match_id=? AND guard_guid=?", row.MatchID, row.GuardGUID).Error; err != nil {
		return nil, nil, nil, err
	}
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, nil, nil, err
	}
	input, out, rules, err := s.p13UFGuardBattleVerifyLoaded(tx.Statement.Context, row, &record, &offer)
	if err != nil {
		return nil, nil, nil, err
	}
	if rules.LocalRule == constUtil.P13UFGuardMirrorRule {
		var match db.P13UnionFightMatch
		if err = tx.Select("id,start_at,end_at").First(&match, "id=?", row.MatchID).Error; err != nil {
			return nil, nil, nil, err
		}
		if rules.MatchStartAt != match.StartAt || rules.MatchEndAt != match.EndAt {
			return nil, nil, nil, errors.New(constUtil.P13UFGuardConflict)
		}
	}
	if row.Status == constUtil.P13UFGuardPending {
		var state db.P13UnionFightGuardState
		if err = tx.First(&state, "match_id=? AND guard_guid=?", row.MatchID, row.GuardGUID).Error; err != nil {
			return nil, nil, nil, err
		}
		current, readErr := p13UFGuardStateFrame(&state)
		before := &pb.InstUFGuard{}
		if readErr != nil || proto.Unmarshal(row.GuardBefore, before) != nil || state.PendingAttemptID != row.ID || state.Generation != row.GuardGeneration || state.SourceHash != row.ReservationHash || current.GetState() != 1 || !proto.Equal(current.FightInfo, before.FightInfo) {
			return nil, nil, nil, errors.New(constUtil.P13UFGuardConflict)
		}
	}
	return input, out, rules, nil
}

// VerifyLoaded authenticates immutable inputs without per-attempt SQL. The
// transactional wrapper separately checks pending reservation ownership.
func (s *GameService) p13UFGuardBattleVerifyLoaded(ctx context.Context, row *db.P13UnionFightGuardBattleAttempt, record *db.P6BattleRecord, offer *db.P13UnionFightGuardAuditOffer) (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, *p13UFGuardBattleRules, error) {
	reject := func() (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, *p13UFGuardBattleRules, error) {
		return nil, nil, nil, errors.New(constUtil.P13UFGuardConflict)
	}
	if row == nil || row.ID != p6Digest([]byte(fmt.Sprintf("%s/%d/%d/guard", row.MatchID, row.PlayerID, row.Stamp))) || row.SourceHash != p13UFGuardBattleHash(row) || row.PlayerID <= 0 || row.GuardGUID <= 0 || row.GuardGeneration < 1 || row.Stamp <= 0 || row.ClientStamp < 0 || (row.ClientStamp > 0 && row.ClientStamp != row.Stamp) || row.NextCalculationAt < 0 || row.CalculationFailures < 0 || row.CreatedAt <= 0 || row.DueAt != row.CreatedAt+5000 || (row.Status != constUtil.P13UFGuardPending && row.Status != constUtil.P13UFGuardSettled) {
		return reject()
	}
	if row.Status == constUtil.P13UFGuardPending && (row.BuildingAppliedAt != 0 || len(row.BuildingProof) > 0) {
		return reject()
	}
	var rules p13UFGuardBattleRules
	if json.Unmarshal(row.Rules, &rules) != nil || row.BuildingKind != "" && row.BuildingKind != rules.BuildingKind || !p13UFGuardRulesValid(&rules) || rules.Policy.PublishedAt > row.CreatedAt || row.CreatedAt < rules.Policy.StartsAt || row.DueAt >= rules.Policy.EndsAt {
		return reject()
	}
	if err := p13UFGuardRulesProof(&rules); err != nil {
		return nil, nil, nil, err
	}
	var player pb.InstUFPlayer
	var guard pb.InstUFGuard
	if proto.Unmarshal(row.PlayerBefore, &player) != nil || proto.Unmarshal(row.GuardBefore, &guard) != nil || player.GetGuid() != row.PlayerID || guard.GetGuid() != row.GuardGUID || player.GetCamp() == guard.GetCamp() || guard.GetState() != 0 {
		return reject()
	}
	request, _ := proto.MarshalOptions{Deterministic: true}.Marshal(&pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(row.GuardGUID)})
	if row.RequestHash != p6Digest(request) || row.FightID != strconv.FormatInt(p13UFFieldGUID(row.MatchID, "guard-battle", row.ID), 10) {
		return reject()
	}
	if record == nil || offer == nil || offer.MatchID != row.MatchID || offer.GuardGUID != row.GuardGUID || record.ID != row.P6RecordID {
		return reject()
	}
	expectedRule := constUtil.P13UFGuardLocalRule
	if offer.GuardRule == constUtil.P13UFGuardMirrorRule {
		expectedRule = constUtil.P13UFGuardMirrorRule
	}
	if !p13UFGuardTemplateRuleKnown(offer.GuardRule) || rules.LocalRule != expectedRule || offer.SourceHash != p13UFGuardOfferHash(offer) || offer.SourceHash != row.OfferHash {
		return reject()
	}
	if rules.LocalRule == constUtil.P13UFGuardMirrorRule && (row.CreatedAt < rules.MatchStartAt || row.DueAt > rules.MatchEndAt || rules.BuildingKind == "base" && row.CreatedAt < rules.MatchStartAt+(rules.MatchEndAt-rules.MatchStartAt)/2) {
		return reject()
	}
	input, e := decodeP6Record(record)
	if e != nil {
		return nil, nil, nil, e
	}
	if record.Source != constUtil.P13UFGuardBattleSource || record.SourceFight != row.FightID || record.PlayerID != row.PlayerID || record.P13MatchID != row.MatchID || record.InputHash != row.InputHash || input.Attacker.GetPlayerId() != row.PlayerID || input.Monster.GetMonsterId() != row.GuardGUID {
		return reject()
	}
	if e = validateP6UFGuardMechanismScope(input); e != nil {
		return nil, nil, nil, e
	}
	expected, err := p13UFGuardAuditMonster(offer, &guard)
	if err != nil {
		return nil, nil, nil, err
	}
	if !proto.Equal(expected, input.Monster) {
		return reject()
	}
	tag := constUtil.P13UFGuardBattleSource + ";match=" + row.MatchID + ";attempt=" + row.ID + ";offer=" + row.OfferHash + ";guard=" + strconv.FormatInt(row.GuardGUID, 10) + ";playerFrame=" + p6Digest(row.PlayerBefore) + ";guardFrame=" + p6Digest(row.GuardBefore)
	if len(input.AppliedProperties["guard-battle"]) != 1 || input.AppliedProperties["guard-battle"][0] != tag || !p13UFGuardPlayerFrameMatches(input, &player, &rules) {
		return reject()
	}
	if _, e = s.ValidateP6UFGuardMechanisms(ctx, input, row.PlayerID, &player, &guard, offer); e != nil {
		return nil, nil, nil, e
	}

	if row.CalculationStage == constUtil.P13UFGuardCaptured {
		if record.Status != constUtil.P6BattleCaptured || record.Authoritative || record.FinishedAt != 0 || len(record.Outcome) != 0 || len(record.Operates) != 0 || record.OutcomeHash != "" || record.ReplayHash != "" || len(row.Terminal) != 0 || row.TerminalHash != "" || row.Status != constUtil.P13UFGuardPending || row.SettledAt != 0 || len(row.PlayerAfter) != 0 || len(row.GuardAfter) != 0 || row.PlayerDied || row.GuardDefeated || row.ReviveSeconds != 0 || row.BuildingEvent != "" {
			return reject()
		}
		return input, nil, &rules, nil
	}
	if row.CalculationStage != constUtil.P13UFGuardComputed || record.Status != constUtil.P6BattleComputed || !record.Authoritative || row.TerminalHash != p6Digest(row.Terminal) || record.FinishedAt != row.CreatedAt {
		return reject()
	}
	var out model.CombatFinalHPOutcome
	if json.Unmarshal(row.Terminal, &out) != nil {
		return reject()
	}
	if e = validateP6UFGuardTerminalForRule(input, &out, &rules); e != nil {
		return nil, nil, nil, e
	}
	raw, _ := json.Marshal(out.Outcome)
	ops, _ := proto.MarshalOptions{Deterministic: true}.Marshal(out.Outcome.Operates)
	if p6Digest(raw) != record.OutcomeHash || p6Digest(record.Outcome) != record.OutcomeHash || !bytes.Equal(record.Operates, ops) || record.ClientResult != record.ServerResult || record.ServerResult != p6PlaybackResult(out.Outcome.Winner) || record.ReplayHash != p6Digest(append(append([]byte(nil), ops...), byte(record.ServerResult))) {
		return reject()
	}
	if row.Status == constUtil.P13UFGuardPending {
		if row.SettledAt != 0 || len(row.PlayerAfter) != 0 || len(row.GuardAfter) != 0 || row.PlayerDied || row.GuardDefeated || row.ReviveSeconds != 0 || row.BuildingEvent != "" {
			return reject()
		}
	} else {
		expected, expectedGuard, dead, defeated, revive, mapErr := p13UFGuardBattleMap(input, &out, &player, &guard, &rules, row.DueAt, row.SettledAt)
		var after pb.InstUFPlayer
		var guardAfter pb.InstUFGuard
		event := ""
		if defeated {
			event = rules.BuildingKind + "-defeated"
		}
		if mapErr != nil || proto.Unmarshal(row.PlayerAfter, &after) != nil || proto.Unmarshal(row.GuardAfter, &guardAfter) != nil || row.PlayerDied != dead || row.GuardDefeated != defeated || row.ReviveSeconds != revive || row.BuildingEvent != event || !proto.Equal(after.FightInfo, expected.FightInfo) || after.GetState() != expected.GetState() || after.GetDeadNum() != expected.GetDeadNum() || after.GetKillNum() != expected.GetKillNum() || after.GetOwnScore() != expected.GetOwnScore() || after.GetOwnToken() != expected.GetOwnToken() || after.GetLeftTime() != expected.GetLeftTime() || after.GetTotalTime() != expected.GetTotalTime() || !proto.Equal(&guardAfter, expectedGuard) {
			return reject()
		}
	}
	return input, &out, &rules, nil
}

func (s *GameService) p13UFGuardBattleReceipt(tx *gorm.DB, row *db.P13UnionFightGuardBattleAttempt) ([]proto.Message, error) {
	if _, _, _, err := s.p13UFGuardBattleVerify(tx, row); err != nil {
		return nil, err
	}
	if row.Status == constUtil.P13UFGuardPending {
		var actor db.P13UnionFightFieldPlayer
		if err := tx.First(&actor, "match_id=? AND player_id=? AND left_at=0", row.MatchID, row.PlayerID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		var profile db.Profile
		if err := tx.First(&profile, row.PlayerID).Error; err != nil {
			return nil, err
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			return nil, err
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return nil, err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget || frame.GetState() != 1 {
			return nil, nil
		}
		return p13UFGuardBegin(row), nil
	}
	// A historical retry never replays stale HP over a later fight. Cold state
	// projections belong to Current/Overlay; the terminal ledger remains queryable.
	return nil, nil
}

func (s *GameService) RunP13UnionFightGuardBattleWorker(ctx context.Context, report func(error)) error {
	ticker := time.NewTicker(constUtil.P13UFGuardWorkerInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.ComputeP13UnionFightGuardBattles(ctx, p13UFNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if report == nil {
				return err
			}
			report(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Compute is called by the independent worker, never by request/field Tick.
// The batch uses a bounded number of calculators and joins them on cancellation.
func (s *GameService) ComputeP13UnionFightGuardBattles(ctx context.Context, now time.Time) error {
	var rows []db.P13UnionFightGuardBattleAttempt
	if err := p13UFCombatOpenCandidates(db.GetIns().WithContext(ctx), "p13_union_fight_guard_battle_attempts", now).Where("status=? AND calculation_stage=? AND quarantined=? AND next_calculation_at<=?", constUtil.P13UFGuardPending, constUtil.P13UFGuardCaptured, false, now.UnixMilli()).Order("next_calculation_at,created_at,id").Limit(constUtil.P13UFGuardWorkerBatch).Find(&rows).Error; err != nil {
		return err
	}
	jobs := make(chan db.P13UnionFightGuardBattleAttempt)
	failures := make(chan error, len(rows))
	var group sync.WaitGroup
	for range constUtil.P13UFGuardWorkerConcurrency {
		group.Add(1)
		go func() {
			defer group.Done()
			for row := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if err := s.p13UFGuardCalculate(ctx, &row, now); err != nil {
					failures <- fmt.Errorf("guard %s: %w", row.ID, err)
				}
			}
		}()
	}
	for _, row := range rows {
		select {
		case jobs <- row:
		case <-ctx.Done():
		}
	}
	close(jobs)
	group.Wait()
	close(failures)
	var errs []error
	for e := range failures {
		errs = append(errs, e)
	}
	if ctx.Err() != nil {
		errs = append(errs, ctx.Err())
	}
	return errors.Join(errs...)
}
func (s *GameService) p13UFGuardCalculate(ctx context.Context, row *db.P13UnionFightGuardBattleAttempt, now time.Time) error {
	input, _, rules, err := s.p13UFGuardBattleVerify(db.GetIns().WithContext(ctx), row)
	if err != nil {
		if ctx.Err() == nil && (errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == constUtil.P13UFGuardConflict || err.Error() == constUtil.P6ReplayConflict || err.Error() == constUtil.CombatInvalid || err.Error() == constUtil.CombatUnsupported) {
			qErr := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				stop, e := p13UFCombatCommitGate(tx, row.MatchID, p13UFNow(ctx))
				if e != nil || stop {
					return e
				}
				return tx.Model(&db.P13UnionFightGuardBattleAttempt{}).Where("id=? AND source_hash=? AND status=? AND calculation_stage=?", row.ID, row.SourceHash, constUtil.P13UFGuardPending, constUtil.P13UFGuardCaptured).Update("quarantined", true).Error
			})
			return errors.Join(err, qErr)
		}
		return err
	}
	terminal, err := s.computeP6UFGuardForRule(ctx, input, rules)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return errors.Join(err, s.p13UFGuardRetry(ctx, row, now))
		}
		return err
	}
	if err = validateP6UFGuardTerminalForRule(input, terminal, rules); err != nil {
		return errors.Join(err, s.p13UFGuardRetry(ctx, row, now))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stop, gateErr := p13UFCombatCommitGate(tx, row.MatchID, p13UFNow(ctx))
		if gateErr != nil || stop {
			return gateErr
		}
		var current db.P13UnionFightGuardBattleAttempt
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", row.ID).Error; e != nil {
			return e
		}
		if current.SourceHash != row.SourceHash || current.CalculationStage != constUtil.P13UFGuardCaptured {
			return nil
		}
		if _, _, _, e := s.p13UFGuardBattleVerify(tx, &current); e != nil {
			return e
		}
		encoded, _ := json.Marshal(terminal)
		outcome, _ := json.Marshal(terminal.Outcome)
		ops, _ := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
		result := p6PlaybackResult(terminal.Outcome.Winner)
		// This writes only a terminal already independently certified by P6. There
		// is no Authoritative=true assignment and no audit-result promotion here.
		p6 := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=? AND input_hash=? AND p13_match_id=? AND source=?", current.P6RecordID, constUtil.P6BattleCaptured, current.InputHash, current.MatchID, constUtil.P13UFGuardBattleSource).Updates(map[string]any{"status": constUtil.P6BattleComputed, "authoritative": terminal.Outcome.Authoritative, "outcome": outcome, "outcome_hash": p6Digest(outcome), "operates": ops, "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), ops...), byte(result))), "finished_at": current.CreatedAt})
		if p6.Error != nil {
			return p6.Error
		}
		if p6.RowsAffected != 1 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		current.Terminal = encoded
		current.TerminalHash = p6Digest(encoded)
		current.CalculationStage = constUtil.P13UFGuardComputed
		current.NextCalculationAt = 0
		current.SourceHash = p13UFGuardBattleHash(&current)
		update := tx.Model(&db.P13UnionFightGuardBattleAttempt{}).Where("id=? AND source_hash=? AND calculation_stage=?", current.ID, row.SourceHash, constUtil.P13UFGuardCaptured).Updates(map[string]any{"terminal": current.Terminal, "terminal_hash": current.TerminalHash, "calculation_stage": current.CalculationStage, "next_calculation_at": 0, "source_hash": current.SourceHash})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		return nil
	})
}
func (s *GameService) p13UFGuardRetry(ctx context.Context, row *db.P13UnionFightGuardBattleAttempt, now time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stop, gateErr := p13UFCombatCommitGate(tx, row.MatchID, p13UFNow(ctx))
		if gateErr != nil || stop {
			return gateErr
		}
		var current db.P13UnionFightGuardBattleAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", row.ID).Error; err != nil {
			return err
		}
		if current.SourceHash != row.SourceHash || current.CalculationStage != constUtil.P13UFGuardCaptured || current.Quarantined {
			return nil
		}
		if _, _, _, err := s.p13UFGuardBattleVerify(tx, &current); err != nil {
			return err
		}
		delay := min(int64(1)<<min(current.CalculationFailures, int64(5)), int64(constUtil.P13UFGuardRetryMaxSeconds)) * 1000
		if now.UnixMilli() <= 0 || now.UnixMilli() > math.MaxInt64-delay {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		current.NextCalculationAt = now.UnixMilli() + delay
		if current.CalculationFailures < math.MaxInt64 {
			current.CalculationFailures++
		}
		current.SourceHash = p13UFGuardBattleHash(&current)
		update := tx.Model(&db.P13UnionFightGuardBattleAttempt{}).Where("id=? AND source_hash=? AND calculation_stage=? AND quarantined=?", current.ID, row.SourceHash, constUtil.P13UFGuardCaptured, false).Updates(map[string]any{"next_calculation_at": current.NextCalculationAt, "calculation_failures": current.CalculationFailures, "source_hash": current.SourceHash})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		return nil
	})
}

func p13UFGuardPlayerFrameMatches(input *model.FrozenBattleInput, before *pb.InstUFPlayer, rules *p13UFGuardBattleRules) bool {
	if input == nil || input.Attacker == nil || input.Attacker.Inst == nil || before == nil || before.FightInfo == nil || rules == nil || before.GetGuid() != input.Attacker.GetPlayerId() {
		return false
	}
	count := len(input.Attacker.Formation)
	if count < 1 || count > 4 || rules.LocalRule == constUtil.P13UFGuardLocalRule && count != 1 || len(before.FightInfo.Index) != count || len(before.FightInfo.HpPos) != count || len(before.FightInfo.MaxHpPos) != count || before.FightInfo.GetAnger() != input.Attacker.Inst.GetAnger() {
		return false
	}
	slots := map[int32]int{}
	for i, pose := range before.FightInfo.Index {
		if _, ok := slots[pose]; ok {
			return false
		}
		slots[pose] = i
	}
	seen := map[string]bool{}
	poses := map[int32]bool{}
	for _, hero := range input.Attacker.Formation {
		if hero == nil || hero.Prop == nil || hero.Inst == nil || hero.GetGuid() == "" || seen[hero.GetGuid()] || poses[hero.GetPose()] {
			return false
		}
		index, ok := slots[hero.GetPose()]
		if !ok || before.FightInfo.HpPos[index] != hero.Inst.GetCurHp() || before.FightInfo.MaxHpPos[index] != hero.Prop.GetHp() {
			return false
		}
		seen[hero.GetGuid()], poses[hero.GetPose()] = true, true
	}
	return true
}

// Mapping is pure and can inspect diagnostic terminals. Production application
// must first call validateP6UFGuardTerminal; this helper cannot certify a result.
func p13UFGuardBattleMap(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, before *pb.InstUFPlayer, guardBefore *pb.InstUFGuard, rules *p13UFGuardBattleRules, due, now int64) (*pb.InstUFPlayer, *pb.InstUFGuard, bool, bool, int64, error) {
	reject := func() (*pb.InstUFPlayer, *pb.InstUFGuard, bool, bool, int64, error) {
		return nil, nil, false, false, 0, errors.New(constUtil.P13UFGuardConflict)
	}
	if input == nil || out == nil || out.Outcome == nil || before == nil || guardBefore == nil || !p13UFGuardRulesValid(rules) || now < due || input.Attacker == nil || input.Monster == nil || len(input.Monster.Formation) != 1 || !p13UFGuardPlayerFrameMatches(input, before, rules) || guardBefore.FightInfo == nil || len(guardBefore.FightInfo.Index) != 1 || guardBefore.FightInfo.Index[0] != 0 || len(guardBefore.FightInfo.HpPos) != 1 || len(guardBefore.FightInfo.MaxHpPos) != 1 || guardBefore.GetGuid() != input.Monster.GetMonsterId() || out.Outcome.Attacker == nil || out.Outcome.Monster == nil || out.Outcome.Attacker.Inst == nil || out.Outcome.Monster.Inst == nil || out.Outcome.Attacker.GetPlayerId() != before.GetGuid() || out.Outcome.Monster.GetMonsterId() != guardBefore.GetGuid() || len(out.FinalActors) != len(input.Attacker.Formation)+1 {
		return reject()
	}
	player := proto.Clone(before).(*pb.InstUFPlayer)
	guard := proto.Clone(guardBefore).(*pb.InstUFGuard)
	initial := map[string]*pb.BattleHero{}
	slots := map[string]int{}
	for _, hero := range input.Attacker.Formation {
		key := "0/" + hero.GetGuid()
		initial[key] = hero
		for index, pose := range before.FightInfo.Index {
			if pose == hero.GetPose() {
				slots[key] = index
			}
		}
	}
	monster := input.Monster.Formation[0]
	if monster == nil || monster.Prop == nil || monster.Inst == nil {
		return reject()
	}
	initial["1/"+monster.GetGuid()] = monster
	slots["1/"+monster.GetGuid()] = 0
	seen := map[string]bool{}
	for _, actor := range out.FinalActors {
		if actor.Camp < 0 || actor.Camp > 1 {
			return reject()
		}
		key := strconv.FormatInt(int64(actor.Camp), 10) + "/" + actor.GUID
		origin, ok := initial[key]
		if !ok || seen[key] {
			return reject()
		}
		fight := player.FightInfo
		if actor.Camp == 1 {
			fight = guard.FightInfo
		}
		index := slots[key]
		raw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(origin)
		if actor.HeroID != origin.GetId() || actor.Pose != origin.GetPose() || actor.BloodNumber != 1 || actor.InitialActorSHA256 != p6Digest(raw) || !combatFinite(actor.CurrentHP) || !combatFinite(actor.MaxHP) || actor.CurrentHP < 0 || actor.MaxHP <= 0 || actor.CurrentHP > actor.MaxHP || fight.HpPos[index] != origin.Inst.GetCurHp() || fight.MaxHpPos[index] != origin.Prop.GetHp() {
			return reject()
		}
		fight.HpPos[index], fight.MaxHpPos[index] = actor.CurrentHP, actor.MaxHP
		seen[key] = true
	}
	if len(seen) != len(initial) {
		return reject()
	}

	pAP, gAP := out.Outcome.Attacker.GetInst().GetAnger(), out.Outcome.Monster.GetInst().GetAnger()
	if !combatFinite(pAP) || !combatFinite(gAP) || pAP < 0 || gAP < 0 || pAP > 300 || gAP > 300 {
		return reject()
	}
	player.FightInfo.Anger = proto.Float64(pAP)
	guard.FightInfo.Anger = proto.Float64(gAP)
	dead, defeated := true, guard.FightInfo.HpPos[0] == 0
	for _, hp := range player.FightInfo.HpPos {
		dead = dead && hp == 0
	}
	if dead && defeated || dead && out.Outcome.Winner != 1 || defeated && out.Outcome.Winner != 0 || out.Outcome.Winner < 0 || out.Outcome.Winner > 1 {
		return reject()
	}
	player.State = proto.Int32(0)
	player.LeftTime = proto.Float32(0)
	player.TotalTime = proto.Float32(0)
	guard.State = proto.Int32(0)
	revive := int64(0)
	if dead {
		if player.GetDeadNum() < 0 || player.GetDeadNum() == math.MaxInt32 {
			return reject()
		}
		player.DeadNum = proto.Int32(player.GetDeadNum() + 1)
		revive = rules.ReviveSeconds[min(int(player.GetDeadNum())-1, len(rules.ReviveSeconds)-1)]
		player.State = proto.Int32(3)
		player.TotalTime = proto.Float32(float32(revive))
		player.LeftTime = proto.Float32(float32(max(int64(0), due+revive*1000-now)) / 1000)
	}
	if defeated {
		guard.State = proto.Int32(2)
	} else if rules.LocalRule == constUtil.P13UFGuardMirrorRule {
		// Business reset only. The original P6 Terminal and final actors retain
		// their exact HP/AP; an uncompleted NPC challenge keeps no wound.
		guard.FightInfo = proto.Clone(guardBefore.FightInfo).(*pb.InstFightable)
		for i, maximum := range guard.FightInfo.MaxHpPos {
			guard.FightInfo.HpPos[i] = maximum
		}
		guard.FightInfo.Anger = proto.Float64(0)
	}
	// Guard defeat grants zero score/token and never increments player kills.
	return player, guard, dead, defeated, revive, nil
}

// Advance applies only already certified terminals. The independent worker
// owns calculation; no computation or five-second wait occurs in field Tick.
func (s *GameService) AdvanceP13UnionFightGuardBattles(ctx context.Context, now time.Time) error {
	var ids []string
	if err := p13UFCombatOpenCandidates(db.GetIns().WithContext(ctx), "p13_union_fight_guard_battle_attempts", now).Model(&db.P13UnionFightGuardBattleAttempt{}).Where("status=? AND calculation_stage=? AND due_at<=?", constUtil.P13UFGuardPending, constUtil.P13UFGuardComputed, now.UnixMilli()).Order("due_at,id").Limit(constUtil.P13UFGuardWorkerBatch).Pluck("id", &ids).Error; err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		var completed *db.P13UnionFightGuardBattleAttempt
		var buildingFrames []proto.Message
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := ServerGameClockGuardTransaction(tx); e != nil {
				return e
			}
			var policy db.P13GuildPolicy
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; e != nil {
				return e
			}
			var row db.P13UnionFightGuardBattleAttempt
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; e != nil {
				return e
			}
			if row.Status != constUtil.P13UFGuardPending || row.DueAt > now.UnixMilli() {
				return nil
			}
			stop, gateErr := p13UFCombatCommitGate(tx, row.MatchID, p13UFNow(ctx))
			if gateErr != nil || stop {
				return gateErr
			}
			input, out, rules, e := s.p13UFGuardBattleVerify(tx, &row)
			if e != nil {
				return e
			}
			if e = validateP6UFGuardTerminalForRule(input, out, rules); e != nil {
				return e
			}
			var state db.P13UnionFightGuardState
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "match_id=? AND guard_guid=?", row.MatchID, row.GuardGUID).Error; e != nil {
				return e
			}
			currentGuard, e := p13UFGuardStateFrame(&state)
			if e != nil {
				return e
			}
			if state.PendingAttemptID != row.ID || state.Generation != row.GuardGeneration || state.SourceHash != row.ReservationHash || currentGuard.GetState() != 1 {
				return errors.New(constUtil.P13UFGuardConflict)
			}
			var actor db.P13UnionFightFieldPlayer
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, "match_id=? AND player_id=?", row.MatchID, row.PlayerID).Error; e != nil {
				return e
			}
			current, e := p13UFFieldReadActor(&actor)
			if e != nil {
				return e
			}
			var before pb.InstUFPlayer
			var guardBefore pb.InstUFGuard
			if proto.Unmarshal(row.PlayerBefore, &before) != nil || proto.Unmarshal(row.GuardBefore, &guardBefore) != nil || !proto.Equal(current.FightInfo, before.FightInfo) || current.GetDeadNum() != before.GetDeadNum() || current.GetKillNum() != before.GetKillNum() || current.GetOwnScore() != before.GetOwnScore() || current.GetOwnToken() != before.GetOwnToken() || (!actor.Hooked && actor.LeftAt == 0 && current.GetState() != 1) {
				return errors.New(constUtil.P13UFGuardConflict)
			}
			after, guardAfter, dead, defeated, revive, e := p13UFGuardBattleMap(input, out, &before, &guardBefore, rules, row.DueAt, now.UnixMilli())
			if e != nil {
				return e
			}
			// Preserve inventory, effects and other fields owned by unrelated operations.
			current.FightInfo = after.FightInfo
			current.DeadNum = after.DeadNum
			current.State = after.State
			current.LeftTime = after.LeftTime
			current.TotalTime = after.TotalTime

			state.Frame, _ = proto.MarshalOptions{Deterministic: true}.Marshal(guardAfter)
			state.FrameHash = p6Digest(state.Frame)
			state.PendingAttemptID = ""
			if state.Generation == math.MaxInt64 {
				return errors.New(constUtil.P13UFGuardConflict)
			}
			state.Generation++
			if defeated {
				state.DefeatedAt = row.DueAt
				state.BuildingEvent = state.BuildingKind + "-defeated"
				if rules.RecoverySeconds > 0 {
					state.RecoverAt = row.DueAt + rules.RecoverySeconds*1000
				}
			}
			state.SourceHash = p13UFGuardStateHash(&state)
			if e = tx.Save(&state).Error; e != nil {
				return e
			}
			row.PlayerAfter, _ = proto.MarshalOptions{Deterministic: true}.Marshal(current)
			row.GuardAfter = state.Frame
			row.PlayerDied = dead
			row.GuardDefeated = defeated
			row.ReviveSeconds = revive
			row.BuildingEvent = state.BuildingEvent
			row.Status = constUtil.P13UFGuardSettled
			row.SettledAt = now.UnixMilli()
			row.SourceHash = p13UFGuardBattleHash(&row)
			if row.GuardDefeated {
				buildingFrames, e = s.p13UFGuardBuildingApply(tx, &row, &state, now)
				if e != nil {
					return e
				}
				row.SourceHash = p13UFGuardBattleHash(&row)
			}
			if e = tx.Save(&row).Error; e != nil {
				return e
			}
			scene, drainFrames, drainErr := p13UFTowerDrain(tx, &actor, current, "guard", row.ID, row.InputHash, row.DueAt, now)
			if drainErr != nil {
				return drainErr
			}
			if e = p13UFFieldSaveActor(tx, &actor, scene); e != nil {
				return e
			}
			buildingFrames = append(buildingFrames, drainFrames...)
			completed = &row
			return nil
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("guard apply %s: %w", id, err))
			continue
		}
		if completed != nil {
			publishErr := p13UFPublishBeforeAbort(ctx, completed.MatchID, func() {
				s.p13UFGuardBattlePublish(ctx, completed)
				for _, frame := range buildingFrames {
					if _, private := frame.(*pb.SCUFPlayerDead); private {
						s.publishP10Private(ctx, completed.PlayerID, frame)
					} else {
						s.p13UFFieldBroadcast(ctx, completed.MatchID, 0, frame)
					}
				}
			})
			if publishErr != nil {
				failures = append(failures, publishErr)
			}
		}
	}
	return errors.Join(failures...)
}
func (s *GameService) p13UFGuardBattlePublish(ctx context.Context, row *db.P13UnionFightGuardBattleAttempt) {
	var player pb.InstUFPlayer
	var guard pb.InstUFGuard
	if proto.Unmarshal(row.PlayerAfter, &player) != nil || proto.Unmarshal(row.GuardAfter, &guard) != nil {
		return
	}
	for _, message := range []proto.Message{&pb.SCUFPlayerFightEnd{Guid: proto.Int64(row.PlayerID)}, &pb.SCUFGuardFightEnd{Guid: proto.Int64(row.GuardGUID), FightSide: guard.FightInfo}} {
		s.p13UFFieldBroadcast(ctx, row.MatchID, 0, message)
	}
	s.publishP10Private(ctx, row.PlayerID, &pb.SCUFPlayerFightEndDetail{Player: &player})
	if row.GuardDefeated {
		s.p13UFFieldBroadcast(ctx, row.MatchID, 0, &pb.SCUFGuardDead{Guid: proto.Int64(row.GuardGUID)})
	}
	if row.PlayerDied {
		s.publishP10Private(ctx, row.PlayerID, &pb.SCUFPlayerDead{Guid: []int64{row.PlayerID}, ReviveTime: []float64{float64(max(int64(0), row.DueAt+row.ReviveSeconds*1000-row.SettledAt)) / 1000}, ReviveItemId: []string{p13UFGuardReviveItem(&player)}})
	}
}
func p13UFGuardReviveItem(player *pb.InstUFPlayer) string {
	for _, item := range player.Items {
		if item.GetId() == "ufitem_012" && item.GetLeftUseNum() > 0 {
			return "ufitem_012"
		}
	}
	return ""
}

// Shared action/death integration calls this in the caller's existing TX.
func p13UFGuardBattleAction(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer) error {
	if actor == nil || frame == nil {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	if frame.GetState() == 3 {
		return errors.New("死亡状态需要原复活流程，不能开始其他场景操作")
	}
	var count int64
	if err := tx.Model(&db.P13UnionFightGuardBattleAttempt{}).Where("match_id=? AND player_id=? AND status=?", actor.MatchID, actor.PlayerID, constUtil.P13UFGuardPending).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("守卫战斗5秒接纳尚未结束，不能开始其他场景操作")
	}

	return nil
}
func p13UFGuardBattleProject(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, now time.Time) error {
	if actor == nil || frame == nil {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	var row db.P13UnionFightGuardBattleAttempt
	query := tx.Where("match_id=? AND player_id=?", actor.MatchID, actor.PlayerID)
	if frame.GetState() == 1 {
		query = query.Where("status=?", constUtil.P13UFGuardPending)
	} else if frame.GetState() == 3 {
		query = query.Where("status=? AND player_died=?", constUtil.P13UFGuardSettled, true)
	} else {
		return nil
	}
	if err := query.Order("created_at DESC,id DESC").First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if row.SourceHash != p13UFGuardBattleHash(&row) {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	if frame.GetState() == 1 {
		frame.LeftTime = proto.Float32(float32(max(int64(0), row.DueAt-now.UnixMilli())) / 1000)
		frame.TotalTime = proto.Float32(5)
	} else {
		var after pb.InstUFPlayer
		if proto.Unmarshal(row.PlayerAfter, &after) != nil {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		// A newer player/guard death owns its own countdown. Do not project an
		// old guard death over a current, different terminal or revival.
		if frame.GetDeadNum() != after.GetDeadNum() || !proto.Equal(frame.FightInfo, after.FightInfo) {
			return nil
		}
		if row.ReviveSeconds <= 0 || row.SettledAt < row.DueAt {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		frame.LeftTime = proto.Float32(float32(max(int64(0), row.DueAt+row.ReviveSeconds*1000-now.UnixMilli())) / 1000)
		frame.TotalTime = proto.Float32(float32(row.ReviveSeconds))
	}
	return nil
}
