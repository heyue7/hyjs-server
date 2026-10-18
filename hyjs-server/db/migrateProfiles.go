package db

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"math"
	"net/url"
	"os"
	"reflect"
	"unicode/utf8"
)

// MigrationReport 只暴露计数和校验摘要，避免会话及密码进入日志。
type MigrationReport struct {
	Profiles int    `json:"profiles"`
	Inserted int    `json:"inserted"`
	Pools    int    `json:"pools"`
	Rewards  int    `json:"rewards"`
	Sequence int64  `json:"sequence"`
	SHA256   string `json:"sha256"`
	Applied  bool   `json:"applied"`
}

// MigrateSQLiteToMySQL 读取 SQLite 一致性快照，幂等导入；任何同 ID 或 UID 差异均拒绝覆盖。
func MigrateSQLiteToMySQL(ctx context.Context, source string, cfg MySQLConfig, apply bool) (*MigrationReport, error) {
	if _, err := os.Stat(source); err != nil {
		return nil, err
	}
	src, err := gorm.Open(sqlite.Open((&url.URL{Scheme: "file", Path: source, RawQuery: "mode=ro"}).String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlSrc, err := src.DB()
	if err != nil {
		return nil, err
	}
	defer sqlSrc.Close()
	sqlSrc.SetMaxOpenConns(1)
	var (
		profiles []Profile
		pools    []GameplayPool
		rewards  []GameplayPoolReward
		sequence int64
	)
	// 历史 Python 表可能缺少 game/password 列，用 NULL 表达原有空值。
	err = src.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tables []string
		if err := tx.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").Scan(&tables).Error; err != nil {
			return err
		}
		known := map[string]bool{"profiles": true, "gameplay_pools": true, "gameplay_pool_rewards": true}
		present := map[string]bool{}
		for _, table := range tables {
			if !known[table] {
				return fmt.Errorf("unsupported source table %s; migration refused", table)
			}
			present[table] = true
		}
		if !present["profiles"] {
			return errors.New("source profiles table missing")
		}
		var cols []struct{ Name string }
		if err := tx.Raw("PRAGMA table_info(profiles)").Scan(&cols).Error; err != nil {
			return err
		}
		have := map[string]bool{}
		for _, c := range cols {
			have[c.Name] = true
		}
		selection := "id,uid,session,player,created_at"
		for _, name := range []string{"game", "password_salt", "password_hash"} {
			if have[name] {
				selection += "," + name
			} else {
				selection += ",NULL AS " + name
			}
		}
		if err := tx.Table("profiles").Select(selection).Order("id").Find(&profiles).Error; err != nil {
			return fmt.Errorf("read source profiles: %w", err)
		}
		if tx.Migrator().HasTable("sqlite_sequence") {
			var seqRow struct{ Seq int64 }
			if err := tx.Raw("SELECT COALESCE(MAX(seq),0) AS seq FROM sqlite_sequence WHERE name='profiles'").Scan(&seqRow).Error; err != nil {
				return fmt.Errorf("source sequence: %w", err)
			}
			sequence = seqRow.Seq
		}
		if present["gameplay_pools"] {
			if err := tx.Order("id").Find(&pools).Error; err != nil {
				return err
			}
		}
		if present["gameplay_pool_rewards"] {
			if err := tx.Order("pool_id,id").Find(&rewards).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("source snapshot: %w", err)
	}
	for _, p := range profiles {
		if len(p.Uid) > 1024 || !utf8.ValidString(p.Uid) || p.Session == nil {
			return nil, fmt.Errorf("invalid source profile %d", p.Id)
		}
		if (p.Player != nil && !utf8.ValidString(*p.Player)) || (p.Game != nil && !utf8.ValidString(*p.Game)) {
			return nil, fmt.Errorf("invalid UTF-8 in profile %d", p.Id)
		}
		if p.Id > sequence {
			sequence = p.Id
		}
	}
	if sequence == math.MaxInt64 {
		return nil, errors.New("source sequence exhausted")
	}
	encoded, err := json.Marshal(struct {
		Profiles []Profile
		Pools    []GameplayPool
		Rewards  []GameplayPoolReward
	}{profiles, pools, rewards})
	if err != nil {
		return nil, err
	}
	// Profile 的 json 标签隐藏敏感字段；校验摘要显式纳入所有原始值。
	var raw []any
	for _, p := range profiles {
		raw = append(raw, []any{p.Id, p.Uid, p.Session, p.Player, p.Game, p.CreatedAt, p.PasswordSalt, p.PasswordHash})
	}
	sensitive, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append(encoded, sensitive...))
	report := &MigrationReport{Profiles: len(profiles), Pools: len(pools), Rewards: len(rewards), Sequence: sequence, SHA256: fmt.Sprintf("%x", sum), Applied: apply}
	dst, err := OpenGameMySQL(ctx, cfg, apply)
	if err != nil {
		return nil, err
	}
	sqlDst, err := dst.DB()
	if err != nil {
		return nil, err
	}
	defer sqlDst.Close()
	if apply && (len(pools) > 0 || len(rewards) > 0) {
		if err := InitGameplayPoolSchema(ctx, dst); err != nil {
			return nil, err
		}
	}
	// 固定连接持有命名锁，涵盖事务与提交后的 AUTO_INCREMENT DDL。
	err = dst.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{NewDB: true})
		var locked int
		lockName := fmt.Sprintf("hyjs-migrate-%x", sha256.Sum256([]byte(cfg.Database)))[:60]
		if err := conn.Raw("SELECT GET_LOCK(?,10)", lockName).Scan(&locked).Error; err != nil {
			return err
		}
		if locked != 1 {
			return errors.New("migration lock unavailable")
		}
		defer conn.Exec("SELECT RELEASE_LOCK(?)", lockName)
		err := conn.Transaction(func(tx *gorm.DB) error {
			for _, p := range profiles {
				var existing []Profile
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? OR uid=?", p.Id, p.Uid).Find(&existing).Error; err != nil {
					return err
				}
				if len(existing) > 0 {
					if len(existing) != 1 || !reflect.DeepEqual(p, existing[0]) {
						return fmt.Errorf("profile %d conflicts with destination", p.Id)
					}
					continue
				}
				report.Inserted++
				if apply {
					if err := tx.Table("profiles").Create(map[string]any{"id": p.Id, "uid": p.Uid, "session": p.Session, "player": p.Player, "game": p.Game, "created_at": p.CreatedAt, "password_salt": p.PasswordSalt, "password_hash": p.PasswordHash}).Error; err != nil {
						return fmt.Errorf("insert profile %d: %w", p.Id, err)
					}
				}
			}
			for _, p := range pools {
				var old GameplayPool
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id=?", p.ID).Error
				if err == nil {
					if !reflect.DeepEqual(p, old) {
						return fmt.Errorf("pool %s conflicts with destination", p.ID)
					}
				} else if errors.Is(err, gorm.ErrRecordNotFound) {
					if apply {
						if err := tx.Create(&p).Error; err != nil {
							return err
						}
					}
				} else {
					return err
				}
			}
			for _, r := range rewards {
				var old GameplayPoolReward
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "pool_id=? AND id=?", r.PoolID, r.ID).Error
				if err == nil {
					if !reflect.DeepEqual(r, old) {
						return fmt.Errorf("pool reward %s/%d conflicts with destination", r.PoolID, r.ID)
					}
				} else if errors.Is(err, gorm.ErrRecordNotFound) {
					if apply {
						if err := tx.Create(&r).Error; err != nil {
							return err
						}
					}
				} else {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		var next int64
		if err := conn.Raw("SELECT COALESCE(AUTO_INCREMENT,1) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles'").Scan(&next).Error; err != nil {
			return err
		}
		if next-1 > report.Sequence {
			report.Sequence = next - 1
		}
		if apply && next <= sequence {
			if err := conn.Exec(fmt.Sprintf("ALTER TABLE profiles AUTO_INCREMENT=%d", sequence+1)).Error; err != nil {
				return err
			}
		}
		// 导入后逐行复读，区分 NULL 与空 blob，并验证原始存档文本没有被重编码。
		if apply {
			for _, p := range profiles {
				var old Profile
				if err := conn.First(&old, p.Id).Error; err != nil {
					return err
				}
				if !reflect.DeepEqual(p, old) {
					return fmt.Errorf("profile %d verification failed", p.Id)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("destination migration: %w", err)
	}
	return report, nil
}
