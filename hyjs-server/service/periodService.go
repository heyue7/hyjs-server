package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) seedP1Periods(ctx context.Context, now time.Time) error {
	zone := time.FixedZone("UTC+8", 8*3600)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 1, 0)
	// 保留待运营配置的登录奖励模板，原奖历未恢复时默认关闭；已有实例保持数据库设置。
	for _, cfg := range []db.PeriodInstance{
		{ID: "totalLogin_p1_" + start.Format("200601") + "_welcome", Title: "登录补给", Enabled: false, Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 1, Rewards: "coin,1000", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Source: "本地兼容服：成功角色登录日按北京时间每日一次；首次登录1000金币，领奖截止下月7日"},
		{ID: "totalLogin_p1_" + start.Format("200601") + "_three", Title: "三日补给", Enabled: false, Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 3, Rewards: "item_001,1", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Source: "本地兼容服：三次不同北京时间登录日获得1份道具，缺席不补记；领奖截止下月7日"},
	} {
		if err := s.validateP1Period(cfg); err != nil {
			return err
		}
		if err := db.GetIns().WithContext(ctx).Where("id = ?", cfg.ID).FirstOrCreate(&cfg).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) validateP1Period(cfg db.PeriodInstance) error {
	if cfg.ID == "" || len(cfg.ID) > 64 || cfg.StartAt <= 0 || cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt || cfg.Goal <= 0 || cfg.Goal > math.MaxInt32 || cfg.Kind == "" || len(cfg.Kind) > 64 {
		return errors.New("活动实例规则无效")
	}
	if cfg.Mode != "cumulative" && cfg.Mode != "daily" && cfg.Mode != "single" {
		return errors.New("活动统计口径无效")
	}
	ok, err := s.openServerRewardSupported(cfg.Rewards)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("活动奖励资源未恢复")
	}
	if cfg.RankPlaces < 0 || cfg.RankPlaces > 1000 {
		return errors.New("活动排名范围无效")
	}
	if cfg.RankPlaces > 0 {
		ok, err = s.openServerRewardSupported(cfg.RankRewards)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("活动排行奖励未恢复")
		}
	}
	return nil
}

// freezeP1Period 在第一次事实写入或截止结算时冻结整个实例，修改未来规则须创建新ID。
func (s *GameService) freezeP1Period(tx *gorm.DB, live db.PeriodInstance, freeze bool) (db.PeriodInstance, error) {
	cfg := live
	if len(live.Frozen) > 0 {
		if err := json.Unmarshal(live.Frozen, &cfg); err != nil {
			return cfg, err
		}
		if cfg.ID != live.ID {
			return cfg, errors.New("活动实例冻结ID不符")
		}
		cfg.Enabled = live.Enabled
	} else if freeze {
		if err := s.validateP1Period(cfg); err != nil {
			return cfg, err
		}
		blob, err := json.Marshal(cfg)
		if err != nil {
			return cfg, err
		}
		if err = tx.Model(&db.PeriodInstance{}).Where("id = ?", live.ID).Update("frozen", blob).Error; err != nil {
			return cfg, err
		}
	}
	return cfg, s.validateP1Period(cfg)
}

// IsP1Activity 按独立命名域路由，不访问数据库或吞掉查询错误。
func (s *GameService) IsP1Activity(id string) bool {
	return strings.HasPrefix(id, constUtil.P1ActivityPrefix) || strings.HasPrefix(id, "totalLogin_p1_") || strings.Contains(id, "_p4_")
}

func (s *GameService) P1ActivityTabs(ctx context.Context, id int64) ([]*pb.ActivityTab, error) {
	if !s.P1Ready {
		return nil, nil
	}
	var rows []db.PeriodInstance
	if err := db.GetIns().WithContext(ctx).Where("enabled = ?", true).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var tabs []*pb.ActivityTab
	now := LocalPlayNow(ctx).UnixMilli()
	var loginTab *pb.ActivityTab
	for _, live := range rows {
		allowed, err := localPlayActivityVisible(db.GetIns().WithContext(ctx), live.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}

		cfg, err := s.freezeP1Period(db.GetIns().WithContext(ctx), live, false)
		if err != nil {
			// false只解析/校验快照；无效单条不影响其余活动，领取入口仍严格拒绝。
			continue
		}
		if now < cfg.StartAt || now >= cfg.RewardEndAt || !s.IsP1Activity(cfg.ID) || isP15SeasonalChild(cfg.ID) {
			continue
		}
		status := int32(0)
		var pending int64
		if err := db.GetIns().WithContext(ctx).Model(&db.RewardDelivery{}).Where("player_id = ? AND source_key = ? AND claimed_at = 0 AND (expires_at = 0 OR expires_at > ?)", id, "activity:"+cfg.ID, now).Count(&pending).Error; err != nil {
			return nil, err
		}
		if pending > 0 {
			status = 1
		}
		if strings.HasPrefix(cfg.ID, "totalLogin_p1_") {
			if loginTab == nil {
				loginTab = &pb.ActivityTab{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Name: proto.String("登录补给"), Order: proto.Int32(20), Status: proto.Int32(0), Param: proto.String("累计登录领取奖励")}
			}
			if status > loginTab.GetStatus() {
				loginTab.Status = proto.Int32(status)
			}
			continue
		}
		tabs = append(tabs, &pb.ActivityTab{ActivityId: proto.String(cfg.ID), Name: proto.String(cfg.Title), Order: proto.Int32(int32(20 + len(tabs))), Status: proto.Int32(status), Param: proto.String(cfg.Source)})
	}
	legacyVisible, err := p1LegacyLoginVisible(db.GetIns().WithContext(ctx), now)
	if err != nil {
		return nil, err
	}
	if loginTab == nil && legacyVisible {
		loginTab = &pb.ActivityTab{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Name: proto.String("登录补给"), Order: proto.Int32(20), Status: proto.Int32(0), Param: proto.String("累计登录领取奖励")}
	}
	if loginTab != nil {
		tabs = append(tabs, loginTab)
	}
	return tabs, nil
}

// P1Activity 复用原累计活动协议；直接领取与领奖中心引用同一附件，跨入口只发一次。
func (s *GameService) P1Activity(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var key string
	switch b := request.(type) {
	case *pb.CSActivityInfo:
		key = b.GetActivityId()
	case *pb.CSActivityReward:
		key = b.GetActivityId()
	default:
		return nil, errors.New("活动请求类型无效")
	}
	if !s.IsP1Activity(key) {
		return nil, errors.New("活动ID不属于公共活动域")
	}
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		var err error
		now := LocalPlayNow(ctx).UnixMilli()
		if key == constUtil.P1LoginDirectoryID {
			response, err = s.p1LoginDirectoryApply(tx, id, player, game, request, now)
		} else {
			response, err = s.p1ActivityApply(tx, id, player, game, key, request, now)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// p1ActivityApply keeps direct, directory and reward-center claims on the existing delivery.
func (s *GameService) p1ActivityApply(tx *gorm.DB, id int64, player *pb.Player, game *model.GameState, key string, request proto.Message, now int64) (proto.Message, error) {
	var response proto.Message
	allowed, err := localPlayActivityAccess(tx, key, false)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errLocalPlayActivitiesInvalid
	}
	if err := s.p15SeasonalGuard(tx, key, LocalPlayNow(tx.Statement.Context)); err != nil {
		return nil, err
	}
	var live db.PeriodInstance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", key).First(&live).Error; err != nil {
		return nil, errors.New("活动实例不存在")
	}
	cfg, err := s.freezeP1Period(tx, live, false)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || now < cfg.StartAt || now >= cfg.RewardEndAt {
		return nil, errors.New("活动未开放或领奖截止")
	}
	progress, frozen, err := s.p1Progress(tx, cfg, id)
	if err != nil {
		return nil, err
	}
	if frozen.Kind == constUtil.P1EventCopyWin && frozen.Mode == "cumulative" && now < frozen.EndAt {
		if err = s.p1ReconcileCopyProgress(tx, progress, frozen, now); err != nil {
			return nil, err
		}
	}
	if frozen.Kind == constUtil.P4PlunderActivityKind {
		if err = s.p4ReconcilePlunderActivityProgress(tx, progress, frozen, now); err != nil {
			return nil, err
		}
	}
	if frozen.Kind == constUtil.P4ArenaWinKind {
		if err = s.p4ReconcileArenaWinProgress(tx, progress, frozen, now); err != nil {
			return nil, err
		}
	}
	resources, err := p1RewardResources(frozen.Rewards)
	if err != nil {
		return nil, err
	}
	value := progress.Value
	if frozen.Mode == "daily" && progress.LastDay != p1Day(LocalPlayNow(tx.Statement.Context)) && now < frozen.EndAt {
		value = 0
	}
	status := int32(0)
	var delivery db.RewardDelivery
	if progress.RewardID > 0 {
		if err = tx.Where("id = ? AND player_id = ?", progress.RewardID, id).First(&delivery).Error; err != nil {
			return nil, err
		}
		status = 1
		if delivery.ClaimedAt > 0 {
			status = 2
		}
	}
	item := &pb.ActivityItem{Index: proto.Int32(1), Value: proto.String(strconv.FormatInt(frozen.Goal, 10)), RewardList: resources, Status: proto.Int32(status), Left: proto.Int32(int32(min(value, frozen.Goal))), Total: proto.Int32(int32(frozen.Goal)), PickOneOrGetAll: proto.Int32(1)}
	switch b := request.(type) {
	case *pb.CSActivityInfo:
		// Keep rule provenance in the frozen record; the original UI renders
		// Information as one line and needs a concise activity description.
		information := frozen.Title + "：完成活动目标即可领取对应奖励。"
		if frozen.Kind == constUtil.P1EventCopyWin && frozen.Mode == "cumulative" {
			information = "主线胜利累计" + strconv.FormatInt(frozen.Goal, 10) + "次，可领取" + frozen.Title + "。"
		}
		response = &pb.SCActivityInfo{ActivityId: proto.String(key), ItemList: []*pb.ActivityItem{item}, StartTime: proto.Int64(frozen.StartAt), EndTime: proto.Int64(frozen.EndAt), RewardEndTime: proto.Int64(frozen.RewardEndAt), Param: proto.String("1"), Information: proto.String(information)}
	case *pb.CSActivityReward:
		if b.GetIndex() != 1 || !p1SameResources(resources, b.GetGetResources()) {
			return nil, errors.New("活动领奖选项无效")
		}
		if progress.RewardID == 0 {
			return nil, errors.New("活动条件未达成")
		}
		gain := &pb.ResultObject{}
		if delivery.ClaimedAt == 0 {
			if delivery.ExpiresAt > 0 && delivery.ExpiresAt <= now {
				return nil, errors.New("活动奖励已过期")
			}
			gain, err = s.GrantFixedReward(player, game, delivery.Rewards)
			if err != nil {
				return nil, err
			}
			blob, err := proto.Marshal(gain)
			if err != nil {
				return nil, err
			}
			delivery.ClaimedAt = now
			delivery.Result = blob
			if err = tx.Save(&delivery).Error; err != nil {
				return nil, err
			}
		}
		item.Status = proto.Int32(2)
		response = &pb.SCActivityReward{ActivityId: proto.String(key), ItemList: []*pb.ActivityItem{item}, Get: gain}
	}
	return response, nil
}

// p1LoginDirectoryApply maps stable protocol indices to existing login instances.
// Removing a child never shifts another reward into an old request's index.
func (s *GameService) p1LoginDirectoryApply(tx *gorm.DB, id int64, player *pb.Player, game *model.GameState, request proto.Message, now int64) (proto.Message, error) {
	if b, ok := request.(*pb.CSActivityReward); ok {
		var member db.P1LoginDirectoryMember
		if err := tx.Where("index_id = ?", b.GetIndex()).First(&member).Error; err != nil {
			return nil, errors.New("登录奖励选项不存在")
		}
		if member.ChildIndex <= 0 || ((!strings.HasPrefix(member.InstanceID, "totalLogin_p1_") || member.InstanceID == constUtil.P1LoginDirectoryID) && member.InstanceID != constUtil.P1LegacyLoginID) {
			return nil, errors.New("登录奖励索引绑定无效")
		}
		child := proto.Clone(b).(*pb.CSActivityReward)
		child.ActivityId = proto.String(member.InstanceID)
		child.Index = proto.Int32(member.ChildIndex)
		var reply proto.Message
		var err error
		if member.InstanceID == constUtil.P1LegacyLoginID {
			reply, err = s.legacyLoginActivityApply(tx, id, player, game, child, now)
		} else {
			reply, err = s.p1ActivityApply(tx, id, player, game, member.InstanceID, child, now)
		}
		if err != nil {
			return nil, err
		}
		result := reply.(*pb.SCActivityReward)
		result.ActivityId = proto.String(constUtil.P1LoginDirectoryID)
		// The original reward callback refreshes the entire directory item list.
		full, err := s.p1LoginDirectoryApply(tx, id, player, game, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.P1LoginDirectoryID)}, now)
		if err != nil {
			return nil, err
		}
		result.ItemList = full.(*pb.SCActivityInfo).ItemList
		return result, nil
	}
	var rows []db.PeriodInstance
	if err := tx.Where("enabled = ? AND id LIKE ?", true, "totalLogin_p1_%").Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := &pb.SCActivityInfo{ActivityId: proto.String(constUtil.P1LoginDirectoryID), Param: proto.String("1"), Information: proto.String("各条奖励独立统计，领取期限见条目。")}
	for _, live := range rows {
		allowed, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}

		if !strings.HasPrefix(live.ID, "totalLogin_p1_") {
			continue
		}
		cfg, err := s.freezeP1Period(tx, live, false)
		if err != nil {
			return nil, err
		}
		if now < cfg.StartAt || now >= cfg.RewardEndAt {
			continue
		}
		index, err := p1LoginDirectoryIndex(tx, cfg.ID, 1)
		if err != nil {
			return nil, err
		}
		reply, err := s.p1ActivityApply(tx, id, player, game, cfg.ID, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)}, now)
		if err != nil {
			return nil, err
		}
		child := reply.(*pb.SCActivityInfo)
		child.ItemList[0].Index = proto.Int32(index)
		child.ItemList[0].Message = proto.String(p1LoginDirectoryCaption(cfg.Title, child.ItemList[0], child.GetRewardEndTime()))
		result.ItemList = append(result.ItemList, child.ItemList...)
		if result.GetStartTime() == 0 || child.GetStartTime() < result.GetStartTime() {
			result.StartTime = proto.Int64(child.GetStartTime())
		}
		if child.GetEndTime() > result.GetEndTime() {
			result.EndTime = proto.Int64(child.GetEndTime())
		}
		if child.GetRewardEndTime() > result.GetRewardEndTime() {
			result.RewardEndTime = proto.Int64(child.GetRewardEndTime())
		}
	}
	legacyVisible, err := p1LegacyLoginVisible(tx, now)
	if err != nil {
		return nil, err
	}
	if legacyVisible {
		reply, err := s.legacyLoginActivityApply(tx, id, player, game, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.P1LegacyLoginID)}, now)
		if err != nil {
			return nil, err
		}
		child := reply.(*pb.SCActivityInfo)
		for _, item := range child.ItemList {
			index, err := p1LoginDirectoryIndex(tx, constUtil.P1LegacyLoginID, item.GetIndex())
			if err != nil {
				return nil, err
			}
			item.Index = proto.Int32(index)
			item.Message = proto.String(p1LoginDirectoryCaption("累计登录", item, max(child.GetRewardEndTime(), child.GetEndTime())))
			result.ItemList = append(result.ItemList, item)
		}
		if result.GetStartTime() == 0 || child.GetStartTime() < result.GetStartTime() {
			result.StartTime = proto.Int64(child.GetStartTime())
		}
		if child.GetEndTime() > result.GetEndTime() {
			result.EndTime = proto.Int64(child.GetEndTime())
		}
		if child.GetRewardEndTime() > result.GetRewardEndTime() {
			result.RewardEndTime = proto.Int64(child.GetRewardEndTime())
		}
	}
	if len(result.ItemList) == 0 {
		return nil, errors.New("登录活动未开放或领奖截止")
	}
	sort.SliceStable(result.ItemList, func(i, j int) bool { return result.ItemList[i].GetTotal() < result.ItemList[j].GetTotal() })
	return result, nil
}

// The shared original tab has one global date range. Each item states its own
// frozen claim deadline so a longer legacy window cannot extend monthly claims.
func p1LoginDirectoryCaption(title string, item *pb.ActivityItem, deadline int64) string {
	caption := title + "·" + strconv.FormatInt(int64(item.GetTotal()), 10) + "天"
	if deadline > 0 {
		last := time.UnixMilli(deadline - 1).In(time.FixedZone("UTC+8", 8*3600))
		caption += "·领至" + last.Format("1/2")
	}
	return caption
}

func p1LoginDirectoryIndex(tx *gorm.DB, key string, child int32) (int32, error) {
	member := db.P1LoginDirectoryMember{InstanceID: key, ChildIndex: child}
	if err := tx.Where("instance_id = ? AND child_index = ?", key, child).First(&member).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
			return 0, err
		}
		if err = tx.Where("instance_id = ? AND child_index = ?", key, child).First(&member).Error; err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	if member.IndexID <= 0 || member.IndexID > math.MaxInt32 {
		return 0, errors.New("登录奖励索引超限")
	}
	return int32(member.IndexID), nil
}

func p1LegacyLoginVisible(tx *gorm.DB, now int64) (bool, error) {
	if !tx.Migrator().HasTable(&db.ActivityRules{}) {
		return false, nil
	}
	var cfg db.ActivityRules
	if err := tx.Where("id = ?", constUtil.P1LegacyLoginID).First(&cfg).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if !cfg.Enabled || now < cfg.StartAt || now >= cfg.RewardEndAt {
		return false, nil
	}
	if cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt {
		return false, nil
	}
	return true, nil
}

func p1SameResources(a, b []*pb.BaseResource) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GetId() != b[i].GetId() || a[i].GetNum() != b[i].GetNum() {
			return false
		}
	}
	return true
}

type p1Rank struct {
	PlayerID int64 `json:"playerId"`
	Value    int64 `json:"value"`
	Rank     int32 `json:"rank"`
}
type p1Settlement struct {
	Rules db.PeriodInstance `json:"rules"`
	Ranks []p1Rank          `json:"ranks"`
}

// SettleP1Periods 冻结截止快照并投递离线领奖附件。持有实例锁时不回锁角色，领取才改角色余额。
func (s *GameService) SettleP1Periods(ctx context.Context) error {
	// 连续运行跨月时建立新实例，无需依赖重启触发。
	if err := s.seedP1Periods(ctx, LocalPlayNow(ctx)); err != nil {
		return err
	}
	var instances []db.PeriodInstance
	if err := db.GetIns().WithContext(ctx).Find(&instances).Error; err != nil {
		return err
	}
	for _, candidate := range instances {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var live db.PeriodInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", candidate.ID).First(&live).Error; err != nil {
				return err
			}
			cfg, err := s.freezeP1Period(tx, live, false)
			if err != nil {
				return err
			}
			wall, err := localPlayCycleNow(tx, cfg.ID, time.Now())
			if err != nil {
				return err
			}
			now := wall.UnixMilli()
			if now < cfg.EndAt {
				return nil
			}
			var count int64
			if err = tx.Model(&db.PeriodSettlement{}).Where("instance_id = ?", cfg.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			var progress []db.PeriodProgress
			if err = tx.Where("instance_id = ?", cfg.ID).Find(&progress).Error; err != nil {
				return err
			}
			// 并列依旧使用稳定玩家ID顺序；当天指标只纳入截止所在最后一个北京时间日期。
			cutoffDay := p1Day(time.UnixMilli(cfg.EndAt - 1))
			ranks := []p1Rank{}
			for _, row := range progress {
				value := row.Value
				if cfg.Mode == "daily" && row.LastDay != cutoffDay {
					value = 0
				}
				if value > 0 {
					ranks = append(ranks, p1Rank{PlayerID: row.PlayerID, Value: value})
				}
			}
			sort.Slice(ranks, func(i, j int) bool {
				if ranks[i].Value == ranks[j].Value {
					return ranks[i].PlayerID < ranks[j].PlayerID
				}
				return ranks[i].Value > ranks[j].Value
			})
			for i := range ranks {
				ranks[i].Rank = int32(i + 1)
				if cfg.Enabled && cfg.RankPlaces > 0 && ranks[i].Rank <= cfg.RankPlaces {
					if _, err = s.EnqueueRewardNotice(tx, ranks[i].PlayerID, "rank:"+cfg.ID, cfg.Title+"排行奖励", cfg.Source, cfg.RankRewards, cfg.RewardEndAt); err != nil {
						return err
					}
				}
			}
			// 停机跨越领取截止也保存过期投递与快照，恢复不延长窗口或额外补发。
			snapshot, err := json.Marshal(p1Settlement{Rules: cfg, Ranks: ranks})
			if err != nil {
				return err
			}
			return tx.Create(&db.PeriodSettlement{InstanceID: cfg.ID, SettledAt: now, Snapshot: snapshot}).Error
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ServeP1Periods 启动时恢复截止结算，后台使用有界时钟并随服务上下文退出。
func (s *GameService) ServeP1Periods(ctx context.Context) error {
	if err := s.SettleP1Periods(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.SettleP1Periods(ctx); err != nil {
				return err
			}
		}
	}
}
