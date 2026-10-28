package service

import (
	"errors"
	"math/big"
	mathrand "math/rand/v2"
	"sync"
)

// Sampling a color count after the preceding colors gives the same law as
// drawing individual chips uniformly without replacement. Small cases use
// integer draws; large cases use the ratio-of-uniforms hypergeometric method
// described by E. Stadlober, J. Comput. Appl. Math. 31 (1990), 181–189.
// Its arithmetic uses 192-bit precision: binary64 cannot distinguish adjacent
// counts throughout the int64 domain, nor accurately subtract log factorials.
// A rejected or numerically invalid sample aborts its surrounding transaction.
const ggoSamplingPrecision uint = 192

func ggoNumber() *big.Float {
	return new(big.Float).SetPrec(ggoSamplingPrecision).SetMode(big.ToNearestEven)
}
func ggoInteger(n int64) *big.Float     { return ggoNumber().SetInt64(n) }
func ggoAdd(a, b *big.Float) *big.Float { return ggoNumber().Add(a, b) }
func ggoSub(a, b *big.Float) *big.Float { return ggoNumber().Sub(a, b) }
func ggoMul(a, b *big.Float) *big.Float { return ggoNumber().Mul(a, b) }
func ggoDiv(a, b *big.Float) *big.Float { return ggoNumber().Quo(a, b) }
func ggoDecimal(s string) *big.Float {
	v, ok := ggoNumber().SetString(s)
	if !ok {
		panic("invalid constant in desert sampler")
	}
	return v
}

var (
	ggoLogTwo       = ggoDecimal("0.6931471805599453094172321214581765680755001343602552541206800094933936")
	ggoHalfLogTwoPi = ggoDecimal("0.9189385332046727417803297364056176398613974736377834128171515404827657")
	ggoOneHalf      = ggoDecimal("0.5")
	// Slightly enlarged constants preserve the rejection envelope.
	ggoHruaScale         = ggoDecimal("1.7155277699214140")
	ggoHruaOffset        = ggoDecimal("0.8989161620588993")
	ggoSmallLogFactorial [32]*big.Float
	ggoSmallLogOnce      sync.Once
)

// ggoLog computes log(x) by range reduction and an atanh series. Inputs are
// positive; the reduced series ratio is at most 1/3. This also handles the
// 128-bit uniform fraction used by the rejection test without a float64 cast.
func ggoLog(x *big.Float) *big.Float {
	mantissa := ggoNumber()
	exponent := x.MantExp(mantissa)
	mantissa.Mul(mantissa, ggoInteger(2))
	exponent--
	z := ggoDiv(ggoSub(mantissa, ggoInteger(1)), ggoAdd(mantissa, ggoInteger(1)))
	squared := ggoMul(z, z)
	term := ggoNumber().Set(z)
	sum := ggoNumber().Set(z)
	for denominator := int64(3); denominator < 2048; denominator += 2 {
		term.Mul(term, squared)
		next := ggoAdd(sum, ggoDiv(term, ggoInteger(denominator)))
		if next.Cmp(sum) == 0 {
			break
		}
		sum = next
	}
	return ggoAdd(ggoMul(sum, ggoInteger(2)), ggoMul(ggoInteger(int64(exponent)), ggoLogTwo))
}

func ggoLogFactorial(n int64) *big.Float {
	if n < 32 {
		ggoSmallLogOnce.Do(func() {
			ggoSmallLogFactorial[0] = ggoInteger(0)
			for i := int64(1); i < 32; i++ {
				ggoSmallLogFactorial[i] = ggoAdd(ggoSmallLogFactorial[i-1], ggoLog(ggoInteger(i)))
			}
		})
		return ggoNumber().Set(ggoSmallLogFactorial[n])
	}
	value := ggoInteger(n)
	result := ggoAdd(ggoSub(ggoMul(ggoAdd(value, ggoOneHalf), ggoLog(value)), value), ggoHalfLogTwoPi)
	// Stirling's series with a remainder below 2^-130 at n >= 32.
	coefficients := [][2]int64{{1, 12}, {-1, 360}, {1, 1260}, {-1, 1680}, {1, 1188}, {-691, 360360}, {1, 156}, {-3617, 122400}, {43867, 244188}, {-174611, 125400}, {77683, 5796}, {-236364091, 1506960}, {657931, 300}, {-3392780147, 93960}, {1723168255201, 2492028}}
	inverse := ggoDiv(ggoInteger(1), value)
	inverseSquared := ggoMul(inverse, inverse)
	power := ggoNumber().Set(inverse)
	for _, fraction := range coefficients {
		result.Add(result, ggoDiv(ggoMul(power, ggoInteger(fraction[0])), ggoInteger(fraction[1])))
		power.Mul(power, inverseSquared)
	}
	return result
}

// A 128-bit fraction avoids the 53-bit granularity of Float64 near 2^63.
func ggoUniform128(random *mathrand.Rand) *big.Float {
	n := new(big.Int).SetUint64(random.Uint64())
	n.Lsh(n, 64)
	n.Or(n, new(big.Int).SetUint64(random.Uint64()))
	scale := new(big.Int).Lsh(big.NewInt(1), 128)
	return ggoDiv(ggoNumber().SetInt(n), ggoNumber().SetInt(scale))
}

func ggoHypergeometricSmall(random *mathrand.Rand, good, bad, sample int64) int64 {
	total := good + bad
	draws := sample
	complement := sample > total/2
	if complement {
		draws = total - sample
	}
	left := good
	for i := int64(0); i < draws && left > 0 && left < total; i++ {
		if random.Int64N(total) < left {
			left--
		}
		total--
	}
	if left == total {
		left -= draws - min(draws, good+bad-total)
	}
	if complement {
		return left
	}
	return good - left
}

func ggoHypergeometric(random *mathrand.Rand, good, bad, sample int64) (int64, error) {
	if random == nil || good < 0 || bad < 0 || good > int64(^uint64(0)>>1)-bad || sample < 0 || sample > good+bad {
		return 0, errors.New("芯片抽样参数无效")
	}
	total := good + bad
	if sample == 0 || good == 0 {
		return 0, nil
	}
	if sample == total {
		return good, nil
	}
	if bad == 0 {
		return sample, nil
	}
	// Symmetries keep the direct, integer-only path bounded even when the
	// population is enormous but one side contains only a few chips.
	if min(sample, total-sample) < 32 {
		return ggoHypergeometricSmall(random, good, bad, sample), nil
	}
	if good < 32 {
		return ggoHypergeometricSmall(random, sample, total-sample, good), nil
	}
	if bad < 32 {
		return sample - ggoHypergeometricSmall(random, sample, total-sample, bad), nil
	}
	n := min(sample, total-sample)
	small, large := min(good, bad), max(good, bad)
	population := ggoInteger(total)
	p := ggoDiv(ggoInteger(small), population)
	q := ggoDiv(ggoInteger(large), population)
	mean := ggoMul(ggoInteger(n), p)
	variance := ggoDiv(ggoMul(ggoMul(ggoMul(ggoInteger(total-n), ggoInteger(n)), p), q), ggoInteger(total-1))
	width := ggoAdd(ggoMul(ggoHruaScale, ggoNumber().Sqrt(ggoAdd(variance, ggoOneHalf))), ggoHruaOffset)
	center := ggoAdd(mean, ggoOneHalf)
	// The mode formula is evaluated with integers so total+2 never overflows.
	numerator := new(big.Int).Mul(big.NewInt(n+1), big.NewInt(small+1))
	denominator := new(big.Int).Add(big.NewInt(total), big.NewInt(2))
	mode := new(big.Int).Quo(numerator, denominator).Int64()
	modeLog := ggoAdd(ggoAdd(ggoLogFactorial(mode), ggoLogFactorial(small-mode)), ggoAdd(ggoLogFactorial(n-mode), ggoLogFactorial(large-n+mode)))
	upper := ggoInteger(min(n, small) + 1)
	zero, one := ggoInteger(0), ggoInteger(1)
	for attempt := 0; attempt < 512; attempt++ {
		u, v := ggoUniform128(random), ggoUniform128(random)
		if u.Sign() == 0 {
			continue
		}
		proposal := ggoAdd(center, ggoDiv(ggoMul(width, ggoSub(v, ggoOneHalf)), u))
		if proposal.Sign() < 0 || proposal.Cmp(upper) >= 0 {
			continue
		}
		candidate, _ := proposal.Int(nil)
		k := candidate.Int64()
		candidateLog := ggoAdd(ggoAdd(ggoLogFactorial(k), ggoLogFactorial(small-k)), ggoAdd(ggoLogFactorial(n-k), ggoLogFactorial(large-n+k)))
		ratio := ggoSub(modeLog, candidateLog)
		if ratio.Sign() > 0 {
			ratio = zero
		}
		// The first two algebraic tests avoid a logarithm for most proposals.
		accept := ggoSub(ggoMul(u, ggoSub(ggoInteger(4), u)), ggoInteger(3)).Cmp(ratio) <= 0
		if !accept && ggoMul(u, ggoSub(u, ratio)).Cmp(one) >= 0 {
			continue
		}
		if !accept {
			accept = ggoMul(ggoInteger(2), ggoLog(u)).Cmp(ratio) <= 0
		}
		if !accept {
			continue
		}
		if good > bad {
			k = n - k
		}
		if n < sample {
			k = good - k
		}
		return k, nil
	}
	return 0, errors.New("芯片抽样未能收敛")
}
