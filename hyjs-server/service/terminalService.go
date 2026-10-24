package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// initializeTerminal creates realm-local policy/jobs and shared account receipts on the same MySQL instance.
func initializeTerminal(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.TerminalSetting{}, &db.TerminalBatch{}, &db.TerminalRecipient{}); err != nil {
		return err
	}
	for _, item := range []struct {
		name string
		row  any
	}{
		{"terminal_account_states", &db.TerminalAccountState{}}, {"terminal_incomes", &db.TerminalIncome{}}, {"terminal_bursts", &db.TerminalBurst{}},
	} {
		if d.Dialector.Name() == "mysql" {
			// Keep the DDL source identical for the runtime and checked-in SQL files.
			raw, err := db.TerminalSchemaSQL(item.name)
			if err != nil {
				return err
			}
			statement := strings.Replace(string(raw), "CREATE TABLE IF NOT EXISTS "+item.name, "CREATE TABLE IF NOT EXISTS "+db.AccountsTableName(item.name), 1)
			if err = d.Exec(statement).Error; err != nil {
				return err
			}
		} else if err := d.AutoMigrate(item.row); err != nil {
			return err
		}
	}
	return initializeRoleWalletSchema(ctx)
}

func terminalDay(now time.Time) string {
	return now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}
func terminalSettings(tx *gorm.DB, serverID int64, lock bool) (model.TerminalSettings, error) {
	cfg := model.TerminalSettings{SignupPoints: 5000, BurstEnabled: true, BurstCostPoints: 1, BurstMinPoints: 1, BurstMaxPoints: 99, Weekly: []model.TerminalWeeklyRule{}}
	raw, _ := json.Marshal(cfg)
	row := db.TerminalSetting{ServerID: serverID, Payload: string(raw)}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return cfg, err
	}
	q := tx
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&row, "server_id=?", serverID).Error; err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(row.Payload), &cfg); err != nil {
		return cfg, err
	}
	cfg.Version = ""
	cfg.Version = adminVersion(cfg)
	return cfg, nil
}
func terminalRealm(ctx context.Context) int64 {
	if id := AdminSelectedServerID(ctx); id > 0 {
		return id
	}
	if s, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
		return s.OriginalServerID()
	}
	return constUtil.GameDefaultServerID
}
func AdminTerminalSettings(ctx context.Context, actor *model.AdminPrincipal) (*model.TerminalSettings, error) {
	d := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(d, actor, false); err != nil {
		return nil, err
	}
	out, err := terminalSettings(d, terminalRealm(ctx), false)
	return &out, err
}
func AdminSaveTerminalSettings(ctx context.Context, actor *model.AdminPrincipal, req model.TerminalSettings) (*model.TerminalSettings, error) {
	var out model.TerminalSettings
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		old, err := terminalSettings(tx, terminalRealm(ctx), true)
		if err != nil {
			return err
		}
		if old.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		req.Version = ""
		raw, err := json.Marshal(req)
		if err != nil {
			return err
		}
		if err = tx.Save(&db.TerminalSetting{ServerID: terminalRealm(ctx), Payload: string(raw)}).Error; err != nil {
			return err
		}
		out = req
		out.Version = adminVersion(req)
		return adminAudit(tx, actor, "terminal.settings", fmt.Sprint(terminalRealm(ctx)), "before="+old.Version+" after="+out.Version)
	})
	return &out, err
}

// terminalCredit locks the recipient role balance before its scoped receipt and income card.
func terminalCredit(tx *gorm.DB, accountID, serverID, playerID int64, key, source, label, reasonID string, cents int64, now time.Time) error {
	owner, err := walletStorageOwner(tx, playerID)
	if err != nil {
		return err
	}
	if owner.AccountID != accountID || AdminProfileServerID(*owner) != serverID {
		return ErrLocalWalletInvalid
	}
	balance, err := walletReadBalance(tx, playerID, true)
	if err != nil {
		return err
	}
	var prior db.LocalWalletEntry
	err = walletFindEntry(tx, playerID, key, "", &prior)
	if err == nil {
		if prior.DeltaCents != cents || prior.Source != source {
			return ErrLocalWalletConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if cents <= 0 || balance.BalanceCents > math.MaxInt64-cents || balance.Revision == math.MaxInt64 {
		return ErrLocalWalletInvalid
	}
	balance.BalanceCents += cents
	balance.Revision++
	entry := db.LocalWalletEntry{PlayerID: playerID, Key: key, Kind: "add", DeltaCents: cents, BalanceAfter: balance.BalanceCents, Revision: balance.Revision, At: now.UnixMilli(), Source: source, Label: label, ReasonID: reasonID}
	if err = walletCreateEntry(tx, &entry); err != nil {
		return err
	}
	if err = walletSaveBalance(tx, balance); err != nil {
		return err
	}
	return db.AccountsTable(tx, "terminal_role_incomes").Create(&db.TerminalRoleIncome{ServerID: serverID, PlayerID: playerID, AccountID: accountID, ID: key, Label: label, AmountCents: cents, CreatedAt: now.UnixMilli(), Revision: balance.Revision, Source: source}).Error
}
func terminalSignup(tx *gorm.DB, account *db.GameAccount, serverID, playerID int64) error {
	// Legacy isolated consumers that have not enabled the terminal retain their original registration path.
	if !tx.Migrator().HasTable(&db.TerminalSetting{}) {
		return nil
	}
	var locked db.GameAccount
	if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, account.ID).Error; err != nil {
		return err
	}
	// Locking reads see the latest committed claim even in an outer repeatable-read transaction.
	for _, table := range []string{"account_wallet_entries", "role_wallet_entries"} {
		var prior db.AccountWalletEntry
		err := db.AccountsTable(tx, table).Clauses(clause.Locking{Strength: "UPDATE"}).Select("`key`").Where("account_id=? AND (`key`=? OR legacy_key=?)", account.ID, "terminal:signup", "terminal:signup").Take(&prior).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	var priorRole db.GameAccountRole
	err := db.AccountsTable(tx, "game_account_roles").Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=? AND name<>? AND level>=? AND NOT (server_id=? AND player_id=?)", account.ID, "", 1, serverID, playerID).Take(&priorRole).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	cfg, err := terminalSettings(tx, serverID, false)
	if err != nil {
		return err
	}
	if cfg.SignupPoints == 0 || locked.Protected {
		return nil
	}
	return terminalCredit(tx, account.ID, serverID, playerID, "terminal:signup", "signup", "首次注册赠送", "", cfg.SignupPoints*100, time.Now())
}
func terminalRoleState(tx *gorm.DB, owner *db.Profile) (*db.TerminalRoleState, error) {
	row := db.TerminalRoleState{AccountID: owner.AccountID, ServerID: AdminProfileServerID(*owner), PlayerID: owner.Id}
	q := db.AccountsTable(tx, "terminal_role_states")
	if err := q.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, err
	}
	err := q.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND player_id=?", row.ServerID, row.PlayerID).First(&row).Error
	return &row, err
}
func terminalProjection(tx *gorm.DB, owner *db.Profile, balance *db.LocalWalletBalance, now time.Time) (*model.TerminalState, error) {
	// The role wallet lock serializes projection and acknowledgement with its credits and daily action.
	if owner.AccountID < 1 {
		return nil, nil
	}
	if _, err := walletReadBalance(tx, owner.Id, true); err != nil {
		return nil, err
	}
	cfg, err := terminalSettings(tx, AdminProfileServerID(*owner), false)
	if err != nil {
		return nil, err
	}
	state, err := terminalRoleState(tx, owner)
	if err != nil {
		return nil, err
	}
	out := &model.TerminalState{BurstEnabled: cfg.BurstEnabled, BurstCostPoints: cfg.BurstCostPoints, BurstMinPoints: cfg.BurstMinPoints, BurstMaxPoints: cfg.BurstMaxPoints, BurstDay: terminalDay(now), IncomeEntries: []model.TerminalIncomeEntry{}}
	var used int64
	if err = db.AccountsTable(tx, "terminal_role_bursts").Where("server_id=? AND player_id=? AND day=?", AdminProfileServerID(*owner), owner.Id, out.BurstDay).Count(&used).Error; err != nil {
		return nil, err
	}
	out.CanBurst = cfg.BurstEnabled && used == 0 && balance.BalanceCents >= cfg.BurstCostPoints*100
	var rows []db.TerminalRoleIncome
	if err = db.AccountsTable(tx, "terminal_role_incomes").Where("server_id=? AND player_id=?", AdminProfileServerID(*owner), owner.Id).Order("revision DESC").Limit(20).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.IncomeEntries = append(out.IncomeEntries, model.TerminalIncomeEntry{ID: r.ID, Label: r.Label, AmountCents: r.AmountCents, CreatedAt: r.CreatedAt, Revision: r.Revision, Source: r.Source})
	}
	if len(rows) > 0 {
		out.LatestIncomeRevision = rows[0].Revision
	}
	if err = db.AccountsTable(tx, "terminal_role_incomes").Where("server_id=? AND player_id=? AND revision>?", AdminProfileServerID(*owner), owner.Id, state.SeenRevision).Count(&out.UnreadCount).Error; err != nil {
		return nil, err
	}
	if out.LatestIncomeRevision > state.DeliveredRevision {
		state.DeliveredRevision = out.LatestIncomeRevision
		if err = db.AccountsTable(tx, "terminal_role_states").Save(state).Error; err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *GameService) LocalWalletBurst(ctx context.Context, id int64, requestID string) (*model.LocalWalletView, error) {
	return s.localWalletBurstAt(ctx, id, requestID, time.Now())
}
func (s *GameService) localWalletBurstAt(ctx context.Context, id int64, requestID string, now time.Time) (*model.LocalWalletView, error) {
	if !s.P3LocalWalletEnabled || ctx.Value(constUtil.LocalWalletAPIContextKey) != true {
		return nil, ErrLocalWalletDisabled
	}
	var out *model.LocalWalletView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, owner *db.Profile, p *pb.Player, _ *model.GameState) error {
		if owner.AccountID < 1 || !walletAccountOwner(tx, owner) || !localWalletSessionAllowed(ctx, owner) {
			return ErrLocalWalletInvalid
		}
		balance, err := walletReadBalance(tx, owner.Id, true)
		if err != nil {
			return err
		}
		var receipt db.TerminalRoleBurst
		err = db.AccountsTable(tx, "terminal_role_bursts").First(&receipt, "server_id=? AND player_id=? AND request_id=?", AdminProfileServerID(*owner), owner.Id, requestID).Error
		replay := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !replay {
			policy, policyErr := localWalletPolicy(tx)
			if policyErr != nil {
				return policyErr
			}
			if !policy.Enabled {
				return ErrLocalWalletDisabled
			}
			cfg, err := terminalSettings(tx, AdminProfileServerID(*owner), false)
			if err != nil {
				return err
			}
			if !cfg.BurstEnabled {
				return ErrLocalWalletDisabled
			}
			var used int64
			if err = db.AccountsTable(tx, "terminal_role_bursts").Where("server_id=? AND player_id=? AND day=?", AdminProfileServerID(*owner), owner.Id, terminalDay(now)).Count(&used).Error; err != nil {
				return err
			}
			if used > 0 {
				return ErrLocalWalletConflict
			}
			cost := cfg.BurstCostPoints * 100
			if balance.BalanceCents < cost {
				return ErrLocalWalletInsufficient
			}
			if balance.Revision > math.MaxInt64-2 {
				return ErrLocalWalletInvalid
			}
			n, err := rand.Int(rand.Reader, big.NewInt(cfg.BurstMaxPoints-cfg.BurstMinPoints+1))
			if err != nil {
				return err
			}
			receipt = db.TerminalRoleBurst{ServerID: AdminProfileServerID(*owner), PlayerID: owner.Id, AccountID: owner.AccountID, RequestID: requestID, Day: terminalDay(now), AwardPoints: cfg.BurstMinPoints + n.Int64()}
			if cost > 0 {
				balance.BalanceCents -= cost
				balance.Revision++
				debit := db.LocalWalletEntry{Key: "terminal:burst:cost:" + requestID, PlayerID: id, Kind: "debit", DeltaCents: -cost, BalanceAfter: balance.BalanceCents, Revision: balance.Revision, At: now.UnixMilli(), Source: "burst", Label: "Burst Link！启动消耗"}
				if err = walletCreateEntry(tx, &debit); err != nil {
					return err
				}
				if err = walletSaveBalance(tx, balance); err != nil {
					return err
				}
			}
			if err = terminalCredit(tx, owner.AccountID, AdminProfileServerID(*owner), id, "terminal:burst:"+requestID, "burst", "Burst Link！每日超频奖励", "", receipt.AwardPoints*100, now); err != nil {
				return err
			}
			if err = db.AccountsTable(tx, "terminal_role_bursts").Create(&receipt).Error; err != nil {
				return err
			}
		}
		b, err := walletReadBalance(tx, id, false)
		if err != nil {
			return err
		}
		policy, err := localWalletPolicy(tx)
		if err != nil {
			return err
		}
		out, err = localWalletProjection(tx, owner, p, b, policy.Enabled, now)
		if err == nil {
			out.BurstAwardPoints = receipt.AwardPoints
			out.AlreadyApplied = replay
		}
		return err
	})
	return out, err
}
func (s *GameService) LocalWalletSeen(ctx context.Context, id, revision int64) (*model.LocalWalletView, error) {
	if !s.P3LocalWalletEnabled || ctx.Value(constUtil.LocalWalletAPIContextKey) != true {
		return nil, ErrLocalWalletDisabled
	}
	var out *model.LocalWalletView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, owner *db.Profile, p *pb.Player, _ *model.GameState) error {
		if owner.AccountID < 1 || !walletAccountOwner(tx, owner) || !localWalletSessionAllowed(ctx, owner) {
			return ErrLocalWalletInvalid
		}
		b, err := walletReadBalance(tx, id, true)
		if err != nil {
			return err
		}
		state, err := terminalRoleState(tx, owner)
		if err != nil {
			return err
		}
		if revision < 0 || revision > state.DeliveredRevision {
			return ErrLocalWalletInvalid
		}
		if revision > state.SeenRevision {
			state.SeenRevision = revision
			if err = db.AccountsTable(tx, "terminal_role_states").Save(state).Error; err != nil {
				return err
			}
		}
		policy, err := localWalletPolicy(tx)
		if err != nil {
			return err
		}
		out, err = localWalletProjection(tx, owner, p, b, policy.Enabled, time.Now())
		return err
	})
	return out, err
}

func terminalBatchView(row db.TerminalBatch, replay bool) *model.TerminalDistributeResponse {
	return &model.TerminalDistributeResponse{BatchID: row.ID, Recipients: row.Recipients, Credited: row.Credited, AlreadyApplied: replay}
}

// terminalFreeze holds the policy row while freezing both recipients and their future immutable labels.
func terminalFreeze(tx *gorm.DB, serverID int64, requestID string, points int64, reason, kind, version string, now time.Time) (db.TerminalBatch, bool, error) {
	cfg, err := terminalSettings(tx, serverID, true)
	if err != nil {
		return db.TerminalBatch{}, false, err
	}
	id := fmt.Sprintf("terminal:%d:%s", serverID, requestID)
	hash := adminVersion(struct {
		Points int64
		Reason string
	}{points, reason})
	var row db.TerminalBatch
	err = tx.First(&row, "id=?", id).Error
	if err == nil {
		if row.BodyHash != hash {
			return row, true, constUtil.ErrAdminConfigConflict
		}
		return row, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return row, false, err
	}
	if version != "" && cfg.Version != version {
		return row, false, constUtil.ErrAdminConfigConflict
	}
	row = db.TerminalBatch{ID: id, ServerID: serverID, RequestID: requestID, BodyHash: hash, Points: points, Reason: reason, Kind: kind, CreatedAt: now.UnixMilli(), Status: "pending"}
	// Page profile saves and batch-read account eligibility; draw all labels from one loaded catalogue.
	var catalogue []db.LocalWalletReason
	if err = tx.Where("enabled=?", true).Order("id").Find(&catalogue).Error; err != nil {
		return row, false, err
	}
	var cursor int64
	for {
		var profiles []db.Profile
		q := tx.Select("id", "account_id", "server_id", "player").Where("id>? AND account_id>0 AND player IS NOT NULL", cursor)
		if serverID == constUtil.GameDefaultServerID {
			q = q.Where("server_id=? OR server_id=0", serverID)
		} else {
			q = q.Where("server_id=?", serverID)
		}
		if err = q.Order("id").Limit(200).Find(&profiles).Error; err != nil {
			return row, false, err
		}
		if len(profiles) == 0 {
			break
		}
		accountIDs := make([]int64, 0, len(profiles))
		for _, profile := range profiles {
			accountIDs = append(accountIDs, profile.AccountID)
		}
		var accounts []db.GameAccount
		if err = db.AccountsTable(tx, "game_accounts").Select("id", "protected").Where("id IN ?", accountIDs).Find(&accounts).Error; err != nil {
			return row, false, err
		}
		eligible := map[int64]bool{}
		for _, account := range accounts {
			eligible[account.ID] = !account.Protected
		}
		recipients := make([]db.TerminalRecipient, 0, len(profiles))
		pageSeen := map[int64]bool{}
		for _, profile := range profiles {
			if !eligible[profile.AccountID] || pageSeen[profile.AccountID] || profile.Player == nil || *profile.Player == "" || *profile.Player == "null" {
				continue
			}
			var character struct {
				Name  string `json:"name"`
				Level int64  `json:"level"`
			}
			if err = json.Unmarshal([]byte(*profile.Player), &character); err != nil {
				return row, false, err
			}
			if character.Name == "" || character.Level < 1 {
				continue
			}
			pageSeen[profile.AccountID] = true
			reasonRow, drawErr := drawWalletReasonRows(catalogue, points*100)
			if drawErr != nil {
				return row, false, drawErr
			}
			recipients = append(recipients, db.TerminalRecipient{BatchID: id, AccountID: profile.AccountID, PlayerID: profile.Id, Label: walletReasonLabel(reasonRow.Label, points*100), ReasonID: reasonRow.ID})
		}
		if len(recipients) > 0 {
			// The composite receipt key deduplicates accounts across pages without retaining every profile in memory.
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&recipients)
			if created.Error != nil {
				return row, false, created.Error
			}
			row.Recipients += created.RowsAffected
		}
		cursor = profiles[len(profiles)-1].Id
	}
	if row.Recipients == 0 {
		row.Status = "complete"
	}
	err = tx.Create(&row).Error
	return row, false, err
}

// terminalResume commits each recipient independently; receipts and role credits are atomic and restart-safe.
func terminalResume(ctx context.Context, id string) error {
	d := db.GetIns().WithContext(ctx)
	var batch db.TerminalBatch
	if err := d.First(&batch, "id=?", id).Error; err != nil {
		return err
	}
	var cursor int64
	var blocked error
	for {
		var recipients []db.TerminalRecipient
		if err := d.Where("batch_id=? AND applied=? AND account_id>?", id, false, cursor).Order("account_id").Limit(100).Find(&recipients).Error; err != nil {
			return err
		}
		if len(recipients) == 0 {
			break
		}
		for _, recipient := range recipients {
			cursor = recipient.AccountID
			err := d.Transaction(func(tx *gorm.DB) error {
				var row db.TerminalRecipient
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "batch_id=? AND account_id=?", id, recipient.AccountID).Error; err != nil {
					return err
				}
				if row.Applied {
					return nil
				}
				var account db.GameAccount
				if err := db.AccountsTable(tx, "game_accounts").First(&account, row.AccountID).Error; err != nil {
					return err
				}
				// Protection changes preserve the frozen receipt for review and never credit the protected account.
				if account.Protected {
					return constUtil.ErrAdminPermission
				}
				if err := terminalCredit(tx, row.AccountID, batch.ServerID, row.PlayerID, id, batch.Kind, row.Label, row.ReasonID, batch.Points*100, time.Now()); err != nil {
					return err
				}
				row.Applied = true
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
				return tx.Model(&db.TerminalBatch{}).Where("id=?", id).UpdateColumn("credited", gorm.Expr("credited + 1")).Error
			})
			if err != nil {
				if ctx.Err() != nil || backgroundWorkerTransient(err) {
					return err
				}
				// Continue unaffected accounts and batches while leaving the failed receipt recoverable.
				blocked = err
			}
		}
	}
	err := d.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&db.TerminalRecipient{}).Where("batch_id=? AND applied=?", id, true).Count(&count).Error; err != nil {
			return err
		}
		status := "complete"
		if count < batch.Recipients {
			status = "blocked"
		}
		return tx.Model(&db.TerminalBatch{}).Where("id=?", id).Updates(map[string]any{"credited": count, "status": status}).Error
	})
	if err != nil {
		return err
	}
	return blocked
}
func AdminTerminalDistribute(ctx context.Context, actor *model.AdminPrincipal, req model.TerminalDistributeRequest) (*model.TerminalDistributeResponse, error) {
	var row db.TerminalBatch
	var replay bool
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var err error
		row, replay, err = terminalFreeze(tx, terminalRealm(ctx), req.RequestID, req.Points, req.Reason, "admin_manual", req.Version, time.Now())
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		return adminAudit(tx, actor, "terminal.distribute", row.ID, fmt.Sprintf("points=%d recipients=%d reason=%s", row.Points, row.Recipients, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	if err = terminalResume(ctx, row.ID); err != nil {
		return nil, err
	}
	err = db.GetIns().WithContext(ctx).First(&row, "id=?", row.ID).Error
	return terminalBatchView(row, replay), err
}
func AdminTerminalDistributions(ctx context.Context, actor *model.AdminPrincipal, page, size int64) (*model.TerminalDistributionsResponse, error) {
	d := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(d, actor, false); err != nil {
		return nil, err
	}
	out := &model.TerminalDistributionsResponse{Items: []model.TerminalDistributionItem{}}
	q := d.Model(&db.TerminalBatch{}).Where("server_id=?", terminalRealm(ctx))
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.TerminalBatch
	if err := q.Order("created_at DESC,id DESC").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, model.TerminalDistributionItem{BatchID: r.ID, Points: r.Points, Recipients: r.Recipients, Credited: r.Credited, CreatedAt: r.CreatedAt, Kind: r.Kind, Status: r.Status})
	}
	return out, nil
}

// ProcessTerminalDistributions resumes durable batches and freezes only today's real Beijing weekly awards.
// Call once at startup and periodically with the runtime lifecycle context; no standalone goroutine is created here.
func (s *GameService) ProcessTerminalDistributions(ctx context.Context) error {
	return s.processTerminalDistributionsAt(ctx, time.Now())
}
func (s *GameService) processTerminalDistributionsAt(ctx context.Context, now time.Time) error {
	serverID := s.OriginalServerID()
	d := db.GetIns().WithContext(ctx)
	var pending []db.TerminalBatch
	if err := d.Where("server_id=? AND status IN ?", serverID, []string{"pending", "blocked"}).Find(&pending).Error; err != nil {
		return err
	}
	for _, row := range pending {
		if err := terminalResume(ctx, row.ID); err != nil {
			if ctx.Err() != nil || backgroundWorkerTransient(err) {
				return err
			}
			elog.Warn("terminal batch blocked", zap.String("batchId", row.ID), zap.Error(err))
		}
	}
	cfg, err := terminalSettings(d, serverID, false)
	if err != nil {
		return err
	}
	weekday := int64(now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Weekday())
	if weekday == 0 {
		weekday = 7
	}
	for _, rule := range cfg.Weekly {
		if !rule.Enabled || rule.Weekday != weekday {
			continue
		}
		var row db.TerminalBatch
		err = d.Transaction(func(tx *gorm.DB) error {
			var err error
			// Rule identity and real date make same-day restarts/config edits replay the frozen batch.
			row, _, err = terminalFreeze(tx, serverID, "weekly-"+terminalDay(now)+"-"+rule.ID, rule.Points, "每周超频点数投放", "admin_weekly", cfg.Version, now)
			if errors.Is(err, constUtil.ErrAdminConfigConflict) {
				id := fmt.Sprintf("terminal:%d:weekly-%s-%s", serverID, terminalDay(now), rule.ID)
				return tx.First(&row, "id=?", id).Error
			}
			return err
		})
		if err != nil {
			if ctx.Err() != nil || backgroundWorkerTransient(err) {
				return err
			}
			elog.Warn("terminal weekly rule deferred", zap.String("ruleId", rule.ID), zap.Error(err))
			continue
		}
		if err = terminalResume(ctx, row.ID); err != nil {
			if ctx.Err() != nil || backgroundWorkerTransient(err) {
				return err
			}
			elog.Warn("terminal batch blocked", zap.String("batchId", row.ID), zap.Error(err))
		}
	}
	return nil
}

// ServeTerminalDistributions owns the runtime tick; operational failures remain isolated from game listeners.
func (s *GameService) ServeTerminalDistributions(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := s.ProcessTerminalDistributions(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if backgroundWorkerTransient(err) {
				return err
			}
			elog.Warn("terminal distribution deferred", zap.Error(err))
		}
		if err := waitBackgroundWorker(ctx, constUtil.TerminalWorkerInterval); err != nil {
			return nil
		}
	}
}
