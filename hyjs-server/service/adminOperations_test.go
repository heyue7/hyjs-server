package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminOperationsCRUDAndDelivery checks real configuration writes, RBAC, frozen conflicts,
// original mail delivery and once-only cold claiming in one finite owned fixture.
func TestAdminOperationsCRUDAndDelivery(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminOperations(f.ctx); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns().WithContext(f.ctx)
	if err := d.AutoMigrate(&db.P4CommerceRule{}, &db.P4WelfareRule{}, &db.P4ActivityRegistry{}, &db.GameplayPool{}, &db.GameplayPoolReward{}, &db.TurntableRules{}, &db.KingRecruitRules{}); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "crud-operator-" + strings.ToLower(t.Name()), PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	viewer := db.AdminUser{Username: "crud-viewer-" + strings.ToLower(t.Name()), PasswordHash: "unused-fixture", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := d.Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	op := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	read := &model.AdminPrincipal{UserID: viewer.ID, Username: viewer.Username, Role: viewer.Role}
	now := time.Now().UnixMilli()
	products := p4CommerceRules{Products: []p4Product{p4BaseProduct("admin_product", "item_005,1", "diamond,10", 1, 1)}}
	commerce := db.P4CommerceRule{ID: "p4:ordinary:admin_template", Kind: "ordinary", ShopType: 1, Enabled: false, StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Rules: adminJSON(products), Source: "finite admin CRUD fixture"}
	welfare := db.P4WelfareRule{ID: "p4:login:admin_template", Kind: constUtil.P4WelfareLogin, Enabled: false, StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Rules: adminJSON(p4WelfareRules{Days: []p4SignDay{{Rewards: "coin,11"}}}), Source: "finite admin CRUD fixture"}
	period := db.PeriodInstance{ID: "totalLogin_p1_admin_template", Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Title: "运营模板", Goal: 1, Rewards: "coin,17", StartAt: now - 1000, EndAt: now + 86400000, RewardEndAt: now + 86400000, Source: "finite admin CRUD fixture"}
	pool := db.GameplayPool{ID: "admin_wheel_template", Name: "运营转盘模板", Source: "finite admin CRUD fixture", MinLevel: 1, SingleCost: "diamond,50", TenCost: "diamond,488", FreeSeconds: 1, CoinPerDraw: 10000, StartAt: now - 1000, EndAt: now + 86400000, RewardEndAt: now + 86400000}
	for _, row := range []any{&commerce, &welfare, &period, &pool} {
		if err := d.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	wheel := db.TurntableRules{PoolID: pool.ID, ScorePerDraw: 1, TopPrize: "diamond,1", AdvancePrize: "diamond,1", AdvancePlaces: 1, Description: "finite fixture"}
	if err := d.Create(&wheel).Error; err != nil {
		t.Fatal(err)
	}
	king := db.KingRecruitRules{ID: 91, WheelPool: pool.ID, Source: "finite selected source fixture"}
	if err := d.Create(&king).Error; err != nil {
		t.Fatal(err)
	}
	rewards := []db.GameplayPoolReward{}
	for i := int32(0); i < 9; i++ {
		rewards = append(rewards, db.GameplayPoolReward{PoolID: pool.ID, ID: i, ResourceID: "coin", Amount: 10, Weight: 1})
	}
	if err := d.Create(&rewards).Error; err != nil {
		t.Fatal(err)
	}
	sources := []model.AdminActivityDTO{adminActivityProjection("commerce", commerce, nil), adminActivityProjection("welfare", welfare, nil), adminActivityProjection("period", period, nil), adminActivityProjection("pool", pool, rewards)}
	for _, source := range sources {
		var cfg map[string]any
		if err := json.Unmarshal(source.Config, &cfg); err != nil {
			t.Fatal(err)
		}
		domain, key, _ := strings.Cut(source.ID, ":")
		newKey := key + "_next"
		if domain == "pool" {
			cfg["pool"].(map[string]any)["id"] = newKey
		} else {
			cfg["id"] = newKey
		}
		request := model.AdminActivityCreateRequest{TemplateID: source.ID, NewID: newKey, Enabled: true, Config: adminJSON(cfg)}
		if _, err := f.s.AdminCreateActivity(f.ctx, read, request); !errors.Is(err, constUtil.ErrAdminPermission) {
			t.Fatal("viewer created activity", err)
		}
		created, err := f.s.AdminCreateActivity(f.ctx, op, request)
		if err != nil || created.ID != domain+":"+newKey || !created.Enabled {
			t.Fatal("activity create", source.ID, err)
		}
		if _, err := f.s.AdminCreateActivity(f.ctx, op, request); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatal("duplicate ID overwrote activity", err)
		}
		update := model.AdminActivityUpdateRequest{Enabled: false, Version: created.Version, Config: created.Config}
		updated, err := f.s.AdminUpdateActivity(f.ctx, op, created.ID, update)
		if err != nil || updated.Enabled {
			t.Fatal("activity disable", source.ID, err)
		}
		if _, err := f.s.AdminUpdateActivity(f.ctx, op, created.ID, update); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatal("stale activity version accepted", err)
		}
	}
	var copiedWheel db.TurntableRules
	if err := d.First(&copiedWheel, "pool_id = ?", pool.ID+"_next").Error; err != nil || copiedWheel.TopPrize != wheel.TopPrize {
		t.Fatal("wheel companion config not cloned", err)
	}
	var selected db.KingRecruitRules
	if err := d.First(&selected, king.ID).Error; err != nil || selected.WheelPool != king.WheelPool {
		t.Fatal("pool creation changed selected source", err)
	}
	// New drafts may remain empty; enabling an empty original period is rejected atomically.
	empty := period
	empty.ID = period.ID + "_empty"
	empty.Rewards = ""
	emptyRequest := model.AdminActivityCreateRequest{TemplateID: "period:" + period.ID, NewID: empty.ID, Enabled: true, Config: adminJSON(empty)}
	if _, err := f.s.AdminCreateActivity(f.ctx, op, emptyRequest); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("empty reward activity enabled or not classified as client configuration error", err)
	}
	if p1Count(t, &db.PeriodInstance{}, "id = ?", empty.ID) != 0 {
		t.Fatal("invalid activity persisted")
	}
	emptyRequest.Enabled = false
	if _, err := f.s.AdminCreateActivity(f.ctx, op, emptyRequest); err != nil {
		t.Fatal("explicit empty draft rejected", err)
	}
	frozen := adminJSON(period)
	if err := d.Model(&db.PeriodInstance{}).Where("id = ?", period.ID).Update("frozen", []byte(frozen)).Error; err != nil {
		t.Fatal(err)
	}
	var current db.PeriodInstance
	if err := d.First(&current, "id = ?", period.ID).Error; err != nil {
		t.Fatal(err)
	}
	dto := adminActivityProjection("period", current, nil)
	current.Rewards = "coin,999"
	if _, err := f.s.AdminUpdateActivity(f.ctx, op, dto.ID, model.AdminActivityUpdateRequest{Enabled: false, Version: dto.Version, Config: adminJSON(current)}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("frozen activity changed", err)
	}
	var unchanged db.PeriodInstance
	if err := d.First(&unchanged, "id = ?", period.ID).Error; err != nil || string(unchanged.Frozen) != string(frozen) || unchanged.Rewards != period.Rewards {
		t.Fatal("frozen history overwritten", err)
	}
	// A new copy of a frozen template starts unfrozen, while old player progress stays byte-for-byte intact.
	progress := db.PeriodProgress{InstanceID: period.ID, PlayerID: f.id, Snapshot: frozen, Value: 1, LastDay: "2026-10-17"}
	if err := d.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	oldProgress, _ := json.Marshal(progress)
	copyPeriod := period
	copyPeriod.ID = period.ID + "_frozen_copy"
	if _, err := f.s.AdminCreateActivity(f.ctx, op, model.AdminActivityCreateRequest{TemplateID: "period:" + period.ID, NewID: copyPeriod.ID, Enabled: true, Config: adminJSON(copyPeriod)}); err != nil {
		t.Fatal("copy frozen template", err)
	}
	var copiedPeriod db.PeriodInstance
	if err := d.First(&copiedPeriod, "id = ?", copyPeriod.ID).Error; err != nil || len(copiedPeriod.Frozen) != 0 {
		t.Fatal("new activity inherited old frozen snapshot", err)
	}
	var keptProgress db.PeriodProgress
	if err := d.First(&keptProgress, "instance_id = ? AND player_id = ?", period.ID, f.id).Error; err != nil {
		t.Fatal(err)
	}
	keptRaw, _ := json.Marshal(keptProgress)
	if string(keptRaw) != string(oldProgress) || string(keptProgress.Snapshot) != string(frozen) {
		t.Fatal("copy rewrote old player progress")
	}
	// The delivery enters original mail/reward centre, leaving balance untouched until claim.
	beforeP, _ := f.profile(t)
	grant := model.AdminGrantRequest{PlayerID: f.id, ResourceID: "coin", Amount: 37, Reason: "运营附件代表", RequestID: uuid.NewString()}
	if _, err := f.s.AdminGrant(f.ctx, read, grant); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer granted resource", err)
	}
	delivery, err := f.s.AdminGrant(f.ctx, op, grant)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.s.AdminGrant(f.ctx, op, grant)
	if err != nil || again.DeliveryID != delivery.DeliveryID {
		t.Fatal("idempotent delivery", err)
	}
	changed := grant
	changed.Amount++
	if _, err := f.s.AdminGrant(f.ctx, op, changed); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("idempotent payload overwritten", err)
	}
	pendingP, _ := f.profile(t)
	if pendingP.GetCoin() != beforeP.GetCoin() || p1Count(t, &db.RewardDelivery{}, "source_key = ?", "admin-grant:"+grant.RequestID) != 1 || p1Count(t, &db.MailMessage{}, "player_id = ?", f.id) != 0 {
		t.Fatal("grant bypassed original mail or duplicated")
	}
	f.restart(t)
	again, err = f.s.AdminGrant(f.ctx, op, grant)
	if err != nil || again.DeliveryID != delivery.DeliveryID {
		t.Fatal("cold delivery retry", err)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.DeliveryID}}); err != nil {
		t.Fatal("original reward claim", err)
	}
	claimedP, _ := f.profile(t)
	if claimedP.GetCoin()-beforeP.GetCoin() != 37 {
		t.Fatal("original grant amount")
	}
	f.restart(t)
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.DeliveryID}}); err != nil {
		t.Fatal("cold claimed retry", err)
	}
	coldP, _ := f.profile(t)
	if !proto.Equal(claimedP, coldP) {
		t.Fatal("cold claim paid twice")
	}
	if p1Count(t, &db.AdminAudit{}, "action = ?", "resource.grant") != 1 {
		t.Fatal("delivery audit repeated")
	}
	t.Logf("4 create/update domains; one empty draft; frozen conflict; original mail/claim/cold; source=%s", fmt.Sprint(selected.WheelPool))
}

func TestAdminAnnouncementPublicationWindowsAndConflicts(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminOperations(f.ctx); err != nil {
		t.Fatal(err)
	}
	u := db.AdminUser{Username: "announcement-operator", Role: constUtil.AdminRoleOperator, Enabled: true, PasswordHash: "unused-fixture"}
	if err := db.GetIns().Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	p := &model.AdminPrincipal{UserID: u.ID, Username: u.Username, Role: u.Role}
	now := time.Now().UnixMilli()
	current, err := AdminSaveAnnouncement(f.ctx, p, 0, model.AdminAnnouncementRequest{Title: "当前公告", Content: "纯文本运营公告", Enabled: true, StartAt: now - 1000, EndAt: now + 60000})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []model.AdminAnnouncementRequest{
		{Title: "未发布", Content: "hidden", Enabled: false},
		{Title: "未来", Content: "future", Enabled: true, StartAt: now + 3600000},
		{Title: "过期", Content: "past", Enabled: true, StartAt: now - 3600000, EndAt: now - 1000},
	} {
		if _, err := AdminSaveAnnouncement(f.ctx, p, 0, r); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := AdminAnnouncements(f.ctx, true)
	if err != nil || len(rows) != 1 || rows[0].ID != current.ID {
		t.Fatal("announcement publication window", err)
	}
	update := model.AdminAnnouncementRequest{Title: "已修改", Content: "versioned plain text", Enabled: true, StartAt: current.StartAt, EndAt: current.EndAt, Version: current.Version}
	saved, err := AdminSaveAnnouncement(f.ctx, p, current.ID, update)
	if err != nil || saved.Title != update.Title {
		t.Fatal("announcement update", err)
	}
	if _, err := AdminSaveAnnouncement(f.ctx, p, current.ID, update); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale announcement overwrite", err)
	}
	all, err := AdminAnnouncements(f.ctx, false)
	if err != nil || len(all) != 4 {
		t.Fatal("admin missing drafts", err)
	}
	if p1Count(t, &db.AdminAudit{}, "action = ?", "announcement.save") != 5 {
		t.Fatal("announcement audit missing or duplicated")
	}
}
