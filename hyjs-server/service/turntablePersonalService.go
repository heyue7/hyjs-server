package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"time"
)

func turntablePersonalOwner(tx *gorm.DB) int64 {
	id, _ := tx.Statement.Context.Value(constUtil.PersonalTurntableOwnerKey).(int64)
	return id
}

// turntableRuntimeGlobal retains the old internal projection while storing new state per owner.
func (s *GameService) turntableRuntimeGlobal(tx *gorm.DB, key string) (*db.TurntableGlobal, error) {
	if key == "" {
		return nil, errors.New("当前区服暂未安排转盘活动")
	}
	id := turntablePersonalOwner(tx)
	if id == 0 {
		var row db.TurntableGlobal
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "pool_id=?", key).Error
		return &row, err
	}
	var cfg db.GameplayPool
	if err := tx.First(&cfg, "id=?", key).Error; err != nil {
		return nil, err
	}
	var row db.TurntablePersonal
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "pool_id=? AND player_id=?", key, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Personal migration preserves this player's consumed finite rewards, never another player's stock.
		var rewards []db.GameplayPoolReward
		if err := tx.Where("pool_id=? AND stock>0", key).Find(&rewards).Error; err != nil {
			return nil, err
		}
		stock := map[int32]int32{}
		for _, r := range rewards {
			var n int64
			if err := tx.Model(&db.TurntableRecord{}).Where("pool_id=? AND player_id=? AND resource_id=? AND jackpot=?", key, id, r.ResourceID, false).Count(&n).Error; err != nil {
				return nil, err
			}
			stock[r.ID] = int32(min(n, int64(r.Stock)))
		}
		raw, e := json.Marshal(stock)
		if e != nil {
			return nil, e
		}
		var legacy db.TurntableState
		if err := tx.First(&legacy, "pool_id=? AND player_id=?", key, id).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		row = db.TurntablePersonal{LegacyScore: legacy.Score, PoolID: key, PlayerID: id, DiamondPool: cfg.InitialDiamondPool, UsedStock: raw, CreatedAt: time.Now().UnixMilli()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return nil, err
		}
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "pool_id=? AND player_id=?", key, id).Error
	}
	if err != nil {
		return nil, err
	}
	return &db.TurntableGlobal{PoolID: key, DiamondPool: row.DiamondPool, ContributionRemainder: row.ContributionRemainder, UsedStock: row.UsedStock, Settled: row.Settled, FinalRanks: []byte("[]")}, nil
}
func saveTurntableRuntimeGlobal(tx *gorm.DB, row *db.TurntableGlobal) error {
	if id := turntablePersonalOwner(tx); id > 0 {
		return tx.Model(&db.TurntablePersonal{}).Where("pool_id=? AND player_id=?", row.PoolID, id).Updates(map[string]any{"diamond_pool": row.DiamondPool, "contribution_remainder": row.ContributionRemainder, "used_stock": row.UsedStock, "settled": row.Settled}).Error
	}
	return tx.Save(row).Error
}

// playBigTurn retries only rolled-back MySQL lock conflicts, preserving the same frozen request stamp.
func (s *GameService) playBigTurn(ctx context.Context, id int64, b *pb.CSPlayBigTurn, stamp int64) ([]proto.Message, error) {
	var result []proto.Message
	var err error
	for attempt := 0; attempt < constUtil.GameLocalMutationMaxAttempts; attempt++ {
		result, err = s.playBigTurnOnce(ctx, id, b, stamp)
		var conflict *mysql.MySQLError
		if !errors.As(err, &conflict) || (conflict.Number != 1213 && conflict.Number != 1205) {
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return result, err
}

// turntableReplayFrozen resolves the receipt before selecting today's scheduled wheel.
func turntableReplayFrozen(tx *gorm.DB, id int64, request *pb.CSPlayBigTurn, stamp int64) (*pb.SCPlayBigTurn, error) {
	if stamp <= 0 {
		return nil, errors.New("转盘请求时间戳无效")
	}
	var stored db.TurntableDrawReceipt
	err := tx.First(&stored, "player_id=? AND stamp=?", id, stamp).Error
	if err == nil {
		if stored.Count != request.GetCount() {
			return nil, errors.New("转盘时间戳冲突")
		}
		out := &pb.SCPlayBigTurn{}
		if err := proto.Unmarshal(stored.Response, out); err != nil {
			return nil, err
		}
		return out, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	old, err := localPlayTurntableReplay(tx, id, request, stamp)
	if err != nil || old != nil {
		return old, err
	}
	var legacy db.TurntableState
	err = tx.Where("player_id=?", id).Order("timestamp DESC").First(&legacy).Error
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
		out := &pb.SCPlayBigTurn{}
		if err := proto.Unmarshal(legacy.Response, out); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, nil
}
