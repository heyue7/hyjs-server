package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// p14FinalSeriesManifest freezes the existing first input and the complete
// published timing rule. Later inputs require their own real formation seal.
type p14FinalSeriesManifest struct {
	Policy                  db.P14FinalSeriesPolicy   `json:"policy"`
	Rules                   string                    `json:"rules"`
	FirstInputID            string                    `json:"firstInputId"`
	FirstInputHash          string                    `json:"firstInputHash"`
	FirstProvenanceHash     string                    `json:"firstProvenanceHash"`
	DrawHash                string                    `json:"drawHash"`
	QualificationHash       string                    `json:"qualificationHash"`
	Pair                    p14FinalDrawPair          `json:"pair"`
	CanonicalViewerServerID int64                     `json:"canonicalViewerServerId"`
	CanonicalViewerPlayerID int64                     `json:"canonicalViewerPlayerId"`
	FormationOffer          db.P14FinalFormationOffer `json:"formationOffer"`
	Slots                   []int64                   `json:"slots"`
}

// First-game157 rows remain byte-identical. The new independent series row
// merely references them and preserves one copy in slot1 for recovery.
func p14FinalSeriesPlan(game *db.P14FinalGame, offer *db.P14FinalFormationOffer, policy *db.P14FinalSeriesPolicy, now time.Time) (*db.P14FinalSeries, []db.P14FinalSeriesSlot, error) {
	if game == nil || offer == nil || policy == nil || !policy.Enabled || policy.Source != constUtil.P14FinalSeriesSource || strings.TrimSpace(policy.Version) == "" || len(policy.Version) > 128 || policy.PublishedAt <= 0 || policy.PublishedAt > offer.StageStart || game.GameIndex != 1 || game.RoundStage != constUtil.P14Final16Stage || game.SeasonIndex != offer.SeasonIndex || game.CreatedAt <= 0 || game.CreatedAt > now.UnixMilli() || game.ReplayHash != p6Digest(game.Replay) || game.ProvenanceHash != p6Digest(game.Provenance) {
		return nil, nil, errors.New(constUtil.P14FinalSeriesCorrupt)
	}
	if err := p14FinalFormationValidateOffer(offer); err != nil {
		return nil, nil, err
	}
	var origin p14FinalGameProvenance
	if p14PromotionRankDecode(game.Provenance, &origin) != nil || origin.SeasonIndex != game.SeasonIndex || origin.RoundStage != game.RoundStage || origin.GameIndex != 1 || origin.Pair.Group != game.GroupID || origin.Pair.Index != game.PairIndex || origin.QualificationHash != offer.QualificationHash {
		return nil, nil, errors.New(constUtil.P14FinalSeriesCorrupt)
	}
	rebuilt, err := p14FinalGameBuild(&origin)
	if err != nil || rebuilt.ID != game.ID || rebuilt.ReplayHash != game.ReplayHash || rebuilt.ProvenanceHash != game.ProvenanceHash {
		return nil, nil, errors.New(constUtil.P14FinalSeriesCorrupt)
	}
	manifest := p14FinalSeriesManifest{Policy: *policy, Rules: constUtil.P14FinalSeriesRules, FirstInputID: game.ID, FirstInputHash: game.ReplayHash, FirstProvenanceHash: game.ProvenanceHash, DrawHash: origin.DrawHash, QualificationHash: origin.QualificationHash, Pair: origin.Pair, FormationOffer: *offer, CanonicalViewerServerID: origin.Pair.LeftServerID, CanonicalViewerPlayerID: origin.Pair.LeftPlayerID}
	var slots []db.P14FinalSeriesSlot
	for index := int32(1); index <= constUtil.P14FinalSeriesMaxGames; index++ {
		scheduled := offer.FightStart + int64(index-1)*constUtil.P14FinalSeriesIntervalMillis
		if scheduled < offer.FightStart || scheduled >= offer.FightEnd {
			return nil, nil, errors.New(constUtil.P14FinalSeriesPending)
		}
		manifest.Slots = append(manifest.Slots, scheduled)
		slot := db.P14FinalSeriesSlot{GameIndex: index, ScheduledAt: scheduled, LockAt: scheduled - constUtil.P14FinalFormationLockMillis, State: constUtil.P14FinalSeriesSlotPlanned, Input: []byte{}, Certificate: []byte{}}
		if index == 1 {
			slot.State = constUtil.P14FinalSeriesSlotInputReady
			slot.InputID = game.ID
			slot.Input = append([]byte{}, game.Replay...)
			slot.InputHash = game.ReplayHash
			slot.ProvenanceHash = game.ProvenanceHash
		}
		slots = append(slots, slot)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	id := p6Digest(append([]byte("p14/final-series/"), raw...))
	pairRaw, _ := json.Marshal(origin.Pair)
	row := &db.P14FinalSeries{ID: id, SeasonIndex: game.SeasonIndex, RoundStage: game.RoundStage, GroupID: game.GroupID, PairIndex: game.PairIndex, PairHash: p6Digest(pairRaw), GameOneID: game.ID, Manifest: raw, ManifestHash: p6Digest(raw), State: constUtil.P14FinalSeriesWaitingResult, CurrentGame: 1, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
	for index := range slots {
		slots[index].SeriesID = id
	}
	return row, slots, nil
}

// This reducer describes best-of-five only after each caller-provided item has
// independently passed the future KF certificate verifier. It performs no DB
// write and has no producer accepting a caller's claimed winner or authority.
func p14FinalSeriesProgress(certified []int32) (left, right, next, state int32, err error) {
	state = constUtil.P14FinalSeriesWaitingResult
	next = 1
	for index, winner := range certified {
		if index >= int(constUtil.P14FinalSeriesMaxGames) || left >= constUtil.P14FinalSeriesWinsRequired || right >= constUtil.P14FinalSeriesWinsRequired {
			return 0, 0, 0, 0, errors.New(constUtil.P14FinalSeriesCorrupt)
		}
		switch winner {
		case 0:
			left++
		case 1:
			right++
		default:
			return 0, 0, 0, 0, errors.New(constUtil.P14FinalSeriesCorrupt)
		}
		next = int32(index) + 2
	}
	if left == constUtil.P14FinalSeriesWinsRequired || right == constUtil.P14FinalSeriesWinsRequired {
		state = constUtil.P14FinalSeriesComplete
		next = int32(len(certified))
	} else if len(certified) > 0 {
		state = constUtil.P14FinalSeriesWaitingInput
	}
	return left, right, next, state, nil
}

func (s *GameService) InitializeP14FinalSeries(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalSeriesPolicy{}, &db.P14FinalSeries{}, &db.P14FinalSeriesSlot{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalSeriesPolicy{ID: 1, Source: constUtil.P14FinalSeriesSource}).Error
}

// QueueP14FinalSeries uses the same season lock as existing formation/seal
// producers. It can recover after a stop by observing actual existing Game1;
// no elapsed-time event counts as a played or forfeited game.
func (s *GameService) QueueP14FinalSeries(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var policy db.P14FinalSeriesPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || !season.Enabled {
			return nil
		}
		var games []db.P14FinalGame
		if err := tx.Where("season_index=? AND round_stage=? AND game_index=1", season.SeasonIndex, constUtil.P14Final16Stage).Order("group_id,pair_index").Find(&games).Error; err != nil {
			return err
		}
		for index := range games {
			game := &games[index]
			var existing db.P14FinalSeries
			err := tx.First(&existing, "season_index=? AND round_stage=? AND group_id=? AND pair_index=?", game.SeasonIndex, game.RoundStage, game.GroupID, game.PairIndex).Error
			if err == nil {
				// Frozen old rows retain their own policy and never migrate to current
				// scheduling rules. Until the KF verifier exists, only real input1 is ready.
				if existing.ManifestHash != p6Digest(existing.Manifest) || existing.GameOneID != game.ID {
					return errors.New(constUtil.P14FinalSeriesCorrupt)
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var offer db.P14FinalFormationOffer
			if err = tx.First(&offer, "season_index=? AND round_stage=?", game.SeasonIndex, game.RoundStage).Error; err != nil {
				return err
			}
			row, slots, err := p14FinalSeriesPlan(game, &offer, &policy, now)
			if err != nil {
				if err.Error() == constUtil.P14FinalSeriesPending {
					continue
				}
				return err
			}
			if err = tx.Create(row).Error; err != nil {
				return err
			}
			if err = tx.Create(&slots).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// TickP14FinalSeries shares the existing worker and only the established test
// context clock. Missing independent KF authority leaves series waiting_result.
func (s *GameService) TickP14FinalSeries(ctx context.Context, now time.Time) error {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	return s.QueueP14FinalSeries(ctx, now)
}

// P14FinalSeriesGuessBoundary is a read-only API for subsequent bet offers.
// Existing158/160 offers keep their original frozen end. New pair offers may
// bind the same real first-input start; a missing series never manufactures it.
func (s *GameService) P14FinalSeriesGuessBoundary(ctx context.Context, tx *gorm.DB, season int32, pairHash string) (int64, string, error) {
	if !gameSchemaHasTable(tx, &db.P14FinalSeries{}) {
		return 0, "", nil
	}
	var row db.P14FinalSeries
	err := tx.WithContext(ctx).First(&row, "season_index=? AND pair_hash=?", season, pairHash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	var manifest p14FinalSeriesManifest
	if row.ManifestHash != p6Digest(row.Manifest) || p14PromotionRankDecode(row.Manifest, &manifest) != nil || manifest.FirstInputID != row.GameOneID || len(manifest.Slots) != int(constUtil.P14FinalSeriesMaxGames) {
		return 0, "", errors.New(constUtil.P14FinalSeriesCorrupt)
	}
	return manifest.Slots[0], row.ManifestHash, nil
}
