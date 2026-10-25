package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func p13P6MatchSources() []string {
	return []string{constUtil.P13UFPlayerBattleSource, constUtil.P13UnionFightBattleAuditP6Source, constUtil.P13UnionFightGuardAuditP6Source, constUtil.P13UFGuardBattleSource}
}

// Only the three known P13 producers can claim this indexed association. Every
// explicit match token must agree; an absent token never becomes a default.
func p13P6MatchFromInput(source string, input *model.FrozenBattleInput) (string, error) {
	known := false
	for _, s := range p13P6MatchSources() {
		known = known || s == source
	}
	if !known {
		return "", nil
	}
	if input == nil || input.BattleType != 14 {
		return "", errors.New("P13 P6 scope requires frozen type14")
	}
	match := ""
	for _, facts := range input.AppliedProperties {
		for _, fact := range facts {
			for _, part := range strings.Split(fact, ";") {
				if !strings.HasPrefix(part, "match=") {
					continue
				}
				value := strings.TrimPrefix(part, "match=")
				if value == "" || len(value) > 64 || strings.TrimSpace(value) != value {
					return "", errors.New("P13 P6 invalid match fact")
				}
				if match != "" && match != value {
					return "", errors.New("P13 P6 conflicting match facts")
				}
				match = value
			}
		}
	}
	if match == "" {
		return "", errors.New("P13 P6 missing frozen match fact")
	}
	return match, nil
}

// BackfillP13P6MatchScopeBatch is an explicit maintenance operation, never a
// request hook. The returned cursor covers only successfully verified rows,
// including on error; the failed row remains untouched and is retried next run.
func BackfillP13P6MatchScopeBatch(ctx context.Context, after string, limit int) (string, int, error) {
	if limit < 1 || limit > 8 {
		return after, 0, errors.New("P13 backfill batch must be 1..8")
	}
	var rows []db.P6BattleRecord
	tx := db.GetIns().WithContext(ctx)
	if e := tx.Select("id,player_id,source,source_fight,input,input_hash,p13_match_id,configuration_hash,engine_version,client_version,seed").Where("source IN ? AND p13_match_id='' AND id>?", p13P6MatchSources(), after).Order("id").Limit(limit).Find(&rows).Error; e != nil {
		return after, 0, e
	}
	cursor := after
	count := 0
	for _, row := range rows {
		if row.ID != p6Digest([]byte(fmt.Sprintf("%d/%s/%s", row.PlayerID, row.Source, row.SourceFight))) {
			return cursor, count, fmt.Errorf("P13 backfill row %s: identity SHA mismatch", row.ID)
		}
		raw, e := decodeP6StoredInput(row.Input)
		if e != nil {
			return cursor, count, fmt.Errorf("P13 backfill row %s: %w", row.ID, e)
		}
		if p6Digest(raw) != row.InputHash {
			return cursor, count, fmt.Errorf("P13 backfill row %s: input SHA mismatch", row.ID)
		}
		var input model.FrozenBattleInput
		// decodeP6Record additionally verifies the frozen table/input closure.
		inputPtr, e := decodeP6Record(&row)
		if e != nil {
			return cursor, count, fmt.Errorf("P13 backfill row %s: %w", row.ID, e)
		}
		input = *inputPtr
		match, e := p13P6MatchFromInput(row.Source, &input)
		if e != nil {
			return cursor, count, fmt.Errorf("P13 backfill row %s: %w", row.ID, e)
		}
		result := tx.Model(&db.P6BattleRecord{}).Where("id=? AND input_hash=? AND p13_match_id=''", row.ID, row.InputHash).Update("p13_match_id", match)
		if result.Error != nil {
			return cursor, count, result.Error
		}
		if result.RowsAffected != 1 {
			return cursor, count, errors.New("P13 backfill concurrent row change; retry checkpoint")
		}
		cursor = row.ID
		count++
	}
	return cursor, count, nil
}
