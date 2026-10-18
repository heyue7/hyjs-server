package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
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

// InitializeP4CopyBenefits 建立独立门禁配置，后续运营启用须显式更新未冻结实例。
func (s *GameService) InitializeP4CopyBenefits(ctx context.Context) error {
	if !s.P4Ready {
		return errors.New("副本福利需要先初始化P4")
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P4CopyBenefitRule{}, &db.P4CopyBenefitGrant{}); err != nil {
		return err
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	cfg := db.P4CopyBenefitRule{ID: "p4:copy:" + start.Format("200601"), Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), WeekdayMask: 65, StartMinute: 0, EndMinute: 1440, CurrencyFactor: 2, DropFactor: 2, Source: "本地兼容策略（默认门禁关闭）：北京时间周末全天，当前已恢复主线金币/经验及已确认原物品/碎片/宝石/伙伴/装备/饰品掉落2倍。额外实例通过公共奖励实际创建新GUID；未知SpecialOdds和Pet池、日常/试炼奖励保持原域门禁。原服运营窗口和叠加顺序未知；本地先应用福利倍数，再应用料理奖励。"}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
		return err
	}
	// 登记有限后端能力与运营门禁，避免继续把已接入能力标作空缺。
	if err := tx.Model(&db.P4ActivityRegistry{}).Where("id=?", 13).Updates(map[string]any{"status": "确认原主线全类型首掉福利，具名本地19～21晚间实例或既有运营窗口；原私服窗口/SpecialOdds未知", "protocols": "CSActivityList/CSActivityInfo/CSCopyFightEnd/CSCopySweep"}).Error; err != nil {
		return err
	}
	return s.seedP4CopyWindow(tx, time.Now())
}

func validateP4CopyBenefit(cfg db.P4CopyBenefitRule) error {
	if cfg.ID == "" || len(cfg.ID) > 64 || cfg.StartAt < 1 || cfg.StartAt >= cfg.EndAt || cfg.WeekdayMask < 1 || cfg.WeekdayMask > 127 || cfg.StartMinute < 0 || cfg.EndMinute > 1440 || cfg.StartMinute >= cfg.EndMinute || cfg.CurrencyFactor < 1 || cfg.CurrencyFactor > constUtil.P4CopyBenefitsMaximumFactor || cfg.DropFactor < 1 || cfg.DropFactor > constUtil.P4CopyBenefitsMaximumFactor {
		return errors.New("副本福利窗口或倍数无效")
	}
	return nil
}

// p4CopyBenefitRule 在角色事务中锁定已发布窗口，重放由存档结算回执和投递来源保护。
func (s *GameService) p4CopyBenefitRule(tx *gorm.DB, now time.Time, freeze bool) (*db.P4CopyBenefitRule, error) {
	if err := s.seedP4CopyWindow(tx, now); err != nil {
		return nil, err
	}
	var rows []db.P4CopyBenefitRule
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("enabled=?", true).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var selected *db.P4CopyBenefitRule
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	minute := int32(local.Hour()*60 + local.Minute())
	for _, live := range rows {
		allowed, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		cfg := live
		if len(live.Frozen) > 0 {
			if json.Unmarshal(live.Frozen, &cfg) != nil || cfg.ID != live.ID {
				return nil, errors.New("副本福利冻结规则损坏")
			}
			cfg.Enabled = live.Enabled
		}
		if err := validateP4CopyBenefit(cfg); err != nil {
			return nil, err
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt || cfg.WeekdayMask&(1<<uint(local.Weekday())) == 0 || minute < cfg.StartMinute || minute >= cfg.EndMinute {
			continue
		}
		if selected != nil {
			return nil, errors.New("副本福利运营窗口重叠")
		}
		if freeze && len(live.Frozen) == 0 {
			raw, err := json.Marshal(cfg)
			if err != nil {
				return nil, err
			}
			if err = tx.Model(&live).Update("frozen", raw).Error; err != nil {
				return nil, err
			}
		}
		selected = &cfg
	}
	return selected, nil
}

// ApplyP4CopyCurrencyBonus 在料理加成之前乘算，调用方使用同一事务保存结算回执。
func (s *GameService) ApplyP4CopyCurrencyBonus(tx *gorm.DB, now time.Time, coin, exp int64) (int64, int64, error) {
	cfg, err := s.p4CopyBenefitRule(tx, now, true)
	if err != nil {
		return 0, 0, err
	}
	factor := int64(1)
	if cfg != nil {
		factor = int64(cfg.CurrencyFactor)
	}
	if coin < 0 || exp < 0 || coin > math.MaxInt32/factor || exp > math.MaxInt32/factor {
		return 0, 0, errors.New("副本福利金币经验超出协议范围")
	}
	return coin * factor, exp * factor, nil
}

// ApplyP4CopyDropBonus doubles confirmed original resources and creates independent bonus instances.
// It does not draw unknown SpecialOdds or pet pools.
func (s *GameService) ApplyP4CopyDropBonus(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, source string, base *pb.ResultObject, now time.Time) (*pb.ResultObject, []*pb.BaseResource, error) {
	if id < 1 || id != p.GetPlayerId() || !p1SourceValid(source) || len(source) > 128 || base == nil {
		return nil, nil, errors.New("副本福利投递来源无效")
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(base)
	if err != nil {
		return nil, nil, err
	}
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	var prior db.P4CopyBenefitGrant
	err = tx.Where("player_id=? AND source_key=?", id, source).First(&prior).Error
	if err == nil {
		if prior.BaseHash != fingerprint {
			return nil, nil, errors.New("副本福利来源内容冲突")
		}
		bonus := &pb.ResultObject{}
		if err = proto.Unmarshal(prior.Bonus, bonus); err != nil {
			return nil, nil, err
		}
		return bonus, p4CopyBonusResources(bonus), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	cfg, err := s.p4CopyBenefitRule(tx, now, true)
	if err != nil {
		return nil, nil, err
	}
	if cfg == nil || cfg.DropFactor == 1 {
		return &pb.ResultObject{}, nil, nil
	}
	counts := map[string]int64{}
	if len(base.GetPets()) > 0 {
		return nil, nil, errors.New(constUtil.P4CopyDropTypeError)
	}
	for _, item := range base.GetSimps() {
		if item == nil || item.GetNum() < 1 {
			return nil, nil, errors.New(constUtil.P4CopyDropTypeError)
		}
		key := item.GetId()
		valid := (strings.HasPrefix(key, "item_") && s.table("Item", "itemId", key) != nil) || (strings.HasPrefix(key, "frag_") && s.table("Frag", "id", key) != nil) || (strings.HasPrefix(key, "MineItem_") && s.table("MineSkill", "MineItemID", key) != nil)
		if !valid || counts[key] > math.MaxInt32-int64(item.GetNum()) {
			return nil, nil, errors.New(constUtil.P4CopyDropTypeError)
		}
		counts[key] += int64(item.GetNum())
	}
	// Confirmed instances contribute one each; GrantFixedReward creates new GUIDs.
	seen := map[string]bool{}
	addInstance := func(key, guid, table, field string) error {
		if guid == "" || seen[guid] || s.table(table, field, key) == nil || counts[key] >= math.MaxInt32 {
			return errors.New(constUtil.P4CopyDropTypeError)
		}
		seen[guid] = true
		counts[key]++
		return nil
	}
	for _, warrior := range base.GetWars() {
		if warrior == nil || !strings.HasPrefix(warrior.GetId(), "hero_") {
			return nil, nil, errors.New(constUtil.P4CopyDropTypeError)
		}
		if err := addInstance(warrior.GetId(), warrior.GetGuid(), "Hero", "id"); err != nil {
			return nil, nil, err
		}
	}
	for _, gear := range base.GetEquips() {
		if gear == nil || !strings.HasPrefix(gear.GetEquipId(), "equip_") {
			return nil, nil, errors.New(constUtil.P4CopyDropTypeError)
		}
		if err := addInstance(gear.GetEquipId(), gear.GetGuid(), "Equip", "id"); err != nil {
			return nil, nil, err
		}
	}
	for _, treasure := range base.GetTreas() {
		if treasure == nil || !strings.HasPrefix(treasure.GetTreasureId(), "trea_") {
			return nil, nil, errors.New(constUtil.P4CopyDropTypeError)
		}
		if err := addInstance(treasure.GetTreasureId(), treasure.GetGuid(), "Treasure", "id"); err != nil {
			return nil, nil, err
		}
	}
	var ids []string
	for key := range counts {
		ids = append(ids, key)
	}
	sort.Strings(ids)
	var content []string
	var total int64
	for _, key := range ids {
		amount := counts[key] * int64(cfg.DropFactor-1)
		if amount < 1 || amount > math.MaxInt32 {
			return nil, nil, errors.New("副本福利掉落数量溢出")
		}
		content = append(content, fmt.Sprintf("%s,%d", key, amount))
		total += amount
	}
	if len(content) == 0 {
		return &pb.ResultObject{}, nil, nil
	}
	// 公共奖励替换内部快照；保留调用方仍在推进的副本指针，确保Settled和进度写回同一对象。
	var (
		run          = g.Run
		copyProgress = g.Copy
	)
	bonus, err := s.grantCopyBattleReward(tx, p, g, strings.Join(content, ";"), "bonus", func(fit string) (*pb.ResultObject, error) {
		return s.GrantFixedReward(p, g, fit)
	})
	if err != nil {
		return nil, nil, err
	}
	g.Run = run
	g.Copy = copyProgress
	raw, err = proto.Marshal(bonus)
	if err != nil {
		return nil, nil, err
	}
	if err = tx.Create(&db.P4CopyBenefitGrant{PlayerID: id, SourceKey: source, RuleID: cfg.ID, BaseHash: fingerprint, Bonus: raw}).Error; err != nil {
		return nil, nil, err
	}
	if err = s.RecordP1Event(tx, id, "copy-bonus:"+source, constUtil.P4CopyBenefitsEventKind, total, now); err != nil {
		return nil, nil, err
	}
	return bonus, p4CopyBonusResources(bonus), nil
}

func p4CopyBonusResources(bonus *pb.ResultObject) []*pb.BaseResource {
	var resources []*pb.BaseResource
	for _, item := range bonus.GetSimps() {
		resources = append(resources, &pb.BaseResource{Id: item.Id, Num: item.Num})
	}
	for _, warrior := range bonus.GetWars() {
		resources = append(resources, &pb.BaseResource{Id: warrior.Id, Guid: warrior.Guid, Num: proto.Int32(1)})
	}
	for _, gear := range bonus.GetEquips() {
		resources = append(resources, &pb.BaseResource{Id: gear.EquipId, Guid: gear.Guid, Num: proto.Int32(1)})
	}
	for _, treasure := range bonus.GetTreas() {
		resources = append(resources, &pb.BaseResource{Id: treasure.TreasureId, Guid: treasure.Guid, Num: proto.Int32(1)})
	}
	return resources
}

// P4CopyBenefitsInfo 沿用APK硬编码allCopyDouble，静态活动页没有独立领奖动作。
func (s *GameService) P4CopyBenefitsInfo(ctx context.Context, now time.Time) ([]proto.Message, error) {
	info := &pb.SCActivityInfo{ActivityId: proto.String(constUtil.P4CopyBenefitsActivityID), Param: proto.String("1"), Information: proto.String("副本福利当前未开放")}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		cfg, err := s.p4CopyBenefitRule(tx, now, false)
		if err != nil {
			return err
		}
		// An explicitly summoned private window remains readable outside its
		// reward hours. The settlement selector above keeps the original gate.
		if cfg == nil {
			cycle, e := localPlayEntryCycle(tx, "CopyBenefits")
			if e != nil {
				return e
			}
			if cycle != nil && now.UnixMilli() >= cycle.StartAt && now.UnixMilli() < cycle.EndAt {
				var snapshot localPlayScopedSnapshot
				if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || len(snapshot.Copy) != 1 {
					return errLocalPlayActivitiesInvalid
				}
				copyRule := snapshot.Copy[0]
				var live db.P4CopyBenefitRule
				if e := tx.First(&live, "id=?", copyRule.ID).Error; e != nil {
					return e
				}
				if !live.Enabled {
					return nil
				}
				if e := validateP4CopyBenefit(copyRule); e != nil {
					return e
				}
				if copyRule.Enabled {
					cfg = &copyRule
				}
			}
		}
		if cfg != nil {
			info.StartTime = proto.Int64(cfg.StartAt)
			info.EndTime = proto.Int64(cfg.EndAt)
			info.RewardEndTime = proto.Int64(cfg.EndAt)
			info.Param = proto.String(fmt.Sprint(cfg.CurrencyFactor))
			info.Information = proto.String(fmt.Sprintf("今日%d:%02d～%d:%02d，主线金币和经验%d倍，战利品数量%d倍。", cfg.StartMinute/60, cfg.StartMinute%60, cfg.EndMinute/60, cfg.EndMinute%60, cfg.CurrencyFactor, cfg.DropFactor))
		}
		return nil
	})
	return []proto.Message{info}, err
}

func (s *GameService) P4CopyBenefitsTabs(ctx context.Context, now time.Time) ([]*pb.ActivityTab, error) {
	r, err := s.P4CopyBenefitsInfo(ctx, now)
	if err != nil {
		return nil, err
	}
	info := r[0].(*pb.SCActivityInfo)
	if info.GetEndTime() == 0 {
		return nil, nil
	}
	return []*pb.ActivityTab{{ActivityId: proto.String(constUtil.P4CopyBenefitsActivityID), Name: proto.String("副本福利"), Param: proto.String(info.GetParam()), Status: proto.Int32(0), Order: proto.Int32(40)}}, nil
}

// P4BoonInfo 接原CopyBenefit页面57053。回包ActivityId同时决定原前往按钮的导航。
// 查询只读取实际福利/轮换，不重置奖池、免费次数或角色存档。
func (s *GameService) P4BoonInfo(ctx context.Context, request *pb.CSCopyBoonInfo, now time.Time) ([]proto.Message, error) {
	key := request.GetActivityId()
	if key == constUtil.P4CopyBenefitsActivityID || strings.HasPrefix(key, "allCopyDouble_p4_lp-") {
		if localPlayPrivateCycleID(key) != "" {
			allowed, err := localPlayActivityAccess(db.GetIns().WithContext(ctx), key, true)
			if err != nil {
				return nil, err
			}
			if !allowed {
				return nil, errLocalPlayActivitiesInvalid
			}
		}
		messages, err := s.P4CopyBenefitsInfo(ctx, now)
		if err != nil {
			return nil, err
		}
		info := messages[0].(*pb.SCActivityInfo)
		return []proto.Message{&pb.SCCopyBoonInfo{ActivityId: proto.String("copyDouble"), Information: info.Information}}, nil
	}
	if key != "warriorWeightImprove" {
		return nil, errors.New(constUtil.P4NativeBoonUnavailable)
	}
	var information string
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		pool, end, err := s.loadP4Rotation(tx, 2, now)
		if err != nil {
			return err
		}
		var total, orange int64
		for _, item := range pool {
			total += item.Weight
			if s.table("Hero", "id", item.ResourceID)["quality"] == "20" {
				orange += item.Weight
			}
		}
		if total <= 0 || orange <= 0 {
			return errors.New(constUtil.P4NativeBoonRotationInvalid)
		}
		information = fmt.Sprintf("当前强者招募橙色角色组权重为%.0f%%。本期结束：%s。原招募费用、免费次数和保底保持，请在招募页面查看当前角色预览。", float64(orange)*100/float64(total), time.UnixMilli(end).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02 15:04"))
		return nil
	})
	// 原type31按钮直接打开招募ShopPage；单一导航值保留原前往按钮。
	return []proto.Message{&pb.SCCopyBoonInfo{ActivityId: proto.String("warriorWeightImprove"), Information: proto.String(information)}}, err
}
