package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeStory installs only response-recovery storage, without inserting quest progress.
func (s *GameService) InitializeStory(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.StoryQuestReceipt{})
}

func storyQuestReceiptHash(row db.StoryQuestReceipt) string {
	row.SourceHash = ""
	// DB fields omit binary frames from ordinary JSON; provenance includes both explicitly.
	raw, _ := json.Marshal(struct {
		Row      db.StoryQuestReceipt `json:"row"`
		Request  []byte               `json:"request"`
		Response []byte               `json:"response"`
	}{Row: row, Request: row.Request, Response: row.Response})
	return p6Digest(raw)
}

// storyMissionPassed 只接受正确章节内已结算的胜利记录。
func storyMissionPassed(g *model.GameState, chapter, mission string) bool {
	for _, copy := range g.Copy.GetPassedCopyInfo() {
		if copy.GetChapterId() == chapter && copy.GetMissionId() == mission && copy.GetStar() > 0 {
			return true
		}
	}
	return false
}

// storyQuestReward 按 APK 主线顺序校验条件，并在同一事务内领取全原主线奖励。
// NPC 对话由客户端本地 StoryManager 执行；OnStepFinished 只发送 questId。
func (s *GameService) storyQuestReward(ctx context.Context, id int64, questId string, stamps ...int64) ([]proto.Message, error) {
	stamp := int64(0)
	if len(stamps) > 0 {
		stamp = stamps[0]
	}
	return s.storyQuestRewardAt(ctx, id, questId, stamp, time.Now())
}

func (s *GameService) storyQuestRewardAt(ctx context.Context, id int64, questId string, stamp int64, now time.Time) ([]proto.Message, error) {
	if stamp < 0 {
		return nil, errors.New("主线任务请求时间戳无效")
	}
	requestRaw, err := proto.Marshal(&pb.CSQuestReward{QuestId: proto.String(questId)})
	if err != nil {
		return nil, err
	}
	row := s.table("StoryQuest", "questId", questId)
	if row == nil || tableNumber(row, "order") < 1 {
		return nil, errors.New("主线任务配置不存在")
	}
	var (
		result  *pb.SCQuestReward
		updated *pb.Player
		copies  *pb.SCCopyPassedInfo
	)
	err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		// The same profile lock covers reward, progress, durable response and concurrent retries.
		// Original zero-stamp socket calls retain the prior one-time claim semantics.
		if stamp > 0 {
			var receipt db.StoryQuestReceipt
			readErr := tx.First(&receipt, "player_id=? AND stamp=?", id, stamp).Error
			if readErr == nil {
				if receipt.QuestID != questId || !bytes.Equal(receipt.Request, requestRaw) {
					return errors.New(constUtil.StoryQuestReceiptConflict)
				}
				if receipt.Version != constUtil.StoryQuestReceiptVersion || receipt.Source != constUtil.StoryQuestReceiptSource || receipt.CreatedAt <= 0 || receipt.CreatedAt > math.MaxInt64-constUtil.StoryQuestReceiptLifetimeMS || receipt.ExpiresAt != receipt.CreatedAt+constUtil.StoryQuestReceiptLifetimeMS || receipt.SourceHash != storyQuestReceiptHash(receipt) || len(receipt.Response) > constUtil.MaxPacket || !slices.Contains(g.Quests, questId) {
					return errors.New("主线奖励已提交来源或回执损坏")
				}
				if now.UnixMilli() < receipt.CreatedAt || now.UnixMilli() >= receipt.ExpiresAt {
					return errors.New(constUtil.StoryQuestReceiptExpired)
				}
				result = &pb.SCQuestReward{}
				if err := proto.Unmarshal(receipt.Response, result); err != nil {
					return err
				}
				if result.GetGet() == nil {
					return errors.New("主线奖励回执缺少原奖励帧")
				}
				// Reuse the existing awarded-instance guard, including growth and ownership.
				if err := s.validateP5GrowthReceipt(g, &pb.SCItemUse{Get: result.GetGet()}); err != nil {
					return err
				}
				updated = proto.Clone(player).(*pb.Player)
				copies = proto.Clone(g.Copy).(*pb.SCCopyPassedInfo)
				return nil
			}
			if !errors.Is(readErr, gorm.ErrRecordNotFound) {
				return readErr
			}
		}
		if slices.Contains(g.Quests, questId) {
			return errors.New("任务奖励已领取")
		}
		// 已领取任务是客户端恢复当前任务的依据，禁止跳过任何前序任务。
		for _, previous := range s.Tables["StoryQuest"] {
			if tableNumber(previous, "order") < tableNumber(row, "order") && !slices.Contains(g.Quests, previous["questId"]) {
				return fmt.Errorf("请先完成前置任务 %s", previous["questId"])
			}
		}
		mission := s.table("CopyMission", "MissionId", row["missionId"])
		if mission == nil {
			return errors.New("主线对应关卡配置不存在")
		}
		if !storyMissionPassed(g, mission["ChapterId"], row["missionId"]) {
			return fmt.Errorf("请先完成第 %s 章第 %s 关", mission["ChapterId"], row["missionId"])
		}
		if row["type"] != "0" && row["type"] != "1" {
			return errors.New("Unsupported story quest type")
		}

		// 先校验整份原奖励，再使用现有实例构造器；实际装备奖励不丢失为简单物品。
		var content []string
		var experience int64
		for _, part := range strings.Split(row["rewards"], ";") {
			pair := strings.Split(part, ",")
			if len(pair) != 2 {
				return errors.New("Invalid story reward")
			}
			num, err := strconv.ParseInt(pair[1], 10, 32)
			if err != nil || num <= 0 {
				return errors.New("Invalid story reward quantity")
			}
			if pair[0] == "exp" {
				if experience > math.MaxInt64-num {
					return errors.New("Experience overflow")
				}
				experience += num
				continue
			}
			supported, err := s.openServerRewardSupported(part)
			if err != nil || !supported {
				return errors.New("Unsupported story reward type")
			}
			content = append(content, part)
		}
		gain := &pb.ResultObject{}
		if len(content) > 0 {
			var err error
			gain, err = s.openServerGain(player, g, strings.Join(content, ";"))
			if err != nil {
				return err
			}
		}
		if experience > 0 {
			if player.GetExp() < 0 || player.GetExp() > math.MaxInt64-experience || experience > math.MaxInt32 {
				return errors.New("Experience overflow")
			}
			if err := s.addCopySweepExperience(player, experience); err != nil {
				return err
			}
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String("exp"), Num: proto.Int32(int32(experience))})
		}
		result = &pb.SCQuestReward{Get: gain}
		g.Quests = append(g.Quests, questId)
		// TableGuide 的 order 记录已完成的大步骤，重登使用该行 loginGuide 续接。
		// 仅推进有明确主线对应关系的断点，养成和装备步骤仍由实际操作完成。
		guideSchedule := map[string]int32{"quest_1": 1, "quest_2": 4, "quest_3": 5, "quest_4": 8, "quest_5": 9}[questId]
		if player.GetNoviceGuideSchedule() < guideSchedule {
			player.NoviceGuideSchedule = proto.Int32(guideSchedule)
		}
		s.refreshCopyProgress(g)
		if stamp > 0 {
			encoded, err := proto.Marshal(result)
			if err != nil {
				return err
			}
			if now.UnixMilli() <= 0 || now.UnixMilli() > math.MaxInt64-constUtil.StoryQuestReceiptLifetimeMS || len(encoded) > constUtil.MaxPacket {
				return errors.New("主线奖励回执超出支持范围")
			}
			receipt := db.StoryQuestReceipt{PlayerID: id, Stamp: stamp, QuestID: questId, Request: requestRaw, Response: encoded, CreatedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + constUtil.StoryQuestReceiptLifetimeMS, Version: constUtil.StoryQuestReceiptVersion, Source: constUtil.StoryQuestReceiptSource}
			receipt.SourceHash = storyQuestReceiptHash(receipt)
			if err := tx.Create(&receipt).Error; err != nil {
				return err
			}
		}
		updated = proto.Clone(player).(*pb.Player)
		copies = proto.Clone(g.Copy).(*pb.SCCopyPassedInfo)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result, &pb.SCPlayer{Player: updated}, copies}, nil
}
