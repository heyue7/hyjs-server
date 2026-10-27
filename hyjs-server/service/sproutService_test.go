package service

import (
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
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

func TestSproutStatus78AdminSettingsSceneAndSession(t *testing.T) {
	f, expiredID, returnID := p10SceneSetup(t)
	if err := f.s.InitializeAdminSproutSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	cfg, err := f.s.AdminSproutSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	days, offlineDays, returnDays := int64(2), int64(7), int64(3)
	save := func(enabled, newEnabled, returnEnabled bool) {
		t.Helper()
		cfg, err = f.s.AdminSaveSproutSettings(f.ctx, actor, model.AdminSproutSettingsSaveRequest{
			Enabled: enabled, Days: &days, NewCharacterEnabled: &newEnabled,
			ReturnEnabled: &returnEnabled, ReturnOfflineDays: &offlineDays, ReturnDays: &returnDays,
			Version: cfg.Version, Reason: "小芽状态隔离回归",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	save(true, true, true)
	var own db.Profile
	if err := db.GetIns().First(&own, f.id).Error; err != nil {
		t.Fatal(err)
	}
	status := func(ids ...int64) (*model.SproutStatusResponse, error) {
		t.Helper()
		return f.s.SproutStatus(f.ctx, model.SproutStatusRequest{PlayerID: f.id, PlayerIDs: ids}, own.Session)
	}
	check := func(out *model.SproutStatusResponse, ids []int64, active []bool) {
		t.Helper()
		if out == nil || out.Icon != "ggo_sprout" || len(out.Players) != len(ids) || out.ServerTime <= 0 {
			t.Fatalf("status envelope: %+v", out)
		}
		for i, player := range out.Players {
			if player.PlayerID != ids[i] || player.Active != active[i] || (player.ExpiresAt > out.ServerTime) != active[i] {
				t.Fatalf("status %d: %+v", i, player)
			}
		}
	}
	// A new role remains queryable before its first scene-presence update.
	out, err := status(f.id)
	if err != nil {
		t.Fatal(err)
	}
	check(out, []int64{f.id}, []bool{true})
	badSession := append([]byte(nil), own.Session...)
	badSession[0] ^= 1
	if _, err := f.s.SproutStatus(f.ctx, model.SproutStatusRequest{PlayerID: f.id, PlayerIDs: []int64{f.id}}, badSession); err == nil {
		t.Fatal("wrong session accepted")
	}
	now := time.Now().UTC()
	old := now.Add(-30 * 24 * time.Hour).UnixMilli()
	last := now.Add(-8 * 24 * time.Hour).UnixMilli()
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", expiredID).Updates(map[string]any{"created_at": old, "return_sprout_started_at": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", returnID).Updates(map[string]any{"created_at": old, "last_sprout_login_at": last, "return_sprout_started_at": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordSproutLogin(f.ctx, returnID); err != nil {
		t.Fatal(err)
	}
	var returned db.Profile
	if err := db.GetIns().First(&returned, returnID).Error; err != nil || returned.ReturnSproutStartedAt < now.UnixMilli() {
		t.Fatal("return login did not activate", err)
	}
	start := returned.ReturnSproutStartedAt
	if err := f.s.RecordSproutLogin(f.ctx, returnID); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&returned, returnID).Error; err != nil || returned.ReturnSproutStartedAt != start {
		t.Fatal("return login retry extended the award", err)
	}
	for _, id := range []int64{f.id, expiredID, returnID} {
		p10SceneConnect(t, f, id, &p10SceneRecorder{})
	}
	ids := []int64{f.id, expiredID, returnID}
	out, err = status(ids...)
	if err != nil {
		t.Fatal(err)
	}
	check(out, ids, []bool{true, false, true})
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, returnID, &pb.CSTransferLine{Line: proto.Int32(2)}, 78001); err != nil || !handled {
		t.Fatal("line transfer", handled, err)
	}
	if _, err := status(ids...); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("different-line batch was not denied", err)
	}
	out, err = status(f.id)
	if err != nil {
		t.Fatal("owner-only retry failed", err)
	}
	check(out, []int64{f.id}, []bool{true})
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, returnID, &pb.CSTransferLine{Line: proto.Int32(1)}, 78002); err != nil || !handled {
		t.Fatal("line return", handled, err)
	}
	save(true, false, true)
	out, err = status(ids...)
	if err != nil {
		t.Fatal(err)
	}
	check(out, ids, []bool{false, false, true})
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", returnID).Update("return_sprout_started_at", now.Add(-4*24*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	out, err = status(ids...)
	if err != nil {
		t.Fatal(err)
	}
	check(out, ids, []bool{false, false, false})
	save(false, true, true)
	out, err = status(ids...)
	if err != nil {
		t.Fatal(err)
	}
	check(out, ids, []bool{false, false, false})
	if _, err := LoginAccount(f.ctx, own.Uid, "local-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := status(f.id); err == nil {
		t.Fatal("rotated session accepted")
	}
}
