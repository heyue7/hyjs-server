package constUtil

import "errors"

// ErrConfigurationReadRetry rolls back a read transaction when a concurrent
// operator edit requires first-use freezing with an exclusive row lock.
var ErrConfigurationReadRetry = errors.New("configuration changed during first-use read; retry transaction")

var ErrExchangeDirectoryRetry = errors.New("exchange directory requires a publication transaction")

type ExchangeDirectoryReadContextKey struct{}
type ExchangeDirectoryWriteContextKey struct{}
