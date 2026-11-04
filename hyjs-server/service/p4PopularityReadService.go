package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// This context is scoped to one MVCC list transaction; trading retains its leases.
type p4PopularityReadKey struct{}

var errP4PopularityNeedsWrite = errors.New("声望商店首次初始化需要写事务")

func p4PopularityReadOnly(tx *gorm.DB) bool {
	return tx != nil && tx.Statement != nil && tx.Statement.Context != nil && tx.Statement.Context.Value(p4PopularityReadKey{}) == true
}

// Verify the request's calendar revision inside the same snapshot as the list.
// A concurrent calendar change can serialize after this read-only transaction.
func p4PopularityReadClock(tx *gorm.DB) error {
	ctx := tx.Statement.Context
	if !ServerGameClockContextBound(ctx) {
		return nil
	}
	serverID, _ := ctx.Value(constUtil.ServerGameClockServerKey).(int64)
	revision, _ := ctx.Value(constUtil.ServerGameClockRevisionKey).(int64)
	var clock db.ServerGameClock
	if err := tx.First(&clock, "server_id=?", serverID).Error; err != nil {
		return err
	}
	if !serverGameClockValid(&clock) {
		return constUtil.ErrAdminConfiguration
	}
	if clock.Revision != revision {
		return fmt.Errorf("%w: 全服日期刚刚调整，请重试当前操作", constUtil.ErrAdminConfigConflict)
	}
	return nil
}

type p4PopularityProfileRead struct {
	Player  *string `gorm:"column:player"`
	HasGame bool    `gorm:"column:has_game"`
}

func p4PopularityReadPlayer(tx *gorm.DB, id int64) (*pb.Player, error) {
	var row p4PopularityProfileRead
	if err := tx.Model(&db.Profile{}).Select("player, game IS NOT NULL AS has_game").Where("id=?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	if row.Player == nil || !row.HasGame {
		return nil, errors.New("请先创建角色")
	}
	var player pb.Player
	if err := json.Unmarshal([]byte(*row.Player), &player); err != nil {
		return nil, err
	}
	return &player, nil
}

func (s *GameService) p4PopularityListRead(ctx context.Context, id int64, now time.Time) (*pb.SCShopProductList, error) {
	ctx = context.WithValue(ctx, p4PopularityReadKey{}, true)
	var response *pb.SCShopProductList
	var opts *sql.TxOptions
	if db.GetIns().Dialector.Name() == "mysql" {
		opts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := p4PopularityReadClock(tx); err != nil {
			return err
		}
		player, err := p4PopularityReadPlayer(tx, id)
		if err != nil {
			return err
		}
		cfg, rules, _, state, err := s.loadP4CommerceMode(tx, id, "", 4, now, false, false, nil, true)
		var items []*pb.ShopItem
		if err == nil {
			items, _, err = s.p4Projection(tx, player, cfg, rules, state)
		} else if errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
			err = nil
		}
		if err != nil {
			return err
		}
		items, err = s.p4PopularityShopItems(tx, id, player, cfg, items, now)
		if err != nil {
			return err
		}
		if p4PopularityEmptyGuide(player, items) {
			return errP4PopularityNeedsWrite
		}
		response = &pb.SCShopProductList{ShopType: proto.Int32(4), ShopItemList: items}
		return nil
	}, opts)
	return response, err
}
