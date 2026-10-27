package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"hyjs-server/model"
)

// trialShopReprice changes only the four explicitly approved red equipment
// fragment prices and star thresholds. Other catalogue fields remain intact.
func trialShopReprice(raw []byte) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	var products []map[string]json.RawMessage
	if err := json.Unmarshal(document["products"], &products); err != nil {
		return nil, err
	}
	wanted := map[string]int32{"frag_equip_037,10": 135, "frag_equip_038,10": 165, "frag_equip_039,10": 135, "frag_equip_040,10": 195}
	seen := make(map[string]bool, len(wanted))
	for _, product := range products {
		var reward string
		if err := json.Unmarshal(product["rewards"], &reward); err != nil {
			return nil, errors.New("试炼商品奖励格式无效")
		}
		stars, ok := wanted[reward]
		if !ok {
			continue
		}
		if seen[reward] {
			return nil, fmt.Errorf("试炼红装碎片商品重复：%s", reward)
		}
		var cost string
		if err := json.Unmarshal(product["cost"], &cost); err != nil || cost != "point,15000" && cost != "point,150" {
			return nil, fmt.Errorf("试炼红装碎片当前价格不符合预期：%s", reward)
		}
		product["cost"] = json.RawMessage(`"point,150"`)
		product["minTrialStars"] = json.RawMessage(fmt.Sprint(stars))
		seen[reward] = true
	}
	if len(seen) != len(wanted) {
		return nil, errors.New("试炼红装碎片目录不完整")
	}
	encoded, err := json.Marshal(products)
	if err != nil {
		return nil, err
	}
	document["products"] = encoded
	// 与既有目录编辑一致：当前及scheduled消费者读取新目录，旧Frozen/回执保留。
	document["liveEditable"] = json.RawMessage(`true`)
	return json.Marshal(document)
}

// ApplyTrialShopReprice uses the same version lock and audit as shop edits.
// Callers must supply the version read from the current point catalogue.
func (s *GameService) ApplyTrialShopReprice(ctx context.Context, actor *model.AdminPrincipal, version string) (*model.AdminShopResponse, error) {
	return s.adminShopMutation(ctx, actor, "point", version, "trial.reprice", func(tx *gorm.DB, raw *adminShopStorage) error {
		if raw.Period == nil || raw.Period.Kind != "point" {
			return errors.New("试炼商店来源不可用")
		}
		updated, err := trialShopReprice(raw.Period.Rules)
		if err != nil {
			return err
		}
		candidate := *raw.Period
		candidate.Rules = updated
		if _, err = s.validateP4Commerce(candidate); err != nil {
			return err
		}
		return tx.Model(raw.Period).Update("rules", updated).Error
	})
}
