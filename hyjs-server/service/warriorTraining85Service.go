package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type training85Rule struct {
	id, v1Distribution, v1GoSource, v1SQLSource, v3Distribution, v3Source string
}

func training85Rules() [3]training85Rule {
	return [3]training85Rule{
		{"training_1", constUtil.WarriorTraining85V1Distribution1, constUtil.WarriorTraining85V1GoSource1, constUtil.WarriorTraining85V1SQLSource1, constUtil.WarriorTraining85Distribution1, constUtil.WarriorTraining85Source1},
		{"training_2", constUtil.WarriorTraining85V1Distribution2, constUtil.WarriorTraining85V1GoSource2, constUtil.WarriorTraining85V1SQLSource2, constUtil.WarriorTraining85Distribution2, constUtil.WarriorTraining85Source2},
		{"training_3", constUtil.WarriorTraining85V1Distribution3, constUtil.WarriorTraining85V1GoSource3, constUtil.WarriorTraining85V1SQLSource3, constUtil.WarriorTraining85Distribution3, constUtil.WarriorTraining85Source3},
	}
}

func training85Defaults() []db.WarriorGrowthRule {
	rules := training85Rules()
	defaults := make([]db.WarriorGrowthRule, 0, len(rules))
	for _, rule := range rules {
		defaults = append(defaults, db.WarriorGrowthRule{ID: rule.id, Enabled: true, Distribution: rule.v3Distribution, CriticalMultiplier: 1, ResetSeconds: 0, Source: rule.v3Source})
	}
	return defaults
}

// ApplyWarriorTraining85 is an explicit maintenance action. Lock and check all
// three rows before changing any, so custom policy and partial upgrades survive.
func (s *GameService) ApplyWarriorTraining85(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(applyWarriorTraining85)
}

func applyWarriorTraining85(tx *gorm.DB) error {
	rules := training85Rules()
	var rows [3]db.WarriorGrowthRule
	var legacy, current int
	for i, rule := range rules {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rows[i], "id = ?", rule.id).Error; err != nil {
			return err
		}
		row := rows[i]
		if row.ID != rule.id || !row.Enabled || row.CriticalMultiplier != 1 || row.ResetSeconds != 0 {
			return fmt.Errorf("%w: 训练规则 %s 已调整", constUtil.ErrAdminConfigConflict, rule.id)
		}
		switch {
		case row.Distribution == rule.v1Distribution && (row.Source == rule.v1GoSource || row.Source == rule.v1SQLSource):
			legacy++
		case row.Distribution == rule.v3Distribution && row.Source == rule.v3Source:
			current++
		default:
			return fmt.Errorf("%w: 训练规则 %s 已调整", constUtil.ErrAdminConfigConflict, rule.id)
		}
	}
	if current == len(rules) {
		return nil
	}
	if legacy != len(rules) {
		return fmt.Errorf("%w: 训练规则版本不一致", constUtil.ErrAdminConfigConflict)
	}
	for _, rule := range rules {
		result := tx.Model(&db.WarriorGrowthRule{}).Where("id = ?", rule.id).Updates(map[string]any{"distribution": rule.v3Distribution, "source": rule.v3Source})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: 训练规则 %s 更新失败", constUtil.ErrAdminConfigConflict, rule.id)
		}
	}
	return nil
}

type training85Distribution struct {
	options [][2]int64
	total   int64
}

// Validate once before charging; sampling never consumes a throw for validation.
func training85ParseDistribution(raw string) (training85Distribution, error) {
	var distribution training85Distribution
	if err := json.Unmarshal([]byte(raw), &distribution.options); err != nil {
		return distribution, err
	}
	for _, option := range distribution.options {
		if option[0] < -10000 || option[0] > 10000 || option[1] <= 0 || distribution.total > math.MaxInt64-option[1] {
			return distribution, errors.New(constUtil.WarriorGrowthConfig)
		}
		distribution.total += option[1]
	}
	if distribution.total <= 0 {
		return distribution, errors.New(constUtil.WarriorGrowthConfig)
	}
	return distribution, nil
}

func training85CryptoDraw(bound int64) (int64, error) {
	dice, err := rand.Int(rand.Reader, big.NewInt(bound))
	if err != nil {
		return 0, err
	}
	return dice.Int64(), nil
}

func (distribution training85Distribution) pick(draw func(int64) (int64, error)) (int64, error) {
	position, err := draw(distribution.total)
	if err != nil {
		return 0, err
	}
	if position < 0 || position >= distribution.total {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	for _, option := range distribution.options {
		position -= option[1]
		if position < 0 {
			return option[0], nil
		}
	}
	return 0, errors.New(constUtil.WarriorGrowthConfig)
}

// Older and operator-defined sources keep their original signed bps rounding.
func training85LegacyChange(limit int32, bps int64) int64 {
	center := int64(limit) * bps / 10000
	if bps == 0 {
		return 0
	}
	if center == 0 {
		if bps > 0 {
			center = 1
		} else {
			center = -1
		}
	}
	return center
}

// Jitter endpoints round inward so the signed magnitude stays within 80..120%.
func training85Jitter(center int64, draw func(int64) (int64, error)) (int64, error) {
	if center == 0 {
		return 0, nil
	}
	sign := int64(1)
	if center < 0 {
		sign, center = -1, -center
	}
	minimum := max(int64(1), (center*constUtil.WarriorTraining85JitterLow+99)/100)
	maximum := max(minimum, center*constUtil.WarriorTraining85JitterHigh/100)
	if minimum == maximum {
		return sign * minimum, nil
	}
	offset, err := draw(maximum - minimum + 1)
	if err != nil {
		return 0, err
	}
	if offset < 0 || offset > maximum-minimum {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	return sign * (minimum + offset), nil
}

func training85UsesGradient(costType int32, source string) bool {
	if costType < 1 || costType > 3 {
		return false
	}
	return source == training85Rules()[costType-1].v3Source
}

// Evaluate round(9500 - 7000*(current/cap)^2) without allocations or
// overflow even at the maximum int32 cap. A half is rounded upward.
func training85GradientThreshold(current, cap int32) (int64, error) {
	if cap <= 0 || current < 0 || current > cap {
		return 0, errors.New(constUtil.WarriorGrowthInvalid)
	}
	c := uint64(current)
	k := uint64(cap)
	denominator := k * k
	high, low := bits.Mul64(c*c, constUtil.WarriorTraining85GradientDrop)
	quotient, remainder := bits.Div64(high, low, denominator)
	threshold := constUtil.WarriorTraining85GradientPeak - int64(quotient)
	if remainder > denominator-remainder {
		threshold--
	}
	return threshold, nil
}

// Sign depends only on the attribute's current ratio. The selected tier scales
// both directions; each step then applies a separate cryptographic jitter.
func training85GradientChange(current, cap int32, rate int64, draw func(int64) (int64, error)) (int64, error) {
	threshold, err := training85GradientThreshold(current, cap)
	if err != nil {
		return 0, err
	}
	dice, err := draw(constUtil.WarriorTraining85GradientScale)
	if err != nil {
		return 0, err
	}
	if dice < 0 || dice >= constUtil.WarriorTraining85GradientScale || rate < 1 || rate > constUtil.WarriorTraining85GradientScale {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	positive := dice < threshold
	distance := int64(cap - current)
	scale := constUtil.WarriorTraining85GradientScale
	if !positive {
		distance = int64(current)
		scale = constUtil.WarriorTraining85NegativeScale
	}
	if distance == 0 {
		return 0, nil
	}
	center := max(int64(1), distance*rate/scale)
	if !positive {
		center = -center
	}
	return training85Jitter(center, draw)
}
