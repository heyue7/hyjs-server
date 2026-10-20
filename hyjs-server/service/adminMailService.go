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

// InitializeAdminMail adds only frozen previews and per-recipient original mailbox receipts.
func (s *GameService) InitializeAdminMail(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminMailBatch{}, &db.AdminMailRecipient{})
}

func adminMailProjection(row db.AdminMailBatch, realm db.AdminGameServer) model.AdminMailDTO {
	return model.AdminMailDTO{ID: row.ID, ServerID: row.ServerID, ServerName: realm.Name, ActorName: row.ActorName, AudienceMode: adminAudienceStoredMode(row.AudienceMode), Title: row.Title, Body: row.Body, Reason: row.Reason, RecipientCount: row.RecipientCount, ExcludedCount: row.ExcludedCount, SuccessCount: row.SuccessCount, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, SentAt: row.SentAt, Version: row.Version}
}

// AdminMailPreview freezes a concrete audience without creating mailbox messages.
func (s *GameService) AdminMailPreview(ctx context.Context, actor *model.AdminPrincipal, req model.AdminMailPreviewRequest) (*model.AdminMailPreviewResponse, error) {
	bodyHash := adminVersion(req)
	var out model.AdminMailPreviewResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		realm, e := s.adminAudienceScope(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		var prior db.AdminMailBatch
		e = tx.First(&prior, "id=?", req.RequestID).Error
		if e == nil {
			if prior.ServerID != realm.ID || prior.ActorID != actor.UserID || prior.BodyHash != bodyHash {
				return constUtil.ErrAdminConfigConflict
			}
			out.Batch = adminMailProjection(prior, *realm)
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
		recipients := make([]db.AdminMailRecipient, 0, len(members))
		for _, member := range members {
			recipients = append(recipients, db.AdminMailRecipient{BatchID: req.RequestID, PlayerID: member.PlayerID, PlayerName: member.PlayerName})
		}
		now := time.Now().UnixMilli()
		batch := db.AdminMailBatch{ID: req.RequestID, ServerID: realm.ID, ActorID: actor.UserID, ActorName: actor.Username, AudienceMode: req.Audience.Mode, Title: req.Title, Body: req.Body, Reason: req.Reason, BodyHash: bodyHash, RecipientCount: int64(len(recipients)), ExcludedCount: excluded, CreatedAt: now, ExpiresAt: now + constUtil.AdminMailPreviewTTL.Milliseconds()}
		batch.Version = adminVersion(struct {
			Content    string
			Recipients []db.AdminMailRecipient
		}{bodyHash, recipients})
		if e = tx.Create(&batch).Error; e != nil {
			return e
		}
		if len(recipients) > 0 {
			if e = tx.CreateInBatches(&recipients, 500).Error; e != nil {
				return e
			}
		}
		out.Batch = adminMailProjection(batch, *realm)
		return adminAudit(tx, actor, "resource.mail.preview", batch.ID, fmt.Sprintf("server=%d; audience=%s; recipients=%d; %s", realm.ID, batch.AudienceMode, batch.RecipientCount, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminMailSend atomically commits original mailbox messages for only the reviewed recipient IDs.
func (s *GameService) AdminMailSend(ctx context.Context, actor *model.AdminPrincipal, id string, req model.AdminMailSendRequest) (*model.AdminMailSendResponse, error) {
	var out model.AdminMailSendResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ServerGameClockGuardTransaction(tx); e != nil {
			return e
		}
		realm, e := s.adminAudienceScope(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		var batch db.AdminMailBatch
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
			out.Batch = adminMailProjection(batch, *realm)
			return nil
		}
		if batch.ExpiresAt <= time.Now().UnixMilli() {
			return constUtil.ErrAdminMailExpired
		}
		if batch.RecipientCount < 1 {
			return constUtil.ErrAdminMailEmpty
		}
		var reused int64
		if e = tx.Model(&db.AdminMailBatch{}).Where("commit_request_id=?", req.RequestID).Count(&reused).Error; e != nil {
			return e
		}
		if reused > 0 {
			return constUtil.ErrAdminConfigConflict
		}
		var recipients []db.AdminMailRecipient
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
				return constUtil.ErrAdminMailAudience
			}
			// Mail uses only the original mailbox protocol; attachments remain outside this independent feature.
			mail, mailErr := s.EnqueueMail(tx, profile.Id, "admin-mail:"+batch.ID, constUtil.P1MailboxSystem, batch.Title, batch.Body, "", constUtil.P1MailSystemType, 0)
			if mailErr != nil {
				return mailErr
			}
			if e = tx.Model(&db.AdminMailRecipient{}).Where("batch_id=? AND player_id=?", batch.ID, profile.Id).Update("mail_id", mail.ID).Error; e != nil {
				return e
			}
		}
		batch.SentAt = time.Now().UnixMilli()
		batch.SuccessCount = batch.RecipientCount
		batch.CommitRequestID = &req.RequestID
		if e = tx.Save(&batch).Error; e != nil {
			return e
		}
		out.Batch = adminMailProjection(batch, *realm)
		return adminAudit(tx, actor, "resource.mail.send", batch.ID, fmt.Sprintf("server=%d; success=%d; request=%s; %s", realm.ID, batch.SuccessCount, req.RequestID, batch.Reason))
	})
	if err != nil {
		return nil, err
	}
	// 大批次提交后刷新在线角色未读邮件点；离线角色下次原CSRedPoint读取同一事实。
	var recipients []db.AdminMailRecipient
	if db.GetIns().WithContext(ctx).Where("batch_id=? AND mail_id>0", id).Find(&recipients).Error == nil {
		for _, recipient := range recipients {
			s.publishInventoryMailRedPoints(ctx, recipient.PlayerID)
		}
	}
	return &out, nil
}

// AdminMailList exposes committed realm-owned mailbox receipts to authorized operators and viewers.
func (s *GameService) AdminMailList(ctx context.Context, actor *model.AdminPrincipal, page, size int64) (*model.AdminMailListResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	realm, err := s.adminAudienceScope(ctx, tx, actor, false)
	if err != nil {
		return nil, err
	}
	query := tx.Model(&db.AdminMailBatch{}).Where("server_id=? AND sent_at>0", realm.ID)
	out := &model.AdminMailListResponse{Items: []model.AdminMailDTO{}}
	if err = query.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.AdminMailBatch
	if err = query.Order("sent_at DESC,id").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out.Items = append(out.Items, adminMailProjection(row, *realm))
	}
	return out, nil
}
