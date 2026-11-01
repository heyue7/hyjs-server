package service

import (
	"bytes"
	"encoding/json"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p10ChatAvatar uses the same first camp as the client's own chat icon.
// An invalid or unowned camp is never projected as another player's avatar.
func (s *GameService) p10ChatAvatar(player *pb.Player, game *model.GameState) (string, bool) {
	heroID, err := P5WorldHeroID(player, game)
	if err != nil || heroID == "" {
		return "", false
	}
	return s.p10ChatIcon(heroID)
}

func (s *GameService) p10ChatIcon(heroID string) (string, bool) {
	hero := s.table("Hero", "id", heroID)
	if hero == nil || hero["iconName"] == "" {
		return "", false
	}
	return heroID, true
}

type p10ChatAvatarRef struct {
	senderID int64
	info     *pb.SimplePlayerInfo
}

// p10ApplyChatAvatars runs after history visibility checks and projects only
// current avatars. Stored frames and exact-retry receipts remain untouched.
// Missing or invalid profiles keep their existing message snapshot.
func (s *GameService) p10ApplyChatAvatars(tx *gorm.DB, refs []p10ChatAvatarRef) {
	ids := make([]int64, 0, len(refs))
	seen := make(map[int64]bool, len(refs))
	for _, ref := range refs {
		if ref.senderID <= 0 || ref.info == nil || ref.info.GetPlayerId() != ref.senderID || seen[ref.senderID] {
			continue
		}
		seen[ref.senderID] = true
		ids = append(ids, ref.senderID)
	}
	if len(ids) == 0 {
		return
	}
	rows, err := tx.Model(&db.Profile{}).Select("id", "player", "game").Where("id IN ?", ids).Rows()
	if err != nil {
		// Avatar lookup is optional; keep the already authorized message snapshots.
		elog.Warn("chat avatar history projection unavailable", zap.Error(err))
		return
	}
	defer rows.Close()
	avatars := make(map[int64]string, len(ids))
	for rows.Next() {
		var row db.Profile
		if err := tx.ScanRows(rows, &row); err != nil {
			elog.Warn("chat avatar history projection unavailable", zap.Error(err))
			return
		}
		if row.Player == nil || row.Game == nil {
			continue
		}
		var player struct {
			HeroWarID *string `json:"heroWarId"`
		}
		var game struct {
			Warriors  json.RawMessage     `json:"warriors"`
			Formation []*pb.FormationItem `json:"formation"`
		}
		if json.Unmarshal([]byte(*row.Player), &player) != nil || json.Unmarshal([]byte(*row.Game), &game) != nil || len(game.Warriors) == 0 {
			continue
		}
		var roster struct {
			Warriors []*struct {
				ID   *string `json:"id"`
				GUID *string `json:"guid"`
			} `json:"warriors"`
		}
		rosterJSON := bytes.TrimSpace(game.Warriors)
		if len(rosterJSON) > 0 && rosterJSON[0] == '{' {
			// Reuse the bounded codec check on just the roster; the remaining
			// game fields never enter expansion or a full game decode.
			wrapped := make([]byte, 0, len(rosterJSON)+13)
			wrapped = append(wrapped, `{"warriors":`...)
			wrapped = append(wrapped, rosterJSON...)
			wrapped = append(wrapped, '}')
			expanded, err := model.ExpandProfileGame86(wrapped)
			if err != nil || json.Unmarshal(expanded, &roster) != nil {
				continue
			}
		} else if json.Unmarshal(rosterJSON, &roster.Warriors) != nil {
			continue
		}
		projection := &model.GameState{Formation: game.Formation}
		for _, warrior := range roster.Warriors {
			if warrior != nil {
				projection.Warriors = append(projection.Warriors, &pb.Warrior{Id: warrior.ID, Guid: warrior.GUID})
			}
		}
		if avatar, ok := s.p10ChatAvatar(&pb.Player{HeroWarId: player.HeroWarID}, projection); ok {
			avatars[row.Id] = avatar
		}
	}
	if err := rows.Err(); err != nil {
		elog.Warn("chat avatar history projection unavailable", zap.Error(err))
		return
	}
	if err := rows.Close(); err != nil {
		elog.Warn("chat avatar history projection unavailable", zap.Error(err))
		return
	}
	for _, ref := range refs {
		if ref.info != nil && ref.info.GetPlayerId() == ref.senderID {
			if avatar, ok := avatars[ref.senderID]; ok {
				ref.info.HeroWarId = proto.String(avatar)
			}
		}
	}
}
