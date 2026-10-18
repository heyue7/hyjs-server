package service

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14ContinuationFixture(t *testing.T) (*p1Fixture, *p1Fixture, *db.P14FinalSeries, *p14FinalSeriesManifest, time.Time) {
	t.Helper()
	a, b, series, manifest, at := p14CurrentFinalSettlementFixture(t)
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		for _, init := range []func(context.Context) error{f.s.InitializeP14FinalResult, f.s.InitializeP14PerGame, f.s.InitializeP14FinalContinuation} {
			if err := init(f.ctx); err != nil {
				t.Fatal(err)
			}
		}
		for _, table := range []any{&db.P14FinalSettlementPolicy{}, &db.P14FinalResultPolicy{}, &db.P14PerGamePolicy{}, &db.P14FinalContinuationPolicy{}} {
			if err := db.GetIns().Model(table).Where("id=1").Updates(map[string]any{"enabled": true, "version": "prepublished-current-actual-profile-domain-v1", "published_at": manifest.FormationOffer.StageStart - 1}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	p14PerGameSwitch(t, a)
	if err := a.s.SettleP14FinalGameOne(a.ctx, series.ID, at); err != nil {
		t.Fatal(err)
	}
	if err := a.s.QueueP14FinalResults(a.ctx, at); err != nil {
		t.Fatal(err)
	}
	var delivery db.P14FinalResultDelivery
	if err := db.GetIns().First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	p14PerGameSwitch(t, b)
	var request model.P14FinalResultImportRequest
	if err := json.Unmarshal(delivery.Payload, &request); err != nil {
		t.Fatal(err)
	}
	b.ctx = context.WithValue(b.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return at.Add(time.Second) })
	key := []byte(strings.Repeat("isolated-peer-key-", 2))
	if _, err := b.s.ImportP14FinalResult(b.ctx, &request, delivery.Payload, P14SignPayload(key, delivery.Payload)); err != nil {
		t.Fatal(err)
	}
	return a, b, series, manifest, at
}

// Genuine current P5 profiles are independently recaptured through81030's
// existing producer for every slot. The qualification/draw here are explicit
// domain fixtures, while the managed original-HTTP runner covers the real flow.
func p14ContinuationCaptureSlot(t *testing.T, a, b *p1Fixture, manifest *p14FinalSeriesManifest, index int32) {
	t.Helper()
	at := time.UnixMilli(manifest.Slots[index-2] + 1000)
	now := time.UnixMilli(manifest.Slots[index-1] + 1)
	var payloads [2][][]byte
	for source, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		if err := p14PerGameCapture(t, f, at); err != nil {
			t.Fatal("actual own upload", index, err)
		}
		if err := f.s.QueueP14PerGameSeals(f.ctx, time.UnixMilli(manifest.Slots[index-1]-300000)); err != nil {
			t.Fatal(err)
		}
		var rows []db.P14PerGameDelivery
		if err := db.GetIns().Order("created_at,id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			var envelope model.P14PerGameImportRequest
			var body p14PerGameSnapshot
			if json.Unmarshal(row.Payload, &envelope) != nil || json.Unmarshal(envelope.Snapshot, &body) != nil {
				t.Fatal("source envelope")
			}
			if body.Offer.GameIndex == index {
				payloads[source] = append(payloads[source], row.Payload)
			}
		}
	}
	key := []byte(strings.Repeat("isolated-peer-key-", 2))
	for source := range payloads {
		target := []*p1Fixture{b, a}[source]
		p14PerGameSwitch(t, target)
		target.ctx = context.WithValue(target.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return now })
		for _, raw := range payloads[source] {
			var request model.P14PerGameImportRequest
			if json.Unmarshal(raw, &request) != nil {
				t.Fatal("envelope")
			}
			if _, err := target.s.ImportP14PerGame(target.ctx, request, raw, P14SignPayload(key, raw)); err != nil {
				t.Fatal("actual sealed source import", index, err)
			}
		}
	}
}

func TestP14FinalContinuationActualFreshSlotsUntilThreeWinsSignedPeerCold(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("two-file fixture; managed HTTP flow is separate")
	}
	if constUtil.P6KFCanonicalNativeProof == "" {
		t.Skip("independent KF native proof required")
	}
	a, b, series, manifest, _ := p14ContinuationFixture(t)
	key := []byte(strings.Repeat("isolated-peer-key-", 2))
	var source0 []byte
	var priorID string
	var played int32 = 1
	for index := int32(2); index <= 5; index++ {
		p14PerGameSwitch(t, a)
		var before db.P14FinalSeries
		if err := db.GetIns().First(&before, "id=?", series.ID).Error; err != nil {
			t.Fatal(err)
		}
		if before.State == constUtil.P14FinalSeriesComplete {
			break
		}
		// The gate is default-off. Even with a genuine preceding certificate it
		// must refuse current-slot generation until the new rule is published.
		if index == 2 {
			if err := db.GetIns().Model(&db.P14FinalContinuationPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.s.SettleP14FinalContinuation(a.ctx, series.ID, time.UnixMilli(manifest.Slots[1])); err == nil {
				t.Fatal("closed continuation gate")
			}
			var count int64
			db.GetIns().Model(&db.P14FinalContinuationInput{}).Count(&count)
			if count != 0 {
				t.Fatal("closed gate produced an input")
			}
			db.GetIns().Model(&db.P14FinalContinuationPolicy{}).Where("id=1").Update("enabled", true)
		}
		p14ContinuationCaptureSlot(t, a, b, manifest, index)
		now := time.UnixMilli(manifest.Slots[index-1] + 2)
		p14PerGameSwitch(t, a)
		if index == 2 {
			var first db.P14FinalGame
			if err := db.GetIns().First(&first).Error; err != nil {
				t.Fatal(err)
			}
			source0 = append([]byte{}, first.Replay...)
			priorID = first.ID
		}
		if err := a.s.SettleP14FinalContinuation(a.ctx, series.ID, now); err != nil {
			t.Fatal("canonical next game", index, err)
		}
		if err := a.s.QueueP14ContinuationResults(a.ctx, now); err != nil {
			t.Fatal(err)
		}
		var input db.P14FinalContinuationInput
		if err := db.GetIns().First(&input, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			t.Fatal(err)
		}
		if input.ID == priorID || bytes.Equal(input.Replay, source0) || len(input.PreviousResultHash) != 64 {
			t.Fatal("reused earlier input")
		}
		var provenance p14ContinuationProvenance
		if json.Unmarshal(input.Provenance, &provenance) != nil || len(provenance.Sources) != 2 || provenance.Sources[0].GameIndex != index || provenance.Sources[1].GameIndex != index {
			t.Fatal("wrong source slot")
		}
		replay := &pb.KFReplayData{}
		if proto.Unmarshal(input.Replay, replay) != nil || replay.GetWinPlayerId() != 0 {
			t.Fatal("claimed input winner")
		}
		for actor, source := range provenance.Sources {
			original := &pb.BattlePlayer{}
			if proto.Unmarshal(source.Battle, original) != nil || !proto.Equal(original, replay.PlayerList[actor]) {
				t.Fatal("modified actual fresh HP/AP/formation")
			}
		}
		priorID = input.ID
		var delivery db.P14FinalContinuationDelivery
		if err := db.GetIns().First(&delivery, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			t.Fatal(err)
		}
		var accepted db.P14FinalContinuationResult
		if err := db.GetIns().First(&accepted, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			t.Fatal(err)
		}
		p14PerGameSwitch(t, b)
		b.ctx = context.WithValue(b.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return now.Add(time.Second) })
		var request model.P14ContinuationResultImportRequest
		if json.Unmarshal(delivery.Payload, &request) != nil {
			t.Fatal("envelope")
		}
		if _, err := b.s.ImportP14ContinuationResult(b.ctx, &request, delivery.Payload, strings.Repeat("0", 64)); err == nil {
			t.Fatal("bad source signature")
		}
		for retry := 0; retry < 3; retry++ {
			reply, err := b.s.ImportP14ContinuationResult(b.ctx, &request, delivery.Payload, P14SignPayload(key, delivery.Payload))
			if err != nil || !reply.Confirmed {
				t.Fatal("signed actual terminal", index, err)
			}
		}
		var peerInput db.P14FinalContinuationInput
		if err := db.GetIns().First(&peerInput, "series_id=? AND game_index=?", series.ID, index).Error; err != nil || peerInput.ID != input.ID || peerInput.PreviousResultHash != input.PreviousResultHash {
			t.Fatal("peer seed/input differs", err)
		}
		var receipt db.P14FinalContinuationReceipt
		if err := db.GetIns().First(&receipt, "series_id=? AND game_index=?", series.ID, index).Error; err != nil || receipt.SourceResultHash != accepted.SourceHash {
			t.Fatal("lost canonical source result", err)
		}
		var noP6 int64
		db.GetIns().Model(&db.P6BattleRecord{}).Count(&noP6)
		if noP6 != 0 {
			t.Fatal("peer invented foreign P6 owner")
		}
		played = index
	}
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		f.restart(t)
		var final db.P14FinalSeries
		if err := db.GetIns().First(&final, "id=?", series.ID).Error; err != nil {
			t.Fatal(err)
		}
		if final.State != constUtil.P14FinalSeriesComplete || final.LeftWins+final.RightWins != played || final.LeftWins != 3 && final.RightWins != 3 || final.WinnerServerID == 0 || final.WinnerPlayerID == 0 {
			t.Fatal("not exactly three real wins", final)
		}
		winners, _, err := p14ContinuationHistoryTX(db.GetIns(), &final, manifest, played)
		if err != nil || len(winners) != int(played) {
			t.Fatal("cold certificate history", err)
		}
		var later []db.P14FinalSeriesSlot
		if err := db.GetIns().Where("series_id=? AND game_index>?", series.ID, played).Find(&later).Error; err != nil {
			t.Fatal(err)
		}
		for _, slot := range later {
			if slot.State != constUtil.P14FinalSeriesSlotUnneeded || len(slot.Input) != 0 || len(slot.Certificate) != 0 {
				t.Fatal("unplayed slot masquerades as winner")
			}
		}
		db.GetIns().Model(&db.P14FinalContinuationPolicy{}).Where("id=1").Update("enabled", false)
		var immutable db.P14FinalGame
		if err := db.GetIns().First(&immutable).Error; err != nil || !bytes.Equal(immutable.Replay, source0) {
			t.Fatal("original157 source mutated", err)
		}
	}
}
