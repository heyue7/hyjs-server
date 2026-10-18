package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// InitializeGameFeedback migrates empty support tables without changing game accounts.
func InitializeGameFeedback(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.GameFeedback{}, &db.GameFeedbackMessage{})
}
func feedbackProjection(row db.GameFeedback) model.FeedbackDTO {
	return model.FeedbackDTO{ID: row.ID, PlayerID: row.PlayerID, Subject: row.Subject, Category: row.Category, State: row.State, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: adminVersion(row)}
}
func feedbackDetail(tx *gorm.DB, row db.GameFeedback) (*model.FeedbackDTO, error) {
	out := feedbackProjection(row)
	var profile db.Profile
	if err := tx.Select("id,player,game").First(&profile, row.PlayerID).Error; err == nil {
		if player, _, err := decodeProfile(&profile); err == nil {
			out.PlayerName = player.GetName()
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var messages []db.GameFeedbackMessage
	if err := tx.Where("feedback_id=?", row.ID).Order("created_at,id").Find(&messages).Error; err != nil {
		return nil, err
	}
	out.Messages = []model.FeedbackMessageDTO{}
	for _, m := range messages {
		out.Messages = append(out.Messages, model.FeedbackMessageDTO{ID: m.ID, Sender: m.Sender, Content: m.Content, CreatedAt: m.CreatedAt})
	}
	return &out, nil
}

// GameFeedbackList only lists the authenticated owner's conversations.
func GameFeedbackList(ctx context.Context, owner int64, page, size int) (*model.FeedbackListResponse, error) {
	return feedbackList(ctx, owner, "", "", page, size)
}
func feedbackList(ctx context.Context, owner int64, state, q string, page, size int) (*model.FeedbackListResponse, error) {
	tx := db.GetIns().WithContext(ctx).Model(&db.GameFeedback{})
	if owner > 0 {
		tx = tx.Where("player_id=?", owner)
	}
	if state != "" {
		tx = tx.Where("state=?", state)
	}
	if q != "" {
		tx = tx.Where("subject LIKE ?", "%"+q+"%")
	}
	out := &model.FeedbackListResponse{Items: []model.FeedbackDTO{}, Page: page, PageSize: size}
	if err := tx.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.GameFeedback
	if err := tx.Order("updated_at DESC,id").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, r := range rows {
		ids = append(ids, r.PlayerID)
	}
	names := map[int64]string{}
	if len(ids) > 0 {
		var profiles []db.Profile
		if err := db.GetIns().WithContext(ctx).Select("id,player,game").Where("id IN ?", ids).Find(&profiles).Error; err != nil {
			return nil, err
		}
		for _, r := range profiles {
			if p, _, err := decodeProfile(&r); err == nil {
				names[r.Id] = p.GetName()
			}
		}
	}
	for _, r := range rows {
		dto := feedbackProjection(r)
		dto.PlayerName = names[r.PlayerID]
		out.Items = append(out.Items, dto)
	}
	return out, nil
}

// GameFeedbackDetail checks ownership before returning any conversation text.
func GameFeedbackDetail(ctx context.Context, owner int64, id string) (*model.FeedbackDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	var row db.GameFeedback
	if err := tx.Where("id=? AND player_id=?", id, owner).First(&row).Error; err != nil {
		return nil, err
	}
	return feedbackDetail(tx, row)
}

// GameFeedbackCreate serializes requests per original account and freezes retries.
func GameFeedbackCreate(ctx context.Context, owner int64, req model.GameFeedbackRequest) (*model.FeedbackDTO, error) {
	var out *model.FeedbackDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, "id=?", owner).Error; err != nil {
			return err
		}
		var old db.GameFeedback
		err := tx.First(&old, "owner_id=? AND request_id=?", owner, req.RequestID).Error
		if err == nil {
			if old.Subject != req.Subject || old.Category != req.Category {
				return constUtil.ErrAdminConfigConflict
			}
			var first db.GameFeedbackMessage
			if err = tx.First(&first, "feedback_id=? AND request_id=?", old.ID, req.RequestID).Error; err != nil {
				return err
			}
			if first.Content != req.Content {
				return constUtil.ErrAdminConfigConflict
			}
			out, err = feedbackDetail(tx, old)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// A finite queue prevents unbounded ticket creation while preserving all old history.
		var count int64
		if err = tx.Model(&db.GameFeedback{}).Where("player_id=? AND created_at>?", owner, time.Now().Add(-24*time.Hour).UnixMilli()).Count(&count).Error; err != nil {
			return err
		}
		if count >= 20 {
			return model.NewErrBizCodeMsg(constUtil.GameFeedbackDailyLimit, "今天反馈较多，请在已有反馈中补充")
		}
		now := time.Now().UnixMilli()
		row := db.GameFeedback{ID: uuid.NewString(), PlayerID: owner, OwnerID: owner, RequestID: req.RequestID, Category: req.Category, Subject: req.Subject, State: "open", CreatedAt: now, UpdatedAt: now}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		m := db.GameFeedbackMessage{ID: uuid.NewString(), FeedbackID: row.ID, RequestID: req.RequestID, Sender: "player", ActorID: owner, Content: req.Content, CreatedAt: now}
		if err = tx.Create(&m).Error; err != nil {
			return err
		}
		out, err = feedbackDetail(tx, row)
		return err
	})
	return out, err
}
func feedbackReply(tx *gorm.DB, row *db.GameFeedback, sender string, actor int64, requestID, content string) (*model.FeedbackDTO, error) {
	var old db.GameFeedbackMessage
	err := tx.First(&old, "feedback_id=? AND request_id=?", row.ID, requestID).Error
	if err == nil {
		if old.Sender != sender || old.ActorID != actor || old.Content != content {
			return nil, constUtil.ErrAdminConfigConflict
		}
		return feedbackDetail(tx, *row)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Freeze successful retries before applying the finite new-message admission cap.
	if sender == "player" {
		var count int64
		if err := tx.Model(&db.GameFeedbackMessage{}).Where("feedback_id=?", row.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= 200 {
			return nil, constUtil.ErrAdminInput
		}
	}
	// Real-clock milliseconds are made monotonic within the conversation so equal-millisecond replies retain their actual append order.
	messageTime := max(time.Now().UnixMilli(), row.UpdatedAt+1)
	m := db.GameFeedbackMessage{ID: uuid.NewString(), FeedbackID: row.ID, RequestID: requestID, Sender: sender, ActorID: actor, Content: content, CreatedAt: messageTime}
	if err = tx.Create(&m).Error; err != nil {
		return nil, err
	}
	row.State = "open"
	if sender == "support" {
		row.State = "replied"
	}
	row.UpdatedAt = messageTime
	if err = tx.Save(row).Error; err != nil {
		return nil, err
	}
	return feedbackDetail(tx, *row)
}

// GameFeedbackReply appends a message and reopens the player's own ticket.
func GameFeedbackReply(ctx context.Context, owner int64, req model.GameFeedbackRequest) (*model.FeedbackDTO, error) {
	var out *model.FeedbackDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.GameFeedback
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND player_id=?", req.ID, owner).Error; err != nil {
			return err
		}
		var err error
		out, err = feedbackReply(tx, &row, "player", owner, req.RequestID, req.Content)
		return err
	})
	return out, err
}

// AdminFeedbackList reads the shared support inbox with role validation.
func AdminFeedbackList(ctx context.Context, p *model.AdminPrincipal, state, q string, page, size int) (*model.FeedbackListResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	return feedbackList(ctx, 0, state, q, page, size)
}

// AdminFeedbackDetail reads one support conversation.
func AdminFeedbackDetail(ctx context.Context, p *model.AdminPrincipal, id string) (*model.FeedbackDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	var row db.GameFeedback
	if err := tx.First(&row, "id=?", id).Error; err != nil {
		return nil, err
	}
	return feedbackDetail(tx, row)
}

// AdminFeedbackReply freezes support text and audits metadata in the same transaction.
func AdminFeedbackReply(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminFeedbackReplyRequest) (*model.FeedbackDTO, error) {
	var out *model.FeedbackDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.GameFeedback
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; err != nil {
			return err
		}
		var existing db.GameFeedbackMessage
		old := tx.First(&existing, "feedback_id=? AND request_id=?", id, req.RequestID).Error
		if old != nil && !errors.Is(old, gorm.ErrRecordNotFound) {
			return old
		}
		if old != nil && adminVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		var err error
		out, err = feedbackReply(tx, &row, "support", p.UserID, req.RequestID, req.Content)
		if err != nil {
			return err
		}
		if old == nil {
			return nil
		}
		return adminAudit(tx, p, "feedback.reply", id, fmt.Sprintf("playerId=%d requestId=%s", row.PlayerID, req.RequestID))
	})
	return out, err
}

// AdminFeedbackState changes only handling metadata, retaining all messages.
func AdminFeedbackState(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminFeedbackStateRequest) (*model.FeedbackDTO, error) {
	var out *model.FeedbackDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.GameFeedback
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; err != nil {
			return err
		}
		if adminVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		row.State = req.State
		row.UpdatedAt = max(time.Now().UnixMilli(), row.UpdatedAt+1)
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, p, "feedback.state", id, "state="+row.State); err != nil {
			return err
		}
		var err error
		out, err = feedbackDetail(tx, row)
		return err
	})
	return out, err
}
