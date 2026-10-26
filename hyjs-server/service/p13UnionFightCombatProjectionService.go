package service

import (
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"time"
)

type p13UFCombatProjectionKey struct {
	Match  string
	Player int64
}
type p13UFCombatClockProjection struct {
	ID, Kind                      string
	CreatedAt, DueAt, TotalMillis int64
	Bound                         *pb.InstUFPlayer
}

// Batch projects only current combat participants. Each chunk reads pending
// once per kind and latest death IDs before fetching bounded full journals.
// Frames remain untouched when no matching current proof exists.
func p13UFFieldCombatProjectBatch(tx *gorm.DB, actors []db.P13UnionFightFieldPlayer, frames []*pb.InstUFPlayer, now time.Time) error {
	if tx == nil || len(actors) != len(frames) {
		return errors.New(constUtil.P13UFPlayerBattleConflict)
	}
	for begin := 0; begin < len(actors); begin += 128 {
		end := min(begin+128, len(actors))
		if err := p13UFCombatProjectChunk(tx, actors[begin:end], frames[begin:end], now); err != nil {
			return err
		}
	}
	return nil
}

func p13UFCombatLatestDeathIDs(tx *gorm.DB, table, playerColumn, orderColumn, status string, pairs [][]any, guard bool) ([]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	// Identifiers are fixed internal call-site literals, never request data.
	sub := tx.Table(table).Select("id, ROW_NUMBER() OVER (PARTITION BY match_id,"+playerColumn+" ORDER BY "+orderColumn+" DESC,id DESC) AS rank_no").Where("status=? AND (match_id,"+playerColumn+") IN ?", status, pairs)
	if guard {
		sub = sub.Where("player_died=?", true)
	}
	var ids []string
	err := tx.Table("(?) AS ranked", sub).Where("rank_no=1").Pluck("id", &ids).Error
	return ids, err
}

func p13UFCombatProjectChunk(tx *gorm.DB, actors []db.P13UnionFightFieldPlayer, frames []*pb.InstUFPlayer, now time.Time) error {
	indices := map[p13UFCombatProjectionKey]int{}
	pendingPairs, deathPairs := [][]any{}, [][]any{}
	for i, actor := range actors {
		frame := frames[i]
		if frame == nil || actor.PlayerID <= 0 || actor.MatchID == "" || frame.GetGuid() != actor.PlayerID || !p13UFMapProjectionKnown(actor.MapProjection) {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		key := p13UFCombatProjectionKey{actor.MatchID, actor.PlayerID}
		if _, seen := indices[key]; seen {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		indices[key] = i
		if frame.GetState() == 1 {
			pendingPairs = append(pendingPairs, []any{actor.MatchID, actor.PlayerID})
		}
		if frame.GetState() == 3 {
			deathPairs = append(deathPairs, []any{actor.MatchID, actor.PlayerID})
		}
	}
	if len(pendingPairs) == 0 && len(deathPairs) == 0 {
		return nil
	}
	var ordinaryPending []db.P13UnionFightPlayerBattleAttempt
	var guardPending []db.P13UnionFightGuardBattleAttempt
	if len(pendingPairs) > 0 {
		if err := tx.Where("status=? AND ((match_id,player_id) IN ? OR (match_id,target_id) IN ?)", constUtil.P13UFPlayerBattlePending, pendingPairs, pendingPairs).Limit(128).Find(&ordinaryPending).Error; err != nil {
			return err
		}
		if err := tx.Where("status=? AND (match_id,player_id) IN ?", constUtil.P13UFGuardPending, pendingPairs).Limit(128).Find(&guardPending).Error; err != nil {
			return err
		}
	}
	ordinaryIDs, err := p13UFCombatLatestDeathIDs(tx, "p13_union_fight_player_battle_attempts", "dead_player_id", "due_at", constUtil.P13UFPlayerBattleSettled, deathPairs, false)
	if err != nil {
		return err
	}
	guardIDs, err := p13UFCombatLatestDeathIDs(tx, "p13_union_fight_guard_battle_attempts", "player_id", "created_at", constUtil.P13UFGuardSettled, deathPairs, true)
	if err != nil {
		return err
	}
	var ordinaryDead []db.P13UnionFightPlayerBattleAttempt
	var guardDead []db.P13UnionFightGuardBattleAttempt
	if len(ordinaryIDs) > 0 {
		if err := tx.Where("id IN ?", ordinaryIDs).Find(&ordinaryDead).Error; err != nil {
			return err
		}
		if len(ordinaryDead) != len(ordinaryIDs) {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
	}
	if len(guardIDs) > 0 {
		if err := tx.Where("id IN ?", guardIDs).Find(&guardDead).Error; err != nil {
			return err
		}
		if len(guardDead) != len(guardIDs) {
			return errors.New(constUtil.P13UFGuardConflict)
		}
	}
	// One frozen-offer fetch serves both ordinary pending and death records.
	matchSet := map[string]bool{}
	for _, row := range ordinaryPending {
		matchSet[row.MatchID] = true
	}
	for _, row := range ordinaryDead {
		matchSet[row.MatchID] = true
	}
	offers := map[string]db.P13UnionFightPlayerBattleOffer{}
	if len(matchSet) > 0 {
		matches := []string{}
		for id := range matchSet {
			matches = append(matches, id)
		}
		var rows []db.P13UnionFightPlayerBattleOffer
		if err := tx.Where("match_id IN ?", matches).Find(&rows).Error; err != nil {
			return err
		}
		for _, offer := range rows {
			offers[offer.MatchID] = offer
		}
	}
	pending := map[p13UFCombatProjectionKey]p13UFCombatClockProjection{}
	deaths := map[p13UFCombatProjectionKey]p13UFCombatClockProjection{}
	putPending := func(key p13UFCombatProjectionKey, value p13UFCombatClockProjection) error {
		index, exists := indices[key]
		if !exists || frames[index].GetState() != 1 {
			return nil
		}
		if _, duplicate := pending[key]; duplicate {
			return errors.New("公会战角色存在重复战斗占用")
		}
		pending[key] = value
		return nil
	}
	putDeath := func(key p13UFCombatProjectionKey, value p13UFCombatClockProjection) error {
		index, exists := indices[key]
		if !exists || frames[index].GetState() != 3 {
			return nil
		}
		old, found := deaths[key]
		if !found || value.DueAt > old.DueAt {
			deaths[key] = value
			return nil
		}
		if value.DueAt == old.DueAt && value.ID != old.ID {
			return errors.New("公会战不同死亡来源生效时刻冲突")
		}
		return nil
	}
	for _, row := range ordinaryPending {
		offer, ok := offers[row.MatchID]
		if !ok {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		rules, e := p13UFPlayerBattleRulesFromOffer(&offer, &row)
		if e != nil || row.SourceHash != p13UFPlayerBattleHash(&row) || row.Status != constUtil.P13UFPlayerBattlePending || row.SettledAt != 0 {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		var before []p13UFPlayerBattleSide
		if json.Unmarshal(row.Before, &before) != nil {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		bound, e := p13UFPlayerBattleFrames(before)
		if e != nil || len(bound) != 2 {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		if bound[0].GetGuid() != row.PlayerID || bound[1].GetGuid() != row.TargetID {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		for _, frame := range bound {
			if e = putPending(p13UFCombatProjectionKey{row.MatchID, frame.GetGuid()}, p13UFCombatClockProjection{ID: row.ID, Kind: "ordinary", CreatedAt: row.CreatedAt, DueAt: row.DueAt, TotalMillis: rules.DurationMillis, Bound: frame}); e != nil {
				return e
			}
		}
	}
	for _, row := range guardPending {
		if e := p13UFCombatGuardClockValid(&row, false); e != nil {
			return e
		}
		bound := &pb.InstUFPlayer{}
		if proto.Unmarshal(row.PlayerBefore, bound) != nil || bound.GetGuid() != row.PlayerID {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		if err = putPending(p13UFCombatProjectionKey{row.MatchID, row.PlayerID}, p13UFCombatClockProjection{ID: row.ID, Kind: "guard", CreatedAt: row.CreatedAt, DueAt: row.DueAt, TotalMillis: 5000, Bound: bound}); err != nil {
			return err
		}
	}
	for _, row := range ordinaryDead {
		offer, ok := offers[row.MatchID]
		if !ok {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		rules, e := p13UFPlayerBattleRulesFromOffer(&offer, &row)
		if e != nil || row.SourceHash != p13UFPlayerBattleHash(&row) || row.Status != constUtil.P13UFPlayerBattleSettled || row.ReviveSeconds <= 0 || row.ReviveSeconds > 86400 || row.SettledAt < row.DueAt || (row.DeadPlayerID != row.PlayerID && row.DeadPlayerID != row.TargetID) || row.DueAt > math.MaxInt64-row.ReviveSeconds*1000 {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		var after []*pb.InstUFPlayer
		if json.Unmarshal(row.After, &after) != nil || len(after) != 2 {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		var bound *pb.InstUFPlayer
		for _, frame := range after {
			if frame == nil {
				return errors.New(constUtil.P13UFPlayerBattleConflict)
			}
			if frame.GetGuid() == row.DeadPlayerID {
				if bound != nil {
					return errors.New(constUtil.P13UFPlayerBattleConflict)
				}
				bound = frame
			}
		}
		if bound == nil || bound.GetState() != 3 || bound.GetDeadNum() <= 0 || row.ReviveSeconds != rules.ReviveSeconds[min(int(bound.GetDeadNum())-1, len(rules.ReviveSeconds)-1)] {
			return errors.New(constUtil.P13UFPlayerBattleConflict)
		}
		if err = putDeath(p13UFCombatProjectionKey{row.MatchID, row.DeadPlayerID}, p13UFCombatClockProjection{ID: row.ID, Kind: "ordinary", CreatedAt: row.CreatedAt, DueAt: row.DueAt, TotalMillis: row.ReviveSeconds * 1000, Bound: bound}); err != nil {
			return err
		}
	}
	for _, row := range guardDead {
		if e := p13UFCombatGuardClockValid(&row, true); e != nil {
			return e
		}
		bound := &pb.InstUFPlayer{}
		var rules p13UFGuardBattleRules
		if proto.Unmarshal(row.PlayerAfter, bound) != nil || bound.GetGuid() != row.PlayerID || bound.GetState() != 3 || bound.GetDeadNum() <= 0 || json.Unmarshal(row.Rules, &rules) != nil || row.ReviveSeconds != rules.ReviveSeconds[min(int(bound.GetDeadNum())-1, len(rules.ReviveSeconds)-1)] {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		if err = putDeath(p13UFCombatProjectionKey{row.MatchID, row.PlayerID}, p13UFCombatClockProjection{ID: row.ID, Kind: "guard", CreatedAt: row.CreatedAt, DueAt: row.DueAt, TotalMillis: row.ReviveSeconds * 1000, Bound: bound}); err != nil {
			return err
		}
	}
	// Validate the entire chunk before mutating caller frames. A newer terminal
	// that does not bind the current HP/death count never falls back to older history.
	for key, index := range indices {
		frame := frames[index]
		candidate, ok := pending[key]
		if frame.GetState() == 3 {
			candidate, ok = deaths[key]
		}
		if !ok || candidate.Bound.GetDeadNum() != frame.GetDeadNum() || !proto.Equal(candidate.Bound.FightInfo, frame.FightInfo) {
			continue
		}
		end := candidate.DueAt
		if frame.GetState() == 3 {
			end += candidate.TotalMillis
		}
		frame.LeftTime = proto.Float32(float32(max(int64(0), end-now.UnixMilli())) / 1000)
		frame.TotalTime = proto.Float32(float32(candidate.TotalMillis) / 1000)
	}
	return nil
}

func p13UFCombatGuardClockValid(row *db.P13UnionFightGuardBattleAttempt, dead bool) error {
	if row.SourceHash != p13UFGuardBattleHash(row) || row.CreatedAt <= 0 || row.CreatedAt > math.MaxInt64-5000 || row.DueAt != row.CreatedAt+5000 {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	var rules p13UFGuardBattleRules
	if json.Unmarshal(row.Rules, &rules) != nil || !p13UFGuardRulesValid(&rules) || row.CreatedAt < rules.Policy.StartsAt || row.DueAt >= rules.Policy.EndsAt || rules.Policy.PublishedAt > row.CreatedAt {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	if dead {
		if row.Status != constUtil.P13UFGuardSettled || !row.PlayerDied || row.ReviveSeconds <= 0 || row.ReviveSeconds > 86400 || row.SettledAt < row.DueAt || row.DueAt > math.MaxInt64-row.ReviveSeconds*1000 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
	} else if row.Status != constUtil.P13UFGuardPending || row.SettledAt != 0 {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	return nil
}
