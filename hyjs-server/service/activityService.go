package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

// InitializeActivities 补建独立表并只插入缺失种子，重启保留运营SQL调整。
func (s *GameService) InitializeActivities(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.ActivityRules{}, &db.ActivityReward{}, &db.ActivityState{}, &db.ActivityLoginState{}); err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if len(s.Tables["OpenServerFund"]) == 0 || len(s.Tables["EveryoneBenefit"]) == 0 {
			return errors.New("基金原表未加载")
		}
		zone := time.FixedZone("UTC+8", 8*3600)
		cfg := db.ActivityRules{ID: "open_server_fund", Enabled: true, MinVIP: 2, DiamondCost: 1000, StartAt: time.Date(2026, 10, 11, 0, 0, 0, 0, zone).UnixMilli(), EndAt: time.Date(2026, 11, 11, 0, 0, 0, 0, zone).UnixMilli(), RewardEndAt: time.Date(2026, 11, 18, 0, 0, 0, 0, zone).UnixMilli(), Source: "APK：VIP2、1000钻；原表等级/人数奖励。时间窗为本地服SQL规则，UTC+8固定2026-10-11至11-11，领取截止11-18；未购买角色也可领取全民福利。"}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			return s.seedLoginActivity(tx, cfg)
		}
		for _, spec := range []struct{ table, key, kind string }{{"OpenServerFund", "level", "openServerFund"}, {"EveryoneBenefit", "count", "everyoneBenefit"}} {
			for _, row := range s.Tables[spec.table] {
				n, err := strconv.ParseInt(row[spec.key], 10, 32)
				if err != nil || n <= 0 {
					return errors.New("活动原表门槛无效")
				}
				if err := tx.Create(&db.ActivityReward{ActivityID: cfg.ID, RewardType: spec.kind, Threshold: int32(n), Reward: row["reward"]}).Error; err != nil {
					return err
				}
			}
		}
		return s.seedLoginActivity(tx, cfg)
	})
}

// openServerFund 锁定真实角色后完成查询、购买或领取，存档与领取标记原子提交。
func (s *GameService) openServerFund(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var result proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		var cfg db.ActivityRules
		if err := tx.Where("id = ?", "open_server_fund").First(&cfg).Error; err != nil {
			return err
		}
		if cfg.MinVIP < 0 || cfg.DiamondCost <= 0 || cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt {
			return errors.New("活动SQL配置无效")
		}
		now := LocalPlayNow(ctx).UnixMilli()
		if !cfg.Enabled || now < cfg.StartAt || now >= cfg.RewardEndAt {
			return errors.New("活动未开放或领取已截止")
		}
		var state db.ActivityState
		err = tx.Where("activity_id = ? AND player_id = ?", cfg.ID, id).First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state = db.ActivityState{ActivityID: cfg.ID, PlayerID: id, Claims: []byte("{}")}
		} else if err != nil {
			return err
		}
		claims := map[string]bool{}
		if len(state.Claims) > 0 {
			if err := json.Unmarshal(state.Claims, &claims); err != nil {
				return err
			}
		}
		if claims == nil {
			return errors.New("活动领取存档无效")
		}
		var rewards []db.ActivityReward
		if err := tx.Where("activity_id = ?", cfg.ID).Order("threshold ASC").Find(&rewards).Error; err != nil {
			return err
		}
		if len(rewards) == 0 {
			return errors.New("活动奖励未配置")
		}
		// MySQL使用READ COMMITTED读取已提交购买人数；各角色购买以独立唯一主键保存。
		var count int64
		if err := tx.Model(&db.ActivityState{}).Where("activity_id = ? AND bought = ?", cfg.ID, true).Count(&count).Error; err != nil {
			return err
		}
		if count > math.MaxInt32 {
			return errors.New("购买人数超出协议范围")
		}
		buyStatus := int32(0)
		if player.GetVip() >= cfg.MinVIP {
			buyStatus = 1
		}
		if state.Bought {
			buyStatus = 2
		}
		var cost []*pb.BaseResource
		var gain *pb.ResultObject
		switch req := request.(type) {
		case *pb.CSOpenServerFundInfo:
		case *pb.CSBuyOpenServerFund:
			if now >= cfg.EndAt {
				return errors.New("基金购买已结束")
			}
			if state.Bought {
				return errors.New("基金已经购买")
			}
			if player.GetVip() < cfg.MinVIP {
				return errors.New("VIP等级不足")
			}
			if player.GetDiamond() < int64(cfg.DiamondCost) {
				return errors.New("钻石不足")
			}
			if count == math.MaxInt32 {
				return errors.New("购买人数超出协议范围")
			}
			player.Diamond = proto.Int64(player.GetDiamond() - int64(cfg.DiamondCost))
			state.Bought = true
			count++
			buyStatus = 2
			cost = []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(cfg.DiamondCost)}}
		case *pb.CSOpenServerFundReward:
			kind := req.GetRewardType()
			if kind != "openServerFund" && kind != "everyoneBenefit" {
				return errors.New("基金奖励类型无效")
			}
			var selected *db.ActivityReward
			for i := range rewards {
				if rewards[i].RewardType == kind && rewards[i].Threshold == req.GetIndex() {
					selected = &rewards[i]
					break
				}
			}
			if selected == nil {
				return errors.New("奖励档位不存在")
			}
			key := kind + ":" + strconv.Itoa(int(req.GetIndex()))
			if claims[key] {
				return errors.New("奖励已领取")
			}
			if kind == "openServerFund" && (!state.Bought || player.GetLevel() < selected.Threshold) {
				return errors.New("基金未购买或角色等级不足")
			}
			if kind == "everyoneBenefit" && count < int64(selected.Threshold) {
				return errors.New("真实购买人数不足")
			}
			// 使用已有固定奖励引擎，物品、装备和余额与领取状态一起落库。
			content := selected.Reward
			// 装备奖励沿用实际背包容量，超容量保持领取标记和余额原状。
			var equipCount int64
			for _, part := range strings.Split(content, ";") {
				pair := strings.Split(part, ",")
				if len(pair) == 2 && strings.HasPrefix(pair[0], "equip_") {
					n, e := strconv.ParseInt(pair[1], 10, 32)
					if e != nil || n <= 0 {
						return errors.New("活动装备奖励数量无效")
					}
					equipCount += n
				}
			}
			if int64(len(game.Equipment))+equipCount > int64(player.GetEquipBagSize()) {
				return errors.New("装备背包已满")
			}

			// 经验水晶沿用Hero类实例，生成独立GUID供伙伴升级选择。
			if strings.HasPrefix(content, "hero_exp_") {
				pair := strings.Split(content, ",")
				if len(pair) != 2 {
					return errors.New("活动经验材料配置无效")
				}
				hero := s.table("Hero", "id", pair[0])
				n, e := strconv.ParseInt(pair[1], 10, 32)
				if hero == nil || hero["heroClass"] != "99" || e != nil || n <= 0 {
					return errors.New("活动经验材料配置无效")
				}
				if int64(len(game.Warriors))+n > int64(player.GetWarriorBagSize()) {
					return errors.New("伙伴背包已满")
				}
				gain = &pb.ResultObject{}
				for i := int64(0); i < n; i++ {
					token := make([]byte, 16)
					if _, e := rand.Read(token); e != nil {
						return e
					}
					guid := hex.EncodeToString(token)
					for _, w := range game.Warriors {
						if w.GetGuid() == guid {
							return errors.New("经验材料GUID冲突")
						}
					}
					m, e := model.GameMessage("Warrior", map[string]any{"id": pair[0], "guid": guid, "level": 1, "exp": 0, "talent": 0, "training": map[string]any{}, "tempTraining": map[string]any{}, "breached": map[string]any{"level": 1}, "awake": map[string]any{}})
					if e != nil {
						return e
					}
					w := m.(*pb.Warrior)
					game.Warriors = append(game.Warriors, w)
					gain.Wars = append(gain.Wars, proto.Clone(w).(*pb.Warrior))
				}
			} else {
				gain, err = s.chapterBoxGain(player, game, content)
				if err != nil {
					return err
				}
			}

			claims[key] = true
		default:
			return errors.New("基金请求类型无效")
		}
		var fund, benefit []*pb.ActivityItem
		for _, reward := range rewards {
			if reward.Threshold <= 0 {
				return errors.New("活动奖励门槛无效")
			}
			var resources []*pb.BaseResource
			for _, part := range strings.Split(reward.Reward, ";") {
				pair := strings.Split(part, ",")
				if len(pair) != 2 {
					return errors.New("活动奖励配置无效")
				}
				n, e := strconv.ParseInt(pair[1], 10, 32)
				if e != nil || n <= 0 {
					return errors.New("活动奖励数量无效")
				}
				resources = append(resources, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(n))})
			}
			status := int32(0)
			if reward.RewardType == "openServerFund" && state.Bought && player.GetLevel() >= reward.Threshold {
				status = 1
			}
			if reward.RewardType == "everyoneBenefit" && count >= int64(reward.Threshold) {
				status = 1
			}
			if claims[reward.RewardType+":"+strconv.Itoa(int(reward.Threshold))] {
				status = 2
			}
			item := &pb.ActivityItem{Index: proto.Int32(reward.Threshold), RewardList: resources, Status: proto.Int32(status)}
			switch reward.RewardType {
			case "openServerFund":
				fund = append(fund, item)
			case "everyoneBenefit":
				benefit = append(benefit, item)
			default:
				return errors.New("SQL奖励类型无效")
			}
		}
		switch req := request.(type) {
		case *pb.CSOpenServerFundInfo:
			result = &pb.SCOpenServerFundInfo{BuyStatus: proto.Int32(buyStatus), BuyCount: proto.Int32(int32(count)), OpenServerFundList: fund, EveryoneBenefitList: benefit}
		case *pb.CSBuyOpenServerFund:
			result = &pb.SCBuyOpenServerFund{BuyStatus: proto.Int32(buyStatus), BuyCount: proto.Int32(int32(count)), Cost: cost, OpenServerFundList: fund, EveryoneBenefitList: benefit}
		case *pb.CSOpenServerFundReward:
			list := fund
			if req.GetRewardType() == "everyoneBenefit" {
				list = benefit
			}
			result = &pb.SCOpenServerFundReward{Get: gain, RefreshList: list}
		}
		if _, ok := request.(*pb.CSOpenServerFundInfo); ok {
			return nil
		}
		state.Claims, err = json.Marshal(claims)
		if err != nil {
			return err
		}
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		return saveProfile(tx, &row, player, game)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

// seedLoginActivity 只写入缺失活动；奖档与周期均明确标注为本地服SQL配置。
func (s *GameService) seedLoginActivity(tx *gorm.DB, fund db.ActivityRules) error {
	cfg := db.ActivityRules{ID: "totalLogin_local_20261011", Enabled: false, StartAt: fund.StartAt, EndAt: fund.EndAt, RewardEndAt: fund.RewardEndAt, Source: "本地服SQL规则：UTC+8按成功角色登录日期累计，每日仅计一次；同日重连不增加，缺席不补记。1/2/3/5/7天固定全领奖励；与月签到、豪华签到、限时签到独立。"}
	created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg)
	if created.Error != nil {
		return created.Error
	}
	if created.RowsAffected == 0 {
		return nil
	}
	for _, r := range []db.ActivityReward{{Threshold: 1, Reward: "coin,10000"}, {Threshold: 2, Reward: "item_001,1"}, {Threshold: 3, Reward: "diamond,50"}, {Threshold: 5, Reward: "coin,30000"}, {Threshold: 7, Reward: "diamond,100"}} {
		r.ActivityID = cfg.ID
		r.RewardType = "totalLogin"
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
	}
	return nil
}

// RecordActivityLogin 由成功CSPlayerLogin调用，按真实日期记录一次，查询活动不会增加进度。
func (s *GameService) RecordActivityLogin(ctx context.Context, id int64) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err := RecordOpenServerLogin(tx, id); err != nil {
			return err
		}
		if err := s.RefreshOpenServerProgress(tx, player, game); err != nil {
			return err
		}
		var cfg db.ActivityRules
		if err := tx.Where("id = ?", "totalLogin_local_20261011").First(&cfg).Error; err != nil {
			return err
		}
		now := LocalPlayNow(ctx)
		if !cfg.Enabled || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			return nil
		}
		if cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt {
			return errors.New("登录活动周期无效")
		}
		var state db.ActivityLoginState
		err = tx.Where("activity_id = ? AND player_id = ?", cfg.ID, id).First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state = db.ActivityLoginState{ActivityID: cfg.ID, PlayerID: id, Claims: []byte("{}")}
		} else if err != nil {
			return err
		}
		day := now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
		if state.LastDay >= day {
			return nil
		}
		if state.Days < 0 || state.Days == math.MaxInt32 {
			return errors.New("登录活动天数无效")
		}
		state.LastDay = day
		state.Days++
		return tx.Save(&state).Error
	})
}

// loginActivity 沿用totalLogin独立入口，服务器按SQL发奖并校验客户端全领资源列表。
func (s *GameService) loginActivity(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var result proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		var err error
		result, err = s.legacyLoginActivityApply(tx, id, player, game, request, LocalPlayNow(ctx).UnixMilli())
		return err
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

// legacyLoginActivityApply keeps the original local login state and claims in the caller transaction.
func (s *GameService) legacyLoginActivityApply(tx *gorm.DB, id int64, player *pb.Player, game *model.GameState, request proto.Message, now int64) (proto.Message, error) {
	var result proto.Message
	var err error
	var cfg db.ActivityRules
	if err := tx.Where("id = ?", "totalLogin_local_20261011").First(&cfg).Error; err != nil {
		return nil, err
	}
	var activityID string
	switch req := request.(type) {
	case *pb.CSActivityInfo:
		activityID = req.GetActivityId()
	case *pb.CSActivityReward:
		activityID = req.GetActivityId()
	default:
		return nil, errors.New("登录活动请求无效")
	}
	if activityID != cfg.ID {
		return nil, errors.New("当前仅恢复独立累计登录活动")
	}
	if cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt {
		return nil, errors.New("登录活动周期无效")
	}
	if !cfg.Enabled || now < cfg.StartAt || now >= cfg.RewardEndAt {
		return nil, errors.New("登录活动未开放或领取已截止")
	}
	var state db.ActivityLoginState
	err = tx.Where("activity_id = ? AND player_id = ?", cfg.ID, id).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state = db.ActivityLoginState{ActivityID: cfg.ID, PlayerID: id, Claims: []byte("{}")}
	} else if err != nil {
		return nil, err
	}
	if state.Days < 0 {
		return nil, errors.New("登录活动存档无效")
	}
	claims := map[string]bool{}
	if len(state.Claims) > 0 {
		if err := json.Unmarshal(state.Claims, &claims); err != nil {
			return nil, err
		}
	}
	if claims == nil {
		return nil, errors.New("登录活动领取存档无效")
	}
	var rewards []db.ActivityReward
	if err := tx.Where("activity_id = ? AND reward_type = ?", cfg.ID, "totalLogin").Order("threshold ASC").Find(&rewards).Error; err != nil {
		return nil, err
	}
	if len(rewards) == 0 {
		return nil, errors.New("登录活动奖励缺失")
	}
	// 构造固定全领档位，任何畸形SQL都在发奖前阻断。
	var items []*pb.ActivityItem
	for _, r := range rewards {
		if r.Threshold <= 0 {
			return nil, errors.New("登录活动门槛无效")
		}
		resources, err := activityLoginResources(r.Reward)
		if err != nil {
			return nil, err
		}
		status := int32(0)
		if state.Days >= r.Threshold {
			status = 1
		}
		if claims[strconv.Itoa(int(r.Threshold))] {
			status = 2
		}
		left := state.Days
		if left > r.Threshold {
			left = r.Threshold
		}
		items = append(items, &pb.ActivityItem{Index: proto.Int32(r.Threshold), Value: proto.String(strconv.Itoa(int(r.Threshold))), RewardList: resources, Status: proto.Int32(status), Left: proto.Int32(left), Total: proto.Int32(r.Threshold), PickOneOrGetAll: proto.Int32(1)})
	}
	switch req := request.(type) {
	case *pb.CSActivityInfo:
		result = &pb.SCActivityInfo{ActivityId: proto.String(cfg.ID), ItemList: items, StartTime: proto.Int64(cfg.StartAt), EndTime: proto.Int64(cfg.EndAt), RewardEndTime: proto.Int64(cfg.RewardEndAt), Param: proto.String("1"), Information: proto.String(cfg.Source)}
		return result, nil
	case *pb.CSActivityReward:
		var selected *pb.ActivityItem
		var content string
		for i, item := range items {
			if item.GetIndex() == req.GetIndex() {
				selected = item
				content = rewards[i].Reward
				break
			}
		}
		if selected == nil || selected.GetStatus() != 1 {
			return nil, errors.New("登录天数不足或奖励已领取")
		}
		actual := req.GetGetResources()
		expected := selected.GetRewardList()
		if len(actual) != len(expected) {
			return nil, errors.New("登录活动奖励列表已变化，请刷新")
		}
		for i, r := range actual {
			if r.GetId() != expected[i].GetId() || r.GetNum() != expected[i].GetNum() || r.GetGuid() != "" {
				return nil, errors.New("登录活动奖励列表已变化，请刷新")
			}
		}
		gain, err := s.chapterBoxGain(player, game, content)
		if err != nil {
			return nil, err
		}
		claims[strconv.Itoa(int(req.GetIndex()))] = true
		selected.Status = proto.Int32(2)
		state.Claims, err = json.Marshal(claims)
		if err != nil {
			return nil, err
		}
		if err := tx.Save(&state).Error; err != nil {
			return nil, err
		}
		result = &pb.SCActivityReward{ActivityId: proto.String(cfg.ID), ItemList: items, Get: gain}
	}
	return result, nil
}

// activityLoginResources 限定本地登录活动为已恢复的固定基础资源奖励。
func activityLoginResources(content string) ([]*pb.BaseResource, error) {
	var result []*pb.BaseResource
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return nil, errors.New("登录奖励SQL格式无效")
		}
		if pair[0] != "coin" && pair[0] != "diamond" && !strings.HasPrefix(pair[0], "item_") {
			return nil, errors.New("登录奖励资源类型尚未恢复")
		}
		n, e := strconv.ParseInt(pair[1], 10, 32)
		if e != nil || n <= 0 {
			return nil, errors.New("登录奖励数量无效")
		}
		result = append(result, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(n))})
	}
	return result, nil
}

// AvailableActivities 返回当前固定窗口内可见的基金与累计登录入口，由共享列表统一聚合。
func (s *GameService) AvailableActivities(ctx context.Context, id int64) ([]*pb.ActivityTab, error) {
	var rows []db.ActivityRules
	if err := db.GetIns().WithContext(ctx).Where("id IN ?", []string{"open_server_fund", "totalLogin_local_20261011"}).Find(&rows).Error; err != nil {
		return nil, err
	}
	var profile db.Profile
	if err := db.GetIns().WithContext(ctx).First(&profile, id).Error; err != nil {
		return nil, err
	}
	if _, _, err := decodeProfileForUpdate(&profile); err != nil {
		return nil, err
	}
	now := LocalPlayNow(ctx).UnixMilli()
	var result []*pb.ActivityTab
	for _, key := range []string{"open_server_fund", "totalLogin_local_20261011"} {
		for _, cfg := range rows {
			if cfg.ID != key || !cfg.Enabled || now < cfg.StartAt || now >= cfg.RewardEndAt {
				continue
			}
			if cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt {
				return nil, errors.New("活动展示周期无效")
			}
			name, entry, order := "开服基金", "openServerFund_local_20261011", int32(2)
			if key == "totalLogin_local_20261011" {
				if s.P1Ready {
					continue
				}
				name = "累计登录"
				entry = key
				order = 3
			}
			result = append(result, &pb.ActivityTab{ActivityId: proto.String(entry), Name: proto.String(name), Order: proto.Int32(order), Status: proto.Int32(0), Param: proto.String("本地服活动")})
		}
	}
	instances, err := s.P1ActivityTabs(ctx, id)
	if err != nil {
		return nil, err
	}
	payments, err := s.PaymentActivityTabs(ctx, LocalPlayNow(ctx))
	if err != nil {
		return nil, err
	}
	result = append(result, instances...)
	result = append(result, payments...)
	welfare, err := s.P4ActivityTabs(ctx, LocalPlayNow(ctx))
	if err != nil {
		return nil, err
	}
	result = append(result, welfare...)
	copyBenefits, err := s.P4CopyBenefitsTabs(ctx, LocalPlayNow(ctx))
	if err != nil {
		return nil, err
	}
	return append(result, copyBenefits...), nil
}
