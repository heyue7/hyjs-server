package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP10Channel 创建透明原频道域；新消息默认关闭，读取实际历史保持可用。
func (s *GameService) InitializeP10Channel(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10ChannelPolicy{}, &db.P10ChannelChat{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P10ChannelPolicy{ID: 1, Source: constUtil.P10ChannelSource}).Error
}

func (s *GameService) p10ChannelOnline(tx *gorm.DB, id int64) bool {
	value, ok := p10ClientSinks.Load(p10ClientKey{s.p10ClientOwner(), id})
	if !ok {
		return false
	}
	var profile db.Profile
	return tx.First(&profile, id).Error == nil && subtle.ConstantTimeCompare(profile.Session, value.(*p10ClientSink).session) == 1
}

// p10ChannelScope 从真实在线公共位置确定野外频道，客户端不提供scene或line。
func (s *GameService) p10ChannelScope(tx *gorm.DB, id int64, kind int32) (string, int32, bool) {
	if kind == constUtil.P13GuildChatType {
		guild, _, err := P13GuildScope(tx, id)
		return guild, 0, err == nil && guild != ""
	}
	if kind == constUtil.P10WorldChatType {
		return "", 0, s.p10ChannelOnline(tx, id)
	}
	var own db.P10ScenePresence
	if tx.First(&own, "player_id=?", id).Error != nil {
		return "", 0, false
	}
	// 原野外频道随当前已开放地图分房，雪原复用森林的在线资格、分线与历史规则。
	// 仍由p10SceneLive重新核角色位置和转场握手，不能仅凭旧presence进入频道。
	if own.Scene != "field_001" && own.Scene != "field_002" {
		return "", 0, false
	}
	_, ok := s.p10SceneLive(tx, own)
	return own.Scene, own.Line, ok
}

func p10ChannelFrame(row db.P10ChannelChat) (*pb.SCChatTcp, error) {
	info := &pb.SimplePlayerInfo{}
	if err := proto.Unmarshal(row.SenderInfo, info); err != nil {
		return nil, errors.New("公共文字发送人快照损坏")
	}
	return &pb.SCChatTcp{ChatType: proto.Int32(row.ChatType), Content: proto.String(row.Content), Time: proto.Int64(row.CreatedAt), Play: info, IsAudio: proto.Int32(0)}, nil
}

func (s *GameService) p10ChannelHistory(tx *gorm.DB, id int64, kind int32) ([]*pb.ChatInfo, error) {
	limit, err := s.p10Setting("MaxChatNumber")
	if err != nil || limit < 1 || limit > 500 {
		return nil, errors.New("原公共聊天历史上限无效")
	}
	scene, line, valid := s.p10ChannelScope(tx, id, kind)
	// 世界历史属于本地服。没有在线野外资格时原初始化读取返回空实际可见集。
	if (kind == constUtil.P10FieldChatType || kind == constUtil.P13GuildChatType) && !valid {
		return nil, nil
	}
	var rows []db.P10ChannelChat
	if err = tx.Where("chat_type=? AND scene=? AND line=?", kind, scene, line).Where("NOT EXISTS (SELECT 1 FROM p10_social_blacks b WHERE (b.owner_id=? AND b.target_id=p10_channel_chats.sender_id) OR (b.owner_id=p10_channel_chats.sender_id AND b.target_id=?))", id, id).Order("id DESC").Limit(int(limit)).Find(&rows).Error; err != nil {
		return nil, err
	}
	list := []*pb.ChatInfo{}
	for _, row := range rows {
		blocked, err := p10Blocked(tx, id, row.SenderID)
		if err != nil {
			return nil, err
		}
		if blocked {
			continue
		}
		frame, err := p10ChannelFrame(row)
		if err != nil {
			return nil, err
		}
		list = append(list, &pb.ChatInfo{ChatType: frame.ChatType, Content: frame.Content, Time: frame.Time, Play: frame.Play, IsAudio: proto.Int32(0)})
		if len(list) >= int(limit) {
			break
		}
	}
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, nil
}

// p10ChannelAt 冻结真实发送资格、节流、原消息帧与回执，提交后向仍有权限的当前会话推送。
func (s *GameService) p10ChannelAt(ctx context.Context, id int64, b *pb.CSChatTcp, stamp int64, now time.Time) ([]proto.Message, error) {
	if err := validateChatContent(b.GetContent()); err != nil {
		return nil, err
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	var reply pb.SCChatTcp
	var recipients []int64
	var scene string
	var line int32
	fresh := false
	kind := b.GetChatType()
	domain := fmt.Sprintf("channel-chat-%d", kind)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		players, err := p10LockProfiles(tx, id, id)
		if err != nil {
			return err
		}
		if stamp > 0 {
			if found, err := p10ReadReceipt(tx, id, domain, stamp, b, &reply); err != nil || found {
				return err
			}
		}
		var cfg db.P10ChannelPolicy
		if err = tx.First(&cfg, 1).Error; err != nil {
			return err
		}
		if kind == constUtil.P10WorldChatType && !cfg.WorldEnabled || kind == constUtil.P10FieldChatType && !cfg.FieldEnabled {
			return errors.New(constUtil.P10ChannelClosed)
		}
		if kind != constUtil.P10WorldChatType && kind != constUtil.P10FieldChatType && kind != constUtil.P13GuildChatType {
			return errors.New("原公共频道类型未恢复")
		}
		if kind == constUtil.P13GuildChatType {
			enabled, err := P13GuildChannelEnabled(tx)
			if err != nil {
				return err
			}
			if !enabled {
				return errors.New("公会文字频道本批尚未开放")
			}
			if !s.p10ChannelOnline(tx, id) {
				return errors.New("请先完成公会真实TCP登录")
			}
		}
		if b.GetIsAudio() != 0 || b.GetFileName() != "" || b.GetDownLoadUrl() != "" || b.GetAudioTime() != 0 || b.GetFileSize() != 0 {
			return errors.New("本批公共频道仅恢复原文字")
		}
		if !p10Text(b.GetContent(), 40) {
			return errors.New("公共文字超过原40个UTF16单位或为空")
		}
		minLevel, err := s.p10Setting("ChatLevel")
		if err != nil {
			return err
		}
		minVIP, err := s.p10Setting("ChatVipLevel")
		if err != nil {
			return err
		}
		if players[id].GetLevel() < minLevel && players[id].GetVip() < minVIP {
			return errors.New("聊天需原20级或VIP2")
		}
		valid := false
		scene, line, valid = s.p10ChannelScope(tx, id, kind)
		if !valid {
			return errors.New("请先完成该公共频道真实TCP场景登录")
		}
		if kind == constUtil.P10FieldChatType {
			cfg, err := p10SceneConfig(tx)
			if err != nil {
				return err
			}
			if cfg == nil {
				return errors.New(constUtil.P10SceneClosed)
			}
		}
		interval, err := s.p10Setting("ChatInterval")
		if err != nil || interval < 1 || interval > math.MaxInt32/1000 {
			return errors.New("原聊天间隔无效")
		}
		state := db.P10SocialChatState{PlayerID: id}
		err = tx.First(&state, "player_id=?", id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if state.LastSendAt > 0 && now.UnixMilli()-state.LastSendAt < int64(interval)*1000 {
			// 原TCP无回执时间戳，同频道同内容同场景在节流内重传复用原帧；其他请求拒绝。
			if stamp == 0 {
				var old db.P10ChannelChat
				if tx.Where("sender_id=?", id).Order("id DESC").First(&old).Error == nil && old.CreatedAt == state.LastSendAt && old.ChatType == kind && old.Scene == scene && old.Line == line && old.Content == b.GetContent() {
					frame, err := p10ChannelFrame(old)
					if err != nil {
						return err
					}
					proto.Reset(&reply)
					proto.Merge(&reply, frame)
					return nil
				}
			}
			return errors.New("原聊天发送间隔未到")
		}
		info, err := s.p10Player(tx, id)
		if err != nil {
			return err
		}
		raw, err := proto.Marshal(info)
		if err != nil {
			return err
		}
		row := db.P10ChannelChat{SenderID: id, ChatType: kind, Scene: scene, Line: line, Content: b.GetContent(), CreatedAt: now.UnixMilli(), SenderInfo: raw}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		state.LastSendAt = row.CreatedAt
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		frame, err := p10ChannelFrame(row)
		if err != nil {
			return err
		}
		proto.Reset(&reply)
		proto.Merge(&reply, frame)
		// 只快照当前真实会话；发送后权限仍逐个校验，离线查询使用持久化原历史。
		p10ClientSinks.Range(func(key, value any) bool {
			k := key.(p10ClientKey)
			if k.service == s.p10ClientOwner() && k.playerID != id {
				recipients = append(recipients, k.playerID)
			}
			return true
		})
		sort.Slice(recipients, func(i, j int) bool { return recipients[i] < recipients[j] })
		fresh = true
		if stamp > 0 {
			return p10SaveReceipt(tx, id, domain, stamp, b, &reply)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		tx := db.GetIns().WithContext(ctx)
		for _, target := range recipients {
			currentScene, currentLine, valid := s.p10ChannelScope(tx, target, kind)
			if !valid || (kind == constUtil.P10FieldChatType || kind == constUtil.P13GuildChatType) && (currentScene != scene || currentLine != line) {
				continue
			}
			blocked, err := p10Blocked(tx, id, target)
			if err != nil || blocked {
				continue
			}
			s.publishP10Private(ctx, target, &reply)
		}
	}
	return []proto.Message{&reply}, nil
}

func (s *GameService) DispatchP10Channel(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSChatTcp:
		if b.GetChatType() != constUtil.P10WorldChatType && b.GetChatType() != constUtil.P10FieldChatType && b.GetChatType() != constUtil.P13GuildChatType {
			return nil, false, nil
		}
		result, err := s.p10ChannelAt(ctx, id, b, stamp, time.Now())
		return result, true, err
	case *pb.CSChatCache:
		if b.GetChatType() != 0 && b.GetChatType() != constUtil.P10WorldChatType && b.GetChatType() != constUtil.P10FieldChatType && b.GetChatType() != constUtil.P13GuildChatType {
			return nil, false, nil
		}
		p10SceneMutation.Lock()
		defer p10SceneMutation.Unlock()
		if b.GetChatType() == 0 {
			result, _, err := s.p10Social(ctx, id, b, stamp, time.Now())
			if err != nil {
				return nil, true, err
			}
			list := result[0].(*pb.SCChatCache)
			for _, kind := range []int32{constUtil.P10WorldChatType, constUtil.P10FieldChatType, constUtil.P13GuildChatType} {
				history, err := s.p10ChannelHistory(db.GetIns().WithContext(ctx), id, kind)
				if err != nil {
					return nil, true, err
				}
				list.ChatList = append(list.ChatList, history...)
			}
			sort.SliceStable(list.ChatList, func(i, j int) bool { return list.ChatList[i].GetTime() < list.ChatList[j].GetTime() })
			limit, err := s.p10Setting("MaxChatNumber")
			if err != nil || limit < 1 {
				return nil, true, errors.New("原聊天历史上限无效")
			}
			if len(list.ChatList) > int(limit) {
				list.ChatList = list.ChatList[len(list.ChatList)-int(limit):]
			}
			return []proto.Message{list}, true, nil
		}
		list, err := s.p10ChannelHistory(db.GetIns().WithContext(ctx), id, b.GetChatType())
		return []proto.Message{&pb.SCChatCache{ChatList: list}}, true, err
	case *pb.CSSceneChatCache:
		p10SceneMutation.Lock()
		defer p10SceneMutation.Unlock()
		list, err := s.p10ChannelHistory(db.GetIns().WithContext(ctx), id, constUtil.P10FieldChatType)
		return []proto.Message{&pb.SCSceneChatCache{ChatList: list}}, true, err
	}
	return nil, false, nil
}
