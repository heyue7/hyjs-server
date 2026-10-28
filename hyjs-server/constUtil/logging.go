package constUtil

import "time"

// Application logs have age and capacity bounds; business ledgers keep their own lifecycle.
const (
	LogDefaultMaxSizeMB    = 5
	LogDefaultMaxBackups   = 2
	LogMaxAgeDays          = 1
	LogRotationInterval    = time.Hour
	LogMaintenanceInterval = time.Minute
	LogSampleInterval      = time.Minute
	LogSampleInitial       = 5
	LogSampleThereafter    = 100
)
