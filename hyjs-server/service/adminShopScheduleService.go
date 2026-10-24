package service

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

func adminOrdinarySchedulableShop(key string) bool {
	switch key {
	case "ordinary", "memory", "popularity", "popularity-rank", "point", "score", "guild":
		return true
	}
	return false
}

func adminSchedulableShop(key string) bool {
	return adminOrdinarySchedulableShop(key) || key == "vip" || key == "pet-market" || key == "union-fight"
}
func adminShopGateOccurrence(row db.AdminActivityOccurrence) bool {
	if row.Mode != "shop-window" {
		return false
	}
	var form model.AdminActivityForm
	return json.Unmarshal(row.FormJSON, &form) == nil && !adminOrdinarySchedulableShop(form.Options.ShopKey) && adminSchedulableShop(form.Options.ShopKey)
}

// Shop scheduling retains the actual native ShopType/Kind and catalogue rules.
// A sale activity (shopType 0) is never substituted for an ordinary shop.
func (s *GameService) adminShopScheduleSourceForm(tx *gorm.DB, key string) (model.AdminActivityForm, error) {
	form := model.AdminActivityForm{Mode: "shop-window", TemplateID: "shop-window-original", Source: "商店已保存目录", Options: model.AdminActivityOptions{ShopKey: key}}
	if !adminSchedulableShop(key) {
		return form, constUtil.ErrAdminInput
	}
	if !adminOrdinarySchedulableShop(key) {
		shop, _, err := s.adminReadShop(tx, key)
		if err != nil {
			return form, err
		}
		ready := key == "pet-market" && shop.Settings.Enabled
		for _, item := range shop.Items {
			ready = ready || item.Enabled
		}
		if len(shop.Items) == 0 || !ready {
			return form, constUtil.ErrAdminConfiguration
		}
		for _, d := range AdminShopDefinitions() {
			if d.ID == key {
				form.Name = d.Label + " · 限时"
				break
			}
		}
		form.Source = "仅控制开放时间，目录与玩法资格沿用原商店"
		form.Options.ShopSource = "native-gate:" + shop.Version
		return form, nil
	}
	source, err := s.AdminOrdinaryShopSource(tx, key)
	if err != nil {
		return form, err
	}
	if !source.Enabled {
		return form, constUtil.ErrP4CommerceNotOpen
	}
	rules, err := s.validateP4Commerce(source)
	if err != nil {
		return form, err
	}
	if len(rules.Products) == 0 {
		return form, constUtil.ErrAdminConfiguration
	}
	for _, d := range AdminShopDefinitions() {
		if d.ID == key {
			form.Name = d.Label + " · 限时"
			break
		}
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return form, err
	}
	form.Options.ShopSource = string(raw)
	for _, p := range rules.Products {
		form.Products = append(form.Products, model.AdminActivityProduct{ID: p.ID, Name: p.Name, Index: int64(p.Index), Rewards: adminTypedResources(p.Rewards), Cost: adminTypedResourceOne(p.Cost), DailyLimit: int64(p.DailyLimit), TotalLimit: int64(p.TotalLimit), Stock: int64(p.Stock), MinLevel: int64(p.MinLevel), MinVip: int64(p.MinVIP), DiscountBps: int64(p.DiscountBps), Weight: p.Weight})
	}
	return form, nil
}

// Refresh reusable scheduling inputs only. Existing issued occurrences keep
// their immutable form/source bytes and historical native consumers.
func (s *GameService) syncAdminShopScheduleDefinitions(tx *gorm.DB) error {
	if !gameSchemaHasTable(tx, &db.AdminShopAvailability{}) {
		return nil
	}
	for _, shop := range AdminShopDefinitions() {
		if !adminSchedulableShop(shop.ID) {
			continue
		}
		availability, err := AdminShopAvailability(tx, shop.ID)
		if err != nil {
			return err
		}
		if availability == nil {
			continue
		}
		id := "shop_window_" + shop.ID
		var row db.AdminActivityDefinition
		found := tx.Where("id=?", id).Take(&row).Error
		if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		if errors.Is(found, gorm.ErrRecordNotFound) && availability.Unlimited {
			continue
		}
		form, sourceErr := s.adminShopScheduleSourceForm(tx, shop.ID)
		usable := sourceErr == nil && !availability.Unlimited
		reason := ""
		if availability.Unlimited {
			reason = "商店当前不限时；关闭后可排期"
		} else if sourceErr != nil {
			reason = "请先在商店管理保存可用商品目录并启用"
		}
		if errors.Is(found, gorm.ErrRecordNotFound) {
			form.RepeatPolicy = "repeat"
			row = db.AdminActivityDefinition{ID: id, Name: shop.Label + " · 限时", Mode: "shop-window", DefaultDays: 7, Enabled: true, Revision: 1, SourceActivityID: "shop:" + shop.ID}
		} else {
			var prior model.AdminActivityForm
			if json.Unmarshal(row.FormJSON, &prior) != nil {
				return constUtil.ErrAdminConfiguration
			}
			form.RepeatPolicy = prior.RepeatPolicy
			if sourceErr != nil {
				form = prior
			}
		}
		// Older auto-generated templates inherited disabled source readiness.
		// An untouched auto template becomes selectable when its source is ready;
		// an operator's explicit template save (including stop) remains authoritative.
		restoreEnabled := false
		if found == nil && usable && !row.Enabled && row.SourceActivityID == "shop:"+shop.ID {
			var edits int64
			if err := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", "activity.template.save", row.ID).Count(&edits).Error; err != nil {
				return err
			}
			restoreEnabled = edits == 0
		}
		form.Name = row.Name
		next := adminJSON(form)
		if string(next) == string(row.FormJSON) && row.Usable == usable && row.UnavailableReason == reason && !restoreEnabled {
			continue
		}
		if restoreEnabled {
			row.Enabled = true
		}
		row.FormJSON = next
		row.Usable = usable
		row.UnavailableReason = reason
		row.UpdatedAtMillis = time.Now().UnixMilli()
		if found == nil {
			row.Revision++
		}
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) adminPersistShopSchedule(tx *gorm.DB, key string, form model.AdminActivityForm, enabled, exists bool) error {
	if !adminSchedulableShop(form.Options.ShopKey) || form.Options.ShopSource == "" {
		return constUtil.ErrAdminInput
	}
	if form.RepeatPolicy != "" && form.RepeatPolicy != "once" && form.RepeatPolicy != "repeat" {
		return constUtil.ErrAdminInput
	}
	if !adminOrdinarySchedulableShop(form.Options.ShopKey) {
		return nil
	}
	var source db.P4CommerceRule
	if json.Unmarshal([]byte(form.Options.ShopSource), &source) != nil || source.Kind != form.Options.ShopKey || source.ShopType <= 0 || !source.Enabled {
		return constUtil.ErrAdminInput
	}
	source.ID = key
	source.StartAt = form.StartAt
	source.EndAt = form.EndAt
	source.ClaimEndAt = form.ClaimEndAt
	source.Enabled = enabled
	source.Frozen = nil
	if _, err := s.validateP4Commerce(source); err != nil {
		return err
	}
	if exists {
		return tx.Save(&source).Error
	}
	return tx.Create(&source).Error
}

// A limited ordinary shop consumes its serving realm's published window. An
// explicitly published empty calendar closes it. Unlimited shops use the normal
// global catalogue. Personal relative windows retain their role ownership.
func (s *GameService) AdminScheduledOrdinaryShopConsumer(tx *gorm.DB, shopKey string, now int64) (string, bool, error) {
	if !adminOrdinarySchedulableShop(shopKey) {
		return "", false, nil
	}
	return s.adminScheduledShopConsumer(tx, shopKey, now)
}
func (s *GameService) AdminScheduledShopWindow(tx *gorm.DB, shopKey string, now int64) (bool, bool, error) {
	key, managed, err := s.adminScheduledShopConsumer(tx, shopKey, now)
	return key != "", managed, err
}
func (s *GameService) adminScheduledShopConsumer(tx *gorm.DB, shopKey string, now int64) (string, bool, error) {
	if !adminSchedulableShop(shopKey) {
		return "", false, nil
	}
	availability, err := AdminShopAvailability(tx, shopKey)
	if err != nil {
		return "", false, err
	}
	if availability == nil || availability.Unlimited {
		return "", false, nil
	}
	tx = tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameServiceContextKey, s))
	managed, err := adminActivityCalendarManaged(tx)
	if err != nil || !managed {
		return "", managed, err
	}
	owner := GameCalendarOwner(tx.Statement.Context)
	if owner > 0 && gameSchemaHasTable(tx, &db.AdminCharacterActivityInstance{}) {
		var issued []db.AdminCharacterActivityInstance
		if err = tx.Where("server_id=? AND player_id=? AND mode=? AND start_at<=? AND end_at>?", s.OriginalServerID(), owner, "shop-window", now, now).Find(&issued).Error; err != nil {
			return "", true, err
		}
		for _, instance := range issued {
			var form model.AdminActivityForm
			if json.Unmarshal(instance.FormJSON, &form) != nil {
				return "", true, constUtil.ErrAdminConfiguration
			}
			if form.Options.ShopKey != shopKey {
				continue
			}
			allowed, e := adminCharacterActivityAccess(tx, instance.ConsumerKey)
			if e != nil {
				return "", true, e
			}
			if allowed {
				return instance.ConsumerKey, true, nil
			}
		}
	}
	var rows []db.AdminActivityOccurrence
	if err = tx.Where("server_id=? AND mode=? AND timing<>? AND enabled=? AND archived=? AND start_at<=? AND end_at>?", s.OriginalServerID(), "shop-window", "character-day", true, false, now, now).Find(&rows).Error; err != nil {
		return "", true, err
	}
	found := ""
	for _, row := range rows {
		var form model.AdminActivityForm
		if json.Unmarshal(row.FormJSON, &form) != nil {
			return "", true, constUtil.ErrAdminConfiguration
		}
		if form.Options.ShopKey != shopKey {
			continue
		}
		if owner > 0 {
			allowed, e := adminAcceptActivityTemplate(tx, owner, row)
			if e != nil {
				return "", true, e
			}
			if !allowed {
				continue
			}
		}
		if found != "" {
			return "", true, constUtil.ErrAdminConfiguration
		}
		found = row.ConsumerKey
	}
	return found, true, nil
}
