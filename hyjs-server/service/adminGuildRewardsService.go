package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminGuildRewardProjection(value any) (model.AdminGuildRewardRow, error) {
	var out model.AdminGuildRewardRow
	out.RowVersion = adminVersion(value)
	out.Editable = true
	var rewards string
	switch p := value.(type) {
	case db.P13UnionFightSettlementPolicy:
		out.AdminGuildRewardPolicy = model.AdminGuildRewardPolicy{Kind: "settlement", Group: p.Group, Enabled: p.Enabled, Version: p.Version, Source: p.Source, ParticipationRewards: adminTypedResources(p.ParticipationRewards), VictoryRewards: adminTypedResources(p.VictoryRewards), TransportMVPRewards: adminTypedResources(p.TransportMVPRewards), TaskRewards: adminTypedResources(p.TaskRewards), PersonalMissionsJSON: p.PersonalMissions, FactoryMissionPrestige: p.FactoryMissionPrestige, WinPrestige: p.WinPrestige, LosePrestige: p.LosePrestige}
		out.PublishedAt = p.PublishedAt
	case db.P13UnionFightWeeklyPolicy:
		out.AdminGuildRewardPolicy = model.AdminGuildRewardPolicy{Kind: "weekly", Group: p.Group, WeekStart: p.WeekStart, Enabled: p.Enabled, Version: p.Version, Source: p.Source}
		out.PublishedAt, rewards = p.PublishedAt, p.Rewards
	case db.P13UnionFightMonthlyPolicy:
		out.AdminGuildRewardPolicy = model.AdminGuildRewardPolicy{Kind: "monthly", Month: p.Month, Enabled: p.Enabled, Version: p.Version, Source: p.Source, MemberTitle: p.MemberTitle, ChairmanTitle: p.ChairmanTitle}
		out.PublishedAt, rewards = p.PublishedAt, p.Rewards
	default:
		return out, constUtil.ErrAdminInput
	}
	out.Ranks = []model.AdminGuildRewardRank{}
	if rewards != "" {
		var ranks []p13UFWeeklyReward
		if err := json.Unmarshal([]byte(rewards), &ranks); err != nil {
			return out, err
		}
		for _, rank := range ranks {
			out.Ranks = append(out.Ranks, model.AdminGuildRewardRank{Rank: rank.Rank, Rewards: adminTypedResources(rank.Rewards)})
		}
	}
	return out, nil
}

func adminGuildRewardFuture(kind, period string, now time.Time) bool {
	var start time.Time
	var err error
	switch kind {
	case "weekly":
		start, err = p13UFWeeklyStart(period)
	case "monthly":
		start, _, err = p13UFMonth(period)
	default:
		return kind == "settlement"
	}
	return err == nil && start.After(now)
}

func adminGuildRewardFrozen(tx *gorm.DB, row model.AdminGuildRewardPolicy) (bool, error) {
	var count int64
	var err error
	switch row.Kind {
	case "weekly":
		err = tx.Model(&db.P13UnionFightWeeklyOffer{}).Where("week_start=? AND `group`=?", row.WeekStart, row.Group).Count(&count).Error
	case "monthly":
		err = tx.Model(&db.P13UnionFightMonthlyOffer{}).Where("month=?", row.Month).Count(&count).Error
	case "settlement":
		// Match snapshots remain independent when a new group policy is published.
		err = tx.Model(&db.P13UnionFightSettlementOffer{}).Joins("JOIN p13_union_fight_matches ON p13_union_fight_matches.id = p13_union_fight_settlement_offers.match_id").Where("p13_union_fight_matches.`group`=?", row.Group).Count(&count).Error
	default:
		return false, constUtil.ErrAdminInput
	}
	return count > 0, err
}

// AdminGuildRewards reads policies without modifying frozen offers or awards.
func (s *GameService) AdminGuildRewards(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminGuildRewardsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	out := &model.AdminGuildRewardsResponse{Items: []model.AdminGuildRewardRow{}}
	var settlement []db.P13UnionFightSettlementPolicy
	var weekly []db.P13UnionFightWeeklyPolicy
	var monthly []db.P13UnionFightMonthlyPolicy
	if err := tx.Order("`group`").Find(&settlement).Error; err != nil {
		return nil, err
	}
	if err := tx.Order("week_start DESC, `group`").Find(&weekly).Error; err != nil {
		return nil, err
	}
	if err := tx.Order("month DESC").Find(&monthly).Error; err != nil {
		return nil, err
	}
	values := []any{}
	for _, row := range settlement {
		values = append(values, row)
	}
	for _, row := range weekly {
		values = append(values, row)
	}
	for _, row := range monthly {
		values = append(values, row)
	}
	now := LocalPlayNow(ctx)
	for _, value := range values {
		row, err := adminGuildRewardProjection(value)
		if err != nil {
			return nil, err
		}
		row.Frozen, err = adminGuildRewardFrozen(tx, row.AdminGuildRewardPolicy)
		if err != nil {
			return nil, err
		}
		if row.Kind != "settlement" {
			period := row.Month
			if row.Kind == "weekly" {
				period = row.WeekStart
			}
			row.Editable = !row.Frozen && adminGuildRewardFuture(row.Kind, period, now)
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

func (s *GameService) adminGuildRewardValue(req model.AdminGuildRewardPolicy, now time.Time) (any, error) {
	ranks := []p13UFWeeklyReward{}
	for _, rank := range req.Ranks {
		reward, err := s.adminTypedResourceSpec(rank.Rewards, false)
		if err != nil {
			return nil, err
		}
		ranks = append(ranks, p13UFWeeklyReward{Rank: rank.Rank, Rewards: reward})
	}
	raw, _ := json.Marshal(ranks)
	switch req.Kind {
	case "settlement":
		p := db.P13UnionFightSettlementPolicy{Group: req.Group, Enabled: req.Enabled, Version: req.Version, Source: req.Source, PublishedAt: now.UnixMilli(), PersonalMissions: req.PersonalMissionsJSON, FactoryMissionPrestige: req.FactoryMissionPrestige, WinPrestige: req.WinPrestige, LosePrestige: req.LosePrestige}
		lists := []struct {
			value    []model.AdminResourceAmount
			target   *string
			optional bool
		}{{req.ParticipationRewards, &p.ParticipationRewards, !req.Enabled}, {req.VictoryRewards, &p.VictoryRewards, !req.Enabled}, {req.TransportMVPRewards, &p.TransportMVPRewards, true}, {req.TaskRewards, &p.TaskRewards, true}}
		for _, list := range lists {
			value, err := s.adminTypedResourceSpec(list.value, list.optional)
			if err != nil {
				return nil, err
			}
			*list.target = value
		}
		if len(p.TransportMVPRewards) > 2048 || len(p.TaskRewards) > 2048 {
			return nil, constUtil.ErrAdminInput
		}
		if int64(p.WinPrestige)+int64(p.FactoryMissionPrestige) > math.MaxInt32 || int64(p.LosePrestige)+int64(p.FactoryMissionPrestige) > math.MaxInt32 {
			return nil, constUtil.ErrAdminInput
		}
		if _, err := p13UFMissionCatalog(&p); err != nil {
			return nil, err
		}
		return p, nil
	case "weekly":
		return db.P13UnionFightWeeklyPolicy{WeekStart: req.WeekStart, Group: req.Group, Enabled: req.Enabled, Version: req.Version, Source: req.Source, PublishedAt: now.UnixMilli(), Rewards: string(raw)}, nil
	case "monthly":
		if req.MemberTitle && !s.adminGuildRewardTitleValid("title_028") || req.ChairmanTitle && !s.adminGuildRewardTitleValid("title_029") {
			return nil, constUtil.ErrAdminInput
		}
		return db.P13UnionFightMonthlyPolicy{Month: req.Month, Enabled: req.Enabled, Version: req.Version, Source: req.Source, PublishedAt: now.UnixMilli(), Rewards: string(raw), MemberTitle: req.MemberTitle, ChairmanTitle: req.ChairmanTitle}, nil
	}
	return nil, constUtil.ErrAdminInput
}

// AdminSaveGuildReward serializes publication with consumers and checks the entire prior row.
func (s *GameService) AdminSaveGuildReward(ctx context.Context, actor *model.AdminPrincipal, req model.AdminGuildRewardSaveRequest, create bool) (*model.AdminGuildRewardRow, error) {
	now := LocalPlayNow(ctx)
	var result model.AdminGuildRewardRow
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var prior any
		var readErr error
		lock := tx.Clauses(clause.Locking{Strength: "UPDATE"})
		switch req.Kind {
		case "settlement":
			var p db.P13UnionFightSettlementPolicy
			readErr = lock.First(&p, "`group`=?", req.Group).Error
			prior = p
		case "weekly":
			var p db.P13UnionFightWeeklyPolicy
			readErr = lock.First(&p, "week_start=? AND `group`=?", req.WeekStart, req.Group).Error
			prior = p
		case "monthly":
			var p db.P13UnionFightMonthlyPolicy
			readErr = lock.First(&p, "month=?", req.Month).Error
			prior = p
		default:
			return constUtil.ErrAdminInput
		}
		if create {
			if req.Kind == "settlement" || readErr == nil {
				return constUtil.ErrAdminConfigConflict
			}
			if !errors.Is(readErr, gorm.ErrRecordNotFound) {
				return readErr
			}
		} else {
			if readErr != nil {
				return readErr
			}
			if adminVersion(prior) != req.RowVersion {
				return constUtil.ErrAdminConfigConflict
			}
		}
		if req.Kind != "settlement" {
			period := req.Month
			if req.Kind == "weekly" {
				period = req.WeekStart
			}
			frozen, err := adminGuildRewardFrozen(tx, req.AdminGuildRewardPolicy)
			if err != nil {
				return err
			}
			if frozen || !adminGuildRewardFuture(req.Kind, period, now) {
				return constUtil.ErrAdminConfigConflict
			}
		}
		value, err := s.adminGuildRewardValue(req.AdminGuildRewardPolicy, now)
		if err != nil {
			return err
		}
		// Only the mutable publication row is saved; frozen offers and receipts stay untouched.
		switch v := value.(type) {
		case db.P13UnionFightSettlementPolicy:
			// The existing reward form edits amounts and ranks. Preserve the
			// separately published combat contract when this older DTO omits it.
			old := prior.(db.P13UnionFightSettlementPolicy)
			v.TerminalRule, v.DrawPrestige = old.TerminalRule, old.DrawPrestige
			v.AbortRule, v.FinalizationGraceMillis = old.AbortRule, old.FinalizationGraceMillis
			if err = tx.Save(&v).Error; err != nil {
				return err
			}
			value = v
		case db.P13UnionFightWeeklyPolicy:
			if err = tx.Save(&v).Error; err != nil {
				return err
			}
		case db.P13UnionFightMonthlyPolicy:
			if err = tx.Save(&v).Error; err != nil {
				return err
			}
		}
		result, err = adminGuildRewardProjection(value)
		if err != nil {
			return err
		}
		result.Frozen, err = adminGuildRewardFrozen(tx, req.AdminGuildRewardPolicy)
		if err != nil {
			return err
		}
		return adminAudit(tx, actor, "guild.reward.save", req.Kind+"/"+req.Group+req.WeekStart+req.Month, req.Reason+" | "+req.RowVersion+" -> "+result.RowVersion)
	})
	return &result, err
}

func (s *GameService) adminGuildRewardTitleValid(id string) bool {
	row := s.table("Title", "titleId", id)
	hours := int64(tableNumber(row, "effectTime"))
	return row != nil && hours > 0 && hours <= 87600 && row["effects"] != ""
}
