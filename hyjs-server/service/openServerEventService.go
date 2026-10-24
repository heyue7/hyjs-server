package service

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// openServerTracksRequest 跳过移动、心跳和只读查询，只检查能改变任务条件的业务。
func openServerTracksRequest(ctx context.Context) bool {
	switch ctx.Value(constUtil.GameRequestContextKey).(type) {
	case *pb.CSPlayerCreate, *pb.CSWarriorRecruitGuide, *pb.CSWarriorRecruit,
		*pb.CSWarriorTrainingReplace, *pb.CSWarriorBreached, *pb.CSWarriorTalent,
		*pb.CSWarriorStrengthen, *pb.CSWarriorAwake, *pb.CSAwakeFuse,
		*pb.CSFormationOn, *pb.CSFormationOff, *pb.CSFormationExChange,
		*pb.CSFormationChange, *pb.CSTaticalFormationLevelUp, *pb.CSUseTaticalFormation,
		*pb.CSItemUse, *pb.CSCombine, *pb.CSTreasureStrengthen, *pb.CSTreasureRefine,
		*pb.CSEquipStrengthen, *pb.CSEquipRefine, *pb.CSEquipStar, *pb.CSEquipForge,
		*pb.CSEquipDecompose, *pb.CSCopyFightEnd, *pb.CSGetOpenServerReward,
		*pb.CSRushFightEnd, *pb.CSShopBuy, *pb.CSQuestReward, *pb.CSOpenServerFundReward, *pb.CSActivityReward:
		return true
	default:
		return false
	}
}

// recordOpenServerSave 对比事务开始时的存档，只有新合成或新通关才增加事件。
func (s *GameService) recordOpenServerSave(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
	var before model.GameState
	if row.Game != nil && *row.Game != "" {
		if err := json.Unmarshal([]byte(*row.Game), &before); err != nil {
			return err
		}
	}
	request := tx.Statement.Context.Value(constUtil.GameRequestContextKey)
	if _, ok := request.(*pb.CSCombine); ok {
		current := game.TreasureCombineReceipt
		prior := before.TreasureCombineReceipt
		if current != nil && current.Response != nil {
			newGuid := ""
			for _, item := range current.Response.GetGet().GetTreas() {
				newGuid = item.GetGuid()
			}
			oldGuid := ""
			if prior != nil && prior.Response != nil {
				for _, item := range prior.Response.GetGet().GetTreas() {
					oldGuid = item.GetGuid()
				}
			}
			if newGuid != "" && newGuid != oldGuid {
				if err := RecordOpenServerCounter(tx, row.Id, constUtil.OpenServerActivityCounterComposeTreasure, 1); err != nil {
					return err
				}
				quality := tableNumber(s.table("Treasure", "id", current.TargetID), "quality")
				key := ""
				if quality == 18 {
					key = constUtil.OpenServerActivityCounterComposePurple
				} else if quality == 20 {
					key = constUtil.OpenServerActivityCounterComposeOrange
				}
				if key != "" {
					if err := RecordOpenServerCounter(tx, row.Id, key, 1); err != nil {
						return err
					}
				}
			}
		}
	}
	if _, ok := request.(*pb.CSCopyFightEnd); ok {
		current := game.Run
		prior := before.Run
		if current != nil && prior != nil && current.Complete && !prior.Complete {
			day := LocalPlayNow(tx.Statement.Context).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
			if err := RecordOpenServerCounter(tx, row.Id, constUtil.OpenServerActivityCounterCopyWin+day, 1); err != nil {
				return err
			}
		}
	}
	return s.RefreshOpenServerProgress(tx, player, game)
}
