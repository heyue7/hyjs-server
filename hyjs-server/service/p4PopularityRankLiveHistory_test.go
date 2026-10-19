package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This is a declared pre-upgrade used quota, not a fabricated arena win or new
// award. It must remain spent even when its historic proof cannot be recovered.
func TestPopularityLiveHistoricalClaimArchiveMissingProofAndGenericCost(t *testing.T) {
	f, operator := adminModesFixture(t)
	p4Fund(t, f)
	now := time.Now()
	key := p4PopularityPublish(t, f, now)
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "popularity").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.NoviceGuideSchedule = proto.Int32(13); return nil }); err != nil {
		t.Fatal(err)
	}
	query := &pb.CSShopProductList{ShopType: proto.Int32(4)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, query, 0, now); err != nil {
		t.Fatal(err)
	}
	var historic db.P4CommerceRule
	var rules p4CommerceRules
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		cfg, r, row, state, err := f.s.loadP4Commerce(tx, f.id, key, 0, now)
		if err != nil {
			return err
		}
		historic = cfg
		rules = r
		state.Total[r.Products[0].ID] = 1
		state.Bought[p4SlotKey(cfg, state, r.Products[0])] = 1
		return saveP4Commerce(tx, row, state)
	}); err != nil {
		t.Fatal(err)
	}
	claim := db.P4PopularityRankClaim{PlayerID: f.id, RuleID: key, ProductID: rules.Products[0].ID, Rank: 500, RankProof: strings.Repeat("a", 64), OfferHash: p6Digest(adminJSON(historic)), Source: historic.Source, Stamp: 74001, ClaimedAt: now.UnixMilli()}
	if err := db.GetIns().Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	original := string(adminJSON(claim))
	page, err := f.s.AdminShop(f.ctx, "popularity-rank")
	if err != nil {
		t.Fatal(err)
	}
	product := page.Items[0]
	product.MaxRank = 1
	product.Cost = model.AdminResourceAmount{ResourceID: "frag_hero_001", Amount: 3}
	product.Rewards = []model.AdminResourceAmount{{ResourceID: "item_001", Amount: 2}}
	if _, err = f.s.AdminSaveShopProduct(f.ctx, operator, "popularity-rank", product.ID, model.AdminShopProductSaveRequest{AdminShopProduct: product, Version: page.Version}, false); err != nil {
		t.Fatal("real current rank edit/generic material", err)
	}
	var archived db.P4PopularityRankOfferHistory
	if err = db.GetIns().First(&archived, "offer_hash=?", claim.OfferHash).Error; err != nil || string(archived.Payload) != string(adminJSON(historic)) {
		t.Fatal("pre-edit exact body not archived", err)
	}
	var old db.P4CommerceRule
	var oldRules p4CommerceRules
	if json.Unmarshal(archived.Payload, &old) != nil || json.Unmarshal(old.Rules, &oldRules) != nil || oldRules.Products[0].MaxRank != 1000 || oldRules.Products[0].Cost != "popularity,20" {
		t.Fatal("old threshold/price replaced")
	}
	list := func() *pb.ShopItem {
		t.Helper()
		messages, e := f.s.p4Commerce(f.ctx, f.id, query, 0, now)
		if e != nil {
			t.Fatal("history blocked whole original list", e)
		}
		items := messages[0].(*pb.SCShopProductList).GetShopItemList()
		if len(items) < 2 {
			t.Fatal("ordinary items disappeared")
		}
		for _, item := range items {
			if item.GetCondition().GetType() == "rank" {
				return item
			}
		}
		t.Fatal("claimed rank offer missing")
		return nil
	}
	current := list()
	if current.GetCanBuy() != 2 || current.GetPrice()[0].GetId() != "frag_hero_001" || current.GetPrice()[0].GetNum() != 3 {
		t.Fatal("live price or preserved quota", current)
	}
	p, g := f.profile(t)
	if p.GetNoviceGuideSchedule() != 13 {
		t.Fatal("missing real arena proof advanced guide")
	}
	before := simpleOwned(p, g, "item_001")
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(4), ShopItemId: current.Id, BuyCount: proto.Int32(1)}, 74002, now); err == nil {
		t.Fatal("old used exchange granted again")
	}
	// Model an upgrade whose old catalogue body is unavailable. The used quota
	// remains authoritative for no-new-grant, without pretending proof is valid.
	if err = db.GetIns().Where("offer_hash=?", claim.OfferHash).Delete(&db.P4PopularityRankOfferHistory{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Update("frozen", nil).Error; err != nil {
		t.Fatal(err)
	}
	if list().GetCanBuy() != 2 {
		t.Fatal("missing history reset one-time quota")
	}
	f.restart(t)
	if list().GetCanBuy() != 2 {
		t.Fatal("cold used quota lost")
	}
	var preserved db.P4PopularityRankClaim
	if err = db.GetIns().First(&preserved, "player_id=? AND rule_id=? AND product_id=?", f.id, key, claim.ProductID).Error; err != nil || string(adminJSON(preserved)) != original {
		t.Fatal("claim hash/proof rewritten", err)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_001") != before {
		t.Fatal("history recovery silently granted item")
	}
	// Even a missing claim row must not turn the entire normal shop into an error;
	// its independently retained Total=1 continues to prevent a new award.
	if err = db.GetIns().Where("player_id=? AND rule_id=? AND product_id=?", f.id, key, claim.ProductID).Delete(&db.P4PopularityRankClaim{}).Error; err != nil {
		t.Fatal(err)
	}
	if list().GetCanBuy() != 2 {
		t.Fatal("missing claim reopened consumed exchange")
	}
}
