package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminMailAndBroadcastExplicitAudienceSeparateProtocols covers cross-page selection, exclusions and protocol separation.
func TestAdminMailAndBroadcastExplicitAudienceSeparateProtocols(t *testing.T) {
	f := p1Setup(t)
	for _, initialize := range []func(context.Context) error{func(ctx context.Context) error { return AdminInitialize(ctx, "") }, InitializeAdminOperations, f.s.InitializeAdminGameServers, f.s.InitializeGlobalGameAccounts, f.s.InitializeAdminBroadcast, f.s.InitializeAdminMail} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	f.ctx = context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, f.s.OriginalServerID())
	d := db.GetIns().WithContext(f.ctx)
	actorRow := db.AdminUser{Username: "mail-" + uuid.NewString(), PasswordHash: "isolated-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := d.Create(&actorRow).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: actorRow.ID, Username: actorRow.Username, Role: actorRow.Role}
	type role struct {
		profile int64
		account int64
	}
	makeRole := func(name string) role {
		t.Helper()
		username := "mail_" + uuid.NewString()[:16]
		if _, err := RegisterGameAccount(f.ctx, username, "isolated-test-password", name); err != nil {
			t.Fatal(err)
		}
		account, err := LoginAccount(f.ctx, username, "isolated-test-password")
		if err != nil {
			t.Fatal(err)
		}
		profile, err := CreatePlayer(f.ctx, account.Id, name, "hero_043")
		if err != nil {
			t.Fatal(err)
		}
		return role{profile: profile.Id, account: account.Id}
	}
	chosen, frozen, unselected := makeRole("邮件甲"), makeRole("邮件乙"), makeRole("邮件丙")
	// A disabled login account is frozen and must be excluded from both all and explicit audiences.
	if err := d.Model(&db.GameAccount{}).Where("id=?", frozen.account).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	all, err := f.s.AdminBroadcastPreview(f.ctx, actor, model.AdminBroadcastPreviewRequest{RequestID: uuid.NewString(), Audience: model.AdminAudience{Mode: constUtil.AdminAudienceModeAll}, Title: "全部补偿", Body: "冻结保护排除", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 3}}, Reason: "all audience fixture"})
	if err != nil {
		t.Fatal(err)
	}
	// The fixture's protected role and the frozen account are not recipients.
	if all.Batch.RecipientCount != 2 || all.Batch.ExcludedCount != 2 || all.Batch.AudienceMode != constUtil.AdminAudienceModeAll {
		t.Fatal("all audience", all.Batch)
	}
	explicit, err := f.s.AdminBroadcastPreview(f.ctx, actor, model.AdminBroadcastPreviewRequest{RequestID: uuid.NewString(), Audience: model.AdminAudience{Mode: constUtil.AdminAudienceModePlayers, PlayerIDs: []int64{chosen.profile, frozen.profile}}, Title: "指定补偿", Body: "只发给勾选角色", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 5}}, Reason: "explicit audience fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Batch.RecipientCount != 1 || explicit.Batch.ExcludedCount != 1 || explicit.Batch.AudienceMode != constUtil.AdminAudienceModePlayers {
		t.Fatal("explicit audience", explicit.Batch)
	}
	confirm := model.AdminBroadcastSendRequest{RequestID: uuid.NewString(), Version: explicit.Batch.Version}
	if _, err = f.s.AdminBroadcastSend(f.ctx, actor, explicit.Batch.ID, confirm); err != nil {
		t.Fatal(err)
	}
	if count := p1Count(t, &db.RewardDelivery{}, "source_key=?", "admin-broadcast:"+explicit.Batch.ID); count != 1 {
		t.Fatal("explicit compensation recipients", count)
	}
	if count := p1Count(t, &db.MailMessage{}, "source_key=?", "admin-broadcast:"+explicit.Batch.ID); count != 0 {
		t.Fatal("compensation created mail", count)
	}
	// Selecting a role outside the selected realm never silently shrinks the reviewed audience.
	foreign := makeRole("外服邮件")
	if err = d.Model(&db.Profile{}).Where("id=?", foreign.profile).Update("server_id", f.s.OriginalServerID()+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminBroadcastPreview(f.ctx, actor, model.AdminBroadcastPreviewRequest{RequestID: uuid.NewString(), Audience: model.AdminAudience{Mode: constUtil.AdminAudienceModePlayers, PlayerIDs: []int64{foreign.profile}}, Title: "跨服", Body: "拒绝", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 1}}, Reason: "foreign fixture"}); !errors.Is(err, constUtil.ErrAdminAudience) {
		t.Fatal("foreign explicit recipient", err)
	}
	mailPreview, err := f.s.AdminMailPreview(f.ctx, actor, model.AdminMailPreviewRequest{RequestID: uuid.NewString(), Audience: model.AdminAudience{Mode: constUtil.AdminAudienceModePlayers, PlayerIDs: []int64{chosen.profile, unselected.profile}}, Title: "独立邮件", Body: "只写邮箱", Reason: "mail fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if mailPreview.Batch.RecipientCount != 2 || mailPreview.Batch.AudienceMode != constUtil.AdminAudienceModePlayers {
		t.Fatal("mail preview", mailPreview.Batch)
	}
	mailRequest := model.AdminMailPreviewRequest{RequestID: mailPreview.Batch.ID, Audience: model.AdminAudience{Mode: constUtil.AdminAudienceModePlayers, PlayerIDs: []int64{chosen.profile, unselected.profile}}, Title: "独立邮件", Body: "只写邮箱", Reason: "mail fixture"}
	mailReplay, err := f.s.AdminMailPreview(f.ctx, actor, mailRequest)
	if err != nil || mailReplay.Batch.Version != mailPreview.Batch.Version {
		t.Fatal("mail preview retry", err)
	}
	mailConfirm := model.AdminMailSendRequest{RequestID: uuid.NewString(), Version: mailPreview.Batch.Version}
	for i := 0; i < 2; i++ {
		if _, err = f.s.AdminMailSend(f.ctx, actor, mailPreview.Batch.ID, mailConfirm); err != nil {
			t.Fatal("mail send or retry", err)
		}
	}
	if count := p1Count(t, &db.MailMessage{}, "source_key=?", "admin-mail:"+mailPreview.Batch.ID); count != 2 {
		t.Fatal("mail recipients", count)
	}
	if count := p1Count(t, &db.RewardDelivery{}, "source_key=?", "admin-mail:"+mailPreview.Batch.ID); count != 0 {
		t.Fatal("mail created reward-center delivery", count)
	}
	if _, err = f.s.AdminMailSend(f.ctx, actor, mailPreview.Batch.ID, model.AdminMailSendRequest{RequestID: uuid.NewString(), Version: mailPreview.Batch.Version}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("different mail confirmation", err)
	}
	mailRequest.RequestID = uuid.NewString()
	expiring, err := f.s.AdminMailPreview(f.ctx, actor, mailRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Model(&db.AdminMailBatch{}).Where("id=?", expiring.Batch.ID).Update("expires_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminMailSend(f.ctx, actor, expiring.Batch.ID, model.AdminMailSendRequest{RequestID: uuid.NewString(), Version: expiring.Batch.Version}); !errors.Is(err, constUtil.ErrAdminMailExpired) {
		t.Fatal("expired mail preview delivered", err)
	}
	mailHistory, err := f.s.AdminMailList(f.ctx, actor, 1, 25)
	if err != nil || mailHistory.Total != 1 || mailHistory.Items[0].SuccessCount != 2 {
		t.Fatal("mail history", err, mailHistory)
	}
	broadcastHistory, err := f.s.AdminBroadcastList(f.ctx, actor, 1, 25)
	if err != nil || broadcastHistory.Total != 1 || broadcastHistory.Items[0].AudienceMode != constUtil.AdminAudienceModePlayers {
		t.Fatal("broadcast history", err, broadcastHistory)
	}
	if err = d.Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	mailRequest.RequestID = uuid.NewString()
	if _, err = f.s.AdminMailPreview(f.ctx, actor, mailRequest); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer mail preview write", err)
	}
	if _, err = f.s.AdminMailList(f.ctx, actor, 1, 25); err != nil {
		t.Fatal("viewer mail read", err)
	}
}
