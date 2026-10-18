package service

import (
	"bytes"
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"testing"
	"time"
)

func TestP14FinalSettlementIndependentWrapperPolicy(t *testing.T) {
	game, offer, scheduling := p14SeriesActualGame(t)
	row, _, err := p14FinalSeriesPlan(game, offer, scheduling, time.UnixMilli(offer.FightStart))
	if err != nil {
		t.Fatal(err)
	}
	var manifest p14FinalSeriesManifest
	if json.Unmarshal(row.Manifest, &manifest) != nil {
		t.Fatal("manifest")
	}
	original := db.P14FinalSettlementPolicy{ID: 1, ScopeProof: constUtil.P6KFCanonicalNativeProof, Enabled: true, Version: "pure-prepublished-local-wrapper-v1", Source: constUtil.P14FinalSettlementSource, PublishedAt: offer.StageStart - 1, WrapperRule: constUtil.P6KFJointViewerCompatibility, WrapperManifestHash: constUtil.P14FinalSettlementWrapperManifest, WrapperDLLHash: constUtil.P14FinalSettlementWrapperDLL}
	if !p14FinalSettlementPolicyValid(&original, &manifest) {
		t.Fatal("published wrapper policy refused")
	}
	for _, change := range []func(*db.P14FinalSettlementPolicy){
		func(p *db.P14FinalSettlementPolicy) { p.Enabled = false },
		func(p *db.P14FinalSettlementPolicy) { p.ScopeProof = constUtil.P6CrossStarterNativeProof },
		func(p *db.P14FinalSettlementPolicy) { p.PublishedAt = offer.StageStart + 1 },
		func(p *db.P14FinalSettlementPolicy) { p.WrapperRule = "" },
		func(p *db.P14FinalSettlementPolicy) { p.WrapperManifestHash = "ordinary-type17-proof" },
		func(p *db.P14FinalSettlementPolicy) { p.WrapperDLLHash = "unknown-wrapper" },
	} {
		copy := original
		change(&copy)
		if p14FinalSettlementPolicyValid(&copy, &manifest) {
			t.Fatal("unpublished or foreign wrapper accepted")
		}
	}
}

// This reuses the preserved actual157 input solely as an isolated domain source.
// No qualification, historical offer or private server played result is claimed.
func TestP14FinalSettlementActualInputOneWinOrProofPendingCold(t *testing.T) {
	f, _, series, manifest, at := p14CurrentFinalSettlementFixture(t)
	p14PerGameSwitch(t, f)
	tx := db.GetIns().WithContext(f.ctx)
	offer := &manifest.FormationOffer
	var game db.P14FinalGame
	if err := tx.First(&game, "id=?", series.GameOneID).Error; err != nil {
		t.Fatal(err)
	}
	var err error
	original := append([]byte{}, game.Replay...)
	count := func(table any) int64 {
		var n int64
		if err := tx.Model(table).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err = f.s.SettleP14FinalGameOne(f.ctx, series.ID, at); err == nil {
		t.Fatal("default-off settled")
	}
	if count(&db.P14FinalSettlement{}) != 0 || count(&db.P14FinalSettlementOffer{}) != 0 || count(&db.P6BattleRecord{}) != 0 {
		t.Fatal("closed gate left writes")
	}
	if err = tx.Model(&db.P14FinalSettlementPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "pure-actual157-canonical-v1", "published_at": offer.StageStart - 1}).Error; err != nil {
		t.Fatal(err)
	}
	// 此隔离用例仅暂换服务器归属，避免复制服务内的原子时钟缓存。
	originalServerID := f.s.ServerID
	f.s.ServerID = 2
	err = f.s.SettleP14FinalGameOne(f.ctx, series.ID, at)
	f.s.ServerID = originalServerID
	if err == nil || count(&db.P14FinalSettlement{}) != 0 || count(&db.P6BattleRecord{}) != 0 {
		t.Fatal("foreign canonical owner aliased local PID")
	}
	err = f.s.SettleP14FinalGameOne(f.ctx, series.ID, at)
	if constUtil.P6KFCanonicalNativeProof == "" {
		if err == nil || count(&db.P14FinalSettlement{}) != 0 || count(&db.P14FinalSettlementOffer{}) != 0 || count(&db.P6BattleRecord{}) != 0 {
			t.Fatal("empty proof wrote result/offer")
		}
		var state db.P14FinalSeries
		if err = tx.First(&state, "id=?", series.ID).Error; err != nil || state.CurrentGame != 1 || state.LeftWins != 0 || state.RightWins != 0 {
			t.Fatal("pending treated as played", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var result db.P14FinalSettlement
	if err = tx.First(&result).Error; err != nil {
		t.Fatal(err)
	}
	if result.WinnerServerID <= 0 || result.WinnerPlayerID != 1 || result.WinnerCamp < 0 || result.WinnerCamp > 1 {
		t.Fatal("same PID lost joint winner")
	}
	for i := 0; i < 3; i++ {
		if err = f.s.SettleP14FinalGameOne(f.ctx, series.ID, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if count(&db.P14FinalSettlement{}) != 1 {
		t.Fatal("duplicate win")
	}
	var state db.P14FinalSeries
	if err = tx.First(&state, "id=?", series.ID).Error; err != nil || state.CurrentGame != 2 || state.LeftWins+state.RightWins != 1 || state.WinnerPlayerID != 0 || state.State != constUtil.P14FinalSeriesWaitingInput {
		t.Fatal("series progress not one real certificate", err)
	}
	f.restart(t)
	tx = db.GetIns().WithContext(f.ctx)
	if err = tx.Model(&db.P14FinalSettlementPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.SettleP14FinalGameOne(f.ctx, series.ID, at.Add(24*time.Hour)); err != nil {
		t.Fatal("closed/cold accepted receipt not readable", err)
	}
	var later []db.P14FinalSeriesSlot
	if err = tx.Where("series_id=? AND game_index>1", series.ID).Find(&later).Error; err != nil {
		t.Fatal(err)
	}
	for _, slot := range later {
		if len(slot.Input) != 0 || len(slot.Certificate) != 0 {
			t.Fatal("unsealed next input or fake result")
		}
	}
	var preserved db.P14FinalGame
	if err = tx.First(&preserved, "id=?", game.ID).Error; err != nil || !bytes.Equal(preserved.Replay, original) {
		t.Fatal("old157 changed", err)
	}
}

// New actual P5 captures have today's source metadata. The qualification, draw
// and time wrappers here are explicitly finite domain fixtures; this helper is
// not evidence of a new signup/qualification. The original157 asset is untouched.
func p14CurrentFinalSettlementFixture(t *testing.T) (*p1Fixture, *p1Fixture, *db.P14FinalSeries, *p14FinalSeriesManifest, time.Time) {
	t.Helper()
	a, _, _, _ := p14Setup(t)
	a.s.ServerID = 1
	sql, _ := db.GetIns().DB()
	sql.Close()
	b, _, _, _ := p14Setup(t)
	b.s.ServerID = 2
	start := time.Now().Add(time.Minute).UnixMilli()
	qualification := p6Digest([]byte("explicit domain qualification fixture, not an actual signup"))
	offer := &db.P14FinalFormationOffer{SeasonIndex: 61, RoundStage: 8, Version: "domain-current-profile-v1", Source: constUtil.P14FinalFormationSource, PublishedAt: start - 1, QualificationHash: qualification, StageStart: start, FightStart: start + 3600000, FightEnd: start + 7*3600000, LockAt: start + 3600000 - 300000}
	offer.SourceHash = p14FinalFormationOfferHash(offer)
	members := []p14FinalSealMember{}
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		for _, init := range []func(context.Context) error{f.s.InitializeP6, f.s.InitializeP14FinalSettlement} {
			if err := init(f.ctx); err != nil {
				t.Fatal(err)
			}
		}
		player, state := f.profile(t)
		if err := f.s.RefreshP5GuildPrayerPower(f.ctx, db.GetIns(), player, state, time.UnixMilli(start+1000)); err != nil {
			t.Fatal(err)
		}
		battle, err := f.s.battlePlayer(player, state)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := f.s.p10FormationProfile(db.GetIns(), f.id)
		if err != nil {
			t.Fatal(err)
		}
		battleRaw, _ := proto.Marshal(battle)
		frameRaw, _ := proto.Marshal(frame)
		configuration, err := f.s.p14ConfigurationHash()
		if err != nil {
			t.Fatal(err)
		}
		row := db.P14FinalFormation{SeasonIndex: 61, RoundStage: 8, ServerID: f.s.OriginalServerID(), PlayerID: f.id, Revision: 1, CapturedAt: start + 1000, FightPower: frame.GetFightPower(), Snapshot: frameRaw, Battle: battleRaw, SnapshotHash: p14SnapshotDigest(frameRaw, battleRaw), ClientVersion: constUtil.P6OriginalClientHash, EngineVersion: constUtil.P6EngineVersion, ConfigurationHash: configuration, QualificationHash: qualification, OfferHash: offer.SourceHash, Source: constUtil.P14FinalFormationSource}
		row.SourceHash = p14FinalFormationHash(&row)
		metadata, _ := json.Marshal(row)
		members = append(members, p14FinalSealMember{Kind: 3, PlayerID: f.id, Metadata: metadata, Formation: frameRaw, Battle: battleRaw})
	}
	gamePolicy := db.P14FinalGamePolicy{ID: 1, Enabled: true, Version: "domain-current-first-input-v1", Source: constUtil.P14FinalGameSource, PublishedAt: start - 1}
	provenance := p14FinalGameProvenance{SeasonIndex: 61, RoundStage: 8, GameIndex: 1, Pair: p14FinalDrawPair{Group: "A", Index: 0, LeftServerID: 1, LeftPlayerID: a.id, RightServerID: 2, RightPlayerID: b.id}, DrawHash: p6Digest([]byte("explicit domain draw fixture")), QualificationHash: qualification, Policy: gamePolicy, RulesHash: p6Digest([]byte(constUtil.P14FinalGameRules)), SealHashes: []string{p6Digest(members[0].Metadata), p6Digest(members[1].Metadata)}, Left: members[0], Right: members[1]}
	game, err := p14FinalGameBuild(&provenance)
	if err != nil {
		t.Fatal(err)
	}
	game.CreatedAt = offer.FightStart
	schedule := &db.P14FinalSeriesPolicy{ID: 1, Enabled: true, Version: "domain-current-schedule-v1", Source: constUtil.P14FinalSeriesSource, PublishedAt: start - 1}
	at := time.UnixMilli(offer.FightStart + 1)
	series, slots, err := p14FinalSeriesPlan(game, offer, schedule, at)
	if err != nil {
		t.Fatal(err)
	}
	var manifest p14FinalSeriesManifest
	if json.Unmarshal(series.Manifest, &manifest) != nil {
		t.Fatal("manifest")
	}
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		tx := db.GetIns()
		if err = tx.Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "season_index": 61}).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range []any{game, offer, series, &slots} {
			if err = tx.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if f.s.OriginalServerID() == 2 {
			if err = tx.Model(&db.P14TrustedPeer{}).Where("server_id=2").Update("server_id", 1).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return a, b, series, &manifest, at
}

// Old157 bytes remain old. Distinct full-source and P6-subset hashes cannot be
// confused or rewritten to let a historical source acquire today's capability.
func TestP14FinalSettlementOldConfigurationStaysPending(t *testing.T) {
	f, series, manifest, at := p14PerGameFixture(t, 1)
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP14FinalSettlement(f.ctx); err != nil {
		t.Fatal(err)
	}
	tx := db.GetIns()
	var game db.P14FinalGame
	if err := tx.First(&game, "id=?", series.GameOneID).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(game)
	if err := tx.Model(&db.P14FinalSettlementPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "domain-legacy-reject", "published_at": manifest.FormationOffer.StageStart - 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.SettleP14FinalGameOne(f.ctx, series.ID, at); err == nil {
		t.Fatal("legacy config acquired current capability")
	}
	var count int64
	for _, table := range []any{&db.P14FinalSettlement{}, &db.P14FinalSettlementOffer{}, &db.P6BattleRecord{}} {
		if err := tx.Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("legacy rejection left writes", err)
		}
	}
	var after db.P14FinalGame
	if err := tx.First(&after, "id=?", game.ID).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(after)
	if !bytes.Equal(before, raw) {
		t.Fatal("legacy source changed")
	}
}
