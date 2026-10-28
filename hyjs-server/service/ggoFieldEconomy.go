package service

import (
	"crypto/rand"
	"errors"
	"io"
	"math"
	mathrand "math/rand/v2"
)

// ggoProtectedChips derives protected units from highest to lowest quality.
func ggoProtectedChips(chips [6]int64, slots int64) ([6]int64, int64, error) {
	var protected [6]int64
	var total int64
	if slots < 0 {
		return protected, 0, errors.New("芯片保护数量无效")
	}
	for _, n := range chips {
		if n < 0 || n > math.MaxInt64-total {
			return protected, 0, errors.New("芯片数量无效")
		}
		total += n
	}
	left := min(slots, total)
	for i := len(chips) - 1; i >= 0; i-- {
		protected[i] = min(left, chips[i])
		left -= protected[i]
	}
	return protected, total, nil
}

// ggoDeathLoss samples uniformly without replacement from the unprotected
// six-color counts, using constant memory and cryptographically seeded randomness.
func ggoDeathLoss(chips [6]int64, slots int64, random *mathrand.Rand) ([6]int64, error) {
	var result [6]int64
	protected, total, err := ggoProtectedChips(chips, slots)
	if err != nil {
		return result, err
	}
	var pool [6]int64
	var remaining int64
	for i := range pool {
		pool[i] = chips[i] - protected[i]
		remaining += pool[i]
	}
	lose := min(total/2, remaining)
	if lose == 0 {
		return result, nil
	}
	if random == nil {
		var seed [32]byte
		if _, err = io.ReadFull(rand.Reader, seed[:]); err != nil {
			return result, err
		}
		random = mathrand.New(mathrand.NewChaCha8(seed))
	}
	// Conditional hypergeometric draws are equivalent to uniformly choosing
	// individual chips, while the number of colors stays bounded.
	draws := lose
	for i := 0; i < len(pool)-1; i++ {
		selected, e := ggoHypergeometric(random, pool[i], remaining-pool[i], draws)
		if e != nil {
			return [6]int64{}, e
		}
		result[i] = selected
		draws -= selected
		remaining -= pool[i]
	}
	result[len(result)-1] = draws
	return result, nil
}

// ggoApplyChipTransfer validates all colors before changing either side, so a
// caller can persist both profile updates and the fight receipt atomically.
func ggoApplyChipTransfer(loser, winner *[6]int64, lost [6]int64) error {
	if loser == nil {
		return errors.New("芯片终端缺失")
	}
	for i, n := range lost {
		if n < 0 || (*loser)[i] < n || winner != nil && ((*winner)[i] < 0 || (*winner)[i] > math.MaxInt64-n) {
			return errors.New("芯片转移数量无效")
		}
	}
	for i, n := range lost {
		(*loser)[i] -= n
		if winner != nil {
			(*winner)[i] += n
		}
	}
	return nil
}

// ggoScaleQuarter maintains per-resource quarter units across encounters and days.
func ggoScaleQuarter(base, remainder, numerator int64) (int64, int64, error) {
	if base < 0 || remainder < 0 || remainder > 3 || numerator < 1 || numerator > 1000 || base > (math.MaxInt64-remainder)/numerator {
		return 0, 0, errors.New("采集数量或余数无效")
	}
	n := base*numerator + remainder
	return n / 4, n % 4, nil
}
