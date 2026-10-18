package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// This is a pure bounded arrangement fixture, not a fabricated qualified roster
// or an assertion that 32 real clients have been run.
func TestP14FinalDrawRealIdentityCapacityGroupsAndUnpairedNeverBecomeWins(t *testing.T) {
	for _, count := range []int{1, 4, 9, 32} {
		var input []p14FinalDrawEntrant
		for i := 0; i < count; i++ {
			input = append(input, p14FinalDrawEntrant{ServerID: int64(i%4 + 1), PlayerID: int64(i/4 + 1), Name: "actual fixture identity", Image: "hero_047", Power: 330})
		}
		before := append([]p14FinalDrawEntrant(nil), input...)
		common := strings.Repeat("a", 64)
		ordered := p14FinalDrawArrange(common, input)
		if len(ordered) != count || !reflect.DeepEqual(before, input) {
			t.Fatal("draw filled missing slots or changed source")
		}
		reversed := append([]p14FinalDrawEntrant(nil), input...)
		for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
			reversed[i], reversed[j] = reversed[j], reversed[i]
		}
		if !reflect.DeepEqual(ordered, p14FinalDrawArrange(common, reversed)) {
			t.Fatal("origin query order changed common draw")
		}
		seen := map[[2]int64]bool{}
		for i, entry := range ordered {
			key := [2]int64{entry.ServerID, entry.PlayerID}
			if seen[key] || entry.Slot != int32(i%8) || entry.Group != string(rune('A'+i/8)) {
				t.Fatal("joint identity or real slot lost")
			}
			seen[key] = true
		}
		pairs := p14FinalDrawPairs(ordered)
		if len(pairs) != count/2 {
			t.Fatal("unpaired slot became a synthetic opponent", count, pairs)
		}
		for _, pair := range pairs {
			if !seen[[2]int64{pair.LeftServerID, pair.LeftPlayerID}] || !seen[[2]int64{pair.RightServerID, pair.RightPlayerID}] || pair.LeftServerID == pair.RightServerID && pair.LeftPlayerID == pair.RightPlayerID {
				t.Fatal("pair contains fabricated/aliased participant")
			}
		}
	}
}

func TestP14FinalDrawCompleteQualificationAndPrepublishedLocalRuleAreRequired(t *testing.T) {
	qualifiedRow, now := p14FinalPureFixture(t, 4)
	qualification, err := p14FinalQualificationValidate(qualifiedRow, now)
	if err != nil {
		t.Fatal(err)
	}
	var input []p14FinalDrawEntrant
	for _, entry := range qualification.Ranking {
		input = append(input, p14FinalDrawEntrant{ServerID: entry.ServerID, PlayerID: entry.PlayerID, Name: entry.Name, Image: "hero_047", Power: entry.Power})
	}
	body := p14FinalDrawSnapshot{SeasonIndex: qualifiedRow.SeasonIndex, Version: "local-fixture-prepublished-v1", Source: constUtil.P14FinalDrawSource, PublishedAt: 1000, QualificationHash: qualifiedRow.ContentHash, RulesHash: p6Digest([]byte(constUtil.P14FinalDrawRules)), Entrants: p14FinalDrawArrange(qualifiedRow.ContentHash, input)}
	body.Pairs = p14FinalDrawPairs(body.Entrants)
	raw, _ := json.Marshal(body)
	row := &db.P14FinalDraw{SeasonIndex: body.SeasonIndex, Content: raw, ContentHash: p6Digest(raw), ObservedAt: 60000}
	if _, err = p14FinalDrawValidate(row, qualification, qualifiedRow.ContentHash, now); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*p14FinalDrawSnapshot){
		func(b *p14FinalDrawSnapshot) { b.PublishedAt = qualification.StageStart + 1 },
		func(b *p14FinalDrawSnapshot) { b.QualificationHash = strings.Repeat("b", 64) },
		func(b *p14FinalDrawSnapshot) { b.Entrants = b.Entrants[:3] },
		func(b *p14FinalDrawSnapshot) { b.Entrants[1] = b.Entrants[0] },
		func(b *p14FinalDrawSnapshot) { b.Entrants[0].Group = "D" },
		func(b *p14FinalDrawSnapshot) { b.Pairs = nil },
	} {
		var changed p14FinalDrawSnapshot
		json.Unmarshal(raw, &changed)
		mutate(&changed)
		content, _ := json.Marshal(changed)
		bad := &db.P14FinalDraw{SeasonIndex: row.SeasonIndex, Content: content, ContentHash: p6Digest(content), ObservedAt: row.ObservedAt}
		if _, err = p14FinalDrawValidate(bad, qualification, qualifiedRow.ContentHash, now); err == nil {
			t.Fatal("invalid real draw accepted")
		}
	}
}

func TestP14FinalDrawEmptyGroupIsPendingEvenWithCompleteEntrants(t *testing.T) {
	var entrants []p14FinalDrawEntrant
	for _, identity := range [][2]int64{{1, 1}, {1, 2}, {2, 1}, {2, 2}} {
		entrants = append(entrants, p14FinalDrawEntrant{ServerID: identity[0], PlayerID: identity[1], Name: "source fixture", Image: "hero_047", Power: 330})
	}
	ready := &p14FinalDrawSnapshot{Entrants: p14FinalDrawArrange(strings.Repeat("a", 64), entrants)}
	actual, err := p14FinalDrawProject(ready, "A")
	if err != nil || len(actual.KFMatchFirstList) != 4 {
		t.Fatal("actual first-round group", err)
	}
	empty, err := p14FinalDrawProject(ready, "B")
	if err != nil || len(empty.KFMatchFirstList) != 0 {
		t.Fatal("genuine unoccupied group", err)
	}
	four, err := p14FinalDrawProject(ready, "")
	if err == nil || four != nil {
		t.Fatal("first-round readiness fabricated a FinalFour response")
	}
}
