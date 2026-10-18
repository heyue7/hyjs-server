package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

func (s *GameService) InitializeP11Plunder(ctx context.Context) error {
	durations := map[string]int64{}
	for _, row := range s.Tables["Item"] {
		if row["category"] != "6" {
			continue
		}
		hours, err := strconv.ParseInt(row["exp"], 10, 64)
		if err != nil || hours <= 0 || hours > 8760 {
			return errors.New("原免战石小时配置无效")
		}
		durations[row["itemId"]] = hours
	}
	if len(durations) != 2 || durations["item_014"] != 8 || durations["item_015"] != 1 {
		return errors.New("原两种免战石完整配置缺失")
	}
	raw, err := json.Marshal(durations)
	if err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err = tx.AutoMigrate(&db.P11PlunderPolicy{}, &db.P11PlunderState{}); err != nil {
		return err
	}
	if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P11PlunderPolicy{ID: 1, ItemHoursJSON: string(raw), ItemHoursHash: p6Digest(raw), Source: constUtil.P11PlunderSource}).Error; err != nil {
		return err
	}
	if err := s.InitializeP11PlunderFight(ctx); err != nil {
		return err
	}
	return s.InitializeP11PlunderSweep(ctx)
}
func p11PlunderConfig(tx *gorm.DB) (*db.P11PlunderPolicy, map[string]int64, error) {
	var cfg db.P11PlunderPolicy
	if err := tx.First(&cfg, 1).Error; err != nil {
		return nil, nil, err
	}
	if cfg.Source == "" || cfg.ItemHoursHash != p6Digest([]byte(cfg.ItemHoursJSON)) {
		return nil, nil, errors.New("免战石运营配置完整性无效")
	}
	hours := map[string]int64{}
	if err := json.Unmarshal([]byte(cfg.ItemHoursJSON), &hours); err != nil {
		return nil, nil, err
	}
	if len(hours) != 2 || hours["item_014"] != 8 || hours["item_015"] != 1 {
		return nil, nil, errors.New("免战石原1/8小时配置未完整")
	}
	return &cfg, hours, nil
}
func p11PlunderState(tx *gorm.DB, id int64) (*db.P11PlunderState, error) {
	state := db.P11PlunderState{PlayerID: id}
	err := tx.First(&state, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &state, nil
	}
	if err != nil {
		return nil, err
	}
	if state.AvoidUntil < 0 || state.LastUseAt < 0 || state.LastUseAt > state.AvoidUntil {
		return nil, errors.New("夺宝免战存档无效")
	}
	return &state, nil
}

// P11PlunderProtected供未来权威双方锁定后的抢夺资格复用，跨日/离线不清除截止；到期立即失去保护。
func P11PlunderProtected(tx *gorm.DB, id int64, now time.Time) (bool, error) {
	state, err := p11PlunderState(tx, id)
	if err != nil {
		return false, err
	}
	return state.AvoidUntil > now.UnixMilli(), nil
}

// p11PlunderAt消费单件、实际到期、P1用量与原回执同事务，旧回执不延长或复活过期保护。
func (s *GameService) p11PlunderAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	switch body := request.(type) {
	case *pb.CSTreasureBattleList:
		return s.p11PlunderListAt(ctx, id, body.GetFragTreasId(), now)
	case *pb.CSFightTreasureStartResult:
		return s.p11PlunderFightAt(ctx, id, body, stamp, now)
	case *pb.CSSweepTreasureFight:
		return s.p11PlunderSweepAt(ctx, id, body, stamp, now)
	}
	if _, ok := request.(*pb.CSRefreshTreasFrag); !ok {
		if _, ok := request.(*pb.CSUseAvoidPlunder); !ok {
			return nil, errors.New(constUtil.P11PlunderClosed)
		}
	}
	var reply proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		state, err := p11PlunderState(tx, id)
		if err != nil {
			return err
		}
		if _, query := request.(*pb.CSRefreshTreasFrag); query {
			out := &pb.SCRefreshTreasFrag{Energy: proto.Int32(p.GetEnergy()), MaxEnergy: proto.Int32(constUtil.EnergyRecoveryLimit), AvoidWarTime: proto.Int64(max(int64(0), state.AvoidUntil-now.UnixMilli()))}
			seen := map[string]bool{}
			for _, item := range g.Items {
				if !strings.HasPrefix(item.GetId(), "frag_trea_") {
					continue
				}
				fragment := s.table("Frag", "id", item.GetId())
				if seen[item.GetId()] || item.GetNum() < 0 || fragment == nil || s.table("Treasure", "id", fragment["target"]) == nil {
					return errors.New("夺宝实际饰品碎片库存损坏")
				}
				seen[item.GetId()] = true
				if item.GetNum() > 0 {
					out.Frags = append(out.Frags, proto.Clone(item).(*pb.SimpleItem))
				}
			}
			reply = out
			return saveProfile(tx, &row, p, g)
		}
		b := request.(*pb.CSUseAvoidPlunder)
		raw, err := proto.Marshal(b)
		if err != nil {
			return err
		}
		if stamp <= 0 {
			return errors.New("原免战石HTTP使用缺少回执时间戳")
		}
		var receipt db.P11ArenaReceipt
		err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, "plunder-avoid", stamp).Error
		if err == nil {
			if receipt.RequestHash != p6Digest(raw) {
				return errors.New("原免战石使用回执请求冲突")
			}
			out := &pb.SCUseAvoidPlunder{}
			if err = proto.Unmarshal(receipt.Response, out); err != nil {
				return err
			}
			// 原客户端使用当前时间加剩余毫秒；重登或重试旧回执重新返回实际剩余，避免UI复活旧保护。
			out.AvoidSurplusTime = proto.Int64(max(int64(0), state.AvoidUntil-now.UnixMilli()))
			reply = out
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		cfg, hours, err := p11PlunderConfig(tx)
		if err != nil {
			return err
		}
		if !cfg.AvoidEnabled {
			return errors.New("原免战石新使用本批尚未开放")
		}
		level, err := s.p10Setting("RobTreasureLevel")
		if err != nil {
			return err
		}
		if p.GetLevel() < level {
			return errors.New("原夺宝等级未满足")
		}
		hour := hours[b.GetUsedItem()]
		if hour < 1 {
			return errors.New("原免战道具未恢复")
		}
		base := max(now.UnixMilli(), state.AvoidUntil)
		duration := hour * int64(time.Hour/time.Millisecond)
		if base > math.MaxInt64-duration {
			return errors.New("免战截止时间溢出")
		}
		cost := &pb.BaseResource{Id: proto.String(b.GetUsedItem()), Num: proto.Int32(1)}
		if err = deductPoolCost(p, g, cost); err != nil {
			return err
		}
		state.AvoidUntil = base + duration
		state.LastUseAt = now.UnixMilli()
		state.LastItem = b.GetUsedItem()
		state.UpdatedAt = now.UnixMilli()
		reply = &pb.SCUseAvoidPlunder{Cost: cost, AvoidSurplusTime: proto.Int64(state.AvoidUntil - now.UnixMilli())}
		if err = tx.Save(state).Error; err != nil {
			return err
		}
		if err = saveProfile(tx, &row, p, g); err != nil {
			return err
		}
		if s.P1Ready {
			source := fmt.Sprintf("p11-avoid:%d:%d", id, stamp)
			if err = s.RecordP1Event(tx, id, source, "treasure_avoid_item_used", 1, now); err != nil {
				return err
			}
		}
		blob, err := proto.Marshal(reply)
		if err != nil {
			return err
		}
		return tx.Create(&db.P11ArenaReceipt{PlayerID: id, Operation: "plunder-avoid", Stamp: stamp, RequestHash: p6Digest(raw), Response: blob, BatchID: b.GetUsedItem(), CreatedAt: now.UnixMilli()}).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}
