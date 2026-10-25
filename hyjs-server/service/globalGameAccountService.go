package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/utils"
)

// InitializeGlobalGameAccounts links existing role rows without rewriting save, password or session bytes.
func (s *GameService) InitializeGlobalGameAccounts(ctx context.Context) error {
	if err := db.InitializeGameAccountsSchema(ctx); err != nil {
		return err
	}
	d := db.GetIns().WithContext(ctx)
	for _, col := range []struct{ name, field string }{{"account_id", "AccountID"}, {"account_auth_revision", "AccountAuthRevision"}} {
		if !d.Migrator().HasColumn(&db.Profile{}, col.name) {
			if err := d.Migrator().AddColumn(&db.Profile{}, col.field); err != nil {
				return err
			}
		}
	}
	var cursor int64
	for {
		var rows []db.Profile
		if err := d.Where("id>?", cursor).Order("id").Limit(200).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, source := range rows {
			if err := d.Transaction(func(tx *gorm.DB) error {
				var profile db.Profile
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, source.Id).Error; err != nil {
					return err
				}
				if _, err := ensureGlobalGameAccount(tx, &profile); err != nil {
					return fmt.Errorf("账号迁移冲突（区服%d，角色%d）: %w", AdminProfileServerID(profile), profile.Id, err)
				}
				return syncGlobalAccountRole(tx, &profile)
			}); err != nil {
				return err
			}
		}
		cursor = rows[len(rows)-1].Id
	}
}

func globalGameAccountsAvailable(tx *gorm.DB) bool {
	table := db.AccountsTableName("game_accounts")
	// An explicitly configured shared schema is authoritative. Never fall back to old realm credentials on a metadata probe failure.
	if strings.Contains(table, ".") {
		return true
	}
	return tx.Migrator().HasTable(table)
}

// ensureGlobalGameAccount copies original credential bytes once and never reinstates an old password on restart.
func ensureGlobalGameAccount(tx *gorm.DB, profile *db.Profile) (*db.GameAccount, error) {
	if profile.AccountID > 0 {
		account, err := AccountForProfile(tx, profile)
		if err != nil {
			return nil, err
		}
		if account.Username != profile.Uid {
			return nil, errors.New("账号归属不一致")
		}
		return account, nil
	}
	var account db.GameAccount
	err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("username=?", profile.Uid).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		nickname := profile.Uid
		if profile.Player != nil && profile.Game != nil {
			if player, _, e := decodeProfile(profile); e == nil && player.GetName() != "" {
				nickname = player.GetName()
			}
		}
		if runes := []rune(nickname); len(runes) > 24 {
			nickname = string(runes[:24])
		}
		created := time.Now().UnixMilli()
		if profile.CreatedAt != nil {
			created = *profile.CreatedAt
		}
		account = db.GameAccount{Username: profile.Uid, Nickname: nickname, PasswordSalt: append([]byte{}, profile.PasswordSalt...), PasswordHash: append([]byte{}, profile.PasswordHash...), Enabled: len(profile.PasswordHash) > 0, Protected: profile.Id == 1, Revision: 1, CreatedAt: created}
		result := db.AccountsTable(tx, "game_accounts").Clauses(clause.OnConflict{DoNothing: true}).Create(&account)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			if err = db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("username=?", profile.Uid).First(&account).Error; err != nil {
				return nil, err
			}
		}
	}
	if !bytes.Equal(profile.PasswordSalt, account.PasswordSalt) || !bytes.Equal(profile.PasswordHash, account.PasswordHash) {
		return nil, errors.New("同名旧账号密码摘要不同，请先处理账号冲突")
	}
	profile.AccountID, profile.AccountAuthRevision = account.ID, account.AuthRevision
	if err = tx.Model(&db.Profile{}).Where("id=? AND account_id=0", profile.Id).Updates(map[string]any{"account_id": account.ID, "account_auth_revision": account.AuthRevision}).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

// AccountForProfile reads the globally owned identity using the caller's transaction, including cross-schema wallet transactions.
func AccountForProfile(tx *gorm.DB, profile *db.Profile) (*db.GameAccount, error) {
	if profile == nil || profile.AccountID < 1 {
		return nil, errors.New("账号身份尚未关联")
	}
	var account db.GameAccount
	if err := db.AccountsTable(tx, "game_accounts").First(&account, profile.AccountID).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

// GameAccountWalletOwner preserves legacy protection and avoids treating a new realm's first normal role as the protected original account.
func GameAccountWalletOwner(tx *gorm.DB, profile *db.Profile) bool {
	if profile == nil {
		return false
	}
	if profile.AccountID == 0 {
		return profile.Id > 1
	}
	account, err := AccountForProfile(tx, profile)
	return err == nil && account.Enabled && !account.Protected && profile.AccountAuthRevision == account.AuthRevision
}

func syncGlobalAccountRole(tx *gorm.DB, profile *db.Profile) error {
	if profile.AccountID < 1 {
		return nil
	}
	row := db.GameAccountRole{AccountID: profile.AccountID, ServerID: AdminProfileServerID(*profile), PlayerID: profile.Id}
	if profile.Player != nil && profile.Game != nil {
		player, _, err := decodeProfile(profile)
		if err != nil {
			return err
		}
		row.Name, row.Level = player.GetName(), int64(player.GetLevel())
	}
	var old db.GameAccountRole
	err := db.AccountsTable(tx, "game_account_roles").Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id=? AND server_id=?", row.AccountID, row.ServerID).First(&old).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && old.PlayerID != row.PlayerID {
		return errors.New("同一区服账号已关联其他角色，拒绝覆盖")
	}
	return db.AccountsTable(tx, "game_account_roles").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "account_id"}, {Name: "server_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "level"})}).Create(&row).Error
}

func globalAccountPasswordMatches(ctx context.Context, account *db.GameAccount, password string) (bool, bool, error) {
	if account == nil || len(account.PasswordSalt) == 0 || len(account.PasswordHash) == 0 {
		return false, false, nil
	}
	return utils.VerifyPasswordCredential(ctx, password, account.PasswordSalt, account.PasswordHash)
}

// loginGlobalGameAccount authenticates once globally and materializes only the selected real runtime's original role shell.
func loginGlobalGameAccount(ctx context.Context, uid, password string) (*db.Profile, error) {
	var snapshot db.GameAccount
	if err := db.AccountsTable(db.GetIns().WithContext(ctx), "game_accounts").Where("username=?", uid).Take(&snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 全局账号不存在时明确要求注册，绝不沿用旧存储的首次登录自动建号。
			return nil, errors.New("账号或密码错误，请先注册账号")
		}
		return nil, err
	}
	// 密码计算在事务外完成；事务内只核对快照并更新会话，缩短账号行锁持有时间。
	valid, legacy, err := utils.VerifyPasswordCredential(ctx, password, snapshot.PasswordSalt, snapshot.PasswordHash)
	if err != nil {
		if passwordCredentialRetryable(err) {
			return nil, err
		}
		return nil, errors.New("账号或密码错误")
	}
	if !valid || !snapshot.Enabled {
		return nil, errors.New("账号或密码错误")
	}
	var (
		upgradeSalt []byte
		upgradeHash []byte
	)
	if legacy {
		// 旧 scrypt 摘要不能直接重算，必须用本次明文密码重新生成 Argon2id 凭据。
		upgradeSalt, upgradeHash, err = utils.NewPasswordCredential(ctx, password)
		if err != nil {
			return nil, err
		}
	}
	var result db.Profile
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var account db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, snapshot.ID).Error; err != nil {
			return err
		}
		// 锁内摘要、启用状态和授权版本必须与事务外快照一致，防止并发改密后发放会话。
		if !account.Enabled || account.AuthRevision != snapshot.AuthRevision || !bytes.Equal(account.PasswordSalt, snapshot.PasswordSalt) || !bytes.Equal(account.PasswordHash, snapshot.PasswordHash) {
			return errors.New("账号状态已变化，请重新登录")
		}
		serverID := constUtil.GameDefaultServerID
		if game, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
			serverID = game.OriginalServerID()
		}
		found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid=?", uid).First(&result).Error
		if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		if found == nil && (AdminProfileServerID(result) != serverID || result.AccountID != account.ID) {
			return errors.New("角色不属于当前区服账号")
		}
		created := errors.Is(found, gorm.ErrRecordNotFound)
		if created {
			result = db.Profile{ServerID: serverID, AccountID: account.ID, Uid: uid, PasswordSalt: append([]byte{}, account.PasswordSalt...), PasswordHash: append([]byte{}, account.PasswordHash...)}
		}
		if legacy {
			result.PasswordSalt, result.PasswordHash = upgradeSalt, upgradeHash
			// 升级只替换摘要列，不递增 AuthRevision/Revision，避免普通登录撤销跨区会话或制造后台版本冲突。
			if err := db.AccountsTable(tx, "game_accounts").Where("id=? AND password_salt=? AND password_hash=?", account.ID, snapshot.PasswordSalt, snapshot.PasswordHash).Updates(map[string]any{"password_salt": upgradeSalt, "password_hash": upgradeHash}).Error; err != nil {
				return err
			}
		}
		result.Session = make([]byte, 16)
		if _, err := rand.Read(result.Session); err != nil {
			return err
		}
		result.AccountAuthRevision = account.AuthRevision
		if created {
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
		} else {
			updates := map[string]any{"session": result.Session, "account_auth_revision": result.AccountAuthRevision}
			if legacy {
				updates["password_salt"] = result.PasswordSalt
				updates["password_hash"] = result.PasswordHash
			}
			if err := tx.Model(&db.Profile{}).Where("id=?", result.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return syncGlobalAccountRole(tx, &result)
	})
	return &result, err
}

func gameAccountProjection(tx *gorm.DB, account db.GameAccount) (model.GameAccountDTO, error) {
	out := model.GameAccountDTO{ID: account.ID, Username: account.Username, Nickname: account.Nickname, Enabled: account.Enabled, Protected: account.Protected, CreatedAt: account.CreatedAt, Version: adminVersion(account), Players: []model.GameAccountRoleDTO{}}
	var roles []db.GameAccountRole
	if err := db.AccountsTable(tx, "game_account_roles").Where("account_id=?", account.ID).Order("server_id").Find(&roles).Error; err != nil {
		return out, err
	}
	for _, role := range roles {
		// Original login may reserve a realm identity before character creation.
		// Only completed characters belong in the account's visible role list.
		if role.Name == "" || role.Level < 1 {
			continue
		}
		out.Players = append(out.Players, model.GameAccountRoleDTO{ServerID: role.ServerID, PlayerID: role.PlayerID, Name: role.Name, Level: role.Level})
	}
	return out, nil
}
func prepareGlobalGameAccount(ctx context.Context, username, password, nickname string) (*db.GameAccount, error) {
	if err := validateNameContent(username); err != nil {
		return nil, err
	}
	if err := validateNameContent(nickname); err != nil {
		return nil, err
	}
	profile, err := gameAccountNewProfile(ctx, username, password)
	if err != nil {
		return nil, err
	}
	return &db.GameAccount{Username: username, Nickname: nickname, PasswordSalt: profile.PasswordSalt, PasswordHash: profile.PasswordHash, Enabled: true, Revision: 1, CreatedAt: time.Now().UnixMilli()}, nil
}
func registerGlobalGameAccount(ctx context.Context, username, password, nickname string) (*model.GameAccountRegisterResponse, error) {
	account, err := prepareGlobalGameAccount(ctx, username, password, nickname)
	if err != nil {
		return nil, err
	}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := db.AccountsTable(tx, "game_accounts").Create(account).Error; err != nil {
			if gameAccountDuplicate(err) {
				return constUtil.ErrGameAccountExists
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.GameAccountRegisterResponse{Registered: true, Username: username}, nil
}

// LogoutCurrentGlobalGameAccount invalidates only the supplied role session.
// A missing or replaced session is already logged out: acknowledging it makes a lost-response retry safe,
// and never revokes a newer login or exposes whether an account exists.
func LogoutCurrentGlobalGameAccount(ctx context.Context, id int64, session []byte) (*model.GameAccountLogoutResponse, error) {
	if id < 1 || len(session) != 16 {
		return nil, ErrLocalWalletInvalid
	}
	replacement := make([]byte, 16)
	if _, err := rand.Read(replacement); err != nil {
		return nil, err
	}
	err := db.GetIns().WithContext(ctx).Model(&db.Profile{}).
		Where("id=? AND session=?", id, session).UpdateColumn("session", replacement).Error
	if err != nil {
		return nil, err
	}
	return &model.GameAccountLogoutResponse{LoggedOut: true}, nil
}

// AdminGlobalGameAccounts lists global identities independently from the selected game realm.
func AdminGlobalGameAccounts(ctx context.Context, actor *model.AdminPrincipal, q string, page, size int64, accountIDs ...int64) (*model.AdminGameAccountsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	query := db.AccountsTable(tx, "game_accounts").Model(&db.GameAccount{})
	if len(accountIDs) > 0 && accountIDs[0] > 0 {
		query = query.Where("id=?", accountIDs[0])
	}
	if q != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	out := &model.AdminGameAccountsResponse{Items: []model.GameAccountDTO{}}
	if err := query.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.GameAccount
	if err := query.Order("id DESC").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		v, err := gameAccountProjection(tx, row)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// AdminCreateGlobalGameAccount creates an identity only; a role is created explicitly within its owning runtime.
func AdminCreateGlobalGameAccount(ctx context.Context, actor *model.AdminPrincipal, req model.AdminGameAccountCreateRequest) (*model.GameAccountDTO, error) {
	if err := adminOperationUser(db.GetIns().WithContext(ctx), actor, true); err != nil {
		return nil, err
	}
	account, err := prepareGlobalGameAccount(ctx, req.Username, req.Password, req.Nickname)
	if err != nil {
		return nil, err
	}
	var out model.GameAccountDTO
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		if err := db.AccountsTable(tx, "game_accounts").Create(account).Error; err != nil {
			if gameAccountDuplicate(err) {
				return constUtil.ErrGameAccountExists
			}
			return err
		}
		if err := adminAudit(tx, actor, "account.create", fmt.Sprint(account.ID), req.Reason); err != nil {
			return err
		}
		var err error
		out, err = gameAccountProjection(tx, *account)
		return err
	})
	return &out, err
}

// AdminUpdateGlobalGameAccount updates nickname/status with exact optimistic version and operator audit.
func AdminUpdateGlobalGameAccount(ctx context.Context, actor *model.AdminPrincipal, id int64, req model.AdminGameAccountUpdateRequest) (*model.GameAccountDTO, error) {
	if err := validateNameContent(req.Nickname); err != nil {
		return nil, err
	}
	var out model.GameAccountDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var account db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, id).Error; err != nil {
			return err
		}
		if account.Protected {
			return constUtil.ErrAdminPermission
		}
		if adminVersion(account) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if account.Enabled != req.Enabled {
			account.AuthRevision++
		}
		account.Nickname, account.Enabled = req.Nickname, req.Enabled
		account.Revision++
		if err := db.AccountsTable(tx, "game_accounts").Save(&account).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, actor, "account.update", fmt.Sprint(id), req.Reason); err != nil {
			return err
		}
		var err error
		out, err = gameAccountProjection(tx, account)
		return err
	})
	return &out, err
}

// AdminResetGlobalGameAccountPassword rotates the global credential and invalidates all previously authorized role sessions.
func AdminResetGlobalGameAccountPassword(ctx context.Context, actor *model.AdminPrincipal, id int64, req model.AdminGameAccountPasswordRequest) (*model.GameAccountDTO, error) {
	if err := adminOperationUser(db.GetIns().WithContext(ctx), actor, true); err != nil {
		return nil, err
	}
	prepared, err := prepareGlobalGameAccount(ctx, "", req.Password, "")
	if err != nil {
		return nil, err
	}
	var out model.GameAccountDTO
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var account db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, id).Error; err != nil {
			return err
		}
		if account.Protected {
			return constUtil.ErrAdminPermission
		}
		if adminVersion(account) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		account.PasswordSalt, account.PasswordHash = prepared.PasswordSalt, prepared.PasswordHash
		account.AuthRevision++
		account.Revision++
		if err := db.AccountsTable(tx, "game_accounts").Save(&account).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, actor, "account.password.reset", fmt.Sprint(id), req.Reason); err != nil {
			return err
		}
		var err error
		out, err = gameAccountProjection(tx, account)
		return err
	})
	return &out, err
}

// CurrentGlobalGameAccount projects the current authenticated role's global identity.
func CurrentGlobalGameAccount(ctx context.Context, id int64, session []byte) (*model.GameAccountDTO, error) {
	owner, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	tx := db.GetIns().WithContext(ctx)
	account, err := AccountForProfile(tx, owner)
	if err != nil {
		return nil, err
	}
	out, err := gameAccountProjection(tx, *account)
	out.CurrentServerID, out.CurrentPlayerID = AdminProfileServerID(*owner), owner.Id
	return &out, err
}

// UpdateCurrentGlobalGameAccount verifies the role session again under lock before a self-service account change.
func UpdateCurrentGlobalGameAccount(ctx context.Context, id int64, session []byte, nickname, oldPassword, newPassword string) (*model.GameAccountDTO, error) {
	if newPassword == "" {
		if err := validateNameContent(nickname); err != nil {
			return nil, err
		}
	}
	owner, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	var (
		prepared *db.GameAccount
		snapshot *db.GameAccount
	)
	if newPassword != "" {
		// 旧密码校验和新密码计算都放在事务外，短事务只负责锁定、比对快照并落库。
		var current db.GameAccount
		if err := db.AccountsTable(db.GetIns().WithContext(ctx), "game_accounts").First(&current, owner.AccountID).Error; err != nil {
			return nil, err
		}
		valid, _, err := globalAccountPasswordMatches(ctx, &current, oldPassword)
		if err != nil {
			if passwordCredentialRetryable(err) {
				return nil, err
			}
			return nil, errors.New("原密码不正确")
		}
		if !valid {
			return nil, errors.New("原密码不正确")
		}
		snapshot = &current
		prepared, err = prepareGlobalGameAccount(ctx, "", newPassword, "")
		if err != nil {
			return nil, err
		}
	}
	var out model.GameAccountDTO
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, owner.AccountID).Error; err != nil {
			return err
		}
		var current db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
			return err
		}
		if !account.Enabled || account.Protected || current.AccountID != account.ID || current.AccountAuthRevision != account.AuthRevision || subtle.ConstantTimeCompare(current.Session, session) != 1 {
			return errors.New("账号登录已失效或受保护")
		}
		if prepared != nil {
			if snapshot == nil || !bytes.Equal(account.PasswordSalt, snapshot.PasswordSalt) || !bytes.Equal(account.PasswordHash, snapshot.PasswordHash) {
				return errors.New("账号凭据已更新，请重新登录")
			}
			account.PasswordSalt, account.PasswordHash = prepared.PasswordSalt, prepared.PasswordHash
			account.AuthRevision++
			// Keep only this already-authenticated role session valid; all other realm sessions retain the old revision.
			if err := tx.Model(&db.Profile{}).Where("id=?", id).Update("account_auth_revision", account.AuthRevision).Error; err != nil {
				return err
			}
		} else {
			account.Nickname = nickname
		}
		account.Revision++
		if err := db.AccountsTable(tx, "game_accounts").Save(&account).Error; err != nil {
			return err
		}
		var err error
		out, err = gameAccountProjection(tx, account)
		out.CurrentServerID, out.CurrentPlayerID = AdminProfileServerID(current), current.Id
		out.PasswordChanged = prepared != nil && err == nil
		return err
	})
	return &out, err
}

// GameAccountProtectedProfile retains the old protected role boundary before migration and uses the account identity afterwards.
func GameAccountProtectedProfile(tx *gorm.DB, id int64) bool {
	if id < 1 {
		return true
	}
	var owner db.Profile
	if err := tx.Select("id", "account_id").First(&owner, id).Error; err != nil {
		return true
	}
	if owner.AccountID == 0 {
		return id <= 1
	}
	account, err := AccountForProfile(tx, &owner)
	return err != nil || account.Protected
}
