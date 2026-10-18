package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"

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
	var result model.GameGuideSkipResponse
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		var row db.Profile
		if err := tx.Select("id", "session").First(&row, id).Error; err != nil {
			return err
		}
		if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
			return errors.New(constUtil.NoviceGuideSessionInvalid)
		}
		applyNoviceGuideSkip(player, game)
		result.Skipped = true
		result.NoviceGuideSchedule = player.GetNoviceGuideSchedule()
		return nil
	})
	return &result, err
}
