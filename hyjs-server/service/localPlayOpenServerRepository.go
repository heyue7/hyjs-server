package service

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
)

func localPlayOpenServerCycle(tx *gorm.DB) (*db.LocalPlayActivityCycle, error) {
	return localPlayBoundCycle(tx, localPlayActivityOwner(tx), "OpenHappy")
}

func localPlayOpenServerRule(tx *gorm.DB) (*db.OpenServerActivityRule, error) {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil || cycle == nil {
		return nil, err
	}
	var snapshot localPlayOpenServerSnapshot
	if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || snapshot.Rule.ID != cycle.ID || len(snapshot.Tasks) != 249 {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &snapshot.Rule, nil
}

func openServerReadTasks(tx *gorm.DB, day int32) ([]db.OpenServerActivityTask, error) {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return nil, err
	}
	if cycle == nil {
		var tasks []db.OpenServerActivityTask
		query := tx.Order("id")
		if day > 0 {
			query = query.Where("open_day <= ?", day)
		}
		err := query.Find(&tasks).Error
		return tasks, err
	}
	var snapshot localPlayOpenServerSnapshot
	if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || snapshot.Rule.ID != cycle.ID || len(snapshot.Tasks) != 249 {
		return nil, errLocalPlayActivitiesInvalid
	}
	var states []db.LocalPlayOpenServerState
	if err := tx.Where("cycle_id=? AND player_id=?", cycle.ID, cycle.PlayerID).Find(&states).Error; err != nil {
		return nil, err
	}
	stock := map[int32]int32{}
	for _, state := range states {
		stock[state.TaskID] = state.UsedStock
	}
	tasks := []db.OpenServerActivityTask{}
	for _, task := range snapshot.Tasks {
		if day > 0 && task.OpenDay > day {
			continue
		}
		task.UsedStock = stock[task.ID]
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func openServerReadTask(tx *gorm.DB, taskID int32) (db.OpenServerActivityTask, error) {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return db.OpenServerActivityTask{}, err
	}
	if cycle == nil {
		var task db.OpenServerActivityTask
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, "id=?", taskID).Error
		return task, err
	}
	tasks, err := openServerReadTasks(tx, 0)
	for _, task := range tasks {
		if task.ID == taskID {
			return task, err
		}
	}
	if err != nil {
		return db.OpenServerActivityTask{}, err
	}
	return db.OpenServerActivityTask{}, gorm.ErrRecordNotFound
}

func openServerReadStates(tx *gorm.DB, id int64) ([]db.OpenServerActivityState, error) {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return nil, err
	}
	states := []db.OpenServerActivityState{}
	if cycle == nil {
		err = tx.Where("player_id=?", id).Find(&states).Error
		return states, err
	}
	if cycle.PlayerID != id {
		return nil, errLocalPlayActivitiesInvalid
	}
	var scoped []db.LocalPlayOpenServerState
	if err := tx.Where("cycle_id=? AND player_id=?", cycle.ID, id).Find(&scoped).Error; err != nil {
		return nil, err
	}
	for _, state := range scoped {
		states = append(states, db.OpenServerActivityState{PlayerID: id, TaskID: state.TaskID, Achieved: state.Achieved, Progress: state.Progress, Claims: state.Claims})
	}
	return states, nil
}

func openServerSaveState(tx *gorm.DB, state *db.OpenServerActivityState) error {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return err
	}
	if cycle == nil {
		return tx.Save(state).Error
	}
	if cycle.PlayerID != state.PlayerID {
		return errLocalPlayActivitiesInvalid
	}
	row := db.LocalPlayOpenServerState{CycleID: cycle.ID, PlayerID: state.PlayerID, TaskID: state.TaskID, Achieved: state.Achieved, Progress: state.Progress, Claims: state.Claims}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cycle_id"}, {Name: "task_id"}}, DoUpdates: clause.AssignmentColumns([]string{"achieved", "progress", "claims"})}).Create(&row).Error
}

func openServerSaveStock(tx *gorm.DB, task db.OpenServerActivityTask, id int64) error {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return err
	}
	if cycle == nil {
		return tx.Model(&task).Update("used_stock", task.UsedStock).Error
	}
	if cycle.PlayerID != id {
		return errLocalPlayActivitiesInvalid
	}
	return tx.Model(&db.LocalPlayOpenServerState{}).Where("cycle_id=? AND player_id=? AND task_id=?", cycle.ID, id, task.ID).Update("used_stock", task.UsedStock).Error
}

func openServerReadCounters(tx *gorm.DB, id int64) ([]db.OpenServerActivityCounter, error) {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return nil, err
	}
	rows := []db.OpenServerActivityCounter{}
	if cycle == nil {
		err = tx.Where("player_id=?", id).Find(&rows).Error
		return rows, err
	}
	if cycle.PlayerID != id {
		return nil, errLocalPlayActivitiesInvalid
	}
	var scoped []db.LocalPlayOpenServerCounter
	if err := tx.Where("cycle_id=? AND player_id=?", cycle.ID, id).Find(&scoped).Error; err != nil {
		return nil, err
	}
	for _, r := range scoped {
		rows = append(rows, db.OpenServerActivityCounter{PlayerID: id, CounterKey: r.CounterKey, Value: r.Value})
	}
	return rows, nil
}

func openServerReadCounter(tx *gorm.DB, id int64, key string) (db.OpenServerActivityCounter, error) {
	rows, err := openServerReadCounters(tx, id)
	if err != nil {
		return db.OpenServerActivityCounter{}, err
	}
	for _, row := range rows {
		if row.CounterKey == key {
			return row, nil
		}
	}
	return db.OpenServerActivityCounter{}, gorm.ErrRecordNotFound
}

func openServerSaveCounter(tx *gorm.DB, row *db.OpenServerActivityCounter) error {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return err
	}
	if cycle == nil {
		return tx.Save(row).Error
	}
	if cycle.PlayerID != row.PlayerID {
		return errLocalPlayActivitiesInvalid
	}
	return tx.Save(&db.LocalPlayOpenServerCounter{CycleID: cycle.ID, PlayerID: row.PlayerID, CounterKey: row.CounterKey, Value: row.Value}).Error
}

func openServerReadReceipt(tx *gorm.DB, id, stamp int64) (db.OpenServerActivityReceipt, error) {
	var row db.OpenServerActivityReceipt
	err := tx.First(&row, "player_id=? AND timestamp=?", id, stamp).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return row, err
	}
	if localPlayActivityOwner(tx) != id || !tx.Migrator().HasTable(&db.LocalPlayOpenServerReceipt{}) {
		return row, err
	}
	var scoped db.LocalPlayOpenServerReceipt
	err = tx.First(&scoped, "player_id=? AND timestamp=?", id, stamp).Error
	return db.OpenServerActivityReceipt{PlayerID: id, Timestamp: stamp, Request: scoped.Request, Response: scoped.Response}, err
}

func openServerCreateReceipt(tx *gorm.DB, row *db.OpenServerActivityReceipt) error {
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return err
	}
	if cycle == nil {
		return tx.Create(row).Error
	}
	if cycle.PlayerID != row.PlayerID {
		return errLocalPlayActivitiesInvalid
	}
	return tx.Create(&db.LocalPlayOpenServerReceipt{PlayerID: row.PlayerID, Timestamp: row.Timestamp, CycleID: cycle.ID, Request: row.Request, Response: row.Response}).Error
}
