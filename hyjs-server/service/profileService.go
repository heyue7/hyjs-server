package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/utils"
	"strings"
	"time"
)

// LoginAccount uses the global identity after migration; untouched isolated legacy stores retain their original account flow.
func LoginAccount(ctx context.Context, uid, password string) (*db.Profile, error) {
	if globalGameAccountsAvailable(db.GetIns().WithContext(ctx)) {
		return loginGlobalGameAccount(ctx, uid, password)
	}
	var (
		snapshot     db.Profile
		snapshotSalt []byte
		snapshotHash []byte
		newSalt      []byte
		newEncoded   []byte
		exists       bool
		upgrade      bool
		err          error
	)
	// 先在事务外读取凭据快照并完成密码计算，避免 Argon2id/scrypt 长时间持有角色行锁。
	lookup := db.GetIns().WithContext(ctx).Where("uid=?", uid).Take(&snapshot).Error
	if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
		return nil, lookup
	}
	if errors.Is(lookup, gorm.ErrRecordNotFound) {
		// 旧存储首次登录保留自动注册兼容，新账号直接写入当前 Argon2id 凭据。
		if err := validateNameContent(uid); err != nil {
			return nil, err
		}
	} else {
		exists = true
		if game, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok && AdminProfileServerID(snapshot) != game.OriginalServerID() {
			return nil, errors.New("账号不属于当前区服")
		}
		if snapshot.AccountID > 0 {
			return nil, errors.New("全局账号资料不可用，请稍后重试")
		}
		if len(snapshot.PasswordHash) == 0 {
			return nil, errors.New("此账号属于早期无密码测试数据，请使用新的本地账号")
		}
		var valid bool
		valid, upgrade, err = utils.VerifyPasswordCredential(ctx, password, snapshot.PasswordSalt, snapshot.PasswordHash)
		if err != nil {
			if passwordCredentialRetryable(err) {
				return nil, err
			}
			return nil, errors.New("账号或密码错误")
		}
		if !valid {
			return nil, errors.New("账号或密码错误")
		}
		snapshotSalt = append([]byte{}, snapshot.PasswordSalt...)
		snapshotHash = append([]byte{}, snapshot.PasswordHash...)
	}
	if !exists || upgrade {
		newSalt, newEncoded, err = utils.NewPasswordCredential(ctx, password)
		if err != nil {
			return nil, err
		}
	}
	serverID := constUtil.GameDefaultServerID
	if game, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
		serverID = game.OriginalServerID()
	}
	var row db.Profile
	// 唯一键竞争与死锁重试整个登录事务；短事务只锁定角色行核对快照并轮换会话。
	for attempt := 0; attempt < 3; attempt++ {
		row = db.Profile{}
		err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			var current db.Profile
			found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", uid).First(&current).Error
			if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
				return found
			}
			if exists {
				if errors.Is(found, gorm.ErrRecordNotFound) {
					return errors.New("账号状态已变化，请重新登录")
				}
				// 快照与锁内摘要必须一致，防止修改密码并发时用旧密码换发新会话。
				if !bytes.Equal(current.PasswordSalt, snapshotSalt) || !bytes.Equal(current.PasswordHash, snapshotHash) {
					return errors.New("账号凭据已更新，请重新登录")
				}
				row = current
			} else {
				if !errors.Is(found, gorm.ErrRecordNotFound) {
					// 并发首次登录已由其他请求创建账号，后到请求不接管该身份。
					return errors.New("账号或密码错误")
				}
				row = db.Profile{ServerID: serverID, Uid: uid}
			}
			if !exists || upgrade {
				row.PasswordSalt, row.PasswordHash = newSalt, newEncoded
			}
			row.Uid = uid
			row.Session = make([]byte, 16)
			if _, err = rand.Read(row.Session); err != nil {
				return err
			}
			return tx.Save(&row).Error
		}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var mysqlErr *mysqlDriver.MySQLError
		if !errors.As(err, &mysqlErr) || (mysqlErr.Number != 1062 && mysqlErr.Number != 1213) {
			break
		}
	}
	return &row, err
}

// AuthorizeAccount 按角色和当前会话授权，旧连接不会沿用轮换前的会话。
func AuthorizeAccount(ctx context.Context, id int64, session []byte) (*db.Profile, error) {
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	if err := authorizeProfileSession(ctx, &row, session); err != nil {
		return nil, err
	}
	return &row, nil
}

// authorizeAccountIdentity reads only the fields needed for a one-way movement acknowledgement.
func authorizeAccountIdentity(ctx context.Context, id int64, session []byte) (*db.Profile, error) {
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).Select("id", "server_id", "session", "account_id", "account_auth_revision").First(&row, id).Error; err != nil {
		return nil, err
	}
	if err := authorizeProfileSession(ctx, &row, session); err != nil {
		return nil, err
	}
	return &row, nil
}

// p10PushAuthorization reads only the identity, policy and bound-session data
// required for this one send. No game save or credential columns are loaded.
type p10PushAuthorization struct {
	ProfileID, ProfileAccountID, ProfileAccountRevision          int64
	ProfileSession                                               []byte
	AccountRowID, AccountRevision                                int64
	AccountEnabled                                               bool
	UpdatePlayerID, UpdateBuild, UpdateLoginAt, UpdateLastSeenAt int64
	UpdateDisconnectedAt, UpdateEpoch                            int64
	UpdateSessionHash, UpdateClientVersion                       string
	UpdateConnected                                              bool
	PolicyID, PolicyMinRun, PolicyMinLogin, PolicyLoginGateAt    int64
	PolicyMaintenanceStartAt, PolicyMaintenanceEpoch             int64
	PolicyMinLoginVersion, PolicyMinRunVersion                   string
	PolicyMaintenance                                            bool
}

func p10PushAuthorizationQuery(tx *gorm.DB, accountTable string, id int64) *gorm.DB {
	account := tx.Statement.Quote(accountTable)
	return tx.Table("profiles AS p").Select(`p.id AS profile_id, p.session AS profile_session,
		p.account_id AS profile_account_id, p.account_auth_revision AS profile_account_revision,
		COALESCE(a.id,0) AS account_row_id, COALESCE(a.enabled,0) AS account_enabled,
		COALESCE(a.auth_revision,0) AS account_revision,
		COALESCE(u.player_id,0) AS update_player_id, COALESCE(u.session_hash,'') AS update_session_hash,
		COALESCE(u.build_code,0) AS update_build, COALESCE(u.client_version,'') AS update_client_version,
		COALESCE(u.login_at,0) AS update_login_at,
		COALESCE(u.last_seen_at,0) AS update_last_seen_at, COALESCE(u.disconnected_at,0) AS update_disconnected_at,
		COALESCE(u.maintenance_epoch,0) AS update_epoch, COALESCE(u.connected,0) AS update_connected,
		COALESCE(c.id,0) AS policy_id, COALESCE(c.min_run_build,0) AS policy_min_run,
		COALESCE(c.min_login_build,0) AS policy_min_login, COALESCE(c.login_gate_at,0) AS policy_login_gate_at,
		COALESCE(c.min_login_version,'') AS policy_min_login_version,
		COALESCE(c.min_run_version,'') AS policy_min_run_version,
		COALESCE(c.maintenance,0) AS policy_maintenance, COALESCE(c.maintenance_start_at,0) AS policy_maintenance_start_at,
		COALESCE(c.maintenance_epoch,0) AS policy_maintenance_epoch`).
		Joins("LEFT JOIN "+account+" AS a ON a.id=p.account_id").
		Joins("LEFT JOIN client_update_sessions AS u ON u.player_id=p.id").
		Joins("LEFT JOIN client_update_policy AS c ON c.id=1").
		Where("p.id=?", id)
}

func p10MissingAccountTable(tx *gorm.DB, err error) bool {
	if tx.Dialector.Name() == "sqlite" {
		return err != nil && err.Error() == "no such table: game_accounts"
	}
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 && strings.HasSuffix(strings.ToLower(mysqlErr.Message), ".game_accounts' doesn't exist")
}

func authorizeP10PushNarrow(ctx context.Context, id int64, session []byte) error {
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).Select("id", "session", "account_id", "account_auth_revision").First(&row, id).Error; err != nil {
		return err
	}
	return authorizeProfileSession(ctx, &row, session)
}

// Every recipient is checked separately at send time, including account and
// client gates; only a legacy installation without accounts falls back.
func authorizeP10Push(ctx context.Context, id int64, session []byte) error {
	if !clientUpdatesEnabled() {
		return authorizeP10PushNarrow(ctx, id, session)
	}
	tx := db.GetIns().WithContext(ctx)
	var value p10PushAuthorization
	if err := p10PushAuthorizationQuery(tx, db.AccountsTableName("game_accounts"), id).Take(&value).Error; err != nil {
		if p10MissingAccountTable(tx, err) {
			return authorizeP10PushNarrow(ctx, id, session)
		}
		return err
	}
	row := db.Profile{Id: value.ProfileID, Session: value.ProfileSession, AccountID: value.ProfileAccountID, AccountAuthRevision: value.ProfileAccountRevision}
	if err := validateProfileSession(&row, session); err != nil {
		return err
	}
	account := db.GameAccount{ID: value.AccountRowID, Enabled: value.AccountEnabled, AuthRevision: value.AccountRevision}
	if err := validateProfileAccount(&row, &account); err != nil {
		return err
	}
	if value.PolicyID == 0 {
		return gorm.ErrRecordNotFound
	}
	policy := db.ClientUpdatePolicy{ID: value.PolicyID, MinRunBuild: value.PolicyMinRun, MinLoginBuild: value.PolicyMinLogin,
		MinRunVersion: value.PolicyMinRunVersion, MinLoginVersion: value.PolicyMinLoginVersion,
		LoginGateAt: value.PolicyLoginGateAt, Maintenance: value.PolicyMaintenance,
		MaintenanceStartAt: value.PolicyMaintenanceStartAt, MaintenanceEpoch: value.PolicyMaintenanceEpoch}
	if !policy.Maintenance && policy.MaintenanceStartAt > 0 && policy.MaintenanceStartAt <= time.Now().UnixMilli() {
		var err error
		policy, err = clientUpdatePolicy(ctx)
		if err != nil {
			return err
		}
		if policy.Maintenance {
			return constUtil.ErrClientUpdateMaintenance
		}
		// A concurrent policy change may have waited on the update; re-read
		// identity and binding before allowing any send in this rare branch.
		return authorizeP10PushNarrow(ctx, id, session)
	}
	binding := db.ClientUpdateSession{}
	if value.UpdateSessionHash == clientUpdateSessionHash(row.Session) {
		binding = db.ClientUpdateSession{PlayerID: value.UpdatePlayerID, SessionHash: value.UpdateSessionHash, BuildCode: value.UpdateBuild,
			ClientVersion: value.UpdateClientVersion,
			LoginAt:       value.UpdateLoginAt, LastSeenAt: value.UpdateLastSeenAt, DisconnectedAt: value.UpdateDisconnectedAt,
			MaintenanceEpoch: value.UpdateEpoch, Connected: value.UpdateConnected}
	}
	if err := clientUpdateSessionError(policy, binding); err != nil {
		return err
	}
	clientUpdateTouchSession(ctx, &row, binding)
	return nil
}

func validateProfileSession(row *db.Profile, session []byte) error {
	if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
		return errors.New("本地会话无效，请重新登录")
	}
	return nil
}

func validateProfileAccount(row *db.Profile, account *db.GameAccount) error {
	if row.AccountID > 0 && (account == nil || account.ID != row.AccountID || !account.Enabled || row.AccountAuthRevision != account.AuthRevision) {
		return errors.New("账号登录已失效，请重新登录")
	}
	return nil
}

// authorizeProfileSession keeps all authorization gates shared by full and push reads.
func authorizeProfileSession(ctx context.Context, row *db.Profile, session []byte) error {
	if err := validateProfileSession(row, session); err != nil {
		return err
	}
	if row.AccountID > 0 {
		account, err := AccountForProfile(db.GetIns().WithContext(ctx), row)
		if err != nil {
			return validateProfileAccount(row, nil)
		}
		if err := validateProfileAccount(row, account); err != nil {
			return err
		}
	}
	// 原协议和附加HTTP入口共享客户端运行资格。
	return CheckClientUpdateSession(ctx, row)
}

func decodeProfile(row *db.Profile) (*pb.Player, *model.GameState, error) {
	if row.Player == nil || row.Game == nil {
		return nil, nil, errors.New("请先创建角色")
	}
	var (
		player pb.Player
		game   model.GameState
	)
	if err := json.Unmarshal([]byte(*row.Player), &player); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal([]byte(*row.Game), &game); err != nil {
		return nil, nil, err
	}
	if err := projectGGOCredit(row, &game); err != nil {
		return nil, nil, err
	}
	PrepareWarriors(&game)
	if err := prepareBagCapacity(&player, &game); err != nil {
		return nil, nil, err
	}
	return &player, &game, nil
}

func saveProfile(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
	return saveProfileAgainst(tx, row, player, game, nil)
}

// saveProfileWithCraftPower 在保存阶段一次读取当前公会源并计算装备变动后的战力。
func saveProfileWithCraftPower(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState, service *GameService) error {
	if bound, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && bound == service {
		return saveProfileAgainstWithCraftPower(tx, row, player, game, nil, true)
	}
	// 非协议调用可能没有绑定服务；保留原有显式投影及后续存档钩子行为。
	if err := service.PrepareP5GuildPrayerProperties(tx.Statement.Context, tx, player, game, p13UFNow(tx.Statement.Context)); err != nil {
		return err
	}
	if err := service.P8RefreshCraftPower(player, game); err != nil {
		return err
	}
	return saveProfile(tx, row, player, game)
}

// saveProfileAgainst compares hot-path output with the actual persisted row,
// before resource recovery rewrites the transaction's consumption baseline.
func saveProfileAgainst(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState, persisted *db.Profile) error {
	return saveProfileAgainstWithCraftPower(tx, row, player, game, persisted, false)
}

func saveProfileAgainstWithCraftPower(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState, persisted *db.Profile, forceCraftPower bool) error {
	if persisted == nil && game.ProfileStorageBaseline != nil {
		baseline := game.ProfileStorageBaseline
		persisted = &db.Profile{Player: &baseline.Player, Game: &baseline.Game}
	}

	if !game.GGOCreditLoaded {
		if err := projectGGOCredit(row, game); err != nil {
			return err
		}
	}
	if err := normalizeGGOCredit(game); err != nil {
		return err
	}
	if err := ConvertGGOOverclockItem(tx, row, player, game); err != nil {
		return err
	}
	// 业务在恢复后的余额上执行；从满值扣到未满时重开计时，其他消费保留余数。
	nowMS := time.Now().UnixMilli()
	accountRoleChanged := row.Player == nil || row.Game == nil
	if row.Player != nil && row.Game != nil {
		var before pb.Player
		if err := json.Unmarshal([]byte(*row.Player), &before); err != nil {
			return err
		}
		accountRoleChanged = before.GetName() != player.GetName() || before.GetLevel() != player.GetLevel()
		if err := finalizeResourceRecovery(player, before.GetEnergy(), before.GetStamina(), nowMS); err != nil {
			return err
		}
	} else if _, err := settleResourceRecovery(player, nowMS); err != nil {
		return err
	}
	// 在成功业务事务内记录真实事件并冻结活动进度，失败随存档一起回滚。
	if svc, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok {
		if err := svc.refreshP5GuildPrayerPower(tx.Statement.Context, tx, player, game, p13UFNow(tx.Statement.Context), forceCraftPower); err != nil {
			return err
		}
		if err := svc.GuardP7ExpeditionSave(tx, row.Id, game); err != nil {
			return err
		}
		if openServerTracksRequest(tx.Statement.Context) {
			if err := svc.recordOpenServerSave(tx, row, player, game); err != nil {
				return err
			}
		}
		if err := svc.recordDailyTaskSave(tx, row, player, game); err != nil {
			return err
		}
		if err := svc.RecordP1Save(tx, row, player, game); err != nil {
			return err
		}
		if err := svc.RecordP4Save(tx, row, player, game); err != nil {
			return err
		}
		if svc.P5Ready {
			if err := svc.RecordP5Save(tx, row, player, game); err != nil {
				return err
			}
			if err := svc.ObserveP5Collection(tx, row.Id, game); err != nil {
				return err
			}
		}
	}
	if err := compactFieldBossState(game); err != nil {
		return err
	}
	var (
		p, err = json.Marshal(player)
		g      []byte
	)
	if err != nil {
		return err
	}
	if persisted != nil && persisted.Game != nil {
		g, err = model.MarshalProfileGame86Reusing(game, []byte(*persisted.Game))
	} else {
		g, err = model.MarshalProfileGame86(game)
	}
	if err != nil {
		return err
	}
	updates := map[string]any{}
	// Keep the public desert total in the same transaction as the authoritative
	// terminal. The six quality counts remain private to the profile owner.
	if game.GGOField != nil && p10SceneScope(game) == constUtil.GGOFieldScene {
		_, total, err := ggoProtectedChips(game.GGOField.Chips, 0)
		if err != nil {
			return err
		}
		if row.GGOChipTotal == nil || *row.GGOChipTotal != total {
			updates["ggo_chip_total"] = total
		}
	} else if row.GGOChipTotal != nil {
		updates["ggo_chip_total"] = nil
	}
	if !row.GGOCreditMigrated || row.GGOCredit != game.GGOCredit {
		updates["ggo_credit"] = game.GGOCredit
		updates["ggo_credit_migrated"] = true
	}
	if persisted == nil || persisted.Player == nil || *persisted.Player != string(p) {
		updates["player"] = string(p)
	}
	if persisted == nil || persisted.Game == nil || *persisted.Game != string(g) {
		updates["game"] = string(g)
	}
	if len(updates) != 0 {
		if err := tx.Model(row).Updates(updates).Error; err != nil {
			return err
		}
	}
	game.ProfileStorageBaseline = &model.ProfileStorageBaseline{Player: string(p), Game: string(g)}
	if accountRoleChanged && row.AccountID > 0 {
		snapshot := *row
		playerJSON, gameJSON := string(p), string(g)
		snapshot.Player, snapshot.Game = &playerJSON, &gameJSON
		return syncGlobalAccountRole(tx, &snapshot)
	}
	return nil
}

// updateProfile 在立即事务内读取并保存存档，业务校验失败时回滚所有奖励。
func updateProfile(ctx context.Context, id int64, fn func(*pb.Player, *model.GameState) error) error {
	return updateProfileWithTx(ctx, id, func(_ *gorm.DB, player *pb.Player, game *model.GameState) error {
		return fn(player, game)
	})
}

// updateProfileWithTx 让相关奖励与限时效果沿用同一角色行锁和提交边界。
func updateProfileWithTx(ctx context.Context, id int64, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	return updateProfileTransaction(ctx, id, true, fn)
}

// updateFieldProfileWithTx lets ordinary movement and heartbeats share the
// activity-window read locks. A transaction that needs to publish an activity
// fact rolls back completely before retrying with the original exclusive locks.
// Its two callers reset their response and notification state on each attempt.
func updateFieldProfileWithTx(ctx context.Context, id int64, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	switch ctx.Value(constUtil.GameRequestContextKey).(type) {
	case *pb.CSMove, *pb.CSPingTcp:
	default:
		return updateProfileWithTx(ctx, id, fn)
	}
	return updateProfileWithSharedPeriods(ctx, id, fn)
}

// updateProfileWithSharedPeriods 仅供确认重试安全的完整事务使用。
// 首次需要创建活动事实或冻结窗口时，先回滚全部事务，再按原独占顺序重试。
func updateProfileWithSharedPeriods(ctx context.Context, id int64, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	err := updateProfileWithTx(shared, id, fn)
	if errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		return updateProfileWithTx(ctx, id, fn)
	}
	return err
}

// updateProfileForP7QueryWithTx 仅供不产生P1事实的试炼查询使用。
// 保留时钟、角色行锁、恢复、存档钩子及原子提交；查询期间不预锁公共活动窗口。
// 开战、领奖、购买、重置及其他mutation必须继续使用updateProfileWithTx。
func updateProfileForP7QueryWithTx(ctx context.Context, id int64, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	return updateProfileTransaction(ctx, id, false, fn)
}

func updateProfileTransaction(ctx context.Context, id int64, prelockPeriods bool, fn func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	changedChipTotal := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		persisted := row
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		beforeChips, err := ggoChipCount(game)
		if err != nil {
			return err
		}
		if prelockPeriods {
			if err := lockProfilePeriods(tx); err != nil {
				return err
			}
		}
		if svc, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
			// Penalty recovery changes this same locked object, without blocking rewards.
			if _, err = svc.syncP12FieldPVPPenaltyForRead(tx, player, game, time.Now()); err != nil {
				return err
			}
			// 在同一角色锁和同一解码对象上解除跨期首领占用，后续动作与保存不会覆盖恢复结果。
			if _, err = svc.ExpireP12WorldBossPendingLocked(tx, player, game, time.Now()); err != nil {
				return err
			}
			if err = svc.RefreshP5GuildPrayerPower(ctx, tx, player, game, p13UFNow(ctx)); err != nil {
				return err
			}
		}
		if err = fn(tx, player, game); err != nil {
			return err
		}
		if err = saveProfileAgainst(tx, &row, player, game, &persisted); err != nil {
			return err
		}
		afterChips, err := ggoChipCount(game)
		if err != nil {
			return err
		}
		changedChipTotal = beforeChips != afterChips
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err == nil && changedChipTotal {
		if svc, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
			svc.ggoNotifyChipPlayerChange(ctx, id)
		}
	}
	return err
}

// CreatePlayer 幂等创建初始角色，已有角色不会被重新覆盖。
func CreatePlayer(ctx context.Context, id int64, name, heroId string) (*db.Profile, error) {
	var (
		row *db.Profile
		err error
	)
	for attempt := 0; attempt < constUtil.GameCreatePlayerMaxAttempts; attempt++ {
		row, err = createPlayerAttempt(ctx, id, name, heroId)
		var mysqlErr *mysqlDriver.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1213 {
			return row, err
		}
		if attempt+1 < constUtil.GameCreatePlayerMaxAttempts {
			// 死锁已经回滚整个事务，沿请求ctx等待后重读角色与全部进度。
			timer := time.NewTimer(constUtil.GameCreatePlayerRetryDelay * time.Duration(attempt+1))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return row, err
}

// createPlayerAttempt 只产生同一InnoDB事务内的存档与进度副作用。
func createPlayerAttempt(ctx context.Context, id int64, name, heroId string) (*db.Profile, error) {
	var row *db.Profile
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = createPlayerLocked(tx, id, name, heroId)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return row, err
}

// createPlayerLocked reuses the original starter state inside the caller's account/audit transaction.
func createPlayerLocked(tx *gorm.DB, id int64, name, heroId string) (*db.Profile, error) {
	var row db.Profile
	err := func() error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if row.Player != nil {
			return nil
		}
		if err := validateNameContent(name); err != nil {
			return err
		}
		// Serialize first-character eligibility before any role-index or save writes.
		// This keeps all realm creation paths in account -> role-index -> wallet lock order.
		var account db.GameAccount
		if row.AccountID > 0 {
			if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, row.AccountID).Error; err != nil {
				return err
			}
		}
		if err := lockProfilePeriods(tx); err != nil {
			return err
		}
		guid := fmt.Sprintf("%d-hero-1", id)
		p, err := model.GameMessage("Player", map[string]any{"playerId": id, "name": name, "level": 1, "exp": 0, "coin": 0, "diamond": 0, "energy": 100, "stamina": 100, "warriorBagSize": 100, "equipBagSize": 100, "petBagSize": 100, "currentWorldSeedId": 1, "noviceGuideSchedule": 0, "heroWarId": guid})
		if err != nil {
			return err
		}
		w, err := model.GameMessage("Warrior", map[string]any{"id": heroId, "guid": guid, "level": 1, "exp": 0, "talent": 0, "training": map[string]any{}, "tempTraining": map[string]any{}, "breached": map[string]any{"level": 1}, "awake": map[string]any{}})
		if err != nil {
			return err
		}
		f, err := model.GameMessage("FormationItem", map[string]any{"formatPos": 1, "battlePos": 1, "things": []any{map[string]any{"pos": 0, "guid": guid, "id": heroId}}})
		if err != nil {
			return err
		}
		pos, err := model.GameMessage("Position", map[string]any{"playerId": id, "scene": "town_main", "current": map[string]any{"x": 0, "z": 0}})
		if err != nil {
			return err
		}
		copyInfo, err := model.GameMessage("SCCopyPassedInfo", map[string]any{"currentChapterId": "1", "currentMissionId": "1"})
		if err != nil {
			return err
		}
		realCreatedAt := time.Now()
		if gameSchemaHasTable(tx, &db.PlayerOpeningCalendar{}) {
			if err := insertPlayerOpeningCalendar(tx, id, LocalPlayAt(tx.Statement.Context, realCreatedAt), realCreatedAt); err != nil {
				return err
			}
		}
		game := &model.GameState{Warriors: []*pb.Warrior{w.(*pb.Warrior)}, Formation: []*pb.FormationItem{f.(*pb.FormationItem)}, Copy: copyInfo.(*pb.SCCopyPassedInfo), Position: pos.(*pb.Position)}
		if account.BetaUser {
			if err := grantBetaTitle(tx, game, realCreatedAt); err != nil {
				return err
			}
		}
		if err = prepareBagCapacity(p.(*pb.Player), game); err != nil {
			return err
		}
		if err = saveProfile(tx, &row, p.(*pb.Player), game); err != nil {
			return err
		}
		if err = tx.Model(&row).Update("created_at", realCreatedAt.UnixMilli()).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if err := syncGlobalAccountRole(tx, &row); err != nil {
			return err
		}
		if row.AccountID > 0 {
			account, err := AccountForProfile(tx, &row)
			if err != nil {
				return err
			}
			return terminalSignup(tx, account, AdminProfileServerID(row), row.Id)
		}
		return nil
	}()
	return &row, err
}
