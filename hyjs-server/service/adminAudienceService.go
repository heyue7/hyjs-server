package service

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminAudienceMember is one frozen character selected for an administrative delivery.
type adminAudienceMember struct {
	PlayerID   int64
	PlayerName string
}

// adminAudienceScope inherits the authenticated realm, actor and runtime binding without requiring normal login.
func (s *GameService) adminAudienceScope(ctx context.Context, tx *gorm.DB, actor *model.AdminPrincipal, write bool) (*db.AdminGameServer, error) {
	if err := adminOperationUser(tx, actor, write); err != nil {
		return nil, err
	}
	id := AdminSelectedServerID(ctx)
	if id < 1 || id != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	var realm db.AdminGameServer
	if err := tx.First(&realm, id).Error; err != nil {
		return nil, err
	}
	// Maintenance compensation and mail must also work while normal realm login is disabled.
	if !realm.RuntimeBound {
		return nil, constUtil.ErrAdminPermission
	}
	return &realm, nil
}

// adminAudienceEligible shares the protected-account boundary, rejects frozen accounts and corrupt identity reads.
func adminAudienceEligible(tx *gorm.DB, profile db.Profile) (string, bool, error) {
	if profile.Player == nil || *profile.Player == "" || *profile.Player == "null" {
		return "", false, nil
	}
	var player struct {
		Name  string `json:"name"`
		Level int64  `json:"level"`
	}
	if err := json.Unmarshal([]byte(*profile.Player), &player); err != nil {
		return "", false, err
	}
	if player.Name == "" || player.Level < 1 {
		return "", false, nil
	}
	if profile.AccountID == 0 {
		return player.Name, profile.Id > 1, nil
	}
	account, err := AccountForProfile(tx, &profile)
	if err != nil {
		return "", false, err
	}
	// Disabled accounts are frozen for login and must not receive new player-visible deliveries.
	return player.Name, account.Enabled && !account.Protected, nil
}

// adminAudienceFreeze resolves either every eligible character in the realm or the explicit cross-page selection.
func adminAudienceFreeze(tx *gorm.DB, realmID int64, mode string, playerIDs []int64) ([]adminAudienceMember, int64, error) {
	query := tx.Select("id", "account_id", "server_id", "player").Where("player IS NOT NULL")
	// Legacy profiles without an explicit realm still belong to the default realm.
	if realmID == constUtil.GameDefaultServerID {
		query = query.Where("(server_id=? OR server_id=0)", realmID)
	} else {
		query = query.Where("server_id=?", realmID)
	}
	explicit := mode == constUtil.AdminAudienceModePlayers
	if explicit {
		query = query.Where("id IN ?", playerIDs)
	}
	var profiles []db.Profile
	if err := query.Order("id").Find(&profiles).Error; err != nil {
		return nil, 0, err
	}
	if explicit && len(profiles) != len(playerIDs) {
		return nil, 0, constUtil.ErrAdminAudience
	}
	members := make([]adminAudienceMember, 0, len(profiles))
	var excluded int64
	for _, profile := range profiles {
		name, eligible, err := adminAudienceEligible(tx, profile)
		if err != nil {
			return nil, 0, err
		}
		if name == "" {
			if explicit {
				return nil, 0, constUtil.ErrAdminAudience
			}
			continue
		}
		if !eligible {
			excluded++
			continue
		}
		members = append(members, adminAudienceMember{PlayerID: profile.Id, PlayerName: name})
	}
	return members, excluded, nil
}

// adminAudienceStoredMode normalizes historical rows created before audience selection existed.
func adminAudienceStoredMode(mode string) string {
	if mode == constUtil.AdminAudienceModePlayers {
		return constUtil.AdminAudienceModePlayers
	}
	return constUtil.AdminAudienceModeAll
}
