package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// CommerceCalendarWindow identifies all three persisted activity boundaries.
type CommerceCalendarWindow struct {
	StartAt, EndAt, ClaimEndAt int64
}

func commerceRestoreDecode(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return constUtil.ErrAdminConfiguration
	}
	return nil
}

func commerceRestoreWindow(row db.P4CommerceRule) CommerceCalendarWindow {
	return CommerceCalendarWindow{row.StartAt, row.EndAt, row.ClaimEndAt}
}

func (s *GameService) commerceRestoreFixedRules(cfg db.P4CommerceRule, allowEditable bool) error {
	// Guild mode adds contribution cost and guild level to each native product.
	// Decode its complete original structure, then validate its dual-cost and
	// level contract. Unknown refresh/quote fields remain rejected by this schema.
	if cfg.Kind == "guild" {
		var guild p4GuildRules
		if err := commerceRestoreDecode(cfg.Rules, &guild); err != nil {
			return err
		}
		if guild.LiveEditable && !allowEditable {
			return constUtil.ErrAdminConfiguration
		}
		for _, product := range guild.Products {
			if len(product.QuantityTiers) > 0 {
				return fmt.Errorf("commerce refresh semantics require review: %w", constUtil.ErrAdminConfiguration)
			}
		}
		_, err := s.p4GuildShopRules(cfg)
		return err
	}
	var rules p4CommerceRules
	if err := commerceRestoreDecode(cfg.Rules, &rules); err != nil {
		return err
	}
	if (rules.LiveEditable && !allowEditable) || ggoHasRandomQuotes(rules) || rules.FreeSeconds != 0 || rules.FreeCap != 0 || rules.ResourceRefreshCap != 0 || rules.RefreshItem != "" || rules.RefreshCost != "" || rules.Slots != 0 {
		return fmt.Errorf("commerce refresh semantics require review: %w", constUtil.ErrAdminConfiguration)
	}
	return nil
}

// RestoreCommerceCalendarWindow runs inside the caller's clock/profile-locked
// transaction. It preserves all progress/stock/quote bytes and changes only the
// exact reviewed live and frozen boundaries. It never creates a quote or state.
// Only reviewed scheduled point/guild shops and fixed activity catalogues are accepted.
// Stable-ID live SKU hashes contain the rule ID/product, with no window component.
func (s *GameService) RestoreCommerceCalendarWindow(tx *gorm.DB, ruleID string, old, next CommerceCalendarWindow) error {
	if tx == nil || tx.Statement == nil || ruleID == "" || old.StartAt <= 0 || old.EndAt <= old.StartAt || old.ClaimEndAt < old.EndAt || next.StartAt <= 0 || next.EndAt <= next.StartAt || next.ClaimEndAt < next.EndAt {
		return constUtil.ErrAdminConfiguration
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fmt.Errorf("commerce migration requires caller transaction: %w", constUtil.ErrAdminConfiguration)
	}
	var live db.P4CommerceRule
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", ruleID).Error; err != nil {
		return err
	}
	if commerceRestoreWindow(live) != old {
		return constUtil.ErrAdminConfigConflict
	}
	switch live.Kind {
	case "half", "discount", "exchange", "sale", "mystic":
		if live.ShopType != 0 {
			return constUtil.ErrAdminConfiguration
		}
	case "point", "guild":
		expected := int32(5)
		if live.Kind == "guild" {
			expected = 8
		}
		if live.ShopType != expected || !strings.HasPrefix(live.ID, "shop_schedule_"+live.Kind+"_") {
			return constUtil.ErrAdminConfiguration
		}
	default:
		return fmt.Errorf("unsupported commerce kind %q: %w", live.Kind, constUtil.ErrAdminConfiguration)
	}
	if err := s.commerceRestoreFixedRules(live, live.ShopType > 0); err != nil {
		return err
	}
	if live.ShopType > 0 {
		source, err := s.AdminOrdinaryShopSource(tx, live.Kind)
		if err != nil {
			return err
		}
		if err := s.commerceRestoreFixedRules(source, true); err != nil {
			return err
		}
	}
	if _, _, err := s.p4CommerceSnapshot(live); err != nil {
		return err
	}
	updates := map[string]any{"start_at": next.StartAt, "end_at": next.EndAt, "claim_end_at": next.ClaimEndAt}
	if len(live.Frozen) > 0 {
		var frozen db.P4CommerceRule
		if err := commerceRestoreDecode(live.Frozen, &frozen); err != nil {
			return fmt.Errorf("frozen rule %s: %w", ruleID, err)
		}
		// Different frozen commercial terms remain legitimate, but their payload
		// must be understood and their own boundaries must match the exact preimage.
		if frozen.ID != live.ID || frozen.ShopType != live.ShopType || frozen.Kind != live.Kind || commerceRestoreWindow(frozen) != old {
			return constUtil.ErrAdminConfigConflict
		}
		if err := s.commerceRestoreFixedRules(frozen, live.ShopType > 0); err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(live.Frozen, &fields); err != nil {
			return err
		}
		fields["startAt"], fields["endAt"], fields["claimEndAt"] = adminJSON(next.StartAt), adminJSON(next.EndAt), adminJSON(next.ClaimEndAt)
		raw, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		updates["frozen"] = raw
	}
	var states []db.P4CommerceState
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("rule_id IN ?", []string{ruleID, ggoQuoteRuleID(ruleID)}).Order("rule_id,player_id").Limit(10001).Find(&states).Error; err != nil {
		return err
	}
	if len(states) > 10000 {
		return constUtil.ErrAdminConfiguration
	}
	for _, row := range states {
		var progress p4CommerceProgress
		if row.RuleID != ruleID {
			return fmt.Errorf("unexpected persisted GGO quote for %s: %w", ruleID, constUtil.ErrAdminConfiguration)
		}
		if err := commerceRestoreDecode(row.Payload, &progress); err != nil {
			return fmt.Errorf("commerce state player=%d rule=%s: %w", row.PlayerID, ruleID, err)
		}
		if len(progress.Offers) > 0 || len(progress.Slots) > 0 || progress.Version < 1 || progress.ResourceUsed < 0 || progress.FreeLeft < 0 {
			return constUtil.ErrAdminConfiguration
		}
		for _, counts := range []map[string]int32{progress.Bought, progress.Total} {
			for _, n := range counts {
				if n < 0 {
					return constUtil.ErrAdminConfiguration
				}
			}
		}
	}
	var stock []db.P4CommerceStock
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("rule_id=?", ruleID).Order("product_id").Limit(10001).Find(&stock).Error; err != nil {
		return err
	}
	if len(stock) > 10000 {
		return constUtil.ErrAdminConfiguration
	}
	for _, row := range stock {
		if row.Used < 0 {
			return constUtil.ErrAdminConfiguration
		}
	}
	if old == next {
		return nil
	}
	return tx.Model(&live).Updates(updates).Error
}
