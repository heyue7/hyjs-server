package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// RestoreActivityCalendarPeriodWindow runs inside the caller's clock/profile lock
// transaction. Any error must abort that outer transaction; this function never commits.
// Payment carries must already have been prepared and saved for the same request.
func RestoreActivityCalendarPeriodWindow(tx *gorm.DB, id, requestID string, before, after FrozenJSONWindow) error {
	const maximumOwnershipRows = 10000
	fail := func(reason string) error { return fmt.Errorf("period %s calendar restore: %s", id, reason) }
	if tx == nil || id == "" || requestID == "" || len(requestID) > 128 {
		return fail("missing transaction or identity")
	}
	if tx.Statement == nil {
		return fail("missing transaction")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fail("caller transaction required")
	}
	var live db.PeriodInstance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&live).Error; err != nil {
		return err
	}
	if live.RankPlaces != 0 {
		return fail("ranked period reopening is unsupported")
	}
	var prior db.ActivityCalendarPeriodArchive
	archiveErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id=?", id).Take(&prior).Error
	if archiveErr == nil {
		if prior.Digest != activityCalendarPeriodArchiveDigest(prior) || prior.RequestID != requestID ||
			(FrozenJSONWindow{prior.BeforeStartAt, prior.BeforeEndAt, prior.BeforeClaimEndAt}) != before ||
			(FrozenJSONWindow{prior.AfterStartAt, prior.AfterEndAt, prior.AfterClaimEndAt}) != after ||
			(FrozenJSONWindow{live.StartAt, live.EndAt, live.RewardEndAt}) != after {
			return fail("settlement archive request conflict")
		}
		return nil
	}
	if !errors.Is(archiveErr, gorm.ErrRecordNotFound) {
		return archiveErr
	}
	if live.ID != id || (FrozenJSONWindow{StartAt: live.StartAt, EndAt: live.EndAt, ClaimEndAt: live.RewardEndAt}) != before {
		return fail("live window conflict")
	}
	var settlements []db.PeriodSettlement
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id=?", id).Find(&settlements).Error; err != nil {
		return err
	}
	var archive *db.ActivityCalendarPeriodArchive
	if len(settlements) > 0 {
		original := settlements[0]
		var settled p1Settlement
		if original.InstanceID != id || original.SettledAt <= 0 || json.Unmarshal(original.Snapshot, &settled) != nil || settled.Rules.ID != id || settled.Rules.RankPlaces != 0 || settled.Rules.Kind != live.Kind || settled.Rules.Mode != live.Mode || settled.Rules.RewardSourceID != live.RewardSourceID || (FrozenJSONWindow{settled.Rules.StartAt, settled.Rules.EndAt, settled.Rules.RewardEndAt}) != before {
			return fail("settlement identity or window conflict")
		}
		settlementPatch := FrozenJSONWindowPatch{StartPath: []string{"rules", "startAt"}, EndPath: []string{"rules", "endAt"}, ClaimEndPath: []string{"rules", "rewardEndAt"}, Identities: []FrozenJSONIdentity{{Path: []string{"rules", "id"}, Expected: id}}, Expected: before, Target: before, MinMillis: 1, MaxMillis: constUtil.ActivityCalendarRestoreMaxMillis}
		if _, _, err := PatchFrozenJSONWindow(original.Snapshot, settlementPatch); err != nil {
			return err
		}
		if !reflect.DeepEqual(p1PeriodSources(settled.Rules), p1PeriodSources(live)) {
			return fail("settlement reward sources conflict")
		}
		archive = &db.ActivityCalendarPeriodArchive{InstanceID: id, RequestID: requestID, SettledAt: original.SettledAt, Snapshot: append([]byte(nil), original.Snapshot...), BeforeStartAt: before.StartAt, BeforeEndAt: before.EndAt, BeforeClaimEndAt: before.ClaimEndAt, AfterStartAt: after.StartAt, AfterEndAt: after.EndAt, AfterClaimEndAt: after.ClaimEndAt}
		archive.Digest = activityCalendarPeriodArchiveDigest(*archive)
	}
	patch := FrozenJSONWindowPatch{StartPath: []string{"startAt"}, EndPath: []string{"endAt"}, ClaimEndPath: []string{"rewardEndAt"}, Identities: []FrozenJSONIdentity{{Path: []string{"id"}, Expected: id}}, Expected: before, Target: after, MinMillis: 1, MaxMillis: constUtil.ActivityCalendarRestoreMaxMillis}
	patchSnapshot := func(raw []byte) ([]byte, error) {
		var frozen db.PeriodInstance
		if err := json.Unmarshal(raw, &frozen); err != nil {
			return nil, err
		}
		if frozen.ID != id || frozen.Kind != live.Kind || frozen.Mode != live.Mode || frozen.RankPlaces != 0 || frozen.RewardSourceID != live.RewardSourceID || !reflect.DeepEqual(p1PeriodSources(frozen), p1PeriodSources(live)) {
			return nil, fail("frozen identity or reward sources conflict")
		}
		if (FrozenJSONWindow{StartAt: frozen.StartAt, EndAt: frozen.EndAt, ClaimEndAt: frozen.RewardEndAt}) != before {
			return nil, fail("frozen window conflict")
		}
		result, _, err := PatchFrozenJSONWindow(raw, patch)
		return result, err
	}
	// Validate the time contract even when there is no frozen publication yet.
	raw, err := json.Marshal(live)
	if err != nil {
		return err
	}
	if _, err = patchSnapshot(raw); err != nil {
		return err
	}
	frozen := live.Frozen
	if len(bytes.TrimSpace(frozen)) > 0 {
		frozen, err = patchSnapshot(frozen)
		if err != nil {
			return err
		}
	}
	var progress []db.PeriodProgress
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id=?", id).Order("player_id").Limit(maximumOwnershipRows + 1).Find(&progress).Error; err != nil {
		return err
	}
	if len(progress) > maximumOwnershipRows {
		return fail("progress ownership scan exceeds maintenance bound")
	}
	snapshots := make(map[int64][]byte, len(progress))
	owners := make(map[int64]bool, len(progress))
	ownerIDs := make([]int64, 0, len(progress))
	for _, saved := range progress {
		if saved.InstanceID != id || saved.PlayerID <= 0 {
			return fail("progress identity conflict")
		}
		snapshots[saved.PlayerID], err = patchSnapshot(saved.Snapshot)
		if err != nil {
			return err
		}
		owners[saved.PlayerID] = true
		ownerIDs = append(ownerIDs, saved.PlayerID)
		if live.Kind == "payment_amount" {
			var carry db.ActivityRechargeCarry
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id=? AND player_id=?", id, saved.PlayerID).Take(&carry).Error; err != nil {
				return fail("payment carry missing")
			}
			if carry.RequestID != requestID || carry.Mode != live.Mode || carry.OpeningAt != after.StartAt || carry.RealCutoff <= 0 || carry.Digest != activityRechargeCarryDigest(carry) {
				return fail("payment carry conflict")
			}
			var facts []ActivityRechargeCarryFact
			if json.Unmarshal(carry.Facts, &facts) != nil {
				return fail("payment carry facts invalid")
			}
			if _, _, err = activityRechargeCarryValue(carry, live, facts, nil, after.StartAt); err != nil {
				return err
			}
		}
	}
	sources := p1PeriodSources(live)
	if len(sources) == 0 || len(sources) > 31 {
		return fail("reward sources missing or exceed original tier bound")
	}
	var deliveries []db.RewardDelivery
	if len(ownerIDs) > 0 {
		// Independent character periods deliberately share a reward source. The
		// player/source unique key owns a delivery; another player's instance
		// provides no authority to inspect or rewrite that delivery's deadline.
		// Retained progress snapshots also reveal aliases whose live instance
		// has since been archived or removed. Do not infer ownership from dates.
		var otherProgress []db.PeriodProgress
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("instance_id", "player_id", "snapshot").Where("player_id IN ? AND instance_id<>?", ownerIDs, id).Order("instance_id,player_id").Limit(maximumOwnershipRows + 1).Find(&otherProgress).Error; err != nil {
			return err
		}
		if len(otherProgress) > maximumOwnershipRows {
			return fail("historical ownership scan exceeds maintenance bound")
		}
		targetSources := make(map[string]bool, len(sources))
		for _, source := range sources {
			targetSources[source] = true
		}
		for _, other := range otherProgress {
			var cfg db.PeriodInstance
			if !owners[other.PlayerID] || other.InstanceID == "" || json.Unmarshal(other.Snapshot, &cfg) != nil || cfg.ID != other.InstanceID {
				return fail("historical progress reward ownership cannot be proved")
			}
			aliases := p1PeriodSources(cfg)
			if len(aliases) == 0 || len(aliases) > 31 {
				return fail("historical reward sources cannot be proved within tier bound")
			}
			for _, source := range aliases {
				if targetSources[source] {
					return fail("same player has another period sharing reward source: " + other.InstanceID)
				}
			}
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id IN ? AND source_key IN ?", ownerIDs, sources).Order("player_id,id").Limit(maximumOwnershipRows + 1).Find(&deliveries).Error; err != nil {
			return err
		}
		if len(deliveries) > maximumOwnershipRows {
			return fail("delivery ownership scan exceeds maintenance bound")
		}
	}
	byID := make(map[int64]db.RewardDelivery, len(deliveries))
	for _, delivery := range deliveries {
		if !owners[delivery.PlayerID] {
			return fail("delivery has no matching progress")
		}
		byID[delivery.ID] = delivery
		if delivery.ClaimedAt == 0 && delivery.ExpiresAt != before.ClaimEndAt {
			return fail("unclaimed delivery deadline conflict")
		}
	}
	for _, saved := range progress {
		if saved.RewardID == 0 {
			continue
		}
		delivery, ok := byID[saved.RewardID]
		if !ok || delivery.PlayerID != saved.PlayerID || delivery.SourceKey != sources[0] {
			return fail("progress reward identity conflict")
		}
	}
	// All dependencies are validated before the first write. Limited column updates
	// retain rules, progress, receipts and the byte payload of every claimed delivery.
	if archive != nil {
		if err = tx.Create(archive).Error; err != nil {
			return err
		}
		var saved db.ActivityCalendarPeriodArchive
		if err = tx.Where("instance_id=?", id).Take(&saved).Error; err != nil {
			return err
		}
		if saved.Digest != archive.Digest || saved.Digest != activityCalendarPeriodArchiveDigest(saved) || !bytes.Equal(saved.Snapshot, archive.Snapshot) || saved.SettledAt != archive.SettledAt {
			return fail("settlement archive verification failed")
		}
	}
	updates := map[string]any{"start_at": after.StartAt, "end_at": after.EndAt, "reward_end_at": after.ClaimEndAt}
	if !bytes.Equal(frozen, live.Frozen) {
		updates["frozen"] = frozen
	}
	if before != after || !bytes.Equal(frozen, live.Frozen) {
		if err = tx.Model(&db.PeriodInstance{}).Where("id=?", id).Updates(updates).Error; err != nil {
			return err
		}
	}
	for _, saved := range progress {
		if bytes.Equal(saved.Snapshot, snapshots[saved.PlayerID]) {
			continue
		}
		if err = tx.Model(&db.PeriodProgress{}).Where("instance_id=? AND player_id=?", id, saved.PlayerID).Update("snapshot", snapshots[saved.PlayerID]).Error; err != nil {
			return err
		}
	}
	for _, delivery := range deliveries {
		if delivery.ClaimedAt != 0 || delivery.ExpiresAt == after.ClaimEndAt {
			continue
		}
		result := tx.Model(&db.RewardDelivery{}).Where("id=? AND player_id=? AND source_key=? AND claimed_at=0 AND expires_at=?", delivery.ID, delivery.PlayerID, delivery.SourceKey, before.ClaimEndAt).Update("expires_at", after.ClaimEndAt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fail("delivery ownership or deadline changed")
		}
	}
	if archive != nil {
		result := tx.Where("instance_id=? AND settled_at=?", id, archive.SettledAt).Delete(&db.PeriodSettlement{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fail("settlement delete conflict")
		}
	}
	return nil
}

func activityCalendarPeriodArchiveDigest(row db.ActivityCalendarPeriodArchive) string {
	row.Digest = ""
	return p6Digest(adminJSON(row))
}
