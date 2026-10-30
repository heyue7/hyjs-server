package service

import (
	"errors"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// The paused, shared-clock patrol snapshot requires the corresponding client
// interpolation and cannot be interpreted by older desert clients.
func ggoClientBuildCompatible(configured, build int64) bool {
	const patrolProtocolBuild int64 = 2026103102
	return configured > 0 && build >= configured && build >= patrolProtocolBuild
}

// Caller holds this player's profile lock. The build is taken only from the
// server's login binding for the same current 16-byte session, never the body.
// A zero draft threshold fails closed; publication validates it separately.
func ggoCheckClientBuildLocked(tx *gorm.DB, id int64, session []byte, cfg *model.AdminFieldGGOConfiguration) error {
	if cfg == nil || cfg.Runtime == nil || cfg.Runtime.MinClientBuild < 1 || len(session) != 16 {
		return ggoFieldError(constUtil.GGOFieldClientUpdateRequired, constUtil.ClientUpdateDownloadText)
	}
	var binding db.ClientUpdateSession
	err := tx.Select("player_id", "session_hash", "build_code").First(&binding, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ggoFieldError(constUtil.GGOFieldClientUpdateRequired, constUtil.ClientUpdateDownloadText)
	}
	if err != nil {
		return err
	}
	if binding.SessionHash != clientUpdateSessionHash(session) || !ggoClientBuildCompatible(cfg.Runtime.MinClientBuild, binding.BuildCode) {
		return ggoFieldError(constUtil.GGOFieldClientUpdateRequired, constUtil.ClientUpdateDownloadText)
	}
	return nil
}

// Preserve the existing update text on the JSON endpoints as well as native
// game errors; the normal wrapper otherwise hides ordinary errors as system errors.
func ggoClientAuthError(err error) error {
	if errors.Is(err, constUtil.ErrClientUpdateRequired) {
		return ggoFieldError(constUtil.GGOFieldClientUpdateRequired, constUtil.ClientUpdateDownloadText)
	}
	return err
}

// ggoExtractionEnabled keeps the release gate closed unless this process was
// explicitly started in the isolated loopback integration environment.
func (s *GameService) ggoExtractionEnabled() bool {
	return constUtil.GGOExtractionReady || s != nil && s.GGOIntegrationReady
}
