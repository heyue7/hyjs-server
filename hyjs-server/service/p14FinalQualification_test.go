package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func p14FinalPureFixture(t *testing.T, count int) (*db.P14FinalQualification, time.Time) {
	t.Helper()
	now := time.UnixMilli(100000)
	body := p14FinalQualificationSnapshot{SeasonIndex: 61, PolicyVersion: "prepublished-v1", Source: constUtil.P14FinalQualificationSource, PublishedAt: 1000, StageStart: 2000, StageEnd: 50000, RankRulesHash: p6Digest([]byte(constUtil.P14PromotionRankRules)), CohortHash: p6Digest([]byte(`[1,2]`)), Sources: []p14FinalScoreSource{{ServerID: 1, Revision: 2, VersionHash: strings.Repeat("a", 64), QualificationHash: strings.Repeat("b", 64)}, {ServerID: 2, Revision: 3, VersionHash: strings.Repeat("c", 64), QualificationHash: strings.Repeat("d", 64)}}}
	serverCount := 2
	if count > 20 {
		serverCount = 4
		body.Sources = append(body.Sources, p14FinalScoreSource{ServerID: 3, Revision: 1, VersionHash: strings.Repeat("e", 64), QualificationHash: strings.Repeat("f", 64)}, p14FinalScoreSource{ServerID: 4, Revision: 1, VersionHash: strings.Repeat("0", 64), QualificationHash: strings.Repeat("1", 64)})
		body.CohortHash = p6Digest([]byte(`[1,2,3,4]`))
	}
	for i := 0; i < count; i++ {
		body.Ranking = append(body.Ranking, p14PromotionRankProjection{ServerID: int64(i%serverCount + 1), PlayerID: int64(i/serverCount + 1), Score: 1, Power: 100, Name: "real qualified source"})
	}
	p14PromotionRankSort(body.Ranking)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return &db.P14FinalQualification{SeasonIndex: 61, Content: raw, ContentHash: p6Digest(raw), ObservedAt: 60000}, now
}

func TestP14FinalQualificationOriginalTop32KeepsJointIdentityNoFillerOrByeWins(t *testing.T) {
	row, now := p14FinalPureFixture(t, 33)
	body, err := p14FinalQualificationValidate(row, now)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range body.Ranking {
		if p14FinalQualificationEntered(body, entry.ServerID, entry.PlayerID) == 1 {
			count++
		}
	}
	if count != 32 || p14FinalQualificationEntered(body, 4, 7) != 1 || p14FinalQualificationEntered(body, 4, 8) != 0 {
		t.Fatal("joint identity or original top32 capacity")
	}
	sparse, _ := p14FinalPureFixture(t, 4)
	body, err = p14FinalQualificationValidate(sparse, now)
	if err != nil || len(body.Ranking) != 4 || p14FinalQualificationEntered(body, 1, 99) != 0 || p14FinalQualificationEntered(body, 2, 1) != 1 {
		t.Fatal("sparse source fabricated an entrant or borrowed identity", err)
	}
}

func TestP14FinalQualificationPrepublishedCompleteSourceDeadlineAndOrderedRank(t *testing.T) {
	mutations := []func(*p14FinalQualificationSnapshot){
		func(b *p14FinalQualificationSnapshot) { b.PublishedAt = b.StageStart + 1 },
		func(b *p14FinalQualificationSnapshot) { b.Sources = b.Sources[:1] },
		func(b *p14FinalQualificationSnapshot) { b.Sources[1].ServerID = 1 },
		func(b *p14FinalQualificationSnapshot) { b.Ranking[0].Score = 2 },
		func(b *p14FinalQualificationSnapshot) { b.Ranking[1] = b.Ranking[0] },
		func(b *p14FinalQualificationSnapshot) { b.Ranking[0], b.Ranking[1] = b.Ranking[1], b.Ranking[0] },
		func(b *p14FinalQualificationSnapshot) { b.Source = "unknown private rule" },
	}
	for i, mutate := range mutations {
		row, now := p14FinalPureFixture(t, 4)
		var body p14FinalQualificationSnapshot
		json.Unmarshal(row.Content, &body)
		mutate(&body)
		row.Content, _ = json.Marshal(body)
		row.ContentHash = p6Digest(row.Content)
		if _, err := p14FinalQualificationValidate(row, now); err == nil {
			t.Fatal("invalid complete source admitted", i)
		}
	}
	row, now := p14FinalPureFixture(t, 4)
	row.ObservedAt = 40000
	if _, err := p14FinalQualificationValidate(row, now); err == nil {
		t.Fatal("before actual cutoff qualification")
	}
}
