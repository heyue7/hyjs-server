package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p1PositiveAmount(text string) (int64, error) {
	n, err := strconv.ParseInt(text, 10, 32)
	if err != nil || n <= 0 {
		return 0, errors.New("奖励数量无效")
	}
	return n, nil
}
func p1Day(at time.Time) string { return at.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02") }

// p1LockPeriods 沿主键正序取得完整窗口锁，避免过滤扫描先锁不同记录。
// 包含未启用实例，后续启用或同事务多种事实沿用已取得的锁顺序。
func p1LockPeriods(tx *gorm.DB) ([]db.PeriodInstance, error) {
	var rows []db.PeriodInstance
	if tx.Dialector.Name() == "mysql" {
		// 无过滤且强制PRIMARY，优化器不能在其他扫描之后filesort；一条语句保留固定取锁顺序。
		err := tx.Raw("SELECT * FROM p1_period_instances FORCE INDEX (PRIMARY) ORDER BY id ASC FOR UPDATE").Scan(&rows).Error
		return rows, err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&rows).Error
	return rows, err
}

// lockProfilePeriods 在角色行锁之后、域动作之前固定公共窗口加锁顺序。
func lockProfilePeriods(tx *gorm.DB) error {
	if s, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && s.P1Ready {
		_, err := p1LockPeriods(tx)
		return err
	}
	return nil
}

// RecordP1Event 与调用方业务同事务记录事实；来源重放不增加任何活动，同源篡改拒绝。
// 调用方先锁Profile。活动行锁只保护周期截止；结算流程不回锁Profile，避免逆序死锁。
func (s *GameService) RecordP1Event(tx *gorm.DB, playerID int64, sourceKey, kind string, amount int64, at time.Time) error {
	if playerID <= 0 || !p1SourceValid(sourceKey) || !p1SourceValid(kind) || len(kind) > 64 || amount <= 0 || at.UnixMilli() <= 0 || at.After(LocalPlayNow(tx.Statement.Context).Add(time.Minute)) {
		return errors.New("事件事实参数无效")
	}
	var prior db.EventFact
	err := tx.Where("player_id = ? AND source_key = ? AND kind = ?", playerID, sourceKey, kind).First(&prior).Error
	if err == nil {
		if prior.Amount != amount {
			return errors.New("事件来源金额与原事实不一致")
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	fact := db.EventFact{PlayerID: playerID, SourceKey: sourceKey, Kind: kind, Amount: amount, OccurredAt: at.UnixMilli(), Day: p1Day(at)}
	if err = tx.Create(&fact).Error; err != nil {
		return err
	}
	configs, err := p1LockPeriods(tx)
	if err != nil {
		return err
	}
	// 以拿到窗口锁的实际提交时间判断截止，晚于截止提交的动作保留事实但不回写截止快照。
	acceptedAt := max(at.UnixMilli(), LocalPlayNow(tx.Statement.Context).UnixMilli())
	for _, live := range configs {
		allowed, err := localPlayActivityEventVisible(tx, live, sourceKey)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}

		if !live.Enabled {
			continue
		}
		cfg, err := s.freezeP1Period(tx, live, false)
		if err != nil {
			return err
		}
		if cfg.Kind != kind {
			continue
		}
		if err = s.validateP1Period(cfg); err != nil {
			return err
		}
		if fact.OccurredAt < cfg.StartAt || fact.OccurredAt >= cfg.EndAt || acceptedAt < cfg.StartAt || acceptedAt >= cfg.EndAt {
			continue
		}
		cfg, err = s.freezeP1Period(tx, live, true)
		if err != nil {
			return err
		}
		var settled int64
		if err = tx.Model(&db.PeriodSettlement{}).Where("instance_id = ?", cfg.ID).Count(&settled).Error; err != nil {
			return err
		}
		if settled > 0 {
			continue
		}
		progress, frozen, err := s.p1Progress(tx, cfg, playerID)
		if err != nil {
			return err
		}
		switch frozen.Mode {
		case "cumulative":
			if progress.Value > math.MaxInt64-amount {
				return errors.New("活动累计溢出")
			}
			progress.Value += amount
		case "daily":
			// 延迟到达的前日事实只留审计，不能倒退当前日进度。
			if progress.LastDay > fact.Day {
				continue
			}
			if progress.LastDay != fact.Day {
				progress.Value = 0
			}
			if progress.Value > math.MaxInt64-amount {
				return errors.New("活动当天累计溢出")
			}
			progress.Value += amount
		case "single":
			progress.Value = max(progress.Value, amount)
		}
		progress.LastDay = fact.Day
		if progress.RewardID == 0 && progress.Value >= frozen.Goal {
			reward, err := s.EnqueueRewardNotice(tx, playerID, "activity:"+cfg.ID, frozen.Title, frozen.Source, frozen.Rewards, frozen.RewardEndAt)
			if err != nil {
				return err
			}
			progress.RewardID = reward.ID
		}
		if err = tx.Save(progress).Error; err != nil {
			return err
		}
	}
	return nil
}

// RecordP1Login 只在角色登录成功后记北京时间登录日，同日重连返回同一事实。
func (s *GameService) RecordP1Login(ctx context.Context, id int64) error {
	if !s.P1Ready {
		return nil
	}
	now := LocalPlayNow(ctx)
	return updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		if err := s.RecordP1Event(tx, id, "login:"+p1Day(now), constUtil.P1EventLoginDay, 1, now); err != nil {
			return err
		}
		if s.P12WantedReady {
			// 登录沿当前角色事务补投已结束自然日两榜；政策关闭时不产生奖励。
			if err := s.SyncP12WantedRewards(tx, id, now); err != nil {
				return err
			}
		}
		if err := s.RecordLocalPlayLogin(tx, id, now); err != nil {
			return err
		}
		return s.p1ReconcileLoginPeriods(tx, id, now)
	})
}

// RecordP1Save 在成功存档事务内衔接现有实际动作；失败和已有回执不产生第二个事实。
func (s *GameService) RecordP1Save(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
	if !s.P1Ready {
		return nil
	}
	request := tx.Statement.Context.Value(constUtil.GameRequestContextKey)
	if row.Player == nil || row.Game == nil {
		return nil
	}
	before, old, err := decodeProfile(row)
	if err != nil {
		return err
	}
	var recruit, copyWin, strength, purchases int64
	var actualCosts []*pb.BaseResource
	costSource := ""
	paidNoReward := false
	switch b := request.(type) {
	case *pb.CSWarriorRecruit:
		recruit = int64(max(0, len(game.Warriors)-len(old.Warriors)))
		current := game.Recruit[b.GetCostType()]
		prior := old.Recruit[b.GetCostType()]
		if current != nil && current.Response != nil && (prior == nil || current.Timestamp > prior.Timestamp) {
			actualCosts = current.Response.GetCost()
			costSource = fmt.Sprintf("recruit:%d:%d", b.GetCostType(), current.Timestamp)
		}
	case *pb.CSShopBuy:
		// 会员与后期专用商店由所属域按冻结价格/回执记录，原商品目录只拥有type1。
		if b.GetShopType() != 1 {
			return nil
		}
		// 商店回执重试提前返回、不执行save；所选价格行已由原业务SHARE锁冻结。
		var cfg db.ShopProduct
		if err = tx.Where("id = ?", b.GetShopItemId()).First(&cfg).Error; err != nil {
			return err
		}
		cost, err := parsePoolCost(cfg.Cost)
		if err != nil {
			return err
		}
		total := int64(cost.GetNum()) * int64(b.GetBuyCount())
		if total <= 0 || total > math.MaxInt32 {
			return errors.New("购买事件数量无效")
		}
		cost.Num = proto.Int32(int32(total))
		actualCosts = []*pb.BaseResource{cost}
		purchases = int64(b.GetBuyCount())
	case *pb.CSWarriorRecruitGuide:
		recruit = int64(max(0, len(game.Warriors)-len(old.Warriors)))
	case *pb.CSCopyFightEnd:
		if old.Run != nil && game.Run != nil && !old.Run.Complete && game.Run.Complete {
			copyWin = 1
		}
	case *pb.CSCopySweep:
		if game.CopySweepReceipt != nil && (old.CopySweepReceipt == nil || game.CopySweepReceipt.Timestamp > old.CopySweepReceipt.Timestamp) {
			copyWin = 1
		}
	case *pb.CSEquipStrengthen:
		paidNoReward = true
		for _, a := range old.Equipment {
			for _, v := range game.Equipment {
				if a.GetGuid() == b.GetGuid() && v.GetGuid() == a.GetGuid() {
					strength = int64(max(0, v.GetLevel()-a.GetLevel()))
				}
			}
		}
	case *pb.CSEquipRefine, *pb.CSEquipStar, *pb.CSEquipForge, *pb.CSWarriorTraining, *pb.CSWarriorStrengthen, *pb.CSWarriorBreached, *pb.CSWarriorTalent, *pb.CSWarriorAwake, *pb.CSTreasureStrengthen, *pb.CSTreasureRefine:
		paidNoReward = true
	default:
		return nil
	}
	now := LocalPlayNow(tx.Statement.Context)
	source := "operation:" + uuid.NewString()
	for _, event := range []struct {
		kind   string
		amount int64
	}{{constUtil.P1EventRecruit, recruit}, {constUtil.P1EventCopyWin, copyWin}, {constUtil.P1EventEquipStrengthen, strength}, {constUtil.P1EventPurchase, purchases}} {
		if event.amount > 0 {
			if err = s.RecordP1Event(tx, row.Id, source, event.kind, event.amount, now); err != nil {
				return err
			}
		}
	}
	if len(actualCosts) > 0 {
		if costSource == "" {
			costSource = source
		}
		if err = s.RecordP1Cost(tx, row.Id, costSource, actualCosts, now); err != nil {
			return err
		}
	}
	// 仅无返奖的培养动作可从余额差准确读取消耗；招募/商店使用原回执或已冻结价格记录原始扣款。
	if paidNoReward {
		if before.GetCoin() > player.GetCoin() {
			if err = s.RecordP1Event(tx, row.Id, source, constUtil.P1EventCoinSpent, before.GetCoin()-player.GetCoin(), now); err != nil {
				return err
			}
		}
		if before.GetDiamond() > player.GetDiamond() {
			if err = s.RecordP1Event(tx, row.Id, source, constUtil.P1EventDiamondSpent, before.GetDiamond()-player.GetDiamond(), now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *GameService) p1Progress(tx *gorm.DB, cfg db.PeriodInstance, id int64) (*db.PeriodProgress, db.PeriodInstance, error) {
	row := &db.PeriodProgress{InstanceID: cfg.ID, PlayerID: id}
	frozen := cfg
	err := tx.Where("instance_id = ? AND player_id = ?", cfg.ID, id).First(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		blob, e := json.Marshal(cfg)
		if e != nil {
			return nil, cfg, e
		}
		row.Snapshot = blob
		return row, cfg, nil
	}
	if err != nil {
		return nil, cfg, err
	}
	if err = json.Unmarshal(row.Snapshot, &frozen); err != nil {
		return nil, cfg, err
	}
	if frozen.ID != cfg.ID || row.Value < 0 {
		return nil, cfg, errors.New("活动冻结存档无效")
	}
	if err = s.validateP1Period(frozen); err != nil {
		return nil, cfg, err
	}
	return row, frozen, nil
}

// RecordP1Cost 记录已发生的原始资源消耗；返奖与扣款同时发生也不会把净额当消费。
func (s *GameService) RecordP1Cost(tx *gorm.DB, id int64, source string, costs []*pb.BaseResource, at time.Time) error {
	totals := map[string]int64{}
	for _, cost := range costs {
		if cost == nil || cost.GetNum() <= 0 || len(cost.GetId()) > 48 {
			return errors.New("实际消费资源无效")
		}
		key := cost.GetId()
		kind := "spend:" + key
		switch key {
		case "coin":
			kind = constUtil.P1EventCoinSpent
		case "diamond":
			kind = constUtil.P1EventDiamondSpent
		case "light", "point", "popularity", "energy", "stamina", "wantedAttTimes":
		default:
			if _, err := s.p4ParseCost(key + ",1"); err != nil {
				return errors.New("消费资源未恢复")
			}
		}
		if totals[kind] > math.MaxInt64-int64(cost.GetNum()) {
			return errors.New("消费累计溢出")
		}
		totals[kind] += int64(cost.GetNum())
	}
	// 固定顺序写入事实，避免同一操作跨多项消费时锁序随map变化。
	keys := make([]string, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, kind := range keys {
		if err := s.RecordP1Event(tx, id, source, kind, totals[kind], at); err != nil {
			return err
		}
	}
	return nil
}

// p1ReconcileLoginPeriods connects newly published open windows to real login days.
// The Profile lock serializes login and claims; the existing reward source keeps
// reconnects and the two claim entrances on one persisted delivery.
func (s *GameService) p1ReconcileLoginPeriods(tx *gorm.DB, id int64, now time.Time) error {
	periods, err := p1LockPeriods(tx)
	if err != nil {
		return err
	}
	for _, live := range periods {
		allowed, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}

		if !live.Enabled {
			continue
		}
		cfg, err := s.freezeP1Period(tx, live, false)
		if err != nil {
			return err
		}
		if localPlayPrivateCycleID(cfg.ID) != "" {
			continue
		}
		// Reconciliation only runs while counting is open. It cannot extend a
		// deadline or mutate a settled leaderboard or grace-period snapshot.
		if cfg.Kind != constUtil.P1EventLoginDay || cfg.Mode != "cumulative" || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			continue
		}
		var settled int64
		if err = tx.Model(&db.PeriodSettlement{}).Where("instance_id = ?", cfg.ID).Count(&settled).Error; err != nil {
			return err
		}
		if settled > 0 {
			continue
		}
		progress, frozen, err := s.p1Progress(tx, cfg, id)
		if err != nil {
			return err
		}
		if frozen.Kind != constUtil.P1EventLoginDay || frozen.Mode != "cumulative" || now.UnixMilli() < frozen.StartAt || now.UnixMilli() >= frozen.EndAt {
			continue
		}
		var facts []db.EventFact
		if err = tx.Where("player_id = ? AND kind = ? AND amount = 1 AND occurred_at >= ? AND occurred_at < ? AND occurred_at <= ?", id, constUtil.P1EventLoginDay, frozen.StartAt, frozen.EndAt, now.UnixMilli()).Order("occurred_at ASC, id ASC").Find(&facts).Error; err != nil {
			return err
		}
		// Accept only the exact source and Beijing day emitted by a successful
		// login. Multiple facts in one day never become extra login days.
		days := map[string]bool{}
		lastDay := progress.LastDay
		for _, fact := range facts {
			day := p1Day(time.UnixMilli(fact.OccurredAt))
			if fact.Day == day && fact.SourceKey == "login:"+day {
				days[day] = true
				lastDay = max(lastDay, day)
			}
		}
		value := int64(len(days))
		if value <= progress.Value {
			continue
		}
		if _, err = s.freezeP1Period(tx, live, true); err != nil {
			return err
		}
		progress.Value = value
		progress.LastDay = lastDay
		if progress.RewardID == 0 && value >= frozen.Goal {
			reward, err := s.EnqueueRewardNotice(tx, id, "activity:"+cfg.ID, frozen.Title, frozen.Source, frozen.Rewards, frozen.RewardEndAt)
			if err != nil {
				return err
			}
			progress.RewardID = reward.ID
		}
		if err = tx.Save(progress).Error; err != nil {
			return err
		}
	}
	return nil
}

// p1ReconcileCopyProgress counts committed wins inside the requested frozen open
// window, including wins before a late publication. It creates no new facts and
// never repairs closed or settled snapshots; existing deliveries remain unique.
func (s *GameService) p1ReconcileCopyProgress(tx *gorm.DB, progress *db.PeriodProgress, cfg db.PeriodInstance, now int64) error {
	if cfg.Kind != constUtil.P1EventCopyWin || cfg.Mode != "cumulative" || now < cfg.StartAt || now >= cfg.EndAt {
		return nil
	}
	var settled int64
	if err := tx.Model(&db.PeriodSettlement{}).Where("instance_id = ?", cfg.ID).Count(&settled).Error; err != nil {
		return err
	}
	if settled > 0 {
		return nil
	}
	var stats struct {
		Total   int64
		LastDay string
	}
	facts, err := localPlayActivityFactScope(tx, cfg.ID)
	if err != nil {
		return err
	}
	if err := facts.Model(&db.EventFact{}).Select("COALESCE(SUM(amount), 0) AS total, COALESCE(MAX(day), '') AS last_day").Where("player_id = ? AND kind = ? AND amount > 0 AND occurred_at >= ? AND occurred_at < ? AND occurred_at <= ?", progress.PlayerID, constUtil.P1EventCopyWin, cfg.StartAt, cfg.EndAt, now).Scan(&stats).Error; err != nil {
		return err
	}
	if stats.Total < 0 {
		return errors.New("活动累计溢出")
	}
	if stats.Total <= progress.Value {
		return nil
	}
	if _, err := s.freezeP1Period(tx, cfg, true); err != nil {
		return err
	}
	progress.Value = stats.Total
	progress.LastDay = max(progress.LastDay, stats.LastDay)
	if progress.RewardID == 0 && stats.Total >= cfg.Goal {
		reward, err := s.EnqueueRewardNotice(tx, progress.PlayerID, "activity:"+cfg.ID, cfg.Title, cfg.Source, cfg.Rewards, cfg.RewardEndAt)
		if err != nil {
			return err
		}
		progress.RewardID = reward.ID
	}
	return tx.Save(progress).Error
}
