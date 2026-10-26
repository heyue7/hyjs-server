package service

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"math"
	"time"
)

type p13UFItemBudgetConfig struct {
	Rule                        string
	Limit, TargetCooldownMillis int64
	Family                      string
}

func (s *GameService) InitializeP13UnionFightItemBudget(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightItemTeamUse{}, &db.P13UnionFightItemTargetCooldown{})
}
func p13UFItemBudgetError() error { return errors.New(constUtil.P13UFItemBudgetConflict) }
func p13UFItemBudgetValidate(item string, cfg p13UFItemBudgetConfig) error {
	if cfg.Rule == "" {
		if cfg.Limit != 0 || cfg.TargetCooldownMillis != 0 || cfg.Family != "" {
			return p13UFItemBudgetError()
		}
		return nil
	}
	family := ""
	switch item {
	case "ufitem_020":
		family = constUtil.P13UFItemBudgetHammer
	case "ufitem_019":
		family = constUtil.P13UFItemBudgetRepair
	case "ufitem_017", "ufitem_018":
		family = constUtil.P13UFItemBudgetBomb
	default:
		return p13UFItemBudgetError()
	}
	if cfg.Rule != constUtil.P13UFItemBudgetRule || cfg.Limit < 1 || cfg.Limit > 1000000000 || cfg.TargetCooldownMillis != constUtil.P13UFItemBudgetTargetCooldownMillis || cfg.Family != family {
		return p13UFItemBudgetError()
	}
	return nil
}
func p13UFItemTeamUseHash(row *db.P13UnionFightItemTeamUse) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFItemTargetCooldownHash(row *db.P13UnionFightItemTargetCooldown) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// Called only after the existing authority, inventory and effective-target checks,
// under guild policy then target locks. The caller owns the whole transaction:
// quota, target clock, effect, stock and original receipt all commit together.
func p13UFItemBudgetConsume(tx *gorm.DB, match string, camp int64, item string, target int64, offerHash string, cfg p13UFItemBudgetConfig, now time.Time) error {
	if err := p13UFItemBudgetValidate(item, cfg); err != nil {
		return err
	}
	if cfg.Rule == "" {
		return nil
	}
	if tx == nil || len(match) != 64 || (camp != 0 && camp != 1) || target <= 0 || len(offerHash) != 64 || now.UnixMilli() <= 0 || now.UnixMilli() > math.MaxInt64-cfg.TargetCooldownMillis {
		return p13UFItemBudgetError()
	}
	var used db.P13UnionFightItemTeamUse
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&used, "match_id=? AND camp=? AND item_id=?", match, camp, item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		used = db.P13UnionFightItemTeamUse{MatchID: match, Camp: camp, ItemID: item, Limit: cfg.Limit, Rule: cfg.Rule, OfferHash: offerHash}
	} else if err != nil {
		return err
	} else if used.SourceHash != p13UFItemTeamUseHash(&used) || used.Used < 0 || used.Used > used.Limit || used.Limit != cfg.Limit || used.Rule != cfg.Rule || used.OfferHash != offerHash {
		return p13UFItemBudgetError()
	}
	if used.Used >= used.Limit {
		return errors.New(constUtil.P13UFItemBudgetExhausted)
	}
	var cooldown db.P13UnionFightItemTargetCooldown
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cooldown, "match_id=? AND target_guid=? AND effect_family=?", match, target, cfg.Family).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cooldown = db.P13UnionFightItemTargetCooldown{MatchID: match, TargetGUID: target, EffectFamily: cfg.Family, Rule: cfg.Rule}
	} else if err != nil {
		return err
	} else if cooldown.SourceHash != p13UFItemTargetCooldownHash(&cooldown) || cooldown.Until <= 0 || cooldown.Rule != cfg.Rule || len(cooldown.OfferHash) != 64 {
		return p13UFItemBudgetError()
	}
	if cooldown.Until > now.UnixMilli() {
		return errors.New(constUtil.P13UFItemTargetCooling)
	}
	used.Used++
	used.SourceHash = p13UFItemTeamUseHash(&used)
	cooldown.Until = now.UnixMilli() + cfg.TargetCooldownMillis
	cooldown.OfferHash = offerHash
	cooldown.SourceHash = p13UFItemTargetCooldownHash(&cooldown)
	if err = tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&used).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&cooldown).Error
}
