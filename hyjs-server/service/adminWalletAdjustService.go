package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminWalletAdjust 在账号及余额锁内完成权限校验、幂等回执、调整和审计。
func AdminWalletAdjust(ctx context.Context, actor *model.AdminPrincipal, playerID int64, req model.AdminWalletAdjustRequest) (*model.AdminWalletAdjustResponse, error) {
	var out model.AdminWalletAdjustResponse
	if AdminSelectedServerID(ctx) < 1 || req.Revision == nil || *req.Revision < 0 || req.Points < 1 || req.Points > constUtil.LocalWalletMaxCreditCents/100 || (req.Action != constUtil.AdminWalletAdjustCredit && req.Action != constUtil.AdminWalletAdjustDebit) {
		return nil, constUtil.ErrAdminInput
	}
	var options []*sql.TxOptions
	if db.GetIns().Dialector.Name() == "mysql" {
		// 仅此事务使用已提交读，避免不同账号的空回执查询相互持有间隙锁。
		options = append(options, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var owner db.Profile
		if err := tx.Select("id", "server_id", "account_id").First(&owner, playerID).Error; err != nil {
			return err
		}
		if err := AdminCheckProfileScope(ctx, owner); err != nil {
			return err
		}
		if owner.AccountID < 1 {
			return constUtil.ErrAdminWalletAdjustUnlinked
		}
		var account db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, owner.AccountID).Error; err != nil {
			return err
		}
		if account.Protected {
			return constUtil.ErrAdminPermission
		}
		balance, err := accountWalletBalance(tx, owner.AccountID, true)
		if err != nil {
			return err
		}
		var (
			amount = req.Points * 100
			label  = constUtil.AdminWalletAdjustCreditLabel + req.Reason
			key    = constUtil.AdminWalletAdjustKeyPrefix + req.RequestID
			prior  db.AccountWalletEntry
		)
		if req.Action == constUtil.AdminWalletAdjustDebit {
			amount = -amount
			label = constUtil.AdminWalletAdjustDebitLabel + req.Reason
		}
		// 当前读看到等待账号锁期间提交的回执，并保持余额到回执的锁序。
		// 已成功请求先比对冻结业务字段，后续交易不会阻止原请求重放。
		err = db.AccountsTable(tx, "account_wallet_entries").Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, "account_id=? AND `key`=?", owner.AccountID, key).Error
		if err == nil {
			if *req.Revision == math.MaxInt64 || prior.DeltaCents != amount || prior.Source != constUtil.AdminWalletAdjustSource || prior.Label != label || prior.ServerID != AdminProfileServerID(owner) || prior.PlayerID != playerID || prior.Revision != *req.Revision+1 {
				return constUtil.ErrAdminWalletAdjustReplay
			}
			out = model.AdminWalletAdjustResponse{BalanceCents: balance.BalanceCents, Revision: balance.Revision, AmountCents: amount, AlreadyApplied: true}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if balance.Revision != *req.Revision {
			return constUtil.ErrAdminWalletAdjustRevision
		}
		if balance.Revision == math.MaxInt64 || (amount > 0 && balance.BalanceCents > math.MaxInt64-amount) {
			return constUtil.ErrAdminWalletAdjustOverflow
		}
		if amount < 0 && balance.BalanceCents < -amount {
			return ErrLocalWalletInsufficient
		}
		before := balance.BalanceCents
		now := time.Now()
		if req.Action == constUtil.AdminWalletAdjustCredit {
			if err = terminalCredit(tx, owner.AccountID, AdminProfileServerID(owner), playerID, key, constUtil.AdminWalletAdjustSource, label, "", amount, now); err != nil {
				return err
			}
		} else {
			entry := db.AccountWalletEntry{AccountID: owner.AccountID, Key: key, ServerID: AdminProfileServerID(owner), PlayerID: playerID, Kind: constUtil.AdminWalletAdjustDebit, DeltaCents: amount, BalanceAfter: before + amount, Revision: balance.Revision + 1, At: now.UnixMilli(), Source: constUtil.AdminWalletAdjustSource, Label: label}
			if err = db.AccountsTable(tx, "account_wallet_entries").Create(&entry).Error; err != nil {
				return err
			}
			balance.BalanceCents, balance.Revision = entry.BalanceAfter, entry.Revision
			if err = db.AccountsTable(tx, "account_wallet_balances").Save(balance).Error; err != nil {
				return err
			}
		}
		out = model.AdminWalletAdjustResponse{BalanceCents: before + amount, Revision: *req.Revision + 1, AmountCents: amount}
		detail := adminJSON(map[string]any{"accountId": owner.AccountID, "playerId": playerID, "serverId": AdminProfileServerID(owner), "action": req.Action, "amountCents": amount, "beforeCents": before, "afterCents": out.BalanceCents, "reason": req.Reason, "requestId": req.RequestID})
		return adminAudit(tx, actor, constUtil.AdminWalletAdjustAuditAction, fmt.Sprint(owner.AccountID), string(detail))
	}, options...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
