package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP10ExpeditionHelp 初始化单独关闭的好友救援域，保持原VIP全部权益。
func (s *GameService) InitializeP10ExpeditionHelp(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10ExpeditionHelpPolicy{}, &db.P10ExpeditionHelpDaily{}, &db.P10ExpeditionHelpAction{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P10ExpeditionHelpPolicy{ID: 1, Source: constUtil.P10ExpeditionHelpSource}).Error
}

func p10ExpeditionHelpNow(ctx context.Context) time.Time {
	if clock, ok := ctx.Value(constUtil.P10ExpeditionHelpClockContextKey{}).(func() time.Time); ok {
		return clock()
	}
	return LocalPlayNow(ctx)
}

func p10ExpeditionHelpDailyHash(row *db.P10ExpeditionHelpDaily) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(struct {
		Metadata      db.P10ExpeditionHelpDaily
		VIPLimitsHash string
	}{copy, p6Digest(copy.VIPLimits)})
	return p6Digest(raw)
}

func p10ExpeditionHelpActionHash(row *db.P10ExpeditionHelpAction) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p10ExpeditionHelpQuote 当日第一次实际救援才冻结奖励和完整VIP目录，不以旧15覆盖VIP0的10次。
func (s *GameService) p10ExpeditionHelpQuote(tx *gorm.DB, id int64, day string, freeze bool) (*db.P10ExpeditionHelpDaily, error) {
	var row db.P10ExpeditionHelpDaily
	err := tx.First(&row, "player_id=? AND day=?", id, day).Error
	if err == nil {
		if row.Reward != "diamond,5" || len(row.ConfigurationHash) != 64 || row.SourceHash != p10ExpeditionHelpDailyHash(&row) {
			return nil, errors.New("远征好友帮助日奖励来源损坏")
		}
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	reward := s.table("GameConfigure", "StaticName", "ExpeditionHelperReward")["NameValue"]
	if reward != "diamond,5" {
		return nil, errors.New("原远征帮助5钻奖励配置缺失")
	}
	limits := map[string]int64{}
	expected := strings.Split(constUtil.P10ExpeditionHelpVIPCatalog, ",")
	for _, cfg := range s.Tables["VipPerks"] {
		vip, err := strconv.ParseInt(cfg["vip"], 10, 64)
		if err != nil || vip < 0 || vip > 12 {
			return nil, errors.New("原远征帮助VIP目录无效")
		}
		amount, err := strconv.ParseInt(cfg["numPerk5"], 10, 64)
		if err != nil || amount < 1 || amount > 30 {
			return nil, errors.New("原远征帮助VIP次数无效")
		}
		original, err := strconv.ParseInt(expected[vip], 10, 64)
		if err != nil || original != amount {
			return nil, errors.New("原远征帮助VIP次数与客户端目录不一致")
		}
		key := strconv.FormatInt(vip, 10)
		if _, found := limits[key]; found {
			return nil, errors.New("原远征帮助VIP目录重复")
		}
		limits[key] = amount
	}
	if len(limits) != 13 {
		return nil, errors.New("原远征帮助VIP0至12目录不完整")
	}
	raw, err := json.Marshal(limits)
	if err != nil {
		return nil, err
	}
	configuration, err := json.Marshal(struct {
		Reward    string
		VIPLimits map[string]int64
	}{reward, limits})
	if err != nil {
		return nil, err
	}
	row = db.P10ExpeditionHelpDaily{PlayerID: id, Day: day, Reward: reward, VIPLimits: raw, ConfigurationHash: p6Digest(configuration)}
	row.SourceHash = p10ExpeditionHelpDailyHash(&row)
	if freeze {
		if err = tx.Create(&row).Error; err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func p10ExpeditionHelpLimit(row *db.P10ExpeditionHelpDaily, vip int32) (int64, error) {
	var limits map[string]int64
	if err := json.Unmarshal(row.VIPLimits, &limits); err != nil {
		return 0, err
	}
	amount, found := limits[strconv.FormatInt(int64(vip), 10)]
	if !found || amount < 1 || amount > 30 {
		return 0, errors.New("当前真实VIP的远征帮助次数未恢复")
	}
	return amount, nil
}

// p10ExpeditionHelpActions 核对当日每个实际救援来源；次数来自一次性事件事实。
func p10ExpeditionHelpActions(tx *gorm.DB, daily *db.P10ExpeditionHelpDaily) ([]db.P10ExpeditionHelpAction, error) {
	var rows []db.P10ExpeditionHelpAction
	if err := tx.Where("helper_id=? AND day=?", daily.PlayerID, daily.Day).Order("created_at,event_serial").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.DailyHash != daily.SourceHash || row.Reward != daily.Reward || row.SourceHash != p10ExpeditionHelpActionHash(&row) || len(row.EventHash) != 64 {
			return nil, errors.New("实际远征帮助次数/事件来源损坏")
		}
	}
	if len(rows) > 30 {
		return nil, errors.New("实际远征帮助次数超过原最高VIP权益")
	}
	return rows, nil
}

// p10ExpeditionHelpFriends 在已锁帮助者下筛选当前双方关系和黑名单，P7列表只接这个实际可见集合。
func p10ExpeditionHelpFriends(tx *gorm.DB, id int64) ([]int64, error) {
	var rows []db.P10SocialRelation
	if err := tx.Where("low_id=? OR high_id=?", id, id).Find(&rows).Error; err != nil {
		return nil, err
	}
	var ids []int64
	for _, row := range rows {
		peer := row.LowID
		if peer == id {
			peer = row.HighID
		}
		blocked, err := p10Blocked(tx, id, peer)
		if err != nil {
			return nil, err
		}
		if !blocked {
			ids = append(ids, peer)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func p10ExpeditionHelpRelation(tx *gorm.DB, id, target int64) (*db.P10SocialRelation, error) {
	blocked, err := p10Blocked(tx, id, target)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, errors.New("黑名单阻止远征好友帮助")
	}
	lo, hi := p10Pair(id, target)
	var relation db.P10SocialRelation
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&relation, "low_id=? AND high_id=?", lo, hi).Error; err != nil {
		return nil, errors.New("仅当前双方好友可帮助远征风险")
	}
	return &relation, nil
}

// p10ExpeditionHelpAward 只在P7同TX已经消费实际风险后调用，奖励、事实、日次数与原空成功回执一起提交。
func (s *GameService) p10ExpeditionHelpAward(tx *gorm.DB, profile *db.Profile, daily *db.P10ExpeditionHelpDaily, action *db.P10ExpeditionHelpAction, request *pb.CSHelpFriendEvent, stamp int64, now time.Time) (*pb.Player, error) {
	p, g, err := decodeProfileForCalendarUpdate(tx, profile)
	if err != nil {
		return nil, err
	}
	if action.OwnerID != request.GetFriendId() || action.MissionSerial != request.GetMissionSerial() || action.EventSerial != request.GetEventSerial() || len(action.EventHash) != 64 || action.StoryID == "" {
		return nil, errors.New("原远征帮助消费事件身份不一致")
	}
	action.HelperID = profile.Id
	action.Day = daily.Day
	action.Reward = daily.Reward
	action.DailyHash = daily.SourceHash
	action.CreatedAt = now.UnixMilli()
	action.SourceHash = p10ExpeditionHelpActionHash(action)
	if err = tx.Create(action).Error; err != nil {
		return nil, err
	}
	if _, err = s.GrantFixedReward(p, g, daily.Reward); err != nil {
		return nil, err
	}
	binding, err := json.Marshal(struct {
		OwnerID                    int64
		MissionSerial, EventSerial string
	}{action.OwnerID, action.MissionSerial, action.EventSerial})
	if err != nil {
		return nil, err
	}
	source := "p10-exp-help:" + p6Digest(binding)
	if err = s.RecordP1Event(tx, profile.Id, source, constUtil.P10ExpeditionHelpEvent, 1, now); err != nil {
		return nil, err
	}
	if err = s.RecordP1Event(tx, profile.Id, source, constUtil.P10ExpeditionHelpDiamondEvent, 5, now); err != nil {
		return nil, err
	}
	if err = saveProfile(tx, profile, p, g); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	// SCHelpFriendEvent原定义没有字段；非nil空BLOB保留合法回执，不能向InnoDB NOT NULL写SQL NULL。
	if err = tx.Create(&db.P10SocialReceipt{PlayerID: profile.Id, Domain: constUtil.P10ExpeditionHelpOperation, Stamp: stamp, RequestHash: p6Digest(raw), Response: []byte{}}).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// p10ExpeditionHelpPending 只接P7真实冻结风险的标识与SHA，显示名称由当前好友存档决定。
type p10ExpeditionHelpPending struct {
	OwnerID       int64
	MissionSerial string
	EventSerial   string
	StoryID       string
	EventHash     string
}

type p10ExpeditionHelpList func(*gorm.DB, []int64, time.Time) ([]p10ExpeditionHelpPending, error)
type p10ExpeditionHelpApply func(*gorm.DB, int64, int64, string, string, time.Time) (*p10ExpeditionHelpPending, error)

// p10ExpeditionHelpAt 将上游实际风险消费与下游奖励统一到caller事务；上游接点未接时明确拒绝。
func (s *GameService) p10ExpeditionHelpAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time, list p10ExpeditionHelpList, apply p10ExpeditionHelpApply) ([]proto.Message, error) {
	if list == nil || apply == nil {
		return nil, errors.New("P7实际远征风险事件接点未接入")
	}
	var reply proto.Message
	var player *pb.Player
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		ids := []int64{id}
		body, mutate := request.(*pb.CSHelpFriendEvent)
		if mutate {
			ids = append(ids, body.GetFriendId())
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var own db.Profile
		for _, pid := range ids {
			var row db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, pid).Error; err != nil {
				return err
			}
			if _, _, err := decodeProfile(&row); err != nil {
				return err
			}
			if pid == id {
				own = row
			}
		}
		p, _, err := decodeProfileForCalendarUpdate(tx, &own)
		if err != nil {
			return err
		}
		player = p
		if mutate {
			prior := &pb.SCHelpFriendEvent{}
			found, err := p10ReadReceipt(tx, id, constUtil.P10ExpeditionHelpOperation, stamp, request, prior)
			if err != nil {
				return err
			}
			if found {
				var action db.P10ExpeditionHelpAction
				if err = tx.First(&action, "owner_id=? AND mission_serial=? AND event_serial=? AND helper_id=?", body.GetFriendId(), body.GetMissionSerial(), body.GetEventSerial(), id).Error; err != nil {
					return err
				}
				daily, err := s.p10ExpeditionHelpQuote(tx, id, action.Day, false)
				if err != nil {
					return err
				}
				if action.DailyHash != daily.SourceHash || action.SourceHash != p10ExpeditionHelpActionHash(&action) || action.Reward != daily.Reward {
					return errors.New("远征好友帮助原回执事件来源损坏")
				}
				reply = prior
				return nil
			}
		}
		var policy db.P10ExpeditionHelpPolicy
		level, err := s.p7ExpeditionSetting("SailingLevel")
		if err != nil {
			return err
		}
		if p.GetLevel() < level {
			return errors.New("角色尚未达到原32级远征帮助开放条件")
		}
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		day := p1Day(now)
		daily, err := s.p10ExpeditionHelpQuote(tx, id, day, false)
		if err != nil {
			return err
		}
		prior, err := p10ExpeditionHelpActions(tx, daily)
		if err != nil {
			return err
		}
		limit, err := p10ExpeditionHelpLimit(daily, p.GetVip())
		if err != nil {
			return err
		}
		if !mutate {
			friends, err := p10ExpeditionHelpFriends(tx, id)
			if err != nil {
				return err
			}
			pending, err := list(tx, friends, now)
			if err != nil {
				return err
			}
			visible := map[int64]bool{}
			for _, pid := range friends {
				visible[pid] = true
			}
			seen := map[string]bool{}
			var events []*pb.FriendsBadEventInfo
			for _, event := range pending {
				if !visible[event.OwnerID] || len(event.EventHash) != 64 || event.MissionSerial == "" || event.EventSerial == "" || event.StoryID == "" {
					return errors.New("上游远征风险超出当前好友或事件来源不完整")
				}
				key := strconv.FormatInt(event.OwnerID, 10) + "\x00" + event.MissionSerial + "\x00" + event.EventSerial
				if seen[key] {
					return errors.New("上游实际远征风险重复")
				}
				seen[key] = true
				var owner db.Profile
				if err = tx.First(&owner, event.OwnerID).Error; err != nil {
					return err
				}
				friend, _, err := decodeProfile(&owner)
				if err != nil {
					return err
				}
				events = append(events, &pb.FriendsBadEventInfo{FriendsId: proto.Int64(event.OwnerID), FriendName: friend.Name, MissionSerial: proto.String(event.MissionSerial), EventSerial: proto.String(event.EventSerial), StoryId: proto.String(event.StoryID)})
			}
			sort.Slice(events, func(i, j int) bool {
				if events[i].GetFriendsId() != events[j].GetFriendsId() {
					return events[i].GetFriendsId() < events[j].GetFriendsId()
				}
				if events[i].GetMissionSerial() != events[j].GetMissionSerial() {
					return events[i].GetMissionSerial() < events[j].GetMissionSerial()
				}
				return events[i].GetEventSerial() < events[j].GetEventSerial()
			})
			reply = &pb.SCFriendBadEvents{TodayHelpTime: proto.Int32(int32(len(prior))), EventList: events}
			return nil
		}
		if !policy.Enabled {
			return errors.New("实际好友远征帮助策略尚未开放")
		}
		if int64(len(prior)) >= limit {
			return errors.New("原当前VIP每日远征好友帮助次数已用完")
		}
		relation, err := p10ExpeditionHelpRelation(tx, id, body.GetFriendId())
		if err != nil {
			return err
		}
		// 上游检查当前任务/风险/到期并消费一次，失败会回滚本域日冻结、奖励与次数。
		event, err := apply(tx, id, body.GetFriendId(), body.GetMissionSerial(), body.GetEventSerial(), now)
		if err != nil {
			return err
		}
		if event == nil {
			return errors.New("上游未返回实际已消费风险事件")
		}
		daily, err = s.p10ExpeditionHelpQuote(tx, id, day, true)
		if err != nil {
			return err
		}
		action := &db.P10ExpeditionHelpAction{OwnerID: event.OwnerID, MissionSerial: event.MissionSerial, EventSerial: event.EventSerial, StoryID: event.StoryID, EventHash: event.EventHash, FriendSince: relation.CreatedAt}
		player, err = s.p10ExpeditionHelpAward(tx, &own, daily, action, body, stamp, now)
		if err != nil {
			return err
		}
		reply = &pb.SCHelpFriendEvent{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, mutate := request.(*pb.CSHelpFriendEvent); mutate {
		return []proto.Message{reply, &pb.SCPlayer{Player: player}}, nil
	}
	return []proto.Message{reply}, nil
}

// DispatchP10ExpeditionHelp 适配P7真实冻结风险API，必须在旧P7帮助fallback之前调用。
func (s *GameService) DispatchP10ExpeditionHelp(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSFriendBadEvents, *pb.CSHelpFriendEvent:
	default:
		return nil, false, nil
	}
	list := func(tx *gorm.DB, owners []int64, now time.Time) ([]p10ExpeditionHelpPending, error) {
		rows, err := s.ListP7ExpeditionHelp(tx, owners, now)
		if err != nil {
			return nil, err
		}
		var pending []p10ExpeditionHelpPending
		for _, row := range rows {
			pending = append(pending, p10ExpeditionHelpPending{OwnerID: row.OwnerID, MissionSerial: row.MissionSerial, EventSerial: row.EventSerial, StoryID: row.StoryID, EventHash: row.EventHash})
		}
		return pending, nil
	}
	apply := func(tx *gorm.DB, helper, owner int64, mission, event string, now time.Time) (*p10ExpeditionHelpPending, error) {
		row, err := s.ApplyP7ExpeditionHelp(tx, helper, owner, mission, event, now)
		if err != nil || row == nil {
			return nil, err
		}
		return &p10ExpeditionHelpPending{OwnerID: row.OwnerID, MissionSerial: row.MissionSerial, EventSerial: row.EventSerial, StoryID: row.StoryID, EventHash: row.EventHash}, nil
	}
	messages, err := s.p10ExpeditionHelpAt(ctx, id, request, stamp, p10ExpeditionHelpNow(ctx), list, apply)
	return messages, true, err
}
