package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// initializeAdminFieldChannelPolicy activates the verified original mechanism once, without undoing later operator shutdown.
func (s *GameService) initializeAdminFieldChannelPolicy(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if !gameSchemaHasTable(tx, &db.P10ScenePolicy{}) {
		return nil
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var policy db.P10ScenePolicy
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, constUtil.P10ScenePolicyID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		// Only the shipped implementation's exact source identity grants this versioned activation.
		if policy.Source != constUtil.P10SceneSource {
			return nil
		}
		marker := db.AdminFieldChannelActivation{ServerID: s.OriginalServerID(), Version: 1, Source: constUtil.P10SceneSource}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&db.P10ScenePolicy{}).Where("id=? AND source=?", constUtil.P10ScenePolicyID, constUtil.P10SceneSource).Update("enabled", true).Error
	})
}

// adminFieldSceneConfig overlays only this map's line capacity onto the existing original presence policy.
func (s *GameService) adminFieldSceneConfig(tx *gorm.DB, scene string, base *db.P10ScenePolicy) (*db.P10ScenePolicy, error) {
	if base == nil || !constUtil.P8OriginalFieldScene(scene) || !gameSchemaHasTable(tx, &db.AdminFieldChannel{}) {
		return base, nil
	}
	var row db.AdminFieldChannel
	err := tx.First(&row, "server_id=? AND scene=?", s.OriginalServerID(), scene).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return base, nil
	}
	if err != nil {
		return nil, err
	}
	if row.LineCount < 1 || row.LineCount > 32 || row.MaxPlayers < 1 || row.MaxPlayers > 1000 {
		return nil, constUtil.ErrAdminConfiguration
	}
	result := *base
	result.LineCount, result.MaxPlayers = int32(row.LineCount), int32(row.MaxPlayers)
	return &result, nil
}

// AdminFieldChannels projects actual live socket counts; planned realms have no fabricated players.
func (s *GameService) AdminFieldChannels(ctx context.Context, actor *model.AdminPrincipal, serverID int64) (*model.AdminFieldChannelsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	out := &model.AdminFieldChannelsResponse{ServerID: serverID, Items: []model.AdminFieldChannelDTO{}}
	var defaults db.P10ScenePolicy
	if err := tx.First(&defaults, constUtil.P10ScenePolicyID).Error; err != nil {
		return nil, err
	}
	out.Enabled = defaults.Enabled && serverID == s.OriginalServerID()
	for _, field := range s.Tables["Field"] {
		scene := field["FieldId"]
		var row db.AdminFieldChannel
		err := tx.First(&row, "server_id=? AND scene=?", serverID, scene).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		value := model.AdminFieldChannelDTO{Scene: scene, Name: s.adminFieldName(scene), LineCount: int64(defaults.LineCount), MaxPlayers: int64(defaults.MaxPlayers), Lines: []model.AdminFieldChannelLine{}}
		if err == nil {
			value.LineCount, value.MaxPlayers, value.Version = row.LineCount, row.MaxPlayers, adminVersion(row)
		}
		for line := int64(1); line <= value.LineCount; line++ {
			var count int64
			if serverID == s.OriginalServerID() {
				peers, err := s.p10ScenePeers(tx, scene, int32(line), 0)
				if err != nil {
					return nil, err
				}
				count = int64(len(peers))
			}
			value.Lines = append(value.Lines, model.AdminFieldChannelLine{Line: line, Online: count})
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

// AdminSaveFieldChannels serializes capacity changes with original line transfer and refuses to evict live players.
func (s *GameService) AdminSaveFieldChannels(ctx context.Context, actor *model.AdminPrincipal, req model.AdminFieldChannelSaveRequest) (*model.AdminFieldChannelsResponse, error) {
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var server db.AdminGameServer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&server, req.ServerID).Error; err != nil {
			return err
		}
		var row db.AdminFieldChannel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=? AND scene=?", req.ServerID, req.Scene).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && adminVersion(row) != req.Version || errors.Is(err, gorm.ErrRecordNotFound) && req.Version != "" {
			return constUtil.ErrAdminConfigConflict
		}
		if req.ServerID == s.OriginalServerID() {
			for line := int32(1); line <= 32; line++ {
				peers, err := s.p10ScenePeers(tx, req.Scene, line, 0)
				if err != nil {
					return err
				}
				if int64(line) > req.LineCount && len(peers) > 0 || int64(len(peers)) > req.MaxPlayers {
					return errors.New("目标线路仍有在线玩家，请先等待玩家离线，或提高线路数量和人数上限")
				}
			}
		}
		row.ServerID, row.Scene, row.LineCount, row.MaxPlayers = req.ServerID, req.Scene, req.LineCount, req.MaxPlayers
		row.Revision++
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, actor, "field.channels.save", fmt.Sprintf("server=%d scene=%s", req.ServerID, req.Scene), fmt.Sprintf("lines=%d capacity=%d", req.LineCount, req.MaxPlayers))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminFieldChannels(ctx, actor, req.ServerID)
}
