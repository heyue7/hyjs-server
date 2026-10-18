package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// nil means the caller should use its original month-keyed SQL repository.
func localPlayMonthSignState(tx *gorm.DB, id int64, month string) (*db.MonthSignState, error) {
	cycle, err := localPlayEntryCycle(tx, "EveryDaySign")
	if err != nil || cycle == nil {
		return nil, err
	}
	if cycle.PlayerID != id {
		return nil, errLocalPlayActivitiesInvalid
	}
	var operation db.LocalPlayActivityOperation
	err = tx.First(&operation, "player_id=? AND request_id=?", id, "month:"+cycle.ID+":"+month).Error
	state := &db.MonthSignState{PlayerID: id, Month: month}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if operation.Action != "month-sign" || operation.ActivityID != cycle.ID || json.Unmarshal(operation.Response, state) != nil || state.PlayerID != id || state.Month != month {
		return nil, errLocalPlayActivitiesInvalid
	}
	return state, nil
}

func localPlaySaveMonthSign(tx *gorm.DB, state *db.MonthSignState) error {
	cycle, err := localPlayEntryCycle(tx, "EveryDaySign")
	if err != nil {
		return err
	}
	if cycle == nil {
		return tx.Save(state).Error
	}
	if cycle.PlayerID != state.PlayerID {
		return errLocalPlayActivitiesInvalid
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	operation := db.LocalPlayActivityOperation{PlayerID: state.PlayerID, RequestID: "month:" + cycle.ID + ":" + state.Month, Action: "month-sign", ActivityID: cycle.ID, Response: raw, CreatedAt: time.Now().UnixMilli()}
	return tx.Save(&operation).Error
}

func localPlayMonthSignRules(tx *gorm.DB) ([]db.MonthSignRule, error) {
	cycle, err := localPlayEntryCycle(tx, "EveryDaySign")
	if err != nil || cycle == nil {
		return nil, err
	}
	var snapshot localPlayScopedSnapshot
	if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || len(snapshot.MonthSign) != 31 {
		return nil, errLocalPlayActivitiesInvalid
	}
	// 历史个人奖历只冻结内容，当前配置停用后不能继续从旧快照领取。
	var live []db.MonthSignRule
	if err := tx.Order("day ASC").Find(&live).Error; err != nil {
		return nil, err
	}
	if len(live) != len(snapshot.MonthSign) {
		return nil, errLocalPlayActivitySource
	}
	for i := range snapshot.MonthSign {
		if live[i].Day != snapshot.MonthSign[i].Day {
			return nil, errLocalPlayActivitySource
		}
		snapshot.MonthSign[i].Enabled = live[i].Enabled
	}
	return snapshot.MonthSign, nil
}

// Positive original stamps are retained across every refresh. TCP zero keeps
// the original daily semantics. No request mutates an old receipt.
func localPlayMonthSignReplay(tx *gorm.DB, id int64, request *pb.CSReceiveMonthSign, stamp int64) (*pb.SCReceiveMonthSign, error) {
	if stamp == 0 {
		return nil, nil
	}
	if stamp < 0 {
		return nil, errLocalPlayActivitiesInvalid
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	var receipt db.P4Receipt
	err = tx.First(&receipt, "player_id=? AND domain=? AND timestamp=?", id, "local-play-month-sign", stamp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(raw, receipt.Request) {
		return nil, errors.New("月签到回执时间戳冲突")
	}
	response := &pb.SCReceiveMonthSign{}
	if err := proto.Unmarshal(receipt.Response, response); err != nil {
		return nil, err
	}
	return response, nil
}

func localPlayMonthSignSaveReceipt(tx *gorm.DB, id int64, request *pb.CSReceiveMonthSign, stamp int64, response *pb.SCReceiveMonthSign) error {
	if stamp == 0 {
		return nil
	}
	if stamp < 0 {
		return errLocalPlayActivitiesInvalid
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	reply, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	return tx.Create(&db.P4Receipt{PlayerID: id, Domain: "local-play-month-sign", Timestamp: stamp, Request: raw, Response: reply}).Error
}
