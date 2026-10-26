package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ShopRefresh75Status shares original role authorization and the trusted game calendar.
// It reads the serving shop window without opening an activity or modifying a quota.
func (s *GameService) ShopRefresh75Status(ctx context.Context, id int64, session []byte) (*model.ShopRefresh75StatusResponse, error) {
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	owner, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	if AdminProfileServerID(*owner) != s.OriginalServerID() || owner.Player == nil {
		return nil, constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return nil, err
	}
	player, _, err := decodeProfile(owner)
	if err != nil {
		return nil, err
	}
	now := LocalPlayNow(ctx)
	day := now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	reset := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location()).UnixMilli()
	out := &model.ShopRefresh75StatusResponse{ServerTime: now.UnixMilli(), Menus: []model.ShopRefresh75Menu{}}
	for i, name := range []string{"商品", "紫装", "橙装"} {
		at := reset
		out.Menus = append(out.Menus, model.ShopRefresh75Menu{Group: int64(i + 1), Name: name, Period: "day", Available: true, ResetAt: &at})
	}
	weekly := model.ShopRefresh75Menu{Group: 4, Name: "红装", Period: "week"}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		key, managed, err := s.AdminScheduledOrdinaryShopConsumer(tx, "point", now.UnixMilli())
		if err != nil || !managed || key == "" {
			return err
		}
		// Only an actual native shop-window catalogue establishes this weekly reset.
		if !strings.HasPrefix(key, "shop_schedule_") {
			return nil
		}
		var live db.P4CommerceRule
		err = tx.Session(&gorm.Session{}).Where("id=? AND kind=? AND shop_type=? AND enabled=?", key, "point", 5, true).First(&live).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		cfg, rules, err := s.p4CommerceSnapshot(live)
		if err != nil {
			return err
		}
		if cfg.EndAt-cfg.StartAt != 7*86400000 || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			return nil
		}
		for _, product := range rules.Products {
			if !product.isEnabled() || player.GetLevel() < product.MinLevel || player.GetVip() < product.MinVIP {
				continue
			}
			rewards, err := p1RewardResources(product.Rewards)
			if err != nil {
				return err
			}
			if s.pointShop48Priority(product.Priority, rewards)/1000 != 4 {
				continue
			}
			at := cfg.EndAt
			weekly.Available, weekly.ResetAt = true, &at
			break
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out.Menus = append(out.Menus, weekly)
	return out, nil
}
