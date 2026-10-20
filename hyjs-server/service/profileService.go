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
	if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
		return nil, errors.New("本地会话无效，请重新登录")
	}
	if row.AccountID > 0 {
		account, err := AccountForProfile(db.GetIns().WithContext(ctx), &row)
		if err != nil || !account.Enabled || row.AccountAuthRevision != account.AuthRevision {
			return nil, errors.New("账号登录已失效，请重新登录")
		}
	}
	return &row, nil
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
	PrepareWarriors(&game)
	if err := prepareBagCapacity(&player, &game); err != nil {
		return nil, nil, err
	}
	return &player, &game, nil
}

func saveProfile(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
	// 业务在恢复后的余额上执行；从满值扣到未满时重开计时，其他消费保留余数。
	nowMS := time.Now().UnixMilli()
	accountRoleChanged := row.Player == nil || row.Game == nil
	if row.Player != nil && row.Game != nil {
		before, _, err := decodeProfile(row)
		if err != nil {
			return err
		}
		accountRoleChanged = before.GetName() != player.GetName() || before.GetLevel() != player.GetLevel()
		if err = finalizeResourceRecovery(player, before.GetEnergy(), before.GetStamina(), nowMS); err != nil {
			return err
		}
	} else if _, err := settleResourceRecovery(player, nowMS); err != nil {
		return err
	}
	// 在成功业务事务内记录真实事件并冻结活动进度，失败随存档一起回滚。
	if svc, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok {
		if err := svc.RefreshP5GuildPrayerPower(tx.Statement.Context, tx, player, game, p13UFNow(tx.Statement.Context)); err != nil {
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
	var (
		p, err = json.Marshal(player)
		g      []byte
	)
	if err != nil {
		return err
	}
	g, err = json.Marshal(game)
	if err != nil {
		return err
	}
	if err := tx.Model(row).Updates(map[string]any{"player": string(p), "game": string(g)}).Error; err != nil {
		return err
	}
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
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForUpdate(&row)
		if err != nil {
			return err
		}
		if _, err = ServerGameClockPrepareProfile(tx, &row, player); err != nil {
			return err
		}
		if err := lockProfilePeriods(tx); err != nil {
			return err
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
		return saveProfile(tx, &row, player, game)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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
		if tx.Migrator().HasTable(&db.PlayerOpeningCalendar{}) {
			if err := insertPlayerOpeningCalendar(tx, id, LocalPlayAt(tx.Statement.Context, realCreatedAt), realCreatedAt); err != nil {
				return err
			}
		}
		game := &model.GameState{Warriors: []*pb.Warrior{w.(*pb.Warrior)}, Formation: []*pb.FormationItem{f.(*pb.FormationItem)}, Copy: copyInfo.(*pb.SCCopyPassedInfo), Position: pos.(*pb.Position)}
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
		return syncGlobalAccountRole(tx, &row)
	}()
	return &row, err
}
