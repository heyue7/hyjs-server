package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p14PairGuessOfferHash(row *db.P14PairGuessOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14PairGuessHash(row *db.P14PairGuess) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// The original prefab sends state7 for this first real32 draw. Later original
// 9/11/13/15/17 pages need their genuine progressed pairs and remain pending.
func p14PairGuessFind(draw *p14FinalDrawSnapshot, server, player int64) (*p14FinalDrawPair, error) {
	var found *p14FinalDrawPair
	for _, pair := range draw.Pairs {
		if pair.LeftServerID == server && pair.LeftPlayerID == player || pair.RightServerID == server && pair.RightPlayerID == player {
			if found != nil {
				return nil, errors.New(constUtil.P14PairGuessCorrupt)
			}
			copy := pair
			found = &copy
		}
	}
	if found == nil {
		return nil, errors.New(constUtil.P14PairGuessCorrupt)
	}
	return found, nil
}
func p14PairGuessReadOffer(row *db.P14PairGuessOffer) error {
	var pair p14FinalDrawPair
	if row == nil || row.SeasonIndex <= 0 || row.BattleType != constUtil.P14Final16BeforeStage || row.PairHash != p6Digest(row.Pair) || p14PromotionRankDecode(row.Pair, &pair) != nil || pair.LeftServerID <= 0 || pair.RightServerID <= 0 || pair.LeftPlayerID <= 0 || pair.RightPlayerID <= 0 || pair.LeftServerID == pair.RightServerID && pair.LeftPlayerID == pair.RightPlayerID || len(row.DrawHash) != 64 || len(row.QualificationHash) != 64 || row.Source != constUtil.P14PairGuessSource || strings.TrimSpace(row.Version) == "" || len(row.Version) > 128 || row.PublishedAt <= 0 || row.PublishedAt > row.OpenAt || row.OpenAt >= row.CloseAt || row.CostCoin != 10000 || row.RevertCoin != 5000 || row.SuccessCoin != 11000 || row.ExtraDiamond != 10 || row.Settlement != constUtil.P14PairGuessSettlement || row.SourceHash != p14PairGuessOfferHash(row) {
		return errors.New(constUtil.P14PairGuessCorrupt)
	}
	return nil
}

// InitializeP14PairGuess adds only default-off independent ordinary-pair tables.
// It reuses the Cross lifecycle and never starts a separate worker.
func (s *GameService) InitializeP14PairGuess(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14PairGuessPolicy{}, &db.P14PairGuessOffer{}, &db.P14PairGuess{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14PairGuessPolicy{ID: 1, Source: constUtil.P14PairGuessSource}).Error
}

// The offer freezes both ordered real participants and the original pool values
// before payment. Neither a synthetic opponent nor a presumed winner is added.
func (s *GameService) p14PairGuessOfferTX(ctx context.Context, tx *gorm.DB, season *db.P14SeasonPolicy, target *pb.GuessPlayer, now time.Time) (*db.P14PairGuessOffer, error) {
	draw, err := s.p14FinalDrawTX(ctx, tx, season, now)
	if err != nil {
		return nil, err
	}
	pair, err := p14PairGuessFind(draw, int64(target.GetServerId()), target.GetPlayerId())
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(pair)
	if err != nil {
		return nil, err
	}
	pairHash := p6Digest(raw)
	var drawRow db.P14FinalDraw
	if err = tx.First(&drawRow, "season_index=?", season.SeasonIndex).Error; err != nil {
		return nil, err
	}
	var offer db.P14PairGuessOffer
	err = tx.First(&offer, "season_index=? AND pair_hash=?", season.SeasonIndex, pairHash).Error
	if err == nil {
		if err = p14PairGuessReadOffer(&offer); err != nil {
			return nil, err
		}
		if offer.DrawHash != drawRow.ContentHash || offer.QualificationHash != draw.QualificationHash || !bytes.Equal(offer.Pair, raw) {
			return nil, errors.New(constUtil.P14PairGuessCorrupt)
		}
		return &offer, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var policy db.P14PairGuessPolicy
	if err = tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if !season.Enabled || !policy.Enabled {
		return nil, errors.New(constUtil.P14PairGuessClosed)
	}
	slots, err := s.p14SeasonCalendar(tx, season.SeasonIndex)
	if err != nil {
		return nil, err
	}
	var start, end int64
	for _, slot := range slots {
		if slot.Stage == constUtil.P14Final16BeforeStage {
			start = slot.StartAt
			end = slot.EndAt
		}
	}
	rules, err := s.p14GuessOriginalRules()
	if err != nil {
		return nil, err
	}
	offer = db.P14PairGuessOffer{SeasonIndex: season.SeasonIndex, PairHash: pairHash, BattleType: constUtil.P14Final16BeforeStage, Pair: raw, DrawHash: drawRow.ContentHash, QualificationHash: draw.QualificationHash, Version: policy.Version, Source: policy.Source, PublishedAt: policy.PublishedAt, OpenAt: start, CloseAt: end, CostCoin: rules.OrdinaryCost, RevertCoin: rules.OrdinaryRevert, SuccessCoin: rules.OrdinarySuccess, ExtraDiamond: rules.OrdinaryExtra, Settlement: constUtil.P14PairGuessSettlement}
	offer.SourceHash = p14PairGuessOfferHash(&offer)
	if err = p14PairGuessReadOffer(&offer); err != nil {
		return nil, err
	}
	if now.UnixMilli() < offer.OpenAt || now.UnixMilli() >= offer.CloseAt {
		return nil, errors.New(constUtil.P14PairGuessClosed)
	}
	if err = tx.Create(&offer).Error; err != nil {
		return nil, err
	}
	return &offer, nil
}

// p14PairGuessAt accepts only original2/7 into a real first-round pair. One
// authenticated joint owner may pay once per pair, independently from champion
// bets. Receipt, actual coin and bet row commit together. Series awards stay gated.
func (s *GameService) p14PairGuessAt(ctx context.Context, id, stamp int64, request *pb.CSKFGuess, now time.Time) ([]proto.Message, error) {
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	response := &pb.SCKFGuess{GuessType: proto.Int32(request.GetGuessType())}
	var player *pb.Player
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, state, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if err = s.RefreshP5GuildPrayerPower(ctx, tx, p, state, p13UFNow(ctx)); err != nil {
			return err
		}
		player = p
		var receipt db.P14Receipt
		err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, "CSKFGuess", stamp).Error
		if err == nil {
			if !bytes.Equal(raw, receipt.Request) {
				return errors.New(constUtil.P14PairGuessCorrupt)
			}
			return proto.Unmarshal(receipt.Response, response)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if request.GetGuessType() != constUtil.P14PairGuessType || request.GetBattleType() != constUtil.P14Final16BeforeStage {
			return errors.New(constUtil.P14ResultsUnverified)
		}
		if request.GetOwnerPlayer().GetPlayerId() != id || int64(request.GetOwnerPlayer().GetServerId()) != s.OriginalServerID() {
			return errors.New(constUtil.P14PairGuessCorrupt)
		}
		if p.GetLevel() < 30 {
			return errors.New(constUtil.P14PairGuessClosed)
		}
		offer, err := s.p14PairGuessOfferTX(ctx, tx, &season, request.GetGuessPlayer(), now)
		if err != nil {
			return err
		}
		var policy db.P14PairGuessPolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !policy.Enabled {
			return errors.New(constUtil.P14PairGuessClosed)
		}
		seriesStart, _, err := s.P14FinalSeriesGuessBoundary(ctx, tx, season.SeasonIndex, offer.PairHash)
		if err != nil {
			return err
		}
		code := constUtil.P14GuessFailed
		if now.UnixMilli() < offer.OpenAt || now.UnixMilli() >= offer.CloseAt || seriesStart > 0 && now.UnixMilli() >= seriesStart {
			code = constUtil.P14GuessEnded
		} else {
			var prior db.P14PairGuess
			err = tx.First(&prior, "season_index=? AND pair_hash=? AND owner_server_id=? AND owner_player_id=?", season.SeasonIndex, offer.PairHash, s.OriginalServerID(), id).Error
			if err == nil {
				if prior.SourceHash != p14PairGuessHash(&prior) || prior.OfferHash != offer.SourceHash {
					return errors.New(constUtil.P14PairGuessCorrupt)
				}
				code = constUtil.P14GuessAlready
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else if p.GetCoin() < offer.CostCoin {
				code = constUtil.P14GuessCoinLack
			} else {
				target := request.GetGuessPlayer()
				row := db.P14PairGuess{SeasonIndex: season.SeasonIndex, PairHash: offer.PairHash, OwnerServerID: s.OriginalServerID(), OwnerPlayerID: id, TargetServerID: int64(target.GetServerId()), TargetPlayerID: target.GetPlayerId(), OfferHash: offer.SourceHash, CostCoin: offer.CostCoin, CoinBefore: p.GetCoin(), CoinAfter: p.GetCoin() - offer.CostCoin, CapturedAt: now.UnixMilli()}
				row.SourceHash = p14PairGuessHash(&row)
				p.Coin = proto.Int64(row.CoinAfter)
				if err = saveProfile(tx, &profile, p, state); err != nil {
					return err
				}
				if err = tx.Create(&row).Error; err != nil {
					return err
				}
				code = constUtil.P14GuessSuccess
			}
		}
		response.IsSuccess = proto.Int32(code)
		encoded, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Create(&db.P14Receipt{PlayerID: id, Operation: "CSKFGuess", Stamp: stamp, Request: raw, Response: encoded}).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: player}}, nil
}
