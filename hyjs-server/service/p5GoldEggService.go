package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"time"
)

type p5GoldEggOption struct {
	ID     string `json:"id"`
	Reward string `json:"reward"`
	Weight int64  `json:"weight"`
	Param  string `json:"originalParam"`
}

func p5GoldEggOriginalReward(row map[string]string) string {
	if row["id"] == "881" && row["itemId"] == "item_360" && row["result"] == "MineItem_028，1" {
		return "MineItem_028,1"
	}
	return row["result"]
}
func (s *GameService) seedP5GoldEgg(tx *gorm.DB) error {
	var count int64
	if e := tx.Model(&db.P5GoldEggRule{}).Where("id=?", constUtil.P5GoldEggRuleID).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	var options []p5GoldEggOption
	for _, row := range s.Tables["ItemUse"] {
		if row["itemId"] != "item_360" {
			continue
		}
		weight, e := strconv.ParseInt(row["odds"], 10, 64)
		if e != nil {
			return e
		}
		options = append(options, p5GoldEggOption{ID: row["id"], Reward: p5GoldEggOriginalReward(row), Weight: weight, Param: row["param"]})
	}
	if e := s.validateP5GoldEgg(options); e != nil {
		return e
	}
	raw, _ := json.Marshal(options)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P5GoldEggRule{ID: constUtil.P5GoldEggRuleID, Enabled: true, Rules: raw, InputHash: "", Source: constUtil.P5GoldEggSource}).Error
}
func (s *GameService) validateP5GoldEgg(options []p5GoldEggOption) error {
	original := map[string]map[string]string{}
	for _, row := range s.Tables["ItemUse"] {
		if row["itemId"] == "item_360" {
			original[row["id"]] = row
		}
	}
	if len(original) != 54 || len(options) != len(original) {
		return errors.New("金蛋原奖项目录不完整")
	}
	seen := map[string]bool{}
	var total int64
	for _, option := range options {
		row := original[option.ID]
		if row == nil || seen[option.ID] || option.Weight < 1 || option.Weight > 1000000000000 || option.Reward != p5GoldEggOriginalReward(row) || option.Param != row["param"] {
			return errors.New("金蛋原奖励或权重无效")
		}
		seen[option.ID] = true
		total += option.Weight
		supported, e := s.openServerRewardSupported(option.Reward)
		if e != nil || !supported {
			return errors.New("金蛋奖励未恢复")
		}
	}
	if total <= 0 {
		return errors.New("金蛋总权重无效")
	}
	return nil
}
func (s *GameService) p5GoldEggOptions(tx *gorm.DB) ([]p5GoldEggOption, error) {
	if e := s.seedP5GoldEgg(tx); e != nil {
		return nil, e
	}
	var cfg db.P5GoldEggRule
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, "id=?", constUtil.P5GoldEggRuleID).Error; e != nil {
		return nil, e
	}
	if !cfg.Enabled {
		return nil, errors.New("失落金蛋暂未开放")
	}
	raw := cfg.Rules
	if len(cfg.Frozen) > 0 {
		hash := sha256.Sum256(cfg.Frozen)
		if cfg.InputHash != hex.EncodeToString(hash[:]) {
			return nil, errors.New("金蛋冻结规则损坏")
		}
		raw = cfg.Frozen
	}
	var options []p5GoldEggOption
	if json.Unmarshal(raw, &options) != nil {
		return nil, errors.New("金蛋规则损坏")
	}
	if e := s.validateP5GoldEgg(options); e != nil {
		return nil, e
	}
	if len(cfg.Frozen) == 0 {
		hash := sha256.Sum256(raw)
		cfg.Frozen = append([]byte(nil), raw...)
		cfg.InputHash = hex.EncodeToString(hash[:])
		if e := tx.Save(&cfg).Error; e != nil {
			return nil, e
		}
	}
	return options, nil
}

// p5GoldEggUse preserves the original one-item direct bag action and atomic receipt.
func (s *GameService) p5GoldEggUse(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, error) {
	out := &pb.SCItemUse{}
	return s.p5Mutation(ctx, id, "item-gold-egg", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		cost := b.GetCostItem()
		if cost.GetId() != "item_360" || cost.GetGuid() != "" || cost.GetNum() != 1 || b.GetGetItemIndex() != 0 {
			return p5Error()
		}
		options, e := s.p5GoldEggOptions(tx)
		if e != nil {
			return e
		}
		weights := make([]db.GameplayPoolReward, len(options))
		for i, option := range options {
			weights[i] = db.GameplayPoolReward{ID: int32(i), Weight: option.Weight, Amount: 1}
		}
		chosen, e := choosePoolReward(weights, false)
		if e != nil {
			return e
		}
		if e = deductPoolCost(p, g, cost); e != nil {
			return e
		}
		gain, e := s.GrantFixedReward(p, g, options[chosen.ID].Reward)
		if e != nil {
			return e
		}
		out.Cost = proto.Clone(cost).(*pb.BaseResource)
		out.Get = gain
		return nil
	})
}

// seedP5GoldEggSupply uses a separate child rule; existing books/gems never gain new limits.
func (s *GameService) seedP5GoldEggSupply(tx *gorm.DB, now time.Time) error {
	if !gameSchemaHasTable(tx, &db.P5GoldEggRule{}) {
		return nil
	}
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5GoldEggSupplyPrefix + start.Format("200601")
	var count int64
	if e := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	var maxCoin int64
	for _, row := range s.Tables["ItemUse"] {
		if row["itemId"] == "item_360" {
			resources, e := p1RewardResources(p5GoldEggOriginalReward(row))
			if e != nil {
				return e
			}
			for _, r := range resources {
				if r.GetId() == "coin" {
					maxCoin = max(maxCoin, int64(r.GetNum()))
				}
			}
		}
	}
	if maxCoin != 2000000 {
		return errors.New("金蛋金币价格锚点未恢复")
	}
	product := p4BaseProduct("item_360", "item_360,1", "coin,"+strconv.FormatInt(maxCoin*2, 10), 1, 85)
	product.Name = s.table("Item", "itemId", "item_360")["name"]
	product.DailyLimit = 1
	product.TotalLimit = 3
	raw, _ := json.Marshal(p4CommerceRules{Products: []p4Product{product}})
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5GoldEggSupplySource}
	if _, e := s.validateP4Commerce(cfg); e != nil {
		return e
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p5AppendGoldEggBinding appends a newly released monthly child after stable indices.
// Existing directory bindings and historical receipts are never renumbered.
func (s *GameService) p5AppendGoldEggBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	key := constUtil.P5GoldEggSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	for _, binding := range *bindings {
		if binding.ChildID == key {
			return nil
		}
	}
	var live db.P4CommerceRule
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", key).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	cfg, rules, e := s.p4CommerceSnapshot(live)
	if e != nil {
		return e
	}
	if cfg.Kind != "exchange" || len(rules.Products) != 1 || rules.Products[0].ID != "item_360" {
		return errors.New("金蛋供给目录无效")
	}
	if len(live.Frozen) == 0 {
		raw, _ := json.Marshal(cfg)
		if e = tx.Model(&live).Update("frozen", raw).Error; e != nil {
			return e
		}
	}
	var index int32
	for _, binding := range *bindings {
		index = max(index, binding.Index)
	}
	*bindings = append(*bindings, p4ExchangeBinding{Index: index + 1, ChildID: key, ChildIndex: rules.Products[0].Index, ProductID: "item_360"})
	directory.Bindings, _ = json.Marshal(*bindings)
	hash := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(hash[:])
	return tx.Save(directory).Error
}

func (s *GameService) p5GoldEggSupplyGuard(tx *gorm.DB) error {
	var cfg db.P5GoldEggRule
	if e := tx.First(&cfg, "id=?", constUtil.P5GoldEggRuleID).Error; e != nil {
		return e
	}
	if !cfg.Enabled {
		return errors.New("失落金蛋暂未开放")
	}
	return nil
}
