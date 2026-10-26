package model

// AdminCopyDropsResponse configures the named local fragment cadence.
type AdminCopyDropsResponse struct {
	Enabled bool   `json:"enabled"`
	Cycle   int64  `json:"cycle"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// AdminCopyDropsSaveRequest saves a bounded cadence with an audit reason.
type AdminCopyDropsSaveRequest struct {
	Enabled bool   `json:"enabled"`
	Cycle   int64  `json:"cycle"`
	Version string `json:"version" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
}
