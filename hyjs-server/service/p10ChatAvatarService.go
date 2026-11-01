package service

import (
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
	var rows []db.Profile
	if err := tx.Select("id", "player", "game").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		// Avatar lookup is optional; keep the already authorized message snapshots.
		elog.Warn("chat avatar history projection unavailable", zap.Error(err))
		return
	}
	avatars := make(map[int64]string, len(rows))
	for _, row := range rows {
		if row.Player == nil || row.Game == nil {
			continue
		}
		var player struct {
			HeroWarID *string `json:"heroWarId"`
		}
		var game struct {
			Warriors []*struct {
				ID   *string `json:"id"`
				GUID *string `json:"guid"`
			} `json:"warriors"`
			Formation []*pb.FormationItem `json:"formation"`
		}
		if json.Unmarshal([]byte(*row.Player), &player) != nil || json.Unmarshal([]byte(*row.Game), &game) != nil {
			continue
		}
		projection := &model.GameState{Formation: game.Formation}
		for _, warrior := range game.Warriors {
			if warrior != nil {
				projection.Warriors = append(projection.Warriors, &pb.Warrior{Id: warrior.ID, Guid: warrior.GUID})
			}
		}
		if avatar, ok := s.p10ChatAvatar(&pb.Player{HeroWarId: player.HeroWarID}, projection); ok {
			avatars[row.Id] = avatar
		}
	}
	for _, ref := range refs {
		if ref.info != nil && ref.info.GetPlayerId() == ref.senderID {
			if avatar, ok := avatars[ref.senderID]; ok {
				ref.info.HeroWarId = proto.String(avatar)
			}
		}
	}
}
