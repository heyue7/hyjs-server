package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p1GatherCatalogIntentKey struct{}
type p1GatherCatalogLeaseKey struct{}
type p1GatherCatalogIntent struct{ write bool }
type p1GatherCatalogLease struct {
	tx    *sql.Tx
	write bool
	rc    bool
}

// The marker is set outside the transaction; every retry must acquire a new
// lease after locking its own profile and before reading any activity rows.
func p1GatherCatalogContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, p1GatherCatalogIntentKey{}, p1GatherCatalogIntent{})
}

func p1GatherCatalogWriteContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, p1GatherCatalogIntentKey{}, p1GatherCatalogIntent{write: true})
}

func p1GatherCatalogKinds() []string {
	return []string{constUtil.P8EventFishGather, constUtil.P8EventMineGather, constUtil.P4KindPower}
}

func p1GatherCatalogKind(kind string) bool {
	return kind == constUtil.P8EventFishGather || kind == constUtil.P8EventMineGather || kind == constUtil.P4KindPower
}

func p1InitializeGatherCatalog(tx *gorm.DB) error {
	for _, kind := range p1GatherCatalogKinds() {
		row := db.P1GatherCatalogGuard{Kind: kind}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	var count int64
	if err := tx.Model(&db.P1GatherCatalogGuard{}).Where("kind IN ?", p1GatherCatalogKinds()).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(p1GatherCatalogKinds())) {
		return errors.New("gather activity catalogue guards are incomplete")
	}
	return nil
}

func p1AcquireGatherCatalog(tx *gorm.DB) (*gorm.DB, error) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return nil, errors.New("gather activity catalogue transaction is missing")
	}
	intent, marked := tx.Statement.Context.Value(p1GatherCatalogIntentKey{}).(p1GatherCatalogIntent)
	if !marked {
		return tx, nil
	}
	// A gather retry without the shared marker can need first freeze. Acquire
	// the directory write gates before any original exclusive period locks.
	return p1LockGatherCatalog(tx, intent.write || !p1SharedRead(tx))
}

// The transaction owner calls this only when it started this exact transaction
// with READ COMMITTED on MySQL. SQLite retains its single-writer semantics.
// An unmarked or original exclusive transaction never receives the capability.
func p1EnableGatherSnapshotRC(tx *gorm.DB) (*gorm.DB, error) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return nil, errors.New("gather activity catalogue transaction is missing")
	}
	intent, marked := tx.Statement.Context.Value(p1GatherCatalogIntentKey{}).(p1GatherCatalogIntent)
	if !marked || intent.write || !p1SharedRead(tx) {
		return tx, nil
	}
	actual, ok := tx.Statement.ConnPool.(*sql.Tx)
	lease, held := tx.Statement.Context.Value(p1GatherCatalogLeaseKey{}).(p1GatherCatalogLease)
	if !ok || actual == nil || !held || lease.tx != actual || lease.write {
		return nil, errors.New("gather activity catalogue read lease is missing")
	}
	lease.rc = true
	return tx.WithContext(context.WithValue(tx.Statement.Context, p1GatherCatalogLeaseKey{}, lease)), nil
}

func p1GatherSnapshotRC(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil || !p1SharedRead(tx) {
		return false
	}
	intent, marked := tx.Statement.Context.Value(p1GatherCatalogIntentKey{}).(p1GatherCatalogIntent)
	actual, ok := tx.Statement.ConnPool.(*sql.Tx)
	lease, held := tx.Statement.Context.Value(p1GatherCatalogLeaseKey{}).(p1GatherCatalogLease)
	return marked && !intent.write && ok && actual != nil && held && lease.tx == actual && !lease.write && lease.rc
}

// Writers take all gates in the same order as readers before locking any
// period. This also covers changing an existing period's event kind.
func p1AcquireGatherCatalogWriter(tx *gorm.DB) (*gorm.DB, error) {
	return p1LockGatherCatalog(tx, true)
}

func p1LockGatherCatalog(tx *gorm.DB, write bool) (*gorm.DB, error) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return nil, errors.New("gather activity catalogue transaction is missing")
	}
	actual, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok || actual == nil {
		return nil, errors.New("gather activity catalogue requires a database transaction")
	}
	lease, held := tx.Statement.Context.Value(p1GatherCatalogLeaseKey{}).(p1GatherCatalogLease)
	if held && lease.tx == actual {
		if write && !lease.write {
			return nil, constUtil.ErrP1GatherCatalogWriteRetry
		}
		return tx, nil
	}
	strength := "SHARE"
	if write {
		strength = "UPDATE"
	}
	for _, kind := range p1GatherCatalogKinds() {
		var row db.P1GatherCatalogGuard
		if err := tx.Clauses(clause.Locking{Strength: strength}).First(&row, "kind = ?", kind).Error; err != nil {
			return nil, fmt.Errorf("lock gather activity catalogue %s: %w", kind, err)
		}
	}
	lease = p1GatherCatalogLease{tx: actual, write: write}
	return tx.WithContext(context.WithValue(tx.Statement.Context, p1GatherCatalogLeaseKey{}, lease)), nil
}

func p1RequireGatherCatalog(tx *gorm.DB, kind string, write bool) error {
	if !p1GatherCatalogKind(kind) {
		return nil
	}
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return errors.New("gather activity catalogue transaction is missing")
	}
	actual, ok := tx.Statement.ConnPool.(*sql.Tx)
	lease, held := tx.Statement.Context.Value(p1GatherCatalogLeaseKey{}).(p1GatherCatalogLease)
	if ok && actual != nil && held && lease.tx == actual && (!write || lease.write) {
		return nil
	}
	if write {
		return constUtil.ErrP1GatherCatalogWriteRetry
	}
	return errors.New("gather activity catalogue guard was not acquired in this transaction")
}
