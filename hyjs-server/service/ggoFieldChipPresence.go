package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ggoChipCount compares economic totals independently of temporary scene
// visibility, so a colour-only exchange does not trigger a display refresh.
func ggoChipCount(game *model.GameState) (int64, error) {
	if game == nil || game.GGOField == nil || game.Position.GetScene() != constUtil.GGOFieldScene {
		return 0, nil
	}
	_, total, err := ggoProtectedChips(game.GGOField.Chips, 0)
	return total, err
}

// Called after the profile transaction has committed. The narrow presence
// lookup scopes the existing coalesced hint to the player's current line.
func (s *GameService) ggoNotifyChipPlayerChange(ctx context.Context, id int64) {
	if s == nil || id < 1 {
		return
	}
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	var presence db.P10ScenePresence
	err := db.GetIns().WithContext(callCtx).Select("scene", "line").First(&presence, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if err != nil {
		// The coalesced broadcaster retries its own SQL failures.
		s.ggoNotifyFieldChange(ctx, 0)
		return
	}
	if presence.Scene == constUtil.GGOFieldScene && presence.Line > 0 {
		s.ggoNotifyFieldChange(ctx, presence.Line)
	}
}

// ggoEnsureChipProjection lazily initializes an old profile. The conditional
// write cannot overwrite a concurrent committed terminal change.
func ggoEnsureChipProjection(tx *gorm.DB, row *db.Profile, game *model.GameState) error {
	if row.Game == nil || row.GGOChipTotal != nil || game.GGOField == nil || p10SceneScope(game) != constUtil.GGOFieldScene {
		return nil
	}
	_, total, err := ggoProtectedChips(game.GGOField.Chips, 0)
	if err != nil {
		return err
	}
	// Scene synchronization may have decoded before another transaction began
	// battle or left the map, both of which also leave the projection NULL.
	// Compare bytes, including whitespace, even if an older MySQL database uses
	// a case-folding or space-padding text collation for its saved game.
	gameEquals := "CAST(game AS BLOB)=CAST(? AS BLOB)"
	if tx.Dialector.Name() == "mysql" {
		gameEquals = "BINARY game=BINARY ?"
	}
	result := tx.Model(&db.Profile{}).Where("id=? AND ggo_chip_total IS NULL", row.Id).Where(gameEquals, *row.Game).UpdateColumn("ggo_chip_total", total)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		row.GGOChipTotal = &total
	}
	return nil
}

type ggoChipPresenceRow struct {
	PlayerID               int64
	SessionHash            string
	OfflineUntil           int64
	Session                []byte
	AccountID              int64
	ProfileAccountRevision int64
	GGOChipTotal           *int64
}

// ggoFieldChipCounts only reads indexed presence and narrow profile/account
// columns. A missing projection is withheld until that player initializes it.
// The caller has already authenticated its own current desert presence.
func (s *GameService) ggoFieldChipCounts(tx *gorm.DB, actor *ggoFieldActor) ([]model.GGOFieldChipCountView, error) {
	result := make([]model.GGOFieldChipCountView, 0)
	if actor.Presence.Scene != constUtil.GGOFieldScene || actor.Presence.Line < 1 || len(actor.Presence.Frame) == 0 {
		return result, nil
	}
	now := time.Now().UnixMilli()
	var rows []ggoChipPresenceRow
	query := tx.Table("p10_scene_presence AS p").
		Select("p.player_id, p.session_hash, p.offline_until, pr.session, pr.account_id, pr.account_auth_revision AS profile_account_revision, pr.ggo_chip_total").
		Joins("JOIN profiles AS pr ON pr.id=p.player_id").
		Where("p.scene=? AND p.line=? AND length(p.frame)>0 AND pr.ggo_chip_total IS NOT NULL", constUtil.GGOFieldScene, actor.Presence.Line).
		Where("(pr.server_id=? OR (?=? AND pr.server_id<=0))", s.OriginalServerID(), s.OriginalServerID(), constUtil.GameDefaultServerID).
		Where("(p.session_hash<>? OR p.offline_until>?)", "", now).
		Order("p.player_id").Limit(1000)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	accountIDs := make([]int64, 0, len(rows))
	seen := make(map[int64]bool)
	for _, row := range rows {
		if row.AccountID > 0 && !seen[row.AccountID] {
			seen[row.AccountID] = true
			accountIDs = append(accountIDs, row.AccountID)
		}
	}
	accounts := make(map[int64]db.GameAccount, len(accountIDs))
	if len(accountIDs) > 0 {
		var found []db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Select("id", "enabled", "auth_revision").Where("id IN ?", accountIDs).Find(&found).Error; err != nil {
			return nil, err
		}
		for _, account := range found {
			accounts[account.ID] = account
		}
	}
	for _, row := range rows {
		if row.GGOChipTotal == nil || *row.GGOChipTotal < 0 {
			continue
		}
		if row.AccountID > 0 {
			account, ok := accounts[row.AccountID]
			if !ok || !account.Enabled || account.AuthRevision != row.ProfileAccountRevision {
				continue
			}
		}
		if row.SessionHash == "" {
			if row.OfflineUntil <= now {
				continue
			}
		} else {
			value, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), row.PlayerID})
			if !ok {
				continue
			}
			sink := value.(*p10ClientSink)
			if len(sink.session) != 16 || subtle.ConstantTimeCompare(sink.session, row.Session) != 1 || row.SessionHash != p6Digest(sink.session) {
				continue
			}
		}
		result = append(result, model.GGOFieldChipCountView{PlayerID: row.PlayerID, Count: *row.GGOChipTotal})
	}
	return result, nil
}
