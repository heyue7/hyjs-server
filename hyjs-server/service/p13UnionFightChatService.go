package service

import (
	"bytes"
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP13UnionFightChat 将文字与指挥分别关闭；开启仍须实际场内身份。
func (s *GameService) InitializeP13UnionFightChat(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightChatPolicy{}, &db.P13UnionFightChat{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightChatPolicy{ID: 1, TalkCooldownMillis: constUtil.P13UnionFightTalkCooldownMillis, Source: constUtil.P13UnionFightChatSource}).Error
}

func p13UFChatFrame(row *db.P13UnionFightChat) (proto.Message, error) {
	if p6Digest(row.Frame) != row.FrameHash {
		return nil, errors.New("公会战交流原帧损坏")
	}
	var frame proto.Message = &pb.SCChatTcp{}
	if row.Kind == 1 {
		frame = &pb.SCUFTalk{}
	} else if row.Kind != 0 {
		return nil, errors.New("公会战交流类型损坏")
	}
	if err := proto.Unmarshal(row.Frame, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

// p13UFChatIdentity 在Profile行锁下重核实际场景、首发与当前公会，回执也不绕过撤权。
func (s *GameService) p13UFChatIdentity(tx *gorm.DB, id int64, now time.Time) (*db.P13UnionFightFieldPlayer, *db.P13UnionFightMatch, *pb.InstUFPlayer, error) {
	var row db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
		return nil, nil, nil, err
	}
	_, game, err := decodeProfile(&row)
	if err != nil {
		return nil, nil, nil, err
	}
	if game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		return nil, nil, nil, errors.New("角色实际位置不在公会战实例")
	}
	actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
	return actor, match, frame, err
}

// p13UFChatHistory 仅返回当前比赛本会的实际文字，按原MaxChatNumber截断并保留时间顺序。
func (s *GameService) p13UFChatHistory(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	var list []*pb.ChatInfo
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, _, err := s.p13UFChatIdentity(tx, id, now)
		if err != nil {
			return err
		}
		if now.UnixMilli() >= match.EndAt {
			return errors.New("原本场已结束，公会战交流历史访问已撤销")
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		limit, err := original.p10Setting("MaxChatNumber")
		if err != nil || limit < 1 || limit > 500 {
			return errors.New("原公会战聊天历史上限无效")
		}
		var rows []db.P13UnionFightChat
		if err = tx.Where("match_id=? AND guild_id=? AND kind=0", match.ID, actor.GuildID).Where("NOT EXISTS (SELECT 1 FROM p10_social_blacks b WHERE (b.owner_id=? AND b.target_id=p13_union_fight_chats.sender_id) OR (b.owner_id=p13_union_fight_chats.sender_id AND b.target_id=?))", id, id).Order("id DESC").Limit(int(limit)).Find(&rows).Error; err != nil {
			return err
		}
		for i := len(rows) - 1; i >= 0; i-- {
			message, err := p13UFChatFrame(&rows[i])
			if err != nil {
				return err
			}
			frame := message.(*pb.SCChatTcp)
			list = append(list, &pb.ChatInfo{ChatType: frame.ChatType, Content: frame.Content, Time: frame.Time, Play: frame.Play, IsAudio: proto.Int32(0)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCUFChatCache{ChatList: list}}, nil
}

// p13UFChat 将原文字/固定指挥与当前比赛绑定，事务提交后只向仍在线的本会首发推送。
func (s *GameService) p13UFChat(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	switch body := request.(type) {
	case *pb.CSChatTcp:
		if err := validateChatContent(body.GetContent()); err != nil {
			return nil, err
		}
	case *pb.CSUFTalk:
		if err := validateChatContent(body.GetContent()); err != nil {
			return nil, err
		}
	}
	var reply proto.Message
	var scope, guild string
	kind := int64(0)
	operation := constUtil.P13UnionFightChatOperation
	if _, ok := request.(*pb.CSUFTalk); ok {
		kind = 1
		operation = constUtil.P13UnionFightTalkOperation
	}
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, frame, err := s.p13UFChatIdentity(tx, id, now)
		if err != nil {
			return err
		}
		scope, guild = match.ID, actor.GuildID
		if now.UnixMilli() >= match.EndAt {
			return errors.New("原本场已结束，不能发送公会战交流")
		}
		if !s.p10ChannelOnline(tx, id) {
			return errors.New("请先完成公会战真实TCP登录")
		}
		if kind == 1 && (frame.GetUnionAuthority() == 0 || actor.Hooked) {
			return errors.New("原公会战指挥需要未挂机的冻结且当前会长/副会长")
		}
		raw, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		bound := append(append([]byte(scope), 0), raw...)
		if stamp > 0 {
			var receipt db.P13GuildReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, operation, stamp).Error
			if err == nil {
				if !bytes.Equal(receipt.Request, bound) {
					return errors.New("公会战交流请求时间戳冲突")
				}
				var stored db.P13UnionFightChat
				if err = tx.Where("match_id=? AND guild_id=? AND sender_id=? AND kind=? AND frame=?", scope, guild, id, kind, receipt.Response).First(&stored).Error; err != nil {
					return err
				}
				reply, err = p13UFChatFrame(&stored)
				return err
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		var policy db.P13UnionFightChatPolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if kind == 0 && !policy.TextEnabled || kind == 1 && !policy.TalkEnabled {
			return errors.New("公会战文字/指挥独立策略尚未开放")
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		interval := policy.TalkCooldownMillis
		if kind == 0 {
			body := request.(*pb.CSChatTcp)
			if body.GetChatType() != constUtil.P13UnionFightChatType || body.GetIsAudio() != 0 || body.GetFileName() != "" || body.GetDownLoadUrl() != "" || body.GetAudioTime() != 0 || body.GetFileSize() != 0 {
				return errors.New("本批公会战频道仅恢复原type6文字")
			}
			if !p10Text(body.GetContent(), 40) {
				return errors.New("公会战文字超过原40个UTF16单位或为空")
			}
			var profile db.Profile
			if err = tx.First(&profile, id).Error; err != nil {
				return err
			}
			player, _, err := decodeProfile(&profile)
			if err != nil {
				return err
			}
			level, err := original.p10Setting("ChatLevel")
			if err != nil {
				return err
			}
			vip, err := original.p10Setting("ChatVipLevel")
			if err != nil {
				return err
			}
			if player.GetLevel() < level && player.GetVip() < vip {
				return errors.New("聊天需原20级或VIP2")
			}
			seconds, err := original.p10Setting("ChatInterval")
			if err != nil {
				return err
			}
			interval = int64(seconds) * 1000
		} else {
			body := request.(*pb.CSUFTalk)
			if len(original.Tables["UnionFightTalk"]) != constUtil.P13UnionFightTalkCount {
				return errors.New("本场冻结原18条公会战指挥目录缺失")
			}
			matched := false
			for _, row := range original.Tables["UnionFightTalk"] {
				text := row["localizedContent"]
				if text != "" && body.GetTalkType() == 0 && text == body.GetContent() {
					matched = true
					break
				}
			}
			if !matched {
				return errors.New("公会战指挥必须来自本场冻结原18条内容")
			}
		}
		if interval < 1000 || interval > 86400000 {
			return errors.New("公会战交流间隔配置无效")
		}
		var previous db.P13UnionFightChat
		err = tx.Where("sender_id=? AND kind=?", id, kind).Order("id DESC").First(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		last := previous.CreatedAt
		var state db.P10SocialChatState
		if kind == 0 {
			err = tx.First(&state, "player_id=?", id).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			state.PlayerID = id
			last = max(last, state.LastSendAt)
		}
		sentAt := now
		if ServerGameClockContextBound(ctx) {
			sentAt = time.Now()
		}
		if last > 0 && sentAt.UnixMilli()-last < interval {
			if stamp == 0 && previous.MatchID == scope && previous.GuildID == guild && previous.CreatedAt == last && bytes.Equal(previous.Request, raw) {
				reply, err = p13UFChatFrame(&previous)
				return err
			}
			return errors.New("公会战交流发送间隔未到")
		}
		info, err := s.p10Player(tx, id)
		if err != nil {
			return err
		}
		if kind == 0 {
			reply = &pb.SCChatTcp{ChatType: proto.Int32(constUtil.P13UnionFightChatType), Content: request.(*pb.CSChatTcp).Content, Time: proto.Int64(sentAt.UnixMilli()), Play: info, IsAudio: proto.Int32(0)}
		} else {
			reply = &pb.SCUFTalk{TalkType: proto.Int32(0), Content: request.(*pb.CSUFTalk).Content, Time: proto.Int64(sentAt.UnixMilli()), Play: info}
		}
		encoded, err := proto.Marshal(reply)
		if err != nil {
			return err
		}
		row := db.P13UnionFightChat{MatchID: scope, GuildID: guild, Kind: kind, SenderID: id, CreatedAt: sentAt.UnixMilli(), Request: raw, Frame: encoded, FrameHash: p6Digest(encoded), Source: constUtil.P13UnionFightChatSource}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		if kind == 0 {
			state.LastSendAt = sentAt.UnixMilli()
			if err = tx.Save(&state).Error; err != nil {
				return err
			}
		}
		fresh = true
		if stamp > 0 {
			return tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: operation, Stamp: stamp, Request: bound, Response: encoded, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		rows, _, err := s.p13UFFieldPeers(db.GetIns().WithContext(ctx), scope, id)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.GuildID != guild {
				continue
			}
			if kind == 0 {
				blocked, err := p10Blocked(db.GetIns().WithContext(ctx), id, row.PlayerID)
				if err != nil || blocked {
					continue
				}
			}
			s.publishP10Private(ctx, row.PlayerID, reply)
		}
	}
	return []proto.Message{reply}, nil
}
