package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"hyjs-server/model"
	"math"
	"math/big"
	"strings"
)

func shopDrawQuantityTier(tiers []model.AdminShopQuantityTier) (*model.AdminShopQuantityTier, error) {
	if len(tiers) == 0 {
		return nil, nil
	}
	var total int64
	for _, tier := range tiers {
		if tier.Weight < 1 || tier.Weight > math.MaxInt64-total {
			return nil, errors.New("数量档权重无效或溢出")
		}
		total += tier.Weight
	}
	value, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return nil, err
	}
	roll := value.Int64()
	for _, tier := range tiers {
		if roll < tier.Weight {
			copy := tier
			return &copy, nil
		}
		roll -= tier.Weight
	}
	return nil, errors.New("数量档抽取失败")
}

func (s *GameService) validateShopQuantityTiers(key, resource string, tiers []model.AdminShopQuantityTier) error {
	if len(tiers) == 0 {
		return nil
	}
	if (key != "memory" && key != "pet-market" && key != "ggo") || len(tiers) > 64 {
		return errors.New("此商店不支持数量档或档位过多")
	}
	seen := map[int64]bool{}
	var sum int64
	for _, tier := range tiers {
		if tier.Amount < 1 || tier.Amount > math.MaxInt32 || tier.Weight < 1 || tier.Weight > math.MaxInt64-sum || seen[tier.Amount] || resource == "hero_exp_004" && tier.Amount > 5 || tier.Cost.Amount < 1 || tier.Cost.Amount > math.MaxInt32 || !ggoShopCostAllowed(key, tier.Cost.ResourceID) {
			return errors.New("数量档数量、权重或消耗无效；金色经验材料每档最多5个")
		}
		if _, err := s.p4ParseCost(fmt.Sprintf("%s,%d", tier.Cost.ResourceID, tier.Cost.Amount)); err != nil {
			return err
		}
		sum += tier.Weight
		seen[tier.Amount] = true
	}
	return nil
}

func shopRefreshEnabled(value *bool, fallback bool) bool {
	return value == nil && fallback || value != nil && *value
}

func shopResourceSpec(v model.AdminResourceAmount) string {
	if v.ResourceID == "" && v.Amount == 0 {
		return ""
	}
	return fmt.Sprintf("%s,%d", v.ResourceID, v.Amount)
}
func shopP4RefreshSettings(key string, r p4CommerceRules) *model.AdminShopRefreshSettings {
	out := &model.AdminShopRefreshSettings{Enabled: shopRefreshEnabled(r.RefreshEnabled, key == "memory"), FreeCap: int64(r.FreeCap), FreeRecoverySeconds: r.FreeSeconds, PaidCap: int64(r.ResourceRefreshCap), Cost: adminTypedResourceOne(r.RefreshCost), Period: "day"}
	if r.RefreshItem != "" {
		v := adminTypedResourceOne(r.RefreshItem)
		out.Item = &v
	}
	return out
}
func (s *GameService) validateShopRefreshSettings(key string, r *model.AdminShopRefreshSettings) error {
	if r == nil {
		return nil
	}
	period := "day"
	if key == "pet-market" {
		period = "week"
	}
	if (key != "ggo" && key != "memory" && key != "pet-market") || r.Period != period || r.FreeCap < 0 || r.FreeCap > math.MaxInt32 || r.PaidCap < 0 || r.PaidCap > math.MaxInt32 || r.FreeRecoverySeconds < 0 || r.FreeRecoverySeconds > math.MaxInt64/1000 || r.FreeCap > 0 && r.FreeRecoverySeconds < 1 || key == "pet-market" && (r.FreeCap != 0 || r.FreeRecoverySeconds != 0 || r.Item != nil) {
		return errors.New("刷新配置无效")
	}
	if r.PaidCap > 0 {
		if r.Cost.Amount < 1 || r.Cost.Amount > math.MaxInt32 || !ggoShopCostAllowed(key, r.Cost.ResourceID) {
			return errors.New("刷新消耗无效")
		}
		if _, err := s.p4ParseCost(shopResourceSpec(r.Cost)); err != nil {
			return err
		}
	}
	if r.Item != nil {
		if key != "memory" || r.Item.Amount < 1 || r.Item.Amount > math.MaxInt32 {
			return errors.New("刷新道具无效")
		}
		if _, err := s.p4ParseCost(shopResourceSpec(*r.Item)); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) shopQuantityRewardResource(id string) bool {
	if s.table("Item", "itemId", id) != nil || s.table("Frag", "id", id) != nil {
		return true
	}
	return strings.HasPrefix(id, "hero_exp_") && s.table("Hero", "id", id) != nil
}
