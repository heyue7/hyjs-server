package service

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func p14SeriesActualGame(t *testing.T) (*db.P14FinalGame, *db.P14FinalFormationOffer, *db.P14FinalSeriesPolicy) {
	t.Helper()
	var raw, frame []byte
	for _, root := range []string{"../../analysis/p14/final-game-actual-native", "../../../analysis/p14/final-game-actual-native"} {
		if data, err := readTestAnalysis75(filepath.Join(root, "actual-Game1-provenance.json")); err == nil {
			raw = data
			frame, err = readTestAnalysis75(filepath.Join(root, "actual-Game1-KFReplayData.pb"))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	var origin p14FinalGameProvenance
	if len(raw) == 0 || json.Unmarshal(raw, &origin) != nil {
		t.Fatal("actual157 first-input source missing")
	}
	game, err := p14FinalGameBuild(&origin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(game.Replay, frame) {
		t.Fatal("actual157 source bytes changed")
	}
	// This is a pure scheduling boundary fixture around an unchanged real input;
	// these test times are not claimed to be a published production series.
	start := origin.Policy.PublishedAt + 600000
	offer := &db.P14FinalFormationOffer{SeasonIndex: origin.SeasonIndex, RoundStage: 8, Version: "pure-series-window-v1", Source: constUtil.P14FinalFormationSource, PublishedAt: start - 1, QualificationHash: origin.QualificationHash, StageStart: start, FightStart: start + 3600000, FightEnd: start + 7*3600000, LockAt: start + 3600000 - constUtil.P14FinalFormationLockMillis}
	offer.SourceHash = p14FinalFormationOfferHash(offer)
	game.CreatedAt = offer.FightStart
	policy := &db.P14FinalSeriesPolicy{ID: 1, Enabled: true, Version: "pure-prepublished-series-v1", Source: constUtil.P14FinalSeriesSource, PublishedAt: start - 1}
	return game, offer, policy
}

// A genuine unchanged first input becomes exactly one ready slot. Elapsed time
// cannot produce results, future inputs, carry-over HP, or automatic forfeits.
func TestP14FinalSeriesPreservesActualGameOneAndWaits(t *testing.T) {
	game, offer, policy := p14SeriesActualGame(t)
	before, _ := json.Marshal(game)
	row, slots, err := p14FinalSeriesPlan(game, offer, policy, time.UnixMilli(offer.FightEnd+86400000))
	if err != nil {
		t.Fatal(err)
	}
	if row.State != constUtil.P14FinalSeriesWaitingResult || row.CurrentGame != 1 || row.LeftWins != 0 || row.RightWins != 0 || row.WinnerPlayerID != 0 || row.WinnerServerID != 0 || len(slots) != 5 {
		t.Fatal("unplayed series invented progress")
	}
	var frozen p14FinalSeriesManifest
	if json.Unmarshal(row.Manifest, &frozen) != nil || frozen.CanonicalViewerServerID != frozen.Pair.LeftServerID || frozen.CanonicalViewerPlayerID != frozen.Pair.LeftPlayerID {
		t.Fatal("canonical joint viewer was not frozen")
	}
	for index, slot := range slots {
		if slot.GameIndex != int32(index+1) || slot.ScheduledAt != offer.FightStart+int64(index)*3600000 || slot.LockAt != slot.ScheduledAt-300000 || len(slot.Certificate) != 0 || slot.CertificateHash != "" {
			t.Fatal("original count/lock or uncertified state lost")
		}
		if index == 0 {
			if slot.InputID != game.ID || !bytes.Equal(slot.Input, game.Replay) || slot.InputHash != game.ReplayHash {
				t.Fatal("first actual input changed")
			}
		} else if slot.State != constUtil.P14FinalSeriesSlotPlanned || len(slot.Input) != 0 || slot.InputID != "" {
			t.Fatal("next-game formation/HP source fabricated")
		}
	}
	after, _ := json.Marshal(game)
	if !bytes.Equal(before, after) {
		t.Fatal("old157 row mutated")
	}
}

func TestP14FinalSeriesRejectsLateOrInsufficientSchedule(t *testing.T) {
	for _, mutation := range []func(*db.P14FinalGame, *db.P14FinalFormationOffer, *db.P14FinalSeriesPolicy){
		func(g *db.P14FinalGame, o *db.P14FinalFormationOffer, p *db.P14FinalSeriesPolicy) {
			p.PublishedAt = o.StageStart + 1
		},
		func(g *db.P14FinalGame, o *db.P14FinalFormationOffer, p *db.P14FinalSeriesPolicy) { p.Enabled = false },
		func(g *db.P14FinalGame, o *db.P14FinalFormationOffer, p *db.P14FinalSeriesPolicy) {
			o.FightEnd = o.FightStart + 4*3600000
			o.SourceHash = p14FinalFormationOfferHash(o)
		},
		func(g *db.P14FinalGame, o *db.P14FinalFormationOffer, p *db.P14FinalSeriesPolicy) {
			g.Replay = append(g.Replay, 0)
		},
	} {
		game, offer, policy := p14SeriesActualGame(t)
		mutation(game, offer, policy)
		if _, _, err := p14FinalSeriesPlan(game, offer, policy, time.UnixMilli(offer.FightEnd)); err == nil {
			t.Fatal("unpublished/truncated/altered source planned")
		}
	}
}

// Abstract certified sequences test only the rules reducer, without inserting
// any real result or granting authority to those unit-test sequence values.
func TestP14FinalSeriesBestOfFiveStopsAtThirdCertifiedWin(t *testing.T) {
	for _, sequence := range [][]int32{{0, 0, 0}, {1, 0, 1, 0, 1}, {0, 1, 0, 1, 0}} {
		left, right, _, state, err := p14FinalSeriesProgress(sequence)
		if err != nil || state != constUtil.P14FinalSeriesComplete || left != 3 && right != 3 {
			t.Fatal("best-of-five terminal incorrect", err)
		}
	}
	if left, right, next, state, err := p14FinalSeriesProgress(nil); err != nil || left != 0 || right != 0 || next != 1 || state != constUtil.P14FinalSeriesWaitingResult {
		t.Fatal("no certificate counted as fight")
	}
	for _, sequence := range [][]int32{{0, 0, 0, 1}, {0, 1, 2}, {0, 1, 0, 1, 0, 1}} {
		if _, _, _, _, err := p14FinalSeriesProgress(sequence); err == nil {
			t.Fatal("post-terminal/unknown result accepted")
		}
	}
}

// This isolated domain fixture imports the already-saved real157 source; it
// does not claim a newly played qualification or series. Repeated/cold ticks
// must retain one series/five slots and zero played results indefinitely.
func TestP14FinalSeriesQueueColdKeepsUncertifiedGameOne(t *testing.T) {
	f, _, _, _ := p14Setup(t)
	game, offer, policy := p14SeriesActualGame(t)
	tx := db.GetIns().WithContext(f.ctx)
	if err := tx.Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"season_index": game.SeasonIndex, "enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(game).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(offer).Error; err != nil {
		t.Fatal(err)
	}
	at := time.UnixMilli(offer.FightStart)
	if err := f.s.QueueP14FinalSeries(f.ctx, at); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := tx.Model(&db.P14FinalSeries{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("default-off planned series", err)
	}
	if err := tx.Model(&db.P14FinalSeriesPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": policy.Version, "source": policy.Source, "published_at": policy.PublishedAt}).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if err := f.s.QueueP14FinalSeries(f.ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	var saved db.P14FinalSeries
	if err := tx.First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	manifest := append([]byte{}, saved.Manifest...)
	f.restart(t)
	tx = db.GetIns().WithContext(f.ctx)
	if err := tx.Model(&db.P14FinalSeriesPolicy{}).Where("id=1").Updates(map[string]any{"version": "changed-after-freeze", "published_at": offer.FightEnd}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.QueueP14FinalSeries(f.ctx, time.UnixMilli(offer.FightEnd+86400000)); err != nil {
		t.Fatal(err)
	}
	var cold db.P14FinalSeries
	if err := tx.First(&cold).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cold.Manifest, manifest) || cold.State != constUtil.P14FinalSeriesWaitingResult || cold.LeftWins != 0 || cold.RightWins != 0 || cold.WinnerPlayerID != 0 {
		t.Fatal("cold/late tick invented series result")
	}
	var slots []db.P14FinalSeriesSlot
	if err := tx.Where("series_id=?", cold.ID).Order("game_index").Find(&slots).Error; err != nil || len(slots) != 5 {
		t.Fatal("duplicate or missing plan", err)
	}
	for index, slot := range slots {
		if len(slot.Certificate) != 0 || index > 0 && len(slot.Input) != 0 {
			t.Fatal("elapsed time manufactured result/input")
		}
	}
	boundary, hash, err := f.s.P14FinalSeriesGuessBoundary(f.ctx, tx, cold.SeasonIndex, cold.PairHash)
	if err != nil || boundary != offer.FightStart || hash != cold.ManifestHash {
		t.Fatal("new guess could bypass actual first start", err)
	}
	var old db.P14FinalGame
	if err = tx.First(&old, "id=?", game.ID).Error; err != nil || !bytes.Equal(old.Replay, game.Replay) || old.ProvenanceHash != game.ProvenanceHash {
		t.Fatal("old157 first source changed", err)
	}
}
