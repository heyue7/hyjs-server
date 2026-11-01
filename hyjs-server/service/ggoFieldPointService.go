package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

type ggoPointPayload struct {
	Resource *ggoResourceNode `json:"resource,omitempty"`
	Monster  *ggoMonsterNode  `json:"monster,omitempty"`
	Legacy   *ggoLegacyBlock  `json:"legacy,omitempty"`
}

func ggoPointPayloads(state *ggoLineState) (map[string]ggoPointPayload, error) {
	result := make(map[string]ggoPointPayload)
	for i := range state.Resources {
		node := &state.Resources[i]
		value := result[node.PointID]
		if value.Resource != nil || value.Monster != nil {
			return nil, errors.New("沙漠点位重复")
		}
		value.Resource = node
		result[node.PointID] = value
	}
	for i := range state.Monsters {
		node := &state.Monsters[i]
		value := result[node.PointID]
		if value.Resource != nil || value.Monster != nil {
			return nil, errors.New("沙漠点位重复")
		}
		value.Monster = node
		result[node.PointID] = value
	}
	for i := range state.LegacyBlocks {
		block := &state.LegacyBlocks[i]
		value := result[block.PointID]
		if value.Legacy != nil {
			return nil, errors.New("沙漠旧版占用重复")
		}
		value.Legacy = block
		result[block.PointID] = value
	}
	return result, nil
}

func ggoPointDue(value ggoPointPayload, cycle int64) int64 {
	state := &ggoLineState{CycleMS: cycle}
	if value.Resource != nil {
		state.Resources = []ggoResourceNode{*value.Resource}
	}
	if value.Monster != nil {
		state.Monsters = []ggoMonsterNode{*value.Monster}
	}
	return ggoNextLineDue(state)
}

// The legacy payload remains a migration/recovery source. Only this bounded
// publication transaction replaces point rows; normal actions never write it.
func ggoInstallPointState(tx *gorm.DB, row *db.GGOFieldLine, state *ggoLineState) error {
	payloads, err := ggoPointPayloads(state)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(payloads))
	for id := range payloads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	points := make([]db.GGOFieldPoint, 0, len(ids))
	for _, id := range ids {
		value := payloads[id]
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		point := db.GGOFieldPoint{ServerID: row.ServerID, Line: row.Line, PointID: id, Payload: raw, NextDueAt: ggoPointDue(value, state.CycleMS)}
		if value.Monster != nil {
			point.MonsterUUID = value.Monster.UUID
		}
		points = append(points, point)
	}
	if err := tx.Where("server_id=? AND line=?", row.ServerID, row.Line).Delete(&db.GGOFieldPoint{}).Error; err != nil {
		return err
	}
	if len(points) > 0 {
		if err := tx.CreateInBatches(&points, 100).Error; err != nil {
			return err
		}
	}
	row.PointStateReady, row.DeploymentRevision, row.CycleMS = true, state.DeploymentRevision, state.CycleMS
	row.GenerationFloor = state.NextGeneration
	return tx.Model(&db.GGOFieldLine{}).Where("server_id=? AND line=?", row.ServerID, row.Line).
		Updates(map[string]any{"point_state_ready": true, "deployment_revision": row.DeploymentRevision, "cycle_ms": row.CycleMS, "generation_floor": row.GenerationFloor, "next_due_at": int64(math.MaxInt64)}).Error
}

func migrateGGOFieldPoints(d *gorm.DB, serverID int64) error {
	for {
		var keys []db.GGOFieldLine
		if err := d.Select("server_id", "line").Where("server_id=? AND point_state_ready=?", serverID, false).Order("line").Limit(32).Find(&keys).Error; err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, key := range keys {
			if err := d.Transaction(func(tx *gorm.DB) error {
				var row db.GGOFieldLine
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=? AND line=?", key.ServerID, key.Line).Error; err != nil {
					return err
				}
				if row.PointStateReady {
					return nil
				}
				var state ggoLineState
				if json.Unmarshal(row.Payload, &state) != nil || row.Revision < 1 || state.NextGeneration < 1 {
					return errors.New("沙漠线路迁移来源无效")
				}
				return ggoInstallPointState(tx, &row, &state)
			}); err != nil {
				return err
			}
		}
	}
}

// RestoreGGOFieldLinePayloads materializes current points for an older binary.
// Run this maintenance command only while every application writer is stopped.
// Each line commits independently, so interruption can be resumed without loss.
func RestoreGGOFieldLinePayloads(ctx context.Context, serverID int64) (int, error) {
	database := db.GetIns().WithContext(ctx)
	restored := 0
	for {
		var keys []db.GGOFieldLine
		if err := database.Select("server_id", "line").Where("server_id=? AND point_state_ready=?", serverID, true).Order("line").Limit(32).Find(&keys).Error; err != nil {
			return restored, err
		}
		if len(keys) == 0 {
			return restored, nil
		}
		for _, key := range keys {
			if err := database.Transaction(func(tx *gorm.DB) error {
				var row db.GGOFieldLine
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=? AND line=?", key.ServerID, key.Line).Error; err != nil {
					return err
				}
				if !row.PointStateReady {
					return nil
				}
				state, err := ggoReadPointState(tx, &row, nil, true)
				if err != nil {
					return err
				}
				if err := ggoPointPublicRevision(&row, state); err != nil {
					return err
				}
				raw, err := json.Marshal(state)
				if err != nil {
					return err
				}
				return tx.Model(&db.GGOFieldLine{}).Where("server_id=? AND line=?", row.ServerID, row.Line).
					Updates(map[string]any{"payload": raw, "revision": row.Revision, "next_due_at": ggoNextLineDue(state), "point_state_ready": false}).Error
			}); err != nil {
				return restored, err
			}
			restored++
		}
	}
}

var ggoPointLineColumns = []string{"server_id", "line", "revision", "updated_at", "next_due_at", "point_state_ready", "deployment_revision", "cycle_ms", "generation_floor"}

func ggoReadPointState(tx *gorm.DB, row *db.GGOFieldLine, pointIDs []string, writable bool) (*ggoLineState, error) {
	var points []db.GGOFieldPoint
	query := tx.Where("server_id=? AND line=?", row.ServerID, row.Line)
	if len(pointIDs) > 0 {
		query = query.Where("point_id IN ?", pointIDs)
	}
	if writable {
		lock := clause.Locking{Strength: "UPDATE"}
		if tx.Dialector.Name() == "mysql" && tx.Statement.Context.Value(constUtil.GGOFieldPointSkipLockedContextKey{}) == true {
			lock.Options = "SKIP LOCKED"
		}
		query = query.Clauses(lock)
	}
	if err := query.Order("point_id").Find(&points).Error; err != nil {
		return nil, err
	}
	state := &ggoLineState{DeploymentRevision: row.DeploymentRevision, NextGeneration: max(1, row.GenerationFloor), CycleMS: row.CycleMS, Resources: []ggoResourceNode{}, Monsters: []ggoMonsterNode{}, pointRows: make(map[string]db.GGOFieldPoint), pointReadOnly: !writable}
	for _, point := range points {
		var value ggoPointPayload
		if json.Unmarshal(point.Payload, &value) != nil {
			return nil, errors.New("沙漠点位状态损坏")
		}
		state.pointRows[point.PointID] = point
		if value.Resource != nil {
			if value.Resource.PointID != point.PointID {
				return nil, errors.New("沙漠点位归属不符")
			}
			state.Resources = append(state.Resources, *value.Resource)
			state.NextGeneration = max(state.NextGeneration, value.Resource.Generation+1)
		}
		if value.Monster != nil {
			if value.Monster.PointID != point.PointID || value.Monster.UUID != point.MonsterUUID {
				return nil, errors.New("沙漠点位归属不符")
			}
			state.Monsters = append(state.Monsters, *value.Monster)
			state.NextGeneration = max(state.NextGeneration, value.Monster.Generation+1)
		}
		if value.Legacy != nil {
			if value.Legacy.PointID != point.PointID {
				return nil, errors.New("沙漠旧版占用归属不符")
			}
			state.LegacyBlocks = append(state.LegacyBlocks, *value.Legacy)
			state.NextGeneration = max(state.NextGeneration, value.Legacy.Generation+1)
		}
	}
	return state, nil
}

func ggoPointPublicRevision(row *db.GGOFieldLine, state *ggoLineState) error {
	var increments int64
	for _, point := range state.pointRows {
		if point.Revision < 0 || point.Revision > math.MaxInt64-increments {
			return errors.New("沙漠场景版本溢出")
		}
		increments += point.Revision
	}
	if increments < 0 || row.Revision > math.MaxInt64-increments {
		return errors.New("沙漠场景版本溢出")
	}
	row.Revision += increments
	return nil
}

func ggoSavePointState(tx *gorm.DB, row *db.GGOFieldLine, state *ggoLineState, now int64) error {
	if state.pointReadOnly {
		return errors.New("沙漠只读场景不能改写点位")
	}
	payloads, err := ggoPointPayloads(state)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(state.pointRows))
	for id := range state.pointRows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		point := state.pointRows[id]
		value := payloads[id]
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if bytes.Equal(raw, point.Payload) {
			continue
		}
		if point.Revision == math.MaxInt64 {
			return errors.New("沙漠点位版本溢出")
		}
		point.Revision++
		point.Payload, point.NextDueAt = raw, ggoPointDue(value, state.CycleMS)
		point.MonsterUUID = ""
		if value.Monster != nil {
			point.MonsterUUID = value.Monster.UUID
		}
		result := tx.Model(&db.GGOFieldPoint{}).Where("server_id=? AND line=? AND point_id=?", row.ServerID, row.Line, id).
			Updates(map[string]any{"revision": point.Revision, "payload": point.Payload, "next_due_at": point.NextDueAt, "monster_uuid": point.MonsterUUID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("沙漠点位写入失败")
		}
		state.pointRows[id] = point
	}
	row.UpdatedAt = now
	return nil
}

// Selected points are locked by (realm, line, point). The line share protects
// only publication, while unrelated resources and monsters remain writable.
func ggoLoadLine(tx *gorm.DB, serverID, line int64, cfg *model.AdminFieldGGOConfiguration, revision string, now int64, pointIDs ...string) (*db.GGOFieldLine, *ggoLineState, bool, error) {
	return ggoLoadPointLine(tx, serverID, line, cfg, revision, now, pointIDs, true)
}

func ggoReadLineSnapshot(tx *gorm.DB, serverID, line int64, cfg *model.AdminFieldGGOConfiguration, revision string, now int64) (*db.GGOFieldLine, *ggoLineState, bool, error) {
	return ggoLoadPointLine(tx, serverID, line, cfg, revision, now, nil, false)
}

// An empty write selection is reserved for full publication/maintenance work.
// Player mutations always pass their point; scene queries use the read entry.
func ggoLoadPointLine(tx *gorm.DB, serverID, line int64, cfg *model.AdminFieldGGOConfiguration, revision string, now int64, pointIDs []string, writable bool) (*db.GGOFieldLine, *ggoLineState, bool, error) {
	published := false
	var observed db.GGOFieldLine
	err := tx.Select(ggoPointLineColumns).First(&observed, "server_id=? AND line=?", serverID, line).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, err
	}
	strength := "SHARE"
	if err != nil || !observed.PointStateReady || observed.DeploymentRevision != revision || observed.CycleMS == 0 {
		strength = "UPDATE"
	}
	if strength == "UPDATE" && observed.PointStateReady && observed.DeploymentRevision != revision {
		if runtime, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && runtime.GGOIntegrationReady {
			var publication db.AdminFieldDeployment
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&publication, "server_id=? AND scene=?", serverID, constUtil.GGOFieldScene).Error; err != nil {
				return nil, nil, false, err
			}
			if !publication.Enabled || adminVersion(publication) != revision {
				return nil, nil, false, constUtil.ErrConfigurationReadRetry
			}
		}
	}
	var row db.GGOFieldLine
	if errors.Is(err, gorm.ErrRecordNotFound) {
		legacy, state, _, err := ggoLoadLegacyLine(tx, serverID, line, cfg, revision, now)
		if err != nil {
			return nil, nil, false, err
		}
		if err := ggoInstallPointState(tx, legacy, state); err != nil {
			return nil, nil, false, err
		}
		row = *legacy
		published = true
	} else {
		if err := tx.Select(ggoPointLineColumns).Clauses(clause.Locking{Strength: strength}).First(&row, "server_id=? AND line=?", serverID, line).Error; err != nil {
			return nil, nil, false, err
		}
		if !row.PointStateReady || row.DeploymentRevision != revision || row.CycleMS == 0 {
			if strength != "UPDATE" {
				return nil, nil, false, constUtil.ErrConfigurationReadRetry
			}
			// A concurrent migration/publication may have replaced the legacy
			// state after our probe. Retry before changing its new revision.
			if row.PointStateReady && !observed.PointStateReady && row.DeploymentRevision != revision {
				return nil, nil, false, constUtil.ErrConfigurationReadRetry
			}
			if row.PointStateReady {
				current, err := ggoReadPointState(tx, &row, nil, true)
				if err != nil {
					return nil, nil, false, err
				}
				if err := ggoPointPublicRevision(&row, current); err != nil {
					return nil, nil, false, err
				}
				raw, err := json.Marshal(current)
				if err != nil {
					return nil, nil, false, err
				}
				if err := tx.Model(&db.GGOFieldLine{}).Where("server_id=? AND line=?", serverID, line).Updates(map[string]any{"payload": raw, "revision": row.Revision}).Error; err != nil {
					return nil, nil, false, err
				}
			}
			legacy, state, _, err := ggoLoadLegacyLine(tx, serverID, line, cfg, revision, now)
			if err != nil {
				return nil, nil, false, err
			}
			if err := ggoInstallPointState(tx, legacy, state); err != nil {
				return nil, nil, false, err
			}
			row = *legacy
			published = true
		}
	}
	state, err := ggoReadPointState(tx, &row, pointIDs, writable)
	if err != nil {
		return nil, nil, false, err
	}
	changed := false
	if writable {
		if err := ggoValidatePointState(state, cfg, len(pointIDs) == 0); err != nil {
			return nil, nil, false, err
		}
		changed, err = ggoRefreshPointState(tx, &row, state, cfg, now)
		if err != nil {
			return nil, nil, false, err
		}
	} else if err := ggoValidateLineState(state, cfg); err != nil {
		return nil, nil, false, err
	}
	if len(pointIDs) == 0 {
		if err := ggoPointPublicRevision(&row, state); err != nil {
			return nil, nil, false, err
		}
	}
	return &row, state, changed || published, nil
}

// Frozen reservations remain consumable after a publication or feature close.
func ggoLoadReservedPoint(tx *gorm.DB, serverID, line int64, pointID string) (*db.GGOFieldLine, *ggoLineState, error) {
	var before db.GGOFieldLine
	if err := tx.Select(ggoPointLineColumns).First(&before, "server_id=? AND line=?", serverID, line).Error; err != nil {
		return nil, nil, err
	}
	strength := "SHARE"
	if !before.PointStateReady {
		strength = "UPDATE"
	}
	if runtime, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && runtime.GGOFieldConfig != nil && runtime.GGOFieldRevision != "" && runtime.GGOFieldRevision != before.DeploymentRevision {
		strength = "UPDATE"
	}
	var row db.GGOFieldLine
	if err := tx.Select(ggoPointLineColumns).Clauses(clause.Locking{Strength: strength}).First(&row, "server_id=? AND line=?", serverID, line).Error; err != nil {
		return nil, nil, err
	}
	if !row.PointStateReady {
		if strength != "UPDATE" {
			return nil, nil, constUtil.ErrConfigurationReadRetry
		}
		var original db.GGOFieldLine
		if err := tx.First(&original, "server_id=? AND line=?", serverID, line).Error; err != nil {
			return nil, nil, err
		}
		var initial ggoLineState
		if json.Unmarshal(original.Payload, &initial) != nil || original.Revision < 1 || initial.NextGeneration < 1 {
			return nil, nil, errors.New("沙漠线路迁移来源无效")
		}
		if err := ggoInstallPointState(tx, &original, &initial); err != nil {
			return nil, nil, err
		}
		row = original
	}
	state, err := ggoReadPointState(tx, &row, []string{pointID}, true)
	return &row, state, err
}

func ggoRefreshPointState(tx *gorm.DB, row *db.GGOFieldLine, state *ggoLineState, cfg *model.AdminFieldGGOConfiguration, now int64) (bool, error) {
	boundary := ggoCycleBoundary(now, cfg.Runtime.Resource.CycleSeconds)
	changed := false
	for i := range state.Resources {
		node := &state.Resources[i]
		if node.ReservedBy != 0 {
			continue
		}
		if node.ScheduledAtMS < boundary || node.Remaining == 0 && node.RefreshAtMS > 0 && node.RefreshAtMS <= now {
			level, err := ggoRollResourceTier(cfg.Runtime.Resource.Tiers)
			if err != nil {
				return false, err
			}
			node.Level, node.Remaining, node.ScheduledAtMS, node.RefreshAtMS = level, cfg.Runtime.Resource.Capacity, boundary, 0
			node.Generation, err = ggoNextGeneration(state)
			if err != nil {
				return false, err
			}
			changed = true
		}
	}
	for i := range state.Monsters {
		node := &state.Monsters[i]
		if node.ReservedBy > 0 && node.ReservedUntilMS > 0 && node.ReservedUntilMS <= now || node.ReservedBy == 0 && node.RefreshAtMS > 0 && node.RefreshAtMS <= now {
			token, err := fieldToken()
			if err != nil {
				return false, err
			}
			node.Generation, err = ggoNextGeneration(state)
			if err != nil {
				return false, err
			}
			node.UUID, node.ReservedBy, node.FightID, node.ReservedUntilMS, node.RefreshAtMS = token, 0, "", 0, 0
			node.PatrolEpochMS, node.PatrolPausedAtMS = now, 0
			changed = true
		}
		if node.PatrolEpochMS == 0 {
			node.PatrolEpochMS = now
			if node.ReservedBy > 0 || node.RefreshAtMS > 0 {
				node.PatrolPausedAtMS = now
			}
			changed = true
		}
		if node.ReservedBy > 0 && node.PatrolPausedAtMS == 0 {
			var point *model.AdminFieldGGOPoint
			for j := range cfg.Runtime.Points {
				if cfg.Runtime.Points[j].ID == node.PointID {
					point = &cfg.Runtime.Points[j]
					break
				}
			}
			migrated, err := ggoMigrateReservedMonsterPatrol(node, point, now)
			if err != nil {
				return false, err
			}
			changed = changed || migrated
		}
	}
	if changed {
		if err := ggoSavePointState(tx, row, state, now); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// Replacing an expired fight can touch two monster points. Lock both in key
// order before releasing the previous reservation and admitting the new one.
func (s *GameService) ggoLockExpiredAndTarget(tx *gorm.DB, id int64, game *model.GameState, targetUUID string, now int64) error {
	if game.GGOField == nil || game.GGOField.MonsterFight == nil {
		return nil
	}
	old := game.GGOField.MonsterFight
	if old.ReservedUntilMS < 1 || now < old.ReservedUntilMS {
		return nil
	}
	keys := []db.GGOFieldPoint{{ServerID: s.OriginalServerID(), Line: old.Line, PointID: old.PointID}}
	var presence db.P10ScenePresence
	if err := tx.First(&presence, "player_id=?", id).Error; err == nil {
		var target db.GGOFieldPoint
		err := tx.Select("server_id", "line", "point_id").Where("server_id=? AND line=? AND monster_uuid=?", s.OriginalServerID(), presence.Line, targetUUID).First(&target).Error
		if err == nil && (target.Line != old.Line || target.PointID != old.PointID) {
			keys = append(keys, target)
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Line != keys[j].Line {
			return keys[i].Line < keys[j].Line
		}
		return keys[i].PointID < keys[j].PointID
	})
	for _, key := range keys {
		if _, _, err := ggoLoadReservedPoint(tx, key.ServerID, key.Line, key.PointID); err != nil {
			return err
		}
	}
	return nil
}
