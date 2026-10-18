package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6HPTransferInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	a := p6Actor("hero_001", "hp-transfer-caster", "001_1,001_2,001_3", 1, 80000)
	a.Inst.CurHp = proto.Float64(52000)
	a.TalentList = []string{"tal_191"}
	input := p6Input(t, s, a, p6Actor("hero_047", "hp-transfer-target", "047_1,047_2,047_3", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(100)
	return input
}

func TestP6OriginalHPTransferSnapshotsStepsAndUneffectRestore(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6HPTransferInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a := e.players[0].actors[0]
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_346"), 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("damageAdd"), 30)
	// HP changes preserve the original load-time value; Update0 is empty.
	a.hp = 36000
	e.time = 1
	e.updateBuffs(a)
	closeCombat(t, a.attribute("damageAdd"), 30)
	suppressed, err := e.suppressBuffsOnce(a)
	if err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("damageAdd"), 0)
	if err = e.restoreBuffsOnce(a, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("damageAdd"), 50)
	e.unloadBuff(a, a.buffs[0])
	closeCombat(t, a.attribute("damageAdd"), 0)
	if e.rng.Draws() != 0 {
		t.Fatal("HP transfer consumed random draws")
	}
}

func TestP6OriginalHPTransferKindsAndNativeBoundDirections(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6HPTransferInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a := e.players[0].actors[0]
	for _, tc := range []struct {
		factor, key string
		want        float64
	}{
		{"perhpTodamageAdd", "damageAdd", 20}, {"perhpTododge", "dodge", 8}, {"perhpTocri", "cri", 8},
		{"perhpToPeratt", "att", a.attribute("att") * 1.08}, {"perhpToperdef", "pdef", a.attribute("pdef") * 1.08},
	} {
		base := a.attribute(tc.key)
		row := map[string]string{"Id": "original-native-bound-probe", "BuffClass": "1", "Factor": tc.factor, "Args1": "-10", "Args2": "4", "Args3": "8", "TotalRound": "2"}
		if tc.factor == "perhpTodamageAdd" {
			row["Args3"] = "20"
		}
		if err := e.loadBuff(a, a, row, 4); err != nil {
			t.Fatal(err)
		}
		want := tc.want
		if tc.key == "damageAdd" || tc.key == "dodge" || tc.key == "cri" {
			want += base
		}
		closeCombat(t, a.attribute(tc.key), want)
		e.unloadBuff(a, a.buffs[0])
		closeCombat(t, a.attribute(tc.key), base)
	}
	// The native damageAdd branch uses Args3 as a lower bound, even at full HP.
	a.hp = a.attribute("maxHp")
	row := map[string]string{"Id": "original-lower-bound-probe", "BuffClass": "1", "Factor": "perhpTodamageAdd", "Args1": "-10", "Args2": "4", "Args3": "20"}
	if err := e.loadBuff(a, a, row, 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("damageAdd"), 20)
}

func TestP6OriginalHPTransferFullThreeSkillsWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6HPTransferInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	loads := 0
	for _, event := range out.Events {
		if event.Kind == "release" && event.Actor == "0_hp-transfer-caster" {
			seen[event.Skill] = true
		}
		if event.Kind == "buff_load" && event.Buff == "buff_115" {
			loads++
		}
	}
	if len(seen) != 3 || loads < 3 || out.Authoritative {
		t.Fatal("complete native three-skill HP-transfer flow missing", seen, loads)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("HP transfer whole original replay diverged", err)
	}
}

func TestP6HPTransferRejectsZeroDivisorAndFactoryMismatch(t *testing.T) {
	for _, tc := range []struct{ field, value string }{{"Args1", "0"}, {"BuffClass", "0"}} {
		s := p6OriginalTables(t)
		s.table("Buff", "Id", "buff_115")[tc.field] = tc.value
		input := p6HPTransferInput(t, s)
		if _, err := s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
			t.Fatal("invalid HP transfer accepted", tc)
		}
	}
}
