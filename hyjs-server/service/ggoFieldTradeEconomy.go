package service

import (
	"errors"
	"math"
	"math/big"

	"hyjs-server/model"
)

// ggoTradeAfter calculates both post-trade terminals before mutating either.
// Each received batch retains its original economic version and six prices.
func ggoTradeAfter(states [2]*model.GGOFieldState, offers [2][6]int64, allowGift bool) ([2]model.GGOFieldState, error) {
	var out [2]model.GGOFieldState
	var offered, held [2]int64
	for i := range states {
		if states[i] == nil {
			return out, errors.New("交易终端缺失")
		}
		out[i] = *states[i]
		_, total, err := ggoProtectedChips(states[i].Chips, 0)
		if err != nil {
			return out, err
		}
		held[i] = total
		_, offered[i], err = ggoProtectedChips(offers[i], 0)
		if err != nil || offered[i] > total {
			return out, errors.New("交易芯片数量无效")
		}
		for color, amount := range offers[i] {
			if amount > states[i].Chips[color] {
				return out, errors.New("交易芯片不足")
			}
			if amount > 0 && (states[i].EconomyVersion == "" || states[i].ExchangeValues[color] < 1) {
				return out, errors.New("交易芯片冻结价格缺失")
			}
		}
	}
	if offered[0] == 0 && offered[1] == 0 || !allowGift && (offered[0] == 0 || offered[1] == 0) {
		return out, errors.New("交易报价为空")
	}
	for i := 0; i < 2; i++ {
		peer := 1 - i
		remain := held[i] - offered[i]
		if remain > math.MaxInt64-offered[peer] {
			return out, errors.New("交易芯片总数溢出")
		}
		if offered[peer] > 0 {
			if remain > 0 && (states[i].EconomyVersion != states[peer].EconomyVersion || states[i].ExchangeValues != states[peer].ExchangeValues) {
				return out, errors.New("交易芯片经济版本不一致")
			}
			if remain == 0 {
				out[i].EconomyVersion, out[i].ExchangeValues = states[peer].EconomyVersion, states[peer].ExchangeValues
			}
		}
		for color := range out[i].Chips {
			base := states[i].Chips[color] - offers[i][color]
			if base > math.MaxInt64-offers[peer][color] {
				return out, errors.New("交易芯片数量溢出")
			}
			out[i].Chips[color] = base + offers[peer][color]
		}
		if states[i].Revision == math.MaxInt64 {
			return out, errors.New("交易终端版本溢出")
		}
		out[i].Revision++
	}
	return out, nil
}

func ggoTradeOfferValue(state *model.GGOFieldState, offer [6]int64) (string, error) {
	if state == nil {
		return "", errors.New("交易终端缺失")
	}
	total := new(big.Int)
	for i, amount := range offer {
		if amount < 0 || amount > state.Chips[i] || amount > 0 && state.ExchangeValues[i] < 1 {
			return "", errors.New("交易报价无效")
		}
		part := new(big.Int).Mul(big.NewInt(amount), big.NewInt(state.ExchangeValues[i]))
		total.Add(total, part)
	}
	return total.String(), nil
}
