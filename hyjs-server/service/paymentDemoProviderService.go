package service

import (
	"context"
	"errors"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// autoConfirmDemoPaymentAt finishes only a persisted local-demo order. It uses
// the same frozen price, profile lock, confirmation ledger and grant transaction
// as the existing operator adapter; no client-supplied success is accepted.
func (s *GameService) autoConfirmDemoPaymentAt(ctx context.Context, order *db.PaymentOrder, now time.Time) (*db.PaymentOrder, error) {
	if !s.P3AutoConfirmDemoPayments || !s.P3LocalWalletEnabled || order.Status != constUtil.P3PaymentPending || now.UnixMilli() < order.CreatedAt || now.UnixMilli() >= order.ExpiresAt {
		return order, nil
	}
	var owner db.Profile
	if err := db.GetIns().WithContext(ctx).Select("id", "uid", "player", "account_id", "account_auth_revision").First(&owner, order.PlayerID).Error; err != nil {
		return nil, err
	}
	// The enabled local wallet covers all created accounts; account 1 stays protected.
	if !walletAccountOwner(db.GetIns().WithContext(ctx), &owner) {
		return order, nil
	}
	trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
	trusted = context.WithValue(trusted, constUtil.LocalWalletPaymentContextKey, true)
	confirmed, err := s.confirmTestPaymentAt(trusted, order.ID, constUtil.P3AutoDemoEventPrefix+order.ID, order.Cents, constUtil.P3PaymentConfirmed, now)
	if errors.Is(err, ErrLocalWalletInsufficient) || errors.Is(err, ErrLocalWalletDisabled) {
		return order, nil
	}
	return confirmed, err
}

// LocalWalletPay retries the player's selected original order. Its frozen
// price, wallet debit and original product grants commit in one transaction.
func (s *GameService) LocalWalletPay(ctx context.Context, id int64, orderID string) (*LocalWalletView, error) {
	if !s.P3LocalWalletEnabled || !s.P3AutoConfirmDemoPayments {
		return nil, ErrLocalWalletDisabled
	}
	if ctx.Value(constUtil.LocalWalletAPIContextKey) != true {
		return nil, ErrLocalWalletInvalid
	}
	var order db.PaymentOrder
	if err := db.GetIns().WithContext(ctx).Where("id=? AND player_id=?", orderID, id).First(&order).Error; err != nil {
		return nil, err
	}
	trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
	trusted = context.WithValue(trusted, constUtil.LocalWalletPaymentContextKey, true)
	if _, err := s.confirmTestPaymentAt(trusted, order.ID, constUtil.P3AutoDemoEventPrefix+order.ID, order.Cents, constUtil.P3PaymentConfirmed, time.Now()); err != nil {
		return nil, err
	}
	return s.LocalWalletSnapshot(ctx, id)
}
