package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Prepaid rows below are explicitly isolated domain fixtures. This test proves
// complete independent series association and actual P1 claim behavior; the
// separate managed HTTP test must establish a natural coin source/real debit.
func TestP14PairGuessAwardCompleteJointHistoryColdAndOriginalClaimOnce(t *testing.T) {
	a, b, series, manifest, at := p14ContinuationFixture(t)
	pair, _ := json.Marshal(manifest.Pair)
	offer := db.P14PairGuessOffer{SeasonIndex: series.SeasonIndex, PairHash: series.PairHash, BattleType: 7, Pair: pair, DrawHash: manifest.DrawHash, QualificationHash: manifest.QualificationHash, Version: "domain-paid-ordinary-v1", Source: constUtil.P14PairGuessSource, PublishedAt: manifest.FormationOffer.StageStart - 1, OpenAt: manifest.FormationOffer.StageStart, CloseAt: manifest.Slots[0], CostCoin: 10000, RevertCoin: 5000, SuccessCoin: 11000, ExtraDiamond: 10, Settlement: constUtil.P14PairGuessSettlement}
	offer.SourceHash = p14PairGuessOfferHash(&offer)
	for side, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		for _, init := range []func(context.Context) error{f.s.InitializeP14PairGuess, f.s.InitializeP14PairGuessAwards} {
			if err := init(f.ctx); err != nil {
				t.Fatal(err)
			}
		}
		guess := db.P14PairGuess{SeasonIndex: series.SeasonIndex, PairHash: series.PairHash, OwnerServerID: f.s.OriginalServerID(), OwnerPlayerID: f.id, TargetServerID: manifest.Pair.LeftServerID, TargetPlayerID: manifest.Pair.LeftPlayerID, OfferHash: offer.SourceHash, CostCoin: 10000, CoinBefore: 10000, CoinAfter: 0, CapturedAt: offer.OpenAt + 1}
		if side == 1 {
			guess.TargetServerID = manifest.Pair.RightServerID
			guess.TargetPlayerID = manifest.Pair.RightPlayerID
		}
		guess.SourceHash = p14PairGuessHash(&guess)
		if err := db.GetIns().Create(&offer).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&guess).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.s.TickP14PairGuessAwards(f.ctx, at); err != nil {
			t.Fatal(err)
		}
		var count int64
		db.GetIns().Model(&db.P14PairGuessAward{}).Count(&count)
		if count != 0 {
			t.Fatal("first real win paid a whole series")
		}
	}
	key := []byte(strings.Repeat("isolated-peer-key-", 2))
	for index := int32(2); index <= 5; index++ {
		p14PerGameSwitch(t, a)
		var current db.P14FinalSeries
		if err := db.GetIns().First(&current, "id=?", series.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.State == constUtil.P14FinalSeriesComplete {
			break
		}
		p14ContinuationCaptureSlot(t, a, b, manifest, index)
		now := time.UnixMilli(manifest.Slots[index-1] + 2)
		p14PerGameSwitch(t, a)
		if err := a.s.SettleP14FinalContinuation(a.ctx, series.ID, now); err != nil {
			t.Fatal(err)
		}
		if err := a.s.QueueP14ContinuationResults(a.ctx, now); err != nil {
			t.Fatal(err)
		}
		var delivery db.P14FinalContinuationDelivery
		if err := db.GetIns().First(&delivery, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			t.Fatal(err)
		}
		var request model.P14ContinuationResultImportRequest
		if err := json.Unmarshal(delivery.Payload, &request); err != nil {
			t.Fatal(err)
		}
		p14PerGameSwitch(t, b)
		b.ctx = context.WithValue(b.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return now.Add(time.Second) })
		if _, err := b.s.ImportP14ContinuationResult(b.ctx, &request, delivery.Payload, P14SignPayload(key, delivery.Payload)); err != nil {
			t.Fatal(err)
		}
	}
	p14PerGameSwitch(t, b)
	var final db.P14FinalSeries
	if err := db.GetIns().First(&final, "id=?", series.ID).Error; err != nil {
		t.Fatal(err)
	}
	// This exact remote certificate is unchanged. Alter only the slot's replay
	// and its self-hash; the added cross-binding must reject zero reward.
	var slot db.P14FinalSeriesSlot
	if err := db.GetIns().First(&slot, "series_id=? AND game_index=1", series.ID).Error; err != nil {
		t.Fatal(err)
	}
	oldInput := append([]byte{}, slot.Input...)
	oldHash := slot.InputHash
	replay := &pb.KFReplayData{}
	if proto.Unmarshal(slot.Input, replay) != nil {
		t.Fatal("original slot")
	}
	replay.RandSeed = proto.Int64(replay.GetRandSeed() + 1)
	changed, _ := proto.Marshal(replay)
	if err := db.GetIns().Model(&db.P14FinalSeriesSlot{}).Where("series_id=? AND game_index=1", series.ID).Updates(map[string]any{"input": changed, "input_hash": p6Digest(changed)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p14PairGuessCertifiedSeriesTX(db.GetIns(), &final, &offer); err == nil {
		t.Fatal("remote certificate detached from slot accepted")
	}
	db.GetIns().Model(&db.P14FinalSeriesSlot{}).Where("series_id=? AND game_index=1", series.ID).Updates(map[string]any{"input": oldInput, "input_hash": oldHash})
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		f.restart(t)
		if err := db.GetIns().First(&final, "id=?", series.ID).Error; err != nil {
			t.Fatal(err)
		}
		paidAt := time.UnixMilli(final.UpdatedAt + 1000)
		before, _ := f.profile(t)
		beforeCoin, beforeDiamond := before.GetCoin(), before.GetDiamond()
		// Current catalogs can close; the genuine frozen paid terms remain due.
		db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Update("enabled", false)
		db.GetIns().Model(&db.P14PairGuessPolicy{}).Where("id=1").Update("enabled", false)
		f.ctx = context.WithValue(f.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return paidAt })
		for n := 0; n < 3; n++ {
			if err := f.s.TickP14PairGuessAwards(f.ctx, time.UnixMilli(manifest.Slots[0]-1)); err != nil {
				t.Fatal(err)
			}
		}
		var award db.P14PairGuessAward
		if err := db.GetIns().First(&award).Error; err != nil {
			t.Fatal(err)
		}
		expectedWon := f.s.OriginalServerID() == final.WinnerServerID
		expectedCoin, expectedDiamond := int64(5000), int64(0)
		if expectedWon {
			expectedCoin, expectedDiamond = 11000, 10
		}
		if award.SelectedWon != expectedWon || award.Coin != expectedCoin || award.Diamond != expectedDiamond || award.WinnerServerID != final.WinnerServerID || award.WinnerPlayerID != final.WinnerPlayerID {
			t.Fatal("samePID joint payout or original terms", award)
		}
		afterEnqueue, _ := f.profile(t)
		if afterEnqueue.GetCoin() != beforeCoin || afterEnqueue.GetDiamond() != beforeDiamond {
			t.Fatal("enqueue directly changed wallet")
		}
		if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{award.RewardID}}); err != nil {
			t.Fatal(err)
		}
		claimed, _ := f.profile(t)
		if claimed.GetCoin() != beforeCoin+expectedCoin || claimed.GetDiamond() != beforeDiamond+expectedDiamond {
			t.Fatal("original actual claim amount")
		}
		if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{award.RewardID}}); err != nil {
			t.Fatal(err)
		}
		again, _ := f.profile(t)
		if again.GetCoin() != claimed.GetCoin() || again.GetDiamond() != claimed.GetDiamond() {
			t.Fatal("duplicate original claim")
		}
		var guess db.P14PairGuess
		if err := db.GetIns().First(&guess).Error; err != nil {
			t.Fatal(err)
		}
		closure, err := p14PairGuessCertifiedSeriesTX(db.GetIns(), &final, &offer)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.s.p14PayPairGuessTX(db.GetIns(), &guess, &offer, &final, closure, paidAt); err != nil {
			t.Fatal("claimed immutable history rejected", err)
		}
		var count int64
		db.GetIns().Model(&db.P14PairGuessAward{}).Count(&count)
		if count != 1 {
			t.Fatal("duplicate payoutjournal", count)
		}
	}
}
