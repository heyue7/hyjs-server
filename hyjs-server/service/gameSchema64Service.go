package service

import (
	"context"
	"database/sql"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// gameSchema64Snapshot owns only startup table existence; business rows remain live.
type gameSchema64Snapshot struct {
	owner    *GameService
	database *gorm.DB
	pool     gorm.ConnPool
	tables   map[string]struct{}
}

// InitializeGameSchema64 records ordinary tables after every startup migration succeeds.
func (s *GameService) InitializeGameSchema64(ctx context.Context) error {
	database := db.GetIns()
	owner := s.p10ClientOwner()
	owner.gameSchema64.Store(nil)
	var names []string
	query := database.WithContext(ctx)
	switch database.Dialector.Name() {
	case "mysql":
		query = query.Raw("SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE'")
	case "sqlite":
		query = query.Raw("SELECT name FROM sqlite_master WHERE type='table'")
	default:
		return nil
	}
	if err := query.Scan(&names).Error; err != nil {
		return err
	}
	tables := make(map[string]struct{}, len(names))
	for _, name := range names {
		tables[name] = struct{}{}
	}
	owner.gameSchema64.Store(&gameSchema64Snapshot{owner: owner, database: database, pool: database.Config.ConnPool, tables: tables})
	return nil
}

// gameSchemaHasTable retains dynamic checks outside its initialized owner/database.
func gameSchemaHasTable(tx *gorm.DB, table any) bool {
	if tx != nil && tx.Statement != nil && tx.Statement.Context != nil {
		if service, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && service != nil {
			owner := service.p10ClientOwner()
			snapshot := owner.gameSchema64.Load()
			if snapshot != nil && snapshot.owner == owner && snapshot.database == db.GetIns() && gameSchema64PoolKnown(tx, snapshot.pool) && tx.Config.ConnPool == snapshot.pool && snapshot.database.Config.ConnPool == snapshot.pool && tx.Statement.Table == "" && tx.Statement.TableExpr == nil {
				name, ok := table.(string)
				if !ok {
					statement := &gorm.Statement{DB: tx}
					if statement.Parse(table) == nil && statement.Schema != nil {
						name = statement.Schema.Table
					}
				}
				if name != "" && !strings.ContainsAny(name, ". `\"()\t\n") {
					if _, exists := snapshot.tables[name]; exists {
						return true
					}
				}
			}
		}
	}
	return tx.Migrator().HasTable(table)
}

func gameSchema64PoolKnown(tx *gorm.DB, pool gorm.ConnPool) bool {
	// Standard transactions retain Config.ConnPool while Statement owns sql.Tx.
	if _, ok := pool.(*sql.DB); !ok {
		return false
	}
	if tx.Statement.ConnPool == pool {
		return true
	}
	_, ok := tx.Statement.ConnPool.(*sql.Tx)
	return ok
}
