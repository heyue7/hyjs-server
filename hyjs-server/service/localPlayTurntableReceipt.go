package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type localPlayWheelReceipt struct {
	Count    int32  `json:"count"`
	Response []byte `json:"response"`
}

// Keep original draw stamps unique across private refreshes. Administrative
// request IDs cannot contain ':', so this receipt namespace cannot collide.
func localPlayTurntableReplay(tx *gorm.DB, id int64, request *pb.CSPlayBigTurn, stamp int64) (*pb.SCPlayBigTurn, error) {
	if localPlayActivityOwner(tx) != id || !tx.Migrator().HasTable(&db.LocalPlayActivityOperation{}) {
		return nil, nil
	}
	if stamp <= 0 {
		return nil, errors.New("转盘请求时间戳无效")
	}
	var receipt db.LocalPlayActivityOperation
	err := tx.First(&receipt, "player_id=? AND request_id=?", id, "wheel:"+strconv.FormatInt(stamp, 10)).Error
	if err == nil {
		var stored localPlayWheelReceipt
		if receipt.Action != "wheel" || json.Unmarshal(receipt.Response, &stored) != nil || stored.Count != request.GetCount() {
			return nil, errors.New("转盘跨期时间戳冲突")
		}
		result := &pb.SCPlayBigTurn{}
		if err := proto.Unmarshal(stored.Response, result); err != nil {
			return nil, err
		}
		return result, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// The legacy service keeps only its latest stamp. Preserve that replay and
	// reject earlier legacy stamps rather than executing them in a fresh cycle.
	var legacy db.TurntableState
	err = tx.Where("player_id=? AND pool_id NOT LIKE ?", id, "lp-%").Order("timestamp DESC").First(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if stamp < legacy.Timestamp {
		return nil, errors.New("旧转盘请求时间戳已过期")
	}
	if stamp == legacy.Timestamp && len(legacy.Response) > 0 {
		if legacy.RequestCount != request.GetCount() {
			return nil, errors.New("转盘旧期时间戳冲突")
		}
		result := &pb.SCPlayBigTurn{}
		if err := proto.Unmarshal(legacy.Response, result); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, nil
}

func localPlayTurntableSaveReceipt(tx *gorm.DB, id int64, request *pb.CSPlayBigTurn, stamp int64, result *pb.SCPlayBigTurn) error {
	cycle, err := localPlayBoundCycle(tx, id, "BigTurnTable")
	if err != nil || cycle == nil {
		return err
	}
	raw, err := proto.Marshal(result)
	if err != nil {
		return err
	}
	stored, err := json.Marshal(localPlayWheelReceipt{Count: request.GetCount(), Response: raw})
	if err != nil {
		return err
	}
	return tx.Create(&db.LocalPlayActivityOperation{PlayerID: id, RequestID: "wheel:" + strconv.FormatInt(stamp, 10), Action: "wheel", ActivityID: cycle.ID, Response: stored, CreatedAt: time.Now().UnixMilli()}).Error
}
