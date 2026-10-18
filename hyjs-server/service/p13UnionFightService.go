package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"time"
)

// InitializeP13UnionFight 迁移报名事实；已有配置保持，独立门禁默认关闭。
func (s *GameService) InitializeP13UnionFight(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightPolicy{}, &db.P13UnionFightSetting{}, &db.P13UnionFightEntry{}, &db.P13UnionFightFighter{}, &db.P13UnionFightSchedule{}, &db.P13UnionFightGroup{}, &db.P13UnionFightMatch{}, &db.P13UnionFightLoadout{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightPolicy{ID: 1, Source: constUtil.P13UnionFightSource}).Error; err != nil {
		return err
	}
	if err := s.InitializeP13UnionFightField(ctx); err != nil {
		return err
	}
	return s.InitializeP13UnionFightSettlement(ctx)
}
func p13UFClock(now time.Time) (time.Time, string, int32, int64) {
	local := now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	stage := int32(0)
	deadline := start.Add(24 * time.Hour).UnixMilli()
	switch local.Weekday() {
	case time.Monday, time.Wednesday, time.Friday:
		stage = constUtil.P13UnionFightStageDeclare
	case time.Tuesday, time.Thursday, time.Saturday:
		if local.Before(start.Add(30 * time.Minute)) {
			stage = constUtil.P13UnionFightStageMatch
			deadline = start.Add(30 * time.Minute).UnixMilli()
		}
	}
	return start, start.Format("2006-01-02"), stage, deadline
}
func p13UFNeedEnabled(tx *gorm.DB) error {
	var cfg db.P13UnionFightPolicy
	if err := tx.First(&cfg, 1).Error; err != nil {
		return err
	}
	if !cfg.EnrollmentEnabled {
		return errors.New(constUtil.P13UnionFightClosed)
	}
	return nil
}
func p13UFDay(now time.Time) string {
	start, _, _, _ := p13UFClock(now)
	for start.Weekday() != time.Monday && start.Weekday() != time.Wednesday && start.Weekday() != time.Friday {
		start = start.AddDate(0, 0, -1)
	}
	return start.Format("2006-01-02")
}

// ServeP13UnionFight 在服务生命周期内检查自动窗口与冻结截止，取消时停止ticker。
func (s *GameService) ServeP13UnionFight(ctx context.Context) error {
	if err := s.TickP13UnionFight(ctx, p13UFNow(ctx)); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(constUtil.P13UnionFightTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.TickP13UnionFight(ctx, p13UFNow(ctx)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// p13UFDeclare 在原宣战日记录当时全部真实成员与职位，不增加公会资产或比赛结果。
func (s *GameService) p13UFDeclare(tx *gorm.DB, guild *db.P13Guild, now time.Time) error {
	_, day, stage, deadline := p13UFClock(now)
	if stage != constUtil.P13UnionFightStageDeclare {
		return errors.New("当前不在原公会宣战日")
	}
	level, err := s.p10Setting("UnionFightApplyLevelLimit")
	if err != nil {
		return err
	}
	if guild.Level < level {
		return errors.New("公会未达到原2级宣战资格")
	}
	var existing db.P13UnionFightEntry
	err = tx.First(&existing, "guild_id=? AND day=?", guild.ID, day).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	entry := db.P13UnionFightEntry{GuildID: guild.ID, Day: day, AppliedAt: now.UnixMilli(), Deadline: deadline, Source: constUtil.P13UnionFightSource}
	if err = tx.Create(&entry).Error; err != nil {
		return err
	}
	var members []db.P13GuildMember
	if err = tx.Where("guild_id=?", guild.ID).Order("player_id").Find(&members).Error; err != nil {
		return err
	}
	for _, member := range members {
		row := db.P13UnionFightFighter{GuildID: guild.ID, Day: day, PlayerID: member.PlayerID, Role: member.Role}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// TickP13UnionFight 使用明确传入的时钟处理原自动窗口与截止冻结；不会制造匹配、胜负或奖励。
func (s *GameService) TickP13UnionFight(ctx context.Context, now time.Time) error {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var cfg db.P13UnionFightPolicy
		if err := tx.First(&cfg, 1).Error; err != nil {
			return err
		}
		if !gate.ManagementEnabled || !cfg.EnrollmentEnabled {
			return nil
		}
		// 截止后只冻结当前仍属于原公会的报名者，已退出者保存历史但不取得入场资格。
		var entries []db.P13UnionFightEntry
		if err := tx.Where("deadline<=? AND frozen_at=0", now.UnixMilli()).Order("day,guild_id").Find(&entries).Error; err != nil {
			return err
		}
		// 同一截止批次共享一次配置序列化，避免每个公会重复构造相同大快照并延长全域锁。
		var frozenTables []byte
		var configurationHash string
		if len(entries) > 0 {
			var err error
			frozenTables, err = json.Marshal(s.Tables)
			if err != nil {
				return err
			}
			configurationHash = p6Digest(frozenTables)
		}
		for _, entry := range entries {
			var members []db.P13UnionFightFighter
			if err := tx.Where("guild_id=? AND day=? AND applied_at>0", entry.GuildID, entry.Day).Find(&members).Error; err != nil {
				return err
			}
			for _, fighter := range members {
				battle := &pb.BattlePlayer{}
				if len(fighter.Battle) == 0 || p6Digest(fighter.Battle) != fighter.SnapshotHash || proto.Unmarshal(fighter.Battle, battle) != nil || battle.GetPlayerId() != fighter.PlayerID {
					return errors.New("公会战报名阵容快照损坏")
				}
				member, err := p13GuildMember(tx, fighter.PlayerID)
				if err != nil {
					return err
				}
				if member.GuildID == entry.GuildID {
					entry.EligibleCount++
				} else {
					fighter.FirstOn = false
					if err = tx.Save(&fighter).Error; err != nil {
						return err
					}
				}
			}
			entry.FrozenAt = now.UnixMilli()
			// 截止同时保留配置内容和引擎/客户端版本，后续对局不使用新发布覆盖旧资格。
			entry.FrozenTables = frozenTables
			entry.ConfigurationHash = configurationHash
			entry.EngineVersion = constUtil.P6EngineVersion
			entry.ClientVersion = constUtil.P6OriginalClientHash
			if err := tx.Save(&entry).Error; err != nil {
				return err
			}
		}
		start, _, stage, _ := p13UFClock(now)
		if stage != constUtil.P13UnionFightStageDeclare || !now.Before(start.Add(5*time.Minute)) {
			return nil
		}
		var settings []db.P13UnionFightSetting
		if err := tx.Where("auto_apply=1").Order("guild_id").Find(&settings).Error; err != nil {
			return err
		}
		for _, setting := range settings {
			var guild db.P13Guild
			err := tx.First(&guild, "id=?", setting.GuildID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			min, err := s.p10Setting("UnionFightApplyLevelLimit")
			if err != nil {
				return err
			}
			if guild.Level < min {
				continue
			}
			if err = s.p13UFDeclare(tx, &guild, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.MatchP13UnionFight(ctx, now); err != nil {
		return err
	}
	return s.TickP13UnionFightSettlement(ctx, now)
}

// p13UFSnapshot 只从当前真实存档生成报名阵容，角色装备改变不会隐式改写已上传阵容。
func (s *GameService) p13UFSnapshot(p *pb.Player, state *model.GameState, fighter *db.P13UnionFightFighter, now time.Time) error {
	battle, err := s.battlePlayer(p, state)
	if err != nil {
		return err
	}
	power, err := s.formationFightPower(p, state)
	if err != nil {
		return err
	}
	raw, err := proto.Marshal(battle)
	if err != nil {
		return err
	}
	fighter.Name = p.GetName()
	fighter.FightPower = int64(power)
	fighter.Battle = raw
	fighter.SnapshotHash = p6Digest(raw)
	fighter.UpdatedAt = now.UnixMilli()
	return nil
}
func (s *GameService) p13UFMutate(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	switch request.(type) {
	case *pb.CSUFUnionApply:
		response = &pb.SCUFUnionApply{}
	case *pb.CSUFAutoApplySet:
		response = &pb.SCUFAutoApplySet{}
	case *pb.CSUFPlayerApply:
		response = &pb.SCUFPlayerApply{}
	case *pb.CSUFSetAutoFight:
		response = &pb.SCUFSetAutoFight{}
	case *pb.CSUFUpdateBattlePlayer:
		response = &pb.SCUFUpdateBattlePlayer{}
	case *pb.CSUFAdjustFirstOn:
		response = &pb.SCUFAdjustFirstOn{}
	case *pb.CSUFSetFightItems:
		response = &pb.SCUFSetFightItems{}
	default:
		return nil, errors.New(constUtil.P13UnionFightUnknown)
	}
	return s.p13GuildMutate(ctx, id, request, response, stamp, now, false, func(tx *gorm.DB, row *db.Profile, p *pb.Player, state *model.GameState) error {
		if err := p13UFNeedEnabled(tx); err != nil {
			return err
		}
		member, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		if items, isItems := request.(*pb.CSUFSetFightItems); isItems && items.GetFightIndex() != 0 {
			if err = s.p13UFSetRoundItems(tx, id, guild.ID, state, items, now); err != nil {
				return err
			}
			response.(*pb.SCUFSetFightItems).ItemIds = append([]string(nil), items.GetItemIds()...)
			return nil
		}
		switch b := request.(type) {
		case *pb.CSUFAutoApplySet:
			if err = p13GuildRole(member, constUtil.P13GuildVice); err != nil {
				return err
			}
			setting := db.P13UnionFightSetting{GuildID: guild.ID, AutoApply: b.GetIsAuto() == 1}
			return tx.Save(&setting).Error
		case *pb.CSUFUnionApply:
			if err = p13GuildRole(member, constUtil.P13GuildVice); err != nil {
				return err
			}
			return s.p13UFDeclare(tx, guild, now)
		}
		_, day, stage, _ := p13UFClock(now)
		if stage != constUtil.P13UnionFightStageDeclare {
			return errors.New("原公会战报名截止已到")
		}
		var entry db.P13UnionFightEntry
		if err = tx.First(&entry, "guild_id=? AND day=?", guild.ID, day).Error; err != nil {
			return errors.New("本会尚未宣战")
		}
		if entry.FrozenAt != 0 || now.UnixMilli() >= entry.Deadline {
			return errors.New("公会战报名名单已冻结")
		}
		if b, adjust := request.(*pb.CSUFAdjustFirstOn); adjust {
			if err = p13GuildRole(member, constUtil.P13GuildVice); err != nil {
				return err
			}
			var target db.P13UnionFightFighter
			if err = tx.First(&target, "guild_id=? AND day=? AND player_id=? AND applied_at>0", guild.ID, day, b.GetOptPlayerId()).Error; err != nil {
				return errors.New("目标不在已报名成员中")
			}
			targetMember, err := p13GuildMember(tx, target.PlayerID)
			if err != nil {
				return err
			}
			if targetMember.GuildID != guild.ID {
				return errors.New("目标已退出本会")
			}
			if member.Role < target.Role {
				return errors.New("不能调整宣战时职位高于自己的成员")
			}
			maximum, err := s.p10Setting("UFMaxAdjustApplyTimes")
			if err != nil {
				return err
			}
			if entry.AdjustTimesUsed >= maximum {
				return errors.New("原首发调整次数已用完")
			}
			promote := b.GetOptType() == 1
			if promote == target.FirstOn {
				return errors.New("目标已经处于所选首发状态")
			}
			var firsts, total int64
			if err = tx.Model(&db.P13UnionFightFighter{}).Where("guild_id=? AND day=? AND applied_at>0 AND first_on=1", guild.ID, day).Count(&firsts).Error; err != nil {
				return err
			}
			limit, err := s.p10Setting("UnionFighterNum")
			if err != nil {
				return err
			}
			if promote && firsts >= int64(limit) {
				return errors.New("原25首发名额已满")
			}
			if !promote {
				if err = tx.Model(&db.P13UnionFightFighter{}).Where("guild_id=? AND day=? AND applied_at>0", guild.ID, day).Count(&total).Error; err != nil {
					return err
				}
				if total <= int64(limit) {
					return errors.New("原报名未超过25人，不能设为替补")
				}
			}
			target.FirstOn = promote
			entry.AdjustTimesUsed++
			if err = tx.Save(&target).Error; err != nil {
				return err
			}
			if err = tx.Save(&entry).Error; err != nil {
				return err
			}
			response.(*pb.SCUFAdjustFirstOn).AdjustTimes = proto.Int32(maximum - entry.AdjustTimesUsed)
			return nil
		}
		var fighter db.P13UnionFightFighter
		if err = tx.First(&fighter, "guild_id=? AND day=? AND player_id=?", guild.ID, day, id).Error; err != nil {
			return errors.New("角色不在原宣战时成员名单")
		}
		switch b := request.(type) {
		case *pb.CSUFPlayerApply:
			if fighter.AppliedAt != 0 {
				response.(*pb.SCUFPlayerApply).IsFirst = proto.Int32(0)
				if fighter.FirstOn {
					response.(*pb.SCUFPlayerApply).IsFirst = proto.Int32(1)
				}
				return nil
			}
			var firsts int64
			if err = tx.Model(&db.P13UnionFightFighter{}).Where("guild_id=? AND day=? AND applied_at>0 AND first_on=1", guild.ID, day).Count(&firsts).Error; err != nil {
				return err
			}
			limit, err := s.p10Setting("UnionFighterNum")
			if err != nil {
				return err
			}
			fighter.FirstOn = firsts < int64(limit)
			fighter.AppliedAt = now.UnixMilli()
			if err = s.p13UFSnapshot(p, state, &fighter, now); err != nil {
				return err
			}
			response.(*pb.SCUFPlayerApply).IsFirst = proto.Int32(0)
			if fighter.FirstOn {
				response.(*pb.SCUFPlayerApply).IsFirst = proto.Int32(1)
			}
		case *pb.CSUFSetAutoFight:
			if fighter.AppliedAt == 0 {
				return errors.New("角色尚未报名公会战")
			}
			fighter.AutoFight = b.GetIsAuto() == 1
		case *pb.CSUFUpdateBattlePlayer:
			if fighter.AppliedAt == 0 {
				return errors.New("角色尚未报名公会战")
			}
			maximum, err := s.p10Setting("UFMaxPlayerAjustTime")
			if err != nil {
				return err
			}
			if fighter.BattleAdjustTimes >= maximum {
				return errors.New("原参战阵容更新次数已用完")
			}
			if err = s.p13UFSnapshot(p, state, &fighter, now); err != nil {
				return err
			}
			fighter.BattleAdjustTimes++
			// 原UI将3减此已用次数；不要把剩余次数直接填到原字段。
			response.(*pb.SCUFUpdateBattlePlayer).BattleArrayAdjustTime = proto.Int32(fighter.BattleAdjustTimes)
		case *pb.CSUFSetFightItems:
			if fighter.AppliedAt == 0 {
				return errors.New("角色尚未报名公会战")
			}
			if b.GetFightIndex() != 0 {
				return errors.New("原每场道具配置须待匹配后开放")
			}
			for _, itemID := range b.GetItemIds() {
				item := s.table("Item", "itemId", itemID)
				if item["category"] != "17" {
					return errors.New("原报名道具不属于公会战类别17")
				}
				owned := int64(0)
				for _, stack := range state.Items {
					if stack.GetId() == itemID {
						owned += int64(stack.GetNum())
					}
				}
				if owned <= 0 {
					return errors.New("尚未持有原报名公会战道具")
				}
			}
			encoded, err := json.Marshal(b.GetItemIds())
			if err != nil {
				return err
			}
			fighter.DefaultItems = string(encoded)
			response.(*pb.SCUFSetFightItems).ItemIds = append([]string(nil), b.GetItemIds()...)
		}
		return tx.Save(&fighter).Error
	})
}

func (s *GameService) p13UFQuery(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	if err := s.TickP13UnionFight(ctx, now); err != nil {
		return nil, err
	}
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, preview := request.(*pb.CSUFInfoPreView); preview {
			member, err := p13GuildMember(tx, id)
			if err != nil {
				return err
			}
			if member.GuildID == "" {
				response = &pb.SCUFInfoPreView{IsUnionApplied: proto.Int32(0)}
				return nil
			}
		}
		_, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		day := p13UFDay(now)
		var fighters []db.P13UnionFightFighter
		if err = tx.Where("guild_id=? AND day=? AND applied_at>0", guild.ID, day).Order("applied_at,player_id").Find(&fighters).Error; err != nil {
			return err
		}
		switch request.(type) {
		case *pb.CSUFInfoPreView:
			matchDate, err := time.ParseInLocation("2006-01-02", day, time.FixedZone("Asia/Shanghai", 8*3600))
			if err != nil {
				return err
			}
			previews, _, err := s.p13UFMatchPreviews(tx, guild.ID, matchDate.AddDate(0, 0, 1).Format("2006-01-02"), id)
			if err != nil {
				return err
			}
			reply := &pb.SCUFInfoPreView{IsUnionApplied: proto.Int32(0)}
			var entry db.P13UnionFightEntry
			if err = tx.First(&entry, "guild_id=? AND day=?", guild.ID, day).Error; err == nil {
				reply.IsUnionApplied = proto.Int32(1)
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if len(previews) > 0 {
				reply.FirstFightInfo = previews[0]
			}
			if len(previews) > 1 {
				reply.SecondFightInfo = previews[1]
			}
			response = reply
		case *pb.CSUFAppliedPlayer:
			maximum, err := s.p10Setting("UFMaxAdjustApplyTimes")
			if err != nil {
				return err
			}
			var entry db.P13UnionFightEntry
			if err = tx.First(&entry, "guild_id=? AND day=?", guild.ID, day).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			reply := &pb.SCUFAppliedPlayer{AdjustTimes: proto.Int32(maximum - entry.AdjustTimesUsed)}
			for _, fighter := range fighters {
				item := &pb.UFUnionFighterInfo{PlayerId: proto.Int64(fighter.PlayerID), Name: proto.String(fighter.Name), Role: proto.Int32(fighter.Role), IsAutoFight: proto.Int32(0), Fight: proto.Float64(float64(fighter.FightPower))}
				if fighter.AutoFight {
					item.IsAutoFight = proto.Int32(1)
				}
				if fighter.FirstOn {
					reply.Firsts = append(reply.Firsts, item)
				} else {
					reply.BackUps = append(reply.BackUps, item)
				}
			}

			response = reply
		case *pb.CSUFCurrentStageInfo:
			_, _, stage, deadline := p13UFClock(now)
			reply := &pb.SCUFCurrentStageInfo{Stage: proto.Int32(stage), NextStageTime: proto.Int64(deadline), ServerTimeNow: proto.Int64(now.UnixMilli()), AppliedNum: proto.Int32(int32(len(fighters))), IsUnionApplied: proto.Int32(0), IsAutoApply: proto.Int32(0), IsAutoFight: proto.Int32(0), IsPlayerApplied: proto.Int32(0), IsIn25: proto.Int32(0), FightPower: proto.Int64(0), AdjustTimes: proto.Int32(0), BattleArrayAdjustTime: proto.Int32(0)}
			var entry db.P13UnionFightEntry
			err = tx.First(&entry, "guild_id=? AND day=?", guild.ID, day).Error
			if err == nil {
				reply.IsUnionApplied = proto.Int32(1)
				maximum, err := s.p10Setting("UFMaxAdjustApplyTimes")
				if err != nil {
					return err
				}
				reply.AdjustTimes = proto.Int32(maximum - entry.AdjustTimesUsed)
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var setting db.P13UnionFightSetting
			err = tx.First(&setting, "guild_id=?", guild.ID).Error
			if err == nil && setting.AutoApply {
				reply.IsAutoApply = proto.Int32(1)
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			for _, fighter := range fighters {
				if fighter.PlayerID == id {
					reply.IsPlayerApplied = proto.Int32(1)
					reply.FightPower = proto.Int64(fighter.FightPower)
					reply.BattleArrayAdjustTime = proto.Int32(fighter.BattleAdjustTimes)
					if fighter.DefaultItems != "" {
						if err := json.Unmarshal([]byte(fighter.DefaultItems), &reply.ItemIds); err != nil {
							return err
						}
					}
					if fighter.FirstOn {
						reply.IsIn25 = proto.Int32(1)
					}
					if fighter.AutoFight {
						reply.IsAutoFight = proto.Int32(1)
					}
				}
			}
			// 原UI读取两个显示起止值；仅使用运营已发布日历，不填零时刻伪造可开赛。
			matchDate, err := time.ParseInLocation("2006-01-02", day, time.FixedZone("Asia/Shanghai", 8*3600))
			if err != nil {
				return err
			}
			matchDay := matchDate.AddDate(0, 0, 1).Format("2006-01-02")
			var schedule db.P13UnionFightSchedule
			err = tx.First(&schedule, "day=?", matchDay).Error
			if err == nil && schedule.FirstStartAt > 0 && schedule.SecondStartAt > schedule.FirstStartAt && schedule.DurationMillis > 0 && schedule.SecondStartAt <= math.MaxInt64-schedule.DurationMillis {
				reply.FightTimeShow = []int64{schedule.FirstStartAt, schedule.SecondStartAt + schedule.DurationMillis}
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			previews, group, err := s.p13UFMatchPreviews(tx, guild.ID, matchDay, id)
			if err != nil {
				return err
			}
			if len(previews) > 0 {
				reply.FirstFightInfo = previews[0]
				reply.Group = proto.String(group)
			}
			if len(previews) > 1 {
				reply.SecondFightInfo = previews[1]
			}
			// 原准备页的3..7阶段分别展示首轮准备、首战、间歇、次战与全部结束。
			// 这里只投影已发布的真实两轮时刻，未决战果继续为空，不推定胜负。
			if stage == 0 && len(previews) == 2 {
				at := now.UnixMilli()
				var current, round int32
				var next int64
				switch {
				case at < previews[0].GetStartFightTime():
					current, round, next = 3, 1, previews[0].GetStartFightTime()
				case at < previews[0].GetEndFightTime():
					current, round, next = 4, 1, previews[0].GetEndFightTime()
				case at < previews[1].GetStartFightTime():
					current, round, next = 5, 2, previews[1].GetStartFightTime()
				case at < previews[1].GetEndFightTime():
					current, round, next = 6, 2, previews[1].GetEndFightTime()
				default:
					current, round, next = 7, 2, previews[1].GetEndFightTime()
				}
				reply.Stage, reply.Round, reply.NextStageTime = proto.Int32(current), proto.Int32(round), proto.Int64(next)
			}
			response = reply
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// DispatchP13UnionFight 开放已有原规则的报名交易，匹配/场景/调整次数与战果保留明确边界。
func (s *GameService) DispatchP13UnionFight(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	if body, ok := request.(*pb.CSChatTcp); ok && body.GetChatType() == constUtil.P13UnionFightChatType {
		messages, err := s.p13UFChat(ctx, id, request, stamp, p13UFNow(ctx))
		return messages, true, err
	}
	if _, ok := request.(*pb.CSUFChatCache); ok {
		messages, err := s.p13UFChatHistory(ctx, id, p13UFNow(ctx))
		return messages, true, err
	}
	if body, ok := request.(*pb.CSChatCache); ok && body.GetChatType() == constUtil.P13UnionFightChatType {
		messages, err := s.p13UFChatHistory(ctx, id, p13UFNow(ctx))
		if err != nil {
			return nil, true, err
		}
		return []proto.Message{&pb.SCChatCache{ChatList: messages[0].(*pb.SCUFChatCache).ChatList}}, true, nil
	}
	if messages, handled, err := s.DispatchP13UnionFightField(ctx, id, request); handled {
		return messages, true, err
	}
	now := p13UFNow(ctx)
	var messages []proto.Message
	var err error
	switch request.(type) {
	case *pb.CSUFFightRecord, *pb.CSUFFightRecordReplay:
		messages, err = s.p13UFRecords(ctx, id, request, now)
	case *pb.CSUFPlayerAskFight:
		if replies, handled, retryErr := s.p13UFPlayerBattleRetry(ctx, id, request.(*pb.CSUFPlayerAskFight), stamp); handled || retryErr != nil {
			return replies, true, retryErr
		}
		if err = s.p13UFObserveFight(ctx, id, request.(*pb.CSUFPlayerAskFight), stamp, now); err != nil {
			return nil, true, err
		}
		if replies, handled, battleErr := s.p13UFTryPlayerBattle(ctx, id, request.(*pb.CSUFPlayerAskFight), stamp, now); handled || battleErr != nil {
			return replies, true, battleErr
		}
		messages, err = s.p13UFAskFightAnyAudit(ctx, id, request.(*pb.CSUFPlayerAskFight), stamp, now)
	case *pb.CSUFRankInfo:
		if b := request.(*pb.CSUFRankInfo); b.GetRankType() == 2 {
			messages, err = s.P13UnionFightMonthlyRank(ctx, id, b, now)
		} else {
			messages, err = s.p13UFWeeklyQuery(ctx, id, b, now)
		}
	case *pb.CSUFPlayerMissionList, *pb.CSUFUnionMissionList:
		messages, err = s.p13UFMissionQuery(ctx, id, request, now)
	case *pb.CSUFFightResult, *pb.CSUFUnionResultInfo:
		messages, err = s.p13UFSettlementQuery(ctx, id, request, now)
	case *pb.CSUFTalk:
		messages, err = s.p13UFChat(ctx, id, request, stamp, now)
	case *pb.CSUFOpenFactoryCost:
		messages, err = s.p13UFFactoryCost(ctx, id, request.(*pb.CSUFOpenFactoryCost), now)
	case *pb.CSUFFieldCariotFactoryOpen:
		messages, err = s.p13UFFactoryOpen(ctx, id, request.(*pb.CSUFFieldCariotFactoryOpen), stamp, now)
	case *pb.CSUFShopList:
		messages, err = s.p13UFShopList(ctx, id, request.(*pb.CSUFShopList), now)
	case *pb.CSUFPlayerUseItem:
		messages, err = s.p13UFItemUse(ctx, id, request.(*pb.CSUFPlayerUseItem), stamp, now)
	case *pb.CSUFPlayerBuyItem:
		messages, err = s.p13UFShopBuy(ctx, id, request.(*pb.CSUFPlayerBuyItem), stamp, now)
	case *pb.CSUFBeginTransfer:
		messages, err = s.p13UFTransferBegin(ctx, id, request.(*pb.CSUFBeginTransfer), now)
	case *pb.CSUFEndTransfer:
		messages, err = s.p13UFTransferEnd(ctx, id, request.(*pb.CSUFEndTransfer), now)
	case *pb.CSUFPlayerCollect:
		messages, err = s.p13UFResourceBegin(ctx, id, request.(*pb.CSUFPlayerCollect), now)
	case *pb.CSUFPlayerUpResource:
		messages, err = s.p13UFResourceDeposit(ctx, id, request.(*pb.CSUFPlayerUpResource), stamp, now)
	case *pb.CSUFLeaderFocus, *pb.CSUFLeaderFocusInfo, *pb.CSUFFocusPointInfo:
		messages, err = s.p13UFFocus(ctx, id, request, stamp, now)
	case *pb.CSUFCurrentStageInfo, *pb.CSUFAppliedPlayer, *pb.CSUFInfoPreView:
		messages, err = s.p13UFQuery(ctx, id, request, now)
	case *pb.CSUFUnionApply, *pb.CSUFAutoApplySet, *pb.CSUFPlayerApply, *pb.CSUFSetAutoFight, *pb.CSUFUpdateBattlePlayer, *pb.CSUFAdjustFirstOn, *pb.CSUFSetFightItems:
		messages, err = s.p13UFMutate(ctx, id, request, stamp, now)
	default:
		return nil, false, nil
	}
	return messages, true, err
}
