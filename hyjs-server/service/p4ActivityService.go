package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
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

// seedP4Periods 复用P1幂等事实/截止快照，未实现的竞技与悬赏不生成事件或奖励。
func (s *GameService) seedP4Periods(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	specs := []struct {
		prefix, title, kind, mode, reward string
		goal                              int64
		rank                              int32
	}{
		{"warriorRescruitTimes", "招募礼包", constUtil.P1EventRecruit, "cumulative", "item_048,1", 10, 0},
		{"warriorGetSpecial", "英雄收藏", constUtil.P4KindCollect, "cumulative", "diamond,50", 5, 0},
		{"mainCopyWinTimes", "主线通关礼包", constUtil.P1EventCopyWin, "cumulative", "item_021,1", 10, 0},
		{"kaiGongActivity", "攻略福利", constUtil.P1EventCopyWin, "cumulative", "item_139,5;item_150,1", 10, 0},
		{"kaiFuJingSai", "开服战力竞赛", constUtil.P4KindPower, "single", "coin,1", math.MaxInt32, 10},
	}
	for _, v := range specs {
		cfg := db.PeriodInstance{ID: v.prefix + "_p4_" + start.Format("200601"), Enabled: false, Title: v.title, Kind: v.kind, Mode: v.mode, Goal: v.goal, Rewards: v.reward, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), RankPlaces: v.rank, RankRewards: "coin,10000;item_021,1", Source: "P4本地兼容服月实例：只累计成功招募/主线通关、已真实拥有的不同英雄类型和服务器阵容战力；战力取期内峰值，截止后前10名同档奖励通过P1邮件/领奖中心发放。重复回执、重复登录及客户端战力值不参与计数。"}
		if v.prefix == "kaiGongActivity" {
			cfg.Source = constUtil.P15KaiGongSource
		}
		var exists int64
		if err := tx.Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if err := s.validateP1Period(cfg); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
	}
	if err := s.seedP4PlunderActivityPeriod(tx, now); err != nil {
		return err
	}
	if err := s.seedP4ArenaWinPeriod(tx, now); err != nil {
		return err
	}
	return s.seedP15Seasonal(tx, now)
}

// RecordP4Save 在已有角色事务里记录实际收藏/战力；月实例首个快照登记已有英雄，后续仅处理新增类型。
func (s *GameService) RecordP4Save(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
	if !s.P4Ready {
		return nil
	}
	now := LocalPlayNow(tx.Statement.Context)
	if err := s.seedP4Periods(tx, now); err != nil {
		return err
	}
	period := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	var state db.P4PlayerStats
	err := tx.Where("player_id=?", p.GetPlayerId()).First(&state).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	first := state.Period != period
	// 新收藏期由真实保存/登录时的当前阵容逐英雄登记，不在工具开启时补事实。
	collectionSource := period
	collectionCycle, err := localPlayBoundCycle(tx, p.GetPlayerId(), "CollectionWarrior")
	if err != nil {
		return err
	}
	if collectionCycle != nil {
		collectionSource = collectionCycle.ID
	}
	oldClasses := map[string]bool{}
	if !first && collectionCycle == nil && row.Player != nil && row.Game != nil {
		_, old, err := decodeProfile(row)
		if err != nil {
			return err
		}
		for _, w := range old.Warriors {
			oldClasses[w.GetId()] = true
		}
	}
	for _, w := range g.Warriors {
		id := w.GetId()
		hero := s.table("Hero", "id", id)
		if hero == nil || !strings.HasPrefix(id, "hero_") || strings.HasPrefix(id, "hero_exp_") || oldClasses[id] {
			continue
		}
		oldClasses[id] = true
		if err := s.RecordP1Event(tx, p.GetPlayerId(), "owned:"+collectionSource+":"+id, constUtil.P4KindCollect, 1, now); err != nil {
			return err
		}
	}
	if g.GuildPrayerProjection == nil {
		if err := s.PrepareP5GuildPrayerProperties(tx.Statement.Context, tx, p, g, p13UFNow(tx.Statement.Context)); err != nil {
			return err
		}
	}
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return err
	}
	if power > 0 && (first || power != state.LastPower) {
		if err := s.RecordP1Event(tx, p.GetPlayerId(), fmt.Sprintf("power:%s:%d", period, power), constUtil.P4KindPower, int64(power), now); err != nil {
			return err
		}
	}
	// 新开启的竞赛接收当前服务器战力，已有窗口继续使用原峰值和幂等事实。
	if power > 0 {
		if err := s.recordP4PowerEntry(tx, p.GetPlayerId(), power, now); err != nil {
			return err
		}
	}
	state.PlayerID = p.GetPlayerId()
	state.Period = period
	state.LastPower = power
	return tx.Save(&state).Error
}

func (s *GameService) recordP4PowerEntry(tx *gorm.DB, id int64, power int32, now time.Time) error {
	// 与RecordP1Event保持相同锁顺序，跨角色同时进入新窗口时避免逆序锁等待。
	instances, err := p1LockPeriods(tx)
	if err != nil {
		return err
	}
	for _, live := range instances {
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
		if cfg.Kind != constUtil.P4KindPower || cfg.Mode != "single" || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			continue
		}
		var count int64
		if err = tx.Model(&db.PeriodProgress{}).Where("player_id=? AND instance_id=?", id, cfg.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err = s.RecordP1Event(tx, id, "power-entry:"+cfg.ID, constUtil.P4KindPower, int64(power), now); err != nil {
			return err
		}
	}
	return nil
}
func (s *GameService) RecordP4Login(ctx context.Context, id int64) error {
	if !s.P4Ready {
		return nil
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if _, err := p1LockPeriods(tx); err != nil {
			return err
		}
		return s.RecordP4Save(tx, &row, p, g)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// P4ActivityTabs 只投放规则已落地的入口；原生同类型只展示一个P4实例，36登记独立保留。
func (s *GameService) P4ActivityTabs(ctx context.Context, now time.Time) ([]*pb.ActivityTab, error) {
	if !s.P4Ready {
		return nil, nil
	}
	returnTabs := []*pb.ActivityTab{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := s.seedP4Periods(tx, now); err != nil {
			return err
		}
		if err := s.seedP4Commerce(tx, now); err != nil {
			return err
		}
		if err := s.seedP4Welfare(tx, now); err != nil {
			return err
		}
		var commerce []db.P4CommerceRule
		if err := tx.Where("enabled=? AND shop_type=?", true, 0).Order("id").Find(&commerce).Error; err != nil {
			return err
		}
		hasExchange := false
		names := map[string]string{"half": "半价抢购", "discount": "折扣商店", "exchange": "限时兑换", "sale": "限时特卖", "mystic": "神秘小屋"}
		for _, r := range commerce {
			allowed, err := localPlayActivityVisible(tx, r.ID)
			if err != nil {
				return err
			}
			if !allowed {
				continue
			}
			if isP15SeasonalChild(r.ID) {
				continue
			}
			cfg, _, err := s.p4CommerceSnapshot(r)
			if err != nil {
				return err
			}
			if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
				continue
			}
			if cfg.Kind == "exchange" {
				hasExchange = true
				continue
			}
			title := names[cfg.Kind]
			if strings.HasPrefix(cfg.ID, constUtil.P5SpecialtySupplyPrefix) {
				title = "专精书兑换"
			}
			if strings.HasPrefix(cfg.ID, constUtil.P5GemSupplyPrefix) {
				title = "宝石包兑换"
			}
			returnTabs = append(returnTabs, &pb.ActivityTab{ActivityId: proto.String(cfg.ID), Name: proto.String(title), Order: proto.Int32(int32(20 + len(returnTabs))), Status: proto.Int32(0), Param: proto.String("本地兼容规则")})
		}
		// One Type23 tab keeps every original product reachable through the client dictionary.
		redOpen, e := s.p4ExchangeRedOpen(tx, now)
		if e != nil {
			return e
		}
		hasExchange = hasExchange || redOpen
		if hasExchange {
			exchangeKey, err := localPlayExchangeKey(tx, now)
			if err != nil {
				return err
			}
			var off int64
			if err := tx.Model(&db.P4ResourceExchangeDirectory{}).Where("id=? AND enabled=?", exchangeKey, false).Count(&off).Error; err != nil {
				return err
			}
			if off == 0 {
				returnTabs = append(returnTabs, &pb.ActivityTab{ActivityId: proto.String(exchangeKey), Name: proto.String("资源兑换"), Order: proto.Int32(int32(20 + len(returnTabs))), Status: proto.Int32(0)})
			}
		}
		month := now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
		// 福利入口尊重运营启停；关闭的实例不会继续展示可领取入口。
		for _, spec := range []struct{ kind, prefix, title string }{{constUtil.P4WelfareCat, "fortuneCat_p4_", "藏宝箱"}, {constUtil.P4WelfareRebate, "consumeRet_p4_", "消费返利"}} {
			ruleKey, err := localPlayWelfareRuleID(tx, spec.kind)
			if err != nil {
				return err
			}
			if ruleKey == "" {
				ruleKey, err = AdminCurrentWelfareRuleKey(tx, spec.kind, now.UnixMilli(), "p4:"+spec.kind+":"+month)
				if err != nil {
					return err
				}
			}
			activityKey, err := localPlayWelfareActivityKey(tx, spec.kind, spec.prefix+month)
			if err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&db.P4WelfareRule{}).Where("id=? AND enabled=?", ruleKey, true).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				returnTabs = append(returnTabs, &pb.ActivityTab{ActivityId: proto.String(activityKey), Name: proto.String(spec.title), Order: proto.Int32(int32(30 + len(returnTabs))), Status: proto.Int32(0)})
			}
		}
		seasonal, err := s.p15SeasonalTabs(tx, now)
		if err != nil {
			return err
		}
		returnTabs = append(returnTabs, seasonal...)
		doubleOpen, err := firstDoubleEnabled(tx)
		if err != nil {
			return err
		}
		if doubleOpen {
			returnTabs = append(returnTabs, &pb.ActivityTab{ActivityId: proto.String("RechangeDouble"), Name: proto.String("首笔充值双倍"), Order: proto.Int32(38), Status: proto.Int32(0)})
		}
		// 原31页使用57053说明接口；投放实际强者轮换的同一份冻结奖池。
		if tx.Migrator().HasTable(&db.GameplayPoolReward{}) {
			var pools int64
			if err := tx.Model(&db.GameplayPoolReward{}).Where("pool_id=?", "recruit_2").Count(&pools).Error; err != nil {
				return err
			}
			if pools > 0 {
				if _, _, err := s.loadP4Rotation(tx, 2, now); err != nil {
					return err
				}
				returnTabs = append(returnTabs, &pb.ActivityTab{ActivityId: proto.String("warriorWeightImprove"), Name: proto.String("强者招募轮换"), Order: proto.Int32(39), Status: proto.Int32(0)})
			}
		}
		return nil
	})
	return returnTabs, err
}

func (s *GameService) seedP4Registry(tx *gorm.DB) error {
	names := strings.Split("EveryDaySign,OpenServerRace,AsunaKitchen,LoginAward,RechargeSurprise,FirstRecharge,MonthCard,VIPBenefits,BattleRace,RobTrea,PrizeTask,PrizeBoss,CopyBenefits,MainCopy,CostTotal,RechargeTotal,DiscountSale,HalfDiscountSale,OpenSeverMoney,AincradStars,Congratulation,OpenHappy,TimeLimitExchange,TimeLimitSail,Festival,Update,fortuneCat,continueRechargeGift,WarriorRescruitTimes,CollectionWarrior,WarriorWeightImprove,KaiGongActivity,RechangeDouble,BigTurnTable,CostReturn,MysticDiscount", ",")
	prefixes := strings.Split(",kaiFuJingSai,,,rechargeSingle,firstRecharge,monthCard,vipBoon,pvpWinTimes,robTreasureTimes,fightRedNameTimes,killRedNameTimes,allCopyDouble,mainCopyWinTimes,costTotal,rechargeTotal,discountSale,halfDiscountSale,openServerFund,aincradStars,celebrationActivity,openServerSpree,limitExchange,limitSale,festival,update,fortuneCat,continuedRecharge,warriorRescruitTimes,warriorGetSpecial,warriorWeightImprove,kaiGongActivity,RechangeDouble,bigTurnTable,consumeRet,mysticDiscount", ",")
	if len(names) != 36 || len(prefixes) != 36 {
		return errors.New("原36活动枚举登记长度无效")
	}
	for i, name := range names {
		row := db.P4ActivityRegistry{ID: int32(i + 1), Name: name, Prefix: prefixes[i], Protocols: "CSActivityInfo/CSActivityReward", Owner: "后续期", Status: "仅登记，待恢复原规则", Evidence: "原APK UI.ActivityType/ActivityData.SCLIST枚举；类型登记不作为业务完成证据"}
		switch i + 1 {
		case 1:
			row.Owner = "P4/P0/P3"
			row.Status = "原月签到与限时签到后端已实现"
			row.Protocols = "CSMonthSignList/CSReceiveMonthSign/CSRewardLimitSignList/CSReceiveLimitMonthSign"
		case 2, 14, 29, 30:
			row.Owner = "P4+P1"
			row.Status = "实际事实+截止投递后端已实现"
		case 3:
			row.Owner = "P4"
			row.Status = "两实际体力时间窗已实现"
			row.Protocols = "CSStaminaRefill/CSStaminaGet"
		case 4:
			row.Owner = "P4+P1"
			row.Status = "独立登录奖历已实现"
			row.Protocols = "CSActivityInfo/CSActivityReward"
		case 5, 6, 7, 8, 15, 16, 28:
			row.Owner = "P3"
			row.Status = "订单/VIP/月卡/支付活动已实现，复用原域"
		case 9:
			row.Owner = "P4/P11+P1"
			row.Status = "实际modern23101挑战胜者事实与默认off本地月福利；原私服档位未知、待实机"
			row.Evidence = "analysis/p4/arena-win-activity-original.il.txt；VsGetItem原胜利次数，复用acceptedP6和P1窗口/奖励"
		case 10:
			row.Owner = "P4/P11+P1"
			row.Status = "实际type3挑战次数与默认off本地月福利；原私服档位未知、待实机"
			row.Evidence = "analysis/p4/arena-win-activity-original.il.txt/arena-win-row-original.il.txt；原RobTrea次数，胜败均计实际accepted挑战"
		case 11, 12:
			row.Owner = "P12"
			row.Status = "待悬赏实际事实联动"
		case 13:
			row.Owner = "P4/P6"
			row.Status = "原窗口金币经验与已确认掉落联动；原57053说明/主线导航已接，未知特殊池关闭"
			row.Protocols = "CSCopyBoonInfo/SCCopyBoonInfo"
		case 17, 18, 23, 24, 36:
			row.Owner = "P4"
			row.Status = "冻结目录/实际扣费/共享库存已实现"
			row.Protocols = "CSActivityShopInfo/CSGetActivityShopReward/CSBuyActivityShop"
			if i+1 == 36 {
				row.Protocols += "/CSMysticDiscountInfo/CSMysticDiscount"
			}
		case 19:
			row.Owner = "P0"
			row.Status = "实际基金已实现，复用原域"
		case 20:
			row.Owner = "P7/P4+P1"
			row.Status = "真实accepted试炼星数；本地首档3星100积分；个人期只累计新动作，完整原档位待恢复"
			row.Protocols = "CSActivityInfo/CSActivityReward"
			row.Evidence = "analysis/growth-first-20261016/aincrad/panel-original.il.txt；ActivityType20原generic页，原提示试炼达星领奖；local-aincrad-earned-stars-v1"
		case 21, 25, 26:
			row.Owner = "P4/P15+P1"
			row.Status = "seasonal-material-v1真实独立庆典/嵌套登录/材料兑换实例；原运营窗口和奖品未知，实机待验"
			row.Protocols = "CSActivityInfo/CSActivityReward"
			if i+1 == 25 || i+1 == 26 {
				row.Protocols = "CSActivityList(activityId)/CSActivityInfo/CSActivityReward/CSActivityShopInfo/CSBuyActivityShop"
			}
			row.Evidence = "analysis/p4/seasonal-activity.il.txt；原Panel的21带activityId登录奖励页、25/26同用Festival与Update子前缀；具名本地实例不代替原运营数据"
		case 22:
			row.Owner = "P0"
			row.Status = "原开服七日实际条件已实现，复用原域"
		case 27:
			row.Owner = "P4"
			row.Status = "藏宝箱冷却/每日/总次数/终奖已实现"
			row.Protocols = "CSFortuneCatInfo/CSFortuneCatReward"
		case 31:
			row.Owner = "P4"
			row.Status = "实际强者轮换权重；原57053说明和招募导航已接"
			row.Protocols = "CSCopyBoonInfo/SCCopyBoonInfo"
		case 32:
			row.Owner = "P4/P15+P1"
			row.Status = "kaigong-mainline-supply-v1真实10次主线/扫荡事实材料福利；原运营未知、实机待验"
			row.Protocols = "CSActivityInfo/CSActivityReward"
			row.Evidence = "analysis/p4/seasonal-panels.il.txt与seasonal-kaigong.il.txt：原Panel32加载RechargeSurprise，其Show使用CSActivityInfo；独立CSKaiGong原非协议调用尚未知"
		case 33:
			row.Owner = "P3/P4"
			row.Status = "original-sku-first-ever-double-v1默认off；终身每原商品首次确认，原57053说明与充值导航"
			row.Protocols = "CSCopyBoonInfo/SCCopyBoonInfo/CSGetPaymentInfo/CSGenerateOrder"
		case 34:
			row.Owner = "P2"
			row.Status = "原非红/红奖池与排名结算已实现，复用原域"
		case 35:
			row.Owner = "P4+P1"
			row.Status = "实际钻石消费每日一次返利后端已实现"
			row.Protocols = "CSActivityInfo/CSConsumeReturnReward/CSActivityReward"
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		// 只提升已有程序登记的未知占位，运营手工状态与证据保持。
		if i+1 == 13 || i+1 == 31 || i+1 == 33 || i+1 == 20 || i+1 == 21 || i+1 == 25 || i+1 == 26 || i+1 == 32 {
			placeholder := []string{"仅登记，待恢复原规则"}
			switch row.ID {
			case 13:
				placeholder = append(placeholder, "待原双倍窗口规则及真实战斗掉落联动")
			case 31:
				placeholder = append(placeholder, "强者轮换首窗口橙组25%真实权重；投放由招募入口可预览")
			case 33:
				placeholder = append(placeholder, "订单/VIP/月卡/支付活动已实现，复用原域")
			}
			if row.ID == 20 {
				placeholder = append(placeholder, "待试炼星数实际结算联动")
			}
			if err := tx.Model(&db.P4ActivityRegistry{}).Where("id=? AND name=? AND status IN ?", row.ID, row.Name, placeholder).Updates(map[string]any{"owner": row.Owner, "status": row.Status, "protocols": row.Protocols, "evidence": row.Evidence}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
