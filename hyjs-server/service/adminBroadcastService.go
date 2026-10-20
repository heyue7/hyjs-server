package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeAdminBroadcast adds only frozen previews and per-recipient original reward-center receipts.
func (s *GameService) InitializeAdminBroadcast(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminBroadcastBatch{}, &db.AdminBroadcastRecipient{})
}

func adminBroadcastProjection(row db.AdminBroadcastBatch, realm db.AdminGameServer) model.AdminBroadcastDTO {
	return model.AdminBroadcastDTO{ID: row.ID, ServerID: row.ServerID, ServerName: realm.Name, ActorName: row.ActorName, AudienceMode: adminAudienceStoredMode(row.AudienceMode), Title: row.Title, Body: row.Body, Rewards: adminTypedResources(row.Rewards), Reason: row.Reason, RecipientCount: row.RecipientCount, ExcludedCount: row.ExcludedCount, SuccessCount: row.SuccessCount, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, SentAt: row.SentAt, Version: row.Version}
}

// AdminBroadcastPreview freezes a concrete audience without creating reward-center deliveries or mail.
func (s *GameService) AdminBroadcastPreview(ctx context.Context, actor *model.AdminPrincipal, req model.AdminBroadcastPreviewRequest) (*model.AdminBroadcastPreviewResponse, error) {
	rewards, err := s.adminTypedResourceSpec(req.Rewards, false)
	if err != nil {
		return nil, err
	}
	bodyHash := adminVersion(req)
	var out model.AdminBroadcastPreviewResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		realm, e := s.adminAudienceScope(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		var prior db.AdminBroadcastBatch
		e = tx.First(&prior, "id=?", req.RequestID).Error
		if e == nil {
			if prior.ServerID != realm.ID || prior.ActorID != actor.UserID || prior.BodyHash != bodyHash {
				return constUtil.ErrAdminConfigConflict
			}
			out.Batch = adminBroadcastProjection(prior, *realm)
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		// Freeze either the complete cross-page realm audience or exactly the checked players.
		members, excluded, e := adminAudienceFreeze(tx, realm.ID, req.Audience.Mode, req.Audience.PlayerIDs)
		if e != nil {
			return e
		}
		recipients := make([]db.AdminBroadcastRecipient, 0, len(members))
		for _, member := range members {
			recipients = append(recipients, db.AdminBroadcastRecipient{BatchID: req.RequestID, PlayerID: member.PlayerID, PlayerName: member.PlayerName})
		}
		now := time.Now().UnixMilli()
		batch := db.AdminBroadcastBatch{ID: req.RequestID, ServerID: realm.ID, ActorID: actor.UserID, ActorName: actor.Username, AudienceMode: req.Audience.Mode, Title: req.Title, Body: req.Body, Rewards: rewards, Reason: req.Reason, BodyHash: bodyHash, RecipientCount: int64(len(recipients)), ExcludedCount: excluded, CreatedAt: now, ExpiresAt: now + constUtil.AdminBroadcastPreviewTTL.Milliseconds()}
		batch.Version = adminVersion(struct {
			Content    string
			Recipients []db.AdminBroadcastRecipient
		}{bodyHash, recipients})
		if e = tx.Create(&batch).Error; e != nil {
			return e
		}
		if len(recipients) > 0 {
			if e = tx.CreateInBatches(&recipients, 500).Error; e != nil {
				return e
			}
		}
		out.Batch = adminBroadcastProjection(batch, *realm)
		return adminAudit(tx, actor, "resource.broadcast.preview", batch.ID, fmt.Sprintf("server=%d; audience=%s; recipients=%d; %s", realm.ID, batch.AudienceMode, batch.RecipientCount, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminBroadcastSend atomically commits original reward-center deliveries for only the reviewed recipient IDs.
func (s *GameService) AdminBroadcastSend(ctx context.Context, actor *model.AdminPrincipal, id string, req model.AdminBroadcastSendRequest) (*model.AdminBroadcastSendResponse, error) {
	var out model.AdminBroadcastSendResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		realm, e := s.adminAudienceScope(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		var batch db.AdminBroadcastBatch
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, "id=? AND server_id=?", id, realm.ID).Error; e != nil {
			return e
		}
		if batch.ActorID != actor.UserID || batch.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		// A committed receipt remains retryable after preview expiry and process restart.
		if batch.SentAt > 0 {
			if batch.CommitRequestID == nil || *batch.CommitRequestID != req.RequestID {
				return constUtil.ErrAdminConfigConflict
			}
			out.Batch = adminBroadcastProjection(batch, *realm)
			return nil
		}
		if batch.ExpiresAt <= time.Now().UnixMilli() {
			return constUtil.ErrAdminBroadcastExpired
		}
		if batch.RecipientCount < 1 {
			return constUtil.ErrAdminBroadcastEmpty
		}
		var reused int64
		if e = tx.Model(&db.AdminBroadcastBatch{}).Where("commit_request_id=?", req.RequestID).Count(&reused).Error; e != nil {
			return e
		}
		if reused > 0 {
			return constUtil.ErrAdminConfigConflict
		}
		var recipients []db.AdminBroadcastRecipient
		if e = tx.Where("batch_id=?", batch.ID).Order("player_id").Find(&recipients).Error; e != nil {
			return e
		}
		if int64(len(recipients)) != batch.RecipientCount {
			return constUtil.ErrAdminConfigConflict
		}
		for _, recipient := range recipients {
			var profile db.Profile
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "account_id", "server_id", "player").First(&profile, recipient.PlayerID).Error; e != nil {
				return e
			}
			_, eligible, checkErr := adminAudienceEligible(tx, profile)
			if checkErr != nil {
				return checkErr
			}
			if AdminProfileServerID(profile) != realm.ID || !eligible {
				return constUtil.ErrAdminBroadcastAudience
			}
			// Compensation uses the original reward-center protocol only and never creates a mailbox message.
			delivery, deliveryErr := s.EnqueueReward(tx, profile.Id, "admin-broadcast:"+batch.ID, batch.Title, batch.Body, batch.Rewards, 0)
			if deliveryErr != nil {
				return deliveryErr
			}
			if e = tx.Model(&db.AdminBroadcastRecipient{}).Where("batch_id=? AND player_id=?", batch.ID, profile.Id).Update("delivery_id", delivery.ID).Error; e != nil {
				return e
			}
		}
		batch.SentAt = time.Now().UnixMilli()
		batch.SuccessCount = batch.RecipientCount
		batch.CommitRequestID = &req.RequestID
		if e = tx.Save(&batch).Error; e != nil {
			return e
		}
		out.Batch = adminBroadcastProjection(batch, *realm)
		return adminAudit(tx, actor, "resource.broadcast.send", batch.ID, fmt.Sprintf("server=%d; success=%d; request=%s; %s", realm.ID, batch.SuccessCount, req.RequestID, batch.Reason))
	})
	if err != nil {
		return nil, err
	}
	// 批次已完整提交后刷新在线角色；离线角色下次原CSRedPoint读取同一待领事实。
	var recipients []db.AdminBroadcastRecipient
	if db.GetIns().WithContext(ctx).Where("batch_id=? AND delivery_id>0", id).Find(&recipients).Error == nil {
		for _, recipient := range recipients {
			s.publishInventoryMailRedPoints(ctx, recipient.PlayerID)
		}
	}
	return &out, nil
}

// AdminBroadcastList exposes committed realm-owned reward-center receipts to authorized operators and viewers.
func (s *GameService) AdminBroadcastList(ctx context.Context, actor *model.AdminPrincipal, page, size int64) (*model.AdminBroadcastListResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	realm, err := s.adminAudienceScope(ctx, tx, actor, false)
	if err != nil {
		return nil, err
	}
	query := tx.Model(&db.AdminBroadcastBatch{}).Where("server_id=? AND sent_at>0", realm.ID)
	out := &model.AdminBroadcastListResponse{Items: []model.AdminBroadcastDTO{}}
	if err = query.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.AdminBroadcastBatch
	if err = query.Order("sent_at DESC,id").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out.Items = append(out.Items, adminBroadcastProjection(row, *realm))
	}
	return out, nil
}
