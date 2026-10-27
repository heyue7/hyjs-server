package service

import (
	"encoding/json"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// RestoreActivityCalendarKingRecruitWindow joins the caller's clock/profile-
// locked restore transaction. Any error must abort that whole transaction.
// Snapshot Frozen and every non-calendar field retain their original values;
// this function never binds rules, creates snapshots or replays a recruit.
func (s *GameService) RestoreActivityCalendarKingRecruitWindow(tx *gorm.DB, poolID string, before, after FrozenJSONWindow) error {
	valid := func(w FrozenJSONWindow) bool {
		return w.StartAt > 0 && w.EndAt > w.StartAt && w.ClaimEndAt >= w.EndAt && w.ClaimEndAt <= constUtil.ActivityCalendarRestoreMaxMillis
	}
	if s == nil || !openingCarryTransaction(tx) || !adminKingRecruitPoolID(poolID) || len(poolID) > 64 || !valid(before) || !valid(after) {
		return constUtil.ErrAdminConfiguration
	}
	var pool db.GameplayPool
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", poolID).Take(&pool).Error; err != nil {
		return err
	}
	if (FrozenJSONWindow{pool.StartAt, pool.EndAt, pool.RewardEndAt}) != before {
		return constUtil.ErrAdminConfigConflict
	}
	var rewards []db.GameplayPoolReward
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Order("id").Limit(1001).Find(&rewards).Error; err != nil {
		return err
	}
	var snapshot db.KingRecruitActivitySnapshot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Take(&snapshot).Error; err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if openingCarryJSON(snapshot.Payload, &fields) != nil || fields == nil {
		return constUtil.ErrAdminConfiguration
	}
	var source adminKingRecruitSource
	if json.Unmarshal(snapshot.Payload, &source) != nil || source.Pool.ID != poolID || (FrozenJSONWindow{source.Pool.StartAt, source.Pool.EndAt, source.Pool.RewardEndAt}) != before {
		return constUtil.ErrAdminConfigConflict
	}
	if source.World < 0 || source.World > 3 {
		return constUtil.ErrAdminConfiguration
	}
	if raw, present := fields["world"]; present {
		var world int64
		if string(raw) == "null" || json.Unmarshal(raw, &world) != nil || world != source.World {
			return constUtil.ErrAdminConfiguration
		}
	}
	if err := validateKingRecruitActivityRules(source.Rules); err != nil {
		return err
	}
	if err := s.validateKingRecruitActivityPool(pool, rewards, source.World); err != nil {
		return err
	}
	if err := s.validateKingRecruitActivityPool(source.Pool, rewards, source.World); err != nil {
		return err
	}
	// Validate exact nested paths and duplicate keys using the shared lossless
	// reader. Preserve unrelated nested objects as raw JSON in the actual patch.
	if _, _, err := PatchFrozenJSONWindow(snapshot.Payload, FrozenJSONWindowPatch{StartPath: []string{"pool", "startAt"}, EndPath: []string{"pool", "endAt"}, ClaimEndPath: []string{"pool", "rewardEndAt"}, Identities: []FrozenJSONIdentity{{Path: []string{"pool", "id"}, Expected: poolID}}, Expected: before, Target: after, MinMillis: 1, MaxMillis: constUtil.ActivityCalendarRestoreMaxMillis}); err != nil {
		return err
	}
	if before == after {
		return nil
	}
	var poolFields map[string]json.RawMessage
	if json.Unmarshal(fields["pool"], &poolFields) != nil || poolFields == nil {
		return constUtil.ErrAdminConfiguration
	}
	poolFields["startAt"], poolFields["endAt"], poolFields["rewardEndAt"] = adminJSON(after.StartAt), adminJSON(after.EndAt), adminJSON(after.ClaimEndAt)
	rawPool, err := json.Marshal(poolFields)
	if err != nil {
		return err
	}
	fields["pool"] = rawPool
	rawSnapshot, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	result := tx.Model(&db.GameplayPool{}).Where("id=? AND start_at=? AND end_at=? AND reward_end_at=?", poolID, before.StartAt, before.EndAt, before.ClaimEndAt).Updates(map[string]any{"start_at": after.StartAt, "end_at": after.EndAt, "reward_end_at": after.ClaimEndAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return constUtil.ErrAdminConfigConflict
	}
	result = tx.Model(&db.KingRecruitActivitySnapshot{}).Where("pool_id=? AND payload=? AND frozen=?", poolID, snapshot.Payload, snapshot.Frozen).Update("payload", rawSnapshot)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return constUtil.ErrAdminConfigConflict
	}
	return nil
}
