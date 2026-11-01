package service

import (
	"math"
	"testing"
)

func TestGGOBossContributionFinalHitULPBoundary(t *testing.T) {
	const maximum = float64(43800000)
	const prior = float64(43339921.424241684)
	const remaining = float64(460078.57575832424)
	if prior <= maximum-remaining {
		t.Fatal("incident fixture no longer reaches the old false rejection")
	}
	got, err := ggoBossContributionTotal(prior, remaining, maximum, true)
	if err != nil || got != maximum {
		t.Fatal("actual clipped final hit must settle exactly at maximum", got, err)
	}
	if _, err = ggoBossContributionTotal(prior, remaining, maximum, false); err == nil {
		t.Fatal("non-final hit admitted an inconsistent previous contribution")
	}
	ulp := math.Nextafter(maximum, math.Inf(1)) - maximum
	within := 1 + 2*ulp
	got, err = ggoBossContributionTotal(maximum-1, within, maximum, true)
	if err != nil || got != maximum {
		t.Fatal("exact two-ULP boundary must clamp", got, err)
	}
	// This exact sum is 2.25 ULP above maximum but its float64 addition
	// rounds to only 2 ULP. The rational guard must still reject it.
	justOver := 1 + 2.25*ulp
	if (maximum-1)+justOver != math.Nextafter(math.Nextafter(maximum, math.Inf(1)), math.Inf(1)) {
		t.Fatal("fixture must exercise a rounded two-ULP sum")
	}
	if _, err = ggoBossContributionTotal(maximum-1, justOver, maximum, true); err == nil {
		t.Fatal("exact amount above two ULP was accepted")
	}
	// The float after the next power of two is twice as widely spaced.
	// Count ULPs at maximum rather than counting representable numbers above it.
	boundary := math.Nextafter(math.Ldexp(1, 54), 0)
	if got, err = ggoBossContributionTotal(boundary-4, 8, boundary, true); err != nil || got != boundary {
		t.Fatal("two ULP at a binade boundary must clamp", got, err)
	}
	if _, err = ggoBossContributionTotal(boundary-4, 10, boundary, true); err == nil {
		t.Fatal("three ULP across a binade boundary was accepted")
	}
	for _, tc := range []struct {
		prior, damage, maximum float64
	}{
		{math.Nextafter(maximum, math.Inf(1)), 1, maximum},
		{maximum - 1, 3, maximum},
		{-1, 1, maximum},
		{0, 0, maximum},
		{0, maximum + 1, maximum},
		{math.NaN(), 1, maximum},
		{0, math.Inf(1), maximum},
		{0, 1, math.Inf(1)},
	} {
		if _, err = ggoBossContributionTotal(tc.prior, tc.damage, tc.maximum, true); err == nil {
			t.Fatal("invalid contribution admitted", tc)
		}
	}
	got, err = ggoBossContributionTotal(100, 50, maximum, false)
	if err != nil || got != 150 {
		t.Fatal("ordinary damage accumulation changed", got, err)
	}
}
