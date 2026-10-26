package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// P13UnionFightNaturalReviveMeta is the comparable, blob-free terminal proof.
type P13UnionFightNaturalReviveMeta struct {
	MatchID                                                      string
	PlayerID, DeadOrdinal                                        int64
	Status, Rule, DeathKind, DeathID, DeathSourceHash            string
	DeathAt, DueAt, AppliedAt                                    int64
	BeforeHash, AfterHash, BeforePositionHash, AfterPositionHash string
	MatchHash, FieldHash, BattleHash, SourceHash                 string
}

// P13UnionFightNaturalReviveMetadata supplies only committed scalar identities.
func P13UnionFightNaturalReviveMetadata(row *db.P13UnionFightReviveReceipt) P13UnionFightNaturalReviveMeta {
	if row == nil {
		return P13UnionFightNaturalReviveMeta{}
	}
	return P13UnionFightNaturalReviveMeta{row.MatchID, row.PlayerID, row.DeadOrdinal, row.Status, row.Rule, row.DeathKind, row.DeathID, row.DeathSourceHash, row.DeathAt, row.DueAt, row.AppliedAt, row.BeforeHash, row.AfterHash, row.BeforePositionHash, row.AfterPositionHash, row.MatchHash, row.FieldHash, row.BattleHash, row.SourceHash}
}

func p13UFNaturalReviveError() error { return errors.New(constUtil.P13UFNaturalReviveConflict) }
func p13UFNaturalReviveHash(row *db.P13UnionFightReviveReceipt) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFNaturalRevivePoint(point *pb.PointNew) bool {
	return point != nil && point.X != nil && point.Y != nil && point.Z != nil && combatFinite(float64(point.GetX())) && combatFinite(float64(point.GetY())) && combatFinite(float64(point.GetZ()))
}

// Restore only the named local lifecycle. The frozen maximum/pose mapping is
// checked when a battle is supplied; no P6 computation or reward occurs.
func p13UFNaturalReviveRestore(before *pb.InstUFPlayer, battle *pb.BattlePlayer) (*pb.InstUFPlayer, error) {
	if before == nil || before.GetGuid() <= 0 || before.GetState() != 3 || before.GetDeadNum() <= 0 || before.GetCamp() < 0 || before.GetCamp() > 1 || before.FightInfo == nil || before.FightInfo.GetGuid() != before.GetGuid() || len(before.FightInfo.Index) == 0 || len(before.FightInfo.Index) != len(before.FightInfo.HpPos) || len(before.FightInfo.Index) != len(before.FightInfo.MaxHpPos) || !combatFinite(before.FightInfo.GetAnger()) || before.FightInfo.GetAnger() < 0 || before.FightInfo.GetAnger() > 300 {
		return nil, p13UFNaturalReviveError()
	}
	slots := map[int32]int{}
	for i, pose := range before.FightInfo.Index {
		maximum := before.FightInfo.MaxHpPos[i]
		if _, ok := slots[pose]; ok || pose < 0 || !combatFinite(maximum) || maximum <= 0 || before.FightInfo.HpPos[i] != 0 {
			return nil, p13UFNaturalReviveError()
		}
		slots[pose] = i
	}
	if battle != nil {
		if battle.GetPlayerId() != before.GetGuid() || len(battle.Formation) != len(slots) {
			return nil, p13UFNaturalReviveError()
		}
		seen := map[int32]bool{}
		for _, hero := range battle.Formation {
			if hero == nil || hero.Prop == nil || seen[hero.GetPose()] {
				return nil, p13UFNaturalReviveError()
			}
			index, ok := slots[hero.GetPose()]
			if !ok || hero.Prop.GetHp() != before.FightInfo.MaxHpPos[index] {
				return nil, p13UFNaturalReviveError()
			}
			seen[hero.GetPose()] = true
		}
	}
	after := proto.Clone(before).(*pb.InstUFPlayer)
	for i, maximum := range after.FightInfo.MaxHpPos {
		after.FightInfo.HpPos[i] = maximum
	}
	after.FightInfo.Anger = proto.Float64(0)
	after.State, after.LeftTime, after.TotalTime = proto.Int32(0), proto.Float32(0), proto.Float32(0)
	return after, nil
}

// P13UnionFightNaturalReviveVerifyMetadata verifies the scalar seal without
// loading PBs. All PB fields are excluded from seal JSON and bound by hashes.
func P13UnionFightNaturalReviveVerifyMetadata(row *db.P13UnionFightReviveReceipt) error {
	if row == nil || len(row.MatchID) != 64 || row.PlayerID <= 0 || row.DeadOrdinal <= 0 || row.DeadOrdinal > math.MaxInt32 || row.Status != constUtil.P13UFNaturalReviveApplied || row.Rule != constUtil.P13UFNaturalReviveRule || row.DeathAt <= 0 || row.DueAt <= row.DeathAt || row.AppliedAt < row.DueAt || len(row.DeathSourceHash) != 64 || len(row.MatchHash) != 64 || len(row.FieldHash) != 64 || len(row.BattleHash) != 64 || len(row.BeforeHash) != 64 || len(row.AfterHash) != 64 || len(row.BeforePositionHash) != 64 || len(row.AfterPositionHash) != 64 || row.SourceHash != p13UFNaturalReviveHash(row) {
		return p13UFNaturalReviveError()
	}
	switch row.DeathKind {
	case constUtil.P13UFNaturalReviveOrdinary, constUtil.P13UFNaturalReviveGuard:
		if row.DueAt-row.DeathAt > 86400000 || (row.DueAt-row.DeathAt)%1000 != 0 {
			return p13UFNaturalReviveError()
		}
		if len(row.DeathID) != 64 {
			return p13UFNaturalReviveError()
		}
	case constUtil.P13UFNaturalReviveTower:
		parts := strings.Split(row.DeathID, "/")
		if len(parts) != 2 {
			return p13UFNaturalReviveError()
		}
		for _, part := range parts {
			value, e := strconv.ParseInt(part, 10, 64)
			if e != nil || value <= 0 || strconv.FormatInt(value, 10) != part {
				return p13UFNaturalReviveError()
			}
		}
	default:
		return p13UFNaturalReviveError()
	}
	return nil
}

// P13UnionFightNaturalReviveVerifyLoaded authenticates small loaded PBs only.
// The caller separately binds death metadata, frozen battle and map snapshots.
func P13UnionFightNaturalReviveVerifyLoaded(row *db.P13UnionFightReviveReceipt) error {
	if err := P13UnionFightNaturalReviveVerifyMetadata(row); err != nil {
		return err
	}
	if row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || row.BeforePositionHash != p6Digest(row.BeforePosition) || row.AfterPositionHash != p6Digest(row.AfterPosition) {
		return p13UFNaturalReviveError()
	}

	before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
	from, to := &pb.PointNew{}, &pb.PointNew{}
	if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || proto.Unmarshal(row.BeforePosition, from) != nil || proto.Unmarshal(row.AfterPosition, to) != nil || !p13UFNaturalRevivePoint(from) || !p13UFNaturalRevivePoint(to) || before.GetGuid() != row.PlayerID || int64(before.GetDeadNum()) != row.DeadOrdinal {
		return p13UFNaturalReviveError()
	}
	expected, err := p13UFNaturalReviveRestore(before, nil)
	if err != nil || !proto.Equal(expected, after) {
		return p13UFNaturalReviveError()
	}
	return nil
}

// P13UnionFightNaturalReviveVerifyPosition binds a loaded receipt to the exact
// frozen camp spawn, allowing mixed-ledger validation to read the map once.
func P13UnionFightNaturalReviveVerifyPosition(row *db.P13UnionFightReviveReceipt, spawn *pb.PointNew) error {
	if row == nil || !p13UFNaturalRevivePoint(spawn) {
		return p13UFNaturalReviveError()
	}
	actual := &pb.PointNew{}
	if proto.Unmarshal(row.AfterPosition, actual) != nil || !proto.Equal(actual, spawn) {
		return p13UFNaturalReviveError()
	}
	return nil
}

type p13UFNaturalReviveDeath struct {
	kind, id, hash     string
	at, due, settledAt int64
	bound              *pb.InstUFPlayer
}

func p13UFNaturalReviveDeathValid(value *p13UFNaturalReviveDeath, id int64) error {
	if value == nil || value.bound == nil || value.bound.GetGuid() != id || value.bound.GetState() != 3 || value.bound.GetDeadNum() <= 0 || value.at <= 0 || value.settledAt < value.at || value.due <= value.at || len(value.hash) != 64 {
		return p13UFNaturalReviveError()
	}
	if _, err := p13UFNaturalReviveRestore(value.bound, nil); err != nil {
		return err
	}
	return nil
}

// Readiness binds the current corpse to the latest persisted death. Item012
// and any later nondeath state are ignored rather than healed a second time.
func p13UFNaturalReviveReady(frame *pb.InstUFPlayer, death *p13UFNaturalReviveDeath, now time.Time) (bool, error) {
	if frame == nil {
		return false, p13UFNaturalReviveError()
	}
	if frame.GetState() != 3 {
		return false, nil
	}
	if err := p13UFNaturalReviveDeathValid(death, frame.GetGuid()); err != nil {
		return false, err
	}
	if death.bound.GetDeadNum() != frame.GetDeadNum() || death.bound.GetCamp() != frame.GetCamp() || !proto.Equal(death.bound.FightInfo, frame.FightInfo) {
		return false, p13UFNaturalReviveError()
	}
	return now.UnixMilli() >= death.due && now.UnixMilli() >= death.settledAt, nil
}

func p13UFNaturalReviveChoose(values []*p13UFNaturalReviveDeath, id int64) (*p13UFNaturalReviveDeath, error) {
	var chosen *p13UFNaturalReviveDeath
	for _, value := range values {
		if err := p13UFNaturalReviveDeathValid(value, id); err != nil {
			return nil, err
		}
		if chosen == nil || value.at > chosen.at {
			chosen = value
		} else if value.at == chosen.at && (value.kind != chosen.kind || value.id != chosen.id) {
			return nil, p13UFNaturalReviveError()
		}
	}
	if chosen == nil {
		return nil, p13UFNaturalReviveError()
	}
	return chosen, nil
}
func p13UFNaturalReviveOrdinary(tx *gorm.DB, row *db.P13UnionFightPlayerBattleAttempt, id int64) (*p13UFNaturalReviveDeath, error) {
	if row.SourceHash != p13UFPlayerBattleHash(row) || row.Status != constUtil.P13UFPlayerBattleSettled || row.DeadPlayerID != id || row.SettledAt < row.DueAt || row.ReviveSeconds <= 0 || row.ReviveSeconds > 86400 || row.DueAt > math.MaxInt64-row.ReviveSeconds*1000 {
		return nil, p13UFNaturalReviveError()
	}
	var offer db.P13UnionFightPlayerBattleOffer
	if err := tx.First(&offer, "match_id=?", row.MatchID).Error; err != nil {
		return nil, err
	}
	rules, err := p13UFPlayerBattleRulesFromOffer(&offer, row)
	if err != nil {
		return nil, err
	}
	var frames []*pb.InstUFPlayer
	if json.Unmarshal(row.After, &frames) != nil || len(frames) != 2 {
		return nil, p13UFNaturalReviveError()
	}
	var dead *pb.InstUFPlayer
	for _, frame := range frames {
		if frame == nil {
			return nil, p13UFNaturalReviveError()
		}
		if frame.GetGuid() == id {
			if dead != nil {
				return nil, p13UFNaturalReviveError()
			}
			dead = frame
		}
	}
	if dead == nil || dead.GetDeadNum() <= 0 || row.ReviveSeconds != rules.ReviveSeconds[min(int(dead.GetDeadNum())-1, len(rules.ReviveSeconds)-1)] {
		return nil, p13UFNaturalReviveError()
	}
	return &p13UFNaturalReviveDeath{kind: constUtil.P13UFNaturalReviveOrdinary, id: row.ID, hash: row.SourceHash, at: row.DueAt, due: row.DueAt + row.ReviveSeconds*1000, settledAt: row.SettledAt, bound: dead}, nil
}
func p13UFNaturalReviveGuard(row *db.P13UnionFightGuardBattleAttempt, id int64) (*p13UFNaturalReviveDeath, error) {
	if err := p13UFCombatGuardClockValid(row, true); err != nil {
		return nil, err
	}
	var rules p13UFGuardBattleRules
	frame := &pb.InstUFPlayer{}
	if row.PlayerID != id || proto.Unmarshal(row.PlayerAfter, frame) != nil || json.Unmarshal(row.Rules, &rules) != nil || frame.GetDeadNum() <= 0 || row.ReviveSeconds != rules.ReviveSeconds[min(int(frame.GetDeadNum())-1, len(rules.ReviveSeconds)-1)] {
		return nil, p13UFNaturalReviveError()
	}
	return &p13UFNaturalReviveDeath{kind: constUtil.P13UFNaturalReviveGuard, id: row.ID, hash: row.SourceHash, at: row.DueAt, due: row.DueAt + row.ReviveSeconds*1000, settledAt: row.SettledAt, bound: frame}, nil
}
func p13UFNaturalReviveTower(tx *gorm.DB, row *db.P13UnionFightTowerHit, id int64) (*p13UFNaturalReviveDeath, error) {
	var tower db.P13UnionFightTowerState
	if err := tx.First(&tower, "match_id=? AND tower_guid=?", row.MatchID, row.TowerGUID).Error; err != nil {
		return nil, err
	}
	if err := p13UFTowerHitVerify(row, &tower); err != nil {
		return nil, err
	}
	frame := &pb.InstUFPlayer{}
	if row.TargetKind != "player" || row.DeadPlayerID != id || row.TargetGUID != id || row.ReviveAt <= row.DueAt || proto.Unmarshal(row.After, frame) != nil {
		return nil, p13UFNaturalReviveError()
	}
	return &p13UFNaturalReviveDeath{kind: constUtil.P13UFNaturalReviveTower, id: fmt.Sprintf("%d/%d", row.TowerGUID, row.Sequence), hash: row.HitHash, at: row.DueAt, due: row.ReviveAt, settledAt: row.AppliedAt, bound: frame}, nil
}

// Latest settled death from each indexed family is checked before choosing;
// an invalid newest row never causes fallback to an older valid death.
func p13UFNaturalReviveLatest(tx *gorm.DB, scope string, id int64) (*p13UFNaturalReviveDeath, error) {
	values := []*p13UFNaturalReviveDeath{}
	var ordinary db.P13UnionFightPlayerBattleAttempt
	err := tx.Where("match_id=? AND dead_player_id=? AND status=?", scope, id, constUtil.P13UFPlayerBattleSettled).Order("due_at DESC,id DESC").First(&ordinary).Error
	if err == nil {
		value, e := p13UFNaturalReviveOrdinary(tx, &ordinary, id)
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var guard db.P13UnionFightGuardBattleAttempt
	err = tx.Where("match_id=? AND player_id=? AND status=? AND player_died=?", scope, id, constUtil.P13UFGuardSettled, true).Order("created_at DESC,id DESC").First(&guard).Error
	if err == nil {
		value, e := p13UFNaturalReviveGuard(&guard, id)
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var tower db.P13UnionFightTowerHit
	err = tx.Where("match_id=? AND dead_player_id=?", scope, id).Order("due_at DESC,tower_guid DESC,sequence DESC").First(&tower).Error
	if err == nil {
		value, e := p13UFNaturalReviveTower(tx, &tower, id)
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return p13UFNaturalReviveChoose(values, id)
}

// P13UnionFightNaturalReviveReceiptVerify is the complete database-backed
// first certification. Mixed terminal proofs may batch the same identities
// and use VerifyLoaded/VerifyPosition without per-receipt reads.
func P13UnionFightNaturalReviveReceiptVerify(tx *gorm.DB, row *db.P13UnionFightReviveReceipt) error {
	if tx == nil {
		return p13UFNaturalReviveError()
	}
	if err := P13UnionFightNaturalReviveVerifyLoaded(row); err != nil {
		return err
	}
	var death *p13UFNaturalReviveDeath
	var err error
	switch row.DeathKind {
	case constUtil.P13UFNaturalReviveOrdinary:
		var source db.P13UnionFightPlayerBattleAttempt
		if err = tx.First(&source, "id=? AND match_id=?", row.DeathID, row.MatchID).Error; err == nil {
			death, err = p13UFNaturalReviveOrdinary(tx, &source, row.PlayerID)
		}
	case constUtil.P13UFNaturalReviveGuard:
		var source db.P13UnionFightGuardBattleAttempt
		if err = tx.First(&source, "id=? AND match_id=?", row.DeathID, row.MatchID).Error; err == nil {
			death, err = p13UFNaturalReviveGuard(&source, row.PlayerID)
		}
	case constUtil.P13UFNaturalReviveTower:
		parts := strings.Split(row.DeathID, "/")
		tower, _ := strconv.ParseInt(parts[0], 10, 64)
		sequence, _ := strconv.ParseInt(parts[1], 10, 64)
		var source db.P13UnionFightTowerHit
		if err = tx.First(&source, "match_id=? AND tower_guid=? AND sequence=?", row.MatchID, tower, sequence).Error; err == nil {
			death, err = p13UFNaturalReviveTower(tx, &source, row.PlayerID)
		}
	}
	if err != nil {
		return err
	}
	before := &pb.InstUFPlayer{}
	if death == nil || p13UFNaturalReviveDeathValid(death, row.PlayerID) != nil || proto.Unmarshal(row.Before, before) != nil || death.kind != row.DeathKind || death.id != row.DeathID || death.hash != row.DeathSourceHash || death.at != row.DeathAt || death.due != row.DueAt || row.AppliedAt < death.settledAt || int64(death.bound.GetDeadNum()) != row.DeadOrdinal || death.bound.GetCamp() != before.GetCamp() || !proto.Equal(death.bound.FightInfo, before.FightInfo) {
		return p13UFNaturalReviveError()
	}
	var match db.P13UnionFightMatch
	if err = tx.First(&match, "id=?", row.MatchID).Error; err != nil {
		return err
	}
	if match.SnapshotHash != row.MatchHash || p6Digest(match.Roster) != row.MatchHash || row.DeathAt < match.StartAt || row.DueAt >= match.EndAt || row.AppliedAt >= match.EndAt {
		return p13UFNaturalReviveError()
	}
	var field db.P13UnionFightField
	if err = tx.First(&field, "match_id=?", row.MatchID).Error; err != nil {
		return err
	}
	var snapshot p13UFFieldSnapshot
	if field.SnapshotHash != row.FieldHash || p6Digest(field.Snapshot) != row.FieldHash || json.Unmarshal(field.Snapshot, &snapshot) != nil || p13UFFieldProjection(&snapshot) != nil {
		return p13UFNaturalReviveError()
	}
	var actor db.P13UnionFightFieldPlayer
	if err = tx.Select("match_id,player_id,guild_id,camp,battle,battle_hash").First(&actor, "match_id=? AND player_id=?", row.MatchID, row.PlayerID).Error; err != nil {
		return err
	}
	fighter, camp, err := p13UFFieldFighter(tx, &match, row.PlayerID, actor.GuildID)
	if err != nil {
		return err
	}
	if camp != int64(before.GetCamp()) || camp != actor.Camp || fighter.SnapshotHash != row.BattleHash {
		return p13UFNaturalReviveError()
	}
	battle := &pb.BattlePlayer{}
	if actor.BattleHash != row.BattleHash || p6Digest(actor.Battle) != row.BattleHash || proto.Unmarshal(actor.Battle, battle) != nil {
		return p13UFNaturalReviveError()
	}
	if _, err = p13UFNaturalReviveRestore(before, battle); err != nil {
		return err
	}
	spawn := snapshot.LeftSpawn
	if before.GetCamp() == 1 {
		spawn = snapshot.RightSpawn
	}
	return P13UnionFightNaturalReviveVerifyPosition(row, spawn)
}

type p13UFNaturalReviveCursor struct {
	match     string
	player    int64
	exhausted bool
}

var p13UFNaturalReviveCursors = struct {
	sync.Mutex
	scan, backfill p13UFNaturalReviveCursor
}{}

// The cursor is scheduler progress only, never business state. Keyset wrap
// prevents a future-due/corrupt prefix from starving another bounded chunk.
func p13UFNaturalReviveCandidates(ctx context.Context, now time.Time, backfill bool) ([]db.P13UnionFightFieldPlayer, error) {
	p13UFNaturalReviveCursors.Lock()
	defer p13UFNaturalReviveCursors.Unlock()
	cursor := &p13UFNaturalReviveCursors.scan
	limit, state := constUtil.P13UFNaturalReviveBatch, int64(3)
	columns := "fp.match_id,fp.player_id"
	if backfill {
		cursor = &p13UFNaturalReviveCursors.backfill
		limit, state = constUtil.P13UFNaturalReviveBackfillBatch, 0
		columns = "fp.match_id,fp.player_id,fp.frame,fp.frame_hash,fp.frame_state"
	}
	if backfill && cursor.exhausted {
		return nil, nil
	}
	query := func(after bool) ([]db.P13UnionFightFieldPlayer, error) {
		rows := []db.P13UnionFightFieldPlayer{}
		q := db.GetIns().WithContext(ctx).Table("p13_union_fight_field_players AS fp").Select(columns).Joins("JOIN p13_union_fight_matches AS m ON m.id=fp.match_id").Where("fp.frame_state=? AND fp.left_at=0 AND m.end_at>?", state, now.UnixMilli())
		if !backfill {
			q = q.Where("m.start_at<=?", now.UnixMilli())
		}
		if after && cursor.match != "" {
			q = q.Where("(fp.match_id>? OR (fp.match_id=? AND fp.player_id>?))", cursor.match, cursor.match, cursor.player)
		}
		err := q.Order("fp.match_id,fp.player_id").Limit(limit).Scan(&rows).Error
		return rows, err
	}
	rows, err := query(true)
	if err == nil && !backfill && len(rows) == 0 && cursor.match != "" {
		*cursor = p13UFNaturalReviveCursor{}
		rows, err = query(false)
	}
	if err == nil && len(rows) > 0 {
		last := rows[len(rows)-1]
		*cursor = p13UFNaturalReviveCursor{match: last.MatchID, player: last.PlayerID}
	}
	if err == nil && backfill && len(rows) < limit {
		cursor.exhausted = true
	}
	return rows, err
}
func p13UFNaturalReviveBackfill(ctx context.Context, now time.Time) error {
	rows, err := p13UFNaturalReviveCandidates(ctx, now, true)
	if err != nil {
		return err
	}
	scopes := make([]string, 0, len(rows))
	for _, row := range rows {
		scopes = append(scopes, row.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	var failures []error
	for _, row := range rows {
		if clocks.paused(row.MatchID) {
			continue
		}
		frame := &pb.InstUFPlayer{}
		if row.FrameHash != p6Digest(row.Frame) || proto.Unmarshal(row.Frame, frame) != nil || frame.GetGuid() != row.PlayerID || frame.GetState() < 0 {
			failures = append(failures, p13UFNaturalReviveError())
			continue
		}
		if frame.GetState() == 0 {
			continue
		}
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, row.MatchID); err != nil {
				return err
			} else if closed {
				return nil
			}
			return tx.Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND player_id=? AND left_at=0 AND frame_hash=? AND frame_state=?", row.MatchID, row.PlayerID, row.FrameHash, row.FrameState).Update("frame_state", int64(frame.GetState())).Error
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		// Failed/cancelled index writes must remain eligible after repair.
		p13UFNaturalReviveCursors.Lock()
		p13UFNaturalReviveCursors.backfill = p13UFNaturalReviveCursor{}
		p13UFNaturalReviveCursors.Unlock()
	}
	return errors.Join(failures...)
}

// InitializeP13UnionFightNaturalRevive creates immutable receipts and performs
// one bounded legacy frame-index backfill; Tick continues keyset progress.
func (s *GameService) InitializeP13UnionFightNaturalRevive(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightReviveReceipt{}, &db.P13UnionFightFieldPlayer{}); err != nil {
		return err
	}
	p13UFNaturalReviveCursors.Lock()
	p13UFNaturalReviveCursors.backfill = p13UFNaturalReviveCursor{}
	p13UFNaturalReviveCursors.Unlock()
	return p13UFNaturalReviveBackfill(ctx, p13UFNow(ctx))
}

// Update only the raw game.position.current leaves. Natural revival must not
// normalize bags, advance resource clocks or rewrite Player through save hooks.
func p13UFNaturalReviveProfilePosition(profile *db.Profile, id int64, spawn *pb.PointNew) (string, bool, error) {
	if profile == nil || profile.Id != id || profile.Game == nil || !p13UFNaturalRevivePoint(spawn) {
		return "", false, p13UFNaturalReviveError()
	}
	var game map[string]json.RawMessage
	if json.Unmarshal([]byte(*profile.Game), &game) != nil || game == nil {
		return "", false, p13UFNaturalReviveError()
	}
	var loading bool
	if raw, ok := game["position_loading"]; ok && json.Unmarshal(raw, &loading) != nil {
		return "", false, p13UFNaturalReviveError()
	}
	position := &pb.Position{}
	if json.Unmarshal(game["position"], position) != nil {
		return "", false, p13UFNaturalReviveError()
	}
	if loading || position.GetPlayerId() != id || position.GetScene() != constUtil.P13UnionFightFieldTarget {
		return "", false, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(game["position"], &object) != nil || object == nil {
		return "", false, p13UFNaturalReviveError()
	}
	current, err := json.Marshal(spawn)
	if err != nil {
		return "", false, err
	}
	object["current"] = current
	game["position"], err = json.Marshal(object)
	if err != nil {
		return "", false, err
	}
	encoded, err := json.Marshal(game)
	return string(encoded), true, err
}

func (s *GameService) p13UFNaturalReviveOne(ctx context.Context, key db.P13UnionFightFieldPlayer, now time.Time) error {
	var revived *pb.SCUFPlayerRevive
	var full *pb.SCUFPlayer
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		// All item/combat lifecycle writers take the same guild gate first.
		if _, err := s.p13UFFieldNeedPolicy(tx); err != nil {
			return err
		}
		if closed, err := p13UFWorldSealed(tx, key.MatchID); err != nil {
			return err
		} else if closed {
			return nil
		}
		var actor db.P13UnionFightFieldPlayer
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, "match_id=? AND player_id=?", key.MatchID, key.PlayerID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return err
		}
		if actor.FrameState != int64(frame.GetState()) || !combatFinite(actor.MovementDebt) || actor.MovementDebt < 0 {
			return p13UFNaturalReviveError()
		}
		if actor.LeftAt != 0 || frame.GetState() != 3 {
			return nil
		}
		var match db.P13UnionFightMatch
		if err = tx.First(&match, "id=?", actor.MatchID).Error; err != nil {
			return err
		}
		if now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return nil
		}
		var completed db.P13UnionFightSettlementResult
		err = tx.Select("match_id").First(&completed, "match_id=?", match.ID).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// A persisted base death closes gameplay before the mixed result worker.
		var base db.P13UnionFightGuardBattleAttempt
		err = tx.Select("id").Where("match_id=? AND status=? AND guard_defeated=? AND building_event=?", match.ID, constUtil.P13UFGuardSettled, true, "base-defeated").First(&base).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var member db.P13GuildMember
		err = tx.Select("player_id,guild_id").First(&member, "player_id=?", actor.PlayerID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if member.GuildID != actor.GuildID {
			return nil
		}
		var guild db.P13Guild
		err = tx.Select("id").First(&guild, "id=?", actor.GuildID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		fighter, camp, err := p13UFFieldFighter(tx, &match, actor.PlayerID, actor.GuildID)
		if err != nil || camp != actor.Camp || fighter.SnapshotHash != actor.BattleHash {
			return p13UFNaturalReviveError()
		}
		var field db.P13UnionFightField
		if err = tx.First(&field, "match_id=?", match.ID).Error; err != nil {
			return err
		}
		var snapshot p13UFFieldSnapshot
		if field.SnapshotHash != p6Digest(field.Snapshot) || json.Unmarshal(field.Snapshot, &snapshot) != nil || p13UFFieldProjection(&snapshot) != nil || p13UFFieldActorProjection(&actor, &snapshot) != nil {
			return p13UFNaturalReviveError()
		}
		death, err := p13UFNaturalReviveLatest(tx, match.ID, actor.PlayerID)
		if err != nil {
			return err
		}
		if death.at < match.StartAt || death.at >= match.EndAt {
			return p13UFNaturalReviveError()
		}
		ready, err := p13UFNaturalReviveReady(frame, death, now)
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}
		var existing db.P13UnionFightReviveReceipt
		err = tx.First(&existing, "match_id=? AND player_id=? AND dead_ordinal=?", match.ID, actor.PlayerID, frame.GetDeadNum()).Error
		if err == nil {
			return p13UFNaturalReviveError()
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		battle := &pb.BattlePlayer{}
		if proto.Unmarshal(actor.Battle, battle) != nil {
			return p13UFNaturalReviveError()
		}
		after, err := p13UFNaturalReviveRestore(frame, battle)
		if err != nil {
			return err
		}
		spawn := snapshot.LeftSpawn
		if actor.Camp == 1 {
			spawn = snapshot.RightSpawn
		}
		if !p13UFNaturalRevivePoint(spawn) {
			return p13UFNaturalReviveError()
		}
		var profile db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, actor.PlayerID).Error; err != nil {
			return err
		}
		// Never overwrite a later login, scene transfer or in-progress loading.
		if actor.SessionHash == "" || len(profile.Session) != 16 || actor.SessionHash != p6Digest(profile.Session) {
			return nil
		}
		gameJSON, eligible, err := p13UFNaturalReviveProfilePosition(&profile, actor.PlayerID, spawn)
		if err != nil {
			return err
		}
		if !eligible {
			return nil
		}
		row := db.P13UnionFightReviveReceipt{MatchID: match.ID, PlayerID: actor.PlayerID, DeadOrdinal: int64(frame.GetDeadNum()), Status: constUtil.P13UFNaturalReviveApplied, Rule: constUtil.P13UFNaturalReviveRule, DeathKind: death.kind, DeathID: death.id, DeathSourceHash: death.hash, DeathAt: death.at, DueAt: death.due, AppliedAt: now.UnixMilli(), MatchHash: match.SnapshotHash, FieldHash: field.SnapshotHash, BattleHash: actor.BattleHash}
		beforePosition := &pb.PointNew{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)}
		if row.Before, err = (proto.MarshalOptions{Deterministic: true}).Marshal(frame); err != nil {
			return err
		}
		if row.After, err = (proto.MarshalOptions{Deterministic: true}).Marshal(after); err != nil {
			return err
		}
		if row.BeforePosition, err = (proto.MarshalOptions{Deterministic: true}).Marshal(beforePosition); err != nil {
			return err
		}
		if row.AfterPosition, err = (proto.MarshalOptions{Deterministic: true}).Marshal(spawn); err != nil {
			return err
		}
		row.BeforeHash, row.AfterHash = p6Digest(row.Before), p6Digest(row.After)
		row.BeforePositionHash, row.AfterPositionHash = p6Digest(row.BeforePosition), p6Digest(row.AfterPosition)
		row.SourceHash = p13UFNaturalReviveHash(&row)
		if err = P13UnionFightNaturalReviveVerifyLoaded(&row); err != nil {
			return err
		}
		actor.X, actor.Y, actor.Z = spawn.GetX(), spawn.GetY(), spawn.GetZ()
		actor.MovementDebt, actor.UpdatedAt = 0, now.UnixMilli()
		if err = p13UFFieldSaveActor(tx, &actor, after); err != nil {
			return err
		}
		if err = tx.Model(&profile).Update("game", gameJSON).Error; err != nil {
			return err
		}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		revived = &pb.SCUFPlayerRevive{Guid: []int64{actor.PlayerID}, Pos: []*pb.PointNew{proto.Clone(spawn).(*pb.PointNew)}}
		full = &pb.SCUFPlayer{Player: []*pb.InstUFPlayer{after}, Local: []*pb.PointNew{proto.Clone(spawn).(*pb.PointNew)}}
		return nil
	})
	if err != nil {
		return err
	}
	if revived != nil {
		if err := p13UFPublishBeforeAbort(ctx, key.MatchID, func() {
			if revived != nil {
				s.p13UFFieldBroadcast(ctx, key.MatchID, 0, revived)
				s.p13UFFieldBroadcast(ctx, key.MatchID, 0, full)
			}
		}); err != nil {
			return err
		}
	}

	return nil
}

// TickP13UnionFightNaturalRevive resumes persisted deaths in bounded chunks.
// No clock projection, request retry or cold boot can apply one ordinal twice.
func (s *GameService) TickP13UnionFightNaturalRevive(ctx context.Context, now time.Time) error {
	if now.UnixMilli() <= 0 {
		return p13UFNaturalReviveError()
	}
	var failures []error
	if err := p13UFNaturalReviveBackfill(ctx, now); err != nil {
		failures = append(failures, err)
	}
	rows, err := p13UFNaturalReviveCandidates(ctx, now, false)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	scopes := make([]string, 0, len(rows))
	for _, key := range rows {
		scopes = append(scopes, key.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return errors.Join(append(failures, clockErr)...)
	}
	for _, key := range rows {
		if clocks.paused(key.MatchID) || clocks.terminal[key.MatchID] > 0 {
			continue
		}
		if err := s.p13UFNaturalReviveOne(ctx, key, now); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
