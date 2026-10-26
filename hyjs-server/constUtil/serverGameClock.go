package constUtil

// ServerGameClockContextKey separates the server calendar from legacy demo tools.
type ServerGameClockContextKey string

const (
	ServerGameClockBoundKey    ServerGameClockContextKey = "server_game_clock_bound"
	ServerGameClockOwnerKey    ServerGameClockContextKey = "server_game_clock_owner"
	ServerGameClockServerKey   ServerGameClockContextKey = "server_game_clock_server"
	ServerGameClockOffsetKey   ServerGameClockContextKey = "server_game_clock_offset"
	ServerGameClockRevisionKey ServerGameClockContextKey = "server_game_clock_revision"
	ServerGameClockHighDaysKey ServerGameClockContextKey = "server_game_clock_high_days"
	ServerGameClockScope                                 = "server"
	ServerGameClockDefaultID   int64                     = 1
	ServerGameClockMaxDays     int64                     = 3650
)
