package service

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func ggoBossDrawQuantity(options []model.AdminFieldQuantity) (int64, error) {
	parts := make([]string, 0, len(options))
	for _, q := range options {
		parts = append(parts, fmt.Sprintf("%d,%d", q.Amount, q.Weight))
	}
	value, err := fieldWeighted(strings.Join(parts, ";"))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func ggoBossAddContent(target map[string]int64, content string) error {
	if content == "" {
		return nil
	}
	for _, piece := range strings.Split(content, ";") {
		pair := strings.Split(piece, ",")
		if len(pair) != 2 {
			return errors.New("首领奖励格式无效")
		}
		n, err := strconv.ParseInt(pair[1], 10, 64)
		if err != nil || n < 0 || n > math.MaxInt32-target[pair[0]] {
			return errors.New("首领奖励数量溢出")
		}
		target[pair[0]] += n
	}
	return nil
}

// Copy is drawn once for the participation material package. Gold has its own
// boosted chance and is not copied; rank, tail, currency and chips are unchanged.
func ggoBossResourceReward(event *ggoBossEvent, sprouts, rank int64, tail bool) (string, error) {
	cfg := event.Boss
	sprouts = max(int64(0), min(sprouts, cfg.SproutMax))
	copyPackage := false
	if sprouts > 0 && cfg.SproutBonusBps > 0 {
		n, err := rand.Int(rand.Reader, big.NewInt(10000))
		if err != nil {
			return "", err
		}
		copyPackage = n.Int64() < min(int64(10000), sprouts*cfg.SproutBonusBps)
	}
	rewards := map[string]int64{}
	for _, drop := range cfg.ResourceDrops {
		bonus := int64(0)
		count := int64(0)
		if drop.ResourceID == "FishItem_025" {
			bonus = sprouts
		}
		content, err := bossDropContent73([]model.AdminFieldDrop{drop}, bonus, cfg.SproutBonusBps)
		if err != nil {
			return "", err
		}
		if err = ggoBossAddContent(rewards, content); err != nil {
			return "", err
		}
		if copyPackage && drop.ResourceID != "FishItem_025" && (strings.HasPrefix(drop.ResourceID, "MineItem_") || strings.HasPrefix(drop.ResourceID, "FishItem_")) {
			count = rewards[drop.ResourceID]
			if count > math.MaxInt32-rewards[drop.ResourceID] {
				return "", errors.New("首领复制奖励溢出")
			}
			rewards[drop.ResourceID] += count
		}
	}
	for _, band := range cfg.RankResourceDrops {
		if rank >= band.From && rank <= band.To {
			content, err := bossDropContent73(band.Drops, 0, 0)
			if err != nil {
				return "", err
			}
			if err = ggoBossAddContent(rewards, content); err != nil {
				return "", err
			}
		}
	}
	if tail {
		content, err := bossDropContent73(cfg.TailResourceDrops, 0, 0)
		if err != nil {
			return "", err
		}
		if err = ggoBossAddContent(rewards, content); err != nil {
			return "", err
		}
	}
	found := false
	for _, tier := range cfg.OverclockRewards {
		if tier.Sprouts != sprouts {
			continue
		}
		amount, err := ggoBossDrawQuantity(tier.Quantities)
		if err != nil {
			return "", err
		}
		if amount < 1 || amount > math.MaxInt32-rewards[constUtil.GGOOverclockID] {
			return "", errors.New("首领超频奖励无效")
		}
		rewards[constUtil.GGOOverclockID] += amount
		found = true
		break
	}
	if !found {
		return "", errors.New("首领小芽超频阶梯缺失")
	}
	keys := make([]string, 0, len(rewards))
	for key := range rewards {
		keys = append(keys, key)
	}
	// Stable order is part of the frozen receipt, independent of map iteration.
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if rewards[key] > 0 {
			parts = append(parts, fmt.Sprintf("%s,%d", key, rewards[key]))
		}
	}
	return strings.Join(parts, ";"), nil
}

func ggoBossChipIndex(id string) (int, error) {
	for i := 0; i < 6; i++ {
		if id == fmt.Sprintf("item_%d", 9701+i) {
			return i, nil
		}
	}
	return 0, errors.New("首领芯片品质无效")
}
func ggoBossChipReward(event *ggoBossEvent, damage float64, rank int64, tail bool) ([6]int64, error) {
	var chips [6]int64
	if !combatFinite(damage) || damage <= 0 {
		return chips, errors.New("首领有效伤害无效")
	}
	cfg := event.Boss
	add := func(id string, n int64) error {
		index, err := ggoBossChipIndex(id)
		if err != nil {
			return err
		}
		if n < 1 || n > math.MaxInt64-chips[index] {
			return errors.New("首领芯片数量溢出")
		}
		chips[index] += n
		return nil
	}
	amount := cfg.ParticipationAmount
	if damage >= float64(cfg.HP)*float64(cfg.ParticipationThresholdBps)/10000 {
		amount = cfg.ThresholdAmount
	}
	if err := add(cfg.ParticipationItemID, amount); err != nil {
		return chips, err
	}
	for _, band := range cfg.RankRewards {
		if rank >= band.From && rank <= band.To {
			if err := add(band.ItemID, band.Amount); err != nil {
				return chips, err
			}
		}
	}
	if tail {
		var total int64
		for _, option := range cfg.TailRewards {
			if option.Weight < 0 || option.Weight > math.MaxInt64-total {
				return chips, errors.New("首领尾刀权重无效")
			}
			total += option.Weight
		}
		if total < 1 {
			return chips, errors.New("首领尾刀权重为空")
		}
		n, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return chips, err
		}
		pick := n.Int64()
		for _, option := range cfg.TailRewards {
			if pick < option.Weight {
				if err = add(option.ItemID, 1); err != nil {
					return chips, err
				}
				break
			}
			pick -= option.Weight
		}
	}
	return chips, nil
}

// The shared HP row owns contribution order and death freezing. No recipient
// profile is locked here; each recipient later claims in its own transaction.
func (s *GameService) ggoBossFreezeAwardsLocked(tx *gorm.DB, event *ggoBossEvent, killerID int64) error {
	var rows []db.FieldBossContribution73
	if err := tx.Where("server_id=? AND spawn_uuid=? AND damage>0", s.OriginalServerID(), event.ID).Order("damage DESC, id ASC, player_id ASC").Find(&rows).Error; err != nil {
		return err
	}
	day := LocalPlayNow(tx.Statement.Context).Format("2006-01-02")
	for i := range rows {
		row := &rows[i]
		if row.Frozen {
			continue
		}
		rank := int64(i + 1)
		reward, err := ggoBossResourceReward(event, row.Sprouts, rank, row.PlayerID == killerID)
		if err != nil {
			return err
		}
		chips, err := ggoBossChipReward(event, row.Damage, rank, row.PlayerID == killerID)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(ggoBossAward{EconomyVersion: event.EconomyVersion, ExchangeValues: event.ExchangeValues, Chips: chips})
		if err != nil {
			return err
		}
		if err = tx.Model(row).Updates(map[string]any{"reward": reward, "reward_day": day, "reward_rank": rank, "frozen": true, "ggo_chips": raw, "ggo_chips_claimed": false}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ggoBossDeliverChipsLocked(tx *gorm.DB, id int64, g *model.GameState, row *db.FieldBossContribution73, eligible bool) error {
	if len(row.GGOChips) == 0 || row.GGOChipsClaimed {
		return nil
	}
	if !eligible {
		return tx.Model(row).Update("ggo_chips_claimed", true).Error
	}
	var award ggoBossAward
	if json.Unmarshal(row.GGOChips, &award) != nil || award.EconomyVersion == "" {
		return errors.New("沙漠首领芯片回执无效")
	}
	terminal := g.GGOField
	// A different economy or a departed character leaves a frozen pending award.
	if terminal == nil || g.Position.GetScene() != constUtil.GGOFieldScene || terminal.EconomyVersion != award.EconomyVersion || terminal.ExchangeValues != award.ExchangeValues {
		return nil
	}
	if terminal.Revision == math.MaxInt64 {
		return errors.New("沙漠终端版本溢出")
	}
	for i, n := range award.Chips {
		if n < 0 || terminal.Chips[i] < 0 || n > math.MaxInt64-terminal.Chips[i] {
			return errors.New("首领芯片数量溢出")
		}
	}
	for i, n := range award.Chips {
		terminal.Chips[i] += n
	}
	terminal.Revision++
	key := row.SpawnUUID
	if err := ggoSaveReceipt(tx, id, ggoSystemReceiptID("boss-award", key), constUtil.GGOFieldBossAwardOperation, p6Digest(row.GGOChips), award.EconomyVersion, &key, award.Chips, 0, time.Now().UnixMilli(), award); err != nil {
		return err
	}
	return tx.Model(row).Update("ggo_chips_claimed", true).Error
}

// Explicit safe-area settlement is the only path that turns late awards from
// another adventure into credit. No login, heartbeat or arrival auto-converts it.
func (s *GameService) ggoSettleLateBossAwardsLocked(tx *gorm.DB, id int64, g *model.GameState) (int64, error) {
	var total, afterID int64
	for {
		var rows []db.FieldBossContribution73
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND player_id=? AND claimed=? AND ggo_chips_claimed=? AND id>? AND LENGTH(ggo_chips)>0", s.OriginalServerID(), id, true, false, afterID).Order("id").Limit(200).Find(&rows).Error; err != nil {
			return 0, err
		}
		for i := range rows {
			row := &rows[i]
			afterID = row.ID
			var award ggoBossAward
			if len(row.GGOChips) == 0 {
				continue
			}
			if json.Unmarshal(row.GGOChips, &award) != nil || award.EconomyVersion == "" {
				return 0, errors.New("首领待结芯片回执无效")
			}
			var credit int64
			for j, n := range award.Chips {
				price := award.ExchangeValues[j]
				if n < 0 || price < 1 || n > math.MaxInt64/price || credit > math.MaxInt64-n*price {
					return 0, errors.New("首领待结信用溢出")
				}
				credit += n * price
			}
			if credit > math.MaxInt64-total {
				return 0, errors.New("首领待结信用溢出")
			}
			if err := ggoCreditAdd(g, credit); err != nil {
				return 0, err
			}
			key := row.SpawnUUID
			response := map[string]any{"eventId": key, "chips": award.Chips, "creditAdded": credit, "creditBalance": ggoCreditBalance(g)}
			if err := ggoSaveReceipt(tx, id, ggoSystemReceiptID("boss-late", key), constUtil.GGOFieldBossLateOperation, p6Digest(row.GGOChips), award.EconomyVersion, &key, [6]int64{}, 0, time.Now().UnixMilli(), response); err != nil {
				return 0, err
			}
			if err := tx.Model(row).Update("ggo_chips_claimed", true).Error; err != nil {
				return 0, err
			}
			total += credit
		}
		if len(rows) < 200 {
			break
		}
	}
	return total, nil
}
