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

// RegisterGameAccount strictly inserts a new original account; duplicates never rotate its session or authenticate it.
func RegisterGameAccount(ctx context.Context, username, password string) (*model.GameAccountRegisterResponse, error) {
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

// AdminCreateGamePlayer creates account, original starter state and audit in one transaction.
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
	row, err := gameAccountNewProfile(ctx, req.Username, req.Password)
	if err != nil {
		return nil, err
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
		if err := tx.Create(row).Error; err != nil {
			if gameAccountDuplicate(err) {
				return fmt.Errorf("%w: %s", constUtil.ErrAdminConfigConflict, constUtil.ErrGameAccountExists)
			}
			return err
		}
		created, err := createPlayerLocked(tx, row.Id, req.Name, req.HeroID)
		if err != nil {
			return err
		}
		out, err = adminPlayerProjection(*created, true)
		if err != nil {
			return err
		}
		return adminAudit(tx, p, "player.create", fmt.Sprint(row.Id), fmt.Sprintf("username=%s; name=%s; hero=%s; reason=%s", req.Username, req.Name, req.HeroID, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
