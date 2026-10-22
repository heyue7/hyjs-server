package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// NoviceGuideSkipped 只识别已持久化的主动跳过标记，不根据等级推断玩家选择。
func NoviceGuideSkipped(game *model.GameState) bool {
	if game == nil {
		return false
	}
	for _, entry := range game.KV {
		if entry.GetKey() == constUtil.NoviceGuideSkipKey && entry.GetValue() == "1" {
			return true
		}
	}
	return false
}

// applyNoviceGuideSkip 结束原始初始教学段，不修改主线任务、库存、等级和后续指引。
func applyNoviceGuideSkip(player *pb.Player, game *model.GameState) {
	for _, entry := range game.KV {
		if entry.GetKey() == constUtil.NoviceGuideSkipKey {
			entry.Value = proto.String("1")
			if player.GetNoviceGuideSchedule() < constUtil.NoviceGuideInitialCompleted {
				player.NoviceGuideSchedule = proto.Int32(constUtil.NoviceGuideInitialCompleted)
			}
			return
		}
	}
	game.KV = append(game.KV, &pb.KeyValueEntry{Key: proto.String(constUtil.NoviceGuideSkipKey), Value: proto.String("1")})
	if player.GetNoviceGuideSchedule() < constUtil.NoviceGuideInitialCompleted {
		player.NoviceGuideSchedule = proto.Int32(constUtil.NoviceGuideInitialCompleted)
	}
}

// SkipNoviceGuide 在原存档锁内再次核会话并幂等保存跳过选择，失败不结束客户端教学。
func SkipNoviceGuide(ctx context.Context, id int64, session []byte) (*model.GameGuideSkipResponse, error) {
	return skipGuide(ctx, id, session, "", nil)
}

// SkipGuide uses the shipped guide table; a skip retires only teaching, never its business action or reward.
func (s *GameService) SkipGuide(ctx context.Context, id int64, session []byte, guideID string) (*model.GameGuideSkipResponse, error) {
	row := s.table("Guide", "guideId", guideID)
	if row == nil || row["guideId"] != guideID {
		return nil, errors.New(constUtil.GuideSkipInvalid)
	}
	copy := make(map[string]string, len(row))
	for key, value := range row {
		copy[key] = value
	}
	// Chained pages inherit their parent's level gate (e.g. arena -> arena shop).
	level, current := tableNumber(row, "levelTrigger"), guideID
	for range len(s.Tables["Guide"]) {
		parent := ""
		for _, candidate := range s.Tables["Guide"] {
			if candidate["endGuide"] == current {
				level = max(level, tableNumber(candidate, "levelTrigger"))
				parent = candidate["guideId"]
				break
			}
		}
		if parent == "" {
			break
		}
		current = parent
	}
	copy["levelTrigger"] = strconv.FormatInt(int64(level), 10)
	return skipGuide(ctx, id, session, guideID, copy)
}

func skipGuide(ctx context.Context, id int64, session []byte, guideID string, guide map[string]string) (*model.GameGuideSkipResponse, error) {
	var result model.GameGuideSkipResponse
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		var row db.Profile
		if err := tx.Select("id", "session").First(&row, id).Error; err != nil {
			return err
		}
		if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
			return errors.New(constUtil.NoviceGuideSessionInvalid)
		}
		if guide == nil {
			applyNoviceGuideSkip(player, game)
		} else {
			order, err := strconv.ParseInt(guide["order"], 10, 32)
			if err != nil || order < -1 || player.GetLevel() < int32(tableNumber(guide, "levelTrigger")) {
				return errors.New(constUtil.GuideSkipInvalid)
			}
			if order >= 0 && order <= int64(constUtil.NoviceGuideInitialCompleted) {
				applyNoviceGuideSkip(player, game)
			} else if order > int64(player.GetNoviceGuideSchedule()) {
				player.NoviceGuideSchedule = proto.Int32(int32(order))
			}
			key := constUtil.GuideSkipKeyPrefix + guideID
			found := false
			for _, entry := range game.KV {
				if entry.GetKey() == key {
					entry.Value = proto.String("1")
					found = true
				}
			}
			if !found {
				game.KV = append(game.KV, &pb.KeyValueEntry{Key: proto.String(key), Value: proto.String("1")})
			}
		}
		result.Skipped = true
		result.NoviceGuideSchedule = player.GetNoviceGuideSchedule()
		result.GuideID = guideID
		return nil
	})
	return &result, err
}
