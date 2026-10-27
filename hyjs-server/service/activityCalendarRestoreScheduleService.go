package service

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// RestoreActivityCalendarScheduleWindows joins the caller's locked restore
// transaction. Plan windows supply column preimages/targets; complete snapshots
// additionally protect fields omitted by adminVersion, notably FormJSON.
// Issued SourceVersion remains the historical acceptance source. The outer
// request audit owns migration provenance, clock/anchor/native consumers and retry.
func RestoreActivityCalendarScheduleWindows(tx *gorm.DB, plan *ActivityCalendarRestorePlan, sources []db.AdminActivityOccurrence, issued []db.AdminCharacterActivityInstance) error {
	if !openingCarryTransaction(tx) || plan == nil || plan.ServerID <= 0 {
		return constUtil.ErrAdminConfiguration
	}
	type issuedKey struct {
		player     int64
		occurrence string
	}
	sourceBefore := map[string]db.AdminActivityOccurrence{}
	issuedBefore := map[issuedKey]db.AdminCharacterActivityInstance{}
	for _, row := range sources {
		if row.ID == "" || row.Mode == "" || row.ConsumerKey == "" || row.ConsumerID == "" || row.ServerID != plan.ServerID || !row.Enabled || row.Archived {
			return constUtil.ErrAdminConfiguration
		}
		if _, ok := sourceBefore[row.ID]; ok {
			return constUtil.ErrAdminConfiguration
		}
		sourceBefore[row.ID] = row
	}
	for _, row := range issued {
		key := issuedKey{row.PlayerID, row.OccurrenceID}
		if row.PlayerID <= 0 || row.ServerID != plan.ServerID || row.ConsumerKey == "" || row.ConsumerID == "" || row.SourceVersion == "" {
			return constUtil.ErrAdminConfiguration
		}
		if _, ok := issuedBefore[key]; ok {
			return constUtil.ErrAdminConfiguration
		}
		issuedBefore[key] = row
	}
	sourceWindows := map[string]ActivityCalendarRestoreWindow{}
	issuedWindows := map[issuedKey]ActivityCalendarRestoreWindow{}
	for _, window := range plan.Windows {
		if window.PlayerID == 0 {
			row, ok := sourceBefore[window.OccurrenceID]
			if !ok || window.ConsumerID != row.ConsumerID || window.Mode != row.Mode || window.SourceVersion != adminVersion(row) || window.Before != [3]int64{row.StartAt, row.EndAt, row.ClaimEndAt} {
				return constUtil.ErrAdminConfigConflict
			}
			if _, exists := sourceWindows[row.ID]; exists {
				return constUtil.ErrAdminConfiguration
			}
			sourceWindows[row.ID] = window
		} else {
			key := issuedKey{window.PlayerID, window.OccurrenceID}
			row, ok := issuedBefore[key]
			if !ok || window.ConsumerID != row.ConsumerID || window.Mode != row.Mode || window.SourceVersion != row.SourceVersion || window.Before != [3]int64{row.StartAt, row.EndAt, row.ClaimEndAt} {
				return constUtil.ErrAdminConfigConflict
			}
			if _, exists := issuedWindows[key]; exists {
				return constUtil.ErrAdminConfiguration
			}
			issuedWindows[key] = window
		}
	}
	if len(sourceWindows) != len(sourceBefore) || len(issuedWindows) != len(issuedBefore) {
		return constUtil.ErrAdminConfiguration
	}
	ids := make([]string, 0, len(sourceBefore))
	for id := range sourceBefore {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	updatedSources := map[string]db.AdminActivityOccurrence{}
	for _, id := range ids {
		before := sourceBefore[id]
		window := sourceWindows[id]
		var current db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&current).Error; err != nil {
			return err
		}
		if !reflect.DeepEqual(current, before) {
			return constUtil.ErrAdminConfigConflict
		}
		oldJSON, err := activityScheduleJSONWindow(before.FormJSON, before.Mode)
		if err != nil {
			return err
		}
		after := FrozenJSONWindow{window.After[0], window.After[1], window.After[2]}
		raw, err := activitySchedulePatchWindow(before.FormJSON, before.Mode, oldJSON, after)
		if err != nil {
			return err
		}
		updated := before
		updated.StartAt, updated.EndAt, updated.ClaimEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
		updated.FormJSON = raw
		if adminActivityTiming(before) == "character-day" {
			if window.StartDay < 1 || window.StartDay > 366 || window.DurationDays < 1 || window.DurationDays > 366 || window.ClaimDays < 0 || window.ClaimDays > 90 || after.EndAt-after.StartAt != window.DurationDays*86400000 || after.ClaimEndAt-after.EndAt != window.ClaimDays*86400000 {
				return constUtil.ErrAdminConfiguration
			}
			updated.StartDay, updated.DurationDays, updated.ClaimDays = window.StartDay, window.DurationDays, window.ClaimDays
		}
		updatedSources[id] = updated
	}
	// Lock the complete issued set for these sources; a newly accepted role must
	// invalidate the complete preimage rather than retain an unmigrated window.
	var currentIssued []db.AdminCharacterActivityInstance
	if len(ids) > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND occurrence_id IN ?", plan.ServerID, ids).Order("player_id,occurrence_id").Find(&currentIssued).Error; err != nil {
			return err
		}
	}
	if len(currentIssued) != len(issuedBefore) {
		return constUtil.ErrAdminConfigConflict
	}
	updatedIssued := make([]db.AdminCharacterActivityInstance, 0, len(currentIssued))
	for _, current := range currentIssued {
		key := issuedKey{current.PlayerID, current.OccurrenceID}
		before, ok := issuedBefore[key]
		if !ok || !reflect.DeepEqual(before, current) {
			return constUtil.ErrAdminConfigConflict
		}
		source, ok := sourceBefore[before.OccurrenceID]
		if !ok || source.Mode != before.Mode || adminActivityTiming(source) != "character-day" {
			return constUtil.ErrAdminConfiguration
		}
		window := issuedWindows[key]
		sourceWindow := sourceWindows[source.ID]
		if window.After != sourceWindow.After {
			return constUtil.ErrAdminConfiguration
		}
		oldJSON, err := activityScheduleJSONWindow(before.FormJSON, before.Mode)
		if err != nil {
			return err
		}
		if before.Mode != "opening-14" && oldJSON != (FrozenJSONWindow{before.StartAt, before.EndAt, before.ClaimEndAt}) {
			return constUtil.ErrAdminConfigConflict
		}
		after := FrozenJSONWindow{window.After[0], window.After[1], window.After[2]}
		jsonAfter := after
		if before.Mode == "opening-14" {
			jsonAfter, err = activityScheduleJSONWindow(updatedSources[source.ID].FormJSON, before.Mode)
			if err != nil {
				return err
			}
		}
		raw, err := activitySchedulePatchWindow(before.FormJSON, before.Mode, oldJSON, jsonAfter)
		if err != nil {
			return err
		}
		updated := before
		updated.StartAt, updated.EndAt, updated.ClaimEndAt = after.StartAt, after.EndAt, after.ClaimEndAt
		updated.FormJSON = raw
		updatedIssued = append(updatedIssued, updated)
	}
	// Validation of every preimage precedes all writes. Callers must roll back
	// if a later native migration or operation receipt fails.
	for _, id := range ids {
		before, after := sourceBefore[id], updatedSources[id]
		if reflect.DeepEqual(before, after) {
			continue
		}
		if before.Revision == math.MaxInt64 {
			return constUtil.ErrAdminConfiguration
		}
		result := tx.Model(&db.AdminActivityOccurrence{}).Where("id=? AND server_id=? AND revision=? AND form_json=? AND start_at=? AND end_at=? AND claim_end_at=?", id, plan.ServerID, before.Revision, before.FormJSON, before.StartAt, before.EndAt, before.ClaimEndAt).Updates(map[string]any{"start_at": after.StartAt, "end_at": after.EndAt, "claim_end_at": after.ClaimEndAt, "start_day": after.StartDay, "duration_days": after.DurationDays, "claim_days": after.ClaimDays, "form_json": after.FormJSON, "revision": before.Revision + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
	}
	for _, after := range updatedIssued {
		before := issuedBefore[issuedKey{after.PlayerID, after.OccurrenceID}]
		if reflect.DeepEqual(before, after) {
			continue
		}
		result := tx.Model(&db.AdminCharacterActivityInstance{}).Where("player_id=? AND occurrence_id=? AND consumer_id=? AND consumer_key=? AND source_version=? AND form_json=? AND start_at=? AND end_at=? AND claim_end_at=?", before.PlayerID, before.OccurrenceID, before.ConsumerID, before.ConsumerKey, before.SourceVersion, before.FormJSON, before.StartAt, before.EndAt, before.ClaimEndAt).Updates(map[string]any{"start_at": after.StartAt, "end_at": after.EndAt, "claim_end_at": after.ClaimEndAt, "form_json": after.FormJSON})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
	}
	return nil
}

func activityScheduleJSONWindow(raw []byte, mode string) (FrozenJSONWindow, error) {
	var fields map[string]json.RawMessage
	if openingCarryJSON(raw, &fields) != nil || fields == nil {
		return FrozenJSONWindow{}, constUtil.ErrAdminConfiguration
	}
	var actualMode string
	if json.Unmarshal(fields["mode"], &actualMode) != nil || actualMode != mode {
		return FrozenJSONWindow{}, constUtil.ErrAdminConfiguration
	}
	var window FrozenJSONWindow
	for key, target := range map[string]*int64{"startAt": &window.StartAt, "endAt": &window.EndAt, "claimEndAt": &window.ClaimEndAt} {
		if json.Unmarshal(fields[key], target) != nil {
			return FrozenJSONWindow{}, constUtil.ErrAdminConfiguration
		}
	}
	return window, nil
}

func activitySchedulePatchWindow(raw []byte, mode string, before, after FrozenJSONWindow) ([]byte, error) {
	result, _, err := PatchFrozenJSONWindow(raw, FrozenJSONWindowPatch{StartPath: []string{"startAt"}, EndPath: []string{"endAt"}, ClaimEndPath: []string{"claimEndAt"}, Identities: []FrozenJSONIdentity{{Path: []string{"mode"}, Expected: mode}}, Expected: before, Target: after, MinMillis: 1, MaxMillis: constUtil.ActivityCalendarRestoreMaxMillis})
	if err != nil {
		return nil, err
	}
	if bytes.Equal(raw, result) {
		return raw, nil
	}
	// Keep nested frozen payloads as RawMessage rather than regenerating their
	// objects from the validator's generic tree. Only the three root values change.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	fields["startAt"], fields["endAt"], fields["claimEndAt"] = adminJSON(after.StartAt), adminJSON(after.EndAt), adminJSON(after.ClaimEndAt)
	return json.Marshal(fields)
}
