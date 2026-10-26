package service

import (
	"hyjs-server/db"
	"time"
)

// sproutExpiry uses real UTC elapsed time; nil new-character switch preserves the legacy rule.
func sproutExpiry(cfg db.AdminSproutSetting, row db.Profile, now time.Time) int64 {
	if !cfg.Enabled {
		return 0
	}
	var expiry int64
	if (cfg.NewCharacterEnabled == nil || *cfg.NewCharacterEnabled) && cfg.Days != nil && *cfg.Days > 0 && row.CreatedAt != nil && *row.CreatedAt > 0 && *row.CreatedAt <= now.UnixMilli() {
		expiry = *row.CreatedAt + *cfg.Days*int64(24*time.Hour/time.Millisecond)
	}
	if cfg.ReturnEnabled != nil && *cfg.ReturnEnabled && cfg.ReturnDays != nil && *cfg.ReturnDays > 0 && row.ReturnSproutStartedAt > 0 && row.ReturnSproutStartedAt <= now.UnixMilli() {
		expiry = max(expiry, row.ReturnSproutStartedAt+*cfg.ReturnDays*int64(24*time.Hour/time.Millisecond))
	}
	if expiry <= now.UnixMilli() {
		return 0
	}
	return expiry
}

func sproutReturnStart(cfg db.AdminSproutSetting, row db.Profile, presenceAt int64, now time.Time) int64 {
	last := max(row.LastSproutLoginAt, presenceAt)
	if !cfg.Enabled || cfg.ReturnEnabled == nil || !*cfg.ReturnEnabled || cfg.ReturnOfflineDays == nil || cfg.ReturnDays == nil || *cfg.ReturnOfflineDays < 1 || *cfg.ReturnDays < 1 || last <= 0 || last > now.UnixMilli() {
		return row.ReturnSproutStartedAt
	}
	if now.UnixMilli()-last >= *cfg.ReturnOfflineDays*int64(24*time.Hour/time.Millisecond) {
		return now.UnixMilli()
	}
	return row.ReturnSproutStartedAt
}
