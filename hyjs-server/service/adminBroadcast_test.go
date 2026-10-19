package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminBroadcastFrozenAudienceAtomicMailIdempotencyAndCold covers native delivery, rollback and cold retry.
func TestAdminBroadcastFrozenAudienceAtomicMailIdempotencyAndCold(t *testing.T) {
	f := p1Setup(t)
	for _, initialize := range []func(context.Context) error{func(ctx context.Context) error { return AdminInitialize(ctx, "") }, InitializeAdminOperations, f.s.InitializeAdminGameServers, f.s.InitializeGlobalGameAccounts, f.s.InitializeAdminBroadcast} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	f.ctx = context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, f.s.OriginalServerID())
	d := db.GetIns().WithContext(f.ctx)
	actorRow := db.AdminUser{Username: "broadcast-" + uuid.NewString(), PasswordHash: "isolated-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := d.Create(&actorRow).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: actorRow.ID, Username: actorRow.Username, Role: actorRow.Role}
	makeRole := func(name string) int64 {
		t.Helper()
		username := "broadcast_" + uuid.NewString()[:16]
		if _, err := RegisterGameAccount(f.ctx, username, "isolated-test-password", name); err != nil {
			t.Fatal(err)
		}
		account, err := LoginAccount(f.ctx, username, "isolated-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, account.Id, name, "hero_043"); err != nil {
			t.Fatal(err)
		}
		return account.Id
	}
	first, second := makeRole("群发甲"), makeRole("群发乙")
	foreign := makeRole("外服角色")
	if err := d.Model(&db.Profile{}).Where("id=?", foreign).Update("server_id", f.s.OriginalServerID()+1).Error; err != nil {
		t.Fatal(err)
	}
	request := model.AdminBroadcastPreviewRequest{RequestID: uuid.NewString(), Title: "维护补偿", Body: "隔离验证正文", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Reason: "finite native broadcast fixture"}
	preview, err := f.s.AdminBroadcastPreview(f.ctx, actor, request)
	if err != nil || preview.Batch.RecipientCount != 2 || preview.Batch.ExcludedCount != 1 {
		t.Fatal("preview audience", err, preview)
	}
	replay, err := f.s.AdminBroadcastPreview(f.ctx, actor, request)
	if err != nil || replay.Batch.Version != preview.Batch.Version {
		t.Fatal("preview retry", err)
	}
	changed := request
	changed.Title = "修改内容"
	if _, err = f.s.AdminBroadcastPreview(f.ctx, actor, changed); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("preview UUID overwrote content", err)
	}
	if count := p1Count(t, &db.RewardDelivery{}, "source_key=?", "admin-broadcast:"+preview.Batch.ID); count != 0 {
		t.Fatal("preview sent mail", count)
	}
	late := makeRole("预览后新增")
	confirm := model.AdminBroadcastSendRequest{RequestID: uuid.NewString(), Version: preview.Batch.Version}
	wrongScope := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, f.s.OriginalServerID()+1)
	if _, err = f.s.AdminBroadcastSend(wrongScope, actor, preview.Batch.ID, confirm); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("cross realm send", err)
	}
	// The later recipient fails after the earlier original notice is written; the enclosing transaction must undo all of it.
	if err = d.Model(&db.Profile{}).Where("id=?", second).Update("server_id", f.s.OriginalServerID()+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminBroadcastSend(f.ctx, actor, preview.Batch.ID, confirm); !errors.Is(err, constUtil.ErrAdminBroadcastAudience) {
		t.Fatal("changed ownership accepted", err)
	}
	if count := p1Count(t, &db.RewardDelivery{}, "source_key=?", "admin-broadcast:"+preview.Batch.ID); count != 0 {
		t.Fatal("failed batch partially delivered", count)
	}
	if count := p1Count(t, &db.MailMessage{}, "source_key=?", "admin-broadcast:"+preview.Batch.ID); count != 0 {
		t.Fatal("failed batch retained mail", count)
	}
	if err = d.Model(&db.Profile{}).Where("id=?", second).Update("server_id", f.s.OriginalServerID()).Error; err != nil {
		t.Fatal(err)
	}
	// Two identical confirmations serialize to one original delivery per frozen recipient.
	var wait sync.WaitGroup
	errorsOut := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, sendErr := f.s.AdminBroadcastSend(f.ctx, actor, preview.Batch.ID, confirm)
			if sendErr == nil && result.Batch.SuccessCount != 2 {
				sendErr = fmt.Errorf("success count %d", result.Batch.SuccessCount)
			}
			errorsOut <- sendErr
		}()
	}
	wait.Wait()
	close(errorsOut)
	for sendErr := range errorsOut {
		if sendErr != nil {
			t.Fatal(sendErr)
		}
	}
	for _, target := range []any{&db.RewardDelivery{}, &db.MailMessage{}} {
		if count := p1Count(t, target, "source_key=?", "admin-broadcast:"+preview.Batch.ID); count != 2 {
			t.Fatal("duplicate or missing notices", count)
		}
	}
	for _, id := range []int64{f.id, foreign, late} {
		if count := p1Count(t, &db.RewardDelivery{}, "source_key=? AND player_id=?", "admin-broadcast:"+preview.Batch.ID, id); count != 0 {
			t.Fatal("excluded role received compensation", id)
		}
	}
	var delivery db.RewardDelivery
	if err = d.First(&delivery, "player_id=? AND source_key=?", first, "admin-broadcast:"+preview.Batch.ID).Error; err != nil {
		t.Fatal(err)
	}
	var beforeRow db.Profile
	if err = d.First(&beforeRow, first).Error; err != nil {
		t.Fatal(err)
	}
	beforePlayer, _, err := decodeProfile(&beforeRow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RewardCenter(f.ctx, first, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err != nil {
		t.Fatal("original claim", err)
	}
	var afterRow db.Profile
	if err = d.First(&afterRow, first).Error; err != nil {
		t.Fatal(err)
	}
	afterPlayer, _, err := decodeProfile(&afterRow)
	if err != nil || afterPlayer.GetCoin() != beforePlayer.GetCoin()+7 {
		t.Fatal("original reward amount", err)
	}
	f.restart(t)
	if _, err = f.s.AdminBroadcastSend(f.ctx, actor, preview.Batch.ID, confirm); err != nil {
		t.Fatal("cold send retry", err)
	}
	if _, err = f.s.RewardCenter(f.ctx, first, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err != nil {
		t.Fatal("native idempotent claim", err)
	}
	if err = db.GetIns().First(&afterRow, first).Error; err != nil {
		t.Fatal(err)
	}
	finalPlayer, _, err := decodeProfile(&afterRow)
	if err != nil || finalPlayer.GetCoin() != afterPlayer.GetCoin() {
		t.Fatal("cold claim duplicated", err)
	}
	history, err := f.s.AdminBroadcastList(f.ctx, actor, 1, 25)
	if err != nil || history.Total != 1 || history.Items[0].SuccessCount != 2 {
		t.Fatal("history", err, history)
	}
	// Expired previews never create original rewards, and a viewer cannot create a preview.
	request.RequestID = uuid.NewString()
	expiring, err := f.s.AdminBroadcastPreview(f.ctx, actor, request)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.AdminBroadcastBatch{}).Where("id=?", expiring.Batch.ID).Update("expires_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminBroadcastSend(f.ctx, actor, expiring.Batch.ID, model.AdminBroadcastSendRequest{RequestID: uuid.NewString(), Version: expiring.Batch.Version}); !errors.Is(err, constUtil.ErrAdminBroadcastExpired) {
		t.Fatal("expired preview delivered", err)
	}
	if err = db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	request.RequestID = uuid.NewString()
	if _, err = f.s.AdminBroadcastPreview(f.ctx, actor, request); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer preview write", err)
	}
}
