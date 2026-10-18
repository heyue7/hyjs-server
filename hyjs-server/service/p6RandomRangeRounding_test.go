package service

import (
	"math"
	"testing"
)

// Actual original case012/0081_3 draw57 exposes ARM64 fused multiply-add.
// Check the original RandomUtil.Range IL operation boundaries at the bit level.
func TestP6OriginalRandomRangeKeepsIntermediateRounding(t *testing.T) {
	rng := NewCombatRandom(42)
	for i := 0; i < 56; i++ {
		rng.Value()
	}
	got := 1 + rng.Range(-.1, .1)
	const want = 1.0743629191929678
	if math.Float64bits(got) != math.Float64bits(want) || rng.Draws() != 57 {
		t.Fatalf("original Range rounding changed %.17g draw%d", got, rng.Draws())
	}
	fused := math.FMA(0.8718145959648387, .2, 1) - .1
	if math.Float64bits(got) == math.Float64bits(fused) {
		t.Fatal("test failed to distinguish the original mul/add from FMA")
	}
}

// Actual native case012 removes two20% reductions from .81 in order. Its
// surviving .4100000000000001 is observable and affects subsequent damage.
func TestP6OriginalDamagePercentRetainsEffectUnEffectResidue(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	a.base["damageReduce"] = 41
	row := map[string]string{"Id": "actual-reduction-residue-probe", "Factor": "damageReduce", "Args1": "20", "TotalRound": "1"}
	for i := 0; i < 2; i++ {
		if err := e.loadBuff(a, a, row, 4); err != nil {
			t.Fatal(err)
		}
	}
	if math.Float64bits(a.originalFraction("damageReduce")) != math.Float64bits(.81) {
		t.Fatal("initial native percentage stack")
	}
	for _, b := range append([]*combatRoundBuff(nil), a.buffs...) {
		e.unloadBuff(a, b)
	}
	if math.Float64bits(a.originalFraction("damageReduce")) != math.Float64bits(.4100000000000001) {
		t.Fatalf("native removal residue %.17g", a.originalFraction("damageReduce"))
	}
}
