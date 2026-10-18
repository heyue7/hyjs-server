package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"time"
)

type p11GrabReward struct {
	Hours  int64
	Reward string
}

func (s *GameService) InitializeP11Grab(ctx context.Context) error {
	var cfg = db.P11GrabPolicy{ID: 1, Source: constUtil.P11GrabSource}
	fields := []struct {
		name string
		dst  *int32
	}{{"grabMaxTimes", &cfg.MaxTimes}, {"grabBuyTimesLimit", &cfg.BuyMax}, {"grabBuyPrice", &cfg.BuyPrice}, {"grabEnergy", &cfg.Energy}}
	for _, field := range fields {
		n, err := s.p10Setting(field.name)
		if err != nil {
			return err
		}
		*field.dst = n
	}
	hours, err := s.p10Setting("grabMaxTime")
	if err != nil {
		return err
	}
	cfg.MaxMillis = int64(hours) * int64(time.Hour/time.Millisecond)
	var rewards []p11GrabReward
	for _, row := range s.Tables["GrabTimeReward"] {
		h, err := strconv.ParseInt(row["time"], 10, 64)
		if err != nil || h < 1 || h > 3 {
			return errors.New("原占位时间奖时长无效")
		}
		supported, err := s.openServerRewardSupported(row["reward"])
		if err != nil || !supported {
			return errors.New("原占位时间奖全档资源未恢复")
		}
		rewards = append(rewards, p11GrabReward{h, row["reward"]})
	}
	if len(rewards) != 3 {
		return errors.New("原占位3个时间奖档缺失")
	}
	raw, err := json.Marshal(rewards)
	if err != nil {
		return err
	}
	cfg.RewardsJSON = string(raw)
	cfg.RewardsHash = p6Digest(raw)
	tx := db.GetIns().WithContext(ctx)
	if err = tx.AutoMigrate(&db.P11GrabPolicy{}, &db.P11GrabSlot{}, &db.P11GrabSession{}, &db.P11GrabDaily{}, &db.P11GrabRewardClaim{}); err != nil {
		return err
	}
	if err = s.InitializeP11GrabFight(ctx); err != nil {
		return err
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
		for position := int64(1); position <= 18; position++ {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P11GrabSlot{Position: position}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func p11GrabConfig(tx *gorm.DB) (*db.P11GrabPolicy, []p11GrabReward, error) {
	var cfg db.P11GrabPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
		return nil, nil, err
	}
	if cfg.Source == "" || cfg.MaxTimes != 3 || cfg.BuyMax != 3 || cfg.BuyPrice != 100 || cfg.Energy != 10 || cfg.MaxMillis != int64(3*time.Hour/time.Millisecond) || cfg.RewardsHash != p6Digest([]byte(cfg.RewardsJSON)) {
		return nil, nil, errors.New("原奖杯争夺全配置未完整")
	}
	var rewards []p11GrabReward
	if err := json.Unmarshal([]byte(cfg.RewardsJSON), &rewards); err != nil {
		return nil, nil, err
	}
	seen := map[int64]bool{}
	for _, r := range rewards {
		if r.Hours < 1 || r.Hours > 3 || seen[r.Hours] || r.Reward == "" {
			return nil, nil, errors.New("原奖杯时间奖档损坏")
		}
		seen[r.Hours] = true
	}
	if len(rewards) != 3 {
		return nil, nil, errors.New("原奖杯三档時間奖不完整")
	}
	return &cfg, rewards, nil
}
func p11GrabDaily(tx *gorm.DB, id int64, now time.Time, cfg *db.P11GrabPolicy) (*db.P11GrabDaily, error) {
	daily := db.P11GrabDaily{PlayerID: id, Day: dailyCopyDay(now), RewardsJSON: cfg.RewardsJSON, RewardsHash: cfg.RewardsHash}
	err := tx.First(&daily, "player_id=? AND day=?", id, daily.Day).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if daily.RewardsHash != p6Digest([]byte(daily.RewardsJSON)) {
		return nil, errors.New("实际占位日奖档冻结损坏")
	}
	remaining := cfg.MaxTimes + daily.Bought - daily.Used
	if daily.Used < 0 || daily.Bought < 0 || daily.Bought > cfg.BuyMax || remaining < 0 || remaining > cfg.MaxTimes {
		return nil, errors.New("奖杯原次数存档无效")
	}
	return &daily, nil
}
func p11GrabClose(tx *gorm.DB, slot *db.P11GrabSlot, at int64, reason string) error {
	if slot.OwnerID == nil {
		return nil
	}
	var session db.P11GrabSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id=?", slot.SessionID).Error; err != nil {
		return err
	}
	if session.PlayerID != *slot.OwnerID || session.Position != slot.Position || session.StartedAt != slot.StartedAt || session.ExpiresAt != slot.ExpiresAt || session.EndedAt != 0 || at < session.StartedAt || at > session.ExpiresAt {
		return errors.New("真实占位分段归属/截止无效")
	}
	session.EndedAt = at
	session.EndReason = reason
	if err := tx.Save(&session).Error; err != nil {
		return err
	}
	slot.OwnerID = nil
	slot.SessionID = ""
	slot.StartedAt = 0
	slot.ExpiresAt = 0
	return tx.Save(slot).Error
}

// p11GrabExpire以既有原3h截止关闭实际分段，重启/重复查询不追加占领时长。
func p11GrabExpire(tx *gorm.DB, now time.Time) error {
	var slots []db.P11GrabSlot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_id IS NOT NULL AND expires_at<=?", now.UnixMilli()).Order("position").Find(&slots).Error; err != nil {
		return err
	}
	for i := range slots {
		if err := p11GrabClose(tx, &slots[i], slots[i].ExpiresAt, "expired"); err != nil {
			return err
		}
	}
	return nil
}
func p11GrabFrame(tx *gorm.DB, slot db.P11GrabSlot) (*pb.GrabPositionModel, error) {
	if slot.Position < 1 || slot.Position > 18 {
		return nil, errors.New("原占位位置越界")
	}
	out := &pb.GrabPositionModel{Position: proto.Int32(int32(slot.Position)), PlayerId: proto.Int64(0), GrabTime: proto.Int64(0), IsGrab: proto.Bool(false), PlayerNam: proto.String("")}
	if slot.OwnerID == nil {
		return out, nil
	}
	var profile db.Profile
	if err := tx.First(&profile, *slot.OwnerID).Error; err != nil {
		return nil, err
	}
	p, _, err := decodeProfile(&profile)
	if err != nil {
		return nil, err
	}
	if slot.Position < 1 || slot.Position > 18 || slot.StartedAt < 1 || slot.ExpiresAt <= slot.StartedAt || slot.SessionID == "" || p.GetPlayerId() != *slot.OwnerID {
		return nil, errors.New("真实占位当前归属无效")
	}
	out.PlayerId = proto.Int64(*slot.OwnerID)
	out.PlayerNam = proto.String(p.GetName())
	out.GrabTime = proto.Int64(slot.StartedAt)
	out.IsGrab = proto.Bool(true)
	return out, nil
}

// p11GrabMillis从实际会话分段计算UTC+8本日交集，重启/换位/日切不重复相加。
func p11GrabMillis(tx *gorm.DB, id int64, now time.Time) (int64, error) {
	location := time.FixedZone("UTC+8", 8*3600)
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UnixMilli()
	end := min(now.UnixMilli(), start+int64(24*time.Hour/time.Millisecond))
	var sessions []db.P11GrabSession
	if err := tx.Where("player_id=? AND started_at<? AND expires_at>?", id, end, start).Order("started_at,id").Find(&sessions).Error; err != nil {
		return 0, err
	}
	var total int64
	lastEnd := int64(0)
	for _, session := range sessions {
		until := session.EndedAt
		if until == 0 {
			until = min(now.UnixMilli(), session.ExpiresAt)
		}
		if session.StartedAt < 1 || session.ExpiresAt <= session.StartedAt || until < session.StartedAt || until > session.ExpiresAt {
			return 0, errors.New("实际占位分段时间损坏")
		}
		a, b := max(session.StartedAt, start), min(until, end)
		if b <= a {
			continue
		}
		if a < lastEnd {
			return 0, errors.New("同角色实际占位分段重叠")
		}
		lastEnd = b
		if total > math.MaxInt64-(b-a) {
			return 0, errors.New("实际占位时长溢出")
		}
		total += b - a
	}
	return total, nil
}

// p11GrabAt按实际空位或已占位来源处理，独立19证书未覆盖的输入保持整笔回滚。
func (s *GameService) p11GrabAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply proto.Message
	operation := ""
	switch request.(type) {
	case *pb.CSGetGrabData:
		reply = &pb.SCGetGrabData{}
	case *pb.CSGetTimeRewardInfo:
		reply = &pb.SCGetTimeRewardInfo{}
	case *pb.CSGrab:
		reply = &pb.SCGrab{}
		operation = "grab-empty"
	case *pb.CSBuyGrab:
		reply = &pb.SCBuyGrab{}
		operation = "grab-buy"
	case *pb.CSGiveUpGrab:
		reply = &pb.SCGiveUpGrab{}
		operation = "grab-giveup"
	case *pb.CSGetTimeReward:
		reply = &pb.SCGetTimeReward{}
		operation = "grab-time"
	default:
		return nil, errors.New(constUtil.P11GrabClosed)
	}
	// 在事务前只观察锁集合；事务内第一次读取直接锁真实profiles，避免旧快照读取并发回执。
	var observedOwner int64
	if grab, ok := request.(*pb.CSGrab); ok {
		var previous db.P11ArenaReceipt
		hint := db.GetIns().WithContext(ctx).First(&previous, "player_id=? AND operation=? AND stamp=?", id, operation, stamp).Error
		if hint != nil && !errors.Is(hint, gorm.ErrRecordNotFound) {
			return nil, hint
		}
		if errors.Is(hint, gorm.ErrRecordNotFound) {
			var observed db.P11GrabSlot
			if err := db.GetIns().WithContext(ctx).First(&observed, "position=?", grab.GetIndex()).Error; err != nil {
				return nil, err
			}
			if observed.OwnerID != nil {
				observedOwner = *observed.OwnerID
			}
		}
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		var defender *db.Profile
		if observedOwner > 0 && observedOwner != id {
			var locked []db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []int64{id, observedOwner}).Order("id").Find(&locked).Error; err != nil {
				return err
			}
			for i := range locked {
				if locked[i].Id == id {
					profile = locked[i]
				}
				if locked[i].Id == observedOwner {
					defender = &locked[i]
				}
			}
			if profile.Id != id || defender == nil {
				return errors.New(constUtil.P11GrabFightConflict)
			}
		} else if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		raw, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		if operation != "" {
			if stamp <= 0 {
				return errors.New("原奖杯变更缺少HTTP回执时间戳")
			}
			var receipt db.P11ArenaReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, operation, stamp).Error
			if err == nil {
				if receipt.RequestHash != p6Digest(raw) {
					return errors.New("原奖杯回执请求冲突")
				}
				if err = proto.Unmarshal(receipt.Response, reply); err != nil {
					return err
				}
				if old, ok := reply.(*pb.SCGrab); ok && old.AttPlayer != nil {
					var settled db.P11GrabFightSettlement
					if err = tx.First(&settled, "player_id=? AND stamp=?", id, stamp).Error; err != nil {
						return err
					}
					verified, err := s.p11ReadGrabFightReceipt(tx, &settled)
					if err != nil {
						return err
					}
					if settled.RequestHash != p6Digest(raw) || !proto.Equal(verified, old) {
						return errors.New(constUtil.P11GrabFightConflict)
					}
					reply = verified
				}
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		cfg, _, err := p11GrabConfig(tx)
		if err != nil {
			return err
		}
		if err = s.PrepareP11GrabRandom(tx, now); err != nil {
			return err
		}
		if err = p11GrabExpire(tx, now); err != nil {
			return err
		}
		daily, err := p11GrabDaily(tx, id, now, cfg)
		if err != nil {
			return err
		}
		var rewards []p11GrabReward
		if err = json.Unmarshal([]byte(daily.RewardsJSON), &rewards); err != nil {
			return err
		}
		switch b := request.(type) {
		case *pb.CSGetGrabData:
			var slots []db.P11GrabSlot
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("position").Find(&slots).Error; err != nil {
				return err
			}
			if len(slots) != 18 {
				return errors.New("原三个场18个位置不完整")
			}
			out := &pb.SCGetGrabData{GrabTimes: proto.Int32(cfg.MaxTimes + daily.Bought - daily.Used), LastCount: proto.Int32(cfg.BuyMax - daily.Bought)}
			for _, slot := range slots {
				frame, err := p11GrabFrame(tx, slot)
				if err != nil {
					return err
				}
				out.GrabData = append(out.GrabData, frame)
			}
			reply = out
		case *pb.CSGetTimeRewardInfo:
			elapsed, err := p11GrabMillis(tx, id, now)
			if err != nil {
				return err
			}
			out := &pb.SCGetTimeRewardInfo{MyGrabMinutes: proto.Int32(int32(elapsed / int64(time.Minute/time.Millisecond)))}
			var claims []db.P11GrabRewardClaim
			if err = tx.Where("player_id=? AND day=?", id, daily.Day).Order("hours").Find(&claims).Error; err != nil {
				return err
			}
			claimed := map[int64]bool{}
			for _, claim := range claims {
				claimed[claim.Hours] = true
			}
			// 原UpdateList把rewardTime.Contains(hour)直接交给getBtn，可领列表不包含已领档。
			for _, reward := range rewards {
				if cfg.TimeRewardEnabled && !claimed[reward.Hours] && elapsed >= reward.Hours*int64(time.Hour/time.Millisecond) {
					out.RewardTime = append(out.RewardTime, int32(reward.Hours))
				}
			}
			reply = out
		case *pb.CSGrab:
			if !cfg.Enabled {
				return errors.New(constUtil.P11GrabClosed)
			}
			if now.UnixMilli() <= 0 || now.UnixMilli() > math.MaxInt64-cfg.MaxMillis {
				return errors.New("实际占位开始时间无效")
			}
			level, err := s.p10Setting("resPlunderLevelLimit")
			if err != nil {
				return err
			}
			if p.GetLevel() < level {
				return errors.New("原奖杯争夺30级资格未满足")
			}
			if _, _, err = s.p11ArenaProjection(&profile, tx); err != nil {
				return err
			}
			var mine []db.P11GrabSlot
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_id=?", id).Find(&mine).Error; err != nil {
				return err
			}
			if len(mine) > 0 {
				return errors.New("原客户端要求先放弃已有占位")
			}
			var slot db.P11GrabSlot
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&slot, "position=?", b.GetIndex()).Error; err != nil {
				return err
			}
			// 当前归属必须与预观察的锁集合一致；过期或被第三人替换时拒绝并回滚，不转换收费分支。
			if slot.OwnerID != nil {
				if defender == nil || observedOwner != *slot.OwnerID || defender.Id != *slot.OwnerID {
					return errors.New(constUtil.P11GrabFightConflict)
				}
				out, err := s.p11GrabFightLocked(ctx, tx, &profile, defender, p, g, cfg, &slot, daily, b, stamp, now)
				if err != nil {
					return err
				}
				reply = out
				break
			}
			if observedOwner != 0 {
				return errors.New(constUtil.P11GrabFightConflict)
			}
			if cfg.MaxTimes+daily.Bought-daily.Used < 1 {
				return errors.New("原抢占剩余次数不足")
			}
			cost := &pb.BaseResource{Id: proto.String("energy"), Num: proto.Int32(cfg.Energy)}
			if p.GetEnergy() < cfg.Energy {
				return errors.New("原空位抢占耐力不足")
			}
			p.Energy = proto.Int32(p.GetEnergy() - cfg.Energy)
			owner := id
			slot.OwnerID = &owner
			slot.SessionID = uuid.NewString()
			slot.StartedAt = now.UnixMilli()
			slot.ExpiresAt = now.UnixMilli() + cfg.MaxMillis
			session := db.P11GrabSession{ID: slot.SessionID, PlayerID: id, Position: slot.Position, StartedAt: slot.StartedAt, ExpiresAt: slot.ExpiresAt}
			if err = tx.Create(&session).Error; err != nil {
				return err
			}
			// 当前读锁和带空位条件的写入共同防止REPEATABLE READ旧快照覆盖其他真实归属。
			occupied := tx.Model(&db.P11GrabSlot{}).Where("position=? AND owner_id IS NULL", slot.Position).Updates(map[string]any{"owner_id": id, "session_id": slot.SessionID, "started_at": slot.StartedAt, "expires_at": slot.ExpiresAt})
			if occupied.Error != nil {
				return occupied.Error
			}
			if occupied.RowsAffected != 1 {
				return errors.New("原空位已被其他真实角色占领")
			}
			daily.Used++
			frame, err := p11GrabFrame(tx, slot)
			if err != nil {
				return err
			}
			reply = &pb.SCGrab{GrabData: frame, CurrEnergy: proto.Int32(p.GetEnergy()), GrabTimes: proto.Int32(cfg.MaxTimes + daily.Bought - daily.Used)}
			if s.P1Ready {
				source := "p11-grab-empty:" + session.ID
				if err = s.RecordP1Cost(tx, id, source, []*pb.BaseResource{cost}, now); err != nil {
					return err
				}
				if err = s.RecordP1Event(tx, id, source, "grab_empty_occupied", 1, now); err != nil {
					return err
				}
			}
		case *pb.CSGiveUpGrab:
			var slot db.P11GrabSlot
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&slot, "position=?", b.GetIndex()).Error; err != nil {
				return err
			}
			if slot.OwnerID == nil || *slot.OwnerID != id {
				return errors.New("原放弃位置不属于当前角色")
			}
			if err = p11GrabClose(tx, &slot, now.UnixMilli(), "giveup"); err != nil {
				return err
			}
			frame, err := p11GrabFrame(tx, slot)
			if err != nil {
				return err
			}
			reply = &pb.SCGiveUpGrab{GrabData: frame}
		case *pb.CSBuyGrab:
			if !cfg.Enabled {
				return errors.New(constUtil.P11GrabClosed)
			}
			count := b.GetCount()
			remaining := cfg.MaxTimes + daily.Bought - daily.Used
			if count < 1 || count > cfg.BuyMax-daily.Bought || remaining+count > cfg.MaxTimes {
				return errors.New("原买次日上限或当前3次容量未满足")
			}
			cost := &pb.BaseResource{Id: proto.String("diamond"), Num: proto.Int32(count * cfg.BuyPrice)}
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
			daily.Bought += count
			reply = &pb.SCBuyGrab{LastCount: proto.Int32(cfg.BuyMax - daily.Bought), Cost: []*pb.BaseResource{cost}, CurrCount: proto.Int32(cfg.MaxTimes + daily.Bought - daily.Used)}
			if s.P1Ready {
				source := fmt.Sprintf("p11-grab-buy:%d:%d", id, stamp)
				if err = s.RecordP1Cost(tx, id, source, []*pb.BaseResource{cost}, now); err != nil {
					return err
				}
			}
		case *pb.CSGetTimeReward:
			if !cfg.TimeRewardEnabled {
				return errors.New("原奖杯累计时间奖本批尚未开放")
			}
			var selected *p11GrabReward
			for i := range rewards {
				if rewards[i].Hours == int64(b.GetTime()) {
					selected = &rewards[i]
					break
				}
			}
			if selected == nil {
				return errors.New("原占位时间奖档不存在")
			}
			elapsed, err := p11GrabMillis(tx, id, now)
			if err != nil {
				return err
			}
			if elapsed < selected.Hours*int64(time.Hour/time.Millisecond) {
				return errors.New("实际占位时间未满足原奖档")
			}
			var claim db.P11GrabRewardClaim
			err = tx.First(&claim, "player_id=? AND day=? AND hours=?", id, daily.Day, selected.Hours).Error
			if err == nil {
				reply = &pb.SCGetTimeReward{Time: b.Time, Add: &pb.ResultObject{}}
			} else {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				gain, err := s.GrantFixedReward(p, g, selected.Reward)
				if err != nil {
					return err
				}
				reply = &pb.SCGetTimeReward{Time: b.Time, Add: gain}
				blob, err := proto.Marshal(reply)
				if err != nil {
					return err
				}
				claim = db.P11GrabRewardClaim{PlayerID: id, Day: daily.Day, Hours: selected.Hours, Millis: elapsed, Reward: selected.Reward, RewardHash: daily.RewardsHash, Response: blob, ClaimedAt: now.UnixMilli()}
				if err = tx.Create(&claim).Error; err != nil {
					return err
				}
				if s.P1Ready {
					source := fmt.Sprintf("p11-grab-time:%d:%s:%d", id, daily.Day, selected.Hours)
					if err = s.RecordP1Event(tx, id, source, "grab_time_reward", 1, now); err != nil {
						return err
					}
				}
			}
		}
		if err = tx.Save(daily).Error; err != nil {
			return err
		}
		if err = saveProfile(tx, &profile, p, g); err != nil {
			return err
		}
		if operation != "" {
			blob, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			return tx.Create(&db.P11ArenaReceipt{PlayerID: id, Operation: operation, Stamp: stamp, RequestHash: p6Digest(raw), Response: blob, BatchID: daily.Day, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}
