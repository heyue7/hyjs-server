package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP1 创建独立公共能力表；既有玩法与存档结构保持原样。
func (s *GameService) InitializeP1(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.RewardDelivery{}, &db.MailMessage{}, &db.EventFact{}, &db.PeriodInstance{}, &db.PeriodProgress{}, &db.PeriodSettlement{}, &db.P1LoginDirectoryMember{}); err != nil {
		return err
	}
	if err := s.seedP1Periods(ctx, time.Now()); err != nil {
		return err
	}
	if err := initializeP1Runtime(); err != nil {
		return err
	}
	s.P1Ready = true
	return nil
}

// GrantFixedReward 复用已验证的固定资源与实例规则，调用方持有角色行锁并统一提交。
func (s *GameService) GrantFixedReward(player *pb.Player, game *model.GameState, content string) (*pb.ResultObject, error) {
	if player == nil || game == nil || strings.TrimSpace(content) == "" {
		return nil, errors.New("奖励内容或角色缺失")
	}
	if len(content) > 16384 || len(strings.Split(content, ";")) > constUtil.P1RewardMaximumBatch {
		return nil, errors.New("奖励批次超过安全上限")
	}
	// 先在独立快照生成完整结果，混合奖励中后续失败不会留下内存半份资源。
	copyPlayer := proto.Clone(player).(*pb.Player)
	blob, err := json.Marshal(game)
	if err != nil {
		return nil, err
	}
	var copyGame model.GameState
	if err = json.Unmarshal(blob, &copyGame); err != nil {
		return nil, err
	}
	// Preserve only the transaction-local source across this internal JSON clone.
	copyGame.GuildPrayerProjection = game.GuildPrayerProjection.Clone()
	gain, err := s.openServerGain(copyPlayer, &copyGame, content)
	if err != nil {
		return nil, err
	}
	proto.Reset(player)
	proto.Merge(player, copyPlayer)
	*game = copyGame
	return gain, nil
}

func p1SourceValid(source string) bool {
	return source != "" && len(source) <= 191 && strings.TrimSpace(source) == source && !strings.ContainsAny(source, "\r\n\x00")
}

// EnqueueReward 在业务事务中冻结离线奖励。相同来源返回原记录，不覆盖已发布内容或复活已过期奖励。
func (s *GameService) EnqueueReward(tx *gorm.DB, playerID int64, sourceKey string, title, content, rewards string, expiresAt int64) (*db.RewardDelivery, error) {
	if playerID <= 0 || !p1SourceValid(sourceKey) || len(title) > 191 || len(content) > 16384 {
		return nil, errors.New("奖励来源或正文无效")
	}
	var saved db.RewardDelivery
	err := tx.Where("player_id = ? AND source_key = ?", playerID, sourceKey).First(&saved).Error
	if err == nil {
		return &saved, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	ok, err := s.openServerRewardSupported(rewards)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("奖励资源未恢复")
	}
	now := LocalPlayNow(tx.Statement.Context).UnixMilli()
	if expiresAt < 0 {
		return nil, errors.New("奖励截止时间无效")
	}
	saved = db.RewardDelivery{PlayerID: playerID, SourceKey: sourceKey, Title: title, Content: content, Rewards: rewards, CreatedAt: now, ExpiresAt: expiresAt}
	if err := tx.Create(&saved).Error; err != nil {
		return nil, err
	}
	return &saved, nil
}

func p1RewardResources(content string) ([]*pb.BaseResource, error) {
	simple, err := activityLoginResources(content)
	if err == nil {
		return simple, nil
	}
	// 展示沿用原BaseResource，角色/装备附件在领取时产生完整实例。
	var list []*pb.BaseResource
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return nil, errors.New("奖励展示格式无效")
		}
		n, err := p1PositiveAmount(pair[1])
		if err != nil {
			return nil, err
		}
		list = append(list, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(n))})
	}
	return list, nil
}

// RewardCenter 只显示未过期未领取附件；同一奖励ID领取一次，重复请求返回空增量。
func (s *GameService) RewardCenter(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		now := LocalPlayNow(ctx).UnixMilli()
		switch b := request.(type) {
		case *pb.CSQueryUserReward:
			if s.P12WantedReady {
				// 查询前在原角色行锁事务补齐悬赏日榜，随后读取相同事务的待领奖列表。
				if err := s.SyncP12WantedRewards(tx, id, time.UnixMilli(now)); err != nil {
					return err
				}
			}
			rows, err := eligiblePendingRewards75(tx, id, now, constUtil.P1RewardMaximumBatch, false)
			if err != nil {
				return err
			}
			if err := projectPeriodRewardNotices(tx, id, rows); err != nil {
				return err
			}
			result := &pb.SCQueryUserReward{}
			for _, row := range rows {
				resources, err := p1RewardResources(row.Rewards)
				if err != nil {
					return err
				}
				result.RewardItemInfoList = append(result.RewardItemInfoList, &pb.RewardItemInfo{RewardId: proto.Int64(row.ID), RewardList: resources, RewardTitle: proto.String(row.Title), RewardContent: proto.String(row.Content), RewardTime: proto.Int64(row.CreatedAt)})
			}
			response = result
		case *pb.CSAddRewardsToPlayer:
			if len(b.GetRewardIds()) < 1 || len(b.GetRewardIds()) > constUtil.P1RewardMaximumBatch {
				return errors.New("领奖数量无效")
			}
			seen := map[int64]bool{}
			gain := &pb.ResultObject{}
			// 验证并锁定全部附件后逐项发奖，任一附件失败令整个批次回滚。
			rows := make([]db.RewardDelivery, 0, len(b.GetRewardIds()))
			for _, rewardID := range b.GetRewardIds() {
				if rewardID <= 0 || seen[rewardID] {
					return errors.New("领奖ID无效或重复")
				}
				seen[rewardID] = true
				var row db.RewardDelivery
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND player_id = ?", rewardID, id).First(&row).Error; err != nil {
					return errors.New("奖励不存在或不属于当前角色")
				}
				if row.ClaimedAt == 0 && row.ExpiresAt != 0 && row.ExpiresAt <= now {
					return errors.New("奖励已过期")
				}
				rows = append(rows, row)
			}
			eligible, err := rechargeRewardEligibility75(tx, rows, now)
			if err != nil {
				return err
			}
			for _, row := range rows {
				if row.ClaimedAt != 0 {
					continue
				}
				if !eligible[row.ID] {
					return errors.New("本期充值条件未达成")
				}
				current, err := s.GrantFixedReward(player, game, row.Rewards)
				if err != nil {
					return err
				}
				blob, err := proto.Marshal(current)
				if err != nil {
					return err
				}
				row.ClaimedAt = now
				row.Result = blob
				if err = tx.Save(&row).Error; err != nil {
					return err
				}
				p1AppendGain(gain, current)
			}
			response = &pb.SCAddRewardsToPlayer{Result: gain}
		default:
			return errors.New("领奖中心请求类型无效")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

func p1AppendGain(target, source *pb.ResultObject) {
	target.Simps = append(target.Simps, source.Simps...)
	target.Wars = append(target.Wars, source.Wars...)
	target.Equips = append(target.Equips, source.Equips...)
	target.Treas = append(target.Treas, source.Treas...)
}
