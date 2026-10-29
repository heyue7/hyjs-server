package service

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

func TestGGOHypergeometricMediumDistribution(t *testing.T) {
	// E[X]=20; Var[X]=40*(1/2)*(1/2)*(40/79) for this population.
	random := rand.New(rand.NewPCG(123, 456))
	const samples = 512
	var sum, squares float64
	for i := 0; i < samples; i++ {
		n, err := ggoHypergeometric(random, 40, 40, 40)
		if err != nil || n < 0 || n > 40 {
			t.Fatal("sample outside support", n, err)
		}
		sum += float64(n)
		squares += float64(n * n)
	}
	mean := sum / samples
	variance := squares/samples - mean*mean
	if mean < 19.4 || mean > 20.6 || variance < 3.5 || variance > 6.8 {
		t.Fatalf("distribution mean=%v variance=%v", mean, variance)
	}
}

func TestGGODeathLossSupportsInt64Population(t *testing.T) {
	random := rand.New(rand.NewPCG(789, 987))
	cases := [][6]int64{
		{math.MaxInt64 - 15, 1, 2, 3, 4, 5},
		{math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6},
	}
	started := time.Now()
	for _, chips := range cases {
		protected, total, err := ggoProtectedChips(chips, 3)
		if err != nil {
			t.Fatal(err)
		}
		lost, err := ggoDeathLoss(chips, 3, nil, random)
		if err != nil {
			t.Fatal(err)
		}
		var actual int64
		for i, n := range lost {
			if n < 0 || n > chips[i]-protected[i] {
				t.Fatal("protected chip selected", chips, lost, protected)
			}
			actual += n
		}
		if actual != min(total/2, total-min(total, int64(3))) {
			t.Fatal("loss count changed", actual, total)
		}
	}
	t.Logf("two int64 boundary samples completed in %s", time.Since(started))
}

func BenchmarkGGODeathLossInt64Balanced(b *testing.B) {
	random := rand.New(rand.NewPCG(123, 456))
	chips := [6]int64{math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6, math.MaxInt64 / 6}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ggoDeathLoss(chips, 3, nil, random); err != nil {
			b.Fatal(err)
		}
	}
}
