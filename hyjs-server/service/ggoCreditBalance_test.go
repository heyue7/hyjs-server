package service

import (
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
	"sync"
	"testing"
)

func TestGGOCreditMigrationRollbackAndLargeBalance(t *testing.T) {
	f, id, ctx, _ := terminalFixture(t)
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	var original db.Profile
	if err := db.GetIns().First(&original, id).Error; err != nil {
		t.Fatal(err)
	}
	var old model.GameState
	if err := json.Unmarshal([]byte(*original.Game), &old); err != nil {
		t.Fatal(err)
	}
	old.Items = append(old.Items, &pb.SimpleItem{Id: proto.String(constUtil.GGOCurrencyID), Num: proto.Int32(25)})
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"game": string(raw), "ggo_credit": int64(0), "ggo_credit_migrated": false}).Error; err != nil {
		t.Fatal(err)
	}
	forced := errors.New("rollback")
	migrate := func(abort bool) error {
		return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
				return err
			}
			p, g, err := decodeProfile(&row)
			if err != nil {
				return err
			}
			if ggoCreditBalance(g) != 25 {
				t.Fatalf("legacy balance: %d", ggoCreditBalance(g))
			}
			if err = saveProfile(tx, &row, p, g); err != nil {
				return err
			}
			if abort {
				return forced
			}
			return nil
		})
	}
	if err = migrate(true); !errors.Is(err, forced) {
		t.Fatal(err)
	}
	var row db.Profile
	if err = db.GetIns().First(&row, id).Error; err != nil || row.GGOCreditMigrated || *row.Game != string(raw) {
		t.Fatal("rollback altered source", err)
	}
	if err = migrate(false); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&row, id).Error; err != nil || !row.GGOCreditMigrated || row.GGOCredit != 25 {
		t.Fatal("migration", row.GGOCredit, err)
	}
	if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		if ggoCreditBalance(g) != 25 {
			return errors.New("migration applied twice")
		}
		if err := ggoCreditAdd(g, math.MaxInt32); err != nil {
			return err
		}
		// Unrelated reward JSON cloning must retain the projected int64 balance.
		if _, err := f.s.GrantFixedReward(p, g, "coin,1"); err != nil {
			return err
		}
		if simpleOwned(p, g, constUtil.GGOCurrencyID) != int64(math.MaxInt32)+25 {
			return errors.New("reward clone lost balance")
		}
		return ggoCreditDeduct(g, 10)
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&row, id).Error; err != nil || row.GGOCredit != int64(math.MaxInt32)+15 {
		t.Fatal("large balance", row.GGOCredit, err)
	}
	_, saved, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range saved.Items {
		if item.GetId() == constUtil.GGOCurrencyID {
			t.Fatal("legacy credit survived migration")
		}
	}
	// Concurrent writes use the same transaction and row-lock path as purchases.
	var workers sync.WaitGroup
	failures := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for n := 0; n < 4; n++ {
				if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
					if err := ggoCreditAdd(g, 3); err != nil {
						return err
					}
					return ggoCreditDeduct(g, 1)
				}); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	f.restart(t)
	if err = db.GetIns().First(&row, id).Error; err != nil || row.GGOCredit != int64(math.MaxInt32)+79 {
		t.Fatal("concurrent balance or cold recovery", row.GGOCredit, err)
	}
	saved.GGOCredit = math.MaxInt64
	if err = ggoCreditAdd(saved, 1); err == nil || saved.GGOCredit != math.MaxInt64 {
		t.Fatal("overflow accepted or balance changed")
	}
	if err = ggoCreditDeduct(saved, -1); err == nil {
		t.Fatal("negative debit accepted")
	}
}

func TestGGOPriceIncreaseAndFrozenGeneration(t *testing.T) {
	cfg := model.AdminFieldGGOShop{PriceNumerator: 7, PriceDenominator: 5, RoundTo: 5, RefreshCost: 15}
	for _, pair := range [][2]int64{{20, 30}, {30, 45}, {40, 60}, {80, 115}, {90, 130}, {350, 490}, {600, 840}, {1200, 1680}} {
		got, err := ggoRaisedPrice(pair[0], cfg)
		if err != nil || got != pair[1] {
			t.Fatal(pair, got, err)
		}
	}
	bad := cfg
	bad.PriceNumerator = 1
	if _, err := ggoRaisedPrice(100, bad); err == nil {
		t.Fatal("decrease accepted")
	}
	s := &GameService{GGOFieldConfig: &model.AdminFieldGGOConfiguration{Runtime: &model.AdminFieldGGORuntime{Enabled: true, Shop: cfg}}}
	p := p4BaseProduct("source", "item_021,1", constUtil.GGOCurrencyID+",80", 1, 1)
	p.Weight = 10000
	rules := p4CommerceRules{Products: []p4Product{p}, RefreshCost: constUtil.GGOCurrencyID + ",10"}
	state := &ggoQuoteState{Day: "2026-10-28"}
	if err := s.ggoDrawRuntimeQuote(nil, rules, state); err != nil {
		t.Fatal(err)
	}
	for _, slot := range state.Slots {
		if slot.Product.Cost != constUtil.GGOCurrencyID+",115" || slot.SourceProductID != "source" {
			t.Fatal("price or SKU", slot)
		}
	}
	if state.RefreshCost != constUtil.GGOCurrencyID+",15" {
		t.Fatal(state.RefreshCost)
	}
	s.GGOFieldConfig.Runtime.Shop.PriceNumerator = 2
	s.GGOFieldConfig.Runtime.Shop.PriceDenominator = 1
	if state.Slots[0].Product.Cost != constUtil.GGOCurrencyID+",115" {
		t.Fatal("accepted price changed")
	}
	if err := s.ggoDrawRuntimeQuote(nil, rules, state); err != nil {
		t.Fatal(err)
	}
	if state.Slots[0].Product.Cost != constUtil.GGOCurrencyID+",160" {
		t.Fatal("price compounded", state.Slots[0].Product.Cost)
	}
}
