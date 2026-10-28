package service

import (
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
)

// projectGGOCredit only prepares the in-memory view. The first successful profile
// save commits the credit column and removal of the old item in one transaction.
func projectGGOCredit(row *db.Profile, game *model.GameState) error {
	if row.GGOCredit < 0 {
		return errors.New("信用余额无效")
	}
	game.GGOCredit = row.GGOCredit
	game.GGOCreditLoaded = true
	return normalizeGGOCredit(game)
}

func normalizeGGOCredit(game *model.GameState) error {
	balance := game.GGOCredit
	found := false
	if balance < 0 {
		return errors.New("信用余额无效")
	}
	for _, item := range game.Items {
		if item.GetId() == constUtil.GGOCurrencyID {
			found = true
			n := int64(item.GetNum())
			if n < 0 || n > math.MaxInt64-balance {
				return errors.New("信用余额溢出")
			}
			balance += n
		}
	}
	if !found {
		game.GGOCreditLoaded = true
		return nil
	}
	items := make([]*pb.SimpleItem, 0, len(game.Items))
	for _, item := range game.Items {
		if item.GetId() != constUtil.GGOCurrencyID {
			items = append(items, item)
		}
	}
	game.Items = items
	game.GGOCredit = balance
	game.GGOCreditLoaded = true
	return nil
}

func ggoCreditBalance(game *model.GameState) int64 {
	balance := game.GGOCredit
	for _, item := range game.Items {
		if item.GetId() == constUtil.GGOCurrencyID {
			n := int64(item.GetNum())
			if n < 0 || balance < 0 || n > math.MaxInt64-balance {
				return -1
			}
			balance += n
		}
	}
	return balance
}

func ggoCreditAdd(game *model.GameState, amount int64) error {
	if err := normalizeGGOCredit(game); err != nil {
		return err
	}
	if amount < 0 || amount > math.MaxInt64-game.GGOCredit {
		return errors.New("信用余额溢出")
	}
	game.GGOCredit += amount
	return nil
}

func ggoCreditDeduct(game *model.GameState, amount int64) error {
	if err := normalizeGGOCredit(game); err != nil {
		return err
	}
	if amount <= 0 || game.GGOCredit < amount {
		return errors.New("信用点不足")
	}
	game.GGOCredit -= amount
	return nil
}
