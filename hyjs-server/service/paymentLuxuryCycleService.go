package service

import (
	"errors"
	"math"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

const luxuryDragonOfferID int64 = 2

// luxuryDragonDefinition uses the original client's offer and equipment IDs.
func (s *GameService) luxuryDragonDefinition(id int64, now time.Time) (db.PaymentLuxuryOffer, error) {
	var config map[string]string
	for _, row := range s.Tables["LuxuryEquip"] {
		if row["id"] == "2" {
			if config != nil {
				return db.PaymentLuxuryOffer{}, errors.New(constUtil.P3PaymentConfigError)
			}
			config = row
		}
	}
	if config == nil {
		return db.PaymentLuxuryOffer{}, errors.New(constUtil.P3PaymentConfigError)
	}
	money, moneyErr := strconv.ParseInt(config["money"], 10, 64)
	hours, hoursErr := strconv.ParseInt(config["limitHours"], 10, 64)
	start := now.UnixMilli()
	if moneyErr != nil || hoursErr != nil || money != 288 || hours != 12 || config["chapter"] != "6" || config["stage"] != "1" || config["prize"] == "" || start <= 0 || start > math.MaxInt64-hours*3600*1000 {
		return db.PaymentLuxuryOffer{}, errors.New(constUtil.P3PaymentConfigError)
	}
	return db.PaymentLuxuryOffer{PlayerID: id, OfferID: luxuryDragonOfferID, StartAt: start, EndAt: start + hours*3600*1000, Cents: money * 100, Reward: config["prize"]}, nil
}

// The original UI numbers stages within each chapter by numeric mission ID.
// For chapter 6 stage 1, find the lowest ID in the original mission table.
func (s *GameService) luxuryDragonGatePassed(g *model.GameState) (bool, error) {
	if g == nil || g.Copy == nil {
		return false, nil
	}
	var first int64
	for _, row := range s.Tables["CopyMission"] {
		if row["ChapterId"] != "6" {
			continue
		}
		id, err := strconv.ParseInt(row["MissionId"], 10, 64)
		if err != nil || id < 1 {
			return false, errors.New(constUtil.P3PaymentConfigError)
		}
		if first == 0 || id < first {
			first = id
		}
	}
	if first == 0 {
		return false, errors.New(constUtil.P3PaymentConfigError)
	}
	return copyMissionPassed(g, "6", strconv.FormatInt(first, 10)), nil
}

// triggerLuxuryDragonLocked is called only after a successful story completion
// or sweep, with the player's profile already locked in the same transaction.
func (s *GameService) triggerLuxuryDragonLocked(tx *gorm.DB, id int64, game *model.GameState, now time.Time) error {
	if !s.LuxuryDragonReady {
		return nil
	}
	passed, err := s.luxuryDragonGatePassed(game)
	if err != nil || !passed {
		return err
	}
	next, err := s.luxuryDragonDefinition(id, now)
	if err != nil {
		return err
	}
	var current db.PaymentLuxuryOffer
	err = tx.Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&next).Error
	}
	if err != nil {
		return err
	}
	if current.StartAt <= 0 || current.EndAt <= current.StartAt || current.Cents <= 0 || current.Reward == "" {
		return errors.New(constUtil.P3PaymentConfigError)
	}
	if current.Claimed || now.UnixMilli() < current.EndAt {
		return nil
	}
	// A qualifying payment in the expired window must never be overwritten,
	// including when the player has yet to collect the earned equipment.
	paid, _, _, err := PaymentTotals(tx, id, current.StartAt, current.EndAt)
	if err != nil || paid >= current.Cents {
		return err
	}
	history := db.PaymentLuxuryRound{PlayerID: id, OfferID: current.OfferID, StartAt: current.StartAt, EndAt: current.EndAt, Cents: current.Cents, Reward: current.Reward, ConfirmedCents: paid}
	if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&history).Error; err != nil {
		return err
	}
	var saved db.PaymentLuxuryRound
	if err = tx.Where("player_id=? AND offer_id=? AND start_at=?", id, current.OfferID, current.StartAt).Take(&saved).Error; err != nil {
		return err
	}
	if saved != history {
		return errors.New(constUtil.P3PaymentConfigError)
	}
	return tx.Model(&db.PaymentLuxuryOffer{}).Where("player_id=? AND offer_id=?", id, current.OfferID).
		Updates(map[string]any{"start_at": next.StartAt, "end_at": next.EndAt, "cents": next.Cents, "reward": next.Reward}).Error
}
