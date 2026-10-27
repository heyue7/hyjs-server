package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func commerceCalendarRestoreFixture(t *testing.T, frozen bool, kind string) (*GameService, db.P4CommerceRule, CommerceCalendarWindow, CommerceCalendarWindow) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "commerce-calendar.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool, _ := db.GetIns().DB(); pool.Close() })
	if err := db.GetIns().AutoMigrate(&db.P4CommerceRule{}, &db.P4CommerceState{}, &db.P4CommerceStock{}); err != nil {
		t.Fatal(err)
	}
	s := &GameService{}
	old := CommerceCalendarWindow{1000, 2000, 3000}
	next := CommerceCalendarWindow{4000, 5000, 6000}
	rules := p4CommerceRules{Products: []p4Product{p4BaseProduct("product", "coin,7", "diamond,3", 1, 1)}}
	rules.LiveEditable = kind == "point" || kind == "guild"
	row := db.P4CommerceRule{ID: "finite-exchange", Kind: kind, Enabled: true, StartAt: old.StartAt, EndAt: old.EndAt, ClaimEndAt: old.ClaimEndAt, Rules: adminJSON(rules), Source: "reviewed fixture"}
	if kind == "guild" {
		row.Rules = adminJSON(p4GuildRules{LiveEditable: true, Products: []p4GuildProduct{{p4Product: rules.Products[0], ContributionCost: 7, GuildLevel: 3}}})
	}
	if kind == "point" || kind == "guild" {
		row.ShopType = 5
		if kind == "guild" {
			row.ShopType = 8
		}
		row.ID = "shop_schedule_" + kind + "_finite"
		source := row
		source.ID = "p4:" + kind + ":source"
		if err := db.GetIns().Create(&source).Error; err != nil {
			t.Fatal(err)
		}
	}
	if frozen {
		row.Frozen = adminJSON(row)
	}
	state := db.P4CommerceState{PlayerID: 42, RuleID: row.ID, Payload: adminJSON(p4CommerceProgress{Day: "2026-10-27", Bought: map[string]int32{"product": 2}, Total: map[string]int32{"product": 4}, Version: 9, FreeAt: 999, DiscountBps: 8000})}
	stock := db.P4CommerceStock{RuleID: row.ID, ProductID: "product", Used: 3}
	for _, entity := range []any{&row, &state, &stock} {
		if err := db.GetIns().Create(entity).Error; err != nil {
			t.Fatal(err)
		}
	}
	return s, row, old, next
}

func TestCommerceCalendarRestorePreservesStateStockAndQuotes(t *testing.T) {
	for _, kind := range []string{"exchange", "point", "guild"} {
		for _, freeze := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/live", true: "/frozen"}[freeze], func(t *testing.T) {
				s, before, old, next := commerceCalendarRestoreFixture(t, freeze, kind)
				var states []db.P4CommerceState
				var stock []db.P4CommerceStock
				if err := db.GetIns().Find(&states).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().Find(&stock).Error; err != nil {
					t.Fatal(err)
				}
				cfg, rules, err := s.p4CommerceSnapshot(before)
				if err != nil {
					t.Fatal(err)
				}
				quote := adminLiveP4SKU(cfg, rules.Products[0], nil)
				if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return s.RestoreCommerceCalendarWindow(tx, before.ID, old, next) }); err != nil {
					t.Fatal(err)
				}
				var after db.P4CommerceRule
				if err := db.GetIns().First(&after, "id=?", before.ID).Error; err != nil {
					t.Fatal(err)
				}
				if commerceRestoreWindow(after) != next || !bytes.Equal(before.Rules, after.Rules) || before.Source != after.Source || before.Enabled != after.Enabled {
					t.Fatal("live terms changed", after)
				}
				if freeze {
					var frozenBefore, frozenAfter map[string]json.RawMessage
					if err := json.Unmarshal(before.Frozen, &frozenBefore); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(after.Frozen, &frozenAfter); err != nil {
						t.Fatal(err)
					}
					for _, key := range []string{"startAt", "endAt", "claimEndAt"} {
						delete(frozenBefore, key)
						delete(frozenAfter, key)
					}
					if !reflect.DeepEqual(frozenBefore, frozenAfter) {
						t.Fatal("frozen non-time fields changed")
					}
				}
				var afterStates []db.P4CommerceState
				var afterStock []db.P4CommerceStock
				if err := db.GetIns().Find(&afterStates).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().Find(&afterStock).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(states, afterStates) || !reflect.DeepEqual(stock, afterStock) {
					t.Fatal("state or stock changed")
				}
				cfg, rules, err = s.p4CommerceSnapshot(after)
				if err != nil || adminLiveP4SKU(cfg, rules.Products[0], nil) != quote || commerceRestoreWindow(cfg) != next {
					t.Fatal("snapshot or existing quote changed", err)
				}
			})
		}
	}
}

func TestCommerceCalendarRestoreRejectsUnknownAndRollsBack(t *testing.T) {
	for _, scenario := range []string{"old-conflict", "frozen-conflict", "unknown-frozen", "unknown-progress", "ggo-quote", "memory-offer", "caller-rollback", "no-transaction"} {
		t.Run(scenario, func(t *testing.T) {
			s, before, old, next := commerceCalendarRestoreFixture(t, true, "exchange")
			var err error
			switch scenario {
			case "old-conflict":
				old.EndAt++
			case "frozen-conflict":
				frozen := before
				frozen.Frozen = nil
				frozen.EndAt++
				err = db.GetIns().Model(&before).Update("frozen", adminJSON(frozen)).Error
			case "unknown-frozen":
				var fields map[string]any
				json.Unmarshal(before.Frozen, &fields)
				fields["unknown"] = true
				err = db.GetIns().Model(&before).Update("frozen", adminJSON(fields)).Error
			case "unknown-progress":
				err = db.GetIns().Model(&db.P4CommerceState{}).Where("rule_id=?", before.ID).Update("payload", []byte(`{"version":9,"unknownWindow":2000}`)).Error
			case "ggo-quote":
				err = db.GetIns().Create(&db.P4CommerceState{PlayerID: 42, RuleID: ggoQuoteRuleID(before.ID), Payload: adminJSON(ggoQuoteState{Generation: 1})}).Error
			case "memory-offer":
				err = db.GetIns().Model(&db.P4CommerceState{}).Where("rule_id=?", before.ID).Update("payload", adminJSON(p4CommerceProgress{Version: 9, Offers: []memoryOffer55{{ID: "quote", ProductID: "product"}}})).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			var preimage db.P4CommerceRule
			if err = db.GetIns().First(&preimage, "id=?", before.ID).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "no-transaction" {
				err = s.RestoreCommerceCalendarWindow(db.GetIns(), before.ID, old, next)
			} else {
				err = db.GetIns().Transaction(func(tx *gorm.DB) error {
					e := s.RestoreCommerceCalendarWindow(tx, before.ID, old, next)
					if scenario == "caller-rollback" && e == nil {
						return errors.New("caller subsequent failure")
					}
					return e
				})
			}
			if err == nil {
				t.Fatal("expected refusal or caller rollback")
			}
			var after db.P4CommerceRule
			if err = db.GetIns().First(&after, "id=?", before.ID).Error; err != nil || !reflect.DeepEqual(preimage, after) {
				t.Fatal("failed migration changed rule", err)
			}
		})
	}
}

func TestCommerceCalendarRestoreGuildRetainsDualCostAndRejectsRepricing(t *testing.T) {
	for _, scenario := range []string{"success", "negative-contribution", "invalid-guild-level", "tier-pricing", "discount-repricing", "unknown-product", "refresh-rules", "quantity-repricing", "frozen-invalid", "source-invalid"} {
		t.Run(scenario, func(t *testing.T) {
			s, before, old, next := commerceCalendarRestoreFixture(t, true, "guild")
			var rules p4GuildRules
			if err := json.Unmarshal(before.Rules, &rules); err != nil {
				t.Fatal(err)
			}
			changed := rules
			switch scenario {
			case "negative-contribution", "frozen-invalid", "source-invalid":
				changed.Products[0].ContributionCost = -1
			case "invalid-guild-level":
				changed.Products[0].GuildLevel = 0
			case "tier-pricing":
				changed.Products[0].TierID = true
			case "discount-repricing":
				changed.Products[0].DiscountBps = 9000
			}
			raw := adminJSON(changed)
			if scenario == "unknown-product" || scenario == "refresh-rules" || scenario == "quantity-repricing" {
				var fields map[string]any
				json.Unmarshal(raw, &fields)
				if scenario == "unknown-product" {
					fields["products"].([]any)[0].(map[string]any)["quoteExpiresAt"] = 99
				} else if scenario == "quantity-repricing" {
					fields["products"].([]any)[0].(map[string]any)["quantityTiers"] = []map[string]any{{}}
				} else {
					fields["refreshEnabled"] = true
				}
				raw = adminJSON(fields)
			}
			switch scenario {
			case "success":
			case "frozen-invalid":
				frozen := before
				frozen.Frozen = nil
				frozen.Rules = raw
				if err := db.GetIns().Model(&before).Update("frozen", adminJSON(frozen)).Error; err != nil {
					t.Fatal(err)
				}
			case "source-invalid":
				if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", "p4:guild:source").Update("rules", raw).Error; err != nil {
					t.Fatal(err)
				}
			default:
				if err := db.GetIns().Model(&before).Update("rules", raw).Error; err != nil {
					t.Fatal(err)
				}
			}
			var preimage db.P4CommerceRule
			db.GetIns().First(&preimage, "id=?", before.ID)
			err := db.GetIns().Transaction(func(tx *gorm.DB) error { return s.RestoreCommerceCalendarWindow(tx, before.ID, old, next) })
			var after db.P4CommerceRule
			db.GetIns().First(&after, "id=?", before.ID)
			if scenario != "success" {
				if err == nil || !reflect.DeepEqual(preimage, after) {
					t.Fatal("invalid guild contract accepted or wrote dates", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			live, err := s.p4GuildShopRules(after)
			if err != nil || live.Products[0].ContributionCost != 7 || live.Products[0].GuildLevel != 3 || !bytes.Equal(before.Rules, after.Rules) {
				t.Fatal("guild terms changed", err)
			}
			var frozen db.P4CommerceRule
			if err := json.Unmarshal(after.Frozen, &frozen); err != nil {
				t.Fatal(err)
			}
			frozenRules, err := s.p4GuildShopRules(frozen)
			if err != nil || !reflect.DeepEqual(live, frozenRules) || commerceRestoreWindow(frozen) != next {
				t.Fatal("guild frozen dual-cost contract lost", err)
			}
		})
	}
}
