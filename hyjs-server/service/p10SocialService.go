package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP10Social(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10SocialPolicy{}, &db.P10SocialRelation{}, &db.P10SocialApply{}, &db.P10SocialBlack{}, &db.P10SocialChat{}, &db.P10SocialChatState{}, &db.P10SocialReceipt{}); err != nil {
		return err
	}
	if err := s.InitializeP10Worship(ctx); err != nil {
		return err
	}
	if err := s.InitializeP10ExpeditionHelp(ctx); err != nil {
		return err
	}
	cfg := db.P10SocialPolicy{ID: constUtil.P10SocialPolicyID, Source: constUtil.P10SocialSource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}
func p10Text(raw string, maxUnits int) bool {
	if !utf8.ValidString(raw) || len(utf16.Encode([]rune(raw))) > maxUnits || strings.TrimSpace(raw) == "" {
		return false
	}
	for _, r := range raw {
		if r < 32 && r != '\n' {
			return false
		}
	}
	return true
}
func (s *GameService) p10Setting(name string) (int32, error) {
	n, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", name)["NameValue"], 10, 32)
	if err != nil || n < 0 {
		return 0, errors.New("原社交配置无效")
	}
	return int32(n), nil
}
func p10Pair(a, b int64) (int64, int64) {
	if a < b {
		return a, b
	}
	return b, a
}
func p10Blocked(tx *gorm.DB, a, b int64) (bool, error) {
	var count int64
	err := tx.Model(&db.P10SocialBlack{}).Where("(owner_id=? AND target_id=?) OR (owner_id=? AND target_id=?)", a, b, b, a).Count(&count).Error
	return count > 0, err
}
func p10Friends(tx *gorm.DB, a, b int64) (bool, error) {
	lo, hi := p10Pair(a, b)
	var count int64
	err := tx.Model(&db.P10SocialRelation{}).Where("low_id=? AND high_id=?", lo, hi).Count(&count).Error
	return count > 0, err
}
func (s *GameService) p10Player(tx *gorm.DB, id int64) (*pb.SimplePlayerInfo, error) {
	var row db.Profile
	if err := tx.First(&row, id).Error; err != nil {
		return nil, err
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	if err = s.PrepareP5GuildPrayerProperties(tx.Statement.Context, tx, p, g, p13UFNow(tx.Statement.Context)); err != nil {
		return nil, err
	}
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return nil, err
	}
	heroID := ""
	for _, w := range g.Warriors {
		if w.GetGuid() == p.GetHeroWarId() {
			heroID = w.GetId()
			break
		}
	}
	return &pb.SimplePlayerInfo{PlayerId: proto.Int64(id), ServerId: proto.Int32(int32(s.OriginalServerID())), Level: p.Level, PlayerName: p.Name, FightPower: proto.Int32(power), HeroWarId: proto.String(heroID)}, nil
}
func p10LockProfiles(tx *gorm.DB, a, b int64) (map[int64]*pb.Player, error) {
	lo, hi := p10Pair(a, b)
	ids := []int64{lo}
	if hi != lo {
		ids = append(ids, hi)
	}
	players := map[int64]*pb.Player{}
	for _, id := range ids {
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return nil, err
		}
		p, _, err := decodeProfile(&row)
		if err != nil {
			return nil, err
		}
		players[id] = p
	}
	return players, nil
}
func p10ReadReceipt(tx *gorm.DB, id int64, domain string, stamp int64, request, response proto.Message) (bool, error) {
	if stamp < 1 {
		return false, errors.New("社交操作缺少重试时间戳")
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return false, err
	}
	var row db.P10SocialReceipt
	err = tx.Where("player_id=? AND domain=? AND stamp=?", id, domain, stamp).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.RequestHash != p6Digest(raw) {
		return false, errors.New("同一社交时间戳不能变更请求")
	}
	return true, proto.Unmarshal(row.Response, response)
}
func p10SaveReceipt(tx *gorm.DB, id int64, domain string, stamp int64, request, response proto.Message) error {
	raw, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	reply, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	row := db.P10SocialReceipt{PlayerID: id, Domain: domain, Stamp: stamp, RequestHash: p6Digest(raw), Response: reply}
	return tx.Create(&row).Error
}
func (s *GameService) p10Social(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, bool, error) {
	fresh := false
	var response proto.Message
	var push proto.Message
	var target int64
	var domain string
	_, isChat := request.(*pb.CSChatTcp)
	anonymousChat := isChat && stamp == 0
	switch b := request.(type) {
	case *pb.CSApplyFriend:
		target = b.GetFriendId()
		domain = "apply"
		response = &pb.SCApplyFriend{}
	case *pb.CSAgreeApply:
		target = b.GetFriendId()
		domain = "agree"
		response = &pb.SCAgreeApply{}
	case *pb.CSDenyApply:
		target = b.GetFriendId()
		domain = "deny"
		response = &pb.SCDenyApply{}
	case *pb.CSDeleteFriend:
		target = b.GetFriendId()
		domain = "delete"
		response = &pb.SCDeleteFriend{}
	case *pb.CSSendToBlackList:
		target = b.GetFriendId()
		domain = "black"
		response = &pb.SCSendToBlackList{}
	case *pb.CSDeleteFromBlackList:
		target = b.GetFriendId()
		domain = "unblack"
		response = &pb.SCDeleteFromBlackList{}
	case *pb.CSSendMassageToFriend:
		target = b.GetPlayerId()
		domain = constUtil.P10LeaveWordReceiptDomain
		response = &pb.SCSendMessageToFriend{}
	case *pb.CSChatTcp:
		target = b.GetPlayerId()
		domain = "private-chat"
		response = &pb.SCChatTcp{}
	}
	if domain != "" && (target < 1 || target == id) {
		return nil, false, errors.New("社交目标无效")
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		lockTarget := target
		if domain == "" {
			lockTarget = id
		}
		players, err := p10LockProfiles(tx, id, lockTarget)
		if err != nil {
			return err
		}
		if domain != "" && !anonymousChat {
			if found, err := p10ReadReceipt(tx, id, domain, stamp, request, response); err != nil || found {
				return err
			}
		}
		var cfg db.P10SocialPolicy
		if err = tx.First(&cfg, constUtil.P10SocialPolicyID).Error; err != nil {
			return err
		}
		if !cfg.Enabled && domain != "" {
			return errors.New(constUtil.P10SocialClosed)
		}
		if domain != "" {
			blocked, err := p10Blocked(tx, id, target)
			if err != nil {
				return err
			}
			switch request.(type) {
			case *pb.CSApplyFriend, *pb.CSAgreeApply, *pb.CSChatTcp, *pb.CSSendMassageToFriend:
				if blocked {
					return errors.New("黑名单阻止双方关系或私聊")
				}
			}
		}
		switch b := request.(type) {
		case *pb.CSApplyFriend:
			if b.GetWord() != "" && !p10Text(b.GetWord(), 40) {
				return errors.New("好友申请文字超过原限制")
			}
			friends, err := p10Friends(tx, id, target)
			if err != nil {
				return err
			}
			if friends {
				break
			}
			maxFriends, err := s.p10Setting("MaxFriendNum")
			if err != nil || maxFriends < 1 {
				return errors.New("原好友上限无效")
			}
			for _, pid := range []int64{id, target} {
				var n int64
				if err = tx.Model(&db.P10SocialRelation{}).Where("low_id=? OR high_id=?", pid, pid).Count(&n).Error; err != nil {
					return err
				}
				if n >= int64(maxFriends) {
					return errors.New("好友名额已满")
				}
			}
			apply := db.P10SocialApply{SenderID: id, ReceiverID: target, Word: b.GetWord(), CreatedAt: now.UnixMilli()}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&apply).Error; err != nil {
				return err
			}
		case *pb.CSAgreeApply:
			friends, err := p10Friends(tx, id, target)
			if err != nil {
				return err
			}
			if friends {
				break
			}
			var apply db.P10SocialApply
			if err = tx.Where("sender_id=? AND receiver_id=?", target, id).First(&apply).Error; err != nil {
				return errors.New("没有发给当前角色的好友申请")
			}
			maxFriends, err := s.p10Setting("MaxFriendNum")
			if err != nil || maxFriends < 1 {
				return errors.New("原好友上限无效")
			}
			for _, pid := range []int64{id, target} {
				var n int64
				if err = tx.Model(&db.P10SocialRelation{}).Where("low_id=? OR high_id=?", pid, pid).Count(&n).Error; err != nil {
					return err
				}
				if n >= int64(maxFriends) {
					return errors.New("好友名额已满")
				}
			}
			lo, hi := p10Pair(id, target)
			relation := db.P10SocialRelation{LowID: lo, HighID: hi, CreatedAt: now.UnixMilli()}
			if err = tx.Create(&relation).Error; err != nil {
				return err
			}
			if err = tx.Where("(sender_id=? AND receiver_id=?) OR (sender_id=? AND receiver_id=?)", id, target, target, id).Delete(&db.P10SocialApply{}).Error; err != nil {
				return err
			}
		case *pb.CSDenyApply:
			if err = tx.Where("sender_id=? AND receiver_id=?", target, id).Delete(&db.P10SocialApply{}).Error; err != nil {
				return err
			}
		case *pb.CSDeleteFriend, *pb.CSSendToBlackList:
			lo, hi := p10Pair(id, target)
			if err = tx.Where("low_id=? AND high_id=?", lo, hi).Delete(&db.P10SocialRelation{}).Error; err != nil {
				return err
			}
			if err = s.InvalidateP10FriendEnergy(tx, id, target); err != nil {
				return err
			}
			if err = tx.Where("(sender_id=? AND receiver_id=?) OR (sender_id=? AND receiver_id=?)", id, target, target, id).Delete(&db.P10SocialApply{}).Error; err != nil {
				return err
			}
			if _, black := request.(*pb.CSSendToBlackList); black {
				row := db.P10SocialBlack{OwnerID: id, TargetID: target, CreatedAt: now.UnixMilli()}
				if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
					return err
				}
			}
		case *pb.CSDeleteFromBlackList:
			if err = tx.Where("owner_id=? AND target_id=?", id, target).Delete(&db.P10SocialBlack{}).Error; err != nil {
				return err
			}
		case *pb.CSFriendList:
			var rows []db.P10SocialRelation
			if err = tx.Where("low_id=? OR high_id=?", id, id).Order("created_at,low_id,high_id").Find(&rows).Error; err != nil {
				return err
			}
			claimed, err := s.P10FriendEnergyClaimed(tx, id, now)
			if err != nil {
				return err
			}
			list := &pb.SCFriendList{GetEnergyNum: proto.Int32(claimed)}
			for _, row := range rows {
				pid := row.LowID
				if pid == id {
					pid = row.HighID
				}
				info, err := s.p10Player(tx, pid)
				if err != nil {
					return err
				}
				canGive, canGet, err := s.P10FriendEnergyFlagsWithCount(tx, id, pid, claimed, now)
				if err != nil {
					return err
				}
				list.FriendList = append(list.FriendList, &pb.FriendInfo{SimplePlayerInfo: info, CanGive: proto.Int32(canGive), CanGet: proto.Int32(canGet)})
			}
			response = list
		case *pb.CSFriendApplyList:
			var rows []db.P10SocialApply
			if err = tx.Where("receiver_id=?", id).Order("created_at,sender_id").Find(&rows).Error; err != nil {
				return err
			}
			list := &pb.SCFriendApplyList{}
			for _, row := range rows {
				info, err := s.p10Player(tx, row.SenderID)
				if err != nil {
					return err
				}
				list.ApplyList = append(list.ApplyList, &pb.FriendApplyInfo{FriendId: proto.Int64(row.SenderID), FriendName: info.PlayerName, Word: proto.String(row.Word), ApplyTime: proto.Int64(row.CreatedAt)})
			}
			response = list
		case *pb.CSFriendBlackList:
			var rows []db.P10SocialBlack
			if err = tx.Where("owner_id=?", id).Order("created_at,target_id").Find(&rows).Error; err != nil {
				return err
			}
			list := &pb.SCFriendBlackList{}
			for _, row := range rows {
				info, err := s.p10Player(tx, row.TargetID)
				if err != nil {
					return err
				}
				list.BlackList = append(list.BlackList, info)
			}
			response = list
		case *pb.CSSearchPlayer, *pb.CSRecommendFriendList:
			var rows []db.Profile
			query := tx.Where("id<>? AND player IS NOT NULL AND game IS NOT NULL", id)
			search := ""
			if b, ok := request.(*pb.CSSearchPlayer); ok {
				search = strings.TrimSpace(b.GetIdOrName())
				if !p10Text(search, 40) {
					return errors.New("搜索内容无效")
				}
				if pid, e := strconv.ParseInt(search, 10, 64); e == nil {
					query = query.Where("id=?", pid)
				}
			}
			if err = query.Order("id").Find(&rows).Error; err != nil {
				return err
			}
			var list []*pb.SimplePlayerInfo
			for _, row := range rows {
				p, _, err := decodeProfile(&row)
				if err != nil {
					continue
				}
				if search != "" && search != strconv.FormatInt(row.Id, 10) && !strings.Contains(p.GetName(), search) {
					continue
				}
				blocked, err := p10Blocked(tx, id, row.Id)
				if err != nil {
					return err
				}
				if blocked {
					continue
				}
				if search == "" {
					friends, err := p10Friends(tx, id, row.Id)
					if err != nil {
						return err
					}
					if friends {
						continue
					}
				}
				info, err := s.p10Player(tx, row.Id)
				if err != nil {
					// 损坏候选不冒充有效战力，也不阻断其他真实搜索结果；完整资料查询仍显式报错。
					continue
				}
				list = append(list, info)
				if len(list) >= 20 {
					break
				}
			}
			if _, ok := request.(*pb.CSSearchPlayer); ok {
				response = &pb.SCSearchPlayer{Player: list}
			} else {
				response = &pb.SCRecommendFriendList{RecommendList: list}
			}
		case *pb.CSChatTcp, *pb.CSSendMassageToFriend:
			// 原留言24105复用私聊同一行锁、黑名单、节流与历史；原云端投递容器未恢复。
			var input *pb.CSChatTcp
			if original, ok := b.(*pb.CSChatTcp); ok {
				input = original
			} else {
				leave := b.(*pb.CSSendMassageToFriend)
				input = &pb.CSChatTcp{ChatType: proto.Int32(constUtil.P10PrivateChatType), PlayerId: leave.PlayerId, Content: leave.Content}
			}
			if input.GetChatType() != constUtil.P10PrivateChatType || input.GetIsAudio() != 0 || input.GetFileName() != "" || input.GetDownLoadUrl() != "" || input.GetAudioTime() != 0 || input.GetFileSize() != 0 {
				return errors.New("本批仅恢复原私聊文本，其他频道/语音待完整权限及资源传输")
			}
			if !p10Text(input.GetContent(), 40) {
				return errors.New("私聊文本超过原40个UTF16单位或为空")
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
			interval, err := s.p10Setting("ChatInterval")
			if err != nil || interval > math.MaxInt32/1000 {
				return errors.New("原聊天间隔无效")
			}
			state := db.P10SocialChatState{PlayerID: id}
			err = tx.Where("player_id=?", id).First(&state).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if state.LastSendAt > 0 && now.UnixMilli()-state.LastSendAt < int64(interval)*1000 {
				// 原TCP头没有时间戳。在原20秒节流窗口内相同内容重传返回旧帧；窗口外是新消息。
				if anonymousChat {
					var old db.P10SocialChat
					if err = tx.Where("sender_id=?", id).Order("id DESC").First(&old).Error; err != nil {
						return err
					}
					if old.CreatedAt == state.LastSendAt && old.ReceiverID == target && old.Content == input.GetContent() && old.ChatType == input.GetChatType() {
						info := &pb.SimplePlayerInfo{}
						if err = proto.Unmarshal(old.SenderInfo, info); err != nil {
							return err
						}
						response = &pb.SCChatTcp{ChatType: proto.Int32(old.ChatType), Content: proto.String(old.Content), Time: proto.Int64(old.CreatedAt), Play: info, IsAudio: proto.Int32(0)}
						break
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
			chat := db.P10SocialChat{SenderID: id, ReceiverID: target, ChatType: constUtil.P10PrivateChatType, Content: input.GetContent(), CreatedAt: now.UnixMilli(), SenderInfo: raw, ReceiverName: players[target].GetName()}
			if err = tx.Create(&chat).Error; err != nil {
				return err
			}
			fresh = true
			state.LastSendAt = now.UnixMilli()
			if err = tx.Save(&state).Error; err != nil {
				return err
			}
			response = &pb.SCChatTcp{ChatType: proto.Int32(chat.ChatType), Content: proto.String(chat.Content), Time: proto.Int64(chat.CreatedAt), Play: info, IsAudio: proto.Int32(0)}
		case *pb.CSChatCache:
			// 原登录会请求未指定chatType的缓存。0返回当前角色可见实际历史，各频道仅过滤已有记录。
			if b.GetChatType() < 0 || b.GetChatType() > 6 {
				return errors.New("聊天历史频道无效")
			}
			limit, err := s.p10Setting("MaxChatNumber")
			if err != nil || limit < 1 || limit > 500 {
				return errors.New("原聊天历史上限无效")
			}
			var rows []db.P10SocialChat
			query := tx.Where("(sender_id=? OR receiver_id=?)", id, id)
			if b.GetChatType() != 0 {
				query = query.Where("chat_type=?", b.GetChatType())
			}
			if err = query.Order("id DESC").Limit(int(limit)).Find(&rows).Error; err != nil {
				return err
			}
			list := &pb.SCChatCache{}
			for i := len(rows) - 1; i >= 0; i-- {
				row := rows[i]
				target := row.ReceiverID
				if target == id {
					target = row.SenderID
				}
				blocked, err := p10Blocked(tx, id, target)
				if err != nil {
					return err
				}
				if blocked {
					continue
				}
				info := &pb.SimplePlayerInfo{}
				if proto.Unmarshal(row.SenderInfo, info) != nil {
					return errors.New("私聊发送人快照损坏")
				}
				list.ChatList = append(list.ChatList, &pb.ChatInfo{ChatType: proto.Int32(row.ChatType), Content: proto.String(row.Content), Time: proto.Int64(row.CreatedAt), Play: info, ReceiverName: proto.String(row.ReceiverName), IsAudio: proto.Int32(0)})
			}
			response = list
		default:
			return errors.New("社交请求尚未恢复")
		}
		if domain == constUtil.P10LeaveWordReceiptDomain && fresh {
			// HTTP只返回原空Ack；已提交真实私聊帧由dispatcher向在线接收方投递。
			push = response
			response = &pb.SCSendMessageToFriend{}
		}
		if domain != "" && !anonymousChat {
			return p10SaveReceipt(tx, id, domain, stamp, request, response)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	messages := []proto.Message{response}
	if push != nil {
		messages = append(messages, push)
	}
	return messages, fresh, nil
}
func (s *GameService) DispatchP10Social(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSSendMassageToFriend:
		messages, fresh, err := s.p10Social(ctx, id, b, stamp, time.Now())
		if err != nil {
			return nil, true, err
		}
		if fresh && len(messages) == 2 {
			s.publishP10Private(ctx, b.GetPlayerId(), messages[1])
		}
		return messages[:1], true, nil
	case *pb.CSFamous, *pb.CSGood:
		messages, err := s.p10Worship(ctx, id, request, stamp, time.Now())
		return messages, true, err
	case *pb.CSApplyFriend, *pb.CSAgreeApply, *pb.CSDenyApply, *pb.CSDeleteFriend, *pb.CSSendToBlackList, *pb.CSDeleteFromBlackList, *pb.CSFriendList, *pb.CSFriendApplyList, *pb.CSFriendBlackList, *pb.CSSearchPlayer, *pb.CSRecommendFriendList, *pb.CSChatTcp, *pb.CSChatCache:
		r, fresh, err := s.p10Social(ctx, id, request, stamp, time.Now())
		if err == nil && fresh {
			if chat, ok := request.(*pb.CSChatTcp); ok {
				s.publishP10Private(ctx, chat.GetPlayerId(), r[0])
			}
		}
		return r, true, err
	}
	return nil, false, nil
}
