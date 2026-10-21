package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

var ErrLocalWalletDisabled = errors.New(constUtil.LocalWalletDisabledMessage)
var ErrLocalWalletInsufficient = errors.New(constUtil.LocalWalletInsufficientMessage)
var ErrLocalWalletConflict = errors.New(constUtil.LocalWalletConflictMessage)
var ErrLocalWalletInvalid = errors.New(constUtil.LocalWalletInvalidMessage)

type LocalWalletView = model.LocalWalletView
type LocalWalletEntryView = model.LocalWalletEntryView
type LocalWalletPendingOrder = model.LocalWalletPendingOrder

func (s *GameService) InitializeLocalWallet(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.LocalWalletPolicy{}, &db.LocalWalletBalance{}, &db.LocalWalletEntry{}, &db.LocalWalletReason{}); err != nil {
		return err
	}
	policy := db.LocalWalletPolicy{ID: 1, Enabled: false, Version: constUtil.LocalWalletVersion, Source: constUtil.LocalWalletSource}
	if err := db.GetIns().WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error; err != nil {
		return err
	}
	if err := initializeWalletReasons(ctx); err != nil {
		return err
	}
	return initializeTerminal(ctx)
}

func localWalletPolicy(tx *gorm.DB) (*db.LocalWalletPolicy, error) {
	var policy db.LocalWalletPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	if policy.Version != constUtil.LocalWalletVersion || policy.Source != constUtil.LocalWalletSource {
		return nil, ErrLocalWalletInvalid
	}
	return &policy, nil
}

// localWalletOwner retains the retired personal-tool eligibility boundary.
func localWalletOwner(row *db.Profile) bool {
	return row != nil && row.Id > 1 && row.Player != nil && strings.HasPrefix(row.Uid, constUtil.P3DemoAccountPrefix)
}

// walletAccountOwner accepts every authenticated local account with a created character.
// Historical account 1 remains protected; legacy personal tools keep their separate demo guard.
func walletAccountOwner(tx *gorm.DB, row *db.Profile) bool {
	return row != nil && row.Player != nil && GameAccountWalletOwner(tx, row)
}

func localWalletLock(tx *gorm.DB, id int64) (*db.LocalWalletBalance, error) {
	return walletReadBalance(tx, id, true)
}

func localWalletView(balance *db.LocalWalletBalance, enabled bool) *LocalWalletView {
	return &LocalWalletView{Name: constUtil.LocalWalletName, BalanceCents: balance.BalanceCents, Revision: balance.Revision, Currency: "CNY", Enabled: enabled, TestOnly: true, Version: constUtil.LocalWalletVersion, Entries: []LocalWalletEntryView{}, PendingOrders: []LocalWalletPendingOrder{}}
}

func localWalletSessionAllowed(ctx context.Context, row *db.Profile) bool {
	session, ok := ctx.Value(constUtil.LocalWalletSessionContextKey).([]byte)
	return ok && len(session) == 16 && subtle.ConstantTimeCompare(session, row.Session) == 1
}

func localWalletProjection(tx *gorm.DB, owner *db.Profile, p *pb.Player, balance *db.LocalWalletBalance, enabled bool, now time.Time) (*LocalWalletView, error) {
	view := localWalletView(balance, enabled)
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	current := now.In(loc)
	start := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, loc).UnixMilli()
	ledger, err := walletLedger(tx, owner.Id, 0, 0, "", 1, 20)
	if err != nil {
		return nil, err
	}
	view.Entries = ledger.Entries
	view.Total = ledger.Total
	view.Page = 1
	view.PageSize = 20
	end := time.Date(current.Year(), current.Month()+1, 1, 0, 0, 0, 0, loc).UnixMilli()
	summary, err := walletSummary(tx, owner.Id, start, end)
	if err != nil {
		return nil, err
	}
	view.Summary = summary
	var orders []db.PaymentOrder
	if err := tx.Where("player_id=? AND status=? AND source=? AND expires_at>?", owner.Id, constUtil.P3PaymentPending, constUtil.P3PaymentSource, now.UnixMilli()).Order("created_at DESC, id DESC").Limit(20).Find(&orders).Error; err != nil {
		return nil, err
	}
	for _, order := range orders {
		var product db.PaymentProduct
		if json.Unmarshal([]byte(order.Snapshot), &product) != nil {
			return nil, ErrLocalWalletInvalid
		}
		view.PendingOrders = append(view.PendingOrders, LocalWalletPendingOrder{OrderID: order.ID, ProductName: product.Name, AmountCents: order.Cents, ExpiresAt: order.ExpiresAt})
	}
	terminal, err := terminalProjection(tx, owner, balance, now)
	if err != nil {
		return nil, err
	}
	if terminal != nil && !enabled {
		terminal.CanBurst = false
	}
	view.Terminal = terminal
	return view, nil
}

// LocalWalletSnapshot/Add require the explicit local-demo feature and original authenticated player/session.
func (s *GameService) LocalWalletSnapshot(ctx context.Context, id int64) (*LocalWalletView, error) {
	if !s.P3LocalWalletEnabled {
		return nil, ErrLocalWalletDisabled
	}
	if ctx.Value(constUtil.LocalWalletAPIContextKey) != true {
		return nil, ErrLocalWalletInvalid
	}
	var result *LocalWalletView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, _ *model.GameState) error {
		if !walletAccountOwner(tx, row) || !localWalletSessionAllowed(ctx, row) {
			return ErrLocalWalletInvalid
		}
		policy, err := localWalletPolicy(tx)
		if err != nil {
			return err
		}
		balance, err := walletReadBalance(tx, id, true)
		if err != nil {
			return err
		}

		result, err = localWalletProjection(tx, row, p, balance, policy.Enabled, time.Now())
		return err
	})
	return result, err
}

func validLocalWalletKey(key string) bool {
	if len(key) < 1 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// LocalWalletAdd remains available to legacy internal migration checks; the HTTP add route always rejects.
func (s *GameService) LocalWalletAdd(ctx context.Context, id int64, requestKey string, cents int64) (*LocalWalletView, error) {
	return s.localWalletAddAt(ctx, id, requestKey, cents, time.Now())
}

func (s *GameService) localWalletAddAt(ctx context.Context, id int64, requestKey string, cents int64, now time.Time) (*LocalWalletView, error) {
	if !s.P3LocalWalletEnabled {
		return nil, ErrLocalWalletDisabled
	}
	if ctx.Value(constUtil.LocalWalletAPIContextKey) != true || !validLocalWalletKey(requestKey) || cents <= 0 || cents > constUtil.LocalWalletMaxCreditCents {
		return nil, ErrLocalWalletInvalid
	}
	var result *LocalWalletView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, _ *model.GameState) error {
		if !walletAccountOwner(tx, row) || !localWalletSessionAllowed(ctx, row) {
			return ErrLocalWalletInvalid
		}
		policy, err := localWalletPolicy(tx)
		if err != nil {
			return err
		}
		balance, err := localWalletLock(tx, id)
		if err != nil {
			return err
		}
		var prior db.LocalWalletEntry
		err = walletFindEntry(tx, id, "add:"+requestKey, "", &prior)
		if err == nil {
			if prior.Kind != "add" || prior.DeltaCents != cents || prior.OrderID != nil || prior.Source != constUtil.LocalWalletVersion {
				return ErrLocalWalletConflict
			}
			result, err = localWalletProjection(tx, row, p, balance, policy.Enabled, now)
			if err != nil {
				return err
			}
			result.AlreadyApplied = true
			result.CreditLabel = prior.Label
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !policy.Enabled {
			return ErrLocalWalletDisabled
		}
		if balance.BalanceCents > math.MaxInt64-cents || balance.Revision == math.MaxInt64 {
			return ErrLocalWalletInvalid
		}
		balance.BalanceCents += cents
		balance.Revision++
		reason, err := drawWalletReason(tx, cents)
		if err != nil {
			return err
		}
		entry := db.LocalWalletEntry{ReasonID: reason.ID, Label: walletReasonLabel(reason.Label, cents), PlayerID: id, Key: "add:" + requestKey, Kind: "add", DeltaCents: cents, BalanceAfter: balance.BalanceCents, Revision: balance.Revision, At: now.UnixMilli(), Source: constUtil.LocalWalletVersion}
		if err = walletCreateEntry(tx, &entry); err != nil {
			return err
		}
		if err = walletSaveBalance(tx, balance); err != nil {
			return err
		}
		result, err = localWalletProjection(tx, row, p, balance, true, now)
		if err == nil {
			result.CreditLabel = entry.Label
		}
		return err
	})
	return result, err
}

// LocalWalletDebitOrder must run inside confirmTestPaymentAt's existing profile-lock transaction.
// Calling it and granting the original product in separate transactions is forbidden by this contract.
func (s *GameService) LocalWalletDebitOrder(tx *gorm.DB, owner *db.Profile, order *db.PaymentOrder, now time.Time) error {
	if !s.P3LocalWalletEnabled {
		return ErrLocalWalletDisabled
	}
	if !walletAccountOwner(tx, owner) || order == nil || order.PlayerID != owner.Id || len(order.ID) < 1 || len(order.ID) > 64 || order.Cents <= 0 || order.Source != constUtil.P3PaymentSource {
		return ErrLocalWalletInvalid
	}
	if tx.Statement.Context.Value(constUtil.LocalWalletAPIContextKey) == true && !localWalletSessionAllowed(tx.Statement.Context, owner) {
		return ErrLocalWalletInvalid
	}
	policy, err := localWalletPolicy(tx)
	if err != nil {
		return err
	}
	balance, err := localWalletLock(tx, owner.Id)
	if err != nil {
		return err
	}
	var prior db.LocalWalletEntry
	err = walletFindEntry(tx, owner.Id, "", order.ID, &prior)
	if err == nil {
		if prior.PlayerID != owner.Id || prior.Key != "order:"+order.ID || prior.Kind != "debit" || prior.DeltaCents != -order.Cents || prior.Source != constUtil.LocalWalletVersion {
			return ErrLocalWalletConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if !policy.Enabled {
		return ErrLocalWalletDisabled
	}
	if order.Status != constUtil.P3PaymentPending || now.UnixMilli() < order.CreatedAt || now.UnixMilli() >= order.ExpiresAt {
		return ErrLocalWalletInvalid
	}
	if balance.BalanceCents < order.Cents {
		return ErrLocalWalletInsufficient
	}
	if balance.Revision == math.MaxInt64 {
		return ErrLocalWalletInvalid
	}
	balance.BalanceCents -= order.Cents
	balance.Revision++
	orderID := order.ID
	var product db.PaymentProduct
	label := "游戏充值"
	if json.Unmarshal([]byte(order.Snapshot), &product) == nil && product.Name != "" {
		label = product.Name
	}
	entry := db.LocalWalletEntry{Label: label, PlayerID: owner.Id, Key: "order:" + order.ID, Kind: "debit", DeltaCents: -order.Cents, BalanceAfter: balance.BalanceCents, Revision: balance.Revision, OrderID: &orderID, At: now.UnixMilli(), Source: constUtil.LocalWalletVersion}
	if err = walletCreateEntry(tx, &entry); err != nil {
		return err
	}
	return walletSaveBalance(tx, balance)
}
