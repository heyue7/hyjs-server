package service

import (
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestP13P6MatchScopeStrictFrozenFacts(t *testing.T) {
	for _, source := range p13P6MatchSources() {
		input := model.FrozenBattleInput{BattleType: 14, AppliedProperties: map[string][]string{"hero": {"origin;match=match-a;frame=abc"}}}
		got, e := p13P6MatchFromInput(source, &input)
		if e != nil || got != "match-a" {
			t.Fatal(source, got, e)
		}
		input.AppliedProperties["other"] = []string{"origin;match=match-b;frame=def"}
		if _, e = p13P6MatchFromInput(source, &input); e == nil {
			t.Fatal("conflicting match accepted")
		}
		input.AppliedProperties = nil
		if _, e = p13P6MatchFromInput(source, &input); e == nil {
			t.Fatal("missing match accepted")
		}
		input.BattleType = 11
		if _, e = p13P6MatchFromInput(source, &input); e == nil {
			t.Fatal("wrong mode accepted")
		}
	}
	if scope, e := p13P6MatchFromInput("unrelated-mode", nil); e != nil || scope != "" {
		t.Fatal("other domains changed", scope, e)
	}
	input := model.FrozenBattleInput{BattleType: 14, AppliedProperties: map[string][]string{"hero": {"origin;match= ;frame=abc"}}}
	if _, e := p13P6MatchFromInput(constUtil.P13UFPlayerBattleSource, &input); e == nil {
		t.Fatal("blank scope accepted")
	}
}
