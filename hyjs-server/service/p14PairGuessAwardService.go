package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP14PairGuessAwards adds only an idempotent paid-award journal.
// New bets retain their original default-off policy. Existing accepted terms
// are paid only after their exact full independent series proves a winner.
func (s *GameService) InitializeP14PairGuessAwards(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P14PairGuessAward{})
}

func p14PairGuessAwardHash(row *db.P14PairGuessAward) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// Mutable claim fields belong to P1. Freeze only the original delivery identity
// and promised attachment; a later real claim must not invalidate paid history.
func p14PairGuessRewardHash(row *db.RewardDelivery) string {
	raw, _ := json.Marshal(struct {
		ID        int64
		PlayerID  int64
		SourceKey string
		Title     string
		Content   string
		Rewards   string
		CreatedAt int64
		ExpiresAt int64
	}{row.ID, row.PlayerID, row.SourceKey, row.Title, row.Content, row.Rewards, row.CreatedAt, row.ExpiresAt})
	return p6Digest(raw)
}

type p14PairGuessSeriesClosure struct {
	SeriesID       string   `json:"seriesId"`
	ManifestHash   string   `json:"manifestHash"`
	SourceResults  []string `json:"sourceResults"`
	WinnerServerID int64    `json:"winnerServerId"`
	WinnerPlayerID int64    `json:"winnerPlayerId"`
}

// p14PairGuessCertifiedSeriesTX binds both remote and local terminal provenance
// to the exact slot bytes, then independently reduces three actual wins. A
// first-round winner remains a first-round winner; no champion is inferred.
func p14PairGuessCertifiedSeriesTX(tx *gorm.DB, series *db.P14FinalSeries, offer *db.P14PairGuessOffer) (string, error) {
	reject := func() (string, error) { return "", errors.New(constUtil.P14PairGuessAwardConflict) }
	if series == nil || offer == nil || p14PairGuessReadOffer(offer) != nil || series.State != constUtil.P14FinalSeriesComplete || series.RoundStage != constUtil.P14Final16Stage || series.SeasonIndex != offer.SeasonIndex || series.PairHash != offer.PairHash {
		return reject()
	}
	manifest, err := p14PerGameReadSeries(series)
	if err != nil {
		return "", err
	}
	pairRaw, err := json.Marshal(manifest.Pair)
	if err != nil || !bytes.Equal(pairRaw, offer.Pair) || manifest.DrawHash != offer.DrawHash || manifest.QualificationHash != offer.QualificationHash || len(manifest.Slots) != 5 || offer.CloseAt > manifest.Slots[0] {
		return reject()
	}
	count := series.LeftWins + series.RightWins
	winners, _, err := p14ContinuationHistoryTX(tx, series, manifest, count)
	if err != nil {
		return "", err
	}
	left, right, next, state, err := p14FinalSeriesProgress(winners)
	if err != nil || left != series.LeftWins || right != series.RightWins || next != series.CurrentGame || state != constUtil.P14FinalSeriesComplete {
		return reject()
	}
	winnerServer, winnerPlayer := manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID
	if right == constUtil.P14FinalSeriesWinsRequired {
		winnerServer, winnerPlayer = manifest.Pair.RightServerID, manifest.Pair.RightPlayerID
	}
	if series.WinnerServerID != winnerServer || series.WinnerPlayerID != winnerPlayer {
		return reject()
	}
	closure := p14PairGuessSeriesClosure{SeriesID: series.ID, ManifestHash: series.ManifestHash, WinnerServerID: winnerServer, WinnerPlayerID: winnerPlayer}
	for index := int32(1); index <= count; index++ {
		var slot db.P14FinalSeriesSlot
		if err = tx.First(&slot, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			return "", err
		}
		replay := &pb.KFReplayData{}
		if proto.Unmarshal(slot.Input, replay) != nil || slot.InputHash != p6Digest(slot.Input) || slot.CertificateHash != p6Digest(slot.Certificate) {
			return reject()
		}
		commonHash := ""
		if index == 1 {
			var local db.P14FinalSettlement
			err = tx.First(&local, "series_id=? AND game_index=1", series.ID).Error
			if err == nil {
				if local.InputID != slot.InputID || local.InputHash != slot.InputHash || local.ProvenanceHash != slot.ProvenanceHash {
					return reject()
				}
				commonHash = local.SourceHash
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				var remote db.P14FinalResultReceipt
				if err = tx.First(&remote, "series_id=? AND game_index=1", series.ID).Error; err != nil {
					return "", err
				}
				input, _, e := p14ReadFinalResultReceipt(&remote)
				if e != nil {
					return "", e
				}
				var source db.P14FinalSettlement
				if json.Unmarshal(remote.SourceResult, &source) != nil || remote.SourceServerID != manifest.CanonicalViewerServerID || source.SeriesID != series.ID || source.GameIndex != index || source.InputID != slot.InputID || source.InputHash != slot.InputHash || source.ProvenanceHash != slot.ProvenanceHash || input == nil || !proto.Equal(input.KFReplay, replay) {
					return reject()
				}
				commonHash = remote.SourceResultHash
			} else {
				return "", err
			}
		} else {
			var local db.P14FinalContinuationResult
			err = tx.First(&local, "series_id=? AND game_index=?", series.ID, index).Error
			if err == nil {
				if local.InputID != slot.InputID || local.InputHash != slot.InputHash || local.ProvenanceHash != slot.ProvenanceHash {
					return reject()
				}
				commonHash = local.SourceHash
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				var remote db.P14FinalContinuationReceipt
				if err = tx.First(&remote, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
					return "", err
				}
				input, _, e := p14ReadContinuationReceipt(&remote)
				if e != nil {
					return "", e
				}
				var source db.P14FinalContinuationResult
				if json.Unmarshal(remote.SourceResult, &source) != nil || remote.SourceServerID != manifest.CanonicalViewerServerID || source.SeriesID != series.ID || source.GameIndex != index || source.InputID != slot.InputID || source.InputHash != slot.InputHash || source.ProvenanceHash != slot.ProvenanceHash || input == nil || !proto.Equal(input.KFReplay, replay) {
					return reject()
				}
				commonHash = remote.SourceResultHash
			} else {
				return "", err
			}
		}
		if len(commonHash) != 64 {
			return reject()
		}
		closure.SourceResults = append(closure.SourceResults, commonHash)
	}
	// Any remaining scheduled slot is explicitly unused, with no invented input,
	// certificate or extra game after the first side's actual third win.
	for index := count + 1; index <= constUtil.P14FinalSeriesMaxGames; index++ {
		var slot db.P14FinalSeriesSlot
		if err = tx.First(&slot, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			return "", err
		}
		if slot.State != constUtil.P14FinalSeriesSlotUnneeded || len(slot.Certificate) != 0 || slot.CertificateHash != "" {
			return reject()
		}
	}
	raw, err := json.Marshal(closure)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

func p14PairGuessAwardTerms(guess *db.P14PairGuess, offer *db.P14PairGuessOffer, series *db.P14FinalSeries) (int64, int64, bool, error) {
	if guess == nil || offer == nil || series == nil || p14PairGuessReadOffer(offer) != nil || guess.SourceHash != p14PairGuessHash(guess) || guess.OfferHash != offer.SourceHash || guess.SeasonIndex != offer.SeasonIndex || guess.PairHash != offer.PairHash || guess.OwnerPlayerID <= 0 || guess.OwnerServerID <= 0 || guess.CostCoin != offer.CostCoin || guess.CoinBefore < guess.CostCoin || guess.CoinAfter != guess.CoinBefore-guess.CostCoin || guess.CapturedAt < offer.OpenAt || guess.CapturedAt >= offer.CloseAt {
		return 0, 0, false, errors.New(constUtil.P14PairGuessAwardConflict)
	}
	var pair p14FinalDrawPair
	if json.Unmarshal(offer.Pair, &pair) != nil || !(guess.TargetServerID == pair.LeftServerID && guess.TargetPlayerID == pair.LeftPlayerID || guess.TargetServerID == pair.RightServerID && guess.TargetPlayerID == pair.RightPlayerID) {
		return 0, 0, false, errors.New(constUtil.P14PairGuessAwardConflict)
	}
	won := guess.TargetServerID == series.WinnerServerID && guess.TargetPlayerID == series.WinnerPlayerID
	if won {
		return offer.SuccessCoin, offer.ExtraDiamond, true, nil
	}
	return offer.RevertCoin, 0, false, nil
}

func p14PairGuessRewardString(coin, diamond int64) string {
	reward := fmt.Sprintf("coin,%d", coin)
	if diamond > 0 {
		reward += fmt.Sprintf(";diamond,%d", diamond)
	}
	return reward
}

func (s *GameService) p14PayPairGuessTX(tx *gorm.DB, guess *db.P14PairGuess, offer *db.P14PairGuessOffer, series *db.P14FinalSeries, closure string, now time.Time) error {
	coin, diamond, won, err := p14PairGuessAwardTerms(guess, offer, series)
	if err != nil {
		return err
	}
	if guess.OwnerServerID != s.OriginalServerID() || len(closure) != 64 || now.UnixMilli() < series.UpdatedAt {
		return errors.New(constUtil.P14PairGuessAwardConflict)
	}
	sourceKey := fmt.Sprintf("%s%d:%s", constUtil.P14PairGuessAwardRewardPrefix, guess.SeasonIndex, guess.PairHash)
	var old db.P14PairGuessAward
	err = tx.First(&old, "season_index=? AND pair_hash=? AND owner_server_id=? AND owner_player_id=?", guess.SeasonIndex, guess.PairHash, guess.OwnerServerID, guess.OwnerPlayerID).Error
	if err == nil {
		if old.SourceHash != p14PairGuessAwardHash(&old) || old.GuessHash != guess.SourceHash || old.OfferHash != offer.SourceHash || old.SeriesID != series.ID || old.SeriesManifestHash != series.ManifestHash || old.SeriesClosureHash != closure || old.WinnerServerID != series.WinnerServerID || old.WinnerPlayerID != series.WinnerPlayerID || old.Coin != coin || old.Diamond != diamond || old.SelectedWon != won || old.Source != constUtil.P14PairGuessAwardSource {
			return errors.New(constUtil.P14PairGuessAwardConflict)
		}
		var reward db.RewardDelivery
		if err = tx.First(&reward, old.RewardID).Error; err != nil {
			return err
		}
		if reward.PlayerID != guess.OwnerPlayerID || reward.SourceKey != sourceKey || reward.Rewards != p14PairGuessRewardString(coin, diamond) || old.RewardHash != p14PairGuessRewardHash(&reward) {
			return errors.New(constUtil.P14PairGuessAwardConflict)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// The original reward-center delivery and new paid journal commit together.
	// Current season/betting switches do not erase a prior real paid promise.
	var owner db.Profile
	if err = tx.First(&owner, guess.OwnerPlayerID).Error; err != nil {
		return err
	}
	if owner.Player == nil {
		return errors.New(constUtil.P14PairGuessAwardConflict)
	}
	delivery, err := s.EnqueueReward(tx, guess.OwnerPlayerID, sourceKey, constUtil.P14PairGuessAwardTitle, constUtil.P14PairGuessAwardContent, p14PairGuessRewardString(coin, diamond), 0)
	if err != nil {
		return err
	}
	if delivery.PlayerID != guess.OwnerPlayerID || delivery.SourceKey != sourceKey || delivery.Title != constUtil.P14PairGuessAwardTitle || delivery.Content != constUtil.P14PairGuessAwardContent || delivery.Rewards != p14PairGuessRewardString(coin, diamond) || delivery.ExpiresAt != 0 {
		return errors.New(constUtil.P14PairGuessAwardConflict)
	}
	row := db.P14PairGuessAward{SeasonIndex: guess.SeasonIndex, PairHash: guess.PairHash, OwnerServerID: guess.OwnerServerID, OwnerPlayerID: guess.OwnerPlayerID, GuessHash: guess.SourceHash, OfferHash: offer.SourceHash, SeriesID: series.ID, SeriesManifestHash: series.ManifestHash, SeriesClosureHash: closure, WinnerServerID: series.WinnerServerID, WinnerPlayerID: series.WinnerPlayerID, SelectedWon: won, Coin: coin, Diamond: diamond, RewardID: delivery.ID, RewardHash: p14PairGuessRewardHash(delivery), Source: constUtil.P14PairGuessAwardSource, CreatedAt: now.UnixMilli()}
	row.SourceHash = p14PairGuessAwardHash(&row)
	return tx.Create(&row).Error
}

// TickP14PairGuessAwards shares the existing cross worker. Read candidates
// outside the transaction, then serialize each source using its established
// season lock and exact complete-series current row. Only local real bettors
// receive rewards; cross-realm PID equality never aliases an external owner.
func (s *GameService) TickP14PairGuessAwards(ctx context.Context, now time.Time) error {
	// The existing isolated business clock is internal context only. P1 enqueue/claim keeps its own wall clock.
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	if !s.P1Ready {
		return nil
	}
	var (
		lastSeason   int32
		lastPair     string
		lastPlayer   int64
		firstFailure error
		failures     int
	)
	// Advance a stable keyset even when a damaged candidate remains unpaid.
	// Each owner has its own transaction; later legitimate bets keep moving.
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(firstFailure, err)
		}
		var rows []db.P14PairGuess
		query := db.GetIns().WithContext(ctx).Table("p14_pair_guesses AS g").Select("g.*").Joins("JOIN p14_final_series AS s ON s.season_index=g.season_index AND s.pair_hash=g.pair_hash AND s.state=?", constUtil.P14FinalSeriesComplete).Joins("LEFT JOIN p14_pair_guess_awards AS a ON a.season_index=g.season_index AND a.pair_hash=g.pair_hash AND a.owner_server_id=g.owner_server_id AND a.owner_player_id=g.owner_player_id").Where("g.owner_server_id=? AND a.reward_id IS NULL", s.OriginalServerID()).Order("g.season_index,g.pair_hash,g.owner_player_id").Limit(constUtil.P14PairGuessAwardBatch)
		if lastSeason > 0 {
			query = query.Where("(g.season_index,g.pair_hash,g.owner_player_id) > (?,?,?)", lastSeason, lastPair, lastPlayer)
		}
		if err := query.Find(&rows).Error; err != nil {
			return errors.Join(firstFailure, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, candidate := range rows {
			lastSeason, lastPair, lastPlayer = candidate.SeasonIndex, candidate.PairHash, candidate.OwnerPlayerID
			err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := ServerGameClockGuardTransaction(tx); err != nil {
					return err
				}

				var lock db.P14SeasonPolicy
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, 1).Error; err != nil {
					return err
				}
				var guess db.P14PairGuess
				if err := tx.First(&guess, "season_index=? AND pair_hash=? AND owner_server_id=? AND owner_player_id=?", candidate.SeasonIndex, candidate.PairHash, candidate.OwnerServerID, candidate.OwnerPlayerID).Error; err != nil {
					return err
				}
				var offer db.P14PairGuessOffer
				if err := tx.First(&offer, "season_index=? AND pair_hash=?", guess.SeasonIndex, guess.PairHash).Error; err != nil {
					return err
				}
				var series db.P14FinalSeries
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&series, "season_index=? AND pair_hash=?", guess.SeasonIndex, guess.PairHash).Error; err != nil {
					return err
				}
				closure, err := p14PairGuessCertifiedSeriesTX(tx, &series, &offer)
				if err != nil {
					return err
				}
				return s.p14PayPairGuessTX(tx, &guess, &offer, &series, closure, now)
			})
			if err != nil {
				failures++
				if firstFailure == nil {
					firstFailure = fmt.Errorf("ordinary guess season%d pair%s player%d: %w", candidate.SeasonIndex, candidate.PairHash, candidate.OwnerPlayerID, err)
				}
			}
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d ordinary paid guesses remain pending: %w", failures, firstFailure)
	}
	return nil
}
