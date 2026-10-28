package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Freeze the original 35-level reward pool at admission; a retry cannot reroll
// the quantities. Chips are rolled independently from this ordinary pool.
func ggoRollMonsterReward(text string) (string, error) {
	if text == "" {
		return "", errors.New("沙漠小怪奖励池缺失")
	}
	values := map[string]int64{}
	for _, spec := range strings.Split(text, "|") {
		pair := strings.Split(spec, ":")
		if len(pair) != 2 || pair[0] == "" {
			return "", errors.New("沙漠小怪奖励池无效")
		}
		selected, err := fieldWeighted(pair[1])
		if err != nil {
			return "", err
		}
		n, err := strconv.ParseInt(selected, 10, 32)
		if err != nil || n < 0 || values[pair[0]] > math.MaxInt32-n {
			return "", errors.New("沙漠小怪奖励数量无效")
		}
		values[pair[0]] += n
	}
	keys := make([]string, 0, len(values))
	for key, n := range values {
		if n > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s,%d", key, values[key]))
	}
	return strings.Join(parts, ";"), nil
}

// The remainder is stored per resource and denominator, so distinct multiplier
// denominators never reinterpret old fractions. Inputs and outputs are integer.
func ggoScaleMonsterReward(raw string, numerator, denominator int64, prior map[string]int64) (string, map[string]int64, error) {
	if numerator < 1 || numerator > 1000 || denominator < 1 || denominator > 1000 {
		return "", nil, errors.New("沙漠小怪倍率无效")
	}
	next := map[string]int64{}
	for key, n := range prior {
		next[key] = n
	}
	if raw == "" {
		return "", next, nil
	}
	values := map[string]int64{}
	for _, part := range strings.Split(raw, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 || pair[0] == "" {
			return "", nil, errors.New("沙漠小怪奖励无效")
		}
		n, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || n < 1 {
			return "", nil, errors.New("沙漠小怪奖励数量无效")
		}
		key := fmt.Sprintf("%d:%s", denominator, pair[0])
		rem := next[key]
		if rem < 0 || rem >= denominator || n > (math.MaxInt64-rem)/numerator {
			return "", nil, errors.New("沙漠小怪奖励余数无效")
		}
		scaled := n*numerator + rem
		amount := scaled / denominator
		next[key] = scaled % denominator
		if amount > math.MaxInt32 || values[pair[0]] > math.MaxInt32-amount {
			return "", nil, errors.New("沙漠小怪奖励数量溢出")
		}
		values[pair[0]] += amount
	}
	keys := make([]string, 0, len(values))
	for key, n := range values {
		if n > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s,%d", key, values[key]))
	}
	return strings.Join(parts, ";"), next, nil
}
