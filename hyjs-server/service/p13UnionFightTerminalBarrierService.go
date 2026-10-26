package service

import (
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFBaseBarrierAt(tx *gorm.DB, scope string, now time.Time) (int64, error) {
	values, err := p13UFBaseBarrierBatch(tx, []string{scope}, now)
	return values[scope], err
}

// A barrier certifies only a due, pending base attempt. It declares no winner
// and never replaces the actual consumed-base terminal clock. Full attempts
// are bounded to two bases per match; P6 records/history are not loaded.
func p13UFBaseBarrierBatch(tx *gorm.DB, scopes []string, now time.Time) (map[string]int64, error) {
	result := map[string]int64{}
	if tx == nil {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if scope == "" {
			return nil, errors.New(constUtil.P13UFGuardConflict)
		}
		if !seen[scope] {
			unique = append(unique, scope)
			seen[scope] = true
		}
	}
	legacyBase := "JSON_UNQUOTE(JSON_EXTRACT(CAST(rules AS CHAR), '$.BuildingKind')) = 'base'"
	if tx.Dialector.Name() == "sqlite" {
		legacyBase = "json_extract(CAST(rules AS TEXT), '$.BuildingKind') = 'base'"
	}
	for start := 0; start < len(unique); start += 128 {
		ids := unique[start:min(start+128, len(unique))]
		candidates := tx.Model(&db.P13UnionFightGuardBattleAttempt{}).
			Select("*, ROW_NUMBER() OVER (PARTITION BY match_id ORDER BY due_at,id) AS base_rank").
			Where("match_id IN ? AND status=? AND due_at<=?", ids, constUtil.P13UFGuardPending, now.UnixMilli()).
			Where("(building_kind = 'base' OR (building_kind = '' AND (" + legacyBase + "))) ")
		var rows []db.P13UnionFightGuardBattleAttempt
		if err := tx.Table("(?) AS base_barrier", candidates).Where("base_rank<=?", 3).Order("match_id,due_at,id").Find(&rows).Error; err != nil {
			return nil, err
		}
		counts := map[string]int{}
		guards := map[string]map[int64]bool{}
		for i := range rows {
			row := &rows[i]
			counts[row.MatchID]++
			if counts[row.MatchID] > 2 || !seen[row.MatchID] || !p13UFBaseBarrierCandidate(row, now) {
				return nil, errors.New(constUtil.P13UFGuardConflict)
			}
			if guards[row.MatchID] == nil {
				guards[row.MatchID] = map[int64]bool{}
			}
			if guards[row.MatchID][row.GuardGUID] {
				return nil, errors.New(constUtil.P13UFGuardConflict)
			}
			guards[row.MatchID][row.GuardGUID] = true
			if result[row.MatchID] == 0 || row.DueAt < result[row.MatchID] {
				result[row.MatchID] = row.DueAt
			}
		}
	}
	return result, nil
}

func p13UFBaseBarrierCandidate(row *db.P13UnionFightGuardBattleAttempt, now time.Time) bool {
	if row == nil || row.ID == "" || row.MatchID == "" || row.Status != constUtil.P13UFGuardPending || row.CreatedAt <= 0 || row.DueAt-row.CreatedAt != 5000 || row.DueAt > now.UnixMilli() || row.SettledAt != 0 || row.BuildingAppliedAt != 0 || len(row.BuildingProof) != 0 || row.SourceHash != p13UFGuardBattleHash(row) {
		return false
	}
	if row.CalculationStage != constUtil.P13UFGuardCaptured && row.CalculationStage != constUtil.P13UFGuardComputed {
		return false
	}
	var rules p13UFGuardBattleRules
	if json.Unmarshal(row.Rules, &rules) != nil || !p13UFGuardRulesValid(&rules) || rules.BuildingKind != "base" || row.BuildingKind != "" && row.BuildingKind != rules.BuildingKind || rules.Policy.PublishedAt > row.CreatedAt || row.CreatedAt < rules.Policy.StartsAt || row.DueAt >= rules.Policy.EndsAt {
		return false
	}
	if rules.LocalRule == constUtil.P13UFGuardMirrorRule && (row.CreatedAt < rules.MatchStartAt+(rules.MatchEndAt-rules.MatchStartAt)/2 || row.DueAt >= rules.MatchEndAt) {
		return false
	}
	guard, player := &pb.InstUFGuard{}, &pb.InstUFPlayer{}
	if proto.Unmarshal(row.GuardBefore, guard) != nil || proto.Unmarshal(row.PlayerBefore, player) != nil || guard.GetGuid() != row.GuardGUID || player.GetGuid() != row.PlayerID || guard.GetCamp() < 0 || guard.GetCamp() > 1 || player.GetCamp() < 0 || player.GetCamp() > 1 || guard.GetCamp() == player.GetCamp() {
		return false
	}
	return guard.GetUFGuardId() == "baseCamp_guard_001" || guard.GetUFGuardId() == "baseCamp_guard_002"
}
