package service

import (
	"math"
	"reflect"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// RestoreActivityCalendarOpeningWindows joins a caller-owned clock/profile-locked
// transaction, after carries and issued consumers have been handled. Shared realm
// and template windows affect all roles, but only existing nonprotected anchors
// are changed. Missing anchors retain the normal fallback/initialization path.
// Any error requires the caller to roll back the entire restore.
func RestoreActivityCalendarOpeningWindows(tx *gorm.DB, realm db.AdminGameServer, anchors []db.PlayerOpeningCalendar, opening db.OpenServerActivityRule, openingAt int64, before, after FrozenJSONWindow) error {
	midnight := func(at int64) bool {
		return at > 0 && at <= constUtil.ActivityCalendarRestoreMaxMillis && (at+8*3600000)%86400000 == 0
	}
	validWindow := func(w FrozenJSONWindow) bool {
		return midnight(w.StartAt) && w.EndAt-w.StartAt == 14*86400000 && w.ClaimEndAt >= w.EndAt && w.ClaimEndAt-w.EndAt <= 90*86400000 && w.ClaimEndAt <= constUtil.ActivityCalendarRestoreMaxMillis
	}
	if !openingCarryTransaction(tx) || realm.ID <= 0 || realm.OpenAt <= 0 || realm.OpenAt > constUtil.ActivityCalendarRestoreMaxMillis || realm.Revision < 0 || realm.Revision == math.MaxInt64 || !midnight(openingAt) || !validWindow(before) || !validWindow(after) || after.StartAt != openingAt || before.ClaimEndAt-before.EndAt != after.ClaimEndAt-after.EndAt || opening.ID != constUtil.OpenServerActivityID || opening.UnlockStartAt != 0 || (FrozenJSONWindow{opening.StartAt, opening.EndAt, opening.RewardEndAt}) != before {
		return constUtil.ErrAdminConfiguration
	}
	expected := map[int64]db.PlayerOpeningCalendar{}
	for _, anchor := range anchors {
		if anchor.PlayerID <= 0 || !midnight(anchor.StartAt) || anchor.CreatedAt <= 0 || anchor.Source != constUtil.PlayerOpeningCalendarSource {
			return constUtil.ErrAdminConfiguration
		}
		if _, duplicate := expected[anchor.PlayerID]; duplicate {
			return constUtil.ErrAdminConfiguration
		}
		expected[anchor.PlayerID] = anchor
	}
	var currentRealm db.AdminGameServer
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", realm.ID).Take(&currentRealm).Error; err != nil {
		return err
	}
	if !reflect.DeepEqual(realm, currentRealm) {
		return constUtil.ErrAdminConfigConflict
	}
	// Follow AdminProfileServerID: legacy zero-server roles belong only to the
	// default realm. All profiles are read so an anchor cannot hide on a nil save.
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id")
	if realm.ID == constUtil.GameDefaultServerID {
		query = query.Where("server_id=? OR server_id=0", realm.ID)
	} else {
		query = query.Where("server_id=?", realm.ID)
	}
	var profiles []db.Profile
	if err := query.Find(&profiles).Error; err != nil {
		return err
	}
	owners := map[int64]db.Profile{}
	ids := make([]int64, 0, len(profiles))
	for _, owner := range profiles {
		if AdminProfileServerID(owner) != realm.ID {
			return constUtil.ErrAdminConfiguration
		}
		owners[owner.Id] = owner
		ids = append(ids, owner.Id)
	}
	var currentAnchors []db.PlayerOpeningCalendar
	if len(ids) > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id IN ?", ids).Order("player_id").Find(&currentAnchors).Error; err != nil {
			return err
		}
	}
	if len(currentAnchors) != len(expected) {
		return constUtil.ErrAdminConfigConflict
	}
	skip := map[int64]bool{}
	for _, anchor := range currentAnchors {
		old, ok := expected[anchor.PlayerID]
		if !ok || !reflect.DeepEqual(old, anchor) {
			return constUtil.ErrAdminConfigConflict
		}
		owner, ok := owners[anchor.PlayerID]
		if !ok {
			return constUtil.ErrAdminConfiguration
		}
		protected := owner.Id <= 1
		if owner.AccountID != 0 {
			account, err := AccountForProfile(tx.Clauses(clause.Locking{Strength: "SHARE"}), &owner)
			if err != nil {
				return err
			}
			protected = account.Protected
		}
		if protected {
			skip[anchor.PlayerID] = true
			continue
		}
		if adminActivitySchemaHasTable(tx, &db.LocalPlayActivityBinding{}) {
			var bindings []db.LocalPlayActivityBinding
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND activity_id=?", anchor.PlayerID, constUtil.OpenServerActivityID).Find(&bindings).Error; err != nil {
				return err
			}
			if len(bindings) > 0 {
				skip[anchor.PlayerID] = true
				continue
			}
		}
		if owner.Player == nil || owner.CreatedAt == nil || *owner.CreatedAt <= 0 {
			return constUtil.ErrAdminConfiguration
		}
	}
	var currentOpening db.OpenServerActivityRule
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", opening.ID).Take(&currentOpening).Error; err != nil {
		return err
	}
	if !reflect.DeepEqual(currentOpening, opening) {
		return constUtil.ErrAdminConfigConflict
	}
	// Complete validation precedes every write; historical tasks, counters,
	// receipts, anchor source/creation and profile birthdays are never written.
	if realm.OpenAt != openingAt {
		result := tx.Model(&db.AdminGameServer{}).Where("id=? AND open_at=? AND revision=?", realm.ID, realm.OpenAt, realm.Revision).Updates(map[string]any{"open_at": openingAt, "revision": realm.Revision + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
	}
	anchorIDs := make([]int64, 0, len(expected))
	for id := range expected {
		anchorIDs = append(anchorIDs, id)
	}
	sort.Slice(anchorIDs, func(i, j int) bool { return anchorIDs[i] < anchorIDs[j] })
	for _, id := range anchorIDs {
		if skip[id] {
			continue
		}
		old := expected[id]
		if old.StartAt == openingAt {
			continue
		}
		result := tx.Model(&db.PlayerOpeningCalendar{}).Where("player_id=? AND start_at=? AND created_at=? AND source=?", id, old.StartAt, old.CreatedAt, old.Source).Update("start_at", openingAt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
	}
	if before != after {
		result := tx.Model(&db.OpenServerActivityRule{}).Where("id=? AND start_at=? AND end_at=? AND reward_end_at=? AND enabled=? AND source=?", opening.ID, before.StartAt, before.EndAt, before.ClaimEndAt, opening.Enabled, opening.Source).Updates(map[string]any{"start_at": after.StartAt, "end_at": after.EndAt, "reward_end_at": after.ClaimEndAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return constUtil.ErrAdminConfigConflict
		}
	}
	return nil
}
