package service

import (
	"bytes"
	"context"
	"errors"
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

// InitializeP13UnionFightFocus 仅初始化持久集结点，不发布未知机关/资源或比赛结果。
func (s *GameService) InitializeP13UnionFightFocus(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightFocus{})
}

func p13UFFocusPoint(row *db.P13UnionFightFocus) (*pb.PointNew, error) {
	if p6Digest(row.Point) != row.PointHash {
		return nil, errors.New("公会战原集结点快照损坏")
	}
	point := &pb.PointNew{}
	if err := proto.Unmarshal(row.Point, point); err != nil {
		return nil, err
	}
	return point, nil
}

// p13UFFocusActive 指挥者退出本场/本会或不再是当前会长时，旧集结令不继续授权。
func p13UFFocusActive(tx *gorm.DB, row *db.P13UnionFightFocus, now time.Time) (bool, error) {
	if row.ClearedAt > 0 || row.ExpiresAt <= now.UnixMilli() {
		return false, nil
	}
	var match db.P13UnionFightMatch
	if err := tx.First(&match, "id=?", row.MatchID).Error; err != nil {
		return false, err
	}
	if now.UnixMilli() >= match.EndAt {
		return false, nil
	}
	member, err := p13GuildMember(tx, row.LeaderID)
	if err != nil {
		return false, err
	}
	if member.GuildID != row.GuildID || member.Role != 2 {
		return false, nil
	}
	var count int64
	if err = tx.Model(&db.P13UnionFightFieldPlayer{}).Where("match_id=? AND player_id=? AND guild_id=? AND left_at=0", row.MatchID, row.LeaderID, row.GuildID).Count(&count).Error; err != nil {
		return false, err
	}
	return count == 1, nil
}

func (s *GameService) p13UFFocusBroadcast(ctx context.Context, scope, guild string, exclude int64, message proto.Message) {
	rows, _, err := s.p13UFFieldPeers(db.GetIns().WithContext(ctx), scope, exclude)
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.GuildID == guild {
			s.publishP10Private(ctx, row.PlayerID, message)
		}
	}
}

func (s *GameService) p13UFFocus(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply proto.Message
	var scope, guild string
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		scope, guild = match.ID, actor.GuildID
		var row db.P13UnionFightFocus
		err = tx.First(&row, "match_id=? AND guild_id=?", scope, guild).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		exists := err == nil
		if body, mutation := request.(*pb.CSUFLeaderFocus); mutation {
			if frame.GetUnionAuthority() != 2 {
				return errors.New("原只有冻结且当前会长可设置集结点")
			}
			if actor.Hooked {
				return errors.New("原挂机状态不能设置集结点")
			}
			if now.UnixMilli() >= match.EndAt {
				return errors.New("原本场已结束，不能设置集结点")
			}
			point, err := proto.Marshal(body.GetPos())
			if err != nil {
				return err
			}
			// 原PointNew零坐标可编码为空字节；保留非nil空BLOB，不能把合法原点写成SQL NULL。
			point = append([]byte{}, point...)
			// 绑定比赛与原字节；HTTP精确stamp跨比赛/不同点不能读取旧回执。
			raw := append(append([]byte(scope), 0), point...)
			if stamp > 0 {
				var receipt db.P13GuildReceipt
				err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, constUtil.P13UnionFightFocusOperation, stamp).Error
				if err == nil {
					if !bytes.Equal(receipt.Request, raw) {
						return errors.New("公会战集结请求时间戳冲突")
					}
					stored := &pb.SCUFLeaderFocus{}
					if err = proto.Unmarshal(receipt.Response, stored); err != nil {
						return err
					}
					reply = stored
					return nil
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			if exists && now.UnixMilli() < row.ReadyAt {
				// 原TCP没有时间戳；同指挥者相同原点重发只回原到期时间，不延长集结/冷却。
				if stamp == 0 && row.LeaderID == id && row.ClearedAt == 0 && bytes.Equal(row.Point, point) {
					reply = &pb.SCUFLeaderFocus{Pos: []*pb.PointNew{proto.Clone(body.GetPos()).(*pb.PointNew)}, LastTime: proto.Int64(row.ReadyAt)}
					return nil
				}
				return errors.New("原会长集结仍在冷却")
			}
			original, err := s.p13UFFieldOriginal(match, tx)
			if err != nil {
				return err
			}
			read := func(name string) (int64, error) {
				value, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", name)["NameValue"], 64)
				if err != nil || value <= 0 || value > 86400 || math.IsNaN(value) || math.IsInf(value, 0) {
					return 0, errors.New("原集结持续/冷却配置缺失")
				}
				return int64(value * 1000), nil
			}
			duration, err := read("UFLeaderFocusLastTime")
			if err != nil {
				return err
			}
			cooldown, err := read("UFLeaderFocusCDTime")
			if err != nil {
				return err
			}
			row = db.P13UnionFightFocus{MatchID: scope, GuildID: guild, LeaderID: id, Point: point, PointHash: p6Digest(point), PublishedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + duration, ReadyAt: now.UnixMilli() + cooldown, Source: constUtil.P13UnionFightFocusSource}
			if err = tx.Save(&row).Error; err != nil {
				return err
			}
			reply = &pb.SCUFLeaderFocus{Pos: []*pb.PointNew{proto.Clone(body.GetPos()).(*pb.PointNew)}, LastTime: proto.Int64(row.ReadyAt)}
			fresh = true
			if stamp > 0 {
				encoded, err := proto.Marshal(reply)
				if err != nil {
					return err
				}
				return tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: constUtil.P13UnionFightFocusOperation, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error
			}
			return nil
		}
		var positions []*pb.PointNew
		ready := int64(0)
		if exists {
			active, err := p13UFFocusActive(tx, &row, now)
			if err != nil {
				return err
			}
			if active {
				point, err := p13UFFocusPoint(&row)
				if err != nil {
					return err
				}
				positions = append(positions, point)
			}
			if row.ReadyAt > now.UnixMilli() {
				ready = row.ReadyAt
			}
		}
		if _, info := request.(*pb.CSUFLeaderFocusInfo); info {
			reply = &pb.SCUFLeaderFocus{Pos: positions, LastTime: proto.Int64(ready)}
		} else {
			reply = &pb.SCUFFocusPointInfo{Pos: positions}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p13UFFocusBroadcast(ctx, scope, guild, id, reply)
	}
	return []proto.Message{reply}, nil
}

// TickP13UnionFightFocus 把实际到期/资格撤销的集结令标记为已清除并推送原空点帧，防每秒重复通知。
func (s *GameService) TickP13UnionFightFocus(ctx context.Context, now time.Time) error {
	var rows []db.P13UnionFightFocus
	if err := db.GetIns().WithContext(ctx).Where("cleared_at=0").Order("match_id,guild_id").Find(&rows).Error; err != nil {
		return err
	}
	scopes := make([]string, 0, len(rows))
	for _, candidate := range rows {
		scopes = append(scopes, candidate.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	for _, row := range rows {
		if clocks.paused(row.MatchID) {
			continue
		}
		cleared := false
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			// 策略关闭也清除旧指令，不能把业务关闭当错误令整个游戏服务退出。
			var management db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&management, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, row.MatchID); err != nil {
				return err
			} else if closed {
				return nil
			}
			var field db.P13UnionFightFieldPolicy
			if err := tx.First(&field, 1).Error; err != nil {
				return err
			}
			var enrollment db.P13UnionFightPolicy
			if err := tx.First(&enrollment, 1).Error; err != nil {
				return err
			}
			var current db.P13UnionFightFocus
			if err := tx.First(&current, "match_id=? AND guild_id=?", row.MatchID, row.GuildID).Error; err != nil {
				return err
			}
			active, err := p13UFFocusActive(tx, &current, now)
			if err != nil {
				return err
			}
			active = active && clocks.terminal[current.MatchID] == 0 && management.ManagementEnabled && field.Enabled && enrollment.EnrollmentEnabled && enrollment.MatchEnabled
			if active || current.ClearedAt > 0 {
				return nil
			}
			if err = tx.Model(&current).Update("cleared_at", now.UnixMilli()).Error; err != nil {
				return err
			}
			cleared = true
			return nil
		})
		if err != nil {
			return err
		}
		if cleared {
			if err := p13UFPublishBeforeAbort(ctx, row.MatchID, func() {
				s.p13UFFocusBroadcast(ctx, row.MatchID, row.GuildID, 0, &pb.SCUFFocusPointInfo{})
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
