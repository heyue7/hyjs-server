package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// seedP4PopularityRank adds a disabled offer without replacing either ordinary
// popularity SKU or their existing per-player state. Operators publish its
// window and version before opening; the first list freezes all terms.
func (s *GameService) seedP4PopularityRank(tx *gorm.DB, now time.Time) error {
	product := p4BaseProduct("p4PopularityRank_local_v1_1", "item_001,1", "popularity,20", 1, 10)
	product.MaxRank, product.DailyLimit, product.TotalLimit = 1000, 1, 1
	rules := p4CommerceRules{Products: []p4Product{product}}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	row := db.P4CommerceRule{ID: constUtil.P4PopularityRankRulePrefix + "local-v1", Kind: constUtil.P4PopularityRankKind, ShopType: 4, Enabled: false, StartAt: 0, EndAt: 0, ClaimEndAt: 0, Rules: raw, Source: constUtil.P4PopularityRankSource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func (s *GameService) p4PopularityRankRules(cfg db.P4CommerceRule, rules p4CommerceRules) error {
	if cfg.Kind != constUtil.P4PopularityRankKind {
		for _, product := range rules.Products {
			if product.MaxRank != 0 {
				return errors.New(constUtil.P4PopularityRankDenied)
			}
		}
		return nil
	}
	if cfg.ShopType != 4 || !strings.HasPrefix(cfg.ID, constUtil.P4PopularityRankRulePrefix) || cfg.Source != constUtil.P4PopularityRankSource || rules.RankPublishedAt <= 0 || rules.RankPublishedAt >= cfg.StartAt || len(rules.Products) > 20 {
		return errors.New(constUtil.P4PopularityRankDenied)
	}
	for _, product := range rules.Products {
		if !strings.HasPrefix(product.ID, constUtil.P4PopularityRankProductPrefix) || product.MaxRank < 1 || product.MaxRank > 1000000 || !strings.HasPrefix(product.Cost, "popularity,") || product.DailyLimit != 1 || product.TotalLimit != 1 || product.Stock != -1 || product.TierID || product.DiscountBps != 10000 || product.MinVIP != 0 {
			return errors.New(constUtil.P4PopularityRankDenied)
		}
	}
	return nil
}

func (s *GameService) p4PopularityRankResolve(tx *gorm.DB, productID string, now time.Time) (string, error) {
	var rows []db.P4CommerceRule
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind=? AND enabled=?", constUtil.P4PopularityRankKind, true).Order("id").Find(&rows).Error; err != nil {
		return "", err
	}
	key := ""
	for _, live := range rows {
		cfg, rules, err := s.p4CommerceSnapshot(live)
		if err != nil {
			return "", err
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			continue
		}
		for _, product := range rules.Products {
			if product.ID == productID {
				if key != "" {
					return "", errors.New("排名商品对应多个开放实例")
				}
				key = cfg.ID
			}
		}
	}
	if key == "" {
		return "", errors.New(constUtil.P4PopularityRankDenied)
	}
	return key, nil
}

// p4PopularityRankBuy runs after the existing profile/offer locks and before
// any deduction. The immutable accepted P6 winner is the sole rank authority.
func (s *GameService) p4PopularityRankBuy(tx *gorm.DB, id int64, p *pb.Player, cfg db.P4CommerceRule, product p4Product, count int32, stamp int64, now time.Time) error {
	if cfg.Kind != constUtil.P4PopularityRankKind {
		return nil
	}
	rank, proof, err := P11ArenaVerifiedBestWonRank(tx, id)
	if err != nil {
		return err
	}
	if count != 1 || rank < 1 || rank > product.MaxRank || len(proof) != 64 {
		return errors.New(constUtil.P4PopularityRankDenied)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	claim := db.P4PopularityRankClaim{PlayerID: id, RuleID: cfg.ID, ProductID: product.ID, Rank: rank, RankProof: proof, OfferHash: p6Digest(raw), Source: cfg.Source, Stamp: stamp, ClaimedAt: now.UnixMilli()}
	if err = tx.Create(&claim).Error; err != nil {
		return err
	}
	// Only the actual prior tutorial cursor advances. Existing unrelated cursors
	// and ranking/gameplay prerequisites retain their original meaning.
	if p.GetNoviceGuideSchedule() == 13 {
		p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted)
	}
	return nil
}

// p4PopularityShopItems merges two original UI tabs while retaining each
// offer's existing state key. Ordinary purchases keep their old monthly quota.
func (s *GameService) p4PopularityShopItems(tx *gorm.DB, id int64, p *pb.Player, cfg db.P4CommerceRule, items []*pb.ShopItem, now time.Time) ([]*pb.ShopItem, error) {
	if cfg.Kind == constUtil.P4PopularityRankKind {
		normal, rules, _, state, err := s.loadP4Commerce(tx, id, "", 4, now)
		if err != nil && !errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
			return nil, err
		}
		if err == nil {
			items, _, err = s.p4Projection(tx, p, normal, rules, state)
			if err != nil {
				return nil, err
			}
		} else {
			items = nil
		}
	}
	var rows []db.P4CommerceRule
	if err := tx.Where("kind=? AND enabled=?", constUtil.P4PopularityRankKind, true).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.GetId()] = true
	}
	for _, live := range rows {
		frozen, rules, err := s.p4CommerceSnapshot(live)
		if err != nil {
			return nil, err
		}
		if now.UnixMilli() < frozen.StartAt || now.UnixMilli() >= frozen.EndAt {
			continue
		}
		frozen, rules, row, state, err := s.loadP4Commerce(tx, id, live.ID, 0, now)
		if err != nil {
			return nil, err
		}
		// A real earlier exchange can resume the later tutorial without granting or
		// charging anything again. A numeric quota without its witness is rejected.
		for _, product := range rules.Products {
			if state.Total[product.ID] == 0 {
				continue
			}
			var claim db.P4PopularityRankClaim
			if err = tx.Where("player_id=? AND rule_id=? AND product_id=?", id, frozen.ID, product.ID).First(&claim).Error; err != nil {
				return nil, err
			}
			raw, e := json.Marshal(frozen)
			if e != nil {
				return nil, e
			}
			if state.Total[product.ID] != 1 || claim.Rank < 1 || claim.Rank > product.MaxRank || len(claim.RankProof) != 64 || claim.OfferHash != p6Digest(raw) || claim.Source != frozen.Source {
				return nil, errors.New("排名兑换实际资格存档损坏")
			}
			if p.GetNoviceGuideSchedule() == 13 {
				p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted)
			}
		}
		projected, _, err := s.p4Projection(tx, p, frozen, rules, state)
		if err != nil {
			return nil, err
		}
		if err = saveP4Commerce(tx, row, state); err != nil {
			return nil, err
		}
		for _, item := range projected {
			if seen[item.GetId()] {
				return nil, errors.New("排名商品目录ID重复")
			}
			seen[item.GetId()] = true
			items = append(items, item)
		}
	}
	return items, nil
}

// An empty operator-owned reward tab has no original purchase target for Guide93.
// Complete only this teaching cursor after its real shop query; no product,
// currency, purchase witness or later level guide is created by this fallback.
func p4PopularityEmptyGuide(p *pb.Player, items []*pb.ShopItem) bool {
	if p.GetNoviceGuideSchedule() != constUtil.P4PopularityGuideCompleted-1 {
		return false
	}
	for _, item := range items {
		if item.GetCondition().GetType() == "rank" {
			return false
		}
	}
	p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted)
	return true
}
