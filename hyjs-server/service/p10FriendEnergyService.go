package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP10FriendEnergy(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10FriendEnergyPolicy{}, &db.P10FriendEnergyGift{}, &db.P10FriendEnergyDaily{}); err != nil {
		return err
	}
	cfg := db.P10FriendEnergyPolicy{ID: constUtil.P10FriendEnergyPolicyID, Amount: 1, Source: constUtil.P10FriendEnergySource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}
func p10EnergyRelation(tx *gorm.DB, a, b int64) (*db.P10SocialRelation, error) {
	blocked, err := p10Blocked(tx, a, b)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, errors.New("黑名单阻止耐力赠领")
	}
	lo, hi := p10Pair(a, b)
	var relation db.P10SocialRelation
	// 等待角色锁后使用当前读，不能沿批量候选的旧事务快照继续给已删除关系发奖。
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("low_id=? AND high_id=?", lo, hi).First(&relation).Error; err != nil {
		return nil, errors.New("仅当前双方好友可赠领耐力")
	}
	return &relation, nil
}
func p10EnergyDaily(tx *gorm.DB, id int64, day string) (*db.P10FriendEnergyDaily, error) {
	row := &db.P10FriendEnergyDaily{PlayerID: id, Day: day}
	err := tx.Where("player_id=? AND day=?", id, day).First(row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if row.Claimed < 0 {
		return nil, errors.New("好友耐力每日记录无效")
	}
	return row, nil
}

// P10FriendEnergyFlags 与原FriendList共享真实赠送/领取按钮状态，不制造可领取余额。
func (s *GameService) P10FriendEnergyClaimed(tx *gorm.DB, id int64, now time.Time) (int32, error) {
	if !gameSchemaHasTable(tx, &db.P10FriendEnergyDaily{}) {
		return 0, nil
	}
	daily, err := p10EnergyDaily(tx, id, p1Day(now))
	if err != nil {
		return 0, err
	}
	return daily.Claimed, nil
}

// 旧签名保留，候选中共享调用切换前仍可完整编译。
func (s *GameService) P10FriendEnergyFlags(tx *gorm.DB, id, target int64, now time.Time) (int32, int32, int32, error) {
	claimed, err := s.P10FriendEnergyClaimed(tx, id, now)
	if err != nil {
		return 0, 0, 0, err
	}
	give, get, err := s.P10FriendEnergyFlagsWithCount(tx, id, target, claimed, now)
	return give, get, claimed, err
}
func (s *GameService) P10FriendEnergyFlagsWithCount(tx *gorm.DB, id, target int64, claimed int32, now time.Time) (int32, int32, error) {
	if !gameSchemaHasTable(tx, &db.P10FriendEnergyPolicy{}) {
		return 0, 0, nil
	}
	var cfg db.P10FriendEnergyPolicy
	if err := tx.First(&cfg, constUtil.P10FriendEnergyPolicyID).Error; err != nil {
		return 0, 0, err
	}
	relation, err := p10EnergyRelation(tx, id, target)
	if err != nil {
		return 0, 0, nil
	}
	give, get := int32(0), int32(0)
	var count int64
	if err = tx.Model(&db.P10FriendEnergyGift{}).Where("sender_id=? AND receiver_id=? AND day=?", id, target, p1Day(now)).Count(&count).Error; err != nil {
		return 0, 0, err
	}
	if cfg.Enabled && count == 0 {
		give = 1
	}
	var gift db.P10FriendEnergyGift
	err = tx.Where("sender_id=? AND receiver_id=? AND day=?", target, id, p1Day(now)).First(&gift).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, err
	}
	limit, err := s.p10Setting("MaxGetFriendEnergy")
	if err != nil {
		return 0, 0, err
	}
	if gift.Amount > 0 && !gift.Claimed && !gift.Invalidated && gift.FriendSince == relation.CreatedAt && claimed < limit {
		get = 1
	}
	return give, get, nil
}
func (s *GameService) p10FriendEnergy(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	var domain string
	var target int64
	give, batch := false, false
	switch b := request.(type) {
	case *pb.CSGiveEnergy:
		target = b.GetFriendId()
		give = true
		domain = "energy-give"
		response = &pb.SCGiveEnergy{}
	case *pb.CSGetEnergy:
		target = b.GetFriendId()
		domain = "energy-get"
		response = &pb.SCGetEnergy{}
	case *pb.CSBatchSendFriendEnergy:
		give = true
		batch = true
		domain = "energy-batch-give"
		response = &pb.SCBatchSendFriendEnergy{}
	case *pb.CSBatchGetFriendEnergy:
		batch = true
		domain = "energy-batch-get"
		response = &pb.SCBatchGetFriendEnergy{}
	default:
		return nil, errors.New("好友耐力请求未恢复")
	}
	if !batch && (target <= 0 || target == id) {
		return nil, errors.New("好友耐力目标无效")
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		ids := []int64{id}
		targets := []int64{target}
		if batch {
			var relations []db.P10SocialRelation
			if err := tx.Where("low_id=? OR high_id=?", id, id).Find(&relations).Error; err != nil {
				return err
			}
			targets = nil
			for _, r := range relations {
				peer := r.LowID
				if peer == id {
					peer = r.HighID
				}
				targets = append(targets, peer)
			}
		}
		ids = append(ids, targets...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })
		var current db.Profile
		for _, pid := range ids {
			var row db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, pid).Error; err != nil {
				return err
			}
			if _, _, err := decodeProfile(&row); err != nil {
				return err
			}
			if pid == id {
				current = row
			}
		}
		if found, err := p10ReadReceipt(tx, id, domain, stamp, request, response); err != nil || found {
			return err
		}
		var social db.P10SocialPolicy
		if err := tx.First(&social, constUtil.P10SocialPolicyID).Error; err != nil {
			return err
		}
		if !social.Enabled {
			return errors.New(constUtil.P10SocialClosed)
		}
		var cfg db.P10FriendEnergyPolicy
		if err := tx.First(&cfg, constUtil.P10FriendEnergyPolicyID).Error; err != nil {
			return err
		}
		if cfg.Amount < 1 || cfg.Amount > 30 || cfg.Source == "" {
			return errors.New("好友耐力有限规则无效")
		}
		if give && !cfg.Enabled {
			return errors.New(constUtil.P10FriendEnergyClosed)
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &current)
		if err != nil {
			return err
		}
		daily, err := p10EnergyDaily(tx, id, p1Day(now))
		if err != nil {
			return err
		}
		limit, err := s.p10Setting("MaxGetFriendEnergy")
		if err != nil || limit < 1 {
			return errors.New("原每日领取上限无效")
		}
		total := int32(0)
		claimedIDs := []int64{}
		for _, peer := range targets {
			relation, err := p10EnergyRelation(tx, id, peer)
			if err != nil {
				if batch {
					continue
				}
				return err
			}
			sender, receiver := id, peer
			if !give {
				sender, receiver = peer, id
			}
			var gift db.P10FriendEnergyGift
			err = tx.Where("sender_id=? AND receiver_id=? AND day=?", sender, receiver, p1Day(now)).First(&gift).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if give {
				if err == nil {
					continue
				}
				gift = db.P10FriendEnergyGift{SenderID: id, ReceiverID: peer, Day: p1Day(now), Amount: cfg.Amount, FriendSince: relation.CreatedAt, CreatedAt: now.UnixMilli()}
				if err = tx.Create(&gift).Error; err != nil {
					return err
				}
				if err = s.RecordP1Event(tx, id, fmt.Sprintf("p10-energy-give:%d:%d:%s", id, peer, gift.Day), "friend_energy_sent", 1, now); err != nil {
					return err
				}
			} else {
				if errors.Is(err, gorm.ErrRecordNotFound) || gift.Claimed || gift.Invalidated || gift.FriendSince != relation.CreatedAt {
					if batch {
						continue
					}
					return errors.New("没有当前关系可领取的当天耐力")
				}
				if gift.Amount < 1 || gift.Amount > 30 {
					return errors.New("冻结赠送耐力无效")
				}
				if daily.Claimed >= limit || p.GetEnergy() > constUtil.EnergyRecoveryLimit-gift.Amount {
					if batch {
						continue
					}
					return errors.New("耐力已满或每日领取次数已用完")
				}
				p.Energy = proto.Int32(p.GetEnergy() + gift.Amount)
				daily.Claimed++
				total += gift.Amount
				claimedIDs = append(claimedIDs, peer)
				gift.Claimed = true
				if err = tx.Save(&gift).Error; err != nil {
					return err
				}
				if err = s.RecordP1Event(tx, id, fmt.Sprintf("p10-energy-get:%d:%d:%s", peer, id, gift.Day), "friend_energy_received", int64(gift.Amount), now); err != nil {
					return err
				}
			}
		}
		if !give {
			if err = tx.Save(daily).Error; err != nil {
				return err
			}
			if err = saveProfile(tx, &current, p, g); err != nil {
				return err
			}
			if batch {
				response = &pb.SCBatchGetFriendEnergy{Add: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("energy"), Num: proto.Int32(total)}}}, GetIds: claimedIDs}
			} else {
				response = &pb.SCGetEnergy{CurrentEnergy: p.Energy}
			}
		}
		return p10SaveReceipt(tx, id, domain, stamp, request, response)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
func (s *GameService) DispatchP10FriendEnergy(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSGiveEnergy, *pb.CSGetEnergy, *pb.CSBatchSendFriendEnergy, *pb.CSBatchGetFriendEnergy:
		r, err := s.p10FriendEnergy(ctx, id, request, stamp, time.Now())
		return r, true, err
	}
	return nil, false, nil
}

// InvalidateP10FriendEnergy 关系删除/拉黑和失效同事务，防止同毫秒重新加好友恢复旧赠送。
func (s *GameService) InvalidateP10FriendEnergy(tx *gorm.DB, a, b int64) error {
	if !gameSchemaHasTable(tx, &db.P10FriendEnergyGift{}) {
		return nil
	}
	return tx.Model(&db.P10FriendEnergyGift{}).Where("(sender_id=? AND receiver_id=?) OR (sender_id=? AND receiver_id=?)", a, b, b, a).Update("invalidated", true).Error
}
