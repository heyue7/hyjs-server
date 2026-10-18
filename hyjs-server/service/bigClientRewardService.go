package service

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// bigClientReward 接受原登录资源完成通知，将原配置奖励一次投递到既有领奖中心。
func (s *GameService) bigClientReward(ctx context.Context, id int64) ([]proto.Message, error) {
	if !s.P1Ready {
		return nil, errors.New(constUtil.BigClientRewardUnavailable)
	}
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, game *model.GameState) error {
		// 原SC没有资源字段，原回调仅继续登录；投递邮件后由原领奖请求更新客户端余额。
		rewards := s.table("GameConfigure", "StaticName", "BigClientReward")["NameValue"]
		if _, err := s.EnqueueRewardNotice(tx, id, constUtil.BigClientRewardSource, constUtil.BigClientRewardTitle, constUtil.BigClientRewardBody, rewards, 0); err != nil {
			return err
		}
		// 保存既有服务器通知记录；该字段不修改客户端资源标志，也不作为历史已发奖凭据。
		game.FullClientVerified = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCBigClientReward{}}, nil
}
