package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminBatchGrant uses the existing delivery receipt and reward consumer for a complete selection.
func (s *GameService) AdminBatchGrant(ctx context.Context, actor *model.AdminPrincipal, req model.AdminBatchGrantRequest) (*model.AdminGrantResponse, error) {
	out := &model.AdminGrantResponse{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "server_id").First(&profile, req.PlayerID).Error; err != nil {
			return err
		}
		if err := AdminCheckProfileScope(ctx, profile); err != nil {
			return err
		}
		hash := adminVersion(req)
		var receipt db.AdminGrantReceipt
		// Current reads observe a preceding retry that committed while this profile lock was waiting.
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&receipt, "request_id=?", req.RequestID).Error
		if err == nil {
			if receipt.ActorID != actor.UserID || receipt.BodyHash != hash {
				return constUtil.ErrAdminConfigConflict
			}
			*out = model.AdminGrantResponse{DeliveryID: receipt.DeliveryID, RequestID: req.RequestID}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		parts := make([]string, 0, len(req.Resources))
		for _, resource := range req.Resources {
			part := fmt.Sprintf("%s,%d", resource.ResourceID, resource.Amount)
			supported, err := s.openServerRewardSupported(part)
			if err != nil || !supported {
				return constUtil.ErrAdminInput
			}
			parts = append(parts, part)
		}
		rewards := strings.Join(parts, ";")
		delivery, err := s.EnqueueRewardNotice(tx, req.PlayerID, "admin-grant:"+req.RequestID, "运营发放", req.Reason, rewards, 0)
		if err != nil {
			return err
		}
		receipt = db.AdminGrantReceipt{RequestID: req.RequestID, ActorID: actor.UserID, BodyHash: hash, DeliveryID: delivery.ID}
		if err := tx.Create(&receipt).Error; err != nil {
			return err
		}
		*out = model.AdminGrantResponse{DeliveryID: delivery.ID, RequestID: req.RequestID}
		return adminAudit(tx, actor, "resource.grant.batch", strconv.FormatInt(req.PlayerID, 10), rewards+"; request="+req.RequestID+"; "+req.Reason)
	})
	if err == nil {
		s.publishInventoryMailRedPoints(ctx, req.PlayerID)
	}
	return out, err
}

// AdminInventoryBatch shares one transaction with the single-resource mutation and its guards.
func (s *GameService) AdminInventoryBatch(ctx context.Context, actor *model.AdminPrincipal, id int64, req model.AdminInventoryBatchRequest) (*model.AdminPlayerInventoryMutationResponse, error) {
	out := &model.AdminPlayerInventoryMutationResponse{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if err := AdminCheckProfileScope(ctx, row); err != nil {
			return err
		}
		target := strconv.FormatInt(id, 10) + ":batch:" + req.RequestID
		hashReq := req
		hashReq.Version = ""
		hash := p3Hash(adminJSON(hashReq))
		var receipt db.AdminAudit
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("target=? AND action=?", target, constUtil.AdminInventoryBatchAuditAction).First(&receipt).Error
		if err == nil {
			var saved struct {
				BodyHash string `json:"bodyHash"`
			}
			if receipt.Actor != actor.Username || json.Unmarshal([]byte(receipt.Detail), &saved) != nil || saved.BodyHash != hash {
				return constUtil.ErrAdminConfigConflict
			}
			if s.P9PetsReady {
				list, err := P9PetList(tx.Clauses(clause.Locking{Strength: "UPDATE"}), id)
				if err != nil {
					return err
				}
				out.Version = adminInventoryVersion(row, list)
			} else {
				out.Version = adminInventoryVersion(row, nil)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		_, err = uuid.Parse(req.RequestID)
		if err != nil {
			return constUtil.ErrAdminInput
		}
		out.Version = req.Version
		for index, resource := range req.Resources {
			amount := resource.Amount
			item := model.AdminPlayerInventoryRequest{Version: out.Version, RequestID: "batch:" + req.RequestID + ":" + strconv.Itoa(index), Operation: "add", Category: resource.Category, ResourceID: resource.ResourceID, Amount: &amount, Reason: req.Reason}
			if err := s.adminApplyPlayerInventory(tx, actor, id, item, out); err != nil {
				return err
			}
		}
		return adminAudit(tx, actor, constUtil.AdminInventoryBatchAuditAction, target, string(adminJSON(struct {
			BodyHash string                           `json:"bodyHash"`
			Request  model.AdminInventoryBatchRequest `json:"request"`
		}{hash, hashReq})))
	})
	return out, err
}
