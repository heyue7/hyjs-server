package constUtil

// P7RushReadRequestCacheKey scopes successful historical verification to one request.
type P7RushReadRequestCacheKey struct{}

const P7RushReadRequestCacheLimit = 128

// P7RushHistoricalQueryKey is set only by the pure-query dispatcher.
type P7RushHistoricalQueryKey struct{}

const (
	P7RushHistoricalReadCacheLimit        = 32
	P7RushHistoricalReadCacheTTLSeconds   = 900
	P7RushHistoricalReadCacheDecodedBytes = 2 * 1024 * 1024
	P7RushHistoricalReadCacheEntryBytes   = 256 * 1024
	P7RushHistoricalReadCacheTotalBytes   = 2 * 1024 * 1024
)
