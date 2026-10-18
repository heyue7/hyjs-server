package constUtil

type adminServerScopeKey struct{}

// AdminServerScopeContextKey carries a validated admin realm selection.
var AdminServerScopeContextKey = adminServerScopeKey{}

type personalTurntableContextKey struct{}

// PersonalTurntableOwnerKey is set only by an authenticated owner consumer.
var PersonalTurntableOwnerKey = personalTurntableContextKey{}
