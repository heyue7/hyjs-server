package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// A single consistent read prevents a base apply between the terminal and
// barrier reads from exposing neither clock. This transaction finishes before
// any worker takes the guild policy lock or opens its mutation transaction.
type p13UFWorldClocks struct {
	terminal map[string]int64
	barrier  map[string]int64
	closed   map[string]bool
}

func p13UFWorldClockBatch(ctx context.Context, scopes []string, now time.Time) (*p13UFWorldClocks, error) {
	clocks := &p13UFWorldClocks{terminal: map[string]int64{}, barrier: map[string]int64{}, closed: map[string]bool{}}
	if len(scopes) == 0 {
		return clocks, nil
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		clocks.closed, err = p13UFWorldClosedBatch(tx, scopes)
		if err != nil {
			return err
		}
		active := make([]string, 0, len(scopes))
		for _, scope := range scopes {
			if !clocks.closed[scope] {
				active = append(active, scope)
			}
		}
		clocks.terminal, err = p13UFMixedTerminalBatch(tx, active)
		if err != nil {
			return err
		}
		clocks.barrier, err = p13UFBaseBarrierBatch(tx, active, now)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return clocks, err
}
func (c *p13UFWorldClocks) paused(scope string) bool { return c.closed[scope] || c.barrier[scope] > 0 }
func (c *p13UFWorldClocks) end(match *db.P13UnionFightMatch) int64 {
	end := match.EndAt
	if at := c.terminal[match.ID]; at > 0 && at < end {
		end = at
	}
	return end
}

// Only committed technical-abort result identity is needed here. No source PB,
// abort proof or pending journal is read/repaired by the world gate.
func p13UFWorldClosedBatch(tx *gorm.DB, scopes []string) (map[string]bool, error) {
	if tx == nil {
		return nil, errors.New("公会战封账查询无效")
	}
	result := map[string]bool{}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range scopes {
		if id == "" {
			return nil, errors.New("公会战封账场次无效")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for offset := 0; offset < len(ids); offset += 128 {
		if err := tx.Statement.Context.Err(); err != nil {
			return nil, err
		}
		var rows []db.P13UnionFightSettlementResult
		if err := tx.Select("match_id,result_kind").Where("match_id IN ? AND result_kind=?", ids[offset:min(offset+128, len(ids))], constUtil.P13UFTechnicalAbortKind).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result[row.MatchID] = true
		}
	}
	return result, nil
}

// Caller already owns the guild-policy UPDATE lock. A current read is required:
// the clock guard may have established an older repeatable-read snapshot before
// that lock was acquired. The same lock serializes abort's result commit.
func p13UFWorldClosed(tx *gorm.DB, scope string) (bool, error) {
	if tx == nil || scope == "" {
		return false, errors.New("公会战封账场次无效")
	}
	var row db.P13UnionFightSettlementResult
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("match_id,result_kind").First(&row, "match_id=?", scope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.ResultKind == constUtil.P13UFTechnicalAbortKind, nil
}

// The caller owns the guild-policy UPDATE lock. Any committed result seals
// the world directory, including normal wins/draws. Keep this separate from
// WorldClosed, whose abort-only meaning is also used by field cleanup.
func p13UFWorldSealed(tx *gorm.DB, scope string) (bool, error) {
	if tx == nil || scope == "" {
		return false, errors.New("公会战封账场次无效")
	}
	var row db.P13UnionFightSettlementResult
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("match_id").First(&row, "match_id=?", scope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Field actors in a sealed abort remain immutable and retain their old left_at.
// A PK lookup of the scalar result excludes them from current/live selection.
func p13UFWorldUnclosedActors(tx *gorm.DB) *gorm.DB {
	return tx.Where("NOT EXISTS (SELECT 1 FROM p13_union_fight_settlement_results AS closed_world WHERE closed_world.match_id=p13_union_fight_field_players.match_id AND closed_world.result_kind=?)", constUtil.P13UFTechnicalAbortKind)
}

func p13UFWorldHasActiveActor(tx *gorm.DB, id int64) (bool, error) {
	var actor db.P13UnionFightFieldPlayer
	err := p13UFWorldUnclosedActors(tx.Clauses(clause.Locking{Strength: "UPDATE"})).Select("match_id,player_id").First(&actor, "player_id=? AND left_at=0", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}
