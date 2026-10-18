package constUtil

import "time"

const (
	BackgroundWorkerInitialBackoff = time.Second
	BackgroundWorkerLateBackoff    = 15 * time.Second
	BackgroundWorkerMaxBackoff     = 30 * time.Second
	BackgroundWorkerHealthyWindow  = time.Minute
	BackgroundWorkerFailure        = "background worker %s: %w"
	BackgroundWorkerUnexpectedStop = "background worker %s stopped without service cancellation"
)
