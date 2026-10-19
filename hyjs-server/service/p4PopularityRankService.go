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
	if cfg.ShopType != 4 || (!strings.HasPrefix(cfg.ID, constUtil.P4PopularityRankRulePrefix) && !strings.HasPrefix(cfg.ID, "shop_schedule_popularity-rank_")) || cfg.Source != constUtil.P4PopularityRankSource || rules.RankPublishedAt <= 0 || rules.RankPublishedAt >= cfg.StartAt || len(rules.Products) > 20 {
		return errors.New(constUtil.P4PopularityRankDenied)
	}
	for _, product := range rules.Products {
		if !strings.HasPrefix(product.ID, constUtil.P4PopularityRankProductPrefix) || product.MaxRank < 1 || product.MaxRank > 1000000 || product.DailyLimit != 1 || product.TotalLimit != 1 || product.Stock != -1 || product.TierID || product.DiscountBps != 10000 || product.MinVIP != 0 {
			return errors.New(constUtil.P4PopularityRankDenied)
		}
		if _, err := s.p4ParseCost(product.Cost); err != nil {
			return err
		}
	}
	return nil
}

// ArchiveP4PopularityRankOffer runs before changing an operational catalogue.
// Exact old bodies and old Frozen bytes remain independently addressable by
// claim.OfferHash; no claim hash, earned rank or quota is rewritten.
func (s *GameService) ArchiveP4PopularityRankOffer(tx *gorm.DB, live *db.P4CommerceRule) error {
	if live == nil || live.Kind != constUtil.P4PopularityRankKind {
		return nil
	}
	candidates := [][]byte{}
	candidates = append(candidates, adminJSON(*live))
	if len(live.Frozen) > 0 {
		candidates = append(candidates, append([]byte{}, live.Frozen...))
		var old db.P4CommerceRule
		if json.Unmarshal(live.Frozen, &old) == nil && old.ID == live.ID && old.Kind == live.Kind {
			old.Enabled = live.Enabled
			candidates = append(candidates, adminJSON(old))
			old.Enabled = true
			candidates = append(candidates, adminJSON(old))
		}
	}
	for _, payload := range candidates {
		var cfg db.P4CommerceRule
		if json.Unmarshal(payload, &cfg) != nil || cfg.ID != live.ID || cfg.Kind != constUtil.P4PopularityRankKind {
			continue
		}
		row := db.P4PopularityRankOfferHistory{OfferHash: p6Digest(payload), RuleID: live.ID, Payload: payload}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// A historic claim keeps its own threshold and proof. Current best rank may
// have improved, so resolve the exact immutable settlement hash of the claim.
func (s *GameService) p4PopularityRankHistoricalClaim(tx *gorm.DB, claim db.P4PopularityRankClaim, current db.P4CommerceRule) (bool, error) {
	if claim.Rank < 1 || len(claim.RankProof) != 64 || len(claim.OfferHash) != 64 {
		return false, nil
	}
	var historic db.P4CommerceRule
	currentRaw := adminJSON(current)
	if p6Digest(currentRaw) == claim.OfferHash {
		historic = current
	} else {
		var archive db.P4PopularityRankOfferHistory
		err := tx.First(&archive, "offer_hash=? AND rule_id=?", claim.OfferHash, claim.RuleID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Recover only an exact retained legacy body. Do not repeatedly insert the
			// current quote on each list read when an unrelated old proof is absent.
			candidate := append([]byte{}, current.Frozen...)
			if p6Digest(candidate) != claim.OfferHash {
				var previous db.P4CommerceRule
				if json.Unmarshal(candidate, &previous) != nil {
					return false, nil
				}
				previous.Enabled = true
				candidate = adminJSON(previous)
			}
			if p6Digest(candidate) != claim.OfferHash {
				return false, nil
			}
			recovered := db.P4PopularityRankOfferHistory{OfferHash: claim.OfferHash, RuleID: claim.RuleID, Payload: candidate}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&recovered).Error; err != nil {
				return false, err
			}
			err = tx.First(&archive, "offer_hash=? AND rule_id=?", claim.OfferHash, claim.RuleID).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if p6Digest(archive.Payload) != claim.OfferHash || json.Unmarshal(archive.Payload, &historic) != nil {
			return false, nil
		}
	}
	if historic.ID != claim.RuleID || historic.Source != claim.Source || historic.Kind != constUtil.P4PopularityRankKind {
		return false, nil
	}
	var rules p4CommerceRules
	if json.Unmarshal(historic.Rules, &rules) != nil || s.p4PopularityRankRules(historic, rules) != nil {
		return false, nil
	}
	allowed := false
	for _, product := range rules.Products {
		if product.ID == claim.ProductID && claim.Rank <= product.MaxRank {
			allowed = true
			break
		}
	}
	if !allowed || !tx.Migrator().HasTable(&db.P11ArenaFightSettlement{}) {
		return false, nil
	}
	var settlements []db.P11ArenaFightSettlement
	if err := tx.Where("winner_id=? AND ((player_id=? AND attacker_new_rank=?) OR (opponent_id=? AND defender_new_rank=?))", claim.PlayerID, claim.PlayerID, claim.Rank, claim.PlayerID, claim.Rank).Find(&settlements).Error; err != nil {
		return false, err
	}
	for _, settled := range settlements {
		proof := p6Digest([]byte(settled.P6RecordID + ":" + settled.ResponseHash + ":" + settled.TerminalHash + ":" + settled.ScopeProof + ":" + settled.PolicyHash))
		if proof != claim.RankProof {
			continue
		}
		if _, err := p11ReadArenaFightReceipt(tx, &settled); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *GameService) p4PopularityRankResolve(tx *gorm.DB, productID string, now time.Time) (string, error) {
	scheduled, managed, err := s.AdminScheduledOrdinaryShopConsumer(tx, "popularity-rank", now.UnixMilli())
	if err != nil {
		return "", err
	}
	if managed && scheduled == "" {
		return "", errors.New(constUtil.P4PopularityRankDenied)
	}
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind=? AND enabled=?", constUtil.P4PopularityRankKind, true)
	if managed {
		query = query.Where("id=?", scheduled)
	} else {
		query = query.Where("id NOT LIKE ?", "shop_schedule_%")
	}
	var rows []db.P4CommerceRule
	if err := query.Order("id").Find(&rows).Error; err != nil {
		return "", err
	}
	key := ""
	for _, live := range rows {
		cfg, rules, err := s.p4CommerceSnapshot(live)
		if err != nil {
			return "", err
		}
		open, err := p4ShopSourceWindow(tx, cfg, now)
		if err != nil {
			return "", err
		}
		if !open {
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
	if err = s.ArchiveP4PopularityRankOffer(tx, &cfg); err != nil {
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
	scheduled, managed, err := s.AdminScheduledOrdinaryShopConsumer(tx, "popularity-rank", now.UnixMilli())
	if err != nil {
		return nil, err
	}
	if managed && scheduled == "" {
		return items, nil
	}
	query := tx.Where("kind=? AND enabled=?", constUtil.P4PopularityRankKind, true)
	if managed {
		query = query.Where("id=?", scheduled)
	} else {
		query = query.Where("id NOT LIKE ?", "shop_schedule_%")
	}
	var rows []db.P4CommerceRule
	if err := query.Order("id").Find(&rows).Error; err != nil {
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
		open, err := p4ShopSourceWindow(tx, frozen, now)
		if err != nil {
			return nil, err
		}
		if !open {
			continue
		}
		frozen, rules, row, state, err := s.loadP4Commerce(tx, id, live.ID, 0, now)
		if err != nil {
			return nil, err
		}
		// A real earlier exchange can resume the later tutorial without granting or
		// charging anything again. Missing proof keeps the used quota closed.
		for _, product := range rules.Products {
			if state.Total[product.ID] == 0 {
				continue
			}
			var claim db.P4PopularityRankClaim
			if err = tx.Where("player_id=? AND rule_id=? AND product_id=?", id, frozen.ID, product.ID).First(&claim).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				continue // Used quota stays closed; absent history cannot grant anything.
			} else if err != nil {
				return nil, err
			}
			if state.Total[product.ID] != 1 {
				return nil, errors.New("排名兑换实际资格存档损坏")
			}
			valid, e := s.p4PopularityRankHistoricalClaim(tx, claim, frozen)
			if e != nil {
				return nil, e
			}
			if !valid {
				continue
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
