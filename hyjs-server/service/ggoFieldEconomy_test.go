package service

import (
	"math"
	"math/rand/v2"
	"testing"

	"hyjs-server/model"
)

func TestGGODeathProtectsHighestAndConservesTransfer(t *testing.T) {
	for _, n := range []int64{0, 2, 3, 4, 5, 6, 9, 10, 20} {
		chips := [6]int64{n, 0, 0, 0, 0, 0}
		got, err := ggoDeathLoss(chips, 3, nil, rand.New(rand.NewPCG(23, uint64(n)+1)))
		if err != nil {
			t.Fatal(err)
		}
		want := min(n/2, max(int64(0), n-3))
		if got[0] != want {
			t.Fatalf("n=%d lost=%d want=%d", n, got[0], want)
		}
	}
	chips := [6]int64{5, 4, 3, 2, 1, 2}
	protected, total, err := ggoProtectedChips(chips, 3)
	if err != nil || total != 17 || protected != ([6]int64{0, 0, 0, 0, 1, 2}) {
		t.Fatal(protected, total, err)
	}
	lost, err := ggoDeathLoss(chips, 3, nil, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	winner := [6]int64{0, 1, 0, 1, 0, 0}
	before := winner
	if err = ggoApplyChipTransfer(&chips, &winner, lost); err != nil {
		t.Fatal(err)
	}
	var count int64
	for i, n := range lost {
		count += n
		if winner[i]-before[i] != n || chips[i] < protected[i] {
			t.Fatal("transfer or protection failed", chips, winner, lost)
		}
	}
	if count != 8 {
		t.Fatal("wrong loss", lost)
	}
}

func TestGGODeathSamplingUsesUnprotectedCounts(t *testing.T) {
	// Pool has three white and one green; protecting the three purple leaves
	// P(2 whites) = 1/2, P(1 white and 1 green) = 1/2.
	random := rand.New(rand.NewPCG(37, 41))
	counts := [6]int64{3, 1, 0, 3, 0, 0}
	matches := 0
	for i := 0; i < 10000; i++ {
		lost, err := ggoDeathLoss(counts, 3, nil, random)
		if err != nil {
			t.Fatal(err)
		}
		if lost[0]+lost[1] != 3 || lost[3] != 0 {
			t.Fatal("invalid pool", lost)
		}
		// Three losses out of four: green is lost with probability 3/4.
		if lost[1] == 1 {
			matches++
		}
	}
	if matches < 7200 || matches > 7800 {
		t.Fatalf("unbalanced sampling: %d", matches)
	}
}

func TestGGOQuarterRemainderAcrossGatherings(t *testing.T) {
	var rem int64
	var got []int64
	for i := 0; i < 4; i++ {
		n, next, err := ggoScaleQuarter(3, rem, 5)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, n)
		rem = next
	}
	want := []int64{3, 4, 4, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
	if rem != 0 {
		t.Fatal(rem)
	}
	for i := 0; i < 4; i++ {
		n, next, err := ggoScaleQuarter(1, rem, 5)
		if err != nil || n != []int64{1, 1, 1, 2}[i] {
			t.Fatal(n, next, err)
		}
		rem = next
	}
}

func TestGGOSettlementUsesFrozenPricesAndChecksOverflow(t *testing.T) {
	game := &model.GameState{GGOCredit: 17, GGOCreditLoaded: true, GGOField: &model.GGOFieldState{Chips: [6]int64{2, 1, 0, 0, 0, 1}, EconomyVersion: "frozen-old", ExchangeValues: [6]int64{3, 9, 30, 90, 300, 900}, Revision: 4}}
	out, err := ggoSettleTerminal(game, "settlement-1")
	if err != nil {
		t.Fatal(err)
	}
	if out.CreditAdded != 915 || out.CreditBalance != 932 || out.Chips != ([6]int64{2, 1, 0, 0, 0, 1}) || out.EconomyVersion != "frozen-old" || out.TerminalRevision != 5 || game.GGOField.Chips != ([6]int64{}) {
		t.Fatal("frozen settlement", out, game.GGOField)
	}
	again, err := ggoSettleTerminal(game, "settlement-2")
	if err != nil || again.CreditAdded != 0 || again.CreditBalance != 932 || again.TerminalRevision != 5 {
		t.Fatal("empty settlement", again, err)
	}
	game.GGOCredit = math.MaxInt64 - 1
	game.GGOField.Chips = [6]int64{1}
	beforeChips, beforeRevision := game.GGOField.Chips, game.GGOField.Revision
	if _, err = ggoSettleTerminal(game, "settlement-overflow"); err == nil {
		t.Fatal("credit overflow accepted")
	}
	if game.GGOCredit != math.MaxInt64-1 || game.GGOField.Chips != beforeChips || game.GGOField.Revision != beforeRevision {
		t.Fatal("overflow mutated terminal or credit")
	}
}

func TestGGODeathLossConfiguredPercentageAndOverflow(t *testing.T) {
	chips := [6]int64{9, 0, 0, 0, 0, 3}
	cases := []struct {
		name    string
		percent *int64
		want    int64
	}{
		{"legacy", nil, 6},
		{"zero", ggoInt64(0), 0},
		{"round-down", ggoInt64(1), 0},
		{"quarter", ggoInt64(25), 3},
		{"half", ggoInt64(50), 6},
		{"three-quarter", ggoInt64(75), 9},
		{"all-unprotected", ggoInt64(100), 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lost, err := ggoDeathLoss(chips, 3, tc.percent, rand.New(rand.NewPCG(7, 11)))
			if err != nil || lost != ([6]int64{tc.want}) {
				t.Fatalf("lost=%v want=%d err=%v", lost, tc.want, err)
			}
		})
	}
	for _, rate := range []int64{-1, 101} {
		if _, err := ggoDeathLoss(chips, 3, &rate, nil); err == nil {
			t.Fatalf("invalid percentage %d accepted", rate)
		}
	}
	for _, rate := range []int64{0, 1, 25, 50, 75, 100} {
		lost, err := ggoDeathLoss([6]int64{math.MaxInt64}, 3, &rate, rand.New(rand.NewPCG(13, uint64(rate)+1)))
		want := min((int64(math.MaxInt64)/100)*rate+(int64(math.MaxInt64)%100)*rate/100, int64(math.MaxInt64)-3)
		if err != nil || lost[0] != want {
			t.Fatalf("rate=%d lost=%v want=%d err=%v", rate, lost, want, err)
		}
	}
}
