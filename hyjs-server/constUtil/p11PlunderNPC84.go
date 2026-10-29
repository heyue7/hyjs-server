package constUtil

const (
	P11PlunderNPC84IDBase        int64 = 2000000000000
	P11PlunderNPC84Count               = 10
	P11PlunderNPC84Source              = "feedback84-plunder-npc-original-heroes-v1"
	P11PlunderNPC84FragmentEvent       = "treasure_fragment_npc_awarded"
	P11Plunder84MatchScanLimit         = 32
)

// P11PlunderNPC84Template declares a local operating roster, not an original
// private-server drop table. Its displayed power is computed from its actors.
type P11PlunderNPC84Template struct {
	TargetPower int32
	Heroes      []string
}

func P11PlunderNPC84Templates() []P11PlunderNPC84Template {
	return []P11PlunderNPC84Template{
		{50000, []string{"hero_047"}},
		{100000, []string{"hero_047", "hero_025"}},
		{200000, []string{"hero_047", "hero_011"}},
		{400000, []string{"hero_047", "hero_003"}},
		{500000, []string{"hero_047", "hero_001"}},
		{600000, []string{"hero_047", "hero_025", "hero_011"}},
		{700000, []string{"hero_047", "hero_003", "hero_025"}},
		{800000, []string{"hero_047", "hero_001", "hero_003"}},
		{900000, []string{"hero_047", "hero_001", "hero_003", "hero_025"}},
		{1000000, []string{"hero_047", "hero_011", "hero_025", "hero_003"}},
	}
}
