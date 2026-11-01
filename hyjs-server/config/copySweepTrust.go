package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// CopySweepHistoryComplete binds an operator-verified legacy grant history to
// one configured realm. An absent or zero value keeps the old timestamp guard.
func CopySweepHistoryComplete(serverID int64) (bool, error) {
	value := strings.TrimSpace(os.Getenv("HYJS_COPY_SWEEP_P9_COMPLETE_SERVER"))
	if value == "" || value == "0" {
		return false, nil
	}
	id, err := strconv.ParseInt(value, 10, 32)
	if err != nil || id < 1 || id != serverID || strconv.FormatInt(id, 10) != value {
		return false, errors.New("扫荡历史证明配置与当前区服不匹配")
	}
	return true, nil
}
