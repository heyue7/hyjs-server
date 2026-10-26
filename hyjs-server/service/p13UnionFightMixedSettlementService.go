package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Keep v1 record bytes unchanged; v2 additionally seals scalar playback results.
type p13UFMixedRecordMeta struct {
	p13UFCombatRecordMeta
	ClientResult, ServerResult int32
	CreatedAt, FinishedAt      int64
}

func p13UFMixedRecordMetadata(row *db.P6BattleRecord) p13UFMixedRecordMeta {
	return p13UFMixedRecordMeta{p13UFCombatRecordMetadata(row), row.ClientResult, row.ServerResult, row.CreatedAt, row.FinishedAt}
}

type p13UFMixedGuardMeta struct {
	ID, MatchID, P6RecordID, FightID, Status, CalculationStage, SourceHash, InputHash, TerminalHash, OfferHash, BuildingEvent, BuildingKind string
	PlayerID, GuardGUID, GuardGeneration, ClientStamp, CreatedAt, DueAt, SettledAt, ReviveSeconds, BuildingAppliedAt                        int64
	PlayerDied, GuardDefeated                                                                                                               bool
}
type p13UFMixedTowerMeta = P13UnionFightTowerHitMeta

type p13UFMixedMeta struct {
	Kind, Key                      string
	CreatedAt, EffectAt, AppliedAt int64
	Included                       bool
	Data                           json.RawMessage
	Record                         *p13UFMixedRecordMeta `json:",omitempty"`
	NoEffect                       bool                  `json:",omitempty"`
}
type p13UFMixedFrozenMeta struct{ Kind, Key, SourceHash string }
type p13UFMixedProof struct {
	Schema, MatchSHA, LeftPreviewSHA, RightPreviewSHA string
	TerminalRule, ResultKind                          string
	Players                                           []int64
	TerminalAt                                        int64
	Winner                                            int
	Reason, LedgerHash                                string
	Kills                                             [2]int32
	Reached                                           [2]int64
	Sources                                           []p13UFMixedMeta
	AuditRefusals                                     []p13UFAuditRefusalMeta `json:",omitempty"`
	Frozen                                            []p13UFMixedFrozenMeta
}
type p13UFMixedEvent struct {
	Kind, Key                                                                         string
	CreatedAt, DueAt, AppliedAt                                                       int64
	Before, After                                                                     []*pb.InstUFPlayer
	GuardBefore, GuardAfter                                                           *pb.InstUFGuard
	CarBefore, CarAfter                                                               *pb.InstUFCariot
	PlayerID, GuardGUID, GuardGeneration, DeadPlayerID, RewardPlayerID, ReviveSeconds int64
	Building                                                                          *p13UFGuardBuildingProof
	Meta                                                                              p13UFMixedMeta
	NoEffect                                                                          bool
	TowerActor                                                                        *pb.BattleHero
	TowerCapture                                                                      *pb.InstUFPlayer
	TowerRelease                                                                      *pb.InstUFPlayer
	GuardRule                                                                         string
	CombatApplied                                                                     map[string][]string
	CombatOwners                                                                      map[string]int64
}

func p13UFMixedError() error { return errors.New(constUtil.P13UFMixedConflict) }
func p13UFMixedGuardMetadata(row *db.P13UnionFightGuardBattleAttempt) p13UFMixedGuardMeta {
	return p13UFMixedGuardMeta{row.ID, row.MatchID, row.P6RecordID, row.FightID, row.Status, row.CalculationStage, row.SourceHash, row.InputHash, row.TerminalHash, row.OfferHash, row.BuildingEvent, row.BuildingKind, row.PlayerID, row.GuardGUID, row.GuardGeneration, row.ClientStamp, row.CreatedAt, row.DueAt, row.SettledAt, row.ReviveSeconds, row.BuildingAppliedAt, row.PlayerDied, row.GuardDefeated}
}
func p13UFMixedTowerMetadata(row *db.P13UnionFightTowerHit) p13UFMixedTowerMeta {
	return P13UnionFightTowerHitMetadata(row)
}

func p13UFMixedJSON(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }
func p13UFMixedSchema(raw []byte) bool {
	var header struct{ Schema string }
	return json.Unmarshal(raw, &header) == nil && header.Schema == constUtil.P13UFMixedProofSchema
}

// p13UFMixedNeeded also protects the peaceful branch from environmental-only
// car damage. Merely freezing disabled/idle sources does not change v1 routing.
func p13UFMixedNeeded(tx *gorm.DB, match *db.P13UnionFightMatch) (bool, error) {
	for _, table := range []any{&db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightGuardAuditAttempt{}, &db.P13UnionFightGuardBattleAttempt{}, &db.P13UnionFightTowerHit{}, &db.P13UnionFightReviveReceipt{}, &db.P13UnionFightShopPurchase{}, &db.P13UnionFightGateImpact{}, &db.P13UnionFightPlayerRecovery{}, &db.P13UnionFightCarImpact{}, &db.P13UnionFightSpeedItemEffect{}, &db.P13UnionFightHealAuraEffect{}, &db.P13UnionFightHealAuraPending{}, &db.P13UnionFightWarDrumEffect{}} {
		var count int64
		if err := tx.Model(table).Where("match_id=?", match.ID).Limit(1).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

// p13UFMixedUnsupported refuses state changes for which current producers have
// no complete effect journal. Receipts prove consumption, not target before HP.
func p13UFMixedUnsupported(tx *gorm.DB, match *db.P13UnionFightMatch, ids []int64, audit []p13UFAuditRefusalMeta) (bool, error) {
	counts := map[string]int64{}
	for _, a := range audit {
		if !a.NoEffect {
			return true, p13UFMixedError()
		}
		counts[a.Kind]++
	}
	for _, table := range []any{&db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightGuardAuditAttempt{}} {
		var count int64
		if err := tx.Model(table).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
			return true, err
		}
		kind := "battle-refusal"
		if _, ok := table.(*db.P13UnionFightGuardAuditAttempt); ok {
			kind = "guard-refusal"
		}
		if count != counts[kind] {
			return true, nil
		}
	}
	for start := 0; start < len(ids); start += 128 {
		var count int64
		if err := tx.Model(&db.P6BattleRecord{}).Where("player_id IN ? AND source IN ? AND p13_match_id=''", ids[start:min(start+128, len(ids))], p13P6MatchSources()).Count(&count).Error; err != nil {
			return true, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (s *GameService) p13UFMixedOrdinary(tx *gorm.DB, match *db.P13UnionFightMatch) ([]p13UFMixedEvent, []p13UFMixedFrozenMeta, error) {
	var directory []db.P13UnionFightPlayerBattleAttempt
	if err := tx.Select("id,p6_record_id").Where("match_id=?", match.ID).Order("due_at,id").Find(&directory).Error; err != nil {
		return nil, nil, err
	}
	if len(directory) == 0 {
		return nil, nil, nil
	}
	var offer db.P13UnionFightPlayerBattleOffer
	if err := tx.First(&offer, "match_id=?", match.ID).Error; err != nil {
		return nil, nil, err
	}
	frozen := []p13UFMixedFrozenMeta{{"ordinary-offer", match.ID, offer.SourceHash}}
	events := make([]p13UFMixedEvent, 0, len(directory))
	for start := 0; start < len(directory); start += 4 {
		keys, records := []string{}, []string{}
		for _, row := range directory[start:min(start+4, len(directory))] {
			keys = append(keys, row.ID)
			records = append(records, row.P6RecordID)
		}
		var rows []db.P13UnionFightPlayerBattleAttempt
		if err := tx.Where("id IN ?", keys).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		var blobs []db.P6BattleRecord
		if err := tx.Where("id IN ?", records).Find(&blobs).Error; err != nil {
			return nil, nil, err
		}
		if len(rows) != len(keys) || len(blobs) != len(records) {
			return nil, nil, p13UFMixedError()
		}
		recordMap := map[string]*db.P6BattleRecord{}
		for i := range blobs {
			recordMap[blobs[i].ID] = &blobs[i]
		}
		for i := range rows {
			row := &rows[i]
			if row.Status != constUtil.P13UFPlayerBattleSettled {
				return nil, nil, nil
			}
			record := recordMap[row.P6RecordID]
			if record == nil || record.P13MatchID != match.ID {
				return nil, nil, p13UFMixedError()
			}
			input, out, before, rules, err := s.p13UFPlayerBattleVerifyLoaded(tx.Statement.Context, row, record, &offer)
			if err != nil {
				return nil, nil, err
			}
			frames, err := p13UFPlayerBattleFrames(before)
			if err != nil {
				return nil, nil, err
			}
			expected, dead, reward, revive, err := p13UFPlayerBattleApply(frames, input, out, rules)
			if err != nil {
				return nil, nil, err
			}
			var actual []*pb.InstUFPlayer
			if json.Unmarshal(row.After, &actual) != nil || len(actual) != 2 || dead != row.DeadPlayerID || reward != row.RewardPlayerID || revive != row.ReviveSeconds {
				return nil, nil, p13UFMixedError()
			}
			for j := range actual {
				if actual[j].GetGuid() == dead {
					expected[j].LeftTime = proto.Float32(float32(max(int64(0), row.DueAt+revive*1000-row.SettledAt)) / 1000)
				}
				if !p13UFMixedPlayerEqual(expected[j], actual[j], true) {
					return nil, nil, p13UFMixedError()
				}
			}
			meta := p13UFMixedMeta{Kind: "ordinary", Key: row.ID, CreatedAt: row.CreatedAt, EffectAt: row.DueAt, AppliedAt: row.SettledAt, Data: p13UFMixedJSON(p13UFCombatAttemptMetadata(row))}
			rec := p13UFMixedRecordMetadata(record)
			meta.Record = &rec
			events = append(events, p13UFMixedEvent{Kind: "ordinary", Key: row.ID, CombatApplied: input.AppliedProperties, CombatOwners: p13UFMixedCombatOwners(input.Attacker, input.Defender), CreatedAt: row.CreatedAt, DueAt: row.DueAt, AppliedAt: row.SettledAt, Before: frames, After: actual, DeadPlayerID: dead, RewardPlayerID: reward, ReviveSeconds: revive, Meta: meta})
		}
	}
	return events, frozen, nil
}

func (s *GameService) p13UFMixedGuards(tx *gorm.DB, match *db.P13UnionFightMatch) ([]p13UFMixedEvent, []p13UFMixedFrozenMeta, error) {
	var directory []db.P13UnionFightGuardBattleAttempt
	if err := tx.Select("id,p6_record_id").Where("match_id=?", match.ID).Order("due_at,id").Find(&directory).Error; err != nil {
		return nil, nil, err
	}
	if len(directory) == 0 {
		return nil, nil, nil
	}
	var offers []db.P13UnionFightGuardAuditOffer
	if err := tx.Where("match_id=?", match.ID).Limit(11).Find(&offers).Error; err != nil {
		return nil, nil, err
	}
	if len(offers) > 10 {
		return nil, nil, p13UFMixedError()
	}
	offerMap := map[int64]*db.P13UnionFightGuardAuditOffer{}
	frozen := []p13UFMixedFrozenMeta{}
	for i := range offers {
		row := &offers[i]
		if row.SourceHash != p13UFGuardOfferHash(row) || row.MonsterHash != p6Digest(row.Monster) || offerMap[row.GuardGUID] != nil {
			return nil, nil, p13UFMixedError()
		}
		offerMap[row.GuardGUID] = row
		frozen = append(frozen, p13UFMixedFrozenMeta{"guard-offer", strconv.FormatInt(row.GuardGUID, 10), row.SourceHash})
	}
	events := make([]p13UFMixedEvent, 0, len(directory))
	for start := 0; start < len(directory); start += 4 {
		keys, records := []string{}, []string{}
		for _, row := range directory[start:min(start+4, len(directory))] {
			keys = append(keys, row.ID)
			records = append(records, row.P6RecordID)
		}
		var rows []db.P13UnionFightGuardBattleAttempt
		if err := tx.Where("id IN ?", keys).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		var blobs []db.P6BattleRecord
		if err := tx.Where("id IN ?", records).Find(&blobs).Error; err != nil {
			return nil, nil, err
		}
		if len(rows) != len(keys) || len(blobs) != len(records) {
			return nil, nil, p13UFMixedError()
		}
		recordMap := map[string]*db.P6BattleRecord{}
		for i := range blobs {
			recordMap[blobs[i].ID] = &blobs[i]
		}
		for i := range rows {
			row := &rows[i]
			if row.Status != constUtil.P13UFGuardSettled {
				return nil, nil, nil
			}
			record := recordMap[row.P6RecordID]
			if record == nil || record.P13MatchID != match.ID {
				return nil, nil, p13UFMixedError()
			}
			input, _, rules, err := s.p13UFGuardBattleVerifyLoaded(tx.Statement.Context, row, record, offerMap[row.GuardGUID])
			if err != nil {
				return nil, nil, err
			}
			if rules.LocalRule == constUtil.P13UFGuardMirrorRule && (rules.MatchStartAt != match.StartAt || rules.MatchEndAt != match.EndAt) {
				return nil, nil, p13UFMixedError()
			}
			before, after, guardBefore, guardAfter := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}, &pb.InstUFGuard{}, &pb.InstUFGuard{}
			if proto.Unmarshal(row.PlayerBefore, before) != nil || proto.Unmarshal(row.PlayerAfter, after) != nil || proto.Unmarshal(row.GuardBefore, guardBefore) != nil || proto.Unmarshal(row.GuardAfter, guardAfter) != nil {
				return nil, nil, p13UFMixedError()
			}
			var building *p13UFGuardBuildingProof
			if row.GuardDefeated {
				building, err = p13UFGuardBuildingProofRead(row)
				if err != nil {
					return nil, nil, err
				}
			} else if row.BuildingAppliedAt != 0 || len(row.BuildingProof) > 0 || row.BuildingEvent != "" {
				return nil, nil, p13UFMixedError()
			}
			dead := int64(0)
			if row.PlayerDied {
				dead = row.PlayerID
			}
			meta := p13UFMixedMeta{Kind: "guard", Key: row.ID, CreatedAt: row.CreatedAt, EffectAt: row.DueAt, AppliedAt: row.SettledAt, Data: p13UFMixedJSON(p13UFMixedGuardMetadata(row))}
			rec := p13UFMixedRecordMetadata(record)
			meta.Record = &rec
			if rules.BuildingKind == "base" && row.GuardDefeated && (building == nil || building.Event != "base-defeated") {
				return nil, nil, p13UFMixedError()
			}
			events = append(events, p13UFMixedEvent{Kind: "guard", Key: row.ID, GuardRule: rules.LocalRule, CombatApplied: input.AppliedProperties, CombatOwners: p13UFMixedCombatOwners(input.Attacker), CreatedAt: row.CreatedAt, DueAt: row.DueAt, AppliedAt: row.SettledAt, Before: []*pb.InstUFPlayer{before}, After: []*pb.InstUFPlayer{after}, GuardBefore: guardBefore, GuardAfter: guardAfter, PlayerID: row.PlayerID, GuardGUID: row.GuardGUID, GuardGeneration: row.GuardGeneration, DeadPlayerID: dead, ReviveSeconds: row.ReviveSeconds, Building: building, Meta: meta})
		}
	}
	return events, frozen, nil
}

func (s *GameService) p13UFMixedTowers(tx *gorm.DB, match *db.P13UnionFightMatch) ([]p13UFMixedEvent, []p13UFMixedFrozenMeta, error) {
	var towers []db.P13UnionFightTowerState
	if err := tx.Where("match_id=?", match.ID).Limit(7).Find(&towers).Error; err != nil {
		return nil, nil, err
	}
	if len(towers) > 6 {
		return nil, nil, p13UFMixedError()
	}
	towerFrames := map[int64]*pb.InstUFTower{}
	if len(towers) > 0 {
		snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
		if err != nil {
			return nil, nil, err
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			return nil, nil, p13UFMixedError()
		}
		for _, tower := range buildings.TowerInfos {
			towerFrames[tower.GetGuid()] = tower
		}
	}
	towerMap := map[int64]*db.P13UnionFightTowerState{}
	frozen := []p13UFMixedFrozenMeta{}
	for i := range towers {
		row := &towers[i]
		frame := towerFrames[row.TowerGUID]
		if !p13UFTowerValid(row) || row.MatchHash != match.SnapshotHash || frame == nil || frame.GetGuard().GetGuid() != row.GuardGUID || int64(frame.GetGuard().GetCamp()) != row.Camp || int64(frame.GetId()) != row.TowerID {
			return nil, nil, p13UFMixedError()
		}
		towerMap[row.TowerGUID] = row
		frozen = append(frozen, p13UFMixedFrozenMeta{"tower", strconv.FormatInt(row.TowerGUID, 10), row.SourceHash})
	}
	var directory []db.P13UnionFightTowerHit
	if err := tx.Select("match_id,tower_guid,sequence").Where("match_id=?", match.ID).Order("tower_guid,sequence").Find(&directory).Error; err != nil {
		return nil, nil, err
	}
	events := make([]p13UFMixedEvent, 0, len(directory))
	for start := 0; start < len(directory); start += 128 {
		pairs := [][]any{}
		for _, row := range directory[start:min(start+128, len(directory))] {
			pairs = append(pairs, []any{row.TowerGUID, row.Sequence})
		}
		var rows []db.P13UnionFightTowerHit
		if err := tx.Where("match_id=? AND (tower_guid,sequence) IN ?", match.ID, pairs).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		if len(rows) != len(pairs) {
			return nil, nil, p13UFMixedError()
		}
		for i := range rows {
			row := &rows[i]
			if err := p13UFTowerHitVerify(row, towerMap[row.TowerGUID]); err != nil {
				return nil, nil, err
			}
			meta := P13UnionFightTowerHitMetadata(row)
			created := meta.HitAt - 600
			if row.Status == "" {
				created = row.DueAt - 600
			}
			event := p13UFMixedEvent{Kind: "tower", Key: fmt.Sprintf("%d/%d", row.TowerGUID, row.Sequence), CreatedAt: created, DueAt: row.DueAt, AppliedAt: row.AppliedAt, DeadPlayerID: row.DeadPlayerID, ReviveSeconds: row.ReviveSeconds}
			event.Meta = p13UFMixedMeta{Kind: event.Kind, Key: event.Key, CreatedAt: event.CreatedAt, EffectAt: event.DueAt, AppliedAt: event.AppliedAt, Data: p13UFMixedJSON(meta)}
			event.Meta.NoEffect = row.Status == constUtil.P13UnionFightTowerHitCanceled

			if row.Status != "" && row.TargetKind == "player" && !p13UFMixedWorldCanceled(event.Meta) {
				event.TowerActor = &pb.BattleHero{}
				event.TowerCapture = &pb.InstUFPlayer{}
				if proto.Unmarshal(row.FrozenActor, event.TowerActor) != nil || proto.Unmarshal(row.CaptureFrame, event.TowerCapture) != nil {
					return nil, nil, p13UFMixedError()
				}
			}
			if row.Status != "" && row.TargetKind == "player" && len(row.ReleaseFrame) > 0 {
				event.TowerRelease = &pb.InstUFPlayer{}
				if proto.Unmarshal(row.ReleaseFrame, event.TowerRelease) != nil {
					return nil, nil, p13UFMixedError()
				}
			}
			if row.Status == constUtil.P13UnionFightTowerHitPending {
				return nil, nil, nil
			}
			if row.Status == constUtil.P13UnionFightTowerHitCanceled {
				event.NoEffect = true
				event.Meta.NoEffect = true
			} else if row.TargetKind == "player" {
				before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
				if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil {
					return nil, nil, p13UFMixedError()
				}
				event.Before = []*pb.InstUFPlayer{before}
				event.After = []*pb.InstUFPlayer{after}
			} else {
				event.CarBefore, event.CarAfter = &pb.InstUFCariot{}, &pb.InstUFCariot{}
				if proto.Unmarshal(row.Before, event.CarBefore) != nil || proto.Unmarshal(row.After, event.CarAfter) != nil {
					return nil, nil, p13UFMixedError()
				}
			}

			events = append(events, event)
		}
	}
	return events, frozen, nil
}

func p13UFMixedPlayerEqual(a, b *pb.InstUFPlayer, clock bool) bool {
	if a == nil || b == nil || a.GetGuid() != b.GetGuid() || a.GetCamp() != b.GetCamp() || !proto.Equal(a.GetFightInfo(), b.GetFightInfo()) || a.GetKillNum() != b.GetKillNum() || a.GetDeadNum() != b.GetDeadNum() || a.GetOwnScore() != b.GetOwnScore() || a.GetOwnToken() != b.GetOwnToken() || a.GetState() != b.GetState() {
		return false
	}
	return !clock || a.GetLeftTime() == b.GetLeftTime() && a.GetTotalTime() == b.GetTotalTime()
}
func p13UFMixedMetaHash(event p13UFMixedEvent) string {
	switch event.Kind {
	case "ordinary":
		var meta p13UFCombatAttemptMeta
		if json.Unmarshal(event.Meta.Data, &meta) == nil {
			return meta.SourceHash
		}
	case "guard":
		var meta p13UFMixedGuardMeta
		if json.Unmarshal(event.Meta.Data, &meta) == nil {
			return meta.SourceHash
		}
	case "tower":
		var meta p13UFMixedTowerMeta
		if json.Unmarshal(event.Meta.Data, &meta) == nil {
			return meta.HitHash
		}
	}
	return ""
}
func p13UFMixedRevives(tx *gorm.DB, match *db.P13UnionFightMatch, events []p13UFMixedEvent, actors []db.P13UnionFightFieldPlayer) ([]p13UFMixedEvent, error) {
	var directory []db.P13UnionFightReviveReceipt
	if err := tx.Select("match_id,player_id,dead_ordinal").Where("match_id=?", match.ID).Order("player_id,dead_ordinal").Find(&directory).Error; err != nil {
		return nil, err
	}
	if len(directory) == 0 {
		return nil, nil
	}
	snapshot, err := p13UFFieldStoredProjection(tx, match.ID)
	if err != nil {
		return nil, err
	}
	var field db.P13UnionFightField
	if err = tx.Select("match_id,snapshot_hash").First(&field, "match_id=?", match.ID).Error; err != nil {
		return nil, err
	}
	actorMap := map[int64]*db.P13UnionFightFieldPlayer{}
	for i := range actors {
		actorMap[actors[i].PlayerID] = &actors[i]
	}
	deaths := map[string]p13UFMixedEvent{}
	for _, event := range events {
		if event.DeadPlayerID > 0 {
			deaths[event.Kind+":"+event.Key] = event
		}
	}
	result := make([]p13UFMixedEvent, 0, len(directory))
	for start := 0; start < len(directory); start += 128 {
		pairs := [][]any{}
		for _, row := range directory[start:min(start+128, len(directory))] {
			pairs = append(pairs, []any{row.PlayerID, row.DeadOrdinal})
		}
		var rows []db.P13UnionFightReviveReceipt
		if err = tx.Where("match_id=? AND (player_id,dead_ordinal) IN ?", match.ID, pairs).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) != len(pairs) {
			return nil, p13UFMixedError()
		}
		for i := range rows {
			row := &rows[i]
			actor := actorMap[row.PlayerID]
			death, ok := deaths[row.DeathKind+":"+row.DeathID]
			if actor == nil || !ok || row.MatchHash != match.SnapshotHash || row.FieldHash != field.SnapshotHash || row.BattleHash != actor.BattleHash || row.DeathSourceHash != p13UFMixedMetaHash(death) || row.AppliedAt < death.AppliedAt || row.DeathAt != death.DueAt || death.DeadPlayerID != row.PlayerID || row.DueAt != p13UFMixedReviveAt(death) {
				return nil, p13UFMixedError()
			}
			if err = P13UnionFightNaturalReviveVerifyLoaded(row); err != nil {
				return nil, err
			}
			spawn := snapshot.LeftSpawn
			if actor.Camp == 1 {
				spawn = snapshot.RightSpawn
			}
			if err = P13UnionFightNaturalReviveVerifyPosition(row, spawn); err != nil {
				return nil, err
			}
			before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
			if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || int64(before.GetDeadNum()) != row.DeadOrdinal {
				return nil, p13UFMixedError()
			}
			var bound *pb.InstUFPlayer
			for _, frame := range death.After {
				if frame.GetGuid() == row.PlayerID {
					bound = frame
				}
			}
			if bound == nil || bound.GetCamp() != before.GetCamp() || bound.GetDeadNum() != before.GetDeadNum() || !proto.Equal(bound.GetFightInfo(), before.GetFightInfo()) {
				return nil, p13UFMixedError()
			}
			key := fmt.Sprintf("%d/%d", row.PlayerID, row.DeadOrdinal)
			meta := p13UFMixedMeta{Kind: "revive", Key: key, CreatedAt: row.DeathAt, EffectAt: row.DueAt, AppliedAt: row.AppliedAt, Data: p13UFMixedJSON(P13UnionFightNaturalReviveMetadata(row))}
			result = append(result, p13UFMixedEvent{Kind: "revive", Key: key, CreatedAt: row.DeathAt, DueAt: row.DueAt, AppliedAt: row.AppliedAt, Before: []*pb.InstUFPlayer{before}, After: []*pb.InstUFPlayer{after}, PlayerID: row.PlayerID, Meta: meta})
		}
	}
	return result, nil
}

// Fold verifies actual state evolution. DueAt controls original kill ordering;
// actual apply time controls before/after chaining. Within an equal apply tick,
// only an event matching the already authenticated prefix can advance.
func p13UFMixedFold(match *db.P13UnionFightMatch, guilds map[int64]string, events []p13UFMixedEvent, cutoff int64) (*p13UFCombatLedger, error) {
	ledger := &p13UFCombatLedger{ActualKills: map[int64]int32{}, ActualDeaths: map[int64]int32{}, ActualScores: map[int64]float64{}, LastCars: map[int64]*pb.InstUFCariot{}, Reached: [2]int64{match.StartAt, match.StartAt}, PlayerKills: map[int64]int32{}, PlayerDeaths: map[int64]int32{}, PlayerScores: map[int64]float64{}, Last: map[int64]*pb.InstFightable{}}
	sort.Slice(events, func(i, j int) bool {
		if events[i].AppliedAt != events[j].AppliedAt {
			return events[i].AppliedAt < events[j].AppliedAt
		}
		if events[i].DueAt != events[j].DueAt {
			return events[i].DueAt < events[j].DueAt
		}
		return events[i].Kind+":"+events[i].Key < events[j].Kind+":"+events[j].Key
	})
	countsKill, countsDeath := map[int64]int32{}, map[int64]int32{}
	ready := func(event p13UFMixedEvent) bool {
		if event.CarBefore != nil {
			previous := ledger.LastCars[event.CarBefore.GetGuid()]
			if previous != nil {
				if p13UFCarHPIdentity(previous) != p13UFCarHPIdentity(event.CarBefore) || previous.GetHp() != event.CarBefore.GetHp() {
					return false
				}
			} else if event.CarBefore.GetHp() != event.CarBefore.GetMaxHp() {
				return false
			}
		}
		for _, frame := range event.Before {
			if frame == nil || frame.GetKillNum() != countsKill[frame.GetGuid()] || frame.GetDeadNum() != countsDeath[frame.GetGuid()] {
				return false
			}
			if previous := ledger.Last[frame.GetGuid()]; previous != nil {
				if !proto.Equal(previous, frame.FightInfo) {
					return false
				}
			} else {
				hp := frame.GetFightInfo()
				if hp == nil || len(hp.HpPos) < 1 || hp.GetAnger() != 0 || len(hp.HpPos) != len(hp.MaxHpPos) {
					return false
				}
				for i, value := range hp.HpPos {
					if value <= 0 || value != hp.MaxHpPos[i] || !combatFinite(value) {
						return false
					}
				}
			}
		}
		return true
	}
	for begin := 0; begin < len(events); {
		end := begin + 1
		for end < len(events) && events[end].AppliedAt == events[begin].AppliedAt {
			end++
		}
		pending := append([]p13UFMixedEvent(nil), events[begin:end]...)
		for len(pending) > 0 {
			chosen := -1
			for i, event := range pending {
				if ready(event) {
					chosen = i
					break
				}
			}
			if chosen < 0 {
				return nil, p13UFMixedError()
			}
			event := pending[chosen]
			pending = append(pending[:chosen], pending[chosen+1:]...)
			if event.CreatedAt < match.StartAt || event.DueAt < event.CreatedAt && event.Kind != "tower" || event.DueAt > match.EndAt && !p13UFMixedWorldCanceled(event.Meta) || event.AppliedAt < event.DueAt || event.CreatedAt > cutoff && event.Kind != "revive" {
				return nil, p13UFMixedError()
			}
			if event.NoEffect {
				continue
			}
			if event.CarAfter != nil {
				ledger.LastCars[event.CarAfter.GetGuid()] = proto.Clone(event.CarAfter).(*pb.InstUFCariot)
			}
			if len(event.Before) != len(event.After) {
				return nil, p13UFMixedError()
			}
			for i, before := range event.Before {
				after := event.After[i]
				id := before.GetGuid()
				expectedGuild := match.LeftGuildID
				if before.GetCamp() == 1 {
					expectedGuild = match.RightGuildID
				}
				if guilds[id] != expectedGuild || after == nil || after.GetGuid() != id || after.GetCamp() != before.GetCamp() || before.GetCamp() < 0 || before.GetCamp() > 1 {
					return nil, p13UFMixedError()
				}
				countsKill[id] = after.GetKillNum()
				countsDeath[id] = after.GetDeadNum()
				ledger.Last[id] = proto.Clone(after.GetFightInfo()).(*pb.InstFightable)
			}
			if event.DeadPlayerID > 0 && event.DueAt <= cutoff {
				ledger.PlayerDeaths[event.DeadPlayerID]++
			}
			if event.Kind == "ordinary" && event.DeadPlayerID > 0 {
				ledger.ActualScores[event.RewardPlayerID] += 5
				if event.DueAt <= cutoff {
					ledger.PlayerKills[event.RewardPlayerID]++
				}
			}
			if event.Kind != "ordinary" && event.RewardPlayerID != 0 {
				return nil, p13UFMixedError()
			}
			if event.Kind == "ordinary" && event.DeadPlayerID > 0 && event.DueAt <= cutoff {
				camp := 0
				if guilds[event.RewardPlayerID] == match.RightGuildID {
					camp = 1
				} else if guilds[event.RewardPlayerID] != match.LeftGuildID {
					return nil, p13UFMixedError()
				}
				if ledger.Kills[camp] == math.MaxInt32 {
					return nil, p13UFMixedError()
				}
				ledger.Kills[camp]++
				ledger.Reached[camp] = max(ledger.Reached[camp], event.DueAt)
				// Ordinary verification already binds the original five-point award.
				for i, before := range event.Before {
					if before.GetGuid() == event.RewardPlayerID {
						delta := event.After[i].GetOwnScore() - before.GetOwnScore()
						if delta != 5 {
							return nil, p13UFMixedError()
						}
						ledger.PlayerScores[event.RewardPlayerID] += delta
					}
				}
			}
		}
		begin = end
	}
	ledger.ActualKills = countsKill
	ledger.ActualDeaths = countsDeath
	return ledger, nil
}

func p13UFMixedCutoff(match *db.P13UnionFightMatch, events []p13UFMixedEvent) (int64, int, string, error) {
	cutoff := match.EndAt
	winner := -1
	for _, event := range events {
		if event.Building == nil || event.Building.Event != "base-defeated" {
			continue
		}
		if len(event.Before) != 1 || event.Before[0].GetCamp() == int32(event.Building.GuardCamp) || event.Before[0].GetCamp() < 0 || event.Before[0].GetCamp() > 1 {
			return 0, -1, "", p13UFMixedError()
		}
		camp := int(event.Before[0].GetCamp())
		if event.DueAt < cutoff {
			cutoff = event.DueAt
			winner = camp
		} else if event.DueAt == cutoff && winner != camp {
			winner = -2
		}
	}
	if winner == -2 {
		return cutoff, winner, "simultaneous-bases", nil
	}
	if winner >= 0 {
		return cutoff, winner, "base-defeated", nil
	}
	return cutoff, -1, "", nil
}

func p13UFMixedPurchases(tx *gorm.DB, match *db.P13UnionFightMatch) ([]p13UFMixedEvent, map[int64]float64, error) {
	var directory []struct{ ID string }
	if err := tx.Model(&db.P13UnionFightShopPurchase{}).Select("id").Where("match_id=?", match.ID).Order("logical_at,id").Find(&directory).Error; err != nil {
		return nil, nil, err
	}
	events := []p13UFMixedEvent{}
	spend := map[int64]float64{}
	for start := 0; start < len(directory); start += 128 {
		ids := []string{}
		for _, row := range directory[start:min(start+128, len(directory))] {
			ids = append(ids, row.ID)
		}
		var rows []db.P13UnionFightShopPurchase
		if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		if len(rows) != len(ids) {
			return nil, nil, p13UFMixedError()
		}
		for i := range rows {
			row := &rows[i]
			if row.MatchID != match.ID || row.LogicalAt < match.StartAt || row.LogicalAt >= match.EndAt {
				return nil, nil, p13UFMixedError()
			}
			if err := p13UFShopPurchaseVerifyLoaded(row); err != nil {
				return nil, nil, err
			}
			before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
			if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil {
				return nil, nil, p13UFMixedError()
			}
			copy := *row
			copy.Before = nil
			copy.After = nil
			copy.QuoteFrame = nil
			meta := p13UFMixedMeta{Kind: "shop", Key: row.ID, CreatedAt: row.LogicalAt, EffectAt: row.LogicalAt, AppliedAt: row.LogicalAt, Data: p13UFMixedJSON(copy)}
			events = append(events, p13UFMixedEvent{Kind: "shop", Key: row.ID, CreatedAt: row.LogicalAt, DueAt: row.LogicalAt, AppliedAt: row.LogicalAt, Before: []*pb.InstUFPlayer{before}, After: []*pb.InstUFPlayer{after}, PlayerID: row.PlayerID, Meta: meta})
			if row.CostCurrency == constUtil.UnionFightShopTokenResource {
				spend[row.PlayerID] += float64(row.TotalPrice)
			}
		}
	}
	return events, spend, nil
}

func p13UFMixedClosure(tx *gorm.DB, match *db.P13UnionFightMatch, ids []int64, events []p13UFMixedEvent, audit []p13UFAuditRefusalMeta) error {
	bound := map[string]bool{}
	accepted := map[string]int64{}
	for _, event := range events {
		if event.Meta.Record != nil {
			record := event.Meta.Record
			if bound[record.ID] || record.P13MatchID != match.ID || record.Status != constUtil.P6BattleComputed || !record.Authoritative {
				return p13UFMixedError()
			}
			bound[record.ID] = true
		}
		switch event.Kind {
		case "ordinary":
			var meta p13UFCombatAttemptMeta
			if json.Unmarshal(event.Meta.Data, &meta) != nil {
				return p13UFMixedError()
			}
			accepted[fmt.Sprintf("%s/%d/%d/%d", constUtil.P13UFPlayerBattleSource, meta.PlayerID, meta.TargetID, meta.ClientStamp)] = meta.CreatedAt
			accepted[fmt.Sprintf("%s/%d/%d/%d", constUtil.P13UnionFightSettlementSource, meta.PlayerID, meta.TargetID, meta.ClientStamp)] = meta.CreatedAt
		case "guard":
			var meta p13UFMixedGuardMeta
			if json.Unmarshal(event.Meta.Data, &meta) != nil {
				return p13UFMixedError()
			}
			accepted[fmt.Sprintf("%s/%d/%d/%d", constUtil.P13UFGuardBattleSource, meta.PlayerID, meta.GuardGUID, meta.ClientStamp)] = meta.CreatedAt
		}
	}
	for _, a := range audit {
		r := a.Record
		if !a.NoEffect || bound[r.ID] || r.P13MatchID != match.ID || r.Status != constUtil.P6BattleComputed || r.Authoritative {
			return p13UFMixedError()
		}
		bound[r.ID] = true
	}
	var intents []db.P13UnionFightSettlementIntent
	if err := tx.Where("match_id=?", match.ID).Find(&intents).Error; err != nil {
		return err
	}
	for _, intent := range intents {
		at, ok := accepted[fmt.Sprintf("%s/%d/%d/%d", intent.Source, intent.PlayerID, intent.TargetID, intent.Stamp)]
		if !ok || intent.ObservedAt < match.StartAt || intent.ObservedAt > at {
			return p13UFMixedError()
		}
	}
	var records []struct{ ID string }
	if err := tx.Model(&db.P6BattleRecord{}).Select("id").Where("p13_match_id=?", match.ID).Find(&records).Error; err != nil {
		return err
	}
	if len(records) != len(bound) {
		return p13UFMixedError()
	}
	for _, record := range records {
		if !bound[record.ID] {
			return p13UFMixedError()
		}
	}
	blocked, err := p13UFMixedUnsupported(tx, match, ids, audit)
	if err != nil {
		return err
	}
	if blocked {
		return p13UFMixedError()
	}
	return nil
}

func p13UFMixedLedgerHash(proof *p13UFMixedProof) string {
	copy := *proof
	copy.LedgerHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func (s *GameService) p13UFSettleMixedCombat(tx *gorm.DB, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, now time.Time) (*db.P13UnionFightSettlementResult, error) {
	var saved db.P13UnionFightSettlementResult
	if err := tx.First(&saved, "match_id=?", match.ID).Error; err == nil {
		return &saved, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	rule, err := p13UFSettlementRules(offer, match)
	if err != nil {
		return nil, err
	}
	newTerminal := rule.TerminalRule == constUtil.P13UFMixedTerminalRule
	if newTerminal && (match.EndAt <= match.StartAt || match.EndAt-match.StartAt > 1800000) {
		return nil, p13UFMixedError()
	}
	for _, table := range []any{&db.P13UnionFightPlayerBattleAttempt{}, &db.P13UnionFightGuardBattleAttempt{}} {
		var count int64
		status := constUtil.P13UFPlayerBattleSettled
		if _, ok := table.(*db.P13UnionFightGuardBattleAttempt); ok {
			status = constUtil.P13UFGuardSettled
		}
		if err := tx.Model(table).Where("match_id=? AND status<>?", match.ID, status).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, nil
		}
	}
	var auraPending int64
	if err := tx.Model(&db.P13UnionFightHealAuraPending{}).Where("match_id=? AND status=?", match.ID, constUtil.P13UFHealAuraPending).Count(&auraPending).Error; err != nil {
		return nil, err
	}
	if auraPending > 0 {
		return nil, nil
	}
	if err := tx.Table("p13_union_fight_heal_aura_effects AS e").Joins("JOIN p13_union_fight_heal_aura_offers AS o ON o.match_id=e.match_id AND o.item_id=?", "ufitem_013").Where("e.match_id=? AND e.ended_at=0 AND o.effect_rule=?", match.ID, constUtil.P13UFHealAuraPendingRule).Count(&auraPending).Error; err != nil {
		return nil, err
	}
	if auraPending > 0 {
		return nil, nil
	}
	var shotPending int64
	if err := tx.Model(&db.P13UnionFightTowerState{}).Where("match_id=? AND pending_release_at>0", match.ID).Count(&shotPending).Error; err != nil {
		return nil, err
	}
	if shotPending > 0 {
		return nil, nil
	}
	var towerPending int64
	if err := tx.Model(&db.P13UnionFightTowerHit{}).Where("match_id=? AND status=?", match.ID, constUtil.P13UnionFightTowerHitPending).Count(&towerPending).Error; err != nil {
		return nil, err
	}
	if towerPending > 0 {
		return nil, nil
	}
	guilds, ids, powers, err := p13UFCombatRoster(tx, match)
	if err != nil || guilds == nil {
		return nil, err
	}
	audit, auditErr := p13UFAuditRefusalsRead(tx, match, true)
	if auditErr != nil {
		return nil, auditErr
	}
	blocked, err := p13UFMixedUnsupported(tx, match, ids, audit)
	if err != nil || blocked {
		return nil, err
	}
	ordinary, ordinaryFrozen, err := s.p13UFMixedOrdinary(tx, match)
	if err != nil {
		return nil, err
	}
	guards, guardFrozen, err := s.p13UFMixedGuards(tx, match)
	if err != nil {
		return nil, err
	}
	towers, towerFrozen, err := s.p13UFMixedTowers(tx, match)
	if err != nil {
		return nil, err
	}
	gates, gateFrozen, err := p13UFMixedGates(tx, match, guards)
	if err != nil {
		return nil, err
	}
	events := append(append(ordinary, guards...), towers...)
	events = append(events, gates...)
	shops, spend, err := p13UFMixedPurchases(tx, match)
	if err != nil {
		return nil, err
	}
	events = append(events, shops...)
	effects, effectErr := p13UFMixedEffects(tx, match, guilds)
	if effectErr != nil {
		return nil, effectErr
	}
	events = append(events, effects.Events...)
	carEvents, carUses, carDirectory, carErr := p13UFMixedCarEvents(tx, match, guilds)
	if carErr != nil {
		return nil, carErr
	}
	events = append(events, carEvents...)
	gateUses, gateUseErr := p13UFMixedGateUses(tx, match, events, guilds)
	if gateUseErr != nil {
		return nil, gateUseErr
	}
	uses := append(append(effects.Uses, gateUses...), carUses...)

	if len(events) == 0 && !newTerminal {
		return nil, nil
	}
	cutoff, winner, reason, err := p13UFMixedCutoff(match, events)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if p13UFMixedWorldCanceled(event.Meta) {
			var meta struct{ CutoffAt, HitAt, DueAt int64 }
			if json.Unmarshal(event.Meta.Data, &meta) != nil || meta.CutoffAt != cutoff || (meta.HitAt <= cutoff && meta.DueAt <= cutoff) {
				return nil, p13UFMixedError()
			}
		}
	}
	if cutoff == 0 || now.UnixMilli() < cutoff {
		return nil, nil
	}
	var actors []db.P13UnionFightFieldPlayer
	if err = tx.Where("match_id=?", match.ID).Order("player_id").Find(&actors).Error; err != nil {
		return nil, err
	}
	if err = p13UFMixedTowerBindings(match, events, actors); err != nil {
		return nil, err
	}
	pendingEvents, pendingErr := p13UFMixedAuraPendings(tx, match, events, actors)
	if pendingErr != nil {
		return nil, pendingErr
	}
	events = append(events, pendingEvents...)
	revives, err := p13UFMixedRevives(tx, match, events, actors)
	if err != nil {
		return nil, err
	}
	events = append(events, revives...)
	if err = p13UFMixedEffectReferences(events, actors, guilds); err != nil {
		return nil, err
	}
	receipts, itemStateFrozen, itemErr := p13UFMixedItemClosure(tx, match, actors, events, uses)
	if itemErr != nil {
		return nil, itemErr
	}
	events = append(events, receipts...)

	if err = p13UFMixedClosure(tx, match, ids, events, audit); err != nil {
		return nil, err
	}
	ledger, err := p13UFMixedFold(match, guilds, events, cutoff)
	if err != nil {
		return nil, err
	}
	guardStates, err := p13UFMixedGuardChain(tx, match, events, now)
	if err != nil {
		return nil, err
	}
	carStates, err := p13UFMixedCarFinal(carDirectory, ledger)
	if err != nil {
		return nil, err
	}
	if winner == -2 && !newTerminal {
		return nil, nil
	}
	if winner == -1 {
		winner, reason = p13UFCombatWinner(ledger.Kills, ledger.Reached, powers)
		if winner < 0 {
			if !newTerminal {
				return nil, nil
			}
			winner = -2
			reason = "equal-comparison"
		}
	}
	resultKind := "win"
	if winner == -2 {
		resultKind = "draw"
	}

	joined := map[string]int{}
	seen := map[int64]bool{}
	awards := []p13UFSettlementAward{}
	winnerGuild, loserGuild := match.LeftGuildID, match.RightGuildID
	if winner == 1 {
		winnerGuild, loserGuild = loserGuild, winnerGuild
	}
	if resultKind == "draw" {
		winnerGuild, loserGuild = "", ""
	}
	var stats []db.P13UnionFightTransportStat
	if err = tx.Where("match_id=?", match.ID).Find(&stats).Error; err != nil {
		return nil, err
	}
	ore := map[int64]int64{}
	for _, stat := range stats {
		if stat.GuildID != guilds[stat.PlayerID] || stat.Ore < 0 || stat.Ore > math.MaxInt32 || stat.Deposits < 1 || stat.UpdatedAt < match.StartAt || stat.UpdatedAt > cutoff {
			return nil, p13UFMixedError()
		}
		ore[stat.PlayerID] = stat.Ore
	}
	rosterHashes, hashErr := p13UFMixedRosterBattleHashes(match)
	if hashErr != nil {
		return nil, hashErr
	}
	for _, actor := range actors {
		if rosterHashes[actor.PlayerID] != actor.BattleHash {
			return nil, p13UFMixedError()
		}
		if actor.GuildID != guilds[actor.PlayerID] || !p13UFSettlementParticipantTimeValid(&actor, cutoff) {
			return nil, nil
		}
		seen[actor.PlayerID] = true
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return nil, err
		}
		camp := int64(0)
		if actor.GuildID == match.RightGuildID {
			camp = 1
		}
		if actor.Camp != camp || frame.GetCamp() != int32(camp) {
			return nil, p13UFMixedError()
		}
		_, missionScore, complete, err := p13UFMissionState(tx, match, &actor, rule)
		if err != nil {
			return nil, err
		}
		expectedToken := ledger.ActualScores[actor.PlayerID] + float64(ore[actor.PlayerID])*3 - spend[actor.PlayerID]
		if frame.GetKillNum() != ledger.ActualKills[actor.PlayerID] || frame.GetDeadNum() != ledger.ActualDeaths[actor.PlayerID] || frame.GetOwnScore() != float64(missionScore)+ledger.ActualScores[actor.PlayerID] || frame.GetOwnToken() != expectedToken || frame.GetState() == 1 || frame.GetState() == 6 || frame.GetState() == 8 {
			return nil, p13UFMixedError()
		}
		if last := ledger.Last[actor.PlayerID]; last != nil {
			if !proto.Equal(last, frame.FightInfo) {
				return nil, p13UFMixedError()
			}
		} else {
			hp := frame.GetFightInfo()
			if hp == nil || hp.GetAnger() != 0 || len(hp.HpPos) < 1 || len(hp.HpPos) != len(hp.MaxHpPos) {
				return nil, p13UFMixedError()
			}
			for i, value := range hp.HpPos {
				if value <= 0 || value != hp.MaxHpPos[i] || !combatFinite(value) {
					return nil, p13UFMixedError()
				}
			}
		}
		if !p13UFSettlementParticipated(&actor, match.StartAt) {
			continue
		}
		joined[actor.GuildID]++
		rewards := rule.ParticipationRewards
		if actor.GuildID == winnerGuild {
			rewards += ";" + rule.VictoryRewards
		}
		tasks := rule.TaskRewards != "" && complete >= 2
		if tasks {
			rewards += ";" + rule.TaskRewards
		}
		awards = append(awards, p13UFSettlementAward{PlayerID: actor.PlayerID, GuildID: actor.GuildID, Rewards: rewards, Tasks: tasks})
	}
	for id := range ledger.Last {
		if !seen[id] {
			return nil, p13UFMixedError()
		}
	}
	if joined[match.LeftGuildID] == 0 || joined[match.RightGuildID] == 0 {
		return nil, nil
	}
	proof := p13UFMixedProof{TerminalRule: rule.TerminalRule, ResultKind: resultKind, Schema: constUtil.P13UFMixedProofSchema, MatchSHA: match.SnapshotHash, LeftPreviewSHA: p6Digest(match.LeftPreview), RightPreviewSHA: p6Digest(match.RightPreview), Players: append([]int64(nil), ids...), TerminalAt: cutoff, Winner: winner, Reason: reason, Kills: ledger.Kills, Reached: ledger.Reached, Frozen: append(append(ordinaryFrozen, guardFrozen...), towerFrozen...)}
	proof.AuditRefusals = audit
	var fieldIdentity db.P13UnionFightField
	if err = tx.Select("match_id,snapshot_hash").First(&fieldIdentity, "match_id=?", match.ID).Error; err != nil {
		return nil, err
	}
	proof.Frozen = append(proof.Frozen, p13UFMixedFrozenMeta{"field", match.ID, fieldIdentity.SnapshotHash})
	for i := range actors {
		proof.Frozen = append(proof.Frozen, p13UFMixedFrozenMeta{"actor", strconv.FormatInt(actors[i].PlayerID, 10), p13UFMixedActorSeal(&actors[i])})
	}
	proof.Frozen = append(proof.Frozen, gateFrozen...)
	carOfferFrozen, carOfferErr := p13UFMixedCarOffersFrozen(tx, match)
	if carOfferErr != nil {
		return nil, carOfferErr
	}
	proof.Frozen = append(proof.Frozen, carOfferFrozen...)
	proof.Frozen = append(proof.Frozen, effects.Frozen...)
	proof.Frozen = append(proof.Frozen, itemStateFrozen...)

	proof.Frozen = append(proof.Frozen, guardStates...)
	proof.Frozen = append(proof.Frozen, carStates...)
	for _, event := range events {
		meta := event.Meta
		meta.Included = !event.NoEffect && event.DueAt <= cutoff
		proof.Sources = append(proof.Sources, meta)
	}
	sort.Slice(proof.Sources, func(i, j int) bool {
		return proof.Sources[i].Kind+":"+proof.Sources[i].Key < proof.Sources[j].Kind+":"+proof.Sources[j].Key
	})
	sort.Slice(proof.Frozen, func(i, j int) bool {
		return proof.Frozen[i].Kind+":"+proof.Frozen[i].Key < proof.Frozen[j].Kind+":"+proof.Frozen[j].Key
	})
	proof.LedgerHash = p13UFMixedLedgerHash(&proof)
	raw, err := json.Marshal(proof)
	if err != nil {
		return nil, err
	}
	if err = p13UFMixedFrozenVerify(tx, match.ID, proof.Frozen); err != nil {
		return nil, err
	}
	result := db.P13UnionFightSettlementResult{ResultKind: resultKind, TerminalAt: cutoff, LeftKills: ledger.Kills[0], RightKills: ledger.Kills[1], LeftReachedAt: ledger.Reached[0], RightReachedAt: ledger.Reached[1], Reason: reason, LedgerHash: proof.LedgerHash, CombatProof: raw}
	return s.p13UFFinishSettlement(tx, match, offer, rule, actors, awards, winnerGuild, loserGuild, powers, now, result)
}

// Metadata verification deliberately selects no P6 input/outcome/replay payload.
func p13UFMixedReadMeta(tx *gorm.DB, matchID, kind string, sources []p13UFMixedMeta) (map[string]json.RawMessage, error) {
	result := map[string]json.RawMessage{}
	ids := []string{}
	pairs := [][]any{}
	for _, source := range sources {
		ids = append(ids, source.Key)
		if kind == "tower" || kind == "revive" || kind == "item-receipt" {
			parts := strings.Split(source.Key, "/")
			if len(parts) != 2 {
				return nil, p13UFMixedError()
			}
			a, e := strconv.ParseInt(parts[0], 10, 64)
			if e != nil {
				return nil, p13UFMixedError()
			}
			b, e := strconv.ParseInt(parts[1], 10, 64)
			if e != nil {
				return nil, p13UFMixedError()
			}
			pairs = append(pairs, []any{a, b})
		}
	}
	switch kind {
	case "aura-pending":
		var rows []db.P13UnionFightHealAuraPending
		if err := tx.Select(p13UFAuraPendingColumns()).Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			if err := p13UFAuraPendingVerifyMeta(&rows[i]); err != nil {
				return nil, err
			}
			result[rows[i].ID] = p13UFMixedJSON(p13UFAuraPendingMetadata(&rows[i]))
		}
	case "recovery":
		var rows []db.P13UnionFightPlayerRecovery
		if err := tx.Select("id,match_id,player_id,kind,item_id,stamp,nonce,caster_id,effect_guid,offer_hash,hp_percent,inventory_before,inventory_after,dead_ordinal,logical_at,applied_at,before_hash,after_hash,source_hash,pending_id").Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.SourceHash != p13UFPlayerRecoveryHash(&row) {
				return nil, p13UFMixedError()
			}
			result[row.ID] = p13UFMixedJSON(row)
		}
	case "aura":
		var rows []db.P13UnionFightHealAuraEffect
		if err := tx.Select("match_id,guid,caster_id,started_at,expires_at,next_tick_at,ended_at,offer_hash,source_hash").Where("match_id=? AND guid IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.SourceHash != p13UFHealAuraEffectHash(&row) {
				return nil, p13UFMixedError()
			}
			result[strconv.FormatInt(row.GUID, 10)] = p13UFMixedJSON(row)
		}
	case "speed":
		var rows []db.P13UnionFightSpeedItemEffect
		if err := tx.Select("match_id,guid,target_id,caster_id,started_at,expires_at,ended_at,multiplier,offer_hash,source_hash").Where("match_id=? AND guid IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.SourceHash != p13UFSpeedEffectHash(&row) {
				return nil, p13UFMixedError()
			}
			result[strconv.FormatInt(row.GUID, 10)] = p13UFMixedJSON(row)
		}
	case "drum":
		var rows []db.P13UnionFightWarDrumEffect
		if err := tx.Select("match_id,guid,caster_id,guild_id,camp,x,y,z,started_at,expires_at,ended_at,offer_hash,source_hash").Where("match_id=? AND guid IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.SourceHash != p13UFDrumEffectHash(&row) {
				return nil, p13UFMixedError()
			}
			result[strconv.FormatInt(row.GUID, 10)] = p13UFMixedJSON(row)
		}
	case "car-impact":
		var rows []db.P13UnionFightCarImpact
		if err := tx.Select("id,match_id,car_guid,ordinal,factory_guid,factory_id,generation,born_at,car_camp,factory_source_hash,generation_source_hash,identity_hash,player_id,camp,item_id,stamp,offer_hash,hp_percent,inventory_before,inventory_after,logical_at,applied_at,hp_before,hp_after,before_hash,after_hash,source_hash").Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result[row.ID] = p13UFMixedJSON(P13UnionFightCarImpactMetadata(&row))
		}
	case "item-receipt":
		var rows []db.P13GuildReceipt
		if err := tx.Select("player_id,operation,stamp,request,created_at").Where("operation=? AND (player_id,stamp) IN ?", constUtil.P13UnionFightItemUseOperation, pairs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result[fmt.Sprintf("%d/%d", row.PlayerID, row.Stamp)] = p13UFMixedJSON(p13UFMixedItemReceiptMetadata(&row))
		}
	case "ordinary":
		var rows []db.P13UnionFightPlayerBattleAttempt
		if err := tx.Select("id,match_id,p6_record_id,status,source_hash,input_hash,terminal_hash,offer_hash,player_id,target_id,client_stamp,created_at,due_at,settled_at,dead_player_id,reward_player_id").Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			result[rows[i].ID] = p13UFMixedJSON(p13UFCombatAttemptMetadata(&rows[i]))
		}
	case "guard":
		var rows []db.P13UnionFightGuardBattleAttempt
		if err := tx.Select("id,match_id,p6_record_id,fight_id,status,calculation_stage,source_hash,input_hash,terminal_hash,offer_hash,building_event,building_kind,player_id,guard_guid,guard_generation,client_stamp,created_at,due_at,settled_at,revive_seconds,building_applied_at,player_died,guard_defeated").Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			result[rows[i].ID] = p13UFMixedJSON(p13UFMixedGuardMetadata(&rows[i]))
		}
	case "tower":
		var rows []db.P13UnionFightTowerHit
		if err := tx.Select("match_id,target_kind,tower_source_hash,immutable_hash,hit_hash,tower_guid,sequence,target_guid,hit_at,due_at,applied_at,status,cancel_reason,actor_guid,frozen_actor_hash,capture_frame_hash,actor_pose,life_ordinal,damage,occupancy_kind,occupancy_id,occupancy_input_hash,occupancy_due_at,release_frame_hash,launch_occupancy_kind,launch_occupancy_id,launch_occupancy_input_hash,launch_occupancy_due_at,cutoff_at,before_hash,after_hash,cancel_frame_hash,coalesced_into,cancel_left_at,dead_player_id,revive_seconds,revive_at").Where("match_id=? AND (tower_guid,sequence) IN ?", matchID, pairs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			result[fmt.Sprintf("%d/%d", rows[i].TowerGUID, rows[i].Sequence)] = p13UFMixedJSON(p13UFMixedTowerMetadata(&rows[i]))
		}
	case "revive":
		var rows []db.P13UnionFightReviveReceipt
		if err := tx.Select("match_id,player_id,dead_ordinal,status,rule,death_kind,death_id,death_source_hash,death_at,due_at,applied_at,before_hash,after_hash,before_position_hash,after_position_hash,match_hash,field_hash,battle_hash,source_hash").Where("match_id=? AND (player_id,dead_ordinal) IN ?", matchID, pairs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			if err := P13UnionFightNaturalReviveVerifyMetadata(&rows[i]); err != nil {
				return nil, err
			}
			result[fmt.Sprintf("%d/%d", rows[i].PlayerID, rows[i].DeadOrdinal)] = p13UFMixedJSON(P13UnionFightNaturalReviveMetadata(&rows[i]))
		}
	case "gate":
		var rows []db.P13UnionFightGateImpact
		if err := tx.Select("id,match_id,gate_guid,ordinal,kind,offer_hash,player_id,stamp,item_id,inventory_before,inventory_after,car_guid,hit_ordinal,percent,logical_at,applied_at,before_hash,after_hash,source_hash").Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result[row.ID] = p13UFMixedJSON(row)
		}
	case "shop":
		var rows []db.P13UnionFightShopPurchase
		if err := tx.Select("id,match_id,player_id,stamp,shop_item_id,item_id,count,quantity,price,total_price,cost_currency,inventory_before,inventory_after,token_before,token_after,stock_before,stock_after,quote_source,quote_hash,before_hash,after_hash,logical_at,source_hash").Where("match_id=? AND id IN ?", matchID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result[row.ID] = p13UFMixedJSON(row)
		}
	default:
		return nil, p13UFMixedError()
	}
	if len(result) != len(sources) {
		return nil, p13UFMixedError()
	}
	return result, nil
}

func p13UFMixedFrozenVerify(tx *gorm.DB, matchID string, frozen []p13UFMixedFrozenMeta) error {
	groups := map[string]map[string]string{}
	for _, meta := range frozen {
		if groups[meta.Kind] == nil {
			groups[meta.Kind] = map[string]string{}
		}
		if groups[meta.Kind][meta.Key] != "" || len(meta.SourceHash) != 64 {
			return p13UFMixedError()
		}
		groups[meta.Kind][meta.Key] = meta.SourceHash
	}
	for kind, wanted := range groups {
		got := map[string]string{}
		switch kind {
		case "car-item-offer":
			var rows []db.P13UnionFightCarItemOffer
			if err := tx.Where("match_id=?", matchID).Limit(3).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if row.SourceHash != p13UFCarItemOfferHash(&row) {
					return p13UFMixedError()
				}
				got[row.ItemID] = row.SourceHash
			}
		case "car-cycle-offer":
			var rows []db.P13UnionFightFactoryCycleOffer
			if err := tx.Where("match_id=?", matchID).Limit(5).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if row.SourceHash != p13UFFactoryCycleOfferHash(&row) {
					return p13UFMixedError()
				}
				got[strconv.FormatInt(row.FactoryGUID, 10)] = row.SourceHash
			}
		case "life-offer":
			var rows []db.P13UnionFightItemOffer
			if err := tx.Select("item_id,source_hash").Where("match_id=?", matchID).Limit(4).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[row.ItemID] = row.SourceHash
			}
		case "aura-offer":
			var rows []db.P13UnionFightHealAuraOffer
			if err := tx.Where("match_id=?", matchID).Limit(2).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if !p13UFHealAuraQuoteValid(&row) {
					return p13UFMixedError()
				}
				got[row.ItemID] = row.SourceHash
			}
		case "speed-offer":
			var rows []db.P13UnionFightSpeedItemOffer
			if err := tx.Select("item_id,source_hash").Where("match_id=?", matchID).Limit(2).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[row.ItemID] = row.SourceHash
			}
		case "drum-offer":
			var rows []db.P13UnionFightWarDrumOffer
			if err := tx.Select("item_id,source_hash").Where("match_id=?", matchID).Limit(2).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[row.ItemID] = row.SourceHash
			}
		case "item-state":
			var rows []db.P13UnionFightItemState
			if err := tx.Where("match_id=?", matchID).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if row.SourceHash != p13UFItemStateHash(&row) {
					return p13UFMixedError()
				}
				got[strconv.FormatInt(row.PlayerID, 10)] = p6Digest(p13UFMixedJSON(row))
			}
		case "ordinary-offer":
			var row db.P13UnionFightPlayerBattleOffer
			if err := tx.Select("match_id,source_hash").First(&row, "match_id=?", matchID).Error; err != nil {
				return err
			}
			got[row.MatchID] = row.SourceHash
		case "guard-offer":
			var rows []db.P13UnionFightGuardAuditOffer
			if err := tx.Select("guard_guid,source_hash").Where("match_id=?", matchID).Limit(11).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[strconv.FormatInt(row.GuardGUID, 10)] = row.SourceHash
			}
		case "gate":
			var rows []db.P13UnionFightGateState
			if err := tx.Select("guid,gate_id,camp,impact_ordinal,initial_hash,frame_hash,source_hash").Where("match_id=?", matchID).Limit(9).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[strconv.FormatInt(row.GUID, 10)] = p13UFMixedGateSeal(&row)
			}
		case "car-attack-offer":
			var rows []db.P13UnionFightCarAttackOffer
			if err := tx.Select("factory_guid,source_hash").Where("match_id=?", matchID).Limit(5).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[strconv.FormatInt(row.FactoryGUID, 10)] = row.SourceHash
			}
		case "car-attack-state":
			extra := []db.P13UnionFightCarAttackState{}
			cursor := int64(0)
			for {
				var rows []db.P13UnionFightCarAttackState
				if err := tx.Where("match_id=? AND factory_guid>?", matchID, cursor).Order("factory_guid").Limit(128).Find(&rows).Error; err != nil {
					return err
				}
				for _, row := range rows {
					key := strconv.FormatInt(row.FactoryGUID, 10)
					if _, exists := wanted[key]; exists {
						got[key] = p13UFMixedCarAttackSeal(&row)
					} else {
						extra = append(extra, row)
					}
					if len(got)+len(extra) > 512 {
						return p13UFMixedError()
					}
					cursor = row.FactoryGUID
				}
				if len(rows) < 128 {
					break
				}
			}
			if len(extra) > 0 {
				if err := p13UFMixedHistoricalNoncausalCarTimers(tx, matchID, extra, groups); err != nil {
					return err
				}
			}
		case "gate-item-offer":
			var rows []db.P13UnionFightGateItemOffer
			if err := tx.Select("item_id,source_hash").Where("match_id=?", matchID).Limit(3).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[row.ItemID] = row.SourceHash
			}
		case "guard-state":
			var rows []db.P13UnionFightGuardState
			if err := tx.Select("guard_guid,generation,frame_hash,initial_hash,pending_attempt_id,defeated_at,recover_at,building_event,source_hash").Where("match_id=?", matchID).Limit(11).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				got[strconv.FormatInt(row.GuardGUID, 10)] = p13UFMixedGuardStateSeal(&row)
			}
		case "car":
			var factories []db.P13UnionFightFactoryState
			if err := tx.Select("match_id,guid,completed_at,source_hash,car_hash").Where("match_id=?", matchID).Limit(5).Find(&factories).Error; err != nil {
				return err
			}
			if len(factories) > 4 {
				return p13UFMixedError()
			}
			byFactory := map[int64]*db.P13UnionFightFactoryState{}
			for i := range factories {
				row := &factories[i]
				byFactory[row.GUID] = row
				if row.CompletedAt > 0 {
					got[strconv.FormatInt(p13UFCarInstanceGUID(row), 10)] = p13UFMixedCarSeal(row, nil)
				}
			}
			cursor := int64(0)
			for {
				var rows []db.P13UnionFightCarGeneration
				if err := tx.Select("car_guid,factory_guid,source_hash,car_hash").Where("match_id=? AND car_guid>?", matchID, cursor).Order("car_guid").Limit(128).Find(&rows).Error; err != nil {
					return err
				}
				for i := range rows {
					row := &rows[i]
					actual := byFactory[row.FactoryGUID]
					if actual == nil {
						return p13UFMixedError()
					}
					copy := *actual
					copy.CarHash = row.CarHash
					got[strconv.FormatInt(row.CarGUID, 10)] = p13UFMixedCarSeal(&copy, row)
					cursor = row.CarGUID
				}
				if len(rows) < 128 {
					break
				}
			}
		case "field":
			var row db.P13UnionFightField
			if err := tx.Select("match_id,snapshot_hash").First(&row, "match_id=?", matchID).Error; err != nil {
				return err
			}
			got[row.MatchID] = row.SnapshotHash
		case "actor":
			cursor := int64(0)
			for {
				var rows []db.P13UnionFightFieldPlayer
				if err := tx.Select("player_id,guild_id,camp,battle_hash").Where("match_id=? AND player_id>?", matchID, cursor).Order("player_id").Limit(128).Find(&rows).Error; err != nil {
					return err
				}
				for _, row := range rows {
					got[strconv.FormatInt(row.PlayerID, 10)] = p13UFMixedActorSeal(&row)
					cursor = row.PlayerID
				}
				if len(rows) < 128 {
					break
				}
			}
		case "tower":
			var rows []db.P13UnionFightTowerState
			if err := tx.Where("match_id=?", matchID).Limit(7).Find(&rows).Error; err != nil {
				return err
			}
			for i := range rows {
				row := &rows[i]
				if !p13UFTowerValid(row) {
					return p13UFMixedError()
				}
				got[strconv.FormatInt(row.TowerGUID, 10)] = row.SourceHash
			}
		default:
			return p13UFMixedError()
		}
		if len(got) != len(wanted) {
			return p13UFMixedError()
		}
		for key, hash := range wanted {
			if got[key] != hash {
				return p13UFMixedError()
			}
		}
	}
	return nil
}

// Historical noncausal timer compatibility: these rows have no creation clock.
// Authenticate existing frozen cars/quotes and exclude every car effect; never
// infer a creation time or repair the immutable terminal proof.
func p13UFMixedHistoricalNoncausalCarTimers(tx *gorm.DB, matchID string, extra []db.P13UnionFightCarAttackState, frozen map[string]map[string]string) error {
	if len(extra) == 0 {
		return nil
	}
	if len(extra) > 512 {
		return p13UFMixedError()
	}
	var match db.P13UnionFightMatch
	if err := tx.First(&match, "id=?", matchID).Error; err != nil {
		return err
	}
	ledger, err := p13UFCarImpactLedger(tx, &match)
	if err != nil {
		return err
	}
	cars := map[int64]p13UFCarImpactCar{}
	for _, car := range ledger.Cars {
		cars[car.CarGUID] = car
	}
	impacted := map[int64]bool{}
	for _, hit := range ledger.Impacts {
		impacted[hit.CarGUID] = true
	}
	var offers []db.P13UnionFightCarAttackOffer
	if err := tx.Where("match_id=?", matchID).Limit(5).Find(&offers).Error; err != nil {
		return err
	}
	if len(offers) > 4 {
		return p13UFMixedError()
	}
	byFactory := map[int64]db.P13UnionFightCarAttackOffer{}
	for _, offer := range offers {
		if offer.SourceHash != p13UFCarAttackOfferHash(&offer) || frozen["car-attack-offer"][strconv.FormatInt(offer.FactoryGUID, 10)] != offer.SourceHash {
			return p13UFMixedError()
		}
		byFactory[offer.FactoryGUID] = offer
	}
	for start := 0; start < len(extra); start += 128 {
		if err := tx.Statement.Context.Err(); err != nil {
			return err
		}
		ids := []int64{}
		for _, row := range extra[start:min(start+128, len(extra))] {
			car, ok := cars[row.CarGUID]
			offer := byFactory[car.FactoryGUID]
			stateKey := car.FactoryGUID
			if car.Generation > 1 {
				stateKey = car.CarGUID
			}
			if !ok || row.MatchID != matchID || !row.Completed || row.Hits != 0 || row.PendingReleaseAt != 0 || row.StateHash != p13UFCarAttackStateHash(&row) || row.FactoryGUID != stateKey || row.NextAttackAt < car.BornAt || row.OfferHash != offer.SourceHash || offer.SourceHash == "" || impacted[row.CarGUID] || frozen["car"][strconv.FormatInt(row.CarGUID, 10)] == "" || car.CurrentHP != car.MaxHP {
				return p13UFMixedError()
			}
			ids = append(ids, row.CarGUID)
		}
		var count int64
		if err := tx.Model(&db.P13UnionFightGateImpact{}).Where("match_id=? AND car_guid IN ?", matchID, ids).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return p13UFMixedError()
		}
		if err := tx.Model(&db.P13UnionFightTowerHit{}).Where("match_id=? AND target_kind=? AND target_guid IN ?", matchID, "car", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return p13UFMixedError()
		}
	}
	return nil
}

func p13UFMixedProofVerify(tx *gorm.DB, match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult) error {
	var proof p13UFMixedProof
	if !strings.HasPrefix(result.Source, constUtil.P13UnionFightMixedSettlementSource) || result.CombatProofHash == "" || result.CombatProofHash != p13UFCombatProofDigest(result) || result.MatchID != match.ID || result.EndAt != match.EndAt || json.Unmarshal(result.CombatProof, &proof) != nil || proof.Schema != constUtil.P13UFMixedProofSchema || proof.MatchSHA != match.SnapshotHash || p6Digest(match.Roster) != match.SnapshotHash || proof.LeftPreviewSHA != p6Digest(match.LeftPreview) || proof.RightPreviewSHA != p6Digest(match.RightPreview) || proof.TerminalAt < match.StartAt || proof.TerminalAt > match.EndAt || (proof.Winner != 0 && proof.Winner != 1 && proof.Winner != -2) || proof.ResultKind != result.ResultKind || proof.TerminalAt != result.TerminalAt || proof.Reason != result.Reason || proof.Kills != [2]int32{result.LeftKills, result.RightKills} || proof.Reached != [2]int64{result.LeftReachedAt, result.RightReachedAt} || proof.LedgerHash != result.LedgerHash || proof.LedgerHash != p13UFMixedLedgerHash(&proof) || len(proof.Sources) == 0 && proof.TerminalRule != constUtil.P13UFMixedTerminalRule {
		return p13UFMixedError()
	}
	winner, loser := match.LeftGuildID, match.RightGuildID
	if proof.Winner == 1 {
		winner, loser = loser, winner
	}
	if proof.Winner == -2 {
		if proof.ResultKind != "draw" || proof.TerminalRule != constUtil.P13UFMixedTerminalRule {
			return p13UFMixedError()
		}
		winner, loser = "", ""
	} else if proof.ResultKind != "win" {
		return p13UFMixedError()
	}
	if result.WinnerGuildID != winner || result.LoserGuildID != loser || result.SettledAt < proof.TerminalAt {
		return p13UFMixedError()
	}
	if proof.Reason != "base-defeated" && proof.Reason != "simultaneous-bases" && proof.TerminalAt != match.EndAt {
		return p13UFMixedError()
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		return p13UFMixedError()
	}
	rosterIDs := map[int64]bool{}
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			if rosterIDs[fighter.PlayerID] {
				return p13UFMixedError()
			}
			rosterIDs[fighter.PlayerID] = true
		}
	}
	if len(rosterIDs) != len(proof.Players) {
		return p13UFMixedError()
	}
	for _, id := range proof.Players {
		if !rosterIDs[id] {
			return p13UFMixedError()
		}
		delete(rosterIDs, id)
	}
	var offer db.P13UnionFightSettlementOffer
	if err := tx.First(&offer, "match_id=?", match.ID).Error; err != nil {
		return err
	}
	rule, ruleErr := p13UFSettlementRules(&offer, match)
	if ruleErr != nil {
		return ruleErr
	}
	if result.OfferHash != offer.RulesHash || proof.TerminalRule != rule.TerminalRule || proof.ResultKind == "draw" && (rule.TerminalRule != constUtil.P13UFMixedTerminalRule || rule.DrawPrestige != 5) {
		return p13UFMixedError()
	}
	if err := p13UFMixedProofBusiness(match, result, &proof); err != nil {
		return err
	}
	if err := p13UFMixedFrozenVerify(tx, match.ID, proof.Frozen); err != nil {
		return err
	}
	if err := p13UFAuditRefusalsVerify(tx, match, proof.AuditRefusals); err != nil {
		return err
	}
	groups := map[string][]p13UFMixedMeta{}
	seen := map[string]bool{}
	records := map[string]bool{}
	for _, source := range proof.Sources {
		key := source.Kind + ":" + source.Key
		if seen[key] || source.Key == "" || source.EffectAt < match.StartAt || source.EffectAt > match.EndAt && !p13UFMixedWorldCanceled(source) || source.AppliedAt < source.EffectAt || source.Included != (!source.NoEffect && source.EffectAt <= proof.TerminalAt) || source.CreatedAt > proof.TerminalAt && source.Kind != "revive" {
			return p13UFMixedError()
		}
		seen[key] = true
		groups[source.Kind] = append(groups[source.Kind], source)
		if source.Record != nil {
			rec := source.Record
			if records[rec.ID] || rec.P13MatchID != match.ID || rec.Status != constUtil.P6BattleComputed || !rec.Authoritative {
				return p13UFMixedError()
			}
			records[rec.ID] = true
		}
	}
	for _, entry := range []struct {
		kind  string
		table any
	}{{"ordinary", &db.P13UnionFightPlayerBattleAttempt{}}, {"guard", &db.P13UnionFightGuardBattleAttempt{}}, {"tower", &db.P13UnionFightTowerHit{}}, {"revive", &db.P13UnionFightReviveReceipt{}}, {"shop", &db.P13UnionFightShopPurchase{}}, {"gate", &db.P13UnionFightGateImpact{}}, {"recovery", &db.P13UnionFightPlayerRecovery{}}, {"aura", &db.P13UnionFightHealAuraEffect{}}, {"aura-pending", &db.P13UnionFightHealAuraPending{}}, {"speed", &db.P13UnionFightSpeedItemEffect{}}, {"drum", &db.P13UnionFightWarDrumEffect{}}, {"car-impact", &db.P13UnionFightCarImpact{}}} {
		var count int64
		if err := tx.Model(entry.table).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(groups[entry.kind])) {
			return p13UFMixedError()
		}
	}
	receiptCount, receiptErr := p13UFMixedReceiptCount(tx, match, proof.Players)
	if receiptErr != nil {
		return receiptErr
	}
	if receiptCount != int64(len(groups["item-receipt"])) {
		return p13UFMixedError()
	}
	var p6Count int64
	if err := tx.Model(&db.P6BattleRecord{}).Where("p13_match_id=?", match.ID).Count(&p6Count).Error; err != nil {
		return err
	}
	if p6Count != int64(len(records)+len(proof.AuditRefusals)) {
		return p13UFMixedError()
	}
	closureEvents := []p13UFMixedEvent{}
	for kind, sources := range groups {
		for start := 0; start < len(sources); start += 128 {
			batch := sources[start:min(start+128, len(sources))]
			got, err := p13UFMixedReadMeta(tx, match.ID, kind, batch)
			if err != nil {
				return err
			}
			recordIDs := []string{}
			for _, source := range batch {
				if source.Record != nil {
					recordIDs = append(recordIDs, source.Record.ID)
				}
			}
			recordMap := map[string]p13UFMixedRecordMeta{}
			if len(recordIDs) > 0 {
				var rows []db.P6BattleRecord
				if err = tx.Select("id,p13_match_id,source,source_fight,status,input_hash,outcome_hash,replay_hash,engine_version,client_version,configuration_hash,player_id,authoritative,client_result,server_result,created_at,finished_at").Where("id IN ?", recordIDs).Find(&rows).Error; err != nil {
					return err
				}
				if len(rows) != len(recordIDs) {
					return p13UFMixedError()
				}
				for i := range rows {
					recordMap[rows[i].ID] = p13UFMixedRecordMetadata(&rows[i])
				}
			}
			for _, source := range batch {
				if string(got[source.Key]) != string(source.Data) {
					return p13UFMixedError()
				}
				if source.Record != nil && recordMap[source.Record.ID] != *source.Record {
					return p13UFMixedError()
				}
				closureEvents = append(closureEvents, p13UFMixedEvent{Kind: source.Kind, Key: source.Key, Meta: source})
			}
		}
	}
	return p13UFMixedClosure(tx, match, proof.Players, closureEvents, proof.AuditRefusals)
}

// p13UFMixedTerminalAt is a small admission/worker gate. A committed building
// consumption receipt supplies the provisional cutoff before reward settlement.
// Full native and source verification still happens once in mixed settlement.
func p13UFMixedTerminalAt(tx *gorm.DB, matchID string) (int64, error) {
	values, err := p13UFMixedTerminalBatch(tx, []string{matchID})
	return values[matchID], err
}

// p13UFMixedTerminalBatch reads only small consumed-base receipts and result
// clocks in batches of128. It keeps provisional-base precedence over rewards.
func p13UFMixedTerminalBatch(tx *gorm.DB, matchIDs []string) (map[string]int64, error) {
	result := map[string]int64{}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range matchIDs {
		if id == "" {
			return nil, p13UFMixedError()
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	for start := 0; start < len(unique); start += 128 {
		ids := unique[start:min(start+128, len(unique))]
		var rows []db.P13UnionFightGuardBattleAttempt
		if err := tx.Select("id,match_id,p6_record_id,terminal_hash,status,guard_defeated,building_event,building_applied_at,building_proof,due_at,settled_at").Where("match_id IN ? AND status=? AND guard_defeated=1 AND building_event=? AND building_applied_at>0", ids, constUtil.P13UFGuardSettled, "base-defeated").Order("match_id,due_at,id").Limit(len(ids)*2 + 1).Find(&rows).Error; err != nil {
			return nil, err
		}
		counts := map[string]int{}
		for _, row := range rows {
			counts[row.MatchID]++
			if counts[row.MatchID] > 2 {
				return nil, p13UFMixedError()
			}
			var proof p13UFGuardBuildingProof
			if row.BuildingAppliedAt != row.SettledAt || row.SettledAt < row.DueAt || json.Unmarshal(row.BuildingProof, &proof) != nil || proof.Schema != constUtil.P13UFMixedBuildingSchema || proof.Seal != p13UFGuardBuildingProofSeal(proof) || proof.MatchID != row.MatchID || proof.AttemptID != row.ID || proof.Event != "base-defeated" || proof.AppliedAt != row.BuildingAppliedAt || proof.DueAt != row.DueAt || proof.TerminalHash != row.TerminalHash || proof.P6RecordID != row.P6RecordID || proof.Gate == nil {
				return nil, p13UFMixedError()
			}
			proof.Gate.Frame = append([]byte(nil), proof.GateFrame...)
			gate, err := p13UFGateRead(proof.Gate)
			if err != nil || gate.GetHp() != 0 || proof.Gate.MatchID != row.MatchID || proof.Gate.Camp != proof.GuardCamp {
				return nil, p13UFMixedError()
			}
			if result[row.MatchID] == 0 || row.DueAt < result[row.MatchID] {
				result[row.MatchID] = row.DueAt
			}
		}
		var sealed []db.P13UnionFightSettlementResult
		if err := tx.Select("match_id,end_at,terminal_at").Where("match_id IN ?", ids).Find(&sealed).Error; err != nil {
			return nil, err
		}
		for _, row := range sealed {
			if result[row.MatchID] > 0 {
				continue
			}
			if row.EndAt <= 0 || row.TerminalAt < 0 || row.TerminalAt > row.EndAt {
				return nil, p13UFMixedError()
			}
			if row.TerminalAt > 0 {
				result[row.MatchID] = row.TerminalAt
			} else {
				result[row.MatchID] = row.EndAt
			}
		}
	}
	return result, nil
}

func p13UFMixedGuardChain(tx *gorm.DB, match *db.P13UnionFightMatch, events []p13UFMixedEvent, now time.Time) ([]p13UFMixedFrozenMeta, error) {
	var states []db.P13UnionFightGuardState
	if err := tx.Where("match_id=?", match.ID).Limit(11).Find(&states).Error; err != nil {
		return nil, err
	}
	if len(states) > 10 {
		return nil, p13UFMixedError()
	}
	groups := map[int64][]p13UFMixedEvent{}
	for _, event := range events {
		if event.Kind == "guard" {
			groups[event.GuardGUID] = append(groups[event.GuardGUID], event)
		}
	}
	if len(groups) != len(states) {
		return nil, p13UFMixedError()
	}
	frozen := []p13UFMixedFrozenMeta{}
	for i := range states {
		state := &states[i]
		current, err := p13UFGuardStateFrame(state)
		if err != nil {
			return nil, err
		}
		if state.PendingAttemptID != "" {
			return nil, p13UFMixedError()
		}
		list := groups[state.GuardGUID]
		if len(list) == 0 {
			return nil, p13UFMixedError()
		}
		sort.Slice(list, func(i, j int) bool { return list[i].GuardGeneration < list[j].GuardGeneration })
		initial := &pb.InstUFGuard{}
		if proto.Unmarshal(state.InitialFrame, initial) != nil {
			return nil, p13UFMixedError()
		}
		var previous *p13UFMixedEvent
		for j := range list {
			event := &list[j]
			if previous == nil {
				if event.GuardGeneration != 1 || !proto.Equal(event.GuardBefore, initial) {
					return nil, p13UFMixedError()
				}
			} else if previous.Building == nil {
				if event.GuardGeneration != previous.GuardGeneration+1 || !proto.Equal(previous.GuardAfter.GetFightInfo(), event.GuardBefore.GetFightInfo()) {
					return nil, p13UFMixedError()
				}
			} else {
				recovery := int64(0)
				switch previous.Building.Event {
				case "tower-defeated":
					recovery = 120000
				case "factory-defeated":
					recovery = 300000
				default:
					return nil, p13UFMixedError()
				}
				if event.GuardGeneration != previous.GuardGeneration+2 || event.CreatedAt < previous.DueAt+recovery || !proto.Equal(event.GuardBefore, initial) {
					return nil, p13UFMixedError()
				}
			}
			previous = event
		}
		last := previous
		if state.Generation == last.GuardGeneration+1 {
			if !proto.Equal(current, last.GuardAfter) {
				return nil, p13UFMixedError()
			}
			if last.Building != nil && (state.DefeatedAt != last.DueAt || state.BuildingEvent != last.Building.Event) {
				return nil, p13UFMixedError()
			}
		} else if state.Generation == last.GuardGeneration+2 && last.Building != nil && last.Building.Event != "base-defeated" {
			recovery := int64(120000)
			if last.Building.Event == "factory-defeated" {
				recovery = 300000
			}
			if now.UnixMilli() < last.DueAt+recovery || state.DefeatedAt != 0 || state.BuildingEvent != "" || !proto.Equal(current, initial) {
				return nil, p13UFMixedError()
			}
		} else {
			return nil, p13UFMixedError()
		}
		frozen = append(frozen, p13UFMixedFrozenMeta{"guard-state", strconv.FormatInt(state.GuardGUID, 10), p13UFMixedGuardStateSeal(state)})
	}
	return frozen, nil
}

func p13UFMixedCarSeal(factory *db.P13UnionFightFactoryState, generation *db.P13UnionFightCarGeneration) string {
	source := ""
	if generation != nil {
		source = generation.SourceHash
	}
	raw, _ := json.Marshal([]string{factory.SourceHash, factory.CarHash, source})
	return p6Digest(raw)
}
func p13UFMixedCarClosure(tx *gorm.DB, match *db.P13UnionFightMatch, ledger *p13UFCombatLedger) ([]p13UFMixedFrozenMeta, error) {
	var factories []db.P13UnionFightFactoryState
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&factories).Error; err != nil {
		return nil, err
	}
	if len(factories) > 4 {
		return nil, p13UFMixedError()
	}
	var generations []db.P13UnionFightCarGeneration
	if err := tx.Where("match_id=?", match.ID).Limit(513).Find(&generations).Error; err != nil {
		return nil, err
	}
	if len(generations) > 508 {
		return nil, p13UFMixedError()
	}
	var cycles []db.P13UnionFightFactoryCycleOffer
	if err := tx.Where("match_id=?", match.ID).Limit(5).Find(&cycles).Error; err != nil {
		return nil, err
	}
	cycleMap := map[int64]*db.P13UnionFightFactoryCycleOffer{}
	for i := range cycles {
		cycleMap[cycles[i].FactoryGUID] = &cycles[i]
	}
	factoryMap := map[int64]*db.P13UnionFightFactoryState{}
	frozen := []p13UFMixedFrozenMeta{}
	seen := map[int64]bool{}
	check := func(factory *db.P13UnionFightFactoryState, generation *db.P13UnionFightCarGeneration) error {
		car, err := p13UFCarItemRead(factory)
		if err != nil {
			return err
		}
		seen[car.GetGuid()] = true
		if last := ledger.LastCars[car.GetGuid()]; last != nil {
			if !proto.Equal(last, car) {
				return p13UFMixedError()
			}
		} else if car.GetHp() != car.GetMaxHp() {
			return p13UFMixedError()
		}
		frozen = append(frozen, p13UFMixedFrozenMeta{"car", strconv.FormatInt(car.GetGuid(), 10), p13UFMixedCarSeal(factory, generation)})
		return nil
	}
	for i := range factories {
		row := &factories[i]
		if _, err := p13UFFactoryFrame(row); err != nil {
			return nil, err
		}
		factoryMap[row.GUID] = row
		if row.CompletedAt > 0 {
			if err := check(row, nil); err != nil {
				return nil, err
			}
		}
	}
	for i := range generations {
		row := &generations[i]
		actual := factoryMap[row.FactoryGUID]
		cycle := cycleMap[row.FactoryGUID]
		if actual == nil || cycle == nil || row.SourceHash != p13UFCarGenerationHash(row) || row.CarHash != p6Digest(row.CarFrame) || row.StateHash != p13UFCarGenerationStateHash(row) || row.Generation < 2 || row.Generation > 128 || row.OfferHash != cycle.SourceHash || cycle.SourceHash != p13UFFactoryCycleOfferHash(cycle) || row.Generation > cycle.MaxGenerations || row.BornAt < match.StartAt || row.BornAt >= match.EndAt {
			return nil, p13UFMixedError()
		}
		factory := &db.P13UnionFightFactoryState{}
		if json.Unmarshal(row.FactorySnapshot, factory) != nil {
			return nil, p13UFMixedError()
		}
		if _, err := p13UFFactoryFrame(factory); err != nil {
			return nil, err
		}
		if factory.SourceHash != actual.SourceHash || factory.GUID != row.FactoryGUID || factory.CompletedAt != row.BornAt {
			return nil, p13UFMixedError()
		}
		factory.CarGeneration = row.Generation
		factory.CarFrame = row.CarFrame
		factory.CarHash = row.CarHash
		factory.StateHash = p13UFFactoryStateDigest(factory)
		if p13UFCarInstanceGUID(factory) != row.CarGUID {
			return nil, p13UFMixedError()
		}
		if err := check(factory, row); err != nil {
			return nil, err
		}
	}
	for id := range ledger.LastCars {
		if !seen[id] {
			return nil, p13UFMixedError()
		}
	}
	return frozen, nil
}

func p13UFMixedActorSeal(actor *db.P13UnionFightFieldPlayer) string {
	raw, _ := json.Marshal(struct {
		PlayerID   int64
		GuildID    string
		Camp       int64
		BattleHash string
	}{actor.PlayerID, actor.GuildID, actor.Camp, actor.BattleHash})
	return p6Digest(raw)
}

func p13UFMixedGates(tx *gorm.DB, match *db.P13UnionFightMatch, guards []p13UFMixedEvent) ([]p13UFMixedEvent, []p13UFMixedFrozenMeta, error) {
	ledger, err := p13UFGateImpactLedger(tx, match)
	if err != nil {
		return nil, nil, err
	}
	events := []p13UFMixedEvent{}
	frozen := []p13UFMixedFrozenMeta{}
	for _, gate := range ledger.Gates {
		frozen = append(frozen, p13UFMixedFrozenMeta{"gate", strconv.FormatInt(gate.GUID, 10), p13UFMixedGateSeal(&gate)})
	}
	for _, hit := range ledger.Impacts {
		// Until target life/consumption journals join this consumer, retain item
		// transitions as an explicit blocker rather than bypassing item cooldowns.
		meta := p13UFMixedMeta{Kind: "gate", Key: hit.ID, CreatedAt: hit.LogicalAt, EffectAt: hit.LogicalAt, AppliedAt: hit.AppliedAt, Data: p13UFMixedJSON(hit)}
		events = append(events, p13UFMixedEvent{Kind: "gate", Key: hit.ID, CreatedAt: hit.LogicalAt, DueAt: hit.LogicalAt, AppliedAt: hit.AppliedAt, Meta: meta})
	}
	// A base admission binds the chain prefix that was already destroyed before
	// that actor accepted the base attempt, not a later final visual gate state.
	for _, event := range guards {
		if event.Building == nil || event.Building.Event != "base-defeated" {
			continue
		}
		gate := event.Building.Gate
		if gate == nil || gate.Camp == int64(event.Before[0].GetCamp()) || event.GuardRule == constUtil.P13UFGuardMirrorRule && (event.CreatedAt < match.StartAt+(match.EndAt-match.StartAt)/2 || gate.Camp == 0 && gate.GateID != 1 && gate.GateID != 2 || gate.Camp == 1 && gate.GateID != 5 && gate.GateID != 6) {
			return nil, nil, p13UFMixedError()
		}
		var latest *db.P13UnionFightGateImpact
		for i := range ledger.Impacts {
			hit := &ledger.Impacts[i]
			if hit.GateGUID == gate.GUID && hit.LogicalAt <= event.CreatedAt && hit.AppliedAt <= event.CreatedAt && (latest == nil || hit.Ordinal > latest.Ordinal) {
				latest = hit
			}
		}
		if latest == nil || latest.AfterHash != gate.FrameHash || latest.Ordinal != gate.ImpactOrdinal {
			return nil, nil, p13UFMixedError()
		}
		_, after, err := p13UFGateImpactVerify(latest)
		if err != nil || after.GetHp() != 0 {
			return nil, nil, p13UFMixedError()
		}
	}
	var carOffers []db.P13UnionFightCarAttackOffer
	if err = tx.Select("factory_guid,source_hash").Where("match_id=?", match.ID).Limit(5).Find(&carOffers).Error; err != nil {
		return nil, nil, err
	}
	if len(carOffers) > 4 {
		return nil, nil, p13UFMixedError()
	}
	for _, offer := range carOffers {
		frozen = append(frozen, p13UFMixedFrozenMeta{"car-attack-offer", strconv.FormatInt(offer.FactoryGUID, 10), offer.SourceHash})
	}
	var carStates []db.P13UnionFightCarAttackState
	if err = tx.Where("match_id=?", match.ID).Limit(513).Find(&carStates).Error; err != nil {
		return nil, nil, err
	}
	if len(carStates) > 512 {
		return nil, nil, p13UFMixedError()
	}
	for _, state := range carStates {
		frozen = append(frozen, p13UFMixedFrozenMeta{"car-attack-state", strconv.FormatInt(state.FactoryGUID, 10), p13UFMixedCarAttackSeal(&state)})
	}
	var itemOffers []db.P13UnionFightGateItemOffer
	if err = tx.Select("item_id,source_hash").Where("match_id=?", match.ID).Limit(3).Find(&itemOffers).Error; err != nil {
		return nil, nil, err
	}
	if len(itemOffers) > 2 {
		return nil, nil, p13UFMixedError()
	}
	for _, offer := range itemOffers {
		frozen = append(frozen, p13UFMixedFrozenMeta{"gate-item-offer", offer.ItemID, offer.SourceHash})
	}
	return events, frozen, nil
}

func p13UFMixedGateSeal(row *db.P13UnionFightGateState) string {
	raw, _ := json.Marshal(struct {
		GUID, GateID, Camp, Ordinal int64
		Initial, Frame, Source      string
	}{row.GUID, row.GateID, row.Camp, row.ImpactOrdinal, row.InitialHash, row.FrameHash, row.SourceHash})
	return p6Digest(raw)
}
func p13UFMixedGuardStateSeal(row *db.P13UnionFightGuardState) string {
	raw, _ := json.Marshal(struct {
		GUID, Generation, DefeatedAt, RecoverAt int64
		Frame, Initial, Pending, Event, Source  string
	}{row.GuardGUID, row.Generation, row.DefeatedAt, row.RecoverAt, row.FrameHash, row.InitialHash, row.PendingAttemptID, row.BuildingEvent, row.SourceHash})
	return p6Digest(raw)
}
func p13UFMixedCarAttackSeal(row *db.P13UnionFightCarAttackState) string {
	raw, _ := json.Marshal(row)
	return p6Digest(raw)
}

func p13UFMixedProofBusiness(match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult, proof *p13UFMixedProof) error {
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		return p13UFMixedError()
	}
	camps := map[int64]int{}
	for camp, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			camps[fighter.PlayerID] = camp
		}
	}
	kills := [2]int32{}
	reached := [2]int64{match.StartAt, match.StartAt}
	baseAt := int64(0)
	baseWinner := -1
	for _, source := range proof.Sources {
		if source.Kind != "tower" && source.Kind != "aura-pending" && source.NoEffect {
			return p13UFMixedError()
		}
		switch source.Kind {
		case "ordinary":
			var meta p13UFCombatAttemptMeta
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != meta.ID || source.EffectAt != meta.DueAt || source.CreatedAt != meta.CreatedAt || source.AppliedAt != meta.SettledAt || meta.Status != constUtil.P13UFPlayerBattleSettled || source.Record == nil || source.Record.ID != meta.P6RecordID || source.Record.PlayerID != meta.PlayerID || source.Record.InputHash != meta.InputHash {
				return p13UFMixedError()
			}
			if meta.DeadPlayerID > 0 && source.Included {
				camp, ok := camps[meta.RewardPlayerID]
				if !ok || kills[camp] == math.MaxInt32 {
					return p13UFMixedError()
				}
				kills[camp]++
				reached[camp] = max(reached[camp], meta.DueAt)
			}
		case "guard":
			var meta p13UFMixedGuardMeta
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != meta.ID || source.EffectAt != meta.DueAt || source.AppliedAt != meta.SettledAt || source.CreatedAt != meta.CreatedAt || meta.Status != constUtil.P13UFGuardSettled || meta.CalculationStage != constUtil.P13UFGuardComputed || source.Record == nil || source.Record.ID != meta.P6RecordID || source.Record.PlayerID != meta.PlayerID || source.Record.InputHash != meta.InputHash || source.Record.SourceFight != meta.FightID {
				return p13UFMixedError()
			}
			if meta.GuardDefeated && meta.BuildingEvent == "base-defeated" {
				camp, ok := camps[meta.PlayerID]
				if !ok || meta.BuildingAppliedAt != meta.SettledAt {
					return p13UFMixedError()
				}
				if baseAt == 0 || meta.DueAt < baseAt {
					baseAt = meta.DueAt
					baseWinner = camp
				} else if meta.DueAt == baseAt && baseWinner != camp {
					baseWinner = -2
				}
			}
		case "tower":
			var meta P13UnionFightTowerHitMeta
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != fmt.Sprintf("%d/%d", meta.TowerGUID, meta.Sequence) || source.EffectAt != meta.DueAt || source.AppliedAt != meta.AppliedAt || source.NoEffect != (meta.Status == constUtil.P13UnionFightTowerHitCanceled) || source.CreatedAt != meta.HitAt-600 || (meta.Status != constUtil.P13UnionFightTowerHitApplied && meta.Status != constUtil.P13UnionFightTowerHitCanceled && meta.Status != constUtil.P13UnionFightTowerHitLegacyApplied) {
				return p13UFMixedError()
			}
			if p13UFMixedWorldCanceled(source) {
				var world struct{ CutoffAt, HitAt, DueAt int64 }
				if json.Unmarshal(source.Data, &world) != nil || world.CutoffAt != proof.TerminalAt || (world.HitAt <= proof.TerminalAt && world.DueAt <= proof.TerminalAt) {
					return p13UFMixedError()
				}
			}

		case "aura-pending":
			var meta db.P13UnionFightHealAuraPending
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || meta.Status == constUtil.P13UFHealAuraPending || !source.NoEffect || p13UFAuraPendingVerifyMeta(&meta) != nil {
				return p13UFMixedError()
			}
		case "recovery":
			var meta db.P13UnionFightPlayerRecovery
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != meta.ID || source.CreatedAt != meta.LogicalAt || source.EffectAt != meta.LogicalAt || source.AppliedAt != meta.AppliedAt {
				return p13UFMixedError()
			}
		case "aura":
			var meta db.P13UnionFightHealAuraEffect
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != strconv.FormatInt(meta.GUID, 10) || source.EffectAt != meta.StartedAt || source.CreatedAt != meta.StartedAt || source.AppliedAt != meta.StartedAt {
				return p13UFMixedError()
			}
		case "speed":
			var meta db.P13UnionFightSpeedItemEffect
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != strconv.FormatInt(meta.GUID, 10) || source.EffectAt != meta.StartedAt || source.CreatedAt != meta.StartedAt || source.AppliedAt != meta.StartedAt {
				return p13UFMixedError()
			}
		case "drum":
			var meta db.P13UnionFightWarDrumEffect
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != strconv.FormatInt(meta.GUID, 10) || source.EffectAt != meta.StartedAt || source.CreatedAt != meta.StartedAt || source.AppliedAt != meta.StartedAt {
				return p13UFMixedError()
			}
		case "car-impact":
			var meta P13UnionFightCarImpactMeta
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != meta.ID || source.EffectAt != meta.LogicalAt || source.CreatedAt != meta.LogicalAt || source.AppliedAt != meta.AppliedAt {
				return p13UFMixedError()
			}
		case "item-receipt":
			var meta p13UFMixedItemReceiptMeta
			if json.Unmarshal(source.Data, &meta) != nil || source.Key != fmt.Sprintf("%d/%d", meta.PlayerID, meta.Stamp) || meta.Operation != constUtil.P13UnionFightItemUseOperation || source.EffectAt != meta.CreatedAt || source.CreatedAt != meta.CreatedAt || source.AppliedAt != meta.CreatedAt {
				return p13UFMixedError()
			}
		case "revive":
			var meta P13UnionFightNaturalReviveMeta
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != fmt.Sprintf("%d/%d", meta.PlayerID, meta.DeadOrdinal) || source.CreatedAt != meta.DeathAt || source.EffectAt != meta.DueAt || source.AppliedAt != meta.AppliedAt {
				return p13UFMixedError()
			}
		case "shop":
			var meta db.P13UnionFightShopPurchase
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != meta.ID || source.CreatedAt != meta.LogicalAt || source.EffectAt != meta.LogicalAt || source.AppliedAt != meta.LogicalAt {
				return p13UFMixedError()
			}
		case "gate":
			var meta db.P13UnionFightGateImpact
			if json.Unmarshal(source.Data, &meta) != nil || meta.MatchID != match.ID || source.Key != meta.ID || source.CreatedAt != meta.LogicalAt || source.EffectAt != meta.LogicalAt || source.AppliedAt != meta.AppliedAt {
				return p13UFMixedError()
			}
		default:
			return p13UFMixedError()
		}
	}
	if err := p13UFMixedAuraMetaBindings(proof); err != nil {
		return err
	}
	if kills != proof.Kills || reached != proof.Reached {
		return p13UFMixedError()
	}
	var left, right pb.UFInfoPreView
	if proto.Unmarshal(match.LeftPreview, &left) != nil || proto.Unmarshal(match.RightPreview, &right) != nil || result.LeftPower != left.GetFightPower() || result.RightPower != right.GetFightPower() {
		return p13UFMixedError()
	}
	expected, reason := -1, ""
	if baseAt > 0 {
		if baseAt != proof.TerminalAt {
			return p13UFMixedError()
		}
		expected = baseWinner
		reason = "base-defeated"
		if expected == -2 {
			reason = "simultaneous-bases"
		}
	} else {
		expected, reason = p13UFCombatWinner(kills, reached, []int64{result.LeftPower, result.RightPower})
		if expected < 0 && proof.TerminalRule == constUtil.P13UFMixedTerminalRule {
			expected = -2
			reason = "equal-comparison"
		}
	}
	if expected != proof.Winner || reason != proof.Reason {
		return p13UFMixedError()
	}
	return nil
}

func p13UFMixedReviveAt(death p13UFMixedEvent) int64 {
	if death.Kind == "tower" {
		var meta P13UnionFightTowerHitMeta
		if json.Unmarshal(death.Meta.Data, &meta) == nil {
			return meta.ReviveAt
		}
		return 0
	}
	return death.DueAt + death.ReviveSeconds*1000
}
func p13UFMixedActorRoster(match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer) bool {
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		return false
	}
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			if fighter.PlayerID == actor.PlayerID {
				return fighter.GuildID == actor.GuildID && fighter.SnapshotHash == actor.BattleHash
			}
		}
	}
	return false
}
func p13UFMixedHeroSeal(hero *pb.BattleHero) string {
	if hero == nil || hero.Inst == nil {
		return ""
	}
	copy := proto.Clone(hero).(*pb.BattleHero)
	copy.Inst.CurHp = proto.Float64(0)
	raw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(copy)
	return p6Digest(raw)
}
func p13UFMixedTowerBindings(match *db.P13UnionFightMatch, events []p13UFMixedEvent, actors []db.P13UnionFightFieldPlayer) error {
	occupancies := map[string]p13UFMixedEvent{}
	immutable := map[string]P13UnionFightTowerHitMeta{}
	for _, event := range events {
		if event.Kind == "ordinary" || event.Kind == "guard" {
			occupancies[event.Kind+":"+event.Key] = event
		}
		if event.Kind == "tower" {
			var meta P13UnionFightTowerHitMeta
			if json.Unmarshal(event.Meta.Data, &meta) != nil {
				return p13UFMixedError()
			}
			if meta.ImmutableHash != "" {
				if _, ok := immutable[meta.ImmutableHash]; ok {
					return p13UFMixedError()
				}
				immutable[meta.ImmutableHash] = meta
			}
		}
	}
	rosterHashes, hashErr := p13UFMixedRosterBattleHashes(match)
	if hashErr != nil {
		return hashErr
	}
	actorMap := map[int64]*db.P13UnionFightFieldPlayer{}
	for i := range actors {
		if rosterHashes[actors[i].PlayerID] != actors[i].BattleHash {
			return p13UFMixedError()
		}
		actorMap[actors[i].PlayerID] = &actors[i]
	}
	parties := map[int64]*pb.BattlePlayer{}
	for _, event := range events {
		if event.Kind != "tower" {
			continue
		}
		var meta P13UnionFightTowerHitMeta
		if json.Unmarshal(event.Meta.Data, &meta) != nil {
			return p13UFMixedError()
		}
		if meta.Status == constUtil.P13UnionFightTowerHitLegacyApplied || meta.TargetKind != "player" || p13UFMixedWorldCanceled(event.Meta) {
			continue
		}
		actor := actorMap[meta.TargetGUID]
		if actor == nil || rosterHashes[actor.PlayerID] != actor.BattleHash {
			return p13UFMixedError()
		}
		party := parties[actor.PlayerID]
		if party == nil {
			party = &pb.BattlePlayer{}
			if p6Digest(actor.Battle) != actor.BattleHash || proto.Unmarshal(actor.Battle, party) != nil {
				return p13UFMixedError()
			}
			parties[actor.PlayerID] = party
		}
		found := false
		for _, hero := range party.Formation {
			if hero.GetGuid() == meta.ActorGUID && int64(hero.GetPose()) == meta.ActorPose && p13UFMixedHeroSeal(hero) == p13UFMixedHeroSeal(event.TowerActor) {
				found = true
			}
		}
		if !found {
			return p13UFMixedError()
		}
		if meta.OccupancyKind != "" {
			occupancy, ok := occupancies[meta.OccupancyKind+":"+meta.OccupancyID]
			if !ok || occupancy.Meta.Record == nil || meta.OccupancyInputHash != occupancy.Meta.Record.InputHash || meta.OccupancyDueAt != occupancy.DueAt || meta.DueAt != occupancy.DueAt {
				return p13UFMixedError()
			}
			bound := false
			for _, frame := range occupancy.Before {
				if frame.GetGuid() == meta.TargetGUID && event.TowerRelease != nil && frame.GetDeadNum() == event.TowerRelease.GetDeadNum() && frame.GetKillNum() == event.TowerRelease.GetKillNum() && proto.Equal(frame.FightInfo, event.TowerRelease.FightInfo) {
					bound = true
				}
			}
			if !bound {
				return p13UFMixedError()
			}
		}
		if meta.LaunchOccupancyID != "" {
			launch, ok := occupancies[meta.LaunchOccupancyKind+":"+meta.LaunchOccupancyID]
			if !ok || launch.Meta.Record == nil || launch.Meta.Record.InputHash != meta.LaunchOccupancyInputHash || launch.DueAt != meta.LaunchOccupancyDueAt {
				return p13UFMixedError()
			}
			bound := false
			for _, frame := range launch.Before {
				if event.TowerCapture != nil && frame.GetGuid() == meta.TargetGUID && frame.GetDeadNum() == event.TowerCapture.GetDeadNum() && frame.GetKillNum() == event.TowerCapture.GetKillNum() && proto.Equal(frame.FightInfo, event.TowerCapture.FightInfo) {
					bound = true
				}
			}
			if !bound {
				return p13UFMixedError()
			}
		}
		if meta.CoalescedInto != "" {
			first, ok := immutable[meta.CoalescedInto]
			if !ok || first.ImmutableHash == meta.ImmutableHash || first.TowerGUID != meta.TowerGUID || first.TargetGUID != meta.TargetGUID || first.LifeOrdinal != meta.LifeOrdinal || first.OccupancyID != meta.OccupancyID || first.OccupancyKind != meta.OccupancyKind {
				return p13UFMixedError()
			}
		}
	}
	return nil
}

func p13UFMixedRosterBattleHashes(match *db.P13UnionFightMatch) (map[int64]string, error) {
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		return nil, p13UFMixedError()
	}
	result := map[int64]string{}
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			if result[fighter.PlayerID] != "" {
				return nil, p13UFMixedError()
			}
			result[fighter.PlayerID] = fighter.SnapshotHash
		}
	}
	return result, nil
}

// Round identity and prestige are shared by weekly/monthly consumers. A draw
// never assigns either guild to the historical loser branch.
func p13UFMixedRoundIdentity(match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult, rule *db.P13UnionFightSettlementPolicy) bool {
	if result.OfferHash == "" || result.EndAt != match.EndAt {
		return false
	}
	if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
		return p13UFAbortEnabled(rule) && result.Source == constUtil.P13UFTechnicalAbortSource && result.WinnerGuildID == "" && result.LoserGuildID == "" && result.TerminalAt >= match.StartAt && result.TerminalAt <= match.EndAt
	}
	if result.ResultKind == "draw" {
		return rule.TerminalRule == constUtil.P13UFMixedTerminalRule && rule.DrawPrestige == 5 && result.WinnerGuildID == "" && result.LoserGuildID == "" && strings.HasPrefix(result.Source, constUtil.P13UnionFightMixedSettlementSource) && result.TerminalAt >= match.StartAt && result.TerminalAt <= match.EndAt
	}
	if result.ResultKind != "" && result.ResultKind != "win" {
		return false
	}
	return result.WinnerGuildID != result.LoserGuildID && (result.WinnerGuildID == match.LeftGuildID || result.WinnerGuildID == match.RightGuildID) && (result.LoserGuildID == match.LeftGuildID || result.LoserGuildID == match.RightGuildID)
}
func p13UFMixedRoundPrestige(match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult, rule *db.P13UnionFightSettlementPolicy, guild string) (int64, int32, error) {
	if !p13UFMixedRoundIdentity(match, result, rule) || guild != match.LeftGuildID && guild != match.RightGuildID {
		return 0, 0, p13UFMixedError()
	}
	if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
		return 0, 0, nil
	}
	if result.ResultKind == "draw" {
		extra := result.WinTaskPrestige
		if guild == match.RightGuildID {
			extra = result.LoseTaskPrestige
		}
		return int64(rule.DrawPrestige), extra, nil
	}
	if guild == result.WinnerGuildID {
		return int64(rule.WinPrestige), result.WinTaskPrestige, nil
	}
	return int64(rule.LosePrestige), result.LoseTaskPrestige, nil
}

func p13UFMixedCombatOwners(players ...*pb.BattlePlayer) map[string]int64 {
	owners := map[string]int64{}
	for _, player := range players {
		if player == nil {
			continue
		}
		for _, hero := range player.Formation {
			owners[hero.GetGuid()] = player.GetPlayerId()
		}
	}
	return owners
}

func p13UFMixedWorldCanceled(meta p13UFMixedMeta) bool {
	if meta.Kind != "tower" || !meta.NoEffect {
		return false
	}
	var row struct {
		Status, CancelReason string
		CutoffAt             int64
	}
	return json.Unmarshal(meta.Data, &row) == nil && row.Status == constUtil.P13UnionFightTowerHitCanceled && row.CancelReason == "world-terminal-cutoff" && row.CutoffAt > 0
}
