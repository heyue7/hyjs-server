package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InventoryMailRedPoints 从原合成门槛和实际待领附件读取状态，零值也下发以清除旧点。
func (s *GameService) InventoryMailRedPoints(ctx context.Context, id int64) (*pb.SCRedPoint, error) {
	values := map[string]bool{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		// UIFragItem 与 CSCombine 均用 Frag.num；未知或不完整行不生成可合成提示。
		for _, item := range game.Items {
			row := s.table("Frag", "id", item.GetId())
			need, parseErr := strconv.ParseInt(row["num"], 10, 32)
			if parseErr != nil || need < 1 || int64(item.GetNum()) < need {
				continue
			}
			switch target := row["target"]; {
			case strings.HasPrefix(target, "hero_"):
				values[constUtil.RedPointWarriorFrag] = true
			case strings.HasPrefix(target, "equip_"):
				values[constUtil.RedPointEquipmentFrag] = true
			case strings.HasPrefix(target, "pet_"):
				values[constUtil.RedPointPetFrag] = true
			}
		}
		values[constUtil.RedPointBackpack] = values[constUtil.RedPointWarriorFrag] || values[constUtil.RedPointEquipmentFrag] || values[constUtil.RedPointPetFrag]
		if tx.Migrator().HasTable(&db.RewardDelivery{}) {
			var count int64
			if err = tx.Model(&db.RewardDelivery{}).Where("player_id=? AND claimed_at=0 AND (expires_at=0 OR expires_at>?)", id, LocalPlayNow(ctx).UnixMilli()).Limit(1).Count(&count).Error; err != nil {
				return err
			}
			values[constUtil.RedPointRewardCenter] = count > 0
		}
		if tx.Migrator().HasTable(&db.MailMessage{}) {
			var count int64
			// 邮件已读/过期沿原邮箱实际时间；附件沿领奖中心的游戏日历。
			if err = tx.Model(&db.MailMessage{}).Where("player_id=? AND read_at=0 AND deleted_at=0 AND (expires_at=0 OR expires_at>?)", id, time.Now().UnixMilli()).Limit(1).Count(&count).Error; err != nil {
				return err
			}
			values[constUtil.RedPointMessage] = count > 0
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := &pb.SCRedPoint{}
	for _, kind := range []string{constUtil.RedPointRewardCenter, constUtil.RedPointMessage, constUtil.RedPointBackpack, constUtil.RedPointWarriorFrag, constUtil.RedPointEquipmentFrag, constUtil.RedPointPetFrag} {
		value := int32(0)
		if values[kind] {
			value = 1
		}
		out.RedPointList = append(out.RedPointList, &pb.RedPoint{Type: proto.String(kind), Value: proto.Int32(value)})
	}
	return out, nil
}

// publishInventoryMailRedPoints 只在事务提交后发送；重新核实会话的现有TCP通道处理在线刷新。
func (s *GameService) publishInventoryMailRedPoints(ctx context.Context, id int64) {
	if points, err := s.InventoryMailRedPoints(ctx, id); err == nil {
		s.publishP10Private(ctx, id, points)
	}
}

func inventoryResourceResponse(message protoreflect.Message, depth int) bool {
	if !message.IsValid() || depth > 6 {
		return false
	}
	switch message.Descriptor().Name() {
	case "ResultObject", "BaseResource", "SimpleItem":
		return true
	}
	changed := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind || field.IsMap() {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if inventoryResourceResponse(list.Get(i).Message(), depth+1) {
					changed = true
					return false
				}
			}
		} else {
			changed = inventoryResourceResponse(value.Message(), depth+1)
		}
		return !changed
	})
	return changed
}

func (s *GameService) afterInventoryMailRequest(ctx context.Context, id int64, request proto.Message, responses []proto.Message) {
	switch request.(type) {
	case *pb.CSAddRewardsToPlayer, *pb.CSMailList, *pb.CSDeleteMail, *pb.CSCombine, *pb.CSItemUse, *pb.CSWarriorFragSell, *pb.CSEquipmentFragSell, *pb.CSPetFragSell:
		s.publishInventoryMailRedPoints(ctx, id)
		return
	}
	// 购买、掉落、活动和邮件等奖励共用原资源容器。实际回包存在资源字段时
	// 统一刷新，覆盖新来源且避开不含资源的移动/心跳等高频请求。
	for _, response := range responses {
		if response != nil && inventoryResourceResponse(response.ProtoReflect(), 0) {
			s.publishInventoryMailRedPoints(ctx, id)
			return
		}
	}
}
