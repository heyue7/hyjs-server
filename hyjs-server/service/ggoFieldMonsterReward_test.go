package service

import "testing"

func TestGGOMonsterFractionalRewardsCarryByResource(t *testing.T) {
	remainder := map[string]int64{}
	expected := []string{"hero_exp_005,1;coin,1", "coin,2;hero_exp_005,2"}
	// Compare the sorted output and check independent per-resource carry.
	expected[0] = "coin,1;hero_exp_005,1"
	for i := 0; i < 2; i++ {
		output, next, err := ggoScaleMonsterReward("coin,1;hero_exp_005,1", 3, 2, remainder)
		if err != nil || output != expected[i] {
			t.Fatal("fractional monster reward", i, output, err)
		}
		remainder = next
	}
	if remainder["2:coin"] != 0 || remainder["2:hero_exp_005"] != 0 {
		t.Fatal("remainders not carried", remainder)
	}
}
