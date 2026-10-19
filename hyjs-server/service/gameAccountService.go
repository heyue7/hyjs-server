package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/scrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

var gameAccountHashSlots = make(chan struct{}, constUtil.GameAccountRegisterHashConcurrency)
var gameAccountRegisterClients = struct {
	sync.Mutex
	windows   map[string]gameAccountRegisterWindow
	lastSweep int64
}{windows: map[string]gameAccountRegisterWindow{}}

type gameAccountRegisterWindow struct {
	StartedAt int64
	Count     int
}

// GameAccountRegisterAllowed bounds guest account creation attempts without introducing external services.
func GameAccountRegisterAllowed(client string, now time.Time) bool {
	gameAccountRegisterClients.Lock()
	defer gameAccountRegisterClients.Unlock()
	current := now.Unix()
	if current-gameAccountRegisterClients.lastSweep >= constUtil.GameAccountRegisterWindowSeconds {
		for key, window := range gameAccountRegisterClients.windows {
			if current-window.StartedAt >= constUtil.GameAccountRegisterWindowSeconds {
				delete(gameAccountRegisterClients.windows, key)
			}
		}
		gameAccountRegisterClients.lastSweep = current
	}

	window, exists := gameAccountRegisterClients.windows[client]
	if exists && current-window.StartedAt >= constUtil.GameAccountRegisterWindowSeconds {
		exists = false
	}
	if !exists {
		if len(gameAccountRegisterClients.windows) >= constUtil.GameAccountRegisterMaxClients {
			return false
		}
		window = gameAccountRegisterWindow{StartedAt: current}
	}
	if window.Count >= constUtil.GameAccountRegisterPerWindow {
		return false
	}
	window.Count++
	gameAccountRegisterClients.windows[client] = window
	return true
}

func gameAccountDuplicate(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &mysqlErr) && mysqlErr.Number == 1062) || db.IsSQLiteUniqueConstraint(err)
}

// gameAccountNewProfile prepares the same salt, digest and session formats used by LoginAccount.
func gameAccountNewProfile(ctx context.Context, username, password string) (*db.Profile, error) {
	select {
	case gameAccountHashSlots <- struct{}{}:
		defer func() { <-gameAccountHashSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	serverID := constUtil.GameDefaultServerID
	if game, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok {
		serverID = game.OriginalServerID()
	}
	if selected := AdminSelectedServerID(ctx); selected > 0 {
		serverID = selected
	}
	row := &db.Profile{ServerID: serverID, Uid: username, PasswordSalt: make([]byte, 16), Session: make([]byte, 16)}
	if _, err := rand.Read(row.PasswordSalt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(row.Session); err != nil {
		return nil, err
	}
	digest, err := scrypt.Key([]byte(password), row.PasswordSalt, 16384, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	row.PasswordHash = digest
	return row, nil
}

// RegisterGameAccount inserts a global identity and its current-runtime role shell after migration; duplicates never log in.
func RegisterGameAccount(ctx context.Context, username, password string, nicknames ...string) (*model.GameAccountRegisterResponse, error) {
	if globalGameAccountsAvailable(db.GetIns().WithContext(ctx)) {
		nickname := username
		if len(nicknames) > 0 && nicknames[0] != "" {
			nickname = nicknames[0]
		}
		return registerGlobalGameAccount(ctx, username, password, nickname)
	}
	var existing int64
	if err := db.GetIns().WithContext(ctx).Model(&db.Profile{}).Where("uid=?", username).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, constUtil.ErrGameAccountExists
	}
	row, err := gameAccountNewProfile(ctx, username, password)
	if err != nil {
		return nil, err
	}
	if err := db.GetIns().WithContext(ctx).Create(row).Error; err != nil {
		if gameAccountDuplicate(err) {
			return nil, constUtil.ErrGameAccountExists
		}
		return nil, err
	}
	return &model.GameAccountRegisterResponse{Registered: true, PlayerID: row.Id, Username: row.Uid}, nil
}

// AdminCreateGamePlayer selects an existing global account or a compatibility account, then creates the current runtime starter with audit.
func (s *GameService) AdminCreateGamePlayer(ctx context.Context, p *model.AdminPrincipal, req model.AdminGamePlayerCreateRequest) (*model.AdminGamePlayerDTO, error) {
	selected := req.ServerID
	if selected == 0 {
		selected = AdminSelectedServerID(ctx)
	}
	if selected == 0 {
		selected = s.OriginalServerID()
	}
	if selected != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, selected)
	if err := adminOperationUser(db.GetIns().WithContext(ctx), p, true); err != nil {
		return nil, err
	}
	var row *db.Profile
	var err error
	if req.AccountID > 0 {
		row = &db.Profile{ServerID: selected, Session: make([]byte, 16)}
		if _, err = rand.Read(row.Session); err != nil {
			return nil, err
		}
	} else {
		row, err = gameAccountNewProfile(ctx, req.Username, req.Password)
		if err != nil {
			return nil, err
		}
	}
	var out model.AdminGamePlayerDTO
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 后台创角尚无玩家会话，先锁住实际全服日期再创建账号与第1天锚点。
		if s.ServerClockReady {
			var clock db.ServerGameClock
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&clock, "server_id=?", s.serverGameClockID()).Error; err != nil {
				return err
			}
			if !serverGameClockValid(&clock) {
				return constUtil.ErrAdminConfiguration
			}
			calendar := context.WithValue(tx.Statement.Context, constUtil.ServerGameClockBoundKey, true)
			calendar = context.WithValue(calendar, constUtil.ServerGameClockServerKey, clock.ServerID)
			calendar = context.WithValue(calendar, constUtil.ServerGameClockOffsetKey, clock.OffsetDays)
			calendar = context.WithValue(calendar, constUtil.ServerGameClockRevisionKey, clock.Revision)
			tx = tx.WithContext(calendar)
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if globalGameAccountsAvailable(tx) {
			var account db.GameAccount
			if req.AccountID > 0 {
				if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, req.AccountID).Error; err != nil {
					return err
				}
				if !account.Enabled || account.Protected {
					return constUtil.ErrAdminPermission
				}
				row.Uid = account.Username
				row.PasswordSalt, row.PasswordHash = append([]byte{}, account.PasswordSalt...), append([]byte{}, account.PasswordHash...)
			} else {
				account = db.GameAccount{Username: req.Username, Nickname: req.Name, PasswordSalt: row.PasswordSalt, PasswordHash: row.PasswordHash, Enabled: true, Revision: 1, CreatedAt: time.Now().UnixMilli()}
				if err := db.AccountsTable(tx, "game_accounts").Create(&account).Error; err != nil {
					if gameAccountDuplicate(err) {
						return constUtil.ErrGameAccountExists
					}
					return err
				}
			}
			row.AccountID, row.AccountAuthRevision = account.ID, account.AuthRevision
		} else if req.AccountID > 0 {
			return constUtil.ErrAdminConfiguration
		}
		if req.AccountID > 0 {
			var existing db.Profile
			found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid=?", row.Uid).First(&existing).Error
			if found == nil {
				if existing.AccountID != req.AccountID || AdminProfileServerID(existing) != selected || existing.Player != nil || existing.Game != nil {
					return constUtil.ErrAdminConfigConflict
				}
				// Registration already created this role shell. Keep its original ID, credentials and session without reviving an old auth revision.
				row = &existing
			} else if !errors.Is(found, gorm.ErrRecordNotFound) {
				return found
			}
		}
		if row.Id == 0 {
			if err := tx.Create(row).Error; err != nil {
				if gameAccountDuplicate(err) {
					return fmt.Errorf("%w: %s", constUtil.ErrAdminConfigConflict, constUtil.ErrGameAccountExists)
				}
				return err
			}
		}
		created, err := createPlayerLocked(tx, row.Id, req.Name, req.HeroID)
		if err != nil {
			return err
		}
		out, err = adminPlayerProjection(*created, true)
		if err != nil {
			return err
		}
		if created.AccountID > 0 {
			account, e := AccountForProfile(tx, created)
			if e != nil {
				return e
			}
			out.Nickname = account.Nickname
		}
		return adminAudit(tx, p, "player.create", fmt.Sprint(row.Id), fmt.Sprintf("username=%s; name=%s; hero=%s; reason=%s", row.Uid, req.Name, req.HeroID, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
