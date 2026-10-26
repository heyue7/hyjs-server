package constUtil

// P7RushReadRequestCacheKey scopes successful historical verification to one request.
type P7RushReadRequestCacheKey struct{}

const P7RushReadRequestCacheLimit = 128

// P7RushHistoricalQueryKey is set only by the pure-query dispatcher.
type P7RushHistoricalQueryKey struct{}
