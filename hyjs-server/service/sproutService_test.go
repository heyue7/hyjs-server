package service

import (
	"hyjs-server/db"
	"testing"
	"time"
)

func TestSproutRealTimeBoundaries73(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	day := int64(24 * time.Hour / time.Millisecond)
	days := int64(3)
	yes, no := true, false
	created := now.UnixMilli() - day
	cfg := db.AdminSproutSetting{Enabled: true, Days: &days}
	row := db.Profile{CreatedAt: &created}
	if sproutExpiry(cfg, row, now) != created+3*day {
		t.Fatal("legacy eligibility lost")
	}
	cfg.NewCharacterEnabled = &no
	if sproutExpiry(cfg, row, now) != 0 {
		t.Fatal("disabled new rule active")
	}
	cfg.ReturnEnabled = &yes
	cfg.ReturnDays = &days
	row.ReturnSproutStartedAt = now.UnixMilli() - 3*day
	if sproutExpiry(cfg, row, now) != 0 {
		t.Fatal("expiry boundary active")
	}
	row.ReturnSproutStartedAt = now.UnixMilli() - day
	if sproutExpiry(cfg, row, now) <= now.UnixMilli() {
		t.Fatal("return inactive")
	}
	cfg.Enabled = false
	if sproutExpiry(cfg, row, now) != 0 {
		t.Fatal("master switch ignored")
	}
}
func TestSproutReturnLoginIdempotency73(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	day := int64(24 * time.Hour / time.Millisecond)
	days := int64(7)
	duration := int64(3)
	yes := true
	cfg := db.AdminSproutSetting{Enabled: true, ReturnEnabled: &yes, ReturnDays: &duration, ReturnOfflineDays: &days}
	row := db.Profile{}
	if sproutReturnStart(cfg, row, 0, now) != 0 {
		t.Fatal("unknown history awarded")
	}
	row.LastSproutLoginAt = now.UnixMilli() - 7*day
	if sproutReturnStart(cfg, row, now.UnixMilli()-day, now) != 0 {
		t.Fatal("long online user counted as offline")
	}
	start := sproutReturnStart(cfg, row, 0, now)
	if start != now.UnixMilli() {
		t.Fatal("exact threshold denied")
	}
	row.LastSproutLoginAt = now.UnixMilli()
	row.ReturnSproutStartedAt = start
	if sproutReturnStart(cfg, row, 0, now.Add(time.Second)) != start {
		t.Fatal("retry extended expiry")
	}
	row.LastSproutLoginAt = now.UnixMilli() - 7*day + 1
	row.ReturnSproutStartedAt = 0
	if sproutReturnStart(cfg, row, 0, now) != 0 {
		t.Fatal("pre-threshold awarded")
	}
}
