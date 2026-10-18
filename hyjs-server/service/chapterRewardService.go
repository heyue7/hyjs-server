package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// chapterStarReward 按原表领取原各章星宝箱，在同一存档事务内保存奖励和领取标记。
func (s *GameService) chapterStarReward(ctx context.Context, id int64, request *pb.CSCopyGetStarAward) ([]proto.Message, error) {
	row := s.table("CopyChapter", "ChapterId", request.GetChapterId())
	if row == nil {
		return nil, errors.New("章节宝箱配置缺失")
	}
	index := strconv.FormatInt(int64(request.GetStarIndex()), 10)
	condition, err := strconv.ParseInt(row["BoxCondition"+index], 10, 32)
	if err != nil || condition <= 0 {
		return nil, errors.New("章节宝箱门槛配置无效")
	}
	var result *pb.SCCopyGetStarAward
	err = updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		// 以已恢复关卡的最好成绩计算门槛，拒绝重复关卡和越界星数造成的虚增。
		var stars int32
		seen := map[string]bool{}
		for _, saved := range g.Copy.GetPassedCopyInfo() {
			if saved.GetChapterId() != request.GetChapterId() {
				continue
			}
			mission := s.table("CopyMission", "MissionId", saved.GetMissionId())
			if mission == nil || mission["ChapterId"] != request.GetChapterId() || seen[saved.GetMissionId()] || saved.GetStar() < 0 || saved.GetStar() > 5 {
				return errors.New("章节关卡星数存档无效")
			}
			seen[saved.GetMissionId()] = true
			stars += saved.GetStar()
		}
		if int64(stars) < condition {
			return errors.New("章节累计星数不足")
		}
		var chapter *pb.CopyChapterInfo
		for _, saved := range g.Copy.GetPassedChapterInfo() {
			if saved.GetChapterId() == request.GetChapterId() {
				if chapter != nil {
					return errors.New("章节宝箱存档重复")
				}
				chapter = saved
			}
		}
		if chapter == nil {
			chapter = &pb.CopyChapterInfo{ChapterId: proto.String(request.GetChapterId())}
			g.Copy.PassedChapterInfo = append(g.Copy.PassedChapterInfo, chapter)
		}
		var claimed *bool
		switch request.GetStarIndex() {
		case 0:
			claimed = chapter.Reward0Claimed
		case 1:
			claimed = chapter.Reward1Claimed
		case 2:
			claimed = chapter.Reward2Claimed
		default:
			return errors.New("章节宝箱序号无效")
		}
		if claimed != nil && *claimed {
			return errors.New("章节星宝箱已领取")
		}
		gain, err := s.chapterBoxGain(player, g, row["Box"+index])
		if err != nil {
			return err
		}
		// 标记与实际入库一起提交，失败或并发重放不会留下半份奖励。
		chapter.Star = proto.Int32(stars)
		switch request.GetStarIndex() {
		case 0:
			chapter.Reward0Claimed = proto.Bool(true)
		case 1:
			chapter.Reward1Claimed = proto.Bool(true)
		case 2:
			chapter.Reward2Claimed = proto.Bool(true)
		}
		result = &pb.SCCopyGetStarAward{Gain: gain}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// UICopyAwardEvent 回调使用 AddResultObj，单帧增量避免重复叠加余额。
	return []proto.Message{result}, nil
}

// chapterBoxGain 解析固定宝箱内容，未知奖励、溢出和实例生成失败均中止事务。
func (s *GameService) chapterBoxGain(player *pb.Player, g *model.GameState, content string) (*pb.ResultObject, error) {
	gain := &pb.ResultObject{}
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return nil, errors.New("章节宝箱奖励配置无效")
		}
		num, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || num <= 0 {
			return nil, errors.New("章节宝箱奖励数量无效")
		}
		key := pair[0]
		switch {
		case key == "coin":
			if player.GetCoin() < 0 || player.GetCoin() > math.MaxInt64-num {
				return nil, errors.New("金币余额溢出")
			}
			player.Coin = proto.Int64(player.GetCoin() + num)
		case key == "diamond":
			if player.GetDiamond() < 0 || player.GetDiamond() > math.MaxInt64-num {
				return nil, errors.New("钻石余额溢出")
			}
			player.Diamond = proto.Int64(player.GetDiamond() + num)
		case strings.HasPrefix(key, "item_"):
			if s.table("Item", "itemId", key) == nil {
				return nil, errors.New("宝箱道具配置缺失")
			}
			var owned *pb.SimpleItem
			for _, item := range g.Items {
				if item.GetId() == key {
					if owned != nil {
						return nil, errors.New("道具存档重复")
					}
					owned = item
				}
			}
			if owned == nil {
				g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
			} else {
				if owned.GetNum() < 0 || int64(owned.GetNum()) > math.MaxInt32-num {
					return nil, errors.New("道具数量溢出")
				}
				owned.Num = proto.Int32(owned.GetNum() + int32(num))
			}
		case strings.HasPrefix(key, "equip_"):
			if err := treasureBagCapacity(player, g, int32(num)); err != nil {
				return nil, err
			}
			row := s.table("Equip", "id", key)
			price, err := strconv.ParseInt(row["sellPrice"], 10, 32)
			if err != nil || price < 0 {
				return nil, errors.New("宝箱装备配置无效")
			}
			for n := int64(0); n < num; n++ {
				token := make([]byte, 16)
				if _, err := rand.Read(token); err != nil {
					return nil, err
				}
				guid := hex.EncodeToString(token)
				for _, saved := range g.Equipment {
					if saved.GetGuid() == guid {
						return nil, errors.New("装备实例 GUID 冲突")
					}
				}
				equip := &pb.Equipment{Guid: proto.String(guid), EquipId: proto.String(key), Level: proto.Int32(1), RefineLv: proto.Int32(0), RefineExp: proto.Int32(0), Star: proto.Int32(0), StarExp: proto.Int32(0), LastHeroId: proto.String(""), CoinPrice: proto.Int32(int32(price)), ForgeLevel: proto.Int32(0)}
				g.Equipment = append(g.Equipment, equip)
				gain.Equips = append(gain.Equips, proto.Clone(equip).(*pb.Equipment))
			}
			continue
		default:
			return nil, errors.New("章节宝箱奖励类型尚未恢复")
		}
		gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(num))})
	}
	return gain, nil
}
