package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p14GuessRules struct {
	MinimumLevel    int64 `json:"minimumLevel"`
	OrdinaryCost    int64 `json:"ordinaryCost"`
	ChampionCost    int64 `json:"championCost"`
	OrdinaryRevert  int64 `json:"ordinaryRevert"`
	ChampionRevert  int64 `json:"championRevert"`
	OrdinarySuccess int64 `json:"ordinarySuccess"`
	ChampionSuccess int64 `json:"championSuccess"`
	OrdinaryExtra   int64 `json:"ordinaryExtraDiamond"`
	ChampionExtra   int64 `json:"championExtraDiamond"`
	// The original pools and separately named local combination are fixed before
	// payment. A later genuine series certificate must authorize any award.
	Settlement string `json:"settlement"`
}

func (s *GameService) p14GuessOriginalRules() (*p14GuessRules, error) {
	values := map[string]int64{}
	for _, key := range []string{"kuafuGuessLevelLimit", "kuafuGuessPuTong", "kuafuGuessGuanJun", "kuafuGuessRevertPuTong", "kuafuGuessRevertGuanJun", "kuafuGuessPuTongSuccess", "kuafuGuessGuanJunSuccess", "kuafuExtraPuTong", "kuafuExtraGuanJun"} {
		row := s.table("GameConfigure", "StaticName", key)
		if row == nil {
			return nil, errors.New(constUtil.P14GuessCorrupt)
		}
		value := row["NameValue"]
		if key != "kuafuGuessLevelLimit" {
			parts := strings.Split(value, ",")
			resource := "coin"
			if key == "kuafuExtraPuTong" || key == "kuafuExtraGuanJun" {
				resource = "diamond"
			}
			if len(parts) != 2 || parts[0] != resource {
				return nil, errors.New(constUtil.P14GuessCorrupt)
			}
			value = parts[1]
		}
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil || number <= 0 {
			return nil, errors.New(constUtil.P14GuessCorrupt)
		}
		values[key] = number
	}
	rules := &p14GuessRules{MinimumLevel: values["kuafuGuessLevelLimit"], OrdinaryCost: values["kuafuGuessPuTong"], ChampionCost: values["kuafuGuessGuanJun"], OrdinaryRevert: values["kuafuGuessRevertPuTong"], ChampionRevert: values["kuafuGuessRevertGuanJun"], OrdinarySuccess: values["kuafuGuessPuTongSuccess"], ChampionSuccess: values["kuafuGuessGuanJunSuccess"], OrdinaryExtra: values["kuafuExtraPuTong"], ChampionExtra: values["kuafuExtraGuanJun"], Settlement: "local-champion-payback-v1; certified-champion=successCoin+extraDiamond; other=guaranteedRevertCoin; certificate-required-no-early-award"}
	raw, _ := json.Marshal(rules)
	return p14GuessDecodeRules(raw, p6Digest(raw))
}

func p14GuessDecodeRules(raw []byte, hash string) (*p14GuessRules, error) {
	var rules p14GuessRules
	if p6Digest(raw) != hash || p14PromotionRankDecode(raw, &rules) != nil || rules.MinimumLevel != 30 || rules.OrdinaryCost != 10000 || rules.ChampionCost != 100000 || rules.OrdinaryRevert != 5000 || rules.ChampionRevert != 50000 || rules.OrdinarySuccess != 11000 || rules.ChampionSuccess != 110000 || rules.OrdinaryExtra != 10 || rules.ChampionExtra != 200 || rules.Settlement != "local-champion-payback-v1; certified-champion=successCoin+extraDiamond; other=guaranteedRevertCoin; certificate-required-no-early-award" {
		return nil, errors.New(constUtil.P14GuessCorrupt)
	}
	return &rules, nil
}

func p14GuessOfferHash(row *db.P14GuessOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14GuessHash(row *db.P14Guess) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p14GuessReadOffer(row *db.P14GuessOffer) (*p14GuessRules, error) {
	if row == nil || row.SeasonIndex <= 0 || row.GuessType != constUtil.P14GuessChampion || row.Source != constUtil.P14GuessSource || strings.TrimSpace(row.Version) == "" || len(row.Version) > 128 || row.PublishedAt <= 0 || row.PublishedAt > row.OpenAt || row.OpenAt >= row.CloseAt || len(row.QualificationHash) != 64 || len(row.DrawHash) != 64 || row.SourceHash != p14GuessOfferHash(row) {
		return nil, errors.New(constUtil.P14GuessCorrupt)
	}
	return p14GuessDecodeRules(row.Rules, row.RulesHash)
}

func (s *GameService) InitializeP14Guess(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14GuessPolicy{}, &db.P14GuessOffer{}, &db.P14Guess{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14GuessPolicy{ID: 1, Source: constUtil.P14GuessSource}).Error
}

// p14GuessOfferTX reuses complete genuine Final32 and actual original stage7.
// The table numbers and local admission policy are frozen before the first bet.
func (s *GameService) p14GuessOfferTX(ctx context.Context, tx *gorm.DB, season *db.P14SeasonPolicy, now time.Time) (*db.P14GuessOffer, *p14FinalDrawSnapshot, error) {
	draw, err := s.p14FinalDrawTX(ctx, tx, season, now)
	if err != nil {
		return nil, nil, err
	}
	var drawRow db.P14FinalDraw
	if err = tx.First(&drawRow, "season_index=?", season.SeasonIndex).Error; err != nil {
		return nil, nil, err
	}
	var offer db.P14GuessOffer
	err = tx.First(&offer, "season_index=? AND guess_type=?", season.SeasonIndex, constUtil.P14GuessChampion).Error
	if err == nil {
		if _, err = p14GuessReadOffer(&offer); err != nil {
			return nil, nil, err
		}
		if offer.DrawHash != drawRow.ContentHash || offer.QualificationHash != draw.QualificationHash {
			return nil, nil, errors.New(constUtil.P14GuessCorrupt)
		}
		return &offer, draw, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	var gate db.P14GuessPolicy
	if err = tx.First(&gate, 1).Error; err != nil {
		return nil, nil, err
	}
	if !season.Enabled || !gate.Enabled {
		return nil, nil, errors.New(constUtil.P14GuessClosed)
	}
	calendar, err := s.p14SeasonCalendar(tx, season.SeasonIndex)
	if err != nil {
		return nil, nil, err
	}
	var start, end int64
	for _, slot := range calendar {
		if slot.Stage == constUtil.P14Final16BeforeStage {
			start = slot.StartAt
			end = slot.EndAt
		}
	}
	rules, err := s.p14GuessOriginalRules()
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return nil, nil, err
	}
	offer = db.P14GuessOffer{SeasonIndex: season.SeasonIndex, GuessType: constUtil.P14GuessChampion, Version: gate.Version, Source: gate.Source, PublishedAt: gate.PublishedAt, OpenAt: start, CloseAt: end, QualificationHash: draw.QualificationHash, DrawHash: drawRow.ContentHash, Rules: raw, RulesHash: p6Digest(raw)}
	offer.SourceHash = p14GuessOfferHash(&offer)
	if _, err = p14GuessReadOffer(&offer); err != nil {
		return nil, nil, err
	}
	if now.UnixMilli() < offer.OpenAt || now.UnixMilli() >= offer.CloseAt {
		return nil, nil, errors.New(constUtil.P14GuessClosed)
	}
	if err = tx.Create(&offer).Error; err != nil {
		return nil, nil, err
	}
	return &offer, draw, nil
}

// p14GuessAt authenticates owner identity from the current profile, records one
// real coin debit and receipt in a single transaction, and never fabricates a
// series outcome. Exact old receipts remain readable after policy closure.
func (s *GameService) p14GuessAt(ctx context.Context, id, stamp int64, request *pb.CSKFGuess, now time.Time) ([]proto.Message, error) {
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
				return errors.New(constUtil.P14GuessCorrupt)
			}
			return proto.Unmarshal(receipt.Response, response)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// The current champion page sends battleType0 and no fight/item fields. Other
		// original pages wait until their actual serialized battleType is recovered.
		if request.GetGuessType() != constUtil.P14GuessChampion || request.GetBattleType() != constUtil.P14GuessCurrent {
			return errors.New(constUtil.P14ResultsUnverified)
		}
		if request.GetOwnerPlayer().GetPlayerId() != id || int64(request.GetOwnerPlayer().GetServerId()) != s.OriginalServerID() {
			return errors.New(constUtil.P14GuessCorrupt)
		}
		offer, draw, err := s.p14GuessOfferTX(ctx, tx, &season, now)
		if err != nil {
			return err
		}
		rules, err := p14GuessReadOffer(offer)
		if err != nil {
			return err
		}
		if int64(p.GetLevel()) < rules.MinimumLevel {
			return errors.New(constUtil.P14GuessClosed)
		}
		var gate db.P14GuessPolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled {
			return errors.New(constUtil.P14GuessClosed)
		}
		stateCode := constUtil.P14GuessFailed
		if now.UnixMilli() < offer.OpenAt || now.UnixMilli() >= offer.CloseAt {
			stateCode = constUtil.P14GuessEnded
		} else {
			var prior db.P14Guess
			err = tx.First(&prior, "season_index=? AND guess_type=? AND owner_server_id=? AND owner_player_id=?", season.SeasonIndex, constUtil.P14GuessChampion, s.OriginalServerID(), id).Error
			if err == nil {
				if prior.SourceHash != p14GuessHash(&prior) || prior.OfferHash != offer.SourceHash {
					return errors.New(constUtil.P14GuessCorrupt)
				}
				stateCode = constUtil.P14GuessAlready
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else {
				target := request.GetGuessPlayer()
				matched := false
				for _, entry := range draw.Entrants {
					if entry.ServerID == int64(target.GetServerId()) && entry.PlayerID == target.GetPlayerId() {
						matched = true
						break
					}
				}
				if !matched {
					return errors.New(constUtil.P14GuessCorrupt)
				}
				if p.GetCoin() < rules.ChampionCost {
					stateCode = constUtil.P14GuessCoinLack
				} else {
					row := db.P14Guess{SeasonIndex: season.SeasonIndex, GuessType: constUtil.P14GuessChampion, OwnerServerID: s.OriginalServerID(), OwnerPlayerID: id, TargetServerID: int64(target.GetServerId()), TargetPlayerID: target.GetPlayerId(), OfferHash: offer.SourceHash, CostCoin: rules.ChampionCost, CoinBefore: p.GetCoin(), CoinAfter: p.GetCoin() - rules.ChampionCost, CapturedAt: now.UnixMilli()}
					row.SourceHash = p14GuessHash(&row)
					p.Coin = proto.Int64(row.CoinAfter)
					if err = saveProfile(tx, &profile, p, state); err != nil {
						return err
					}
					if err = tx.Create(&row).Error; err != nil {
						return err
					}
					stateCode = constUtil.P14GuessSuccess
				}
			}
		}
		response.IsSuccess = proto.Int32(stateCode)
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

// p14GuessListAt returns only real draw candidates and the actual local owner's
// selection. Unknown federation-wide popularity is omitted rather than invented.
func (s *GameService) p14GuessListAt(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	var response *pb.SCKFGuessList
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		offer, draw, err := s.p14GuessOfferTX(ctx, tx, &season, now)
		if err != nil {
			return err
		}
		var profile db.Profile
		if err = tx.First(&profile, id).Error; err != nil {
			return err
		}
		owner, _, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		rules, err := p14GuessReadOffer(offer)
		if err != nil {
			return err
		}
		if int64(owner.GetLevel()) < rules.MinimumLevel {
			return errors.New(constUtil.P14GuessClosed)
		}
		response = &pb.SCKFGuessList{}
		for _, entry := range draw.Entrants {
			response.InfoList = append(response.InfoList, &pb.GuessPlayerInfo{PlayerId: proto.Int64(entry.PlayerID), ServerId: proto.Int32(int32(entry.ServerID)), Name: proto.String(entry.Name), FrightPower: proto.Int32(entry.Power), MainHeroId: proto.String(entry.Image)})
		}
		var selected db.P14Guess
		err = tx.First(&selected, "season_index=? AND guess_type=? AND owner_server_id=? AND owner_player_id=?", season.SeasonIndex, constUtil.P14GuessChampion, s.OriginalServerID(), id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if selected.SourceHash != p14GuessHash(&selected) || selected.OfferHash != offer.SourceHash {
			return errors.New(constUtil.P14GuessCorrupt)
		}
		response.GuessPlayerId = proto.Int64(selected.TargetPlayerID)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
