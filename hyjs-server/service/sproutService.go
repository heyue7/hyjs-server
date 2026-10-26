package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// SproutActivePlayers resolves eligibility in two indexed batch reads without profile payloads.
func (s *GameService) SproutActivePlayers(tx *gorm.DB, ids []int64, now time.Time) (map[int64]bool, error) {
	out := make(map[int64]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	cfg, profiles, err := s.sproutProfiles(tx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range profiles {
		out[row.Id] = sproutExpiry(cfg, row, now) > 0
	}
	return out, nil
}
func (s *GameService) sproutProfiles(tx *gorm.DB, ids []int64) (db.AdminSproutSetting, []db.Profile, error) {
	var cfg db.AdminSproutSetting
	var profiles []db.Profile
	if err := tx.First(&cfg, 1).Error; err != nil {
		return cfg, nil, err
	}
	err := tx.Select("id", "created_at", "last_sprout_login_at", "return_sprout_started_at").Where("id IN ? AND server_id = ?", ids, s.OriginalServerID()).Find(&profiles).Error
	return cfg, profiles, err
}

// RecordSproutLogin runs only after successful CSPlayerLogin. Profile row locking makes retries idempotent.
func (s *GameService) RecordSproutLogin(ctx context.Context, id int64) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "server_id", "last_sprout_login_at", "return_sprout_started_at").First(&row, id).Error; err != nil {
			return err
		}
		if AdminProfileServerID(row) != s.OriginalServerID() {
			return constUtil.ErrAdminPermission
		}
		var cfg db.AdminSproutSetting
		if err := tx.First(&cfg, 1).Error; err != nil {
			return err
		}
		var presence db.P10ScenePresence
		if err := tx.Select("player_id", "updated_at").First(&presence, "player_id=?", id).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().UTC()
		start := sproutReturnStart(cfg, row, presence.UpdatedAt, now)
		return tx.Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"last_sprout_login_at": max(row.LastSproutLoginAt, now.UnixMilli()), "return_sprout_started_at": start}).Error
	})
}

// SproutStatus authorizes the requesting role and limits peers to its current scene and line.
func (s *GameService) SproutStatus(ctx context.Context, req model.SproutStatusRequest, session []byte) (*model.SproutStatusResponse, error) {
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	own, err := AuthorizeAccount(ctx, req.PlayerID, session)
	if err != nil {
		return nil, err
	}
	if AdminProfileServerID(*own) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	tx := db.GetIns().WithContext(ctx)
	var presence db.P10ScenePresence
	err = tx.Select("player_id", "scene", "line", "session_hash").First(&presence, "player_id=?", req.PlayerID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	allowed := map[int64]bool{req.PlayerID: true}
	if err == nil && presence.SessionHash == p6Digest(session) {
		var peers []db.P10ScenePresence
		if err = tx.Select("player_id").Where("player_id IN ? AND scene=? AND line=? AND session_hash<>''", req.PlayerIDs, presence.Scene, presence.Line).Find(&peers).Error; err != nil {
			return nil, err
		}
		for _, peer := range peers {
			allowed[peer.PlayerID] = true
		}
	}
	for _, id := range req.PlayerIDs {
		if !allowed[id] {
			return nil, constUtil.ErrAdminPermission
		}
	}
	cfg, profiles, err := s.sproutProfiles(tx, req.PlayerIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]db.Profile, len(profiles))
	for _, row := range profiles {
		byID[row.Id] = row
	}
	now := time.Now().UTC()
	out := &model.SproutStatusResponse{ServerTime: now.UnixMilli(), Icon: constUtil.AdminSproutIcon, Players: make([]model.SproutStatusPlayer, 0, len(req.PlayerIDs))}
	for _, id := range req.PlayerIDs {
		row, ok := byID[id]
		if !ok {
			return nil, constUtil.ErrAdminPermission
		}
		expiry := sproutExpiry(cfg, row, now)
		out.Players = append(out.Players, model.SproutStatusPlayer{PlayerID: id, Active: expiry > 0, ExpiresAt: expiry})
	}
	return out, nil
}
