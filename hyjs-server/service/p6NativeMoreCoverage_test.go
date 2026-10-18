package service

import "testing"

func TestP6NativeOriginalMoreRealCoverageTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v55.json", "bcc334e612ff9efee0c0082c5df8927b7ecf5591e8c93971e78225b0027bbc13", 14)
}

func TestP6NativeOriginalMoreRealCoverageActorDetailAndBuffMetadataGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v55.json"
		sha  = "bcc334e612ff9efee0c0082c5df8927b7ecf5591e8c93971e78225b0027bbc13"
	)
	t.Run("44-actor-properties-and-counter-mode", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6OriginalDispelTableAliasesRetainInertIdentity(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	owner, target := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(owner, target, e.table("Buff", "Id", "buff_375"), 12); err != nil {
		t.Fatal(err)
	}
	before := e.rng.Draws()
	if err := e.loadBuff(owner, target, e.table("Buff", "Id", "buff_312"), 2); err != nil {
		t.Fatal(err)
	}
	if len(target.buffs) != 2 || target.buffs[0].row["Id"] != "buff_375" || target.buffs[1].row["Id"] != "buff_312" || e.rng.Draws() != before {
		t.Fatal("original class3 clearDebuff alias acquired an invented effect")
	}
	closeCombat(t, target.attribute("minusAddperhp"), -40)
	row := map[string]string{"Factor": "clearBuff", "BuffClass": "3"}
	if !p6OriginalFactoryNoEffect(row) {
		t.Fatal("original clearBuff alias acquired an effect")
	}
	e.originalDispelEffect(target, row)
	if len(target.buffs) != 2 || e.rng.Draws() != before {
		t.Fatal("inert dispel alias changed identity collection or RNG")
	}
}
