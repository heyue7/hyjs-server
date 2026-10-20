package service

import (
	"context"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"path/filepath"
	"testing"
	"time"
)

func TestAdminGuildRewards31CASPermissionFutureAndFrozen(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "guild-rewards.sqlite")); err != nil {
		t.Fatal(err)
	}
	tx := db.GetIns()
	connection, err := tx.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	if err = tx.AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}, &db.P13GuildPolicy{}, &db.P13UnionFightSettlementPolicy{}, &db.P13UnionFightSettlementOffer{}, &db.P13UnionFightMatch{}, &db.P13UnionFightWeeklyPolicy{}, &db.P13UnionFightWeeklyOffer{}, &db.P13UnionFightMonthlyPolicy{}, &db.P13UnionFightMonthlyOffer{}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Create(&db.P13GuildPolicy{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "reward-operator", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err = tx.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username}
	s := &GameService{}
	req := model.AdminGuildRewardSaveRequest{AdminGuildRewardPolicy: model.AdminGuildRewardPolicy{Kind: "monthly", Month: "2099-01", Version: "first", Source: "explicit test policy"}, Reason: "创建未来期"}
	if _, err = s.AdminSaveGuildReward(ctx, nil, req, true); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatalf("nil actor accepted: %v", err)
	}
	row, err := s.AdminSaveGuildReward(ctx, actor, req, true)
	if err != nil {
		t.Fatal(err)
	}
	req.RowVersion = row.RowVersion
	req.Version = "second"
	changed, err := s.AdminSaveGuildReward(ctx, actor, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminSaveGuildReward(ctx, actor, req, false); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	req.RowVersion = changed.RowVersion
	frozen := db.P13UnionFightMonthlyOffer{Month: "2099-01", Rules: []byte("untouched immutable snapshot"), RulesHash: "frozen-hash"}
	if err = tx.Create(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminSaveGuildReward(ctx, actor, req, false); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("frozen period accepted: %v", err)
	}
	var actual db.P13UnionFightMonthlyOffer
	if err = tx.First(&actual, "month=?", frozen.Month).Error; err != nil {
		t.Fatal(err)
	}
	if string(actual.Rules) != string(frozen.Rules) || actual.RulesHash != frozen.RulesHash {
		t.Fatal("frozen offer changed")
	}
	req.RowVersion = ""
	req.Month = "2000-01"
	if _, err = s.AdminSaveGuildReward(ctx, actor, req, true); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("past period accepted: %v", err)
	}
	// Updating the group's publication leaves the previous match's immutable rules intact.
	settlement := db.P13UnionFightSettlementPolicy{Group: "A", Version: "old", Source: "old publication"}
	if err = tx.Create(&settlement).Error; err != nil {
		t.Fatal(err)
	}
	oldOffer := db.P13UnionFightSettlementOffer{MatchID: "old-match", Rules: []byte("old settlement rules"), RulesHash: "old-hash"}
	if err = tx.Create(&oldOffer).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Create(&db.P13UnionFightMatch{ID: "old-match", Group: "A", LeftPreview: []byte{}, RightPreview: []byte{}, Roster: []byte{}}).Error; err != nil {
		t.Fatal(err)
	}
	settleReq := model.AdminGuildRewardSaveRequest{AdminGuildRewardPolicy: model.AdminGuildRewardPolicy{Kind: "settlement", Group: "A", Version: "next", Source: "new matches only"}, RowVersion: adminVersion(settlement), Reason: "调整后续场次"}
	if _, err = s.AdminSaveGuildReward(ctx, actor, settleReq, false); err != nil {
		t.Fatal(err)
	}
	var kept db.P13UnionFightSettlementOffer
	if err = tx.First(&kept, "match_id=?", oldOffer.MatchID).Error; err != nil {
		t.Fatal(err)
	}
	if string(kept.Rules) != string(oldOffer.Rules) || kept.RulesHash != oldOffer.RulesHash {
		t.Fatal("settlement snapshot changed")
	}
	listed, err := s.AdminGuildRewards(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listed.Items {
		if entry.Kind == "settlement" && (!entry.Frozen || !entry.Editable) {
			t.Fatal("new match publication should remain editable")
		}
		if entry.Kind == "monthly" && entry.Month == "2099-01" && (!entry.Frozen || entry.Editable) {
			t.Fatal("frozen month should be read only")
		}
	}
	if err = tx.Model(&user).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	req.Month = "2099-02"
	if _, err = s.AdminSaveGuildReward(ctx, actor, req, true); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatalf("revoked write accepted: %v", err)
	}
}

func TestAdminGuildRewards31PeriodBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	if adminGuildRewardFuture("weekly", "2026-10-19", now) || adminGuildRewardFuture("weekly", "2026-10-27", now) || !adminGuildRewardFuture("weekly", "2026-10-26", now) || adminGuildRewardFuture("monthly", "2026-10", now) || !adminGuildRewardFuture("monthly", "2026-11", now) {
		t.Fatal("period start boundaries changed")
	}
}
