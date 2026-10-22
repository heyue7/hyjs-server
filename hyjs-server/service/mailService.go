package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// EnqueueMail 在持有玩家行锁的调用方事务内创建原协议邮件，删除和过期不重建同源邮件。
func (s *GameService) EnqueueMail(tx *gorm.DB, playerID int64, sourceKey, mailbox, title, body, targetID string, mailType int32, expiresAt int64) (*db.MailMessage, error) {
	if playerID <= 0 || !p1SourceValid(sourceKey) || mailbox == "" || len(mailbox) > 32 || len(title) > 191 || len(body) > 16384 || len(targetID) > 191 || mailType < 0 || expiresAt < 0 {
		return nil, errors.New("邮件参数无效")
	}
	var row db.MailMessage
	err := tx.Where("player_id = ? AND source_key = ?", playerID, sourceKey).First(&row).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	now := time.Now().UnixMilli()
	// 原删除协议只有类型和时间，序号化同玩家同类型毫秒时间避免误删另一封。
	var last db.MailMessage
	err = tx.Where("player_id = ? AND type = ?", playerID, mailType).Order("created_at DESC").First(&last).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && last.CreatedAt >= now {
		now = last.CreatedAt + 1
	}
	row = db.MailMessage{PlayerID: playerID, SourceKey: sourceKey, Mailbox: mailbox, Title: title, Content: body, TargetID: targetID, Type: mailType, CreatedAt: now, ExpiresAt: expiresAt}
	if err = tx.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// EnqueueRewardNotice 保留兼容入口，仅投递领奖中心；业务邮件由调用方显式 EnqueueMail。
func (s *GameService) EnqueueRewardNotice(tx *gorm.DB, playerID int64, sourceKey, title, body, rewards string, expiresAt int64) (*db.RewardDelivery, error) {
	return s.EnqueueReward(tx, playerID, sourceKey, title, body, rewards, expiresAt)
}

// Mailbox 读取原协议邮箱；取列表视为已读，未领附件的邮件不得删除。
func (s *GameService) Mailbox(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		now := time.Now().UnixMilli()
		active := tx.Model(&db.MailMessage{}).Where("player_id = ? AND deleted_at = 0 AND (expires_at = 0 OR expires_at > ?)", id, now)
		switch b := request.(type) {
		case *pb.CSCheckNewMail:
			var count int64
			if err := active.Where("read_at = 0").Count(&count).Error; err != nil {
				return err
			}
			has := int32(0)
			if count > 0 {
				has = 1
			}
			response = &pb.SCCheckNewMail{HasNew: proto.Int32(has)}
		case *pb.CSMailList:
			filter := b.GetMailType()
			if len(filter) > 32 || strings.ContainsAny(filter, "\r\n\x00") {
				return errors.New("邮箱类型无效")
			}
			if filter != "" && filter != "all" && filter != "allMail" {
				active = active.Where("mailbox = ?", filter)
			}
			var rows []db.MailMessage
			if err := active.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
				return err
			}
			// 邮件与领奖中心使用同一奖励快照文案；批量读取，保留原邮件标识和已读流程。
			var rewardIDs []int64
			for _, row := range rows {
				if row.RewardID > 0 && strings.HasPrefix(row.SourceKey, "activity:") {
					rewardIDs = append(rewardIDs, row.RewardID)
				}
			}
			notices := map[int64]db.RewardDelivery{}
			if len(rewardIDs) > 0 {
				var rewards []db.RewardDelivery
				if err := tx.Where("player_id=? AND id IN ?", id, rewardIDs).Find(&rewards).Error; err != nil {
					return err
				}
				if err := projectPeriodRewardNotices(tx, id, rewards); err != nil {
					return err
				}
				for _, reward := range rewards {
					notices[reward.ID] = reward
				}
			}
			result := &pb.SCMailList{}
			indices := map[string]int{}
			var unread []int64
			for _, row := range rows {
				if notice, ok := notices[row.RewardID]; ok {
					row.Title, row.Content = notice.Title, notice.Content
				}
				index, ok := indices[row.Mailbox]
				if !ok {
					index = len(result.MailBoxList)
					indices[row.Mailbox] = index
					result.MailBoxList = append(result.MailBoxList, &pb.MailBox{Id: proto.String(row.Mailbox)})
				}
				result.MailBoxList[index].MailList = append(result.MailBoxList[index].MailList, &pb.MailItem{Title: proto.String(row.Title), Content: proto.String(row.Content), Time: proto.Int64(row.CreatedAt), TargetId: proto.String(row.TargetID), Type: proto.Int32(row.Type)})
				if row.ReadAt == 0 {
					unread = append(unread, row.ID)
				}
			}
			if len(unread) > 0 {
				if err := tx.Model(&db.MailMessage{}).Where("id IN ? AND player_id = ?", unread, id).Update("read_at", now).Error; err != nil {
					return err
				}
			}
			response = result
		case *pb.CSDeleteMail:
			if b.GetCreateTime() <= 0 || b.GetType() < 0 {
				return errors.New("邮件标识无效")
			}
			var row db.MailMessage
			if err := tx.Where("player_id = ? AND type = ? AND created_at = ?", id, b.GetType(), b.GetCreateTime()).First(&row).Error; err != nil {
				return errors.New("邮件不存在")
			}
			if row.RewardID > 0 {
				var reward db.RewardDelivery
				if err := tx.Where("id = ? AND player_id = ?", row.RewardID, id).First(&reward).Error; err != nil {
					return err
				}
				if reward.ClaimedAt == 0 && (reward.ExpiresAt == 0 || reward.ExpiresAt > now) {
					return errors.New("请先在领奖中心领取附件")
				}
			}
			if row.DeletedAt == 0 {
				if err := tx.Model(&row).Update("deleted_at", now).Error; err != nil {
					return err
				}
			}
			response = &pb.SCDeleteMail{}
		default:
			return errors.New("邮箱请求类型无效")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
