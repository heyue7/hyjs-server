package service

import (
	"crypto/rand"
	"errors"
	"hyjs-server/db"
	"math"
	"math/big"
)

// recruitHighProbability35 用百万分率向下取整；线性软保底是本服规则。
func recruitHighProbability35(high, total int64, since, every int32) int64 {
	const scale int64 = 1000000
	base := new(big.Int).Mul(big.NewInt(high), big.NewInt(scale))
	base.Div(base, big.NewInt(total))
	p := base.Int64()
	if every <= 0 {
		return p
	}
	n := int64(since) + 1
	h := int64(every)
	if n >= h {
		return scale
	}
	soft := h * 8 / 10
	if n > soft {
		p += (scale - p) * (n - soft) / (h - soft)
	}
	return p
}

func recruitAddWeight35(total, weight int64) (int64, error) {
	if weight <= 0 || weight > 1000000000000 || total < 0 || total > math.MaxInt64-weight {
		return 0, errors.New("奖池权重无效")
	}
	return total + weight, nil
}

// recruitProbabilityGroups35 先验证全部权重，再划分高低品质，防止求和溢出。
func recruitProbabilityGroups35(rewards []db.GameplayPoolReward, since, every int32) ([]db.GameplayPoolReward, []db.GameplayPoolReward, int64, error) {
	var high, low []db.GameplayPoolReward
	var total, highWeight int64
	if since < 0 {
		return nil, nil, 0, errors.New("连续未中奖次数无效")
	}
	for _, r := range rewards {
		next, err := recruitAddWeight35(total, r.Weight)
		if err != nil {
			return nil, nil, 0, err
		}
		total = next
		if r.Guaranteed {
			high = append(high, r)
			highWeight += r.Weight
		} else {
			low = append(low, r)
		}
	}
	if total == 0 || (every > 0 && highWeight == 0) {
		return nil, nil, 0, errors.New("奖池缺少有效奖励或保底奖励")
	}
	return high, low, recruitHighProbability35(highWeight, total, since, every), nil
}

// chooseRecruitProbabilityReward 每抽独立判定品质，组内继续使用原整数权重抽取。
func chooseRecruitProbabilityReward(rewards []db.GameplayPoolReward, since, every int32) (*db.GameplayPoolReward, error) {
	high, low, p, err := recruitProbabilityGroups35(rewards, since, every)
	if err != nil {
		return nil, err
	}
	if every <= 0 {
		return choosePoolReward(rewards, false)
	}
	if len(low) == 0 {
		return choosePoolReward(high, false)
	}
	roll, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return nil, err
	}
	if roll.Int64() < p {
		return choosePoolReward(high, false)
	}
	return choosePoolReward(low, false)
}
