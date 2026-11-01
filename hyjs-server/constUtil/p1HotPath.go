package constUtil

import "errors"

// P1SharedReadContextKey marks a transaction that must roll back before any
// operation requiring exclusive activity-window ownership.
type P1SharedReadContextKey struct{}

// ErrP1ExclusiveRetry asks the owning transaction to retry under its original
// exclusive activity locks. It must not be handled inside the transaction.
var ErrP1ExclusiveRetry = errors.New("activity window requires an exclusive transaction retry")

// ErrP1GatherCatalogWriteRetry requires a full retry with catalogue write
// locks acquired before any period or resource locks.
var ErrP1GatherCatalogWriteRetry = errors.New("gather activity catalogue requires a write transaction retry")
